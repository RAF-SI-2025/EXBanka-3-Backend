package models

import "time"

type TransferFilter struct {
	DateFrom  *time.Time
	DateTo    *time.Time
	MinAmount *float64
	MaxAmount *float64
	Status    string
	Page      int
	PageSize  int
}
