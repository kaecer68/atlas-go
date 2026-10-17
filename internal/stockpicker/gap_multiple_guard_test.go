package stockpicker

import (
	"context"
	"testing"
)

// TestBacktest_RejectsGapMultiple locks k3 decision 3-1 in the stockpicker panel
// backtest: a corrupted stored multiple is made of two usable closes, so the
// price guard passed and the outcome was aggregated into the win rate as a
// +hundreds-of-percent forward return.
func TestBacktest_RejectsGapMultiple(t *testing.T) {
	bars := risingSeries(t, "2330", "2026-01-05", 40)
	// The trigger on 2026-01-10 resolves its 5-day horizon on 2026-01-15.
	for i := range bars {
		if bars[i].Date.Equal(mustDate(t, "2026-01-15")) {
			bars[i].Close *= 10
		}
	}
	flows := flowsFor(t, "2026-01-05", 40, 1000)
	panel := &staticPanel{
		bars:  map[string][]HistoricalBar{"2330": bars},
		flows: map[string][]FlowPoint{"2330": flows},
	}
	cfg := BacktestConfig{
		Universe:    []string{"2330"},
		Start:       mustDate(t, "2026-01-05"),
		End:         mustDate(t, "2026-01-30"),
		AsOf:        mustDate(t, "2026-02-20"),
		ForwardDays: 5,
		CostRate:    0.00585,
		Source:      "stockpicker",
	}

	outcomes, err := RunBacktest(context.Background(), cfg, panel)
	if err != nil {
		t.Fatalf("RunBacktest: %v", err)
	}
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes for a monotonic uptrend")
	}
	for _, o := range outcomes {
		if o.ForwardReturn > 0.5 {
			t.Errorf("outcome %s @ %s has ForwardReturn %v: a corrupted multiple was recorded as a return",
				o.Source, o.TriggerDate, o.ForwardReturn)
		}
		if o.TriggerDate == "2026-01-10" {
			t.Errorf("outcome for 2026-01-10 recorded despite its horizon bar carrying a 10x close")
		}
	}

	// Control: the clean panel yields strictly more outcomes for the same config.
	cleanPanel := &staticPanel{
		bars:  map[string][]HistoricalBar{"2330": risingSeries(t, "2330", "2026-01-05", 40)},
		flows: map[string][]FlowPoint{"2330": flows},
	}
	cleanOutcomes, err := RunBacktest(context.Background(), cfg, cleanPanel)
	if err != nil {
		t.Fatalf("RunBacktest (clean): %v", err)
	}
	if len(cleanOutcomes) <= len(outcomes) {
		t.Errorf("clean panel produced %d outcomes, gated panel %d: normal data must not change", len(cleanOutcomes), len(outcomes))
	}
}
