package database

import (
	"fmt"
	"log"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
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

	log.Printf("Connected to PostgreSQL at %s:%s/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)
	return db, nil
}

func Migrate(db *gorm.DB) error {
	log.Println("Running client-service database migrations...")
	if err := db.AutoMigrate(
		&models.Client{},
		&models.Permission{},
	); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("Client-service migrations complete")
	return nil
}

func SeedPermissions(db *gorm.DB) error {
	if err := db.Model(&models.Permission{}).
		Where("subject_type = '' OR subject_type IS NULL").
		Update("subject_type", models.PermissionSubjectEmployee).Error; err != nil {
		return fmt.Errorf("failed to backfill permission subject types: %w", err)
	}

	for _, perm := range models.DefaultPermissions {
		p := perm
		result := db.Where(models.Permission{Name: p.Name}).Assign(models.Permission{
			Description: p.Description,
			SubjectType: p.SubjectType,
		}).FirstOrCreate(&p)
		if result.Error != nil {
			return fmt.Errorf("failed to seed permission %q: %w", p.Name, result.Error)
		}
	}

	log.Println("Permissions seeded")
	return nil
}
