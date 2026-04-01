package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	exchangev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/gen/proto/exchange/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/provider"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()

	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("DB connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.Migrate(db); err != nil {
		slog.Error("DB migration failed", "error", err)
		os.Exit(1)
	}

	cronScheduler := service.StartCronJobs(db)

	go func() {
		slog.Info("Running market data seed in background...")
		if err := database.SeedMarketData(db); err != nil {
			slog.Error("Market seed failed", "error", err)
		}
	}()
	defer cronScheduler.Stop()

	exchangeH := handler.NewExchangeHandler()
	marketRepo := repository.NewMarketRepository(db)
	marketProvider := provider.NewDatabaseMarketProvider(marketRepo)
	marketSvc := service.NewMarketService(marketProvider)
	marketH := handler.NewMarketHTTPHandler(cfg, marketSvc, marketRepo)

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
	httpMux.Handle("/api/v1/exchanges", middleware.CORS(http.HandlerFunc(marketH.ListExchanges)))
	httpMux.Handle("/api/v1/exchanges/", middleware.CORS(http.HandlerFunc(marketH.ExchangeRoutes)))
	httpMux.Handle("/api/v1/listings", middleware.CORS(http.HandlerFunc(marketH.ListListings)))
	httpMux.Handle("/api/v1/listings/", middleware.CORS(http.HandlerFunc(marketH.ListingRoutes)))
	httpMux.Handle("/api/v1/portfolio", middleware.CORS(http.HandlerFunc(marketH.GetPortfolio)))
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
