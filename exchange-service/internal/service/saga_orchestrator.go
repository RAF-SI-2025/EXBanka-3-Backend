package service

import (
	"fmt"
	"log/slog"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/repository"
	"gorm.io/gorm"
)

// SagaStep is one unit of work in a saga: a forward action and its compensation.
// Each Forward and Compensate runs inside its own DB transaction so individual
// step failures roll back partial DB writes for that step before moving on.
type SagaStep struct {
	Name       string
	Forward    func(tx *gorm.DB) error
	Compensate func(tx *gorm.DB) error
}

type SagaOrchestrator struct {
	sagaRepo *repository.SagaRepository
	db       *gorm.DB
}

func NewSagaOrchestrator(sagaRepo *repository.SagaRepository, db *gorm.DB) *SagaOrchestrator {
	return &SagaOrchestrator{sagaRepo: sagaRepo, db: db}
}

// Run starts a fresh saga: creates the transaction record, persists per-step
// records, then executes Forward functions sequentially. On any failure it
// rolls back compensations from the last completed step backward.
// Returns the saga ID and a non-nil error if the saga (including compensations)
// did not finish cleanly.
func (o *SagaOrchestrator) Run(sagaType, payload string, steps []SagaStep) (uint, error) {
	saga := &models.SagaTransactionRecord{
		Type:    sagaType,
		Status:  models.SagaStatusInProgress,
		Payload: payload,
	}
	if err := o.sagaRepo.CreateTransaction(saga); err != nil {
		return 0, fmt.Errorf("create saga transaction: %w", err)
	}

	stepRecords := make([]*models.SagaStepRecord, 0, len(steps))
	for i, step := range steps {
		rec, err := o.sagaRepo.AppendStep(saga.ID, i+1, step.Name)
		if err != nil {
			_ = o.sagaRepo.SetStatusWithError(saga.ID, models.SagaStatusFailed, err.Error())
			return saga.ID, fmt.Errorf("append step %d: %w", i+1, err)
		}
		stepRecords = append(stepRecords, rec)
	}

	for i, step := range steps {
		rec := stepRecords[i]
		_ = o.sagaRepo.MarkStepInProgress(rec.ID)
		_ = o.sagaRepo.SetCurrentStep(saga.ID, i+1)

		err := o.db.Transaction(func(tx *gorm.DB) error {
			return step.Forward(tx)
		})
		if err != nil {
			slog.Error("saga step failed", "saga_id", saga.ID, "step", step.Name, "error", err)
			_ = o.sagaRepo.MarkStepFailed(rec.ID, err.Error())
			_ = o.sagaRepo.SetStatusWithError(saga.ID, models.SagaStatusRollingBack, err.Error())

			compErr := o.compensateBackward(saga.ID, steps, stepRecords, i-1)
			if compErr != nil {
				_ = o.sagaRepo.SetStatusWithError(saga.ID, models.SagaStatusRollingBack, compErr.Error())
				return saga.ID, fmt.Errorf("step %s failed: %w; compensation error: %v", step.Name, err, compErr)
			}
			_ = o.sagaRepo.SetStatus(saga.ID, models.SagaStatusRolledBack)
			return saga.ID, fmt.Errorf("step %s failed: %w", step.Name, err)
		}
		_ = o.sagaRepo.MarkStepCompleted(rec.ID)
	}

	_ = o.sagaRepo.SetStatus(saga.ID, models.SagaStatusCompleted)
	return saga.ID, nil
}

// compensateBackward runs Compensate for every step from index `from` down to 0
// whose record is currently in completed (or in_progress) state. Compensations
// run in reverse order. Returns the first compensation error, after attempting
// the rest.
func (o *SagaOrchestrator) compensateBackward(sagaID uint, steps []SagaStep, recs []*models.SagaStepRecord, from int) error {
	var firstErr error
	for i := from; i >= 0; i-- {
		step := steps[i]
		rec := recs[i]
		if step.Compensate == nil {
			continue
		}
		err := o.db.Transaction(func(tx *gorm.DB) error {
			return step.Compensate(tx)
		})
		if err != nil {
			slog.Error("saga compensation failed", "saga_id", sagaID, "step", step.Name, "error", err)
			_ = o.sagaRepo.MarkStepFailed(rec.ID, "compensation: "+err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = o.sagaRepo.MarkStepCompensated(rec.ID)
	}
	return firstErr
}

// RetryCompensations re-attempts compensations for a saga stuck in
// rolling_back. Compensates any step still marked completed (i.e. not yet
// compensated). Caller is responsible for choosing which sagas to retry and
// for incrementing retry_count.
func (o *SagaOrchestrator) RetryCompensations(saga *models.SagaTransactionRecord, steps []SagaStep) error {
	stepsByNumber := make(map[int]*models.SagaStepRecord, len(saga.Steps))
	for i := range saga.Steps {
		stepsByNumber[saga.Steps[i].StepNumber] = &saga.Steps[i]
	}

	var firstErr error
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		rec, ok := stepsByNumber[i+1]
		if !ok {
			continue
		}
		if rec.Status != models.SagaStepStatusCompleted {
			continue
		}
		if step.Compensate == nil {
			_ = o.sagaRepo.MarkStepCompensated(rec.ID)
			continue
		}
		err := o.db.Transaction(func(tx *gorm.DB) error {
			return step.Compensate(tx)
		})
		if err != nil {
			slog.Error("saga retry compensation failed", "saga_id", saga.ID, "step", step.Name, "error", err)
			_ = o.sagaRepo.MarkStepFailed(rec.ID, "retry compensation: "+err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = o.sagaRepo.MarkStepCompensated(rec.ID)
	}
	return firstErr
}
