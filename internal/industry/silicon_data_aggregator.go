package industry

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SiliconDataAggregator fetches silicon cycle indicators from the macro
// data pipeline and updates the SiliconCycleTracker via DetectPhase.
//
// Two of the six SiliconIndicators are currently supported:
//   - TSMC monthly revenue YoY (from FinMind via TSMCRevenueProvider)
//   - Philadelphia SOX index YoY (from Yahoo Finance via SOXIndexProvider)
//
// The remaining four indicators default to 0.0 and do not affect phase
// transitions until dedicated data providers are integrated (see
// ExtractSiliconIndicators for details).
type SiliconDataAggregator struct {
	tracker       *SiliconCycleTracker
	macroProvider marketdata.MacroDataProvider
}

// NewSiliconDataAggregator creates a silicon data aggregator wired to the
// given tracker and macro provider.
func NewSiliconDataAggregator(tracker *SiliconCycleTracker, mp marketdata.MacroDataProvider) *SiliconDataAggregator {
	return &SiliconDataAggregator{tracker: tracker, macroProvider: mp}
}

// AggregateSiliconIndicators fetches the latest macro snapshot, extracts
// silicon cycle indicators, and runs phase detection. It is safe for
// concurrent use via the underlying SiliconCycleTracker mutex.
func (a *SiliconDataAggregator) AggregateSiliconIndicators(ctx context.Context) error {
	if a.macroProvider == nil {
		return fmt.Errorf("silicon_data_aggregator: no macro provider configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	snap, err := a.macroProvider.FetchSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("silicon_data_aggregator: fetch snapshot: %w", err)
	}

	indicators := ExtractSiliconIndicators(snap)
	phase := a.tracker.DetectPhase(time.Now(), indicators)

	logging.Info("silicon_data_aggregator", "phase_detected",
		"phase", int(phase),
		"phase_name", GetPhaseName(phase),
		"tsmc_revenue_yoy_pct", fmt.Sprintf("%.1f%%", indicators.TSMCMonthlyRevenueYoY*100),
		"sox_yoy_pct", fmt.Sprintf("%.1f%%", indicators.PhiladelphiaSOXIndexYoY*100),
		"transition_count", a.tracker.GetTransitionCount(),
	)

	return nil
}
