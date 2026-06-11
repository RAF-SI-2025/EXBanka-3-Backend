package repository

import (
	"errors"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
	"gorm.io/gorm"
)

type SagaRepository struct {
	db *gorm.DB
}

func NewSagaRepository(db *gorm.DB) *SagaRepository {
	return &SagaRepository{db: db}
}

func (r *SagaRepository) DB() *gorm.DB { return r.db }

func (r *SagaRepository) CreateTransaction(saga *models.SagaTransactionRecord) error {
	now := time.Now().UTC()
	if saga.Status == "" {
		saga.Status = models.SagaStatusInProgress
	}
	saga.CreatedAt = now
	saga.UpdatedAt = now
	return r.db.Create(saga).Error
}

func (r *SagaRepository) AppendStep(sagaID uint, stepNumber int, name string) (*models.SagaStepRecord, error) {
	now := time.Now().UTC()
	step := &models.SagaStepRecord{
		SagaID:     sagaID,
		StepNumber: stepNumber,
		StepName:   name,
		Status:     models.SagaStepStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := r.db.Create(step).Error; err != nil {
		return nil, err
	}
	return step, nil
}

// AppendAttempt adds one entry to the saga's append-only log (files/SAGA.md).
// Phase is "F1".."F5" or "C1".."C5"; outcome is models.SagaAttemptOutcome*.
func (r *SagaRepository) AppendAttempt(sagaID uint, phase, outcome, errMsg string) error {
	return r.db.Create(&models.SagaAttemptRecord{
		SagaID:    sagaID,
		Phase:     phase,
		Outcome:   outcome,
		Error:     errMsg,
		CreatedAt: time.Now().UTC(),
	}).Error
}

func (r *SagaRepository) UpdateStep(stepID uint, fields map[string]interface{}) error {
	fields["updated_at"] = time.Now().UTC()
	return r.db.Model(&models.SagaStepRecord{}).Where("id = ?", stepID).Updates(fields).Error
}

func (r *SagaRepository) MarkStepInProgress(stepID uint) error {
	return r.UpdateStep(stepID, map[string]interface{}{
		"status": models.SagaStepStatusInProgress,
	})
}

func (r *SagaRepository) MarkStepCompleted(stepID uint) error {
	now := time.Now().UTC()
	return r.UpdateStep(stepID, map[string]interface{}{
		"status":      models.SagaStepStatusCompleted,
		"executed_at": now,
	})
}

func (r *SagaRepository) MarkStepFailed(stepID uint, errMsg string) error {
	return r.UpdateStep(stepID, map[string]interface{}{
		"status":        models.SagaStepStatusFailed,
		"error_message": errMsg,
	})
}

func (r *SagaRepository) MarkStepCompensated(stepID uint) error {
	return r.UpdateStep(stepID, map[string]interface{}{
		"status": models.SagaStepStatusCompensated,
	})
}

func (r *SagaRepository) UpdateTransaction(sagaID uint, fields map[string]interface{}) error {
	fields["updated_at"] = time.Now().UTC()
	return r.db.Model(&models.SagaTransactionRecord{}).Where("id = ?", sagaID).Updates(fields).Error
}

func (r *SagaRepository) SetCurrentStep(sagaID uint, current int) error {
	return r.UpdateTransaction(sagaID, map[string]interface{}{"current_step": current})
}

func (r *SagaRepository) SetStatus(sagaID uint, status string) error {
	return r.UpdateTransaction(sagaID, map[string]interface{}{"status": status})
}

func (r *SagaRepository) SetStatusWithError(sagaID uint, status, errMsg string) error {
	return r.UpdateTransaction(sagaID, map[string]interface{}{
		"status": status,
		"error":  errMsg,
	})
}

func (r *SagaRepository) IncrementRetry(sagaID uint) error {
	return r.db.Model(&models.SagaTransactionRecord{}).
		Where("id = ?", sagaID).
		Updates(map[string]interface{}{
			"retry_count": gorm.Expr("retry_count + 1"),
			"updated_at":  time.Now().UTC(),
		}).Error
}

func (r *SagaRepository) GetTransaction(sagaID uint) (*models.SagaTransactionRecord, error) {
	var saga models.SagaTransactionRecord
	if err := r.db.
		Preload("Steps", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("step_number ASC, id ASC")
		}).
		Preload("Attempts", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("id ASC")
		}).
		First(&saga, sagaID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &saga, nil
}

func (r *SagaRepository) ListSteps(sagaID uint) ([]models.SagaStepRecord, error) {
	var steps []models.SagaStepRecord
	if err := r.db.Where("saga_id = ?", sagaID).
		Order("step_number ASC, id ASC").
		Find(&steps).Error; err != nil {
		return nil, err
	}
	return steps, nil
}

func (r *SagaRepository) ListStuckRollingBack(olderThan time.Time, maxRetries int) ([]models.SagaTransactionRecord, error) {
	var sagas []models.SagaTransactionRecord
	if err := r.db.Preload("Steps", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("step_number ASC, id ASC")
	}).
		Where("status = ? AND updated_at < ? AND retry_count < ?",
			models.SagaStatusRollingBack, olderThan, maxRetries).
		Order("updated_at ASC, id ASC").
		Find(&sagas).Error; err != nil {
		return nil, err
	}
	return sagas, nil
}
