package service

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

var (
	ErrFundNotFound     = errors.New("investicioni fond nije pronadjen")
	ErrFundPositionNone = errors.New("klijent nema poziciju u fondu")
)

// fundWithdrawalCommissionRate is the % commission charged to clients on a
// withdrawal (zero for supervisors topping up/withdrawing on the bank's behalf).
const fundWithdrawalCommissionRate = 0.005 // 0.5%

// FundService is the entry point for the FOND domain. It owns:
//   - Investment-fund CRUD (supervisors only)
//   - Investments and withdrawals by clients and the bank
//   - Derived computations (fund value, profit, share, position value)
//   - Daily performance snapshots
type FundService struct {
	fundRepo      *repository.FundRepository
	portfolioRepo *repository.PortfolioRepository
	marketRepo    *repository.MarketRepository
	orderRepo     *repository.OrderRepository
	rateProvider  RateProviderInterface
}

func NewFundService(
	fundRepo *repository.FundRepository,
	portfolioRepo *repository.PortfolioRepository,
	marketRepo *repository.MarketRepository,
	orderRepo *repository.OrderRepository,
	rateProvider RateProviderInterface,
) *FundService {
	return &FundService{
		fundRepo:      fundRepo,
		portfolioRepo: portfolioRepo,
		marketRepo:    marketRepo,
		orderRepo:     orderRepo,
		rateProvider:  rateProvider,
	}
}

// --- DTOs ---

type CreateFundInput struct {
	Naziv         string
	Opis          string
	MinimalniUlog float64
	ManagerID     uint // supervisor creating the fund
}

type InvestInFundInput struct {
	FundID          uint
	ClientID        uint
	ClientType      string // "client" or "bank"
	SourceAccountID uint
	Amount          float64
}

type WithdrawFromFundInput struct {
	FundID               uint
	ClientID             uint
	ClientType           string // "client" or "bank"
	DestinationAccountID uint
	Amount               float64 // 0 means "withdraw whole position"
	WithdrawAll          bool
}

type FundSummary struct {
	Fund                *models.InvestmentFundRecord
	FundValueRSD        float64
	LiquidCashRSD       float64
	HoldingsValueRSD    float64
	TotalInvestedRSD    float64
	ProfitRSD           float64
	ManagerID           uint
	ParticipantsCount   int
	WithdrawalCommRate  float64
}

type FundPositionView struct {
	FundID            uint
	FundNaziv         string
	UkupanUlozeniRSD  float64
	UdeoProcenat      float64 // % share of total invested
	TrenutnaVrednost  float64 // share * fund value
	ProfitRSD         float64 // current value - invested
	FundValueRSD      float64
}

type WithdrawResult struct {
	GrossWithdrawn  float64
	Commission      float64
	NetToAccount    float64
	Liquidated      bool
	LiquidatedItems []LiquidatedItem
}

type LiquidatedItem struct {
	AssetID     uint
	Ticker      string
	Quantity    float64
	PricePerRSD float64
	TotalRSD    float64
}

// --- Create / Read ---

func (s *FundService) CreateFund(input CreateFundInput) (*models.InvestmentFundRecord, error) {
	naziv := strings.TrimSpace(input.Naziv)
	if naziv == "" {
		return nil, fmt.Errorf("naziv fonda je obavezan")
	}
	if input.MinimalniUlog <= 0 {
		return nil, fmt.Errorf("minimalni ulog mora biti pozitivan")
	}
	if input.ManagerID == 0 {
		return nil, fmt.Errorf("menadzer (supervisor) je obavezan")
	}
	return s.fundRepo.CreateFundWithAccount(naziv, strings.TrimSpace(input.Opis), input.MinimalniUlog, input.ManagerID)
}

func (s *FundService) GetFund(id uint) (*models.InvestmentFundRecord, error) {
	fund, err := s.fundRepo.GetFundByID(id)
	if err != nil {
		return nil, err
	}
	if fund == nil {
		return nil, ErrFundNotFound
	}
	return fund, nil
}

func (s *FundService) ListFunds() ([]models.InvestmentFundRecord, error) {
	return s.fundRepo.ListFunds()
}

func (s *FundService) ListFundsByManager(managerID uint) ([]models.InvestmentFundRecord, error) {
	return s.fundRepo.ListFundsByManager(managerID)
}

