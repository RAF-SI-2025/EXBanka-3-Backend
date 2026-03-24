package database

import (
	"fmt"
	"log/slog"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	slog.Info("Loan-service connected to PostgreSQL", "host", cfg.DBHost, "db", cfg.DBName)
	return db, nil
}

func Migrate(db *gorm.DB) error {
	slog.Info("Running loan-service database migrations...")
	if err := db.AutoMigrate(
		&models.Loan{},
		&models.LoanInstallment{},
	); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	slog.Info("Loan-service migrations complete")
	return nil
}
