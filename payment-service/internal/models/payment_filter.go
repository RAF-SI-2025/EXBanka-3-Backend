package models

import "time"

type PaymentFilter struct {
	DateFrom  *time.Time
	DateTo    *time.Time
	MinAmount *float64
	MaxAmount *float64
	Status    string
	Page      int
	PageSize  int
}
