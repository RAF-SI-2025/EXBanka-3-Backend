package handler_test

import (
	"context"
	"errors"
	"testing"

	exchangev1 "github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/gen/proto/exchange/v1"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/handler"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// --- mock service ---

type mockExchangeService struct {
	rates  []service.ExchangeRate
	result *service.ExchangeResult
	err    error
}

func (m *mockExchangeService) GetRateList() []service.ExchangeRate {
	return m.rates
}

func (m *mockExchangeService) CalculateExchange(from, to string, amount float64) (*service.ExchangeResult, error) {
	return m.result, m.err
}

func newMockSvc() *mockExchangeService {
	return &mockExchangeService{
		rates: []service.ExchangeRate{
			{From: "EUR", To: "RSD", Rate: 117.0},
			{From: "EUR", To: "USD", Rate: 1.08},
			{From: "USD", To: "EUR", Rate: 0.926},
		},
		result: &service.ExchangeResult{
			FromCurrency: "EUR",
			ToCurrency:   "RSD",
			InputAmount:  100,
			OutputAmount: 11700,
			Rate:         117.0,
		},
	}
}

// --- GetRateList tests ---

func TestGetRateList_ReturnsAllRates(t *testing.T) {
	h := handler.NewExchangeHandlerWithService(newMockSvc())

	resp, err := h.GetRateList(context.Background(), &exchangev1.GetRateListRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Rates) != 3 {
		t.Errorf("expected 3 rates, got %d", len(resp.Rates))
	}
}

func TestGetRateList_RatesHaveCorrectFields(t *testing.T) {
	h := handler.NewExchangeHandlerWithService(newMockSvc())

	resp, err := h.GetRateList(context.Background(), &exchangev1.GetRateListRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, r := range resp.Rates {
		if r.From == "EUR" && r.To == "RSD" && r.Rate == 117.0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected EUR→RSD rate=117.0 in response")
	}
}

func TestGetRateList_EmptyListReturnsEmptyResponse(t *testing.T) {
	svc := &mockExchangeService{rates: []service.ExchangeRate{}}
	h := handler.NewExchangeHandlerWithService(svc)

	resp, err := h.GetRateList(context.Background(), &exchangev1.GetRateListRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Rates) != 0 {
		t.Errorf("expected empty rates, got %d", len(resp.Rates))
	}
}

// --- CalculateExchange tests ---

func TestCalculateExchange_Success(t *testing.T) {
	h := handler.NewExchangeHandlerWithService(newMockSvc())

	resp, err := h.CalculateExchange(context.Background(), &exchangev1.CalculateExchangeRequest{
		FromCurrency: "EUR",
		ToCurrency:   "RSD",
		Amount:       100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.OutputAmount != 11700 {
		t.Errorf("expected output=11700, got %f", resp.OutputAmount)
	}
	if resp.Rate != 117.0 {
		t.Errorf("expected rate=117.0, got %f", resp.Rate)
	}
	if resp.FromCurrency != "EUR" || resp.ToCurrency != "RSD" {
		t.Errorf("unexpected currencies: %s→%s", resp.FromCurrency, resp.ToCurrency)
	}
}

func TestCalculateExchange_MissingFromCurrency_ReturnsError(t *testing.T) {
	h := handler.NewExchangeHandlerWithService(newMockSvc())

	_, err := h.CalculateExchange(context.Background(), &exchangev1.CalculateExchangeRequest{
		FromCurrency: "",
		ToCurrency:   "RSD",
		Amount:       100,
	})

	if err == nil {
		t.Fatal("expected error for missing from_currency, got nil")
	}
}

func TestCalculateExchange_MissingToCurrency_ReturnsError(t *testing.T) {
	h := handler.NewExchangeHandlerWithService(newMockSvc())

	_, err := h.CalculateExchange(context.Background(), &exchangev1.CalculateExchangeRequest{
		FromCurrency: "EUR",
		ToCurrency:   "",
		Amount:       100,
	})

	if err == nil {
		t.Fatal("expected error for missing to_currency, got nil")
	}
}

func TestCalculateExchange_ServiceError_ReturnsError(t *testing.T) {
	svc := &mockExchangeService{err: errors.New("unknown currency pair")}
	h := handler.NewExchangeHandlerWithService(svc)

	_, err := h.CalculateExchange(context.Background(), &exchangev1.CalculateExchangeRequest{
		FromCurrency: "EUR",
		ToCurrency:   "XYZ",
		Amount:       100,
	})

	if err == nil {
		t.Fatal("expected error for unknown currency, got nil")
	}
}

func TestCalculateExchange_InputAmountPreserved(t *testing.T) {
	h := handler.NewExchangeHandlerWithService(newMockSvc())

	resp, err := h.CalculateExchange(context.Background(), &exchangev1.CalculateExchangeRequest{
		FromCurrency: "EUR",
		ToCurrency:   "RSD",
		Amount:       100,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.InputAmount != 100 {
		t.Errorf("expected input_amount=100, got %f", resp.InputAmount)
	}
}
