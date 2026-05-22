package live

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

// BenchmarkComparisonResponse is the response for GET /api/dashboard/benchmark-comparison.
type BenchmarkComparisonResponse struct {
	SnapshotTime    time.Time        `json:"snapshot_time"`
	SessionCount    int              `json:"session_count"`
	PortfolioReturn float64          `json:"portfolio_return"`
	TAIEXReturn     float64          `json:"taiex_return"`
	Outperformance  float64          `json:"outperformance"`
	Alpha           float64          `json:"alpha"`
	Beta            float64          `json:"beta"`
	TrackingError   float64          `json:"tracking_error"`
	SharpeRatio     float64          `json:"sharpe_ratio"`
	InfoRatio       float64          `json:"info_ratio"`
	EquityCurve     []BenchmarkPoint `json:"equity_curve"`
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
	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": "read sessions dir: " + err.Error()}
	}

	var points []sessionPoint
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			logging.Warn("benchmark", "corrupted_summary_skipped", logging.Err(err))
			continue
		}
		if summary.PortfolioValue == 0 {
			continue
		}
		date := domain.SessionDateFromID(summary.SessionID)
		points = append(points, sessionPoint{
			name:  summary.SessionID,
			date:  date,
			value: summary.PortfolioValue,
		})
	}

	if len(points) == 0 {
		return http.StatusOK, BenchmarkComparisonResponse{
			SnapshotTime: time.Now(),
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

	dailyReturns := make([]float64, 0, len(points)-1)
	for i := 1; i < len(points); i++ {
		if points[i-1].value > 0 {
			dailyReturns = append(dailyReturns, (points[i].value-points[i-1].value)/points[i-1].value)
		}
	}

	var taiexReturn float64
	calc := h.getTAIEXCalculator()
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if ret, err := calc.Get1MonthReturn(ctx); err == nil {
		taiexReturn = ret
	} else {
		logging.Warn("benchmark", "taiex_return_fetch_failed", logging.Err(err))
	}

	outperformance := portfolioReturn - taiexReturn

	beta := reporting.CalculateBeta(dailyReturns, taiexReturn)
	alpha := reporting.CalculateAlpha(portfolioReturn, beta, taiexReturn)
	trackingError := reporting.CalculateTrackingError(dailyReturns)

	var sharpeRatio float64
	if len(dailyReturns) > 0 {
		sharpeRatio = reporting.CalculateSharpeRatio(dailyReturns)
	}

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
	}
}

// getTAIEXCalculator returns the configured calculator or creates a default one.
func (h *Handlers) getTAIEXCalculator() *marketdata.TAIEXReturnCalculator {
	return marketdata.NewTAIEXReturnCalculator()
}

// buildBenchmarkEquityCurve constructs normalized equity curves for portfolio and benchmark.
// Portfolio is normalized starting at 100, benchmark grows at a constant rate.
func buildBenchmarkEquityCurve(points []sessionPoint, taiexReturn float64) []BenchmarkPoint {
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
	dailyBenchRet := math.Pow(1+taiexReturn, 1.0/n) - 1

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
