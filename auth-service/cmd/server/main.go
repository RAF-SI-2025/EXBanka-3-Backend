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

	authv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/gen/proto/auth/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/middleware"
	infrasvc "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()
	cfg.GRPCPort = envOrDefault("AUTH_GRPC_PORT", "9091")
	cfg.HTTPPort = envOrDefault("AUTH_HTTP_PORT", "8081")
	closeTokenRevocation := util.ConfigureTokenRevocationFromEnv("auth-service")
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
	if err := database.SeedPermissions(db); err != nil {
		slog.Error("Permission seeding failed", "error", err)
		os.Exit(1)
	}
	if err := database.SeedDefaultAdmin(db); err != nil {
		slog.Error("Admin seeding failed", "error", err)
		os.Exit(1)
	}
	if err := database.SeedDefaultEmployees(db); err != nil {
		slog.Error("Employee seeding failed", "error", err)
		os.Exit(1)
	}

	notifSvc := infrasvc.NewNotificationService(cfg)
	authH := handler.NewAuthHandler(cfg, db, notifSvc)
	clientAuthH := handler.NewClientAuthHandler(cfg, db, notifSvc)
	logoutH := handler.NewLogoutHandler(cfg)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
		),
	)

	authv1.RegisterAuthServiceServer(grpcServer, authH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		slog.Error("gRPC listen failed", "error", err)
		os.Exit(1)
	}

	go func() {
		slog.Info("Auth gRPC server listening", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := authv1.RegisterAuthServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		slog.Error("Failed to register auth HTTP gateway", "error", err)
		os.Exit(1)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	httpMux.HandleFunc("/ready", readinessCheck(sqlDB))
	httpMux.Handle("/api/v1/auth/client/login", middleware.CORS(http.HandlerFunc(clientAuthH.Login)))
	httpMux.Handle("/api/v1/auth/client/activate", middleware.CORS(http.HandlerFunc(clientAuthH.Activate)))
	httpMux.Handle("/api/v1/auth/logout", middleware.CORS(http.HandlerFunc(logoutH.Logout)))
	httpMux.Handle("/api/v1/auth/client/logout", middleware.CORS(http.HandlerFunc(logoutH.Logout)))
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		slog.Info("Auth HTTP gateway listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down auth-service gracefully")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("auth-service stopped")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"auth-service"}`)
}

func readinessCheck(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"not_ready","service":"auth-service","dependency":"database"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ready","service":"auth-service"}`)
	}
}

func envOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
