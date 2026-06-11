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

	// Outcomes recorded in the append-only attempt log (SagaAttemptRecord).
	SagaAttemptOutcomeOK  = "ok"
	SagaAttemptOutcomeErr = "err"
)

type SagaTransactionRecord struct {
	ID          uint             `gorm:"primaryKey"`
	Type        string           `gorm:"not null;index"`
	Status      string           `gorm:"not null;default:'in_progress';index"`
	CurrentStep int              `gorm:"column:current_step;not null;default:0"`
	Payload     string           `gorm:"type:text"`
	Error       string           `gorm:"type:text"`
	RetryCount  int              `gorm:"column:retry_count;not null;default:0"`
	CreatedAt   time.Time        `gorm:"not null"`
	UpdatedAt   time.Time        `gorm:"not null"`
	Steps       []SagaStepRecord `gorm:"foreignKey:SagaID"`
	// Attempts is the append-only log: one row per forward/compensation attempt,
	// in execution order. See SagaAttemptRecord.
	Attempts []SagaAttemptRecord `gorm:"foreignKey:SagaID"`
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

// SagaAttemptRecord is one entry in the saga's append-only log: a single forward
// or compensation attempt and its outcome. Unlike SagaStepRecord (one mutable
// row per step), this table grows by one row per attempt, so a compensator that
// fails then succeeds appears as two rows ({C2: err} then {C2: ok}). This is the
// "log" the SAGA spec asserts on (files/SAGA.md). Ordered by ID (insertion).
type SagaAttemptRecord struct {
	ID        uint      `gorm:"primaryKey"`
	SagaID    uint      `gorm:"column:saga_id;not null;index"`
	Phase     string    `gorm:"column:phase;not null"`   // "F1".."F5" (forward) or "C1".."C5" (compensation)
	Outcome   string    `gorm:"column:outcome;not null"` // "ok" | "err"
	Error     string    `gorm:"column:error;type:text"`
	CreatedAt time.Time `gorm:"not null"`
}

func (SagaAttemptRecord) TableName() string { return "saga_step_attempts" }
