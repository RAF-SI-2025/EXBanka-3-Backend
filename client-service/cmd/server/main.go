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

	clientv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/gen/proto/client/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/middleware"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()
	cfg.GRPCPort = envOrDefault("CLIENT_GRPC_PORT", "9093")
	cfg.HTTPPort = envOrDefault("CLIENT_HTTP_PORT", "8083")

	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("DB connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.Migrate(db); err != nil {
		slog.Error("DB migration failed", "error", err)
		os.Exit(1)
	}
	if err := database.SeedPermissions(db); err != nil {
		slog.Error("Permission seeding failed", "error", err)
		os.Exit(1)
	}

	clientH := handler.NewClientHandler(cfg, db)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
			middleware.AuthInterceptor(cfg),
		),
	)

	clientv1.RegisterClientServiceServer(grpcServer, clientH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("gRPC listen failed", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Client gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := clientv1.RegisterClientServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register client HTTP gateway", "error", err)
		os.Exit(1)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		slog.Info("Client HTTP gateway listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down client-service gracefully")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("client-service stopped")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"client-service"}`)
}

func envOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
