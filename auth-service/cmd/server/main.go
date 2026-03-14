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

	authv1 "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/gen/proto/auth/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/middleware"
	infrasvc "github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/service"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

func main() {
	cfg := config.Load()
	cfg.GRPCPort = envOrDefault("AUTH_GRPC_PORT", "9091")
	cfg.HTTPPort = envOrDefault("AUTH_HTTP_PORT", "8081")

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
	if err := database.SeedDefaultAdmin(db); err != nil {
		log.Fatalf("Admin seeding failed: %v", err)
	}

	notifSvc := infrasvc.NewNotificationService(cfg)
	authH := handler.NewAuthHandler(cfg, db, notifSvc)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			middleware.LoggingInterceptor(),
		),
	)

	authv1.RegisterAuthServiceServer(grpcServer, authH)
	reflection.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen failed: %v", err)
	}

	go func() {
		log.Printf("Auth gRPC server listening on :%s", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	ctx := context.Background()
	gwMux := runtime.NewServeMux()
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcEndpoint := "localhost:" + cfg.GRPCPort

	if err := authv1.RegisterAuthServiceHandlerFromEndpoint(ctx, gwMux, grpcEndpoint, dialOpts); err != nil {
		log.Fatalf("Failed to register auth HTTP gateway: %v", err)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", healthCheck)
	httpMux.Handle("/", middleware.CORS(gwMux))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: httpMux,
	}

	go func() {
		log.Printf("Auth HTTP gateway listening on :%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down auth-service gracefully...")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
	log.Println("auth-service stopped")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"auth-service"}`)
}

func envOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
