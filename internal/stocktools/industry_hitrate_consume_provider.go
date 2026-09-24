package stocktools

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// ─── industry hit-rate consumption provider (2026-09-24, #1942/#1948) ──────
//
// The sectorallocation industry hit-rate consumption chain reads the canonical
// industry hit-rate through its own port (sectorallocation.IndustryHitRateProvider
// — see internal/sectorallocation/industry_hitrate_consume.go) so that package
// does not have to import stockpicker (stockpicker -> ledger -> portfolio ->
// sectorallocation). This adapter is the production binding: it wraps the
// read-only canonical aggregate served by /api/stock/industry_winrate
// (SQLiteWinRateProvider) and maps its rows onto the consumption shape.
//
// The chain is config-gated and off by default
// (sector_allocation.industry_hit_rate_consume_enabled), so this binding is
// inert until an operator flips the gate.

// hitRateReadTimeout bounds one consumption read. The decorator runs on the
// /api/capital-flow/daily request path, so the read must not be able to stall a
// response: a local read-only SQLite aggregate finishes far below this, and a
// timeout degrades to the fail-closed path (reason provider_error).
const hitRateReadTimeout = 2 * time.Second

// SectorAllocationHitRateProvider adapts IndustryWinRateProvider (the canonical
// read-only aggregate) to the sectorallocation consumption port.
type SectorAllocationHitRateProvider struct {
	inner  IndustryWinRateProvider
	regime string // "" = all regimes; reserved for the promotion follow-up
}

// NewSectorAllocationHitRateProvider binds the read-only canonical aggregate.
// A nil inner provider yields an adapter that always reports "not found", i.e.
// the consumption chain fails closed instead of panicking.
func NewSectorAllocationHitRateProvider(inner IndustryWinRateProvider) *SectorAllocationHitRateProvider {
	return &SectorAllocationHitRateProvider{inner: inner}
}

// LoadIndustryWinRate implements sectorallocation.IndustryHitRateProvider.
//
// conditionID is part of the port signature for call-site symmetry; the
// canonical aggregate is keyed by source ("stockpicker-<condition>"), so the
// value is not passed through.
//
// found=false (no outcomes in the window, or an empty regime stratum) maps to
// an empty report and a nil error: "no evidence" is a normal answer that the
// consumer must fail closed on, not an error.
func (p *SectorAllocationHitRateProvider) LoadIndustryWinRate(source, _ string, rollingWindow string) (sectorallocation.IndustryHitRateReport, error) {
	if p == nil || p.inner == nil {
		return sectorallocation.IndustryHitRateReport{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), hitRateReadTimeout)
	defer cancel()

	report, found, err := p.inner.LoadIndustryWinRate(ctx, source, rollingWindow, p.regime)
	if err != nil {
		return sectorallocation.IndustryHitRateReport{}, fmt.Errorf("stocktools: sector allocation hit-rate %s/%s: %w", source, rollingWindow, err)
	}
	if !found {
		return sectorallocation.IndustryHitRateReport{}, nil
	}
	rows := make([]sectorallocation.IndustryHitRateSummary, 0, len(report.Industries))
	for _, row := range report.Industries {
		rows = append(rows, sectorallocation.IndustryHitRateSummary{
			IndustryID:        row.IndustryID,
			Direction:         row.Direction,
			WilsonLower:       row.WilsonLower,
			WilsonUpper:       row.WilsonUpper,
			WinRate:           row.WinRate,
			Observations:      row.Observations,
			CalibrationStatus: row.CalibrationStatus,
		})
	}
	return sectorallocation.IndustryHitRateReport{Industries: rows}, nil
}

// assert the port is satisfied at compile time.
var _ sectorallocation.IndustryHitRateProvider = (*SectorAllocationHitRateProvider)(nil)
