package service

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestEmitNotifications_AllBranches(t *testing.T) {
	n := notify.NewClient("", "") // no-op Emit

	// emitOrderNotification: nil notifier / nil order / no recipient / emit.
	emitOrderNotification(nil, nil, "T", "t", "b")
	emitOrderNotification(n, nil, "T", "t", "b")
	emitOrderNotification(n, &models.OrderRecord{}, "T", "t", "b")
	emitOrderNotification(n, &models.OrderRecord{
		NotifyUserID: 5, NotifyUserType: "client", NotifyEmail: "e@x.com",
	}, "ORDER_DONE", "Done", "body")

	// emitOtcNotification: nil / invalid recipient / invalid type / emit.
	emitOtcNotification(nil, 5, "client", "T", "t", "b")
	emitOtcNotification(n, 0, "client", "T", "t", "b")
	emitOtcNotification(n, 5, "bogus", "T", "t", "b")
	emitOtcNotification(n, 5, "client", "OTC_X", "t", "b")

	// WithNotifier setter (does not dereference its repo args).
	if NewOrderService(nil, nil, nil).WithNotifier(n) == nil {
		t.Error("WithNotifier returned nil")
	}
}

func TestCronHelpers_EmptyDB(t *testing.T) {
	db := openSagaTestDB(t, "cron_helpers")
	otcSvc := NewOtcService(repository.NewPortfolioRepository(db), repository.NewOtcRepository(db)).
		WithNotifier(notify.NewClient("", ""))

	// All no-ops on an empty DB, but exercise the cron helper bodies.
	remindExpiringOtcContracts(otcSvc)
	expireDueOtcContracts(otcSvc)
	expireDueInterbankReservations(repository.NewInterbankOtcRepository(db))
}
