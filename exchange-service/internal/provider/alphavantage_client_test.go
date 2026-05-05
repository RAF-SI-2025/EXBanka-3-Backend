package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client wired to srv with rate limiting disabled.
func newTestClient(srv *httptest.Server) *AlphaVantageClient {
	c := NewAlphaVantageClient("test-key")
	c.baseURL = srv.URL
	c.minInterval = 0
	c.httpClient = srv.Client()
	return c
}

func TestNewAlphaVantageClient_Defaults(t *testing.T) {
	c := NewAlphaVantageClient("k")
	if c.apiKey != "k" {
		t.Errorf("expected apiKey=k, got %q", c.apiKey)
	}
	if !strings.Contains(c.baseURL, "alphavantage") {
		t.Errorf("expected default baseURL, got %q", c.baseURL)
	}
	if c.minInterval <= 0 {
		t.Error("expected positive minInterval default")
	}
}

func TestRateLimit_RespectsInterval(t *testing.T) {
	c := NewAlphaVantageClient("k")
	c.minInterval = 25 * time.Millisecond
	start := time.Now()
	c.rateLimit() // first call sets lastRequest
	c.rateLimit() // second call should sleep
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("expected at least 20ms elapsed, got %v", elapsed)
	}
}

func TestGetQuote_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("function"); got != "GLOBAL_QUOTE" {
			t.Errorf("expected function=GLOBAL_QUOTE, got %s", got)
		}
		if got := r.URL.Query().Get("symbol"); got != "AAPL" {
			t.Errorf("expected symbol=AAPL, got %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Global Quote": map[string]string{
				"05. price":  "214.33",
				"03. high":   "215.00",
				"04. low":    "213.50",
				"06. volume": "68123412",
				"09. change": "1.20",
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	q, err := c.GetQuote("AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Price != 214.33 || q.Volume != 68123412 || q.Change != 1.20 {
		t.Errorf("unexpected quote: %+v", q)
	}
}

func TestGetQuote_BadShape_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Global Quote":"not-an-object"}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetQuote("X"); err == nil {
		t.Fatal("expected error for unexpected shape")
	}
}

func TestGetDailyPrices_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Time Series (Daily)": map[string]interface{}{
				"2024-01-15": map[string]string{
					"1. open":   "100.0",
					"2. high":   "102.5",
					"3. low":    "99.5",
					"4. close":  "101.0",
					"5. volume": "12345",
				},
				"not-a-date": map[string]string{ // should be skipped
					"1. open": "1",
				},
			},
		})
	}))
	defer srv.Close()

	prices, err := newTestClient(srv).GetDailyPrices("AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("expected 1 valid price entry, got %d", len(prices))
	}
	p := prices[0]
	if p.Open != 100.0 || p.High != 102.5 || p.Close != 101.0 || p.Volume != 12345 {
		t.Errorf("unexpected price: %+v", p)
	}
}

func TestGetDailyPrices_BadShape_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetDailyPrices("X"); err == nil {
		t.Fatal("expected error for missing time series")
	}
}

