// Package charter — Phase C4 A/B delta conversion.
//
// This file converts one C3 step report (cmd/experimental/charter-ab
// step-<n>.json) into a CharterDelta — the writeback unit of the evolution
// loop. Each delta carries the switch, window, metric deltas, paired t-test
// and BCa Sharpe CI, plus a Verdict that decides how the delta is recorded
// into the baseline policy (internal/baseline writeback.go):
//
//	significant_enable — p<0.05 and daily-return improvement → may become a
//	                     runtime constraint (Constraints/ExecutionPolicy)
//	directional_watch  — p>=0.05 but multiple directional indicators improve
//	                     → recorded as a watch entry, never enforced
//	inert              — the arm changed nothing (identical runs)
//	degenerate         — BCa CIs cannot support inference (path-dependent
//	                     stats like MaxDrawdown) → recorded as a finding
//
// Maturity: experimental (Phase C harness).
package charter

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Verdict classifies one charter A/B step for baseline writeback.
type Verdict string

const (
	// VerdictSignificantEnable — paired t-test p<0.05 and the feature arm
	// improves daily returns (mean diff > 0).
	VerdictSignificantEnable Verdict = "significant_enable"
	// VerdictDirectionalWatch — p>=0.05 but at least two of Sharpe /
	// MaxDrawdown / WinRate move in the improving direction. Recorded as a
	// watch entry; runtime values are NOT overridden.
	VerdictDirectionalWatch Verdict = "directional_watch"
	// VerdictInert — the arm changed nothing (all metric deltas ≈ 0).
	VerdictInert Verdict = "inert"
	// VerdictDegenerate — the BCa bootstrap CIs cannot support inference, so
	// no significance claim is possible. Recorded as a finding.
	VerdictDegenerate Verdict = "degenerate"
)

// DeltaWindow is the A/B window a delta was measured on.
type DeltaWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Days  int    `json:"days"`
}

// MetricDiffs are the feature-minus-baseline metric deltas of one step.
// MaxDD is baseline-minus-feature (positive = the feature arm has a shallower
// drawdown, i.e. better).
type MetricDiffs struct {
	Sharpe      float64 `json:"sharpe_diff"`
	MaxDD       float64 `json:"max_drawdown_diff"`
	WinRate     float64 `json:"win_rate_diff"`
	TotalReturn float64 `json:"total_return_diff"`
	RawRecs     int     `json:"raw_recs_diff"`
}

// PairedT summarizes the paired t-test on daily return differences
// (feature − baseline).
type PairedT struct {
	T           float64 `json:"t"`
	DF          int     `json:"df"`
	P           float64 `json:"p"`
	MeanDiff    float64 `json:"mean_daily_return_diff"`
	Significant bool    `json:"significant"` // p < 0.05
}

// BCaSharpeCI summarizes the Sharpe-difference BCa bootstrap 95% CI.
type BCaSharpeCI struct {
	Observed    float64 `json:"observed_diff"`
	CI95Low     float64 `json:"ci95_low"`
	CI95High    float64 `json:"ci95_high"`
	Resamples   int     `json:"resamples"`
	Significant bool    `json:"significant"` // CI excludes 0 (and not degenerate)
	Degenerate  bool    `json:"degenerate"`  // CI does not bracket the observed diff
}

// CharterDelta is the writeback unit of the evolution loop (Phase C4): one
// charter A/B step converted from a C3 step report into a verdict + evidence
// suitable for baseline policy writeback.
type CharterDelta struct {
	Step          int         `json:"step"`
	Switch        string      `json:"switch"`
	Window        DeltaWindow `json:"window"`
	MetricDiffs   MetricDiffs `json:"metric_diffs"`
	PairedT       PairedT     `json:"paired_t"`
	BCaSharpe95CI BCaSharpeCI `json:"bca_sharpe_95_ci"`
	// MaxDDDegenerate records whether the MaxDrawdown BCa CI was degenerate
	// (path-dependent statistic; resampling destroys the temporal ordering).
	MaxDDDegenerate bool    `json:"max_drawdown_bca_degenerate"`
	Verdict         Verdict `json:"verdict"`
	Evidence        string  `json:"evidence"`
}

// ExperimentID is the stable promotion key for a delta. The baseline
// writeback uses it for idempotency (re-running a writeback must not
// duplicate records).
func (d CharterDelta) ExperimentID() string {
	return fmt.Sprintf("charter-ab-step-%d", d.Step)
}

