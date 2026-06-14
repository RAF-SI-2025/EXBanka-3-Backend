package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/notification-service/internal/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	cfg := config.Load()

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

	repo := repository.NewNotificationRepository(db)
	emailSvc := service.NewSMTPEmailService(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)
	notifH := handler.NewNotificationHTTPHandler(cfg, repo)
	internalH := handler.NewInternalHTTPHandler(cfg, repo, emailSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheck)
	mux.HandleFunc("/ready", readinessCheck(sqlDB))

	// Caller-facing routes (JWT). ServeMux prefers the longest matching
	// pattern, so the exact sub-paths win over the trailing-slash prefix.
	mux.Handle("/api/v1/notifications", middleware.CORS(http.HandlerFunc(notifH.Collection)))
	mux.Handle("/api/v1/notifications/unread-count", middleware.CORS(http.HandlerFunc(notifH.UnreadCount)))
	mux.Handle("/api/v1/notifications/read-all", middleware.CORS(http.HandlerFunc(notifH.ReadAll)))
	mux.Handle("/api/v1/notifications/", middleware.CORS(http.HandlerFunc(notifH.Routes)))

	// Service-to-service emit (shared-secret, not gateway-exposed).
	mux.Handle("/internal/v1/notifications", http.HandlerFunc(internalH.Emit))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: mux,
	}

	go func() {
		slog.Info("Notification HTTP server listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down notification-service gracefully")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("notification-service stopped")
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"notification-service"}`)
}

func readinessCheck(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := db.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":"not_ready","service":"notification-service","dependency":"database"}`)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ready","service":"notification-service"}`)
	}
}
