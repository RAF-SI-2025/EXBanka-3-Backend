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

	paymentv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/gen/proto/payment/v1"
	prv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/gen/proto/payment_recipient/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/middleware"
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
	if err := database.SeedSifrePlacanja(db); err != nil {
		slog.Error("Sifre placanja seed failed", "error", err)
		os.Exit(1)
	}

	recipientH := handler.NewPaymentRecipientHandler(db)
	paymentH := handler.NewPaymentHandler(db, cfg)
	createPaymentHTTPH := handler.NewCreatePaymentHTTPHandler(db, cfg)
	mobileVerificationH := handler.NewPaymentMobileVerificationHandler(db, cfg)
	prenosHTTPH := handler.NewPrenosHTTPHandler(db, cfg)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
			middleware.AuthInterceptor(cfg),
		),
	)

	prv1.RegisterPaymentRecipientServiceServer(grpcServer, recipientH)
	paymentv1.RegisterPaymentServiceServer(grpcServer, paymentH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("gRPC listen failed", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Payment gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := prv1.RegisterPaymentRecipientServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register recipient HTTP gateway", "error", err)
		os.Exit(1)
	}
	if err := paymentv1.RegisterPaymentServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register payment HTTP gateway", "error", err)
		os.Exit(1)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	combinedPaymentHandler := handler.NewCombinedPaymentHandler(createPaymentHTTPH, mobileVerificationH, gwMux)
	httpMux.Handle("/api/v1/payments", middleware.CORS(combinedPaymentHandler))
	httpMux.Handle("/api/v1/payments/", middleware.CORS(combinedPaymentHandler))
	httpMux.Handle("/api/v1/prenos", middleware.CORS(prenosHTTPH))
	httpMux.Handle("/api/v1/prenos/", middleware.CORS(prenosHTTPH))
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		slog.Info("Payment HTTP gateway listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down payment-service gracefully")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("payment-service stopped")
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"payment-service"}`)
}
