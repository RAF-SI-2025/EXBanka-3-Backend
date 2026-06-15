package service

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

// dividendQuartersPerYear is the divisor in the per-quarter dividend formula:
// Dividenda = Quantity × Price × (DividendYield / 4)  (§Celina 3).
const dividendQuartersPerYear = 4.0

// DividendService pays quarterly stock dividends into holders' accounts.
type DividendService struct {
	divRepo   *repository.DividendRepository
	orderRepo *repository.OrderRepository
	taxSvc    *TaxService
	rate      RateProviderInterface
	notifier  *notify.Client
}

func NewDividendService(
	divRepo *repository.DividendRepository,
	orderRepo *repository.OrderRepository,
	taxSvc *TaxService,
	rate RateProviderInterface,
) *DividendService {
	return &DividendService{divRepo: divRepo, orderRepo: orderRepo, taxSvc: taxSvc, rate: rate}
}

// WithNotifier wires the optional best-effort notification client.
func (s *DividendService) WithNotifier(n *notify.Client) *DividendService {
	s.notifier = n
	return s
}

// DividendRunResult summarises a distribution run.
type DividendRunResult struct {
	Period    string
	Eligible  int
	PaidOut   int
	Skipped   int // already paid this quarter, or no usable account
	Failed    int // credit/persist error
	GrossByCC map[string]float64
}

// DistributeForDate pays the dividend for the quarter that `now` falls in to all
// eligible stock holders. Idempotent per (asset, holder, quarter): re-running the
// same quarter never double-pays. Clients are taxed 15% on the gain (recorded in
// the monthly tax tracking); bank-owned (actuary) holdings are paid untaxed into
// bank profit. Per §Celina 3 the payout goes, in order of preference, to: the
// account the stock was bought from (if it still exists in the listing currency),
// else the holder's default account in that currency, else converted to RSD.
func (s *DividendService) DistributeForDate(now time.Time) (DividendRunResult, error) {
	period := QuarterPeriod(now)
	result := DividendRunResult{Period: period, GrossByCC: map[string]float64{}}

	holdings, err := s.divRepo.ListDividendEligibleHoldings()
	if err != nil {
		return result, fmt.Errorf("list eligible holdings: %w", err)
	}
	result.Eligible = len(holdings)

	for _, h := range holdings {
		gross := roundPnL(h.Quantity * h.Price * (h.DividendYield / dividendQuartersPerYear))
		if gross <= 0.005 {
			result.Skipped++
			continue
		}

		exists, err := s.divRepo.PayoutExists(h.AssetID, h.UserID, h.UserType, period)
		if err != nil {
			slog.Error("dividend: payout-exists check failed", "assetID", h.AssetID, "userID", h.UserID, "error", err)
			result.Failed++
			continue
		}
		if exists {
			result.Skipped++
			continue
		}

		accountID, creditCurrency, creditAmount := s.resolvePayoutAccount(h, gross)
		if accountID == 0 {
			slog.Warn("dividend: no usable account for holder, skipping",
				"userID", h.UserID, "userType", h.UserType, "ticker", h.Ticker)
			result.Skipped++
			continue
		}

		if err := s.orderRepo.CreditAccount(accountID, creditAmount); err != nil {
			slog.Error("dividend: failed to credit account", "accountID", accountID, "error", err)
			result.Failed++
			continue
		}

		var taxRSD float64
		if h.UserType == "client" {
			if err := s.taxSvc.RecordCapitalGainTax(h.UserID, h.UserType, h.AssetID, gross, "stock", h.Currency); err != nil {
				slog.Error("dividend: failed to record capital-gain tax", "userID", h.UserID, "assetID", h.AssetID, "error", err)
			}
			taxRSD = roundPnL(s.toRSD(gross, h.Currency) * taxRate)
		}

		payout := &models.DividendPayoutRecord{
			AssetID:          h.AssetID,
			Ticker:           h.Ticker,
			UserID:           h.UserID,
			UserType:         h.UserType,
			AccountID:        accountID,
			Quantity:         h.Quantity,
			PricePerShare:    h.Price,
			DividendYield:    h.DividendYield,
			Currency:         h.Currency,
			GrossAmount:      gross,
			CreditedAmount:   creditAmount,
			CreditedCurrency: creditCurrency,
			TaxRSD:           taxRSD,
			Period:           period,
			PaidAt:           now.UTC(),
		}
		if err := s.divRepo.CreatePayout(payout); err != nil {
			// Account is already credited; a missing history row is the lesser
			// evil, but log loudly so it can be reconciled.
			slog.Error("dividend: account credited but payout record failed", "userID", h.UserID, "assetID", h.AssetID, "error", err)
			result.Failed++
			continue
		}

		s.notifyHolder(h, payout)
		result.PaidOut++
		result.GrossByCC[h.Currency] += gross
	}

	slog.Info("dividend: distribution complete",
		"period", result.Period, "eligible", result.Eligible,
		"paid", result.PaidOut, "skipped", result.Skipped, "failed", result.Failed)
	return result, nil
}

