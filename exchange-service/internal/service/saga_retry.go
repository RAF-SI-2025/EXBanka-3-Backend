package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
)

const (
	sagaMaxRetries     = 3
	sagaRetryStaleness = 5 * time.Minute
)

// SagaRetryRunner re-attempts compensations for sagas that got stuck in
// rolling_back. Stuck means: the saga has been in rolling_back state for
// longer than sagaRetryStaleness. After sagaMaxRetries failed attempts the
// saga is marked requires_manual_intervention.
type SagaRetryRunner struct {
	sagaRepo     *repository.SagaRepository
	otcRepo      *repository.OtcRepository
	orchestrator *SagaOrchestrator
}

func NewSagaRetryRunner(sagaRepo *repository.SagaRepository, otcRepo *repository.OtcRepository, orchestrator *SagaOrchestrator) *SagaRetryRunner {
	return &SagaRetryRunner{sagaRepo: sagaRepo, otcRepo: otcRepo, orchestrator: orchestrator}
}

func (r *SagaRetryRunner) Run() {
	cutoff := time.Now().UTC().Add(-sagaRetryStaleness)
	sagas, err := r.sagaRepo.ListStuckRollingBack(cutoff, sagaMaxRetries)
	if err != nil {
		slog.Error("saga retry: failed to list stuck sagas", "error", err)
		return
	}
	if len(sagas) == 0 {
		return
	}

	slog.Info("saga retry: starting", "count", len(sagas))
	for i := range sagas {
		r.retrySaga(&sagas[i])
	}
}

func (r *SagaRetryRunner) retrySaga(saga *models.SagaTransactionRecord) {
	steps, err := r.buildSteps(saga)
	if err != nil {
		slog.Error("saga retry: cannot rebuild steps", "saga_id", saga.ID, "error", err)
		_ = r.sagaRepo.SetStatusWithError(saga.ID, models.SagaStatusRequiresManualIntervention, err.Error())
		return
	}

	if err := r.sagaRepo.IncrementRetry(saga.ID); err != nil {
		slog.Error("saga retry: increment failed", "saga_id", saga.ID, "error", err)
		return
	}

	// Apply exponential backoff *between* attempts only when the cron picks the
	// same saga up again on a future tick. Each tick performs at most one retry
	// per saga; the cutoff math above guarantees we don't hammer it.
	if err := r.orchestrator.RetryCompensations(saga, steps); err != nil {
		slog.Error("saga retry: compensation failed again", "saga_id", saga.ID, "attempt", saga.RetryCount+1, "error", err)
		if saga.RetryCount+1 >= sagaMaxRetries {
			_ = r.sagaRepo.SetStatusWithError(saga.ID, models.SagaStatusRequiresManualIntervention, err.Error())
		}
		return
	}
	_ = r.sagaRepo.SetStatus(saga.ID, models.SagaStatusRolledBack)
	slog.Info("saga retry: rolled back successfully", "saga_id", saga.ID)
}

// buildSteps reconstructs the saga's step plan from its persisted payload.
// Only OTC exercise sagas are supported today.
func (r *SagaRetryRunner) buildSteps(saga *models.SagaTransactionRecord) ([]SagaStep, error) {
	if saga.Type != models.SagaTypeOtcExercise {
		return nil, errors.New("unknown saga type")
	}
	var payload OtcExerciseSagaPayload
	if err := json.Unmarshal([]byte(saga.Payload), &payload); err != nil {
		return nil, err
	}
	contract, err := r.otcRepo.GetContractByID(payload.ContractID)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return BuildOtcExerciseSteps(contract), nil
}
