package models

import "time"

const (
	SagaTypeOtcExercise = "otc_exercise"

	SagaStatusInProgress                 = "in_progress"
	SagaStatusCompleted                  = "completed"
	SagaStatusFailed                     = "failed"
	SagaStatusRollingBack                = "rolling_back"
	SagaStatusRolledBack                 = "rolled_back"
	SagaStatusRequiresManualIntervention = "requires_manual_intervention"

	SagaStepStatusPending     = "pending"
	SagaStepStatusInProgress  = "in_progress"
	SagaStepStatusCompleted   = "completed"
	SagaStepStatusFailed      = "failed"
	SagaStepStatusCompensated = "compensated"
)

type SagaTransactionRecord struct {
	ID          uint              `gorm:"primaryKey"`
	Type        string            `gorm:"not null;index"`
	Status      string            `gorm:"not null;default:'in_progress';index"`
	CurrentStep int               `gorm:"column:current_step;not null;default:0"`
	Payload     string            `gorm:"type:text"`
	Error       string            `gorm:"type:text"`
	RetryCount  int               `gorm:"column:retry_count;not null;default:0"`
	CreatedAt   time.Time         `gorm:"not null"`
	UpdatedAt   time.Time         `gorm:"not null"`
	Steps       []SagaStepRecord  `gorm:"foreignKey:SagaID"`
}

func (SagaTransactionRecord) TableName() string { return "saga_transactions" }

type SagaStepRecord struct {
	ID           uint                  `gorm:"primaryKey"`
	SagaID       uint                  `gorm:"column:saga_id;not null;index"`
	Saga         SagaTransactionRecord `gorm:"foreignKey:SagaID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	StepNumber   int                   `gorm:"column:step_number;not null"`
	StepName     string                `gorm:"column:step_name;not null"`
	Status       string                `gorm:"not null;default:'pending'"`
	ExecutedAt   *time.Time            `gorm:"column:executed_at"`
	ErrorMessage string                `gorm:"column:error_message;type:text"`
	CreatedAt    time.Time             `gorm:"not null"`
	UpdatedAt    time.Time             `gorm:"not null"`
}

func (SagaStepRecord) TableName() string { return "saga_steps" }
