package service

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

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

// maxInlineCompensateAttempts bounds how many times a single compensator is
// retried inline (within the request) before the saga is left in rolling_back
// for the retry cron. Compensators are idempotent and transactional, so an
// immediate retry is safe; this lets a transient compensation failure self-heal
// without waiting for the cron, and gives the SAGA spec's in-request convergence
// (files/SAGA.md).
const maxInlineCompensateAttempts = 3

// logAttempt appends one entry to the saga's append-only log. It is called
// outside the step's own transaction so an "err" entry survives even though the
// failed step's side effects rolled back. Best-effort: a logging failure must
// not abort the saga.
func (o *SagaOrchestrator) logAttempt(sagaID uint, phase string, cause error) {
	outcome := models.SagaAttemptOutcomeOK
	msg := ""
	if cause != nil {
		outcome = models.SagaAttemptOutcomeErr
		msg = cause.Error()
	}
	_ = o.sagaRepo.AppendAttempt(sagaID, phase, outcome, msg)
}

// Run starts a fresh saga: creates the transaction record, persists per-step
// records, then executes Forward functions sequentially. On any failure it
// rolls back compensations from the last completed step backward.
// Returns the saga ID and a non-nil error if the saga (including compensations)
// did not finish cleanly.
func (o *SagaOrchestrator) Run(sagaType, payload string, steps []SagaStep) (uint, error) {
	return o.RunWithFaults(sagaType, payload, steps, nil)
}

// RunWithFaults is Run with an optional fault-injection config. faults is nil in
// production (Run delegates here with nil); it is only ever non-nil in the SAGA
// test suite, behind the SAGA_FAULT_HOOKS env flag (files/SAGA.md). With faults
// == nil the behavior is identical to the original Run.
func (o *SagaOrchestrator) RunWithFaults(sagaType, payload string, steps []SagaStep, faults *SagaFaultConfig) (uint, error) {
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

		if d := faults.delayFor(i + 1); d > 0 {
			time.Sleep(d)
		}

		sideEffectsApplied, err := o.runForward(step, i+1, faults)
		o.logAttempt(saga.ID, fmt.Sprintf("F%d", i+1), err)
		if err != nil {
			slog.Error("saga step failed", "saga_id", saga.ID, "step", step.Name, "error", err)
			_ = o.sagaRepo.MarkStepFailed(rec.ID, err.Error())
			_ = o.sagaRepo.SetStatusWithError(saga.ID, models.SagaStatusRollingBack, err.Error())

			// A normal/"before" failure applied no side effects, so compensation
			// starts at the previous step (i-1). A forced "after" failure
			// committed the step's side effects, so its own compensator must run
			// too (start at i).
			from := i - 1
			if sideEffectsApplied {
				from = i
			}
			compErr := o.compensateBackward(saga.ID, steps, stepRecords, from, faults)
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

// runForward executes one forward action, honoring fault injection. It returns
// whether the step's side effects were applied before the failure (true only for
// a forced "after" failure) and the error, if any.
func (o *SagaOrchestrator) runForward(step SagaStep, stepNum int, faults *SagaFaultConfig) (bool, error) {
	if faults.shouldFailForward(stepNum) && !faults.forceFailAfter() {
		return false, fmt.Errorf("forced fault: forward F%d failed before side effects", stepNum)
	}
	if err := o.db.Transaction(func(tx *gorm.DB) error {
		return step.Forward(tx)
	}); err != nil {
		return false, err
	}
	if faults.shouldFailForward(stepNum) && faults.forceFailAfter() {
		return true, fmt.Errorf("forced fault: forward F%d failed after side effects", stepNum)
	}
	return false, nil
}

// compensateBackward runs Compensate for every step from index `from` down to 0,
// in reverse order. Each compensator is retried up to maxInlineCompensateAttempts
// times inline; one that still fails is left marked failed for the retry cron.
// Every attempt is recorded in the append-only log. Returns the first unrecovered
// compensation error, after attempting the rest.
func (o *SagaOrchestrator) compensateBackward(sagaID uint, steps []SagaStep, recs []*models.SagaStepRecord, from int, faults *SagaFaultConfig) error {
	var firstErr error
	for i := from; i >= 0; i-- {
		step := steps[i]
		rec := recs[i]
		if step.Compensate == nil {
			continue
		}
		phase := fmt.Sprintf("C%d", i+1)

		var lastErr error
		compensated := false
		for attempt := 1; attempt <= maxInlineCompensateAttempts; attempt++ {
			err := o.runCompensation(step, i+1, faults)
			o.logAttempt(sagaID, phase, err)
			if err != nil {
				lastErr = err
				slog.Error("saga compensation failed", "saga_id", sagaID, "step", step.Name, "attempt", attempt, "error", err)
				continue
			}
			_ = o.sagaRepo.MarkStepCompensated(rec.ID)
			compensated = true
			break
		}
		if !compensated {
			_ = o.sagaRepo.MarkStepFailed(rec.ID, "compensation: "+lastErr.Error())
			if firstErr == nil {
				firstErr = lastErr
			}
		}
	}
	return firstErr
}

// runCompensation executes one compensator, honoring fault injection. faults is
// nil outside the test suite, in which case it simply runs the compensator.
func (o *SagaOrchestrator) runCompensation(step SagaStep, stepNum int, faults *SagaFaultConfig) error {
	if faults.shouldFailCompensation(stepNum) {
		return fmt.Errorf("forced fault: compensation C%d failed", stepNum)
	}
	return o.db.Transaction(func(tx *gorm.DB) error {
		return step.Compensate(tx)
	})
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
		// SAGA-1: retry must operate on two states:
		//   - completed: never compensated yet (the original code's only branch)
		//   - failed with "compensation:" prefix: compensation already attempted
		//     and failed at least once. compensateBackward marks the step `failed`
		//     in that case, so without this branch retry skips every step and
		//     falsely flips the saga to rolled_back.
		isCompleted := rec.Status == models.SagaStepStatusCompleted
		isFailedCompensation := rec.Status == models.SagaStepStatusFailed &&
			(strings.HasPrefix(rec.ErrorMessage, "compensation:") || strings.HasPrefix(rec.ErrorMessage, "retry compensation:"))
		if !isCompleted && !isFailedCompensation {
			continue
		}
		if step.Compensate == nil {
			_ = o.sagaRepo.MarkStepCompensated(rec.ID)
			continue
		}
		err := o.runCompensation(step, i+1, nil)
		o.logAttempt(saga.ID, fmt.Sprintf("C%d", i+1), err)
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
