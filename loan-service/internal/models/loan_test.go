package models_test

import (
	"reflect"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/models"
)

// --- Loan model tests ---

func TestLoan_VrstaHas5Types(t *testing.T) {
	types := models.ValidLoanTypes()
	if len(types) != 5 {
		t.Errorf("expected 5 loan types, got %d", len(types))
	}
}

func TestLoan_VrstaContainsGotovinski(t *testing.T) {
	assertContains(t, models.ValidLoanTypes(), "gotovinski")
}

func TestLoan_VrstaContainsStambeni(t *testing.T) {
	assertContains(t, models.ValidLoanTypes(), "stambeni")
}

func TestLoan_VrstaContainsAuto(t *testing.T) {
	assertContains(t, models.ValidLoanTypes(), "auto")
}

func TestLoan_VrstaContainsRefinansirajuci(t *testing.T) {
	assertContains(t, models.ValidLoanTypes(), "refinansirajuci")
}

func TestLoan_VrstaContainsStudentski(t *testing.T) {
	assertContains(t, models.ValidLoanTypes(), "studentski")
}

func TestLoan_StatusHasExpectedValues(t *testing.T) {
	expected := []string{"zahtev", "odobren", "odbijen", "aktivan", "zatvoren"}
	statuses := models.ValidLoanStatuses()
	for _, e := range expected {
		assertContains(t, statuses, e)
	}
}

func TestLoan_InterestTypesAreCorrect(t *testing.T) {
	types := models.ValidInterestTypes()
	assertContains(t, types, "fiksna")
	assertContains(t, types, "varijabilna")
}

func TestLoan_HasRequiredFields(t *testing.T) {
	typ := reflect.TypeOf(models.Loan{})
	required := []string{"ID", "Vrsta", "BrojKredita", "Iznos", "Period",
		"KamatnaStopa", "TipKamate", "IznosRate", "Status", "ClientID", "CurrencyID"}
	for _, name := range required {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("Loan is missing required field %q", name)
		}
	}
}

func TestLoan_BrojKreditaHasUniqueIndex(t *testing.T) {
	typ := reflect.TypeOf(models.Loan{})
	f, ok := typ.FieldByName("BrojKredita")
	if !ok {
		t.Fatal("BrojKredita field not found on Loan")
	}
	tag := f.Tag.Get("gorm")
	if !containsStr(tag, "uniqueIndex") {
		t.Errorf("BrojKredita gorm tag %q does not contain 'uniqueIndex'", tag)
	}
}

func TestLoan_StatusDefaultIsZahtev(t *testing.T) {
	typ := reflect.TypeOf(models.Loan{})
	f, ok := typ.FieldByName("Status")
	if !ok {
		t.Fatal("Status field not found on Loan")
	}
	tag := f.Tag.Get("gorm")
	if !containsStr(tag, "default:'zahtev'") {
		t.Errorf("Status gorm tag %q does not contain \"default:'zahtev'\", got %q", tag, tag)
	}
}

// --- LoanInstallment model tests ---

func TestLoanInstallment_HasRequiredFields(t *testing.T) {
	typ := reflect.TypeOf(models.LoanInstallment{})
	required := []string{"ID", "LoanID", "RedniBroj", "Iznos",
		"KamataStopaSnapshot", "DatumDospeca", "Status"}
	for _, name := range required {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("LoanInstallment is missing required field %q", name)
		}
	}
}

func TestLoanInstallment_StatusHasExpectedValues(t *testing.T) {
	expected := []string{"ocekuje", "placena", "kasni"}
	statuses := models.ValidInstallmentStatuses()
	for _, e := range expected {
		assertContains(t, statuses, e)
	}
}

func TestLoanInstallment_StatusDefaultIsOcekuje(t *testing.T) {
	typ := reflect.TypeOf(models.LoanInstallment{})
	f, ok := typ.FieldByName("Status")
	if !ok {
		t.Fatal("Status field not found on LoanInstallment")
	}
	tag := f.Tag.Get("gorm")
	if !containsStr(tag, "default:'ocekuje'") {
		t.Errorf("LoanInstallment.Status gorm tag does not contain \"default:'ocekuje'\", got %q", tag)
	}
}

func TestLoanInstallment_DatumPlacanjaIsNullable(t *testing.T) {
	typ := reflect.TypeOf(models.LoanInstallment{})
	f, ok := typ.FieldByName("DatumPlacanja")
	if !ok {
		t.Fatal("DatumPlacanja field not found on LoanInstallment")
	}
	if f.Type.Kind() != reflect.Ptr {
		t.Errorf("DatumPlacanja should be a pointer (nullable), got kind %s", f.Type.Kind())
	}
}

// --- helpers ---

func assertContains(t *testing.T, slice []string, val string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			return
		}
	}
	t.Errorf("expected %q in slice %v", val, slice)
}

func containsStr(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
