package handler

import (
	"errors"
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/interbank"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestInterbankOtcHandler_ExerciseFinaliseHelpers(t *testing.T) {
	db := newFundTestDB(t, "ib_ex_finalise")
	db.Exec(`INSERT INTO currencies (id, kod) VALUES (1, 'RSD')`)
	// Buyer (client 7) RSD account funded + reserved.
	db.Exec(`INSERT INTO accounts (id, currency_id, status, client_id, stanje, raspolozivo_stanje) VALUES (1, 1, 'aktivan', 7, 1000, 1000)`)

	cfg := &config.Config{JWTSecret: testJWTSecret}
	reg, _ := interbank.NewRegistryFromJSON(333,
		`[{"code":444,"baseUrl":"http://127.0.0.1:1","outboundKey":"o","inboundKey":"i","displayName":"P"}]`)
	h := NewInterbankOtcHTTPHandler(
		cfg, reg, interbank.NewClient(reg),
		repository.NewInterbankOtcRepository(db), nil, repository.NewRemotePublicStockRepository(db),
		repository.NewInterbankOptionContractRepository(db),
		repository.NewInterbankExerciseRepository(db),
		repository.NewInterbankWalletRepository(db),
		repository.NewPortfolioRepository(db),
		repository.NewMarketRepository(db),
		db,
	)

	seedEx := func(txid string) *models.InterbankPendingExercise {
		row := &models.InterbankPendingExercise{
			TxRoutingNumber: 333, TxID: txid, Direction: models.InterbankExerciseDirectionOutbound,
			PartnerRoutingNumber: 444, NegotiationRoutingNumber: 444, NegotiationID: "neg",
			StockTicker: "ACME", StockAmount: 3, PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10,
			CashAmount: 40, BuyerRoutingNumber: 333, BuyerID: "client-7",
			SellerRoutingNumber: 444, SellerID: "client-9", Status: models.InterbankExerciseStatusPending,
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed exercise %s: %v", txid, err)
		}
		return row
	}
	seedContract := func(negID string) *models.InterbankOptionContract {
		c := &models.InterbankOptionContract{
			NegotiationRoutingNumber: 444, NegotiationID: negID, BuyerLocalID: "client-7",
			SellerRoutingNumber: 444, SellerID: "client-9", StockTicker: "ACME", Amount: 3,
			PricePerUnitCurrency: "RSD", PricePerUnitAmount: 10, Status: models.InterbankOptionContractStatusValid,
		}
		if err := db.Create(c).Error; err != nil {
			t.Fatalf("seed contract %s: %v", negID, err)
		}
		return c
	}

	// finaliseExerciseCommit: debit cash, add stock holding, mark exercised.
	c1, ct1 := seedEx("ex-commit"), seedContract("n1")
	if err := h.finaliseExerciseCommit(c1, ct1, 99, 1, 7); err != nil {
		t.Fatalf("finaliseExerciseCommit: %v", err)
	}
	if c1.Status != models.InterbankExerciseStatusCommitted {
		t.Errorf("commit status=%s", c1.Status)
	}
	h.markExerciseCommitDispatched(c1)

	// finaliseExerciseRejected: release + mark rejected + finalise.
	c2, ct2 := seedEx("ex-reject"), seedContract("n2")
	if err := h.finaliseExerciseRejected(c2, ct2, "partner NO"); err != nil {
		t.Fatalf("finaliseExerciseRejected: %v", err)
	}

	// finaliseExerciseTransportFailure: release + mark failed + best-effort ROLLBACK (dead URL).
	c3, ct3 := seedEx("ex-fail"), seedContract("n3")
	h.finaliseExerciseTransportFailure(c3, ct3, 444, errors.New("boom"))
}
