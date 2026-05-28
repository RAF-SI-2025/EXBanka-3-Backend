package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	exchangev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/gen/proto/exchange/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/cache"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/provider"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/util"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()
	closeTokenRevocation := util.ConfigureTokenRevocationFromEnv("exchange-service")
	defer closeTokenRevocation()

	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("DB connection failed", "error", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("DB handle unavailable", "error", err)
		os.Exit(1)
	}
	if err := database.Migrate(db); err != nil {
		slog.Error("DB migration failed", "error", err)
		os.Exit(1)
	}

	redisClient := cache.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := redisClient.Ping(ctx); err != nil {
			slog.Warn("Redis unavailable, continuing with local exchange caches", "error", err)
			_ = redisClient.Close()
			redisClient = nil
		} else {
			slog.Info("Redis connected for exchange-service cache/coordination", "addr", cfg.RedisAddr, "db", cfg.RedisDB)
			defer redisClient.Close()
		}
		cancel()
	}

	// Shared rate provider: same static rates used by the exchange rates page, with caching.
	staticRates := provider.NewStaticRateProvider()
	rateProvider := provider.NewCachedProvider(staticRates, staticRates, 24*time.Hour)
	if redisClient != nil {
		rateProvider.WithRedis(redisClient)
	}

	// Build shared repos and services before starting cron so they're reusable.
	marketRepo := repository.NewMarketRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	portfolioRepo := repository.NewPortfolioRepository(db)
	otcRepo := repository.NewOtcRepository(db)
	sagaRepo := repository.NewSagaRepository(db)
	taxRepo := repository.NewTaxRepository(db)
	taxSvc := service.NewTaxService(taxRepo, marketRepo, rateProvider)
	portfolioSvc := service.NewPortfolioService(
		portfolioRepo,
		taxSvc,
		marketRepo,
		orderRepo,
	)

	sagaOrchestrator := service.NewSagaOrchestrator(sagaRepo, db)
	sagaRetryRunner := service.NewSagaRetryRunner(sagaRepo, otcRepo, sagaOrchestrator)

	fundRepo := repository.NewFundRepository(db)
	fundSvc := service.NewFundService(fundRepo, portfolioRepo, marketRepo, orderRepo, rateProvider)

	// Inter-bank protocol wiring (Celina 5). The registry parses
	// PARTNER_BANKS_JSON at startup and is the routing/auth source of
	// truth for every /interbank request, inbound or outbound.
	ibRegistry, err := interbank.NewRegistryFromJSON(
		interbank.RoutingNumber(cfg.OwnRoutingNumber),
		cfg.PartnerBanksJSON,
	)
	if err != nil {
		slog.Error("Inter-bank registry init failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Inter-bank registry loaded",
		"own_routing", cfg.OwnRoutingNumber,
		"partner_count", len(ibRegistry.All()),
	)
	ibInboundRepo := repository.NewInterbankInboundRepository(db)
	ibNegRepo := repository.NewInterbankOtcRepository(db)
	ibPendingRepo := repository.NewInterbankPendingTxRepository(db)
	ibOptionContractRepo := repository.NewInterbankOptionContractRepository(db)
	ibWalletRepo := repository.NewInterbankWalletRepository(db)
	ibPaymentRepo := repository.NewInterbankPaymentRepository(db)
	ibPaymentWalletRepo := repository.NewInterbankPaymentWalletRepository(db)
	ibClient := interbank.NewClient(ibRegistry)
	ibExerciseRepo := repository.NewInterbankExerciseRepository(db)
	ibOtcProcessor := interbank.NewOtcTxProcessor(db, ibRegistry, ibNegRepo, ibPendingRepo, ibOptionContractRepo, ibWalletRepo)
	ibPaymentProcessor := interbank.NewPaymentTxProcessor(db, ibRegistry, ibPaymentRepo, ibPaymentWalletRepo)
	ibExerciseProcessor := interbank.NewExerciseTxProcessor(db, ibRegistry, ibNegRepo, ibExerciseRepo, portfolioRepo, marketRepo, ibWalletRepo)
	ibDispatchProcessor := interbank.NewDispatchTxProcessor(ibOtcProcessor, ibPaymentProcessor, ibExerciseProcessor, ibPaymentRepo, ibPendingRepo, ibExerciseRepo)
	ibServer := interbank.NewServer(ibRegistry, ibInboundRepo, ibDispatchProcessor)
	ibOtcH := interbank.NewOTCHandler(ibRegistry, portfolioRepo, interbank.StubDisplayNameResolver{})
	ibNegH := interbank.NewNegotiationsHandler(ibRegistry, ibNegRepo, ibClient, db, ibWalletRepo)

	ibReconcile := service.NewInterbankReconcileRunner(db, ibRegistry, ibClient, ibPaymentRepo, ibPaymentWalletRepo)
	ibPublicStockRepo := repository.NewRemotePublicStockRepository(db)
	ibPublicStockCache := service.NewPublicStockCacheRunner(ibRegistry, ibClient, ibPublicStockRepo)

	cronScheduler := service.StartCronJobs(db, portfolioSvc, rateProvider, sagaRetryRunner, fundSvc, ibReconcile, ibPublicStockCache)

	go func() {
		slog.Info("Running market data seed in background...")
		if err := database.SeedMarketData(db); err != nil {
			slog.Error("Market seed failed", "error", err)
		}
	}()
	defer cronScheduler.Stop()

	exchangeH := handler.NewExchangeHandler()
	marketProvider := provider.NewDatabaseMarketProvider(marketRepo)
	marketSvc := service.NewMarketService(marketProvider)
	marketH := handler.NewMarketHTTPHandler(cfg, marketSvc, marketRepo)

	orderSvc := service.NewOrderService(orderRepo, marketRepo, rateProvider)
	orderH := handler.NewOrderHTTPHandler(cfg, orderSvc).WithFundService(fundSvc)
	portfolioH := handler.NewPortfolioHTTPHandler(cfg, portfolioSvc)
	otcSvc := service.NewOtcService(portfolioRepo, otcRepo).WithOrchestrator(sagaOrchestrator)
	otcH := handler.NewOtcHTTPHandler(cfg, otcSvc).WithSagaQuerier(sagaRepo)

	taxCollector := service.NewTaxCollector(taxSvc, orderRepo, taxRepo)
	taxH := handler.NewTaxHTTPHandler(cfg, taxSvc, taxCollector)

	fundH := handler.NewFundHTTPHandler(cfg, fundSvc)

	ibOtcLocalH := handler.NewInterbankOtcHTTPHandler(
		cfg, ibRegistry, ibClient, ibNegRepo, ibNegH, ibPublicStockRepo,
		ibOptionContractRepo, ibExerciseRepo, ibWalletRepo, portfolioRepo, marketRepo, db,
	)
	ibPaymentLocalH := handler.NewInterbankPaymentHTTPHandler(cfg, ibRegistry, ibClient, ibPaymentRepo, ibPaymentWalletRepo, db)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
			middleware.AuthInterceptor(cfg),
		),
	)

	exchangev1.RegisterExchangeServiceServer(grpcServer, exchangeH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("gRPC listen failed", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Exchange gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := exchangev1.RegisterExchangeServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register exchange HTTP gateway", "error", err)
		os.Exit(1)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	httpMux.HandleFunc("/ready", readinessCheck(sqlDB))
	httpMux.Handle("/api/v1/exchanges", middleware.CORS(http.HandlerFunc(marketH.ListExchanges)))
	httpMux.Handle("/api/v1/exchanges/", middleware.CORS(http.HandlerFunc(marketH.ExchangeRoutes)))
	httpMux.Handle("/api/v1/listings", middleware.CORS(http.HandlerFunc(marketH.ListListings)))
	httpMux.Handle("/api/v1/listings/", middleware.CORS(http.HandlerFunc(marketH.ListingRoutes)))
	httpMux.Handle("/api/v1/portfolio", middleware.CORS(http.HandlerFunc(portfolioH.PortfolioCollection)))
	httpMux.Handle("/api/v1/portfolio/", middleware.CORS(http.HandlerFunc(portfolioH.PortfolioRoutes)))
	httpMux.Handle("/api/v1/otc/", middleware.CORS(http.HandlerFunc(otcH.OtcRoutes)))
	httpMux.Handle("/api/v1/orders", middleware.CORS(http.HandlerFunc(orderH.OrdersCollection)))
	httpMux.Handle("/api/v1/orders/", middleware.CORS(http.HandlerFunc(orderH.OrderRoutes)))
	httpMux.Handle("/api/v1/tax/", middleware.CORS(http.HandlerFunc(taxH.TaxRoutes)))
	httpMux.Handle("/api/v1/funds", middleware.CORS(http.HandlerFunc(fundH.FundRoutes)))
	httpMux.Handle("/api/v1/funds/", middleware.CORS(http.HandlerFunc(fundH.FundRoutes)))
	httpMux.Handle("/api/v1/interbank-otc/", middleware.CORS(http.HandlerFunc(ibOtcLocalH.Routes)))
	httpMux.Handle("/api/v1/payments/cross-bank", middleware.CORS(http.HandlerFunc(ibPaymentLocalH.Routes)))
	httpMux.Handle("/api/v1/payments/cross-bank/", middleware.CORS(http.HandlerFunc(ibPaymentLocalH.Routes)))
	// Inter-bank wire endpoint — partner-bank traffic, no CORS, X-Api-Key auth.
	httpMux.Handle("/interbank", ibServer)
	httpMux.Handle("/public-stock", interbank.AuthMiddleware(ibRegistry, http.HandlerFunc(ibOtcH.PublicStock)))
	httpMux.Handle("/user/", interbank.AuthMiddleware(ibRegistry, http.HandlerFunc(ibOtcH.UserInfo)))
	httpMux.Handle("/negotiations", interbank.AuthMiddleware(ibRegistry, http.HandlerFunc(ibNegH.Collection)))
	httpMux.Handle("/negotiations/", interbank.AuthMiddleware(ibRegistry, http.HandlerFunc(ibNegH.Item)))
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		slog.Info("Exchange HTTP gateway listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down exchange-service gracefully")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("exchange-service stopped")
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"exchange-service"}`)
}

func readinessCheck(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"not_ready","service":"exchange-service","dependency":"database"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ready","service":"exchange-service"}`)
	}
}
