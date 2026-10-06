package stocktools

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// ─── family-expectancy (read-only instrument) ──────────────────────────────
//
// GET /api/stock/family-expectancy exposes the FAMILY-level view of the
// persisted raw signal outcomes (stock_signal_outcomes): one row per source,
// with the canonical cost-aware caliber shared with
// /api/stock/win_rate, /api/stock/condition_winrate and
// /api/stock/industry_winrate (hit = forward_return - cost_rate > 0, Wilson
// 95% CI, min_samples gate) plus a net-expectancy t statistic, per-regime
// strata and a trailing 60-trading-day sub-window.
//
// It answers "which signal family is still alive?" in one call. Read-only by
// construction (the ledger handle is opened with mode=ro) and additive: it
// aggregates the persisted rows on the fly and changes no existing number.

// FamilyExpectancyProvider is the minimal read-only interface the
// /api/stock/family-expectancy handler depends on: one full-family read.
// Kept separate from WinRateProvider / ConditionWinRateProvider /
// IndustryWinRateProvider so the existing fakes stay valid.
type FamilyExpectancyProvider interface {
	// LoadFamilyExpectancy returns one row per signal family (source) over
	// every stored outcome (no rolling-window cutoff — the recent view is
	// the trailing sub-window). trailingDays is the trailing window length
	// in distinct trigger dates; generatedAt is stamped into the report (the
	// handler owns the clock).
	//
	// found=false when the ledger holds no attributable rows (empty ledger,
	// or rows whose source is empty).
	LoadFamilyExpectancy(ctx context.Context, trailingDays int, generatedAt string) (stockpicker.FamilyExpectancyReport, bool, error)
}

// LoadFamilyExpectancy implements FamilyExpectancyProvider.
//
// Cost rate and min-samples come from the live parameters config — the same
// values the daily aggregation job and the sibling endpoints use — so family
// numbers stay comparable with the per-symbol, condition and industry views.
func (p *SQLiteWinRateProvider) LoadFamilyExpectancy(ctx context.Context, trailingDays int, generatedAt string) (stockpicker.FamilyExpectancyReport, bool, error) {
	all, err := stockpicker.LoadOutcomes(ctx, p.db, "", "", "")
	if err != nil {
		return stockpicker.FamilyExpectancyReport{}, false, fmt.Errorf("stocktools: load family outcomes: %w", err)
	}

	params := config.GetParametersConfig()
	report := stockpicker.FamilyExpectancy(
		all,
		params.Stockpicker.Costs.RoundTripPct.Value,
		params.Stockpicker.Calibration.MinSamples.Value,
		0.95,
		trailingDays,
		generatedAt,
	)
	if len(report.Families) == 0 {
		return report, false, nil
	}
	return report, true, nil
}
