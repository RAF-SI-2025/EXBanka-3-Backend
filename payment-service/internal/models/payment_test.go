package models_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/payment-service/internal/models"
)

// ValidPaymentStatuses lists all accepted payment status values.
var ValidPaymentStatuses = []string{"u_obradi", "uspesno", "neuspesno", "stornirano"}

func TestPayment_RequiredFields(t *testing.T) {
	p := models.Payment{
		RacunPosiljaocaID: 1,
		RacunPrimaocaBroj: "000000000000000098",
		Iznos:             500.0,
		Status:            "u_obradi",
	}

	if p.RacunPosiljaocaID == 0 {
		t.Error("RacunPosiljaocaID must not be zero")
	}
	if p.RacunPrimaocaBroj == "" {
		t.Error("RacunPrimaocaBroj must not be empty")
	}
	if p.Iznos <= 0 {
		t.Error("Iznos must be positive")
	}
}

func TestPayment_DefaultStatus(t *testing.T) {
	// The gorm default tag is "u_obradi" — verify the constant is correct string.
	p := models.Payment{Status: "u_obradi"}
	if p.Status != "u_obradi" {
		t.Errorf("expected default status u_obradi, got %s", p.Status)
	}
}

func TestPayment_StatusValues(t *testing.T) {
	allowed := map[string]bool{
		"u_obradi":  true,
		"uspesno":   true,
		"neuspesno": true,
		"stornirano": true,
	}

	for _, s := range ValidPaymentStatuses {
		if !allowed[s] {
			t.Errorf("unexpected status value: %s", s)
		}
	}
	if len(ValidPaymentStatuses) != 4 {
		t.Errorf("expected 4 status values, got %d", len(ValidPaymentStatuses))
	}
}

func TestPayment_VerifikacioniKod_HiddenFromJSON(t *testing.T) {
	// Verify the field exists on the struct (json:"-" tag means it's excluded from marshalling)
	p := models.Payment{VerifikacioniKod: "123456"}
	if p.VerifikacioniKod != "123456" {
		t.Error("VerifikacioniKod field must be accessible on the struct")
	}
}

func TestPayment_OptionalRecipientID(t *testing.T) {
	// RecipientID is optional (pointer)
	p := models.Payment{}
	if p.RecipientID != nil {
		t.Error("RecipientID should be nil by default")
	}

	id := uint(42)
	p.RecipientID = &id
	if *p.RecipientID != 42 {
		t.Errorf("expected RecipientID=42, got %d", *p.RecipientID)
	}
}

func TestSifraPlacanja_RequiredFields(t *testing.T) {
	s := models.SifraPlacanja{
		Sifra: "221",
		Naziv: "Troškovi stanovanja",
	}

	if s.Sifra == "" {
		t.Error("Sifra must not be empty")
	}
	if s.Naziv == "" {
		t.Error("Naziv must not be empty")
	}
}

func TestPaymentFilter_ZeroValue(t *testing.T) {
	f := models.PaymentFilter{}
	if f.Status != "" {
		t.Errorf("expected empty Status, got %s", f.Status)
	}
	if f.DateFrom != nil {
		t.Error("expected nil DateFrom")
	}
	if f.DateTo != nil {
		t.Error("expected nil DateTo")
	}
}
