package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/cron"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/database"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/middleware"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
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

	loanRepo := repository.NewLoanRepository(db)
	installmentRepo := repository.NewInstallmentRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	notifier := service.NewNotificationService(cfg)
	loanSvc := service.NewLoanServiceWithNotifier(db, loanRepo, installmentRepo, accountRepo, notifier)
	loanH := handler.NewLoanHandlerWithConfig(loanSvc, cfg, db)

	// Start cron jobs in the background.
	go runDailyCron(cron.NewInstallmentCollector(db, installmentRepo, loanRepo, accountRepo))
	go runMonthlyCron(cron.NewInterestRateUpdater(loanRepo, db))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthCheck)
	mux.Handle("/api/v1/loans/", middleware.CORS(loanH))

	httpServer := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: middleware.CORS(mux),
	}

	go func() {
		slog.Info("Loan-service HTTP server listening", "port", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down loan-service gracefully")
	if err := httpServer.Close(); err != nil {
		slog.Error("HTTP shutdown error", "error", err)
	}
	slog.Info("loan-service stopped")
}

// runDailyCron fires at 02:00 every day to collect due installments.
func runDailyCron(collector *cron.InstallmentCollector) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, now.Location())
		time.Sleep(time.Until(next))
		if err := collector.Run(time.Now()); err != nil {
			slog.Error("Installment collection cron failed", "error", err)
		}
	}
}

// runMonthlyCron fires on the 1st of each month to adjust variable interest rates.
func runMonthlyCron(updater *cron.InterestRateUpdater) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month()+1, 1, 3, 0, 0, 0, now.Location())
		time.Sleep(time.Until(next))
		if err := updater.Run(); err != nil {
			slog.Error("Interest rate update cron failed", "error", err)
		}
	}
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok","service":"loan-service"}`)
}
