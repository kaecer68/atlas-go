package stockpicker

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

// --- synthetic panel fixtures ---

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

// staticPanel is an in-memory PanelSource for tests.
type staticPanel struct {
	bars  map[string][]HistoricalBar
	flows map[string][]FlowPoint
}

func (p *staticPanel) Bars(_ context.Context, symbol string) ([]HistoricalBar, error) {
	return p.bars[symbol], nil
}

func (p *staticPanel) Flows(_ context.Context, symbol string) ([]FlowPoint, error) {
	return p.flows[symbol], nil
}

// risingSeries builds n consecutive daily bars closing at 100+i for symbol.
func risingSeries(t *testing.T, symbol string, start string, n int) []HistoricalBar {
	t.Helper()
	d := mustDate(t, start)
	bars := make([]HistoricalBar, n)
	for i := range bars {
		bars[i] = HistoricalBar{Date: d.AddDate(0, 0, i), Close: float64(100 + i), Volume: 1000}
	}
	return bars
}

func flowsFor(t *testing.T, start string, n int, net float64) []FlowPoint {
	t.Helper()
	d := mustDate(t, start)
	out := make([]FlowPoint, n)
	for i := range out {
		out[i] = FlowPoint{Date: d.AddDate(0, 0, i).Format("2006-01-02"), ForeignNet: net}
	}
	return out
}

// TestBacktest_NoFutureData is the PIT red line: when the panel contains a
// bar dated after the as-of date (i.e. the backtest would read t+1 data that
// was not yet known at t), RunBacktest must fail.
func TestBacktest_NoFutureData(t *testing.T) {
	bars := risingSeries(t, "2330", "2026-01-05", 30) // last bar 2026-02-03
	panel := &staticPanel{bars: map[string][]HistoricalBar{"2330": bars}}

	cfg := BacktestConfig{
		Universe: []string{"2330"},
		Start:    mustDate(t, "2026-01-05"),
		End:      mustDate(t, "2026-01-20"),
		AsOf:     mustDate(t, "2026-01-20"), // data only known through 01-20
		CostRate: 0.00585,
	}
	_, err := RunBacktest(context.Background(), cfg, panel)
	if err == nil {
		t.Fatal("RunBacktest succeeded despite panel containing a bar after as-of (lookahead); want error")
	}
	if !strings.Contains(err.Error(), "lookahead") {
		t.Fatalf("error = %q, want it to mention lookahead", err)
	}

	// Control: same panel, as-of at the last bar → no lookahead, no error.
	cfg.AsOf = mustDate(t, "2026-02-03")
	if _, err := RunBacktest(context.Background(), cfg, panel); err != nil {
		t.Fatalf("control run with as-of == last bar should pass: %v", err)
	}
}

// TestBacktest_NoFutureData_MissingAsOf verifies the engine refuses to run
// without an explicit as-of date (P0-5: no implicit time.Now() in the engine).
func TestBacktest_NoFutureData_MissingAsOf(t *testing.T) {
	panel := &staticPanel{bars: map[string][]HistoricalBar{"2330": risingSeries(t, "2330", "2026-01-05", 10)}}
	cfg := BacktestConfig{
		Universe: []string{"2330"},
		Start:    mustDate(t, "2026-01-05"),
		End:      mustDate(t, "2026-01-08"),
		CostRate: 0.00585,
	}
	if _, err := RunBacktest(context.Background(), cfg, panel); err == nil {
		t.Fatal("RunBacktest without AsOf should error")
	}
}

// TestBacktest_ProducesOutcomes verifies the engine emits SignalOutcome rows
// with correct fields for a monotonic uptrend (momentum condition fires) and
// positive foreign flows (foreign-3d-net-buy fires).
func TestBacktest_ProducesOutcomes(t *testing.T) {
	bars := risingSeries(t, "2330", "2026-01-05", 40)
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
		t.Fatal("expected outcomes for a monotonic uptrend with positive flows")
	}

	// Every outcome: both demo conditions should fire on every eligible date.
	allowed := map[string]bool{
		"stockpicker-foreign-3d-net-buy":    true,
		"stockpicker-momentum-20d-positive": true,
	}
	seen := make(map[string]bool)
	for _, o := range outcomes {
		if o.Symbol != "2330" {
			t.Errorf("Symbol = %q, want 2330", o.Symbol)
		}
		if _, err := time.Parse("2006-01-02", o.TriggerDate); err != nil {
			t.Errorf("TriggerDate %q not YYYY-MM-DD: %v", o.TriggerDate, err)
		}
		if !allowed[o.Source] {
			t.Errorf("unexpected source %q", o.Source)
		}
		seen[o.Source] = true
		if o.CostRate != 0.00585 {
			t.Errorf("CostRate = %v, want 0.00585", o.CostRate)
		}
		if math.Abs(o.NetForwardReturn-(o.ForwardReturn-0.00585)) > 1e-12 {
			t.Errorf("NetForwardReturn = %v, want ForwardReturn - cost (%v)", o.NetForwardReturn, o.ForwardReturn-0.00585)
		}
	}
	for src := range allowed {
		if !seen[src] {
			t.Errorf("condition %q never fired; want both demo conditions in the outcomes", src)
		}
	}
}