// SummariseFund returns the derived view of a fund used by the discovery /
// detail pages.
func (s *FundService) SummariseFund(fund *models.InvestmentFundRecord) (*FundSummary, error) {
	account, err := s.fundRepo.GetAccountByID(fund.AccountID)
	if err != nil {
		return nil, err
	}
	liquid := 0.0
	if account != nil {
		liquid = account.Stanje
	}

	holdings, err := s.portfolioRepo.ListHoldingsForUser(fund.ID, models.PortfolioOwnerFund)
	if err != nil {
		return nil, err
	}

	holdingsValue := 0.0
	for i := range holdings {
		h := &holdings[i]
		holdingsValue += s.holdingValueInRSD(h)
	}

	invested, err := s.fundRepo.TotalInvestedInFund(fund.ID)
	if err != nil {
		return nil, err
	}

	participants, err := s.fundRepo.ListPositionsForFund(fund.ID)
	if err != nil {
		return nil, err
	}

	fundValue := round2RSD(liquid + holdingsValue)
	return &FundSummary{
		Fund:                fund,
		FundValueRSD:        fundValue,
		LiquidCashRSD:       round2RSD(liquid),
		HoldingsValueRSD:    round2RSD(holdingsValue),
		TotalInvestedRSD:    round2RSD(invested),
		ProfitRSD:           round2RSD(fundValue - invested),
		ManagerID:           fund.ManagerID,
		ParticipantsCount:   len(participants),
		WithdrawalCommRate:  fundWithdrawalCommissionRate,
	}, nil
}

// --- Investing ---

func (s *FundService) InvestInFund(input InvestInFundInput) (*models.ClientFundTransactionRecord, error) {
	if input.Amount <= 0 {
		return nil, fmt.Errorf("iznos uplate mora biti pozitivan")
	}
	if input.SourceAccountID == 0 {
		return nil, fmt.Errorf("izvorni racun je obavezan")
	}
	if input.ClientType != "client" && input.ClientType != "bank" {
		return nil, fmt.Errorf("nepoznat tip ucesnika")
	}

	fund, err := s.GetFund(input.FundID)
	if err != nil {
		return nil, err
	}
	if input.Amount < fund.MinimalniUlog {
		// Allow follow-on contributions below the minimum if a position already exists.
		pos, err := s.fundRepo.GetPosition(input.ClientID, input.ClientType, fund.ID)
		if err != nil {
			return nil, err
		}
		if pos == nil || pos.UkupanUlozeniIznos <= 0 {
			return nil, fmt.Errorf("minimalni ulog je %.2f RSD", fund.MinimalniUlog)
		}
	}

	source, err := s.fundRepo.GetAccountByID(input.SourceAccountID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, fmt.Errorf("izvorni racun nije pronadjen")
	}
	if source.Status != "aktivan" {
		return nil, fmt.Errorf("izvorni racun nije aktivan")
	}
	if source.CurrencyCode != "RSD" {
		return nil, fmt.Errorf("uplata u fond mora biti sa RSD racuna")
	}

	switch input.ClientType {
	case "client":
		if !source.BelongsToClient(input.ClientID) {
			return nil, fmt.Errorf("racun ne pripada klijentu")
		}
	case "bank":
		if !source.IsBankOwned() {
			return nil, fmt.Errorf("izabrani racun nije bankin racun")
		}
	}
	if source.RaspolozivoStanje < input.Amount {
		return nil, fmt.Errorf("nedovoljno sredstava na izvornom racunu")
	}

	return s.fundRepo.RecordInvestment(input.ClientID, input.ClientType, fund.ID, source.ID, fund.AccountID, round2RSD(input.Amount))
}

// --- Withdrawals & liquidation ---

