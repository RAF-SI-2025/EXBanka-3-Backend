package service

import (
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// StartCronJobs sets up and starts the cron scheduler for the exchange service.
// portfolioSvc is created in main and shared with the portfolio HTTP handler.
// sagaRetry is optional; when non-nil it is invoked every 5 minutes to retry
// stuck SAGA compensations.
func StartCronJobs(db *gorm.DB, portfolioSvc *PortfolioService, rateProvider RateProviderInterface, sagaRetry *SagaRetryRunner, fundSvc *FundService) *cron.Cron {
	c := cron.New()

	// Refresh listing prices every 15 minutes.
	_, err := c.AddFunc("@every 15m", func() {
		refreshListingPrices(db)
	})
	if err != nil {
		slog.Error("Failed to add price refresh cron job", "error", err)
	}

	// Order execution engine: attempt to fill active orders every minute.
	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	executor := NewOrderExecutor(orderRepo, marketRepo, portfolioSvc, rateProvider)
	_, err = c.AddFunc("@every 1m", func() {
		executor.Run()
	})
	if err != nil {
		slog.Error("Failed to add order executor cron job", "error", err)
	}

	// OTC option expiry: release reserved shares after settlement date passes.
	otcSvc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db))
	_, err = c.AddFunc("@every 1h", func() {
		expireDueOtcContracts(otcSvc)
	})
	if err != nil {
		slog.Error("Failed to add OTC expiry cron job", "error", err)
	}

	// SAGA retry: re-run compensations for sagas stuck in rolling_back.
	if sagaRetry != nil {
		_, err = c.AddFunc("@every 5m", func() {
			sagaRetry.Run()
		})
		if err != nil {
			slog.Error("Failed to add SAGA retry cron job", "error", err)
		}
	}

	// Monthly tax collection: runs at 02:00 on the 1st of each month.
	taxRepo := repository.NewTaxRepository(db)
	taxSvc := NewTaxService(taxRepo, marketRepo, rateProvider)
	taxCollector := NewTaxCollector(taxSvc, orderRepo, taxRepo)
	_, err = c.AddFunc("0 2 1 * *", func() {
		period := PreviousMonthPeriod()
		slog.Info("Starting monthly tax collection", "period", period)
		res := taxCollector.CollectForPeriod(period)
		slog.Info("Monthly tax collection finished",
			"period", res.Period,
			"users_processed", res.UsersProcessed,
			"total_collected_rsd", res.TotalCollected,
			"debts", len(res.Debts),
		)
	})
	if err != nil {
		slog.Error("Failed to add tax collection cron job", "error", err)
	}

	// Daily fund performance snapshot: runs at 23:55 UTC every day.
	if fundSvc != nil {
		_, err = c.AddFunc("55 23 * * *", func() {
			if err := fundSvc.RecordDailyPerformance(time.Now().UTC()); err != nil {
				slog.Error("Failed to record fund performance snapshots", "error", err)
				return
			}
			slog.Info("Recorded daily fund performance snapshots")
		})
		if err != nil {
			slog.Error("Failed to add fund snapshot cron job", "error", err)
		}
	}

	c.Start()
	slog.Info("Exchange-service cron jobs started", "jobs", len(c.Entries()))
	return c
}

func expireDueOtcContracts(otcSvc *OtcService) {
	expired, err := otcSvc.ExpireDueContracts(time.Now().UTC())
	if err != nil {
		slog.Error("Failed to expire OTC contracts", "error", err)
		return
	}
	if expired > 0 {
		slog.Info("Expired OTC contracts", "count", expired)
	}
}

func refreshListingPrices(db *gorm.DB) {
	slog.Info("Starting listing price refresh...")

	var listings []models.MarketListingRecord
	if err := db.Where("type != ?", "option").Find(&listings).Error; err != nil {
		slog.Error("Failed to load listings for refresh", "error", err)
		return
	}

	now := time.Now().UTC()
	updated := 0

	for _, listing := range listings {
		// Simulate small price movements (±2%) since we don't have a live API key guaranteed.
		// When a real Alpha Vantage key is configured, this can be replaced with live fetches.
		drift := (rand.Float64() - 0.5) * 0.04 // ±2%
		newPrice := math.Round(listing.Price*(1+drift)*100) / 100
		if newPrice < 0.01 {
			newPrice = 0.01
		}
		newAsk := math.Round(newPrice*1.002*100) / 100
		newBid := math.Round(newPrice*0.998*100) / 100
		change := math.Round((newPrice-listing.Price)*100) / 100

		// Volume fluctuation
		volDrift := 0.9 + rand.Float64()*0.2
		newVolume := int64(math.Round(float64(listing.Volume) * volDrift))

		if err := db.Model(&models.MarketListingRecord{}).
			Where("id = ?", listing.ID).
			Updates(map[string]interface{}{
				"price":        newPrice,
				"ask":          newAsk,
				"bid":          newBid,
				"volume":       newVolume,
				"last_refresh": now,
			}).Error; err != nil {
			slog.Error("Failed to update listing price", "ticker", listing.Ticker, "error", err)
			continue
		}

		// Record daily price snapshot if new day
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		var existing models.MarketListingDailyPriceInfoRecord
		err := db.Where("listing_id = ? AND date = ?", listing.ID, today).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			dailyRecord := models.MarketListingDailyPriceInfoRecord{
				ListingID: listing.ID,
				Date:      today,
				Price:     newPrice,
				High:      newAsk,
				Low:       newBid,
				Change:    change,
				Volume:    newVolume,
			}
			db.Create(&dailyRecord)
		} else if err == nil {
			// Update today's high/low
			updates := map[string]interface{}{
				"price":  newPrice,
				"change": change,
				"volume": newVolume,
			}
			if newAsk > existing.High {
				updates["high"] = newAsk
			}
			if newBid < existing.Low {
				updates["low"] = newBid
			}
			db.Model(&existing).Updates(updates)
		}

		updated++
	}

	slog.Info("Listing price refresh complete", "updated", updated)
}
