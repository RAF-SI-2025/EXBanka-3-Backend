package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AlphaVantageClient is an HTTP client for the Alpha Vantage API with rate limiting and retry logic.
type AlphaVantageClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	mu          sync.Mutex
	lastRequest time.Time
	minInterval time.Duration // rate limit interval between requests
}

func NewAlphaVantageClient(apiKey string) *AlphaVantageClient {
	return &AlphaVantageClient{
		apiKey:  apiKey,
		baseURL: "https://www.alphavantage.co/query",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		minInterval: 12 * time.Second, // 5 req/min free tier
	}
}

func (c *AlphaVantageClient) rateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastRequest)
	if elapsed < c.minInterval {
		time.Sleep(c.minInterval - elapsed)
	}
	c.lastRequest = time.Now()
}

func (c *AlphaVantageClient) doRequest(params map[string]string) (map[string]interface{}, error) {
	c.rateLimit()

	url := c.baseURL + "?"
	parts := make([]string, 0, len(params)+1)
	parts = append(parts, "apikey="+c.apiKey)
	for k, v := range params {
		parts = append(parts, k+"="+v)
	}
	url += strings.Join(parts, "&")

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*2) * time.Second
			slog.Warn("AlphaVantage retry", "attempt", attempt+1, "backoff", backoff)
			time.Sleep(backoff)
		}

		resp, err := c.httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("AlphaVantage returned %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("AlphaVantage returned %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}

		if _, ok := result["Error Message"]; ok {
			return nil, fmt.Errorf("AlphaVantage error: %v", result["Error Message"])
		}
		if note, ok := result["Note"]; ok {
			lastErr = fmt.Errorf("AlphaVantage rate limit note: %v", note)
			continue
		}

		return result, nil
	}

	return nil, fmt.Errorf("AlphaVantage request failed after 3 attempts: %w", lastErr)
}

// QuoteResponse holds parsed stock quote data.
type QuoteResponse struct {
	Price  float64
	High   float64
	Low    float64
	Volume int64
	Change float64
}

// GetQuote fetches real-time quote for a ticker.
func (c *AlphaVantageClient) GetQuote(ticker string) (*QuoteResponse, error) {
	data, err := c.doRequest(map[string]string{
		"function": "GLOBAL_QUOTE",
		"symbol":   ticker,
	})
	if err != nil {
		return nil, err
	}

	gq, ok := data["Global Quote"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected GLOBAL_QUOTE response format")
	}

	return &QuoteResponse{
		Price:  parseFloat(gq, "05. price"),
		High:   parseFloat(gq, "03. high"),
		Low:    parseFloat(gq, "04. low"),
		Volume: parseInt(gq, "06. volume"),
		Change: parseFloat(gq, "09. change"),
	}, nil
}

// DailyPrice holds one day of price data.
type DailyPrice struct {
	Date   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// GetDailyPrices fetches daily historical prices for a ticker.
func (c *AlphaVantageClient) GetDailyPrices(ticker string) ([]DailyPrice, error) {
	data, err := c.doRequest(map[string]string{
		"function":   "TIME_SERIES_DAILY",
		"symbol":     ticker,
		"outputsize": "compact",
	})
	if err != nil {
		return nil, err
	}

	ts, ok := data["Time Series (Daily)"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected TIME_SERIES_DAILY response format")
	}

	prices := make([]DailyPrice, 0, len(ts))
	for dateStr, values := range ts {
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		v := values.(map[string]interface{})
		prices = append(prices, DailyPrice{
			Date:   date,
			Open:   parseFloat(v, "1. open"),
			High:   parseFloat(v, "2. high"),
			Low:    parseFloat(v, "3. low"),
			Close:  parseFloat(v, "4. close"),
			Volume: parseInt(v, "5. volume"),
		})
	}

	return prices, nil
}

// CompanyOverview holds company info data.
type CompanyOverview struct {
	Name              string
	Exchange          string
	Sector            string
	OutstandingShares int64
	DividendYield     float64
}

// GetCompanyOverview fetches company details.
func (c *AlphaVantageClient) GetCompanyOverview(ticker string) (*CompanyOverview, error) {
	data, err := c.doRequest(map[string]string{
		"function": "OVERVIEW",
		"symbol":   ticker,
	})
	if err != nil {
		return nil, err
	}

	return &CompanyOverview{
		Name:              safeString(data, "Name"),
		Exchange:          safeString(data, "Exchange"),
		Sector:            safeString(data, "Sector"),
		OutstandingShares: parseInt(data, "SharesOutstanding"),
		DividendYield:     parseFloat(data, "DividendYield"),
	}, nil
}

// ForexRate holds a forex exchange rate.
type ForexRate struct {
	FromCurrency string
	ToCurrency   string
	ExchangeRate float64
	BidPrice     float64
	AskPrice     float64
}

// GetForexRate fetches the exchange rate for a currency pair.
func (c *AlphaVantageClient) GetForexRate(fromCurrency, toCurrency string) (*ForexRate, error) {
	data, err := c.doRequest(map[string]string{
		"function":      "CURRENCY_EXCHANGE_RATE",
		"from_currency": fromCurrency,
		"to_currency":   toCurrency,
	})
	if err != nil {
		return nil, err
	}

	rate, ok := data["Realtime Currency Exchange Rate"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected CURRENCY_EXCHANGE_RATE response format")
	}

	return &ForexRate{
		FromCurrency: safeString(rate, "1. From_Currency Code"),
		ToCurrency:   safeString(rate, "3. To_Currency Code"),
		ExchangeRate: parseFloat(rate, "5. Exchange Rate"),
		BidPrice:     parseFloat(rate, "8. Bid Price"),
		AskPrice:     parseFloat(rate, "9. Ask Price"),
	}, nil
}

func parseFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f
	case float64:
		return val
	case json.Number:
		f, _ := val.Float64()
		return f
	}
	return 0
}

func parseInt(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch val := v.(type) {
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
		return i
	case float64:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	}
	return 0
}

func safeString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