// WithdrawFromFund withdraws cash from a fund. Auto-liquidates fund holdings
// at current market price if the requested amount exceeds liquid cash.
// Commission is charged to clients (not to bank-on-behalf withdrawals).
func (s *FundService) WithdrawFromFund(input WithdrawFromFundInput) (*WithdrawResult, error) {
	if input.ClientType != "client" && input.ClientType != "bank" {
		return nil, fmt.Errorf("nepoznat tip ucesnika")
	}
	if input.DestinationAccountID == 0 {
		return nil, fmt.Errorf("destinacioni racun je obavezan")
	}

	fund, err := s.GetFund(input.FundID)
	if err != nil {
		return nil, err
	}

	pos, err := s.fundRepo.GetPosition(input.ClientID, input.ClientType, fund.ID)
	if err != nil {
		return nil, err
	}
	if pos == nil || pos.UkupanUlozeniIznos <= 0 {
		return nil, ErrFundPositionNone
	}

	summary, err := s.SummariseFund(fund)
	if err != nil {
		return nil, err
	}

	clientShare := 0.0
	if summary.TotalInvestedRSD > 0 {
		clientShare = pos.UkupanUlozeniIznos / summary.TotalInvestedRSD
	}
	maxAvailable := round2RSD(clientShare * summary.FundValueRSD)

	requested := round2RSD(input.Amount)
	if input.WithdrawAll || requested <= 0 {
		requested = maxAvailable
	}
	if requested <= 0 {
		return nil, fmt.Errorf("fond nema vrednost za isplatu")
	}
	if requested > maxAvailable+0.01 {
		return nil, fmt.Errorf("iznos prevazilazi udeo klijenta (%.2f RSD)", maxAvailable)
	}

	destination, err := s.fundRepo.GetAccountByID(input.DestinationAccountID)
	if err != nil {
		return nil, err
	}
	if destination == nil {
		return nil, fmt.Errorf("destinacioni racun nije pronadjen")
	}
	if destination.Status != "aktivan" {
		return nil, fmt.Errorf("destinacioni racun nije aktivan")
	}
	if destination.CurrencyCode != "RSD" {
		return nil, fmt.Errorf("isplata mora ici na RSD racun")
	}
	switch input.ClientType {
	case "client":
		if !destination.BelongsToClient(input.ClientID) {
			return nil, fmt.Errorf("destinacioni racun ne pripada klijentu")
		}
	case "bank":
		if !destination.IsBankOwned() {
			return nil, fmt.Errorf("destinacioni racun mora biti bankin")
		}
	}

	// Auto-liquidate fund holdings if liquid cash is insufficient.
	liquidatedItems := []LiquidatedItem{}
	liquidated := false
	if summary.LiquidCashRSD < requested {
		shortfall := requested - summary.LiquidCashRSD
		items, err := s.liquidateFundHoldings(fund, shortfall)
		if err != nil {
			return nil, fmt.Errorf("auto-likvidacija nije uspela: %w", err)
		}
		liquidatedItems = items
		liquidated = len(items) > 0
	}

	commission := 0.0
	if input.ClientType == "client" {
		commission = round2RSD(requested * fundWithdrawalCommissionRate)
	}

	_, err = s.fundRepo.RecordWithdrawal(input.ClientID, input.ClientType, fund.ID, fund.AccountID, destination.ID, requested, commission)
	if err != nil {
		return nil, err
	}

	return &WithdrawResult{
		GrossWithdrawn:  requested,
		Commission:      commission,
		NetToAccount:    round2RSD(requested - commission),
		Liquidated:      liquidated,
		LiquidatedItems: liquidatedItems,
	}, nil
}

