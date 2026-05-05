package provider_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/provider"
)

func TestMockMarketProvider_GetExchanges_SortedByAcronym(t *testing.T) {
	p := provider.NewMockMarketProvider()

	exchanges, err := p.GetExchanges()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exchanges) < 6 {
		t.Fatalf("expected at least 6 seeded exchanges, got %d", len(exchanges))
	}
	if !sort.SliceIsSorted(exchanges, func(i, j int) bool {
		return exchanges[i].Acronym < exchanges[j].Acronym
	}) {
		t.Errorf("expected exchanges sorted by acronym, got: %v", exchangeAcronyms(exchanges))
	}
}

func TestMockMarketProvider_GetListings_SortedByTicker(t *testing.T) {
	p := provider.NewMockMarketProvider()

	listings, err := p.GetListings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listings) < 12 {
		t.Fatalf("expected at least 12 seeded listings, got %d", len(listings))
	}
	if !sort.SliceIsSorted(listings, func(i, j int) bool {
		return listings[i].Ticker < listings[j].Ticker
	}) {
		t.Errorf("expected listings sorted by ticker")
	}
	for _, l := range listings {
		if l.Ask <= l.Bid {
			t.Errorf("ticker %s: expected Ask > Bid, got ask=%v bid=%v", l.Ticker, l.Ask, l.Bid)
		}
		if l.Type != models.ListingTypeStock {
			t.Errorf("ticker %s: expected stock type, got %s", l.Ticker, l.Type)
		}
	}
}

func TestMockMarketProvider_GetListing_Found(t *testing.T) {
	p := provider.NewMockMarketProvider()

	got, err := p.GetListing("AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected listing for AAPL")
	}
	if got.Ticker != "AAPL" || got.Name == "" {
		t.Errorf("unexpected listing: %+v", got)
	}
	if got.Exchange.Acronym != "NASDAQ" {
		t.Errorf("expected NASDAQ, got %s", got.Exchange.Acronym)
	}
}

func TestMockMarketProvider_GetListing_NotFound(t *testing.T) {
	p := provider.NewMockMarketProvider()

	got, err := p.GetListing("DOES_NOT_EXIST")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown ticker, got %+v", got)
	}
}

func TestMockMarketProvider_GetHistory_Found(t *testing.T) {
	p := provider.NewMockMarketProvider()

	history, err := p.GetHistory("MSFT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 30 {
		t.Errorf("expected 30 history entries, got %d", len(history))
	}
	for _, day := range history {
		if day.Price <= 0 {
			t.Errorf("expected positive price, got %v", day.Price)
		}
		if day.High < day.Low {
			t.Errorf("expected high >= low, got high=%v low=%v", day.High, day.Low)
		}
		if day.Volume <= 0 {
			t.Errorf("expected positive volume, got %v", day.Volume)
		}
	}
	// Dates ascending.
	for i := 1; i < len(history); i++ {
		if !history[i].Date.After(history[i-1].Date) {
			t.Errorf("expected dates ascending at index %d", i)
		}
	}
}

func TestMockMarketProvider_GetHistory_NotFound(t *testing.T) {
	p := provider.NewMockMarketProvider()

	got, err := p.GetHistory("XYZZY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown ticker, got %+v", got)
	}
}

func TestMockMarketProvider_GetPortfolio_DeterministicByOwnerID(t *testing.T) {
	p := provider.NewMockMarketProvider()

	first, err := p.GetPortfolio(123, models.PortfolioOwnerTypeClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := p.GetPortfolio(123, models.PortfolioOwnerTypeClient)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first == nil || second == nil {
		t.Fatal("expected non-nil portfolios")
	}
	if first.OwnerID != 123 || first.OwnerType != models.PortfolioOwnerTypeClient {
		t.Errorf("unexpected owner: id=%d type=%s", first.OwnerID, first.OwnerType)
	}
	if first.PositionCount != second.PositionCount {
		t.Errorf("portfolio not deterministic for same ownerID: %d vs %d",
			first.PositionCount, second.PositionCount)
	}
}

func TestMockMarketProvider_String(t *testing.T) {
	p := provider.NewMockMarketProvider()
	s := p.String()
	if !strings.Contains(s, "MockMarketProvider") {
		t.Errorf("expected MockMarketProvider in string, got %s", s)
	}
}

func exchangeAcronyms(items []models.Exchange) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Acronym
	}
	return out
}
