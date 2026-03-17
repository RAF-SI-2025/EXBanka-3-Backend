package provider

import (
	"errors"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// StaticRateProvider serves hardcoded exchange rates as a development/fallback provider.
type StaticRateProvider struct {
	rates map[string]map[string]float64
}

func NewStaticRateProvider() *StaticRateProvider {
	return &StaticRateProvider{
		rates: map[string]map[string]float64{
			"EUR": {"RSD": 117.15, "USD": 1.08, "CHF": 0.96, "GBP": 0.85, "JPY": 161.50, "CAD": 1.47, "AUD": 1.63},
			"USD": {"RSD": 108.48, "EUR": 0.926, "CHF": 0.888, "GBP": 0.786, "JPY": 149.53, "CAD": 1.362, "AUD": 1.508},
			"CHF": {"RSD": 122.03, "EUR": 1.042, "USD": 1.126},
			"GBP": {"RSD": 137.82, "EUR": 1.176, "USD": 1.272},
			"JPY": {"RSD": 0.715, "EUR": 0.0062, "USD": 0.0067},
			"CAD": {"RSD": 79.71, "EUR": 0.68, "USD": 0.734},
			"AUD": {"RSD": 71.87, "EUR": 0.613, "USD": 0.663},
			"RSD": {"EUR": 0.00854, "USD": 0.00922, "CHF": 0.00819, "GBP": 0.00726, "JPY": 1.398, "CAD": 0.01254, "AUD": 0.01391},
		},
	}
}

func (p *StaticRateProvider) GetRate(from, to string) (float64, error) {
	if from == to {
		return 1.0, nil
	}
	if row, ok := p.rates[from]; ok {
		if rate, ok := row[to]; ok {
			return rate, nil
		}
	}
	return 0, errors.New("unknown currency pair")
}

func (p *StaticRateProvider) GetAllRates() []service.ExchangeRate {
	var result []service.ExchangeRate
	for from, targets := range p.rates {
		for to, rate := range targets {
			result = append(result, service.ExchangeRate{From: from, To: to, Rate: rate})
		}
	}
	return result
}
