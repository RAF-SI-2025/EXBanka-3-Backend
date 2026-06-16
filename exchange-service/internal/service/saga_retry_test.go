package service

import (
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
)

func TestSagaRetryRunner_BuildStepsErrors(t *testing.T) {
	db := openSagaTestDB(t, "saga_retry")
	sagaRepo := repository.NewSagaRepository(db)
	r := NewSagaRetryRunner(sagaRepo, repository.NewOtcRepository(db), NewSagaOrchestrator(sagaRepo, db))

	// No stuck sagas -> early return.
	r.Run()

	past := time.Now().UTC().Add(-time.Hour)
	seed := func(typ, payload string) uint {
		s := &models.SagaTransactionRecord{
			Type: typ, Status: models.SagaStatusRollingBack, Payload: payload,
			RetryCount: 0, CreatedAt: past, UpdatedAt: past,
		}
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("seed saga: %v", err)
		}
		// Force updated_at into the past so the staleness filter selects it.
		db.Model(&models.SagaTransactionRecord{}).Where("id = ?", s.ID).UpdateColumn("updated_at", past)
		return s.ID
	}

	// Three sagas whose steps can't be rebuilt -> each goes to manual intervention.
	ids := []uint{
		seed("unknown_type", "{}"),
		seed(models.SagaTypeOtcExercise, "not-json"),
		seed(models.SagaTypeOtcExercise, `{"contractId":99999}`),
	}

	r.Run()

	for _, id := range ids {
		var s models.SagaTransactionRecord
		if err := db.First(&s, id).Error; err != nil {
			t.Fatalf("get saga %d: %v", id, err)
		}
		if s.Status != models.SagaStatusRequiresManualIntervention {
			t.Errorf("saga %d status=%s, want requires_manual_intervention", id, s.Status)
		}
	}
}
