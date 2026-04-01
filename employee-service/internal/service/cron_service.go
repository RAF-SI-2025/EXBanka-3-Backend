package service

import (
	"log/slog"

	"github.com/robfig/cron/v3"
)

// StartCronJobs sets up and starts the cron scheduler for the employee service.
func StartCronJobs(empSvc *EmployeeService) *cron.Cron {
	c := cron.New()

	// Reset all agent usedLimit at 23:59 every day
	_, err := c.AddFunc("59 23 * * *", func() {
		count, err := empSvc.ResetAllAgentUsedLimits()
		if err != nil {
			slog.Error("Failed to reset agent used limits", "error", err)
			return
		}
		slog.Info("Daily agent used limit reset complete", "agents_reset", count)
	})
	if err != nil {
		slog.Error("Failed to add daily limit reset cron job", "error", err)
	}

	c.Start()
	slog.Info("Employee-service cron jobs started", "jobs", len(c.Entries()))
	return c
}