// liquidateFundHoldings sells fund holdings at the current market price until
// the fund has at least `requiredRSD` in liquid cash. Cash proceeds are
// credited to the fund's RSD account. Returns the per-asset liquidations.
//
// This is a simplified market-sell flow (no commission, no order routing) that
// mirrors what the order executor does on a fill, scoped to fund-owned assets.
func (s *FundService) liquidateFundHoldings(fund *models.InvestmentFundRecord, requiredRSD float64) ([]LiquidatedItem, error) {
	holdings, err := s.portfolioRepo.ListHoldingsForUser(fund.ID, models.PortfolioOwnerFund)
	if err != nil {
		return nil, err
	}

	liquidated := []LiquidatedItem{}
	remaining := requiredRSD
	now := time.Now().UTC()

	for i := range holdings {
		if remaining <= 0 {
			break
		}
		h := &holdings[i]
		if h.Quantity <= 0 {
			continue
		}
		priceRSD := s.assetPriceInRSD(&h.Asset)
		if priceRSD <= 0 {
			continue
		}
		holdingValueRSD := round2RSD(priceRSD * h.Quantity)
		sellQty := h.Quantity
		sellValue := holdingValueRSD
		if holdingValueRSD > remaining {
			// Sell a partial chunk that covers the remaining shortfall.
			sellQty = round2RSD(remaining / priceRSD)
			if sellQty <= 0 {
				continue
			}
			sellValue = round2RSD(priceRSD * sellQty)
		}

		order := &models.OrderRecord{
			UserID:            fund.ID,
			UserType:          models.PortfolioOwnerFund,
			AssetID:           h.AssetID,
			OrderType:         "market",
			Direction:         "sell",
			Quantity:          int64(math.Ceil(sellQty)),
			ContractSize:      1,
			PricePerUnit:      h.Asset.Bid,
			Status:            "done",
			IsDone:            true,
			RemainingPortions: 0,
			AccountID:         fund.AccountID,
			LastModification:  now,
			CreatedAt:         now,
		}
		if err := s.orderRepo.CreateOrder(order); err != nil {
			return nil, fmt.Errorf("kreiranje likvidacionog ordera nije uspelo: %w", err)
		}
		txRec := &models.OrderTransactionRecord{
			OrderID:      order.ID,
			Quantity:     order.Quantity,
			PricePerUnit: h.Asset.Bid,
			ExecutedAt:   now,
		}
		if err := s.orderRepo.CreateOrderTransaction(txRec); err != nil {
			return nil, fmt.Errorf("kreiranje likvidacione transakcije nije uspelo: %w", err)
		}
		// Reduce the fund's holding by the sold quantity.
		if _, err := s.portfolioRepo.RecordSellFill(fund.ID, models.PortfolioOwnerFund, h.AssetID, sellQty, h.Asset.Bid); err != nil {
			return nil, fmt.Errorf("azuriranje fondske pozicije nije uspelo: %w", err)
		}
		// Credit the fund's RSD account with the (RSD-converted) proceeds.
		if err := s.fundRepo.CreditAccount(fund.AccountID, sellValue); err != nil {
			return nil, err
		}
		liquidated = append(liquidated, LiquidatedItem{
			AssetID:     h.AssetID,
			Ticker:      h.Asset.Ticker,
			Quantity:    sellQty,
			PricePerRSD: priceRSD,
			TotalRSD:    sellValue,
		})
		remaining -= sellValue
	}

	if remaining > 0.01 {
		return liquidated, fmt.Errorf("nedovoljno hartija u fondu za pokrivanje povlacenja")
	}
	return liquidated, nil
}

// --- Positions / clients view ---

func (s *FundService) ListClientFundPositions(clientID uint, clientType string) ([]FundPositionView, error) {
	positions, err := s.fundRepo.ListPositionsForClient(clientID, clientType)
	if err != nil {
		return nil, err
	}

	views := make([]FundPositionView, 0, len(positions))
	for _, pos := range positions {
		fund, err := s.fundRepo.GetFundByID(pos.FundID)
		if err != nil {
			return nil, err
		}
		if fund == nil {
			continue
		}
		summary, err := s.SummariseFund(fund)
		if err != nil {
			return nil, err
		}
		share := 0.0
		currentValue := 0.0
		if summary.TotalInvestedRSD > 0 {
			share = pos.UkupanUlozeniIznos / summary.TotalInvestedRSD
			currentValue = round2RSD(share * summary.FundValueRSD)
		}
		views = append(views, FundPositionView{
			FundID:           fund.ID,
			FundNaziv:        fund.Naziv,
			UkupanUlozeniRSD: round2RSD(pos.UkupanUlozeniIznos),
			UdeoProcenat:     round2RSD(share * 100),
			TrenutnaVrednost: currentValue,
			ProfitRSD:        round2RSD(currentValue - pos.UkupanUlozeniIznos),
			FundValueRSD:     summary.FundValueRSD,
		})
	}
	return views, nil
}

func (s *FundService) GetClientPosition(clientID uint, clientType string, fundID uint) (*FundPositionView, error) {
	pos, err := s.fundRepo.GetPosition(clientID, clientType, fundID)
	if err != nil {
		return nil, err
	}
	if pos == nil {
		return nil, ErrFundPositionNone
	}
	fund, err := s.GetFund(fundID)
	if err != nil {
		return nil, err
	}
	summary, err := s.SummariseFund(fund)
	if err != nil {
		return nil, err
	}
	share := 0.0
	currentValue := 0.0
	if summary.TotalInvestedRSD > 0 {
		share = pos.UkupanUlozeniIznos / summary.TotalInvestedRSD
		currentValue = round2RSD(share * summary.FundValueRSD)
	}
	return &FundPositionView{
		FundID:           fund.ID,
		FundNaziv:        fund.Naziv,
		UkupanUlozeniRSD: round2RSD(pos.UkupanUlozeniIznos),
		UdeoProcenat:     round2RSD(share * 100),
		TrenutnaVrednost: currentValue,
		ProfitRSD:        round2RSD(currentValue - pos.UkupanUlozeniIznos),
		FundValueRSD:     summary.FundValueRSD,
	}, nil
}