// Classify decides the writeback verdict for the delta:
//
//	a. significant_enable — p<0.05 and the feature arm improves daily returns.
//	b. directional_watch  — p>=0.05 but at least two of Sharpe / MaxDrawdown /
//	   WinRate move in the improving direction (e.g. +StrategyFilter: Sharpe
//	   up, MaxDD down, WinRate up).
//	c. inert              — the arm changed nothing (all deltas ≈ 0; identical
//	   runs, e.g. +MacroFlow / +ConvictionFloor in a consolidation window).
//	d. degenerate         — remaining cases whose MaxDrawdown BCa CI cannot
//	   support inference (e.g. +PeriodOnly / +CashReserve).
func (d CharterDelta) Classify() Verdict {
	if d.PairedT.Significant && d.PairedT.MeanDiff > 0 {
		return VerdictSignificantEnable
	}
	improving := 0
	if d.MetricDiffs.Sharpe > 0 {
		improving++
	}
	if d.MetricDiffs.MaxDD > 0 {
		improving++
	}
	if d.MetricDiffs.WinRate > 0 {
		improving++
	}
	if improving >= 2 {
		return VerdictDirectionalWatch
	}
	if d.isZeroEffect() {
		return VerdictInert
	}
	if d.MaxDDDegenerate {
		return VerdictDegenerate
	}
	return VerdictInert
}

func (d CharterDelta) isZeroEffect() bool {
	const eps = 1e-9
	return math.Abs(d.MetricDiffs.Sharpe) < eps &&
		math.Abs(d.MetricDiffs.MaxDD) < eps &&
		math.Abs(d.MetricDiffs.WinRate) < eps &&
		math.Abs(d.MetricDiffs.TotalReturn) < eps &&
		d.MetricDiffs.RawRecs == 0
}

// EvidenceString renders the delta's full evidence: window, paired t-test
// (t, p, mean diff), Sharpe diff + BCa 95% CI, MaxDD diff + degeneracy,
// win-rate diff, total-return diff and raw-recommendation delta.
func (d CharterDelta) EvidenceString() string {
	return fmt.Sprintf(
		"step=%d switch=%s window=%s..%s (%d days) verdict=%s; "+
			"paired_t(t=%.4f, p=%.4g, mean_daily_return_diff=%.6f, significant=%t); "+
			"sharpe_diff=%.4f (BCa 95%% CI [%.4f, %.4f], significant=%t, degenerate=%t); "+
			"max_drawdown_diff=%.4f (BCa degenerate=%t); win_rate_diff=%.4f; "+
			"total_return_diff=%.4f; raw_recs_diff=%d",
		d.Step, d.Switch, d.Window.Start, d.Window.End, d.Window.Days, d.Verdict,
		d.PairedT.T, d.PairedT.P, d.PairedT.MeanDiff, d.PairedT.Significant,
		d.MetricDiffs.Sharpe, d.BCaSharpe95CI.CI95Low, d.BCaSharpe95CI.CI95High,
		d.BCaSharpe95CI.Significant, d.BCaSharpe95CI.Degenerate,
		d.MetricDiffs.MaxDD, d.MaxDDDegenerate,
		d.MetricDiffs.WinRate, d.MetricDiffs.TotalReturn, d.MetricDiffs.RawRecs,
	)
}

// LoadDeltasFromReports reads step-{1..steps}.json C3 step reports from dir
// and converts each into a CharterDelta, in step order. Missing or unparsable
// files return an error: the writeback must not silently drop evidence.
func LoadDeltasFromReports(dir string, steps int) ([]CharterDelta, error) {
	deltas := make([]CharterDelta, 0, steps)
	for step := 1; step <= steps; step++ {
		path := filepath.Join(dir, fmt.Sprintf("step-%d.json", step))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read step report %s: %w", path, err)
		}
		d, err := ParseDeltaReport(data)
		if err != nil {
			return nil, fmt.Errorf("parse step report %s: %w", path, err)
		}
		deltas = append(deltas, d)
	}
	return deltas, nil
}