// TestBacktest_NetHitUsed verifies hit is determined on the net return: a
// positive gross return below the round-trip cost must NOT count as a hit
// (P0-3, k3 review R1).
func TestBacktest_NetHitUsed(t *testing.T) {
	// 40 rising bars; override a trigger/fwd pair so the 5-day gross return is
	// +0.4% (positive) but below the 0.585% round-trip cost (net negative).
	bars := risingSeries(t, "2330", "2026-01-05", 40)
	bars[30].Close = 100.0 // trigger 2026-02-04
	bars[35].Close = 100.4 // +5 sessions → gross +0.4%
	flows := flowsFor(t, "2026-01-05", 40, 1000)
	panel := &staticPanel{
		bars:  map[string][]HistoricalBar{"2330": bars},
		flows: map[string][]FlowPoint{"2330": flows},
	}

	cfg := BacktestConfig{
		Universe:    []string{"2330"},
		Start:       mustDate(t, "2026-02-04"),
		End:         mustDate(t, "2026-02-04"),
		AsOf:        mustDate(t, "2026-02-13"),
		ForwardDays: 5,
		CostRate:    0.00585,
		Source:      "stockpicker",
	}
	outcomes, err := RunBacktest(context.Background(), cfg, panel)
	if err != nil {
		t.Fatalf("RunBacktest: %v", err)
	}
	found := false
	for _, o := range outcomes {
		if o.ForwardReturn > 0 {
			found = true
			if o.Hit {
				t.Errorf("outcome %s gross=%.4f net=%.4f: Hit=true, want false (gross < cost 0.585%%)",
					o.TriggerDate, o.ForwardReturn, o.NetForwardReturn)
			}
			if o.NetForwardReturn >= 0 {
				t.Errorf("outcome %s: NetForwardReturn = %.4f, want negative", o.TriggerDate, o.NetForwardReturn)
			}
		}
	}
	if !found {
		t.Fatal("test fixture produced no positive-gross outcome; adjust fixture")
	}
}

// TestBacktest_FundamentalsExcluded verifies no fundamentals condition exists
// in the PR 1c demo set (P0-1: value / all_weather stay live_observe_only).
func TestBacktest_FundamentalsExcluded(t *testing.T) {
	for _, id := range DemoConditions() {
		s := string(id)
		for _, forbidden := range []string{"pe", "pb", "div", "yield", "value", "all-weather", "fundamental"} {
			// Token match on hyphen-delimited parts: "divergence" contains
			// the substring "div" but is a price/volume condition.
			tokens := map[string]bool{}
			for tok := range strings.SplitSeq(s, "-") {
				tokens[tok] = true
			}
			if tokens[forbidden] || (forbidden == "all-weather" && strings.Contains(s, forbidden)) {
				t.Fatalf("condition %q contains fundamentals keyword %q; fundamentals must stay live_observe_only", s, forbidden)
			}
		}
	}
	desc := DescribeConditions()
	if strings.Contains(desc, "fundamentals") {
		t.Fatalf("DescribeConditions mentions fundamentals: %q", desc)
	}
}

// TestBacktest_NoForwardTruth skips trigger dates whose forward window falls
// past the panel end instead of fabricating a return.
func TestBacktest_NoForwardTruth(t *testing.T) {
	bars := risingSeries(t, "2330", "2026-01-05", 12)
	panel := &staticPanel{bars: map[string][]HistoricalBar{"2330": bars}}
	cfg := BacktestConfig{
		Universe:    []string{"2330"},
		Start:       mustDate(t, "2026-01-05"),
		End:         mustDate(t, "2026-01-20"), // window extends past panel end
		AsOf:        mustDate(t, "2026-01-20"),
		ForwardDays: 5,
		CostRate:    0.00585,
	}
	outcomes, err := RunBacktest(context.Background(), cfg, panel)
	if err != nil {
		t.Fatalf("RunBacktest: %v", err)
	}
	// Only dates with 5 future bars (i <= 6) can produce outcomes.
	maxTrigger := mustDate(t, "2026-01-05").AddDate(0, 0, 6)
	for _, o := range outcomes {
		d := mustDate(t, o.TriggerDate)
		if d.After(maxTrigger) {
			t.Errorf("outcome trigger %s has no forward truth (past panel end)", o.TriggerDate)
		}
	}
}
