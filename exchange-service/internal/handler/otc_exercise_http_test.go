package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/config"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

// TestOtcHTTP_ExerciseContractAndSagaStatus drives the local OTC exercise SAGA
// through the handler, then reads the saga status — covering exerciseContract +
// getSagaStatus + the saga response mapper.
func TestOtcHTTP_ExerciseContractAndSagaStatus(t *testing.T) {
	db := newTestDB(t, "h_otc_exercise")
	seedOtcHandlerAccounts(t, db) // accounts 1 (client 200), 2 (client 100), USD
	_, listingID := seedExchangeAndListing(t, db, "HEX")
	now := time.Now().UTC()
	if err := db.Create(&models.PortfolioHoldingRecord{
		UserID: 200, UserType: "client", AssetID: listingID, Quantity: 10,
		ReservedQuantity: 3, PublicQuantity: 10, AvgBuyPrice: 90, AccountID: 1, CreatedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	var holding models.PortfolioHoldingRecord
	db.Where("user_id = 200").First(&holding)
	ct := &models.OtcContractRecord{
		StockListingID: listingID, SellerHoldingID: holding.ID, Amount: 3, StrikePrice: 50,
		SettlementDate: now.AddDate(0, 0, 1),
		BuyerID:        100, BuyerType: "client", BuyerAccountID: 2,
		SellerID:       200, SellerType: "client", SellerAccountID: 1,
		Status:         models.OtcContractStatusValid, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(ct).Error; err != nil {
		t.Fatalf("seed contract: %v", err)
	}

	cfg := &config.Config{JWTSecret: testJWTSecret}
	sagaRepo := repository.NewSagaRepository(db)
	otcSvc := service.NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db)).
		WithOrchestrator(service.NewSagaOrchestrator(sagaRepo, db))
	h := NewOtcHTTPHandler(cfg, otcSvc).WithSagaQuerier(sagaRepo)

	// Buyer (client 100) exercises.
	rec := do(t, h.OtcRoutes, http.MethodPost, fmt.Sprintf("/api/v1/otc/contracts/%d/exercise", ct.ID), clientToken(t), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("exercise status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		SagaID uint `json:"sagaId"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.SagaID == 0 {
		t.Fatal("expected a saga id")
	}

	// Read the saga status.
	if rec := do(t, h.OtcRoutes, http.MethodGet, fmt.Sprintf("/api/v1/otc/saga/%d", resp.SagaID), clientToken(t), ""); rec.Code != http.StatusOK {
		t.Errorf("saga status=%d body=%s", rec.Code, rec.Body.String())
	}
}