func TestGetCompanyOverview_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"Name": "Apple Inc.",
			"Exchange": "NASDAQ",
			"Sector": "Technology",
			"SharesOutstanding": "15000000000",
			"DividendYield": "0.0050"
		}`))
	}))
	defer srv.Close()

	ov, err := newTestClient(srv).GetCompanyOverview("AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ov.Name != "Apple Inc." || ov.Exchange != "NASDAQ" || ov.Sector != "Technology" {
		t.Errorf("unexpected overview: %+v", ov)
	}
	if ov.OutstandingShares != 15000000000 || ov.DividendYield != 0.005 {
		t.Errorf("unexpected numeric fields: shares=%d yield=%v", ov.OutstandingShares, ov.DividendYield)
	}
}

func TestGetForexRate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("from_currency"); got != "USD" {
			t.Errorf("expected from_currency=USD, got %s", got)
		}
		_, _ = w.Write([]byte(`{
			"Realtime Currency Exchange Rate": {
				"1. From_Currency Code": "USD",
				"3. To_Currency Code": "EUR",
				"5. Exchange Rate": "0.92",
				"8. Bid Price": "0.919",
				"9. Ask Price": "0.921"
			}
		}`))
	}))
	defer srv.Close()

	rate, err := newTestClient(srv).GetForexRate("USD", "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rate.FromCurrency != "USD" || rate.ToCurrency != "EUR" {
		t.Errorf("unexpected currencies: %+v", rate)
	}
	if rate.ExchangeRate != 0.92 || rate.BidPrice != 0.919 || rate.AskPrice != 0.921 {
		t.Errorf("unexpected rates: %+v", rate)
	}
}

func TestGetForexRate_BadShape_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetForexRate("USD", "EUR"); err == nil {
		t.Fatal("expected error for missing rate")
	}
}

func TestDoRequest_ErrorMessage_FailsImmediately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Error Message":"Invalid API call"}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetQuote("BAD"); err == nil {
		t.Fatal("expected error from Error Message")
	}
}

func TestDoRequest_RateLimitNote_RetriesThenFails(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"Note":"thank you for using Alpha Vantage. Throttled."}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Avoid the 2s/4s backoff in doRequest by giving it a short-circuit:
	// not exposed, but we accept the wait here is bounded by the test's own
	// backoff (2s + 4s = 6s) — keep this case but skip in -short mode.
	if testing.Short() {
		t.Skip("skipping retry-with-backoff in -short")
	}
	if _, err := c.GetQuote("X"); err == nil {
		t.Fatal("expected failure after retries")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestDoRequest_NonRetriableStatus_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetQuote("X"); err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDoRequest_BadJSON_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv).GetQuote("X"); err == nil {
		t.Fatal("expected JSON parse error")
	}
}

// --- helpers ---

func TestParseFloat_AllVariants(t *testing.T) {
	m := map[string]interface{}{
		"str":    "  3.14 ",
		"float":  6.5,
		"jsnum":  json.Number("9.25"),
		"empty":  "",
		"weird":  []int{1, 2},
	}
	if got := parseFloat(m, "str"); got != 3.14 {
		t.Errorf("string: got %v", got)
	}
	if got := parseFloat(m, "float"); got != 6.5 {
		t.Errorf("float: got %v", got)
	}
	if got := parseFloat(m, "jsnum"); got != 9.25 {
		t.Errorf("json.Number: got %v", got)
	}
	if got := parseFloat(m, "missing"); got != 0 {
		t.Errorf("missing: got %v", got)
	}
	if got := parseFloat(m, "weird"); got != 0 {
		t.Errorf("weird: got %v", got)
	}
}

func TestParseInt_AllVariants(t *testing.T) {
	m := map[string]interface{}{
		"str":   " 42 ",
		"float": 7.9,
		"jsnum": json.Number("1000"),
		"weird": []int{1},
	}
	if got := parseInt(m, "str"); got != 42 {
		t.Errorf("string: got %v", got)
	}
	if got := parseInt(m, "float"); got != 7 {
		t.Errorf("float (truncates): got %v", got)
	}
	if got := parseInt(m, "jsnum"); got != 1000 {
		t.Errorf("json.Number: got %v", got)
	}
	if got := parseInt(m, "missing"); got != 0 {
		t.Errorf("missing: got %v", got)
	}
	if got := parseInt(m, "weird"); got != 0 {
		t.Errorf("weird: got %v", got)
	}
}

func TestSafeString(t *testing.T) {
	m := map[string]interface{}{
		"a": "hello",
		"b": 42,
	}
	if got := safeString(m, "a"); got != "hello" {
		t.Errorf("string: got %q", got)
	}
	if got := safeString(m, "b"); got != "" {
		t.Errorf("non-string: expected empty, got %q", got)
	}
	if got := safeString(m, "missing"); got != "" {
		t.Errorf("missing: expected empty, got %q", got)
	}
}
