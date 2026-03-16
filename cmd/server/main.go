package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/internal/swagger"
)

func main() {
	httpPort := envOrDefault("HTTP_PORT", "8080")

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/swagger.json", swagger.HandlerJSON)
	httpMux.HandleFunc("/swagger-ui", swagger.HandlerUI)
	httpMux.HandleFunc("/health", healthCheck)

	httpServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: httpMux,
	}

	go func() {
		slog.Info("Backend shared runtime listening", "port", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down gracefully...")
	if err := httpServer.Close(); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("Server stopped")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"EXBanka"}`)
}

func envOrDefault(key, defaultVal string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultVal
}
