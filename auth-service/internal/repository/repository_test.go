package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/auth-service/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Employee{}, &models.Permission{}, &models.Token{}, &models.Client{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// =====================
// Constructor checks
// =====================

func TestNewEmployeeRepository(t *testing.T) {
	if repository.NewEmployeeRepository(nil) == nil {
		t.Error("expected non-nil")
	}
}

func TestNewClientRepository(t *testing.T) {
	if repository.NewClientRepository(nil) == nil {
		t.Error("expected non-nil")
	}
}

func TestNewTokenRepository(t *testing.T) {
	if repository.NewTokenRepository(nil) == nil {
		t.Error("expected non-nil")
	}
}

// =====================
// EmployeeRepository
// =====================

func TestEmployeeRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "auth_emp_byid")
	emp := models.Employee{Ime: "A", Prezime: "B", Email: "a@b.c", Pol: "M", Username: "ab", Password: "p", SaltPassword: "s", Aktivan: true}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := repository.NewEmployeeRepository(db)
	got, err := repo.FindByID(emp.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Email != "a@b.c" {
		t.Errorf("Email = %s", got.Email)
	}
}

func TestEmployeeRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "auth_emp_byid_nf")
	repo := repository.NewEmployeeRepository(db)
	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error")
	}
}

func TestEmployeeRepository_FindByEmail(t *testing.T) {
	db := newTestDB(t, "auth_emp_byemail")
	emp := models.Employee{Ime: "A", Prezime: "B", Email: "a@b.c", Pol: "M", Username: "ab", Password: "p", SaltPassword: "s", Aktivan: true}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := repository.NewEmployeeRepository(db)
	got, err := repo.FindByEmail("a@b.c")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if got.ID != emp.ID {
		t.Errorf("ID mismatch")
	}
}

func TestEmployeeRepository_FindByEmail_NotFound(t *testing.T) {
	db := newTestDB(t, "auth_emp_byemail_nf")
	repo := repository.NewEmployeeRepository(db)
	if _, err := repo.FindByEmail("nope@x.y"); err == nil {
		t.Error("expected error")
	}
}

func TestEmployeeRepository_UpdateFields(t *testing.T) {
	db := newTestDB(t, "auth_emp_update")
	emp := models.Employee{Ime: "A", Prezime: "B", Email: "a@b.c", Pol: "M", Username: "ab", Password: "p", SaltPassword: "s", Aktivan: false}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := repository.NewEmployeeRepository(db)

	if err := repo.UpdateFields(emp.ID, map[string]interface{}{"aktivan": true, "pozicija": "Manager"}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}

	got, _ := repo.FindByID(emp.ID)
	if !got.Aktivan {
		t.Error("expected Aktivan=true")
	}
	if got.Pozicija != "Manager" {
		t.Errorf("Pozicija = %s", got.Pozicija)
	}
}

// =====================
// ClientRepository
// =====================

func TestClientRepository_FindByEmail(t *testing.T) {
	db := newTestDB(t, "auth_client_byemail")
	c := models.Client{Ime: "A", Prezime: "B", Email: "c@d.e", Password: "p", SaltPassword: "s", Aktivan: true}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := repository.NewClientRepository(db)
	got, err := repo.FindByEmail("c@d.e")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("ID mismatch")
	}
}

func TestClientRepository_FindByEmail_NotFound(t *testing.T) {
	db := newTestDB(t, "auth_client_byemail_nf")
	repo := repository.NewClientRepository(db)
	if _, err := repo.FindByEmail("nope@x.y"); err == nil {
		t.Error("expected error")
	}
}

