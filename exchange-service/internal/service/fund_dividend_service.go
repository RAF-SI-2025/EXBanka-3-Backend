package service

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
)

// FundDividendRunResult summarises a fund-dividend distribution run.
type FundDividendRunResult struct {
	Period    string
	Eligible  int
	Processed int
	Skipped   int
	Failed    int
}

// DistributeFundDividends pays the quarter's dividends on fund-held stocks
// (Celina 4). For each eligible holding the gross dividend (converted to RSD)
// flows into the fund's cash account, then the fund's DividendPolicy decides the
// rest: "reinvest" buys whole shares of the paying stock back, "payout"
// distributes the cash to participants pro-rata to their share. Idempotent per
// (fund, asset, quarter). Builds on — and stays consistent with — the Celina 3
// client/bank payout (DividendService), which deliberately skips fund holdings.
func (s *FundService) DistributeFundDividends(now time.Time) (FundDividendRunResult, error) {
	period := QuarterPeriod(now)
	result := FundDividendRunResult{Period: period}
	if s.dividendRepo == nil {
		return result, fmt.Errorf("fund dividends: dividend repo not wired")
	}

	holdings, err := s.dividendRepo.ListFundDividendEligibleHoldings()
	if err != nil {
		return result, fmt.Errorf("list fund dividend holdings: %w", err)
	}
	result.Eligible = len(holdings)

	for _, h := range holdings {
		grossNative := h.Quantity * h.Price * (h.DividendYield / dividendQuartersPerYear)
		grossRSD := round2RSD(s.dividendToRSD(grossNative, h.Currency))
		if grossRSD <= 0.005 {
			result.Skipped++
			continue
		}
		fundID := h.UserID

		exists, err := s.fundRepo.FundDividendExists(fundID, h.AssetID, period)
		if err != nil {
			slog.Error("fund dividend: exists check failed", "fundID", fundID, "assetID", h.AssetID, "error", err)
			result.Failed++
			continue
		}
		if exists {
			result.Skipped++
			continue
		}

		fund, err := s.fundRepo.GetFundByID(fundID)
		if err != nil || fund == nil {
			result.Skipped++
			continue
		}

		rec, err := s.processFundDividend(fund, h, grossRSD, period, now)
		if err != nil {
			slog.Error("fund dividend: processing failed", "fundID", fundID, "assetID", h.AssetID, "error", err)
			result.Failed++
			continue
		}
		if err := s.fundRepo.CreateFundDividend(rec); err != nil {
			slog.Error("fund dividend: money moved but record failed", "fundID", fundID, "assetID", h.AssetID, "error", err)
			result.Failed++
			continue
		}
		result.Processed++
	}

	slog.Info("fund dividend: distribution complete",
		"period", result.Period, "eligible", result.Eligible,
		"processed", result.Processed, "skipped", result.Skipped, "failed", result.Failed)
	return result, nil
}

