package service_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/transfer-service/internal/service"
)

func TestHTTPExchangeRateService_GetRate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/exchange/rates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"rates":[
			{"from":"EUR","to":"RSD","rate":117.5},
			{"from":"USD","to":"RSD","rate":108.0}
		]}`))
	}))
	defer srv.Close()

	c := service.NewHTTPExchangeRateService(srv.URL)
	rate, err := c.GetRate("EUR", "RSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate != 117.5 {
		t.Errorf("expected 117.5, got %v", rate)
	}
}

func TestHTTPExchangeRateService_GetRate_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rates":[]}`))
	}))
	defer srv.Close()

	c := service.NewHTTPExchangeRateService(srv.URL)
	if _, err := c.GetRate("XYZ", "RSD"); err == nil {
		t.Fatal("expected error for missing rate")
	}
}

func TestHTTPExchangeRateService_GetRate_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := service.NewHTTPExchangeRateService(srv.URL)
	if _, err := c.GetRate("EUR", "RSD"); err == nil {
		t.Fatal("expected error for non-200")
	}
}

func TestHTTPExchangeRateService_GetRate_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := service.NewHTTPExchangeRateService(srv.URL)
	if _, err := c.GetRate("EUR", "RSD"); err == nil {
		t.Fatal("expected JSON error")
	}
}

func TestHTTPExchangeRateService_GetRate_NetworkError(t *testing.T) {
	c := service.NewHTTPExchangeRateService("http://127.0.0.1:1") // unlikely to be listening
	if _, err := c.GetRate("EUR", "RSD"); err == nil {
		t.Fatal("expected network error")
	}
}

func TestHTTPExchangeRateService_GetSellRate_AppliesSpread(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rates":[{"from":"EUR","to":"RSD","rate":100.0}]}`))
	}))
	defer srv.Close()

	c := service.NewHTTPExchangeRateService(srv.URL)
	sell, err := c.GetSellRate("EUR", "RSD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// spread = 1.5%, so sell = 100 * 0.985 = 98.5
	if sell != 98.5 {
		t.Errorf("expected 98.5 (mid * 0.985), got %v", sell)
	}
}

func TestHTTPExchangeRateService_GetSellRate_PropagatesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rates":[]}`))
	}))
	defer srv.Close()

	c := service.NewHTTPExchangeRateService(srv.URL)
	if _, err := c.GetSellRate("XYZ", "RSD"); err == nil {
		t.Fatal("expected error")
	}
}
