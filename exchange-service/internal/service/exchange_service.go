package service

import "fmt"

// ExchangeRate represents a single currency pair with its rate.
type ExchangeRate struct {
	From string
	To   string
	Rate float64
}

// ExchangeResult holds the outcome of a currency conversion calculation.
type ExchangeResult struct {
	FromCurrency string
	ToCurrency   string
	InputAmount  float64
	OutputAmount float64
	Rate         float64
}

// RateProviderInterface allows mocking the underlying rate source in tests.
type RateProviderInterface interface {
	GetRate(from, to string) (float64, error)
	GetAllRates() []ExchangeRate
}

type ExchangeService struct {
	provider RateProviderInterface
}

func NewExchangeServiceWithProvider(provider RateProviderInterface) *ExchangeService {
	return &ExchangeService{provider: provider}
}

// GetRateList returns all available currency pair exchange rates.
func (s *ExchangeService) GetRateList() []ExchangeRate {
	return s.provider.GetAllRates()
}

// CalculateExchange converts amount from one currency to another.
func (s *ExchangeService) CalculateExchange(fromCurrency, toCurrency string, amount float64) (*ExchangeResult, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if fromCurrency == "" || toCurrency == "" {
		return nil, fmt.Errorf("currency codes are required")
	}

	rate, err := s.provider.GetRate(fromCurrency, toCurrency)
	if err != nil {
		return nil, fmt.Errorf("exchange rate not available for %s→%s: %w", fromCurrency, toCurrency, err)
	}

	return &ExchangeResult{
		FromCurrency: fromCurrency,
		ToCurrency:   toCurrency,
		InputAmount:  amount,
		OutputAmount: amount * rate,
		Rate:         rate,
	}, nil
}