func TestClientRepository_FindByID(t *testing.T) {
	db := newTestDB(t, "auth_client_byid")
	c := models.Client{Ime: "A", Prezime: "B", Email: "c@d.e", Password: "p", SaltPassword: "s", Aktivan: true}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := repository.NewClientRepository(db)
	got, err := repo.FindByID(c.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Email != "c@d.e" {
		t.Errorf("Email = %s", got.Email)
	}
}

func TestClientRepository_FindByID_NotFound(t *testing.T) {
	db := newTestDB(t, "auth_client_byid_nf")
	repo := repository.NewClientRepository(db)
	if _, err := repo.FindByID(9999); err == nil {
		t.Error("expected error")
	}
}

func TestClientRepository_UpdateFields(t *testing.T) {
	db := newTestDB(t, "auth_client_update")
	c := models.Client{Ime: "A", Prezime: "B", Email: "c@d.e", Password: "p", SaltPassword: "s", Aktivan: false}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	repo := repository.NewClientRepository(db)

	if err := repo.UpdateFields(c.ID, map[string]interface{}{"aktivan": true}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}

	got, _ := repo.FindByID(c.ID)
	if !got.Aktivan {
		t.Error("expected Aktivan=true")
	}
}

// =====================
// TokenRepository
// =====================

func TestTokenRepository_Create(t *testing.T) {
	db := newTestDB(t, "auth_tok_create")
	repo := repository.NewTokenRepository(db)

	tok := &models.Token{
		EmployeeID: 1,
		Token:      "abc",
		Type:       models.TokenTypeReset,
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	if err := repo.Create(tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tok.ID == 0 {
		t.Error("expected ID set")
	}
}

func TestTokenRepository_FindValid_Found(t *testing.T) {
	db := newTestDB(t, "auth_tok_find_ok")
	emp := models.Employee{Ime: "A", Prezime: "B", Email: "x@y.z", Pol: "M", Username: "u", Password: "p", SaltPassword: "s", Aktivan: true}
	if err := db.Create(&emp).Error; err != nil {
		t.Fatalf("create emp: %v", err)
	}
	repo := repository.NewTokenRepository(db)

	tok := &models.Token{
		EmployeeID: emp.ID,
		Token:      "valid-token",
		Type:       models.TokenTypeReset,
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	if err := repo.Create(tok); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.FindValid("valid-token", models.TokenTypeReset)
	if err != nil {
		t.Fatalf("FindValid: %v", err)
	}
	if got.Token != "valid-token" {
		t.Errorf("Token = %s", got.Token)
	}
}

func TestTokenRepository_FindValid_Expired(t *testing.T) {
	db := newTestDB(t, "auth_tok_find_exp")
	repo := repository.NewTokenRepository(db)

	tok := &models.Token{
		EmployeeID: 1,
		Token:      "expired",
		Type:       models.TokenTypeReset,
		ExpiresAt:  time.Now().Add(-time.Hour),
	}
	if err := repo.Create(tok); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.FindValid("expired", models.TokenTypeReset); err == nil {
		t.Error("expected error for expired token")
	}
}

func TestTokenRepository_FindValid_Used(t *testing.T) {
	db := newTestDB(t, "auth_tok_find_used")
	repo := repository.NewTokenRepository(db)

	tok := &models.Token{
		EmployeeID: 1,
		Token:      "used",
		Type:       models.TokenTypeReset,
		ExpiresAt:  time.Now().Add(time.Hour),
		Used:       true,
	}
	if err := repo.Create(tok); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.FindValid("used", models.TokenTypeReset); err == nil {
		t.Error("expected error for used token")
	}
}

func TestTokenRepository_InvalidateEmployeeTokens(t *testing.T) {
	db := newTestDB(t, "auth_tok_invalidate")
	repo := repository.NewTokenRepository(db)

	for _, s := range []string{"t1", "t2"} {
		if err := repo.Create(&models.Token{
			EmployeeID: 5,
			Token:      s,
			Type:       models.TokenTypeReset,
			ExpiresAt:  time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	if err := repo.InvalidateEmployeeTokens(5, models.TokenTypeReset); err != nil {
		t.Fatalf("InvalidateEmployeeTokens: %v", err)
	}

	var count int64
	db.Model(&models.Token{}).Where("employee_id = ? AND used = ?", 5, true).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 used tokens, got %d", count)
	}
}
