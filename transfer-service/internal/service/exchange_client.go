package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sellRateSpread is the spread applied when the bank sells the destination currency.
// Must stay in sync with the exchange-service provider spread constant (1.5%).
const sellRateSpread = 0.015

// HTTPExchangeRateService calls the exchange-service HTTP API to get rates.
type HTTPExchangeRateService struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPExchangeRateService(baseURL string) *HTTPExchangeRateService {
	return &HTTPExchangeRateService{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type rateListResponse struct {
	Rates []rateItem `json:"rates"`
}

type rateItem struct {
	FromCurrency string  `json:"from"`
	ToCurrency   string  `json:"to"`
	Rate         float64 `json:"rate"`
}

func (s *HTTPExchangeRateService) GetRate(fromCurrencyKod, toCurrencyKod string) (float64, error) {
	resp, err := s.httpClient.Get(s.baseURL + "/api/v1/exchange/rates")
	if err != nil {
		return 0, fmt.Errorf("failed to call exchange service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("exchange service returned status %d", resp.StatusCode)
	}

	var result rateListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode exchange response: %w", err)
	}

	for _, r := range result.Rates {
		if r.FromCurrency == fromCurrencyKod && r.ToCurrency == toCurrencyKod {
			return r.Rate, nil
		}
	}

	return 0, fmt.Errorf("exchange rate not found: %s -> %s", fromCurrencyKod, toCurrencyKod)
}

// GetSellRate returns the rate the bank uses when selling toCurrency to the client.
// This is the mid-market rate reduced by the sell spread, giving the client fewer
// destination-currency units than the mid rate — the bank always sells at a disadvantage to the client.
func (s *HTTPExchangeRateService) GetSellRate(fromCurrencyKod, toCurrencyKod string) (float64, error) {
	mid, err := s.GetRate(fromCurrencyKod, toCurrencyKod)
	if err != nil {
		return 0, err
	}
	return mid * (1 - sellRateSpread), nil
}
