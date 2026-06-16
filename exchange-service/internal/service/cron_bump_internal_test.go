package service

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

type cronRateProv struct{}

func (cronRateProv) GetRate(from, to string) (float64, error) { return 100, nil }
func (cronRateProv) GetAllRates() []ExchangeRate             { return nil }

func TestStartCronJobs_AllDepsWired(t *testing.T) {
	db := openSagaTestDB(t, "cron_all_deps")
	rates := cronRateProv{}

	portfolioRepo := repository.NewPortfolioRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	taxRepo := repository.NewTaxRepository(db)
	otcRepo := repository.NewOtcRepository(db)
	sagaRepo := repository.NewSagaRepository(db)
	dividendRepo := repository.NewDividendRepository(db)

	taxSvc := NewTaxService(taxRepo, marketRepo, rates)
	portfolioSvc := NewPortfolioService(portfolioRepo, taxSvc, marketRepo, orderRepo)
	fundSvc := NewFundService(repository.NewFundRepository(db), portfolioRepo, marketRepo, orderRepo, rates)
	dividendSvc := NewDividendService(dividendRepo, orderRepo, taxSvc, rates)
	sagaRetry := NewSagaRetryRunner(sagaRepo, otcRepo, NewSagaOrchestrator(sagaRepo, db))

	reg, _ := interbank.NewRegistryFromJSON(333, "[]")
	client := interbank.NewClient(reg)
	ibReconcile := NewInterbankReconcileRunner(
		db, reg, client,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	)
	ibCache := NewPublicStockCacheRunner(reg, client, repository.NewRemotePublicStockRepository(db))
	emailSvc := NewSMTPEmailService("127.0.0.1", 1, "f@x.com")
	notifier := notify.NewClient("", "")

	c := StartCronJobs(db, portfolioSvc, rates, sagaRetry, fundSvc, dividendSvc, ibReconcile, ibCache, emailSvc, notifier)
	if c == nil {
		t.Fatal("StartCronJobs returned nil")
	}
	c.Stop()
}

func TestInterbankReconcile_BumpHelpers(t *testing.T) {
	db := openSagaTestDB(t, "ib_bumps")
	r := NewInterbankReconcileRunner(
		db, nil, nil,
		repository.NewInterbankPaymentRepository(db),
		repository.NewInterbankPaymentWalletRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewInterbankOtcRepository(db),
	)
	past := time.Now().UTC().Add(-time.Hour)

	neg := &models.InterbankOtcNegotiation{
		NegotiationRoutingNumber: 333, NegotiationID: "n-bump", LocalRole: models.InterbankNegotiationRoleSeller,
		CounterpartyRoutingNumber: 444, BuyerRoutingNumber: 444, BuyerID: "b", SellerRoutingNumber: 333, SellerID: "s",
		StockTicker: "ACME", Amount: 1, PricePerUnitCurrency: "RSD", PricePerUnitAmount: 1,
		PremiumCurrency: "RSD", PremiumAmount: 1, SettlementDate: past.Format(time.RFC3339),
		LastModifiedByRoutingNumber: 444, LastModifiedByID: "b", UpdatedAt: past,
	}
	if err := db.Create(neg).Error; err != nil {
		t.Fatalf("seed negotiation: %v", err)
	}
	r.bumpNegotiationUpdatedAt(neg)

	ex := &models.InterbankPendingExercise{
		TxRoutingNumber: 333, TxID: "ex-bump", Direction: models.InterbankExerciseDirectionOutbound,
		PartnerRoutingNumber: 444, NegotiationRoutingNumber: 444, NegotiationID: "n",
		StockTicker: "ACME", StockAmount: 1, PricePerUnitCurrency: "RSD", PricePerUnitAmount: 1,
		CashAmount: 1, BuyerRoutingNumber: 333, BuyerID: "b", SellerRoutingNumber: 444, SellerID: "s",
		Status: models.InterbankExerciseStatusPending, CreatedAt: past, UpdatedAt: past,
	}
	if err := db.Create(ex).Error; err != nil {
		t.Fatalf("seed exercise: %v", err)
	}
	r.bumpExerciseUpdatedAt(ex)
}