// resolvePayoutAccount applies the §Celina 3 account-selection fallback chain.
// Returns (accountID, currency, amount); accountID 0 means no account is usable.
func (s *DividendService) resolvePayoutAccount(h repository.DividendEligibleHolding, gross float64) (uint, string, float64) {
	// 1) The account the stock was bought from, if it still exists in the
	//    listing currency.
	if h.AccountID != 0 {
		if _, cur, err := s.orderRepo.GetAccountBalance(h.AccountID); err == nil && cur == h.Currency {
			return h.AccountID, h.Currency, gross
		}
	}
	// 2) The holder's default active account in the listing currency.
	if id, _ := s.divRepo.FindActiveAccountByCurrency(h.UserID, h.UserType, h.Currency); id != 0 {
		return id, h.Currency, gross
	}
	// 3) Convert to RSD and pay into the holder's RSD account.
	if id, _ := s.divRepo.FindActiveAccountByCurrency(h.UserID, h.UserType, rsdCurrency); id != 0 {
		return id, rsdCurrency, roundPnL(s.toRSD(gross, h.Currency))
	}
	return 0, "", 0
}

func (s *DividendService) notifyHolder(h repository.DividendEligibleHolding, p *models.DividendPayoutRecord) {
	if s.notifier == nil || h.UserType != "client" {
		return
	}
	s.notifier.Emit(notify.Event{
		UserID:   h.UserID,
		UserType: "client",
		Type:     "DIVIDEND_PAID",
		Title:    "Isplaćena dividenda",
		Body: fmt.Sprintf("Primili ste dividendu za %s: %.2f %s (%s).",
			p.Ticker, p.CreditedAmount, p.CreditedCurrency, p.Period),
		Link: "/portfolio",
	})
}

// ListPayoutsForUser returns a holder's dividend history (assetID 0 = all).
func (s *DividendService) ListPayoutsForUser(userID uint, userType string, assetID uint) ([]models.DividendPayoutRecord, error) {
	return s.divRepo.ListPayoutsForUser(userID, userType, assetID)
}

// toRSD converts amount from currency to RSD via the rate provider, falling back
// to 1:1 when no rate exists (mirrors TaxService.toRSD).
func (s *DividendService) toRSD(amount float64, currency string) float64 {
	if currency == rsdCurrency || currency == "" {
		return amount
	}
	rate, err := s.rate.GetRate(currency, rsdCurrency)
	if err != nil || rate == 0 {
		return amount * fallbackRSDRate
	}
	return amount * rate
}

// QuarterPeriod returns the "YYYY-Qn" label for t (e.g. 2026-06-30 -> "2026-Q2").
func QuarterPeriod(t time.Time) string {
	q := (int(t.Month())-1)/3 + 1
	return fmt.Sprintf("%d-Q%d", t.Year(), q)
}

// IsLastWorkingDayOfQuarter reports whether t is the last weekday (Mon–Fri) of a
// quarter-closing month (March, June, September, December) — the §Celina 3
// dividend payout day.
func IsLastWorkingDayOfQuarter(t time.Time) bool {
	switch t.Month() {
	case time.March, time.June, time.September, time.December:
	default:
		return false
	}
	last := lastWorkingDayOfMonth(t.Year(), t.Month())
	return t.Day() == last.Day()
}

// lastWorkingDayOfMonth returns the last Mon–Fri date in the given month.
func lastWorkingDayOfMonth(year int, month time.Month) time.Time {
	// Day 0 of next month == last day of this month.
	d := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}
