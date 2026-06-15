package service

import (
	"math"
	"time"

	"github.com/RAF-SI-2025/EXBanka-3-Backend/exchange-service/internal/models"
)

// minMonthlyReturnsForStats is the smallest number of month-over-month returns
// (i.e. monthly snapshots minus one) needed before risk/return metrics are
// shown. Below this the series is too short for a meaningful std-dev (§Celina 4
// "metrike imaju smisla tek kada postoji dovoljno istorijskih podataka").
const minMonthlyReturnsForStats = 2

// stdDevEpsilon is the floor below which a monthly-return std-dev is treated as
// zero, so floating-point noise on near-constant returns can't explode the
// reward-to-variability ratio.
const stdDevEpsilon = 1e-9

// FundStatistics holds the §Celina 4 risk/return metrics for a fund. All ratios
// are annualized from monthly returns. When Available is false the fund has too
// little history and the numeric fields should be ignored.
type FundStatistics struct {
	Available           bool    `json:"available"`
	MonthsOfData        int     `json:"monthsOfData"` // number of monthly returns used
	AnnualizedReturn    float64 `json:"annualizedReturn"`
	Volatility          float64 `json:"volatility"`
	RewardToVariability float64 `json:"rewardToVariability"`
	MaxDrawdown         float64 `json:"maxDrawdown"`
}

// ComputeStatistics derives the fund's metrics from its full daily snapshot
// history. Max drawdown uses the daily series; the annualized return,
// volatility and reward-to-variability use month-end values resampled from it.
func (s *FundService) ComputeStatistics(fundID uint) (FundStatistics, error) {
	snapshots, err := s.fundRepo.ListPerformance(fundID, time.Unix(0, 0).UTC(), time.Now().UTC())
	if err != nil {
		return FundStatistics{}, err
	}
	return computeFundStatistics(snapshots), nil
}

func computeFundStatistics(snapshots []models.FundPerformanceHistoryRecord) FundStatistics {
	stats := FundStatistics{}
	if len(snapshots) == 0 {
		return stats
	}

	// Max drawdown over the full daily series.
	stats.MaxDrawdown = round4(maxDrawdown(snapshots))

	monthly := monthEndValues(snapshots)
	returns := simpleReturns(monthly)
	stats.MonthsOfData = len(returns)
	if len(returns) < minMonthlyReturnsForStats {
		return stats // Available stays false
	}

	mean := meanOf(returns)
	std := stdDevSample(returns, mean)

	// Annualized return from the geometric mean of monthly returns.
	growth := 1.0
	for _, r := range returns {
		growth *= (1 + r)
	}
	annualized := math.Pow(growth, 12.0/float64(len(returns))) - 1

	stats.Available = true
	stats.AnnualizedReturn = round4(annualized)
	stats.Volatility = round4(std * math.Sqrt(12)) // annualized volatility
	// Guard against floating-point noise: near-constant returns yield a tiny
	// non-zero std that would otherwise blow the ratio up to ~1e15.
	if std > stdDevEpsilon {
		// Reward-to-variability (Sharpe, risk-free = 0), annualized.
		stats.RewardToVariability = round4((mean / std) * math.Sqrt(12))
	}
	return stats
}

// monthEndValues returns the last snapshot value of each calendar month, in
// chronological order. Snapshots are assumed already sorted ascending by date.
func monthEndValues(snapshots []models.FundPerformanceHistoryRecord) []float64 {
	type ym struct {
		y int
		m time.Month
	}
	order := make([]ym, 0)
	last := map[ym]float64{}
	for _, snap := range snapshots {
		key := ym{snap.Date.Year(), snap.Date.Month()}
		if _, seen := last[key]; !seen {
			order = append(order, key)
		}
		last[key] = snap.FundValue
	}
	out := make([]float64, 0, len(order))
	for _, k := range order {
		out = append(out, last[k])
	}
	return out
}

func simpleReturns(values []float64) []float64 {
	if len(values) < 2 {
		return nil
	}
	out := make([]float64, 0, len(values)-1)
	for i := 1; i < len(values); i++ {
		if values[i-1] == 0 {
			continue
		}
		out = append(out, values[i]/values[i-1]-1)
	}
	return out
}

// maxDrawdown returns the largest peak-to-trough decline as a positive fraction
// (e.g. 0.25 = a 25% drop from a prior peak).
func maxDrawdown(snapshots []models.FundPerformanceHistoryRecord) float64 {
	peak := math.Inf(-1)
	worst := 0.0
	for _, snap := range snapshots {
		v := snap.FundValue
		if v > peak {
			peak = v
		}
		if peak > 0 {
			dd := (peak - v) / peak
			if dd > worst {
				worst = dd
			}
		}
	}
	return worst
}

func meanOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func stdDevSample(xs []float64, mean float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	ss := 0.0
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(xs)-1))
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// BenchmarkPoint is one point of the cross-fund average performance index.
type BenchmarkPoint struct {
	Date       string  `json:"date"`
	IndexValue float64 `json:"indexValue"` // average of all funds, each rebased to 100 at its own start
}

// AverageFundBenchmark builds the §Celina 4 comparison curve: every fund's
// snapshot series is rebased to 100 at its own first snapshot, then averaged per
// date across all funds that have data on that date. Lets the detail page
// overlay one fund against the system-wide average.
func (s *FundService) AverageFundBenchmark() ([]BenchmarkPoint, error) {
	funds, err := s.fundRepo.ListFunds()
	if err != nil {
		return nil, err
	}
	sums := map[string]float64{}
	counts := map[string]int{}
	dateOrder := make([]string, 0)
	seen := map[string]bool{}

	for i := range funds {
		snaps, err := s.fundRepo.ListPerformance(funds[i].ID, time.Unix(0, 0).UTC(), time.Now().UTC())
		if err != nil {
			return nil, err
		}
		if len(snaps) == 0 || snaps[0].FundValue <= 0 {
			continue
		}
		base := snaps[0].FundValue
		for _, snap := range snaps {
			d := snap.Date.Format("2006-01-02")
			if !seen[d] {
				seen[d] = true
				dateOrder = append(dateOrder, d)
			}
			sums[d] += (snap.FundValue / base) * 100
			counts[d]++
		}
	}

	sortStrings(dateOrder)
	out := make([]BenchmarkPoint, 0, len(dateOrder))
	for _, d := range dateOrder {
		if counts[d] == 0 {
			continue
		}
		out = append(out, BenchmarkPoint{Date: d, IndexValue: round2RSD(sums[d] / float64(counts[d]))})
	}
	return out, nil
}

// sortStrings sorts date strings ascending (YYYY-MM-DD sorts lexicographically).
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j-1] > xs[j]; j-- {
			xs[j-1], xs[j] = xs[j], xs[j-1]
		}
	}
}
