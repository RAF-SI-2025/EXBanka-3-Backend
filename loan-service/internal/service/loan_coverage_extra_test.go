package service_test

import (
	"testing"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/notify"
	"github.com/RAF-SI-2025/EXBanka-3-Backend/loan-service/internal/service"
)

func TestBaseInterestRate_Varijabilna_AllBands(t *testing.T) {
	cases := []struct {
		amount float64
		want   float64
	}{
		{50_000, 4.5},
		{250_000, 3.8},
		{750_000, 3.2},
		{3_000_000, 2.5},
		{6_000_000, 2.0},
	}
	for _, c := range cases {
		if got := service.BaseInterestRate(c.amount, "varijabilna"); got != c.want {
			t.Errorf("BaseInterestRate(%.0f, varijabilna)=%v, want %v", c.amount, got, c.want)
		}
	}
}

func TestWithAppNotifier_Chains(t *testing.T) {
	svc := service.NewLoanService(nil, nil, nil, nil)
	if svc.WithAppNotifier(notify.NewClient("", "")) != svc {
		t.Error("WithAppNotifier should return the same service for chaining")
	}
}
