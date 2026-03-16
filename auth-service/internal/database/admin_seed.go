package database

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/util"
	"gorm.io/gorm"
)

func SeedDefaultAdmin(db *gorm.DB) error {
	const defaultAdminPassword = "Admin123!"

	var existing models.Employee
	if result := db.Where("email = ?", "admin@bank.com").First(&existing); result.Error == nil {
		ok, err := util.VerifyPassword(defaultAdminPassword, existing.SaltPassword, existing.Password)
		if err != nil {
			return fmt.Errorf("failed to verify existing admin password: %w", err)
		}

		var allPerms []models.Permission
		if err := db.Where("subject_type = ?", models.PermissionSubjectEmployee).Find(&allPerms).Error; err != nil {
			return fmt.Errorf("failed to fetch permissions: %w", err)
		}

		if ok && existing.Aktivan {
			if err := db.Model(&existing).Association("Permissions").Replace(allPerms); err != nil {
				return fmt.Errorf("failed to sync admin permissions: %w", err)
			}
			slog.Info("Admin already exists, skipping seed")
			return nil
		}

		salt, err := util.GenerateSalt()
		if err != nil {
			return fmt.Errorf("failed to generate salt: %w", err)
		}

		hashedPwd, err := util.HashPassword(defaultAdminPassword, salt)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		updates := map[string]interface{}{
			"password":      hashedPwd,
			"salt_password": salt,
			"aktivan":       true,
		}
		if err := db.Model(&existing).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to repair existing admin: %w", err)
		}
		if err := db.Model(&existing).Association("Permissions").Replace(allPerms); err != nil {
			return fmt.Errorf("failed to sync admin permissions: %w", err)
		}

		slog.Info("Default admin user repaired successfully")
		return nil
	} else if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing admin: %w", result.Error)
	}

	salt, err := util.GenerateSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	hashedPwd, err := util.HashPassword(defaultAdminPassword, salt)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	var allPerms []models.Permission
	if err := db.Where("subject_type = ?", models.PermissionSubjectEmployee).Find(&allPerms).Error; err != nil {
		return fmt.Errorf("failed to fetch permissions: %w", err)
	}

	admin := models.Employee{
		Ime:           "Admin",
		Prezime:       "User",
		DatumRodjenja: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Pol:           "M",
		Email:         "admin@bank.com",
		BrojTelefona:  "0601234567",
		Adresa:        "Admin Street 1",
		Username:      "admin",
		Password:      hashedPwd,
		SaltPassword:  salt,
		Pozicija:      "Administrator",
		Departman:     "IT",
		Aktivan:       true,
		Permissions:   allPerms,
	}

	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("failed to create admin: %w", err)
	}

	slog.Info("Default admin user created successfully")
	return nil
}
