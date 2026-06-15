package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/client-service/internal/service"
	"gorm.io/gorm"
)

func TestCreateClient_ValidationAndRepoErrors(t *testing.T) {
	ok := func() *mockClientRepo {
		return &mockClientRepo{
			emailExistsFn: func(string, uint) (bool, error) { return false, nil },
			createFn:      func(c *models.Client) error { c.ID = 1; return nil },
		}
	}

	// Invalid phone.
	in := validCreateClientInput()
	in.BrojTelefona = "abc"
	if _, _, err := newTestClientService(ok(), &mockPermRepo{}).CreateClient(in); err == nil {
		t.Error("expected error for invalid phone")
	}
	// Invalid email.
	in = validCreateClientInput()
	in.Email = "not-an-email"
	if _, _, err := newTestClientService(ok(), &mockPermRepo{}).CreateClient(in); err == nil {
		t.Error("expected error for invalid email")
	}
	// Date of birth in the future.
	in = validCreateClientInput()
	in.DatumRodjenja = time.Now().Add(48 * time.Hour).Unix()
	if _, _, err := newTestClientService(ok(), &mockPermRepo{}).CreateClient(in); err == nil {
		t.Error("expected error for future date of birth")
	}
	// Email already in use.
	exists := &mockClientRepo{emailExistsFn: func(string, uint) (bool, error) { return true, nil }}
	if _, _, err := newTestClientService(exists, &mockPermRepo{}).CreateClient(validCreateClientInput()); err == nil {
		t.Error("expected error for duplicate email")
	}
	// EmailExists repo error.
	existsErr := &mockClientRepo{emailExistsFn: func(string, uint) (bool, error) { return false, errors.New("db down") }}
	if _, _, err := newTestClientService(existsErr, &mockPermRepo{}).CreateClient(validCreateClientInput()); err == nil {
		t.Error("expected error when EmailExists fails")
	}
	// Create repo error.
	createErr := &mockClientRepo{
		emailExistsFn: func(string, uint) (bool, error) { return false, nil },
		createFn:      func(*models.Client) error { return errors.New("insert failed") },
	}
	if _, _, err := newTestClientService(createErr, &mockPermRepo{}).CreateClient(validCreateClientInput()); err == nil {
		t.Error("expected error when Create fails")
	}
}

func TestNewClientService_Constructs(t *testing.T) {
	// The constructor only wraps the handle in repositories; a zero-value *gorm.DB
	// is enough to exercise it without a driver dependency.
	if service.NewClientService(&config.Config{}, &gorm.DB{}) == nil {
		t.Fatal("NewClientService returned nil")
	}
}
