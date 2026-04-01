package service

import (
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// StartCronJobs sets up and starts the cron scheduler for the exchange service.
func StartCronJobs(db *gorm.DB) *cron.Cron {
	c := cron.New()

	// Refresh listing prices every 15 minutes
	_, err := c.AddFunc("@every 15m", func() {
		refreshListingPrices(db)
	})
	if err != nil {
		slog.Error("Failed to add price refresh cron job", "error", err)
	}

	c.Start()
	slog.Info("Exchange-service cron jobs started", "jobs", len(c.Entries()))
	return c
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
