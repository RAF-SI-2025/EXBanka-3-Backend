package service

import (
	"math"
	"testing"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

func snap(y int, m time.Month, d int, v float64) models.FundPerformanceHistoryRecord {
	return models.FundPerformanceHistoryRecord{Date: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), FundValue: v}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestComputeFundStatistics_InsufficientData(t *testing.T) {
	// One monthly point → zero returns → not available.
	st := computeFundStatistics([]models.FundPerformanceHistoryRecord{snap(2026, 1, 31, 100)})
	if st.Available {
		t.Errorf("expected not available with a single snapshot, got %+v", st)
	}
	if st.MonthsOfData != 0 {
		t.Errorf("expected 0 monthly returns, got %d", st.MonthsOfData)
	}
}

func TestComputeFundStatistics_SteadyGrowth(t *testing.T) {
	// +10% each month for 3 months → 3 monthly returns of 0.10.
	snaps := []models.FundPerformanceHistoryRecord{
		snap(2026, 1, 31, 100),
		snap(2026, 2, 28, 110),
		snap(2026, 3, 31, 121),
		snap(2026, 4, 30, 133.1),
	}
	st := computeFundStatistics(snaps)
	if !st.Available || st.MonthsOfData != 3 {
		t.Fatalf("expected available with 3 returns, got %+v", st)
	}
	// Geometric annualized: (1.1^3)^(12/3) - 1.
	want := math.Pow(math.Pow(1.1, 3), 4) - 1
	if !approx(st.AnnualizedReturn, round4(want)) {
		t.Errorf("annualizedReturn=%v, want ~%v", st.AnnualizedReturn, round4(want))
	}
	// Constant returns → zero volatility and zero reward-to-variability.
	if st.Volatility != 0 {
		t.Errorf("expected 0 volatility for constant returns, got %v", st.Volatility)
	}
	if st.RewardToVariability != 0 {
		t.Errorf("expected 0 reward-to-variability when std=0, got %v", st.RewardToVariability)
	}
	if st.MaxDrawdown != 0 {
		t.Errorf("expected 0 drawdown on monotonic growth, got %v", st.MaxDrawdown)
	}
}

func TestComputeFundStatistics_MaxDrawdownAndVolatility(t *testing.T) {
	// 100 -> 110 -> 90 -> 120. Peak 110, trough 90 => drawdown 20/110.
	snaps := []models.FundPerformanceHistoryRecord{
		snap(2026, 1, 31, 100),
		snap(2026, 2, 28, 110),
		snap(2026, 3, 31, 90),
		snap(2026, 4, 30, 120),
	}
	st := computeFundStatistics(snaps)
	if !st.Available {
		t.Fatalf("expected available, got %+v", st)
	}
	wantDD := round4(20.0 / 110.0)
	if !approx(st.MaxDrawdown, wantDD) {
		t.Errorf("maxDrawdown=%v, want ~%v", st.MaxDrawdown, wantDD)
	}
	if st.Volatility <= 0 {
		t.Errorf("expected positive volatility for varying returns, got %v", st.Volatility)
	}
}

func TestMonthEndValues_TakesLastPerMonth(t *testing.T) {
	snaps := []models.FundPerformanceHistoryRecord{
		snap(2026, 1, 10, 100),
		snap(2026, 1, 20, 105), // later January wins
		snap(2026, 2, 5, 110),
	}
	got := monthEndValues(snaps)
	if len(got) != 2 || got[0] != 105 || got[1] != 110 {
		t.Errorf("expected [105 110], got %v", got)
	}
}