// ParseDeltaReport converts one C3 step report (JSON bytes) into a
// CharterDelta with the verdict classified and evidence rendered.
func ParseDeltaReport(data []byte) (CharterDelta, error) {
	var rep stepReportDelta
	if err := json.Unmarshal(data, &rep); err != nil {
		return CharterDelta{}, fmt.Errorf("unmarshal step report: %w", err)
	}
	// Strict validation: a truncated / mismatched report must not silently
	// produce a bogus delta (e.g. an "inert" verdict from missing stats).
	if len(rep.FeatureSwitches) == 0 {
		return CharterDelta{}, fmt.Errorf("step report missing feature_switches")
	}
	if rep.Window.Start == "" || rep.Window.End == "" || rep.Window.Days == 0 {
		return CharterDelta{}, fmt.Errorf("step report missing window")
	}
	if rep.Stats.PairedT.DF == 0 && rep.Stats.PairedT.T == 0 {
		return CharterDelta{}, fmt.Errorf("step report missing paired t-test stats")
	}
	if rep.Stats.Sharpe.Resamples == 0 && rep.Stats.MaxDD.Resamples == 0 {
		return CharterDelta{}, fmt.Errorf("step report missing BCa bootstrap stats")
	}
	return deltaFromReport(rep), nil
}

// ─── report mirror ────────────────────────────────────────────────────────

// stepReportDelta mirrors the subset of the C3 step report JSON (written by
// cmd/experimental/charter-ab) that the delta conversion needs. It lives in
// this package (not in cmd) so internal/baseline can consume the writeback
// unit without importing the command package.
type stepReportDelta struct {
	Step            int             `json:"step"`
	FeatureSwitches []string        `json:"feature_switches"`
	Window          stepDeltaWindow `json:"window"`
	Baseline        stepRunMetrics  `json:"baseline"`
	Feature         stepRunMetrics  `json:"feature"`
	Stats           struct {
		PairedT stepTTest `json:"paired_t_test_daily_return_diff"`
		Sharpe  stepBCa   `json:"sharpe_diff_bca_95"`
		MaxDD   stepBCa   `json:"max_drawdown_diff_bca_95"`
	} `json:"stats"`
	Attribution struct {
		RawRecsBaseline int `json:"raw_recs_baseline"`
		RawRecsFeature  int `json:"raw_recs_feature"`
	} `json:"attribution"`
}

type stepDeltaWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Days  int    `json:"days"`
}

type stepRunMetrics struct {
	WinRate     float64 `json:"win_rate"`
	TotalReturn float64 `json:"total_return"`
	Sharpe      float64 `json:"sharpe"`
	MaxDrawdown float64 `json:"max_drawdown"`
}

type stepTTest struct {
	T           float64 `json:"t"`
	DF          int     `json:"df"`
	P           float64 `json:"p"`
	MeanDiff    float64 `json:"mean_daily_return_diff"`
	Significant bool    `json:"significant"`
}

type stepBCa struct {
	Observed    float64 `json:"observed_diff"`
	CI95Low     float64 `json:"ci95_low"`
	CI95High    float64 `json:"ci95_high"`
	Resamples   int     `json:"resamples"`
	Significant bool    `json:"significant"`
	Degenerate  bool    `json:"degenerate"`
}

func deltaFromReport(rep stepReportDelta) CharterDelta {
	switchName := ""
	if len(rep.FeatureSwitches) > 0 {
		switchName = rep.FeatureSwitches[len(rep.FeatureSwitches)-1]
	}
	d := CharterDelta{
		Step:   rep.Step,
		Switch: switchName,
		Window: DeltaWindow{Start: rep.Window.Start, End: rep.Window.End, Days: rep.Window.Days},
		MetricDiffs: MetricDiffs{
			Sharpe:      rep.Feature.Sharpe - rep.Baseline.Sharpe,
			MaxDD:       rep.Baseline.MaxDrawdown - rep.Feature.MaxDrawdown,
			WinRate:     rep.Feature.WinRate - rep.Baseline.WinRate,
			TotalReturn: rep.Feature.TotalReturn - rep.Baseline.TotalReturn,
			RawRecs:     rep.Attribution.RawRecsFeature - rep.Attribution.RawRecsBaseline,
		},
		PairedT: PairedT{
			T:           rep.Stats.PairedT.T,
			DF:          rep.Stats.PairedT.DF,
			P:           rep.Stats.PairedT.P,
			MeanDiff:    rep.Stats.PairedT.MeanDiff,
			Significant: rep.Stats.PairedT.Significant,
		},
		BCaSharpe95CI: BCaSharpeCI{
			Observed:    rep.Stats.Sharpe.Observed,
			CI95Low:     rep.Stats.Sharpe.CI95Low,
			CI95High:    rep.Stats.Sharpe.CI95High,
			Resamples:   rep.Stats.Sharpe.Resamples,
			Significant: rep.Stats.Sharpe.Significant,
			Degenerate:  rep.Stats.Sharpe.Degenerate,
		},
		MaxDDDegenerate: rep.Stats.MaxDD.Degenerate,
	}
	d.Verdict = d.Classify()
	d.Evidence = d.EvidenceString()
	return d
}
