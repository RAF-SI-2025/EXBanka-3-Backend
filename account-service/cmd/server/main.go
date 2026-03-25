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

	accountv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/gen/proto/account/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/account-service/internal/service"
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
	if err := database.SeedCurrencies(db); err != nil {
		slog.Error("Currency seed failed", "error", err)
		os.Exit(1)
	}
	if err := database.SeedSifreDelatnosti(db); err != nil {
		slog.Error("Sifre delatnosti seed failed", "error", err)
		os.Exit(1)
	}
	if err := database.SeedBankAccounts(db); err != nil {
		slog.Error("Bank accounts seed failed", "error", err)
		os.Exit(1)
	}

	accountH := handler.NewAccountHandler(db, cfg)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
			middleware.AuthInterceptor(cfg),
		),
	)

	accountv1.RegisterAccountServiceServer(grpcServer, accountH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("gRPC listen failed", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Account gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := accountv1.RegisterAccountServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register account HTTP gateway", "error", err)
		os.Exit(1)
	}

	firmaH := handler.NewFirmaHandler(db, cfg)
	createAccH := handler.NewCreateAccountHTTPHandler(db, cfg)

	accountRepo := repository.NewAccountRepository(db)
	currencyRepo := repository.NewCurrencyRepository(db)
	accountNotifSvc := service.NewNotificationService(cfg)
	accountSvc := service.NewAccountServiceWithRepos(accountRepo, currencyRepo, accountNotifSvc)
	listClientAccH := handler.NewListClientAccountsHTTPHandlerWithConfig(accountRepo, cfg)
	listAllAccH := handler.NewListAllAccountsHTTPHandler(accountSvc, cfg)
	currencyH := handler.NewCurrencyHTTPHandler(currencyRepo, cfg)

	cardRepo := repository.NewCardRepository(db)
	notifSvc := service.NewNotificationService(cfg)
	cardSvc := service.NewCardService(cardRepo, accountRepo, notifSvc)
	cardH := handler.NewCardHTTPHandlerWithConfig(cardSvc, cfg)

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	httpMux.Handle("/api/v1/firme", middleware.CORS(http.HandlerFunc(firmaH.Create)))
	httpMux.Handle("/api/v1/sifre-delatnosti", middleware.CORS(http.HandlerFunc(firmaH.ListSifreDelatnosti)))
	httpMux.Handle("/api/v1/accounts/create", middleware.CORS(createAccH))
	httpMux.Handle("/api/v1/accounts/search", middleware.CORS(listAllAccH))
	httpMux.Handle("/api/v1/accounts/client/", middleware.CORS(listClientAccH))
	httpMux.Handle("/api/v1/currencies", middleware.CORS(currencyH))
	httpMux.Handle("/api/v1/cards/", middleware.CORS(cardH))
	httpMux.Handle("/api/v1/cards", middleware.CORS(cardH))
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		slog.Info("Account HTTP gateway listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down account-service gracefully")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("account-service stopped")
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"account-service"}`)
}
