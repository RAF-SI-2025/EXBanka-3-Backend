package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// ExchangeTimeStatus describes the current trading state of an exchange.
type ExchangeTimeStatus struct {
	IsOpen       bool   `json:"isOpen"`
	PreMarket    bool   `json:"preMarket"`
	AfterHours   bool   `json:"afterHours"`
	ManualMode   bool   `json:"manualMode"`
	Message      string `json:"message"`
	LocalTime    string `json:"localTime"`
	WorkingHours string `json:"workingHours"`
}

// GetExchangeTimeStatus checks whether the given exchange is currently open.
func GetExchangeTimeStatus(exchange models.Exchange) ExchangeTimeStatus {
	status := ExchangeTimeStatus{
		WorkingHours: exchange.WorkingHours,
		ManualMode:   exchange.UseManualTime,
	}

	// Manual override mode
	if exchange.UseManualTime {
		status.IsOpen = exchange.ManualTimeOpen
		if status.IsOpen {
			status.Message = "Exchange is OPEN (manual mode)"
		} else {
			status.Message = "Exchange is CLOSED (manual mode)"
		}
		return status
	}

	// Parse timezone
	loc, err := time.LoadLocation(exchange.Timezone)
	if err != nil {
		status.Message = fmt.Sprintf("invalid timezone: %s", exchange.Timezone)
		return status
	}

	now := time.Now().In(loc)
	status.LocalTime = now.Format("15:04:05 MST")

	// Weekend check
	weekday := now.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		status.Message = "Exchange is CLOSED (weekend)"
		return status
	}

	// Parse working hours "HH:MM-HH:MM"
	openTime, closeTime, err := parseWorkingHours(exchange.WorkingHours, now)
	if err != nil {
		status.Message = fmt.Sprintf("invalid working hours format: %s", exchange.WorkingHours)
		return status
	}

	// Pre-market: 1 hour before open
	preMarketStart := openTime.Add(-1 * time.Hour)
	// After-hours: up to 4 hours after close
	afterHoursEnd := closeTime.Add(4 * time.Hour)

	switch {
	case now.Before(preMarketStart):
		status.Message = "Exchange is CLOSED (before pre-market)"
	case now.Before(openTime):
		status.PreMarket = true
		status.Message = "Exchange is in PRE-MARKET"
	case now.Before(closeTime):
		status.IsOpen = true
		status.Message = "Exchange is OPEN"
	case now.Before(afterHoursEnd):
		status.AfterHours = true
		status.Message = "Exchange is in AFTER-HOURS"
	default:
		status.Message = "Exchange is CLOSED"
	}

	return status
}

func parseWorkingHours(wh string, refDate time.Time) (open, close time.Time, err error) {
	parts := strings.Split(wh, "-")
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("expected HH:MM-HH:MM, got %s", wh)
	}

	openParts := strings.Split(strings.TrimSpace(parts[0]), ":")
	closeParts := strings.Split(strings.TrimSpace(parts[1]), ":")
	if len(openParts) != 2 || len(closeParts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("expected HH:MM-HH:MM, got %s", wh)
	}

	oh := parseInt(openParts[0])
	om := parseInt(openParts[1])
	ch := parseInt(closeParts[0])
	cm := parseInt(closeParts[1])

	loc := refDate.Location()
	y, m, d := refDate.Date()
	open = time.Date(y, m, d, oh, om, 0, 0, loc)
	close = time.Date(y, m, d, ch, cm, 0, 0, loc)

	return open, close, nil
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}
