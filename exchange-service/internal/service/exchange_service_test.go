package service_test

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// --- mock rate provider ---

type mockRateProvider struct {
	rates    map[string]map[string]float64
	allRates []service.ExchangeRate
	fetchErr error
}

func (m *mockRateProvider) GetRate(from, to string) (float64, error) {
	if m.fetchErr != nil {
		return 0, m.fetchErr
	}
	if from == to {
		return 1.0, nil
	}
	if r, ok := m.rates[from]; ok {
		if rate, ok := r[to]; ok {
			return rate, nil
		}
	}
	return 0, errors.New("unknown currency pair")
}

func (m *mockRateProvider) GetAllRates() []service.ExchangeRate {
	return m.allRates
}

func newMockProvider() *mockRateProvider {
	return &mockRateProvider{
		rates: map[string]map[string]float64{
			"EUR": {"RSD": 117.0, "USD": 1.08, "CHF": 0.96},
			"USD": {"EUR": 0.926, "RSD": 108.33},
			"RSD": {"EUR": 0.00855, "USD": 0.00923},
		},
		allRates: []service.ExchangeRate{
			{From: "EUR", To: "RSD", Rate: 117.0},
			{From: "EUR", To: "USD", Rate: 1.08},
			{From: "USD", To: "EUR", Rate: 0.926},
			{From: "RSD", To: "EUR", Rate: 0.00855},
		},
	}
}

// --- tests ---

func TestGetRateList_ReturnsNonEmptyList(t *testing.T) {
	svc := service.NewExchangeServiceWithProvider(newMockProvider())

	rates := svc.GetRateList()

	if len(rates) == 0 {
		t.Fatal("expected non-empty rate list, got empty")
	}
}

func TestGetRateList_ContainsCurrencyPairs(t *testing.T) {
	svc := service.NewExchangeServiceWithProvider(newMockProvider())

	rates := svc.GetRateList()

	found := false
	for _, r := range rates {
		if r.From == "EUR" && r.To == "RSD" {
			found = true
			if r.Rate != 117.0 {
				t.Errorf("expected EUR→RSD rate=117.0, got %f", r.Rate)
			}
			break
		}
	}
	if !found {
		t.Error("expected EUR→RSD pair in rate list")
	}
}

func TestCalculateExchange_EURtoRSD_CorrectConversion(t *testing.T) {
	svc := service.NewExchangeServiceWithProvider(newMockProvider())

	result, err := svc.CalculateExchange("EUR", "RSD", 100)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OutputAmount != 11700 {
		t.Errorf("expected 11700 RSD, got %f", result.OutputAmount)
	}
	if result.Rate != 117.0 {
		t.Errorf("expected rate=117.0, got %f", result.Rate)
	}
	if result.FromCurrency != "EUR" || result.ToCurrency != "RSD" {
		t.Errorf("unexpected currencies: %s→%s", result.FromCurrency, result.ToCurrency)
	}
}

func TestCalculateExchange_SameCurrency_ReturnsSameAmount(t *testing.T) {
	svc := service.NewExchangeServiceWithProvider(newMockProvider())

	result, err := svc.CalculateExchange("EUR", "EUR", 250)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OutputAmount != 250 {
		t.Errorf("expected same amount 250, got %f", result.OutputAmount)
	}
	if result.Rate != 1.0 {
		t.Errorf("expected rate=1.0 for same currency, got %f", result.Rate)
	}
}

func TestCalculateExchange_InvalidCurrency_ReturnsError(t *testing.T) {
	svc := service.NewExchangeServiceWithProvider(newMockProvider())

	_, err := svc.CalculateExchange("EUR", "XYZ", 100)

	if err == nil {
		t.Fatal("expected error for unknown currency, got nil")
	}
}

func TestCalculateExchange_NegativeAmount_ReturnsError(t *testing.T) {
	svc := service.NewExchangeServiceWithProvider(newMockProvider())

	_, err := svc.CalculateExchange("EUR", "RSD", -50)

	if err == nil {
		t.Fatal("expected error for negative amount, got nil")
	}
}

func TestCalculateExchange_ProviderError_ReturnsError(t *testing.T) {
	provider := newMockProvider()
	provider.fetchErr = errors.New("API unavailable")
	svc := service.NewExchangeServiceWithProvider(provider)

	_, err := svc.CalculateExchange("EUR", "USD", 100)

	if err == nil {
		t.Fatal("expected error when provider fails, got nil")
	}
}
