package live

import (
	"context"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

// BenchmarkComparisonResponse is the response for GET /api/dashboard/benchmark-comparison.
// Optional metrics use *float64 so missing/insufficient data serializes as null.
type BenchmarkComparisonResponse struct {
	SnapshotTime    time.Time        `json:"snapshot_time"`
	SessionCount    int              `json:"session_count"`
	PortfolioReturn float64          `json:"portfolio_return"`
	TAIEXReturn     *float64         `json:"taiex_return,omitempty"`
	Outperformance  *float64         `json:"outperformance,omitempty"`
	Alpha           *float64         `json:"alpha,omitempty"`
	Beta            *float64         `json:"beta,omitempty"`
	TrackingError   *float64         `json:"tracking_error,omitempty"`
	SharpeRatio     *float64         `json:"sharpe_ratio,omitempty"`
	InfoRatio       *float64         `json:"info_ratio,omitempty"`
	EquityCurve     []BenchmarkPoint `json:"equity_curve"`
	// Source / Degraded (SSOT P1-3) label the L-cold portfolio history.
	Source   string `json:"source,omitempty"`
	Degraded bool   `json:"degraded,omitempty"`
}

// BenchmarkPoint is a single point on the benchmark comparison equity curve.
type BenchmarkPoint struct {
	Label     string  `json:"label"`
	Portfolio float64 `json:"portfolio"`
	Benchmark float64 `json:"benchmark"`
	Outperf   float64 `json:"outperf"`
}

type sessionPoint struct {
	date  time.Time
	value float64
	name  string
}

// HandleBenchmarkComparison returns portfolio vs TAIEX benchmark comparison metrics.
func (h *Handlers) HandleBenchmarkComparison(r *http.Request) (int, any) {
	// SSOT (P1-1/P1-2): portfolio history comes from the shared
	// SessionHistoryProvider (PG-first on production). The old scan read
	// LedgerDir/sessions/*/summary.json directly — an import source only on
	// production — so the comparison curve silently diverged from the
	// performance report's PG equity curve.
	svc := h.getService()
	history := svc.HistoryPoints()

	var points []sessionPoint
	for _, p := range history {
		points = append(points, sessionPoint{
			name:  p.SessionID,
			date:  p.Date,
			value: p.PortfolioValue,
		})
	}

	if len(points) == 0 {
		return http.StatusOK, BenchmarkComparisonResponse{
			SnapshotTime: time.Now(),
			Source:       svc.HistorySource(),
			Degraded:     svc.HistoryDegraded(),
		}
	}

	slices.SortFunc(points, func(a, b sessionPoint) int {
		return a.date.Compare(b.date)
	})

	firstValue := points[0].value
	lastValue := points[len(points)-1].value
	var portfolioReturn float64
	if firstValue > 0 {
		portfolioReturn = (lastValue - firstValue) / firstValue
	}

	dailyReturns := make([]float64, 0, max(0, len(points)-1))
	for i := 1; i < len(points); i++ {
		if points[i-1].value > 0 {
			dailyReturns = append(dailyReturns, (points[i].value-points[i-1].value)/points[i-1].value)
		}
	}

	var taiexReturn *float64
	calc := h.getTAIEXCalculator()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if ret, err := calc.Get1MonthReturn(ctx); err == nil {
		taiexReturn = &ret
	} else {
		logging.Warn("benchmark", "taiex_return_fetch_failed", logging.Err(err))
	}

	var outperformance *float64
	if taiexReturn != nil {
		v := portfolioReturn - *taiexReturn
		outperformance = &v
	}

	beta := reporting.CalculateBeta(dailyReturns, taiexReturn)
	alpha := reporting.CalculateAlpha(portfolioReturn, beta, taiexReturn)
	trackingError := reporting.CalculateTrackingError(dailyReturns)
	sharpeRatio := reporting.CalculateSharpeRatio(dailyReturns)
	infoRatio := reporting.CalculateInfoRatio(outperformance, trackingError)

	equityCurve := buildBenchmarkEquityCurve(points, taiexReturn)

	return http.StatusOK, BenchmarkComparisonResponse{
		SnapshotTime:    time.Now(),
		SessionCount:    len(points),
		PortfolioReturn: portfolioReturn,
		TAIEXReturn:     taiexReturn,
		Outperformance:  outperformance,
		Alpha:           alpha,
		Beta:            beta,
		TrackingError:   trackingError,
		SharpeRatio:     sharpeRatio,
		InfoRatio:       infoRatio,
		EquityCurve:     equityCurve,
		Source:          svc.HistorySource(),
		Degraded:        svc.HistoryDegraded(),
	}
}

// getTAIEXCalculator returns the configured calculator or creates a default one.
func (h *Handlers) getTAIEXCalculator() *marketdata.TAIEXReturnCalculator {
	return marketdata.NewTAIEXReturnCalculator()
}

// buildBenchmarkEquityCurve constructs normalized equity curves for portfolio and benchmark.
// Portfolio is normalized starting at 100, benchmark grows at a constant rate.
func buildBenchmarkEquityCurve(points []sessionPoint, taiexReturn *float64) []BenchmarkPoint {
	if len(points) == 0 {
		return nil
	}

	baseValue := points[0].value
	if baseValue <= 0 {
		return nil
	}

	curve := make([]BenchmarkPoint, len(points))
	n := float64(len(points) - 1)
	if n < 1 {
		n = 1
	}

	// When TAIEX return is unavailable, render a flat benchmark at 100.
	benchRet := 0.0
	if taiexReturn != nil {
		benchRet = *taiexReturn
	}
	dailyBenchRet := math.Pow(1+benchRet, 1.0/n) - 1

	benchVal := 100.0
	for i, p := range points {
		portVal := 100.0
		if baseValue > 0 {
			portVal = (p.value / baseValue) * 100.0
		}
		outperf := portVal - benchVal

		label := p.name
		if !p.date.IsZero() {
			label = p.date.Format("01/02")
		}

		curve[i] = BenchmarkPoint{
			Label:     label,
			Portfolio: math.Round(portVal*100) / 100,
			Benchmark: math.Round(benchVal*100) / 100,
			Outperf:   math.Round(outperf*100) / 100,
		}

		benchVal *= (1 + dailyBenchRet)
	}

	return curve
}