func (s *FundService) ListFundHoldings(fundID uint) ([]models.PortfolioHoldingRecord, error) {
	return s.portfolioRepo.ListHoldingsForUser(fundID, models.PortfolioOwnerFund)
}

// --- Performance ---

func (s *FundService) RecordDailyPerformance(referenceTime time.Time) error {
	if referenceTime.IsZero() {
		referenceTime = time.Now().UTC()
	}
	snapshotDate := time.Date(referenceTime.Year(), referenceTime.Month(), referenceTime.Day(), 0, 0, 0, 0, time.UTC)

	funds, err := s.fundRepo.ListFunds()
	if err != nil {
		return err
	}
	for i := range funds {
		summary, err := s.SummariseFund(&funds[i])
		if err != nil {
			return err
		}
		if err := s.fundRepo.SavePerformanceSnapshot(funds[i].ID, snapshotDate, summary.FundValueRSD); err != nil {
			return err
		}
	}
	return nil
}

func (s *FundService) GetPerformance(fundID uint, granularity string) ([]models.FundPerformanceHistoryRecord, error) {
	now := time.Now().UTC()
	var from time.Time
	switch strings.ToLower(granularity) {
	case "monthly", "mesecno", "month", "":
		from = now.AddDate(0, -1, 0)
	case "quarterly", "kvartalno", "quarter":
		from = now.AddDate(0, -3, 0)
	case "yearly", "godisnje", "year":
		from = now.AddDate(-1, 0, 0)
	case "all":
		from = time.Time{}
	default:
		return nil, fmt.Errorf("nepoznata granularnost: %s", granularity)
	}
	if from.IsZero() {
		from = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return s.fundRepo.ListPerformance(fundID, from, now)
}

// --- Manager reassignment ---

// ReassignManagedFunds moves every fund managed by `oldManagerID` to be managed
// by `newManagerID`. Returns the number of funds reassigned.
func (s *FundService) ReassignManagedFunds(oldManagerID, newManagerID uint) (int64, error) {
	if oldManagerID == 0 || newManagerID == 0 {
		return 0, fmt.Errorf("oldManagerID i newManagerID su obavezni")
	}
	return s.fundRepo.ReassignFundsManager(oldManagerID, newManagerID)
}

// --- Buy-for-fund pre-flight ---

// ValidateFundBuyOrder ensures that the supervisor `actorID` manages the fund
// `fundID`, that the order's account is the fund's own account, and that the
// fund's account currency matches the asset's currency. Called by the HTTP
// handler before delegating to OrderService.CreateOrder.
func (s *FundService) ValidateFundBuyOrder(fundID, actorID uint, accountID uint, assetCurrency string) (*models.InvestmentFundRecord, error) {
	fund, err := s.GetFund(fundID)
	if err != nil {
		return nil, err
	}
	if fund.ManagerID != actorID {
		return nil, fmt.Errorf("supervisor ne upravlja ovim fondom")
	}
	if accountID != fund.AccountID {
		return nil, fmt.Errorf("buy order mora koristiti racun fonda")
	}
	account, err := s.fundRepo.GetAccountByID(fund.AccountID)
	if err != nil || account == nil {
		return nil, fmt.Errorf("racun fonda nije pronadjen")
	}
	if account.Status != "aktivan" {
		return nil, fmt.Errorf("racun fonda nije aktivan")
	}
	return fund, nil
}

// --- Helpers ---

func (s *FundService) holdingValueInRSD(h *models.PortfolioHoldingRecord) float64 {
	priceRSD := s.assetPriceInRSD(&h.Asset)
	return priceRSD * h.Quantity
}

func (s *FundService) assetPriceInRSD(asset *models.MarketListingRecord) float64 {
	currency := asset.Exchange.Currency
	if currency == "" || currency == "RSD" {
		return asset.Price
	}
	rate, err := s.rateProvider.GetRate(currency, "RSD")
	if err != nil || rate == 0 {
		return asset.Price
	}
	return asset.Price * rate
}

func round2RSD(v float64) float64 {
	return math.Round(v*100) / 100
}