// processFundDividend runs the inflow + policy for one holding atomically and
// returns the audit record (unsaved).
func (s *FundService) processFundDividend(fund *models.InvestmentFundRecord, h repository.DividendEligibleHolding, grossRSD float64, period string, now time.Time) (*models.FundDividendRecord, error) {
	rec := &models.FundDividendRecord{
		FundID: fund.ID, AssetID: h.AssetID, Ticker: h.Ticker, Period: period,
		Quantity: h.Quantity, GrossRSD: grossRSD, Policy: fund.DividendPolicy, PaidAt: now.UTC(),
	}

	// For payout, resolve the per-participant plan BEFORE opening the write
	// transaction. Reading inside the tx would deadlock the single-writer SQLite
	// test DB, and keeps the transaction to pure money moves on Postgres too.
	var payoutPlan []fundPayoutTarget
	if fund.DividendPolicy == models.FundDividendPolicyPayout {
		plan, err := s.computePayoutPlan(fund, grossRSD)
		if err != nil {
			return nil, err
		}
		payoutPlan = plan
	}

	var notifyPayouts []fundPayoutTarget
	txErr := s.fundRepo.DB().Transaction(func(tx *gorm.DB) error {
		// Inflow: the dividend always lands in the fund's cash first.
		if err := repository.CreditAccountTx(tx, fund.AccountID, grossRSD); err != nil {
			return err
		}

		switch fund.DividendPolicy {
		case models.FundDividendPolicyPayout:
			distributed := 0.0
			for _, t := range payoutPlan {
				if err := repository.DebitAccountTx(tx, fund.AccountID, t.AmountRSD); err != nil {
					return err
				}
				if err := repository.CreditAccountTx(tx, t.AccountID, t.AmountRSD); err != nil {
					return err
				}
				// Persist the per-participant audit row so the UI can show who
				// received how much (keyed to the parent dividend by
				// fund_id + asset_id + period).
				if err := tx.Create(&models.FundDividendPayoutRecord{
					FundID: fund.ID, AssetID: h.AssetID, Period: period, Ticker: h.Ticker,
					ClientID: t.ClientID, ClientType: t.ClientType, AccountID: t.AccountID,
					AmountRSD: t.AmountRSD, PaidAt: now.UTC(),
				}).Error; err != nil {
					return err
				}
				distributed = round2RSD(distributed + t.AmountRSD)
			}
			rec.DistributedRSD = distributed
			notifyPayouts = payoutPlan
		default: // reinvest
			shares, costRSD, err := s.reinvestFundDividendTx(tx, fund, h, grossRSD, now)
			if err != nil {
				return err
			}
			rec.ReinvestedShares = shares
			rec.ReinvestedRSD = costRSD
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	for _, t := range notifyPayouts {
		s.notifyFundDividendPayout(fund, h.Ticker, t)
	}
	return rec, nil
}

// reinvestFundDividendTx buys whole shares of the paying stock with the freshly
// received dividend cash, leaving any remainder as fund cash. Returns the shares
// bought and the RSD spent. Mirrors the liquidation path's order-ledger writes.
func (s *FundService) reinvestFundDividendTx(tx *gorm.DB, fund *models.InvestmentFundRecord, h repository.DividendEligibleHolding, grossRSD float64, now time.Time) (float64, float64, error) {
	priceRSD := s.dividendToRSD(h.Price, h.Currency)
	if priceRSD <= 0 {
		return 0, 0, nil // can't price — leave the inflow as cash
	}
	shares := math.Floor(grossRSD / priceRSD)
	if shares <= 0 {
		return 0, 0, nil // dividend too small for a whole share — stays as cash
	}
	costRSD := round2RSD(shares * priceRSD)

	if err := repository.DebitAccountTx(tx, fund.AccountID, costRSD); err != nil {
		return 0, 0, err
	}
	order := &models.OrderRecord{
		UserID: fund.ID, UserType: models.PortfolioOwnerFund, AssetID: h.AssetID,
		OrderType: "market", Direction: "buy", Quantity: int64(shares), ContractSize: 1,
		PricePerUnit: h.Price, Status: "done", IsDone: true, RemainingPortions: 0,
		AccountID: fund.AccountID, LastModification: now, CreatedAt: now,
	}
	if err := tx.Create(order).Error; err != nil {
		return 0, 0, err
	}
	if err := tx.Create(&models.OrderTransactionRecord{
		OrderID: order.ID, Quantity: order.Quantity, PricePerUnit: h.Price, ExecutedAt: now,
	}).Error; err != nil {
		return 0, 0, err
	}
	if err := repository.RecordBuyFillTx(tx, fund.ID, models.PortfolioOwnerFund, h.AssetID, fund.AccountID, shares, h.Price); err != nil {
		return 0, 0, err
	}
	return shares, costRSD, nil
}

type fundPayoutTarget struct {
	ClientID   uint
	ClientType string
	AccountID  uint
	AmountRSD  float64
}

// computePayoutPlan resolves how the dividend cash splits across participants
// pro-rata to their share of the fund: each gets floor(gross × share) into their
// RSD account (rounded down so the total never exceeds the gross; any remainder
// stays in fund cash). Participants without a payable account are skipped, their
// slice staying in the fund. Read-only — the actual debits/credits run later in
// the distribution transaction.
func (s *FundService) computePayoutPlan(fund *models.InvestmentFundRecord, grossRSD float64) ([]fundPayoutTarget, error) {
	positions, err := s.fundRepo.ListPositionsForFund(fund.ID)
	if err != nil {
		return nil, err
	}
	total := 0.0
	for _, p := range positions {
		total += p.UkupanUlozeniIznos
	}
	if total <= 0 || len(positions) == 0 {
		return nil, nil // no participants — leave as cash
	}

	targets := make([]fundPayoutTarget, 0, len(positions))
	for _, p := range positions {
		share := p.UkupanUlozeniIznos / total
		amount := math.Floor(grossRSD*share*100) / 100
		if amount <= 0 {
			continue
		}
		acctID, err := s.fundRepo.FindParticipantRSDAccount(p.ClientID, p.ClientType)
		if err != nil {
			return nil, err
		}
		if acctID == 0 {
			continue // no payable account — their slice stays in fund cash
		}
		targets = append(targets, fundPayoutTarget{
			ClientID: p.ClientID, ClientType: p.ClientType, AccountID: acctID, AmountRSD: amount,
		})
	}
	return targets, nil
}

func (s *FundService) notifyFundDividendPayout(fund *models.InvestmentFundRecord, ticker string, t fundPayoutTarget) {
	if s.notifier == nil || t.ClientType != "client" {
		return
	}
	s.notifier.Emit(notify.Event{
		UserID:   t.ClientID,
		UserType: "client",
		Type:     "FUND_DIVIDEND_PAID",
		Title:    "Dividenda iz fonda",
		Body: fmt.Sprintf("Fond \"%s\" vam je isplatio dividendu (%s): %.2f RSD.",
			fund.Naziv, ticker, t.AmountRSD),
		Link: "/funds",
	})
}

// ListFundDividends returns a fund's dividend history.
func (s *FundService) ListFundDividends(fundID uint) ([]models.FundDividendRecord, error) {
	return s.fundRepo.ListFundDividends(fundID)
}

// ListFundDividendPayouts returns the per-participant payout breakdown for a
// fund's payout-policy distributions.
func (s *FundService) ListFundDividendPayouts(fundID uint) ([]models.FundDividendPayoutRecord, error) {
	return s.fundRepo.ListFundDividendPayouts(fundID)
}

// SetDividendPolicy updates a fund's dividend policy. The managing supervisor
// may change their own fund; an admin may change any fund (admin override).
func (s *FundService) SetDividendPolicy(fundID, actorID uint, isAdmin bool, policy string) (*models.InvestmentFundRecord, error) {
	if policy != models.FundDividendPolicyReinvest && policy != models.FundDividendPolicyPayout {
		return nil, fmt.Errorf("nepoznata politika dividendi: %s", policy)
	}
	fund, err := s.GetFund(fundID)
	if err != nil {
		return nil, err
	}
	if fund.ManagerID != actorID && !isAdmin {
		return nil, fmt.Errorf("supervisor ne upravlja ovim fondom")
	}
	if err := s.fundRepo.UpdateDividendPolicy(fundID, policy); err != nil {
		return nil, err
	}
	fund.DividendPolicy = policy
	return fund, nil
}

// dividendToRSD converts amount from currency to RSD via the rate provider,
// falling back to 1:1 when no rate exists.
func (s *FundService) dividendToRSD(amount float64, currency string) float64 {
	if currency == "" || currency == "RSD" {
		return amount
	}
	rate, err := s.rateProvider.GetRate(currency, "RSD")
	if err != nil || rate == 0 {
		return amount
	}
	return amount * rate
}
