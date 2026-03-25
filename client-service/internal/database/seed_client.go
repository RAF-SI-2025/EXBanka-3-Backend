package database

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"golang.org/x/crypto/pbkdf2"
	"gorm.io/gorm"
)

const (
	defaultClientEmail    = "klijent@bank.com"
	defaultClientPassword = "Klijent123!"
)

func generateSalt() (string, error) {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

func hashPassword(password, saltB64 string) (string, error) {
	saltBytes, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return "", fmt.Errorf("invalid salt encoding: %w", err)
	}
	hash := pbkdf2.Key([]byte(password), saltBytes, 100_000, 32, sha256.New)
	return base64.StdEncoding.EncodeToString(hash), nil
}

func SeedDefaultClient(db *gorm.DB) error {
	var existing models.Client
	if result := db.Where("email = ?", defaultClientEmail).First(&existing); result.Error == nil {
		slog.Info("Default client already exists, skipping seed")
		return nil
	}

	salt, err := generateSalt()
	if err != nil {
		return err
	}
	hashedPwd, err := hashPassword(defaultClientPassword, salt)
	if err != nil {
		return err
	}

	// Get client permissions
	var clientPerms []models.Permission
	db.Where("subject_type = ?", models.PermissionSubjectClient).Find(&clientPerms)

	client := models.Client{
		Ime:           "Petar",
		Prezime:       "Jovanovic",
		DatumRodjenja: 946684800, // 2000-01-01
		Pol:           "M",
		Email:         defaultClientEmail,
		BrojTelefona:  "0641234567",
		Adresa:        "Knez Mihailova 10, Beograd",
		Password:      hashedPwd,
		SaltPassword:  salt,
		Aktivan:       true,
		Permissions:   clientPerms,
	}

	if err := db.Create(&client).Error; err != nil {
		return fmt.Errorf("failed to create default client: %w", err)
	}

	slog.Info("Default client created", "email", defaultClientEmail)
	return nil
}
