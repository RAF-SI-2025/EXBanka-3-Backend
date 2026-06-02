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

	employeev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/gen/proto/employee/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/middleware"
	infrasvc "github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/service"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/employee-service/internal/util"
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
	closeTokenRevocation := util.ConfigureTokenRevocationFromEnv("employee-service")
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

	notifSvc := infrasvc.NewNotificationService(cfg)
	employeeSvc := infrasvc.NewEmployeeService(cfg, db, notifSvc)
	employeeH := handler.NewEmployeeHandlerWithService(employeeSvc, db)
	actuaryHTTPHandler := handler.NewActuaryHTTPHandler(cfg, employeeSvc, db)
	auditHTTPHandler := handler.NewAuditHTTPHandler(cfg, db)

	cronScheduler := infrasvc.StartCronJobs(employeeSvc)
	defer cronScheduler.Stop()

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
	httpMux.HandleFunc("/ready", readinessCheck(sqlDB))
	httpMux.Handle("/api/v1/actuaries", middleware.CORS(http.HandlerFunc(actuaryHTTPHandler.ListActuaries)))
	httpMux.Handle("/api/v1/actuaries/", middleware.CORS(http.HandlerFunc(actuaryHTTPHandler.ActuaryRoutes)))
	httpMux.Handle("/api/v1/audit-logs", middleware.CORS(http.HandlerFunc(auditHTTPHandler.ListAuditLogs)))
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

func readinessCheck(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"not_ready","service":"employee-service","dependency":"database"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ready","service":"employee-service"}`)
	}
}

func envOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
