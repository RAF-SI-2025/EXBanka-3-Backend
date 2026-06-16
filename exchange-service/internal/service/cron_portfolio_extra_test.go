package service_test

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/service"
)

func TestCheckPriceAlerts(t *testing.T) {
	db := openTestDB(t, "check_alerts")
	if err := repository.NewPriceAlertRepository(db).Create(&models.PriceAlert{
		UserID: 1, UserType: "client", Ticker: "AAA", Condition: "ABOVE",
		Threshold: 100, NotificationEmail: "e@x.com", IsActive: true,
	}); err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	// Dead SMTP: the trigger path runs but the email send fails (alert not
	// deactivated), exercising both the match and the send-failure branch.
	email := service.NewSMTPEmailService("127.0.0.1", 1, "f@x.com")
	service.CheckPriceAlerts(db, email, "AAA", 150) // above threshold -> triggers
	service.CheckPriceAlerts(db, email, "AAA", 50)  // below -> no trigger
}

func TestPortfolioService_ExerciseOption_Errors(t *testing.T) {
	db := openTestDB(t, "exercise_errors")
	psvc := service.NewPortfolioService(
		repository.NewPortfolioRepository(db),
		service.NewTaxService(repository.NewTaxRepository(db), repository.NewMarketRepository(db), &mockRateProv{}),
		repository.NewMarketRepository(db),
		repository.NewOrderRepository(db),
	)

	if err := psvc.ExerciseOption(99999, 5); err == nil {
		t.Error("missing holding should error")
	}

	assetID := seedAsset(t, db, "STK", 50, "USD")
	h := &models.PortfolioHoldingRecord{
		UserID: 1, UserType: "client", AssetID: assetID, Quantity: 10,
		AccountID: 1, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(h).Error; err != nil {
		t.Fatalf("seed holding: %v", err)
	}
	if err := psvc.ExerciseOption(h.ID, 5); err == nil {
		t.Error("exercising a non-option holding should error")
	}
}
