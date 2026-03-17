package models

// AccountFilter holds optional filter/pagination parameters for account queries.
type AccountFilter struct {
	ClientName string
	Tip        string
	Vrsta      string
	Status     string
	CurrencyID *uint
	Page       int
	PageSize   int
}
