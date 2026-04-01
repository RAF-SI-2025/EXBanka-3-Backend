package service

import (
	"fmt"
	"strings"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

type MarketDataProvider interface {
	GetExchanges() ([]models.Exchange, error)
	GetListings() ([]models.Listing, error)
	GetListing(ticker string) (*models.Listing, error)
	GetHistory(ticker string) ([]models.ListingDailyPriceInfo, error)
	GetPortfolio(ownerID uint, ownerType models.PortfolioOwnerType) (*models.Portfolio, error)
}

type ExchangeToggler interface {
	GetExchangeByAcronym(acronym string) (*models.MarketExchangeRecord, error)
	ToggleExchangeManualTime(acronym string, useManual bool, manualOpen bool) error
}

type MarketService struct {
	provider MarketDataProvider
}

func NewMarketService(provider MarketDataProvider) *MarketService {
	return &MarketService{provider: provider}
}

func (s *MarketService) ListExchanges() ([]models.Exchange, error) {
	return s.provider.GetExchanges()
}

func (s *MarketService) ListListings(query string) ([]models.Listing, error) {
	listings, err := s.provider.GetListings()
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return listings, nil
	}

	filtered := make([]models.Listing, 0, len(listings))
	for _, listing := range listings {
		if strings.Contains(strings.ToLower(listing.Ticker), query) || strings.Contains(strings.ToLower(listing.Name), query) {
			filtered = append(filtered, listing)
		}
	}
	return filtered, nil
}

func (s *MarketService) GetListing(ticker string) (*models.Listing, error) {
	listing, err := s.provider.GetListing(strings.ToUpper(strings.TrimSpace(ticker)))
	if err != nil {
		return nil, err
	}
	if listing == nil {
		return nil, fmt.Errorf("listing not found")
	}
	return listing, nil
}

func (s *MarketService) GetListingHistory(ticker string) ([]models.ListingDailyPriceInfo, error) {
	history, err := s.provider.GetHistory(strings.ToUpper(strings.TrimSpace(ticker)))
	if err != nil {
		return nil, err
	}
	if history == nil {
		return nil, fmt.Errorf("listing history not found")
	}
	return history, nil
}

func (s *MarketService) GetPortfolio(ownerID uint, ownerType models.PortfolioOwnerType) (*models.Portfolio, error) {
	if ownerID == 0 {
		return nil, fmt.Errorf("owner id is required")
	}
	if ownerType == "" {
		return nil, fmt.Errorf("owner type is required")
	}

	return s.provider.GetPortfolio(ownerID, ownerType)
}
