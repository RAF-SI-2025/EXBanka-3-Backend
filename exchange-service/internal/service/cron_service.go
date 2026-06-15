package service

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// StartCronJobs sets up and starts the cron scheduler for the exchange service.
// portfolioSvc is created in main and shared with the portfolio HTTP handler.
// sagaRetry is optional; when non-nil it is invoked every 5 minutes to retry
// stuck SAGA compensations. interbankReconcile is optional; when non-nil it
// is invoked every 2 minutes to retry stuck cross-bank payments.
// publicStockCache is optional; when non-nil it is invoked every 5 minutes
// to refresh partner /public-stock snapshots.
func StartCronJobs(
	db *gorm.DB,
	portfolioSvc *PortfolioService,
	rateProvider RateProviderInterface,
	sagaRetry *SagaRetryRunner,
	fundSvc *FundService,
	dividendSvc *DividendService,
	interbankReconcile *InterbankReconcileRunner,
	publicStockCache *PublicStockCacheRunner,
	emailSvc EmailSender,
	notifier *notify.Client,
) *cron.Cron {
	c := cron.New()

	// Refresh listing prices every 15 minutes; check price alerts after each refresh.
	_, err := c.AddFunc("@every 15m", func() {
		refreshListingPrices(db, emailSvc)
	})
	if err != nil {
		slog.Error("Failed to add price refresh cron job", "error", err)
	}

	// Order execution engine: attempt to fill active orders every minute.
	orderRepo := repository.NewOrderRepository(db)
	marketRepo := repository.NewMarketRepository(db)
	executor := NewOrderExecutor(orderRepo, marketRepo, portfolioSvc, rateProvider).WithNotifier(notifier)
	_, err = c.AddFunc("@every 1m", func() {
		executor.Run()
	})
	if err != nil {
		slog.Error("Failed to add order executor cron job", "error", err)
	}

	// OTC option expiry: release reserved shares after settlement date passes,
	// and remind both parties a few days before settlement.
	otcSvc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db)).WithNotifier(notifier)
	ibOtcRepo := repository.NewInterbankOtcRepository(db)
	_, err = c.AddFunc("@every 1h", func() {
		expireDueOtcContracts(otcSvc)
		expireDueInterbankReservations(ibOtcRepo)
		remindExpiringOtcContracts(otcSvc)
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

	// Inter-bank payment reconciliation: retransmit stuck NEW_TX and
	// pending COMMIT_TX/ROLLBACK_TX messages every 2 minutes. The
	// partner's idempotency by transactionId means replays are safe.
	if interbankReconcile != nil {
		_, err = c.AddFunc("@every 2m", func() {
			interbankReconcile.Run()
		})
		if err != nil {
			slog.Error("Failed to add inter-bank reconcile cron job", "error", err)
		}
	}

	// Partner /public-stock cache refresh: fan out every 5 minutes so
	// the local cross-bank OTC discovery page reads pre-fetched
	// snapshots instead of paying the network cost on every request.
	if publicStockCache != nil {
		// Kick off one refresh immediately so the cache is warm on
		// process start rather than waiting up to 5m for the first
		// tick.
		go publicStockCache.Run()
		_, err = c.AddFunc("@every 5m", func() {
			publicStockCache.Run()
		})
		if err != nil {
			slog.Error("Failed to add public-stock cache cron job", "error", err)
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

	// Quarterly dividend payout: a daily 23:30 UTC tick that only acts on the
	// last working day of March/June/September/December (§Celina 3). Idempotent
	// per quarter, so a missed or duplicated tick never double-pays.
	if dividendSvc != nil {
		_, err = c.AddFunc("30 23 * * *", func() {
			now := time.Now().UTC()
			if !IsLastWorkingDayOfQuarter(now) {
				return
			}
			slog.Info("Starting quarterly dividend distribution", "period", QuarterPeriod(now))
			res, derr := dividendSvc.DistributeForDate(now)
			if derr != nil {
				slog.Error("Quarterly dividend distribution failed", "error", derr)
			} else {
				slog.Info("Quarterly dividend distribution finished",
					"period", res.Period, "eligible", res.Eligible,
					"paid", res.PaidOut, "skipped", res.Skipped, "failed", res.Failed)
			}
			// Fund-held stocks pay dividends too (Celina 4) — distributed into the
			// fund per its policy. Runs after the client/bank payout above.
			if fundSvc != nil {
				fres, ferr := fundSvc.DistributeFundDividends(now)
				if ferr != nil {
					slog.Error("Quarterly fund dividend distribution failed", "error", ferr)
				} else {
					slog.Info("Quarterly fund dividend distribution finished",
						"period", fres.Period, "eligible", fres.Eligible,
						"processed", fres.Processed, "skipped", fres.Skipped, "failed", fres.Failed)
				}
			}
		})
		if err != nil {
			slog.Error("Failed to add dividend payout cron job", "error", err)
		}
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

// otcExpiryReminderDays is how many days before settlement both parties are
// reminded that an OTC option contract is about to expire.
const otcExpiryReminderDays = 3

func remindExpiringOtcContracts(otcSvc *OtcService) {
	reminded, err := otcSvc.SendExpiryReminders(time.Now().UTC(), otcExpiryReminderDays)
	if err != nil {
		slog.Error("Failed to send OTC expiry reminders", "error", err)
		return
	}
	if reminded > 0 {
		slog.Info("Sent OTC expiry reminders", "count", reminded)
	}
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

// expireDueInterbankReservations un-reserves the seller's stock for
// cross-bank OTC options that were never exercised and whose settlement
// date has passed (§2.7.2). The backstop for any reservation a failed
// accept left behind, and the normal path for accepted-but-unexercised
// options.
func expireDueInterbankReservations(ibOtcRepo *repository.InterbankOtcRepository) {
	released, err := ibOtcRepo.ExpireDueSellerReservations(time.Now().UTC())
	if err != nil {
		slog.Error("Failed to expire inter-bank seller reservations", "error", err)
		return
	}
	if released > 0 {
		slog.Info("Released expired inter-bank seller reservations", "count", released)
	}
}

func refreshListingPrices(db *gorm.DB, emailSvc EmailSender) {
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

		// Check price alerts AFTER the price has been successfully persisted.
		CheckPriceAlerts(db, emailSvc, listing.Ticker, newPrice)

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

// CheckPriceAlerts evaluates all active alerts for the given ticker against
// newPrice. For each triggered alert it:
//  1. Sends an email notification to the address stored at alert creation.
//  2. Deactivates the alert (is_active = false) — only on successful email send.
//
// Exported so it can be called directly from tests without going through the
// 15-minute cron schedule.
func CheckPriceAlerts(db *gorm.DB, emailSvc EmailSender, ticker string, newPrice float64) {
	alertRepo := repository.NewPriceAlertRepository(db)
	alerts, err := alertRepo.GetActiveByTicker(ticker)
	if err != nil {
		slog.Error("CheckPriceAlerts: fetch failed", "ticker", ticker, "error", err)
		return
	}
	for _, alert := range alerts {
		triggered := false
		switch alert.Condition {
		case "ABOVE":
			triggered = newPrice >= alert.Threshold // scenario 27: ABOVE fires at >=
		case "BELOW":
			triggered = newPrice <= alert.Threshold // scenario 27: BELOW fires at <=
		}
		if !triggered {
			continue
		}

		subject := fmt.Sprintf("Price Alert: %s %s %.2f", alert.Ticker, alert.Condition, alert.Threshold)
		body := fmt.Sprintf(
			"Your price alert has been triggered.\n\nAsset: %s\nCondition: %s %.2f\nCurrent price: %.2f\n\nThis alert has been deactivated and will not fire again.",
			alert.Ticker, alert.Condition, alert.Threshold, newPrice,
		)
		if err := emailSvc.Send(alert.NotificationEmail, subject, body); err != nil {
			slog.Error("CheckPriceAlerts: email failed, alert NOT deactivated",
				"alertID", alert.ID, "error", err)
			continue // do not deactivate — retry on next tick
		}

		// Scenario 27: deactivate AFTER successful email so alert never fires again.
		if err := alertRepo.Deactivate(alert.ID); err != nil {
			slog.Error("CheckPriceAlerts: deactivate failed",
				"alertID", alert.ID, "error", err)
		} else {
			slog.Info("Price alert triggered and deactivated",
				"alertID", alert.ID, "ticker", ticker, "newPrice", newPrice)
		}
	}
}
