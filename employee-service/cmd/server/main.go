package main

import (
	"context"
	"fmt"
	"log"
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
	cfg := config.Load()
	cfg.GRPCPort = envOrDefault("EMPLOYEE_GRPC_PORT", "9092")
	cfg.HTTPPort = envOrDefault("EMPLOYEE_HTTP_PORT", "8082")

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("DB connection failed: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("DB migration failed: %v", err)
	}
	if err := database.SeedPermissions(db); err != nil {
		log.Fatalf("Permission seeding failed: %v", err)
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
		log.Fatalf("gRPC listen failed: %v", err)
	}

	go func() {
		log.Printf("Employee gRPC server listening on :%s", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := employeev1.RegisterEmployeeServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		log.Fatalf("Failed to register employee HTTP gateway: %v", err)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		log.Printf("Employee HTTP gateway listening on :%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down employee-service gracefully...")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
	log.Println("employee-service stopped")
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
