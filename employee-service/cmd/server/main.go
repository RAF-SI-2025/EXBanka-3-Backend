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

	employeev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/gen/proto/employee/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/middleware"
	infrasvc "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()
	cfg.GRPCPort = envOrDefault("EMPLOYEE_GRPC_PORT", "9092")
	cfg.HTTPPort = envOrDefault("EMPLOYEE_HTTP_PORT", "8082")

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

	notifSvc := infrasvc.NewNotificationService(cfg)
	employeeH := handler.NewEmployeeHandler(cfg, db, notifSvc)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
			middleware.AuthInterceptor(cfg),
		),
	)

	employeev1.RegisterEmployeeServiceServer(grpcServer, employeeH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("gRPC listen failed", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Employee gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := employeev1.RegisterEmployeeServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register employee HTTP gateway", "error", err)
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
		slog.Info("Employee HTTP gateway listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down employee-service gracefully")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("employee-service stopped")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"employee-service"}`)
}

func envOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
