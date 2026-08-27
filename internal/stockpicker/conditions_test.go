package stockpicker

// conditions_test.go — PR 2a configurable condition engine tests (TDD).
//
// Covers: registry register/lookup, foreign-3d-net-buy eval (3d window >
// threshold triggers, <= threshold does not, missing data fail-open),
// momentum eval, parameters flowing from configs/parameters.json,
// fundamentals live_observe_only placeholder, condition injection into
// RunBacktest, and DemoConditions()/DefaultConditions() compatibility.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// --- 1. registry register / lookup ---

func TestConditionRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewConditionRegistry()
	if err := reg.Register(Condition{ID: "a", Name: "A", Type: ConditionTypePrice, Params: map[string]float64{}}); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := reg.Register(Condition{ID: "b", Name: "B", Type: ConditionTypeFlow, Params: map[string]float64{}}); err != nil {
		t.Fatalf("register b: %v", err)
	}

	c, ok := reg.Lookup("a")
	if !ok || c == nil || c.ID != "a" {
		t.Fatalf("Lookup(a) = (%v, %v), want (a, true)", c, ok)
	}
	if _, ok := reg.Lookup("nope"); ok {
		t.Fatal("Lookup(nope) = true, want false")
	}

	if err := reg.Register(Condition{ID: "a"}); err == nil {
		t.Fatal("duplicate registration must error")
	}
	if err := reg.Register(Condition{}); err == nil {
		t.Fatal("empty-ID registration must error")
	}

	ids := reg.IDs()
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("IDs() = %v, want [a b] in registration order", ids)
	}
	all := reg.All()
	if len(all) != 2 || all[0].ID != "a" || all[1].ID != "b" {
		t.Fatalf("All() order = %+v, want [a b]", all)
	}
}

// --- 2. foreign-3d-net-buy eval ---

func TestCondition_EvalForeign3DNetBuy(t *testing.T) {
	reg := NewDefaultConditionRegistry(nil)
	cond, ok := reg.Lookup(string(ConditionForeign3DNetBuy))
	if !ok {
		t.Fatalf("registry missing %s", ConditionForeign3DNetBuy)
	}

	flowDates := []string{"2026-01-05", "2026-01-06", "2026-01-07"}
	trigger := mustDate(t, "2026-01-08")

	pos := map[string]FlowPoint{
		"2026-01-05": {Date: "2026-01-05", ForeignNet: 1000},
		"2026-01-06": {Date: "2026-01-06", ForeignNet: 1000},
		"2026-01-07": {Date: "2026-01-07", ForeignNet: 1000},
	}
	if !cond.Eval(nil, pos, flowDates, trigger) {
		t.Fatal("3-day cumulative foreign net buy > 0 must trigger")
	}

	neg := map[string]FlowPoint{
		"2026-01-05": {Date: "2026-01-05", ForeignNet: -500},
		"2026-01-06": {Date: "2026-01-06", ForeignNet: -500},
		"2026-01-07": {Date: "2026-01-07", ForeignNet: -500},
	}
	if cond.Eval(nil, neg, flowDates, trigger) {
		t.Fatal("negative 3-day cumulative foreign net buy must not trigger")
	}

	zero := map[string]FlowPoint{
		"2026-01-05": {Date: "2026-01-05", ForeignNet: 0},
		"2026-01-06": {Date: "2026-01-06", ForeignNet: 0},
		"2026-01-07": {Date: "2026-01-07", ForeignNet: 0},
	}
	if cond.Eval(nil, zero, flowDates, trigger) {
		t.Fatal("zero cumulative foreign net buy (sum == threshold) must not trigger")
	}

	// Missing data → fail-open (no trigger): fewer than 3 flow dates <= t.
	if cond.Eval(nil, pos, []string{"2026-01-05"}, trigger) {
		t.Fatal("insufficient PIT flow history must fail open (no trigger)")
	}
	// Empty flow history → no trigger.
	if cond.Eval(nil, pos, nil, trigger) {
		t.Fatal("empty flow history must fail open (no trigger)")
	}
}

// --- 3. momentum eval ---

func TestCondition_EvalMomentum20D(t *testing.T) {
	reg := NewDefaultConditionRegistry(nil)
	cond, ok := reg.Lookup(string(ConditionMomentum20D))
	if !ok {
		t.Fatalf("registry missing %s", ConditionMomentum20D)
	}

	// 25 rising bars close 100..124; at the last bar momentum =
	// 124/104 - 1 = +19.2% > 0 → triggers.
	bars := risingSeries(t, "2330", "2026-01-05", 25)
	last := bars[len(bars)-1].Date
	if !cond.Eval(bars, nil, nil, last) {
		t.Fatal("positive 20-day momentum must trigger")
	}

	// 25 falling bars close 200..176 → momentum < 0 → no trigger.
	falling := make([]HistoricalBar, 25)
	d := mustDate(t, "2026-01-05")
	for i := range falling {
		falling[i] = HistoricalBar{Date: d.AddDate(0, 0, i), Close: float64(200 - i), Volume: 1000}
	}
	if cond.Eval(falling, nil, nil, falling[len(falling)-1].Date) {
		t.Fatal("negative 20-day momentum must not trigger")
	}

	// Not enough history (< 21 bars) → no trigger.
	if cond.Eval(bars[:15], nil, nil, bars[14].Date) {
		t.Fatal("insufficient bars for 20d momentum must not trigger")
	}
}

// --- 4. parameters from configs/parameters.json ---

func TestCondition_ParamsFromConfig(t *testing.T) {
	cfg, err := config.LoadParametersConfig("../../configs/parameters.json")
	if err != nil {
		t.Fatalf("load parameters.json: %v", err)
	}
	reg := NewDefaultConditionRegistry(&cfg.Stockpicker.Conditions)

	foreign, ok := reg.Lookup(string(ConditionForeign3DNetBuy))
	if !ok {
		t.Fatal("missing foreign condition")
	}
	if got := foreign.Param(ParamWindowDays, -1); got != 3 {
		t.Errorf("foreign window_days = %v, want 3 (from parameters.json)", got)
	}
	if got := foreign.Param(ParamThreshold, -1); got != 0 {
		t.Errorf("foreign threshold = %v, want 0 (from parameters.json)", got)
	}

	mom, ok := reg.Lookup(string(ConditionMomentum20D))
	if !ok {
		t.Fatal("missing momentum condition")
	}
	if got := mom.Param(ParamWindowDays, -1); got != 20 {
		t.Errorf("momentum window_days = %v, want 20 (from parameters.json)", got)
	}
	if got := mom.Param(ParamThreshold, -1); got != 0 {
		t.Errorf("momentum threshold = %v, want 0 (from parameters.json)", got)
	}

	// Window/threshold must change behavior (they are not hard-coded).
	flowDates := []string{"2026-01-05", "2026-01-06", "2026-01-07"}
	flows500 := map[string]FlowPoint{
		"2026-01-05": {Date: "2026-01-05", ForeignNet: 500},
		"2026-01-06": {Date: "2026-01-06", ForeignNet: 500},
		"2026-01-07": {Date: "2026-01-07", ForeignNet: 500},
	}
	trigger := mustDate(t, "2026-01-08")

	win2 := config.StockpickerConditionsParameters{
		Foreign3DNetBuy: config.StockpickerConditionWindow{
			WindowDays: config.ParameterMetadata[float64]{Value: 2},
			Threshold:  config.ParameterMetadata[float64]{Value: 1000},
		},
	}
	foreign2, _ := NewDefaultConditionRegistry(&win2).Lookup(string(ConditionForeign3DNetBuy))
	// window=2 → last 2 days sum 1000; threshold 1000 → 1000 > 1000 is false.
	if foreign2.Eval(nil, flows500, flowDates, trigger) {
		t.Fatal("window=2 threshold=1000 with 2-day sum 1000 must not trigger (window/threshold must take effect)")
	}

	win3 := config.StockpickerConditionsParameters{
		Foreign3DNetBuy: config.StockpickerConditionWindow{
			WindowDays: config.ParameterMetadata[float64]{Value: 3},
			Threshold:  config.ParameterMetadata[float64]{Value: 1000},
		},
	}
	foreign3, _ := NewDefaultConditionRegistry(&win3).Lookup(string(ConditionForeign3DNetBuy))
	// Same inputs, window=3 → 3-day sum 1500 > 1000 → triggers. The same
	// flows with the default window=3 threshold=0 also trigger, so the
	// difference is driven purely by the configured parameters.
	if !foreign3.Eval(nil, flows500, flowDates, trigger) {
		t.Fatal("window=3 threshold=1000 with 3-day sum 1500 must trigger (window/threshold must take effect)")
	}

	// momentum window=10 threshold=0.5: rising 11 bars close 100..110 →
	// 110/100-1 = 0.10 < 0.5 → no trigger; with threshold 0 it would fire.
	mom10 := config.StockpickerConditionsParameters{
		Momentum20DPosit: config.StockpickerConditionWindow{
			WindowDays: config.ParameterMetadata[float64]{Value: 10},
			Threshold:  config.ParameterMetadata[float64]{Value: 0.5},
		},
	}
	mom2, _ := NewDefaultConditionRegistry(&mom10).Lookup(string(ConditionMomentum20D))
	rise11 := risingSeries(t, "2330", "2026-01-05", 11)
	if mom2.Eval(rise11, nil, nil, rise11[len(rise11)-1].Date) {
		t.Fatal("momentum 0.10 below threshold 0.5 must not trigger (threshold must take effect)")
	}
}

// --- 5. fundamentals live_observe_only ---

func TestCondition_FundamentalLiveObserveOnly(t *testing.T) {
	c := NewFundamentalPlaceholder()
	if !c.IsLiveObserveOnly() {
		t.Fatal("fundamentals placeholder must be marked live_observe_only (P0-1)")
	}
	if c.Type != ConditionTypeFundamentalLive {
		t.Fatalf("Type = %q, want fundamental-live", c.Type)
	}
	if c.ID == "" {
		t.Fatal("placeholder must have an ID")
	}

	bars := risingSeries(t, "2330", "2026-01-05", 40)
	flows := flowsFor(t, "2026-01-05", 40, 1000)
	flowDates := make([]string, len(flows))
	for i, f := range flows {
		flowDates[i] = f.Date
	}
	// Eval must always be false, regardless of how rich the inputs are.
	if c.Eval(bars, flowsForMap(flows), flowDates, mustDate(t, "2026-01-30")) {
		t.Fatal("fundamentals placeholder must never fire in backtest (P0-1)")
	}
	if c.Eval(nil, nil, nil, time.Time{}) {
		t.Fatal("fundamentals placeholder must never fire even with empty inputs")
	}

	// And it must not be part of the default backtest condition set.
	for _, id := range DemoConditions() {
		if string(id) == c.ID {
			t.Fatalf("fundamentals placeholder %q must not be a default backtest condition", c.ID)
		}
	}
	for _, def := range DefaultConditions() {
		if def.ID == c.ID {
			t.Fatalf("DefaultConditions() must not include fundamentals placeholder %q", c.ID)
		}
	}
}

// flowsForMap converts []FlowPoint into the map form the engine passes to Eval.
func flowsForMap(flows []FlowPoint) map[string]FlowPoint {
	out := make(map[string]FlowPoint, len(flows))
	for _, f := range flows {
		out[f.Date] = f
	}
	return out
}

// --- 6. RunBacktest with injected conditions ---

func TestRunBacktest_ConditionsInjected(t *testing.T) {
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

	momentumOnly, ok := NewDefaultConditionRegistry(nil).Lookup(string(ConditionMomentum20D))
	if !ok {
		t.Fatal("registry missing momentum condition")
	}
	outcomes, err := RunBacktest(context.Background(), cfg, panel, *momentumOnly)
	if err != nil {
		t.Fatalf("RunBacktest: %v", err)
	}
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes from the injected momentum condition")
	}
	for _, o := range outcomes {
		if o.Source != "stockpicker-"+string(ConditionMomentum20D) {
			t.Errorf("Source = %q, want only %q (injected conditions must run exclusively)",
				o.Source, "stockpicker-"+string(ConditionMomentum20D))
		}
	}

	// Control: a condition that never fires yields zero outcomes for it.
	never := NewFundamentalPlaceholder()
	out2, err := RunBacktest(context.Background(), cfg, panel, never)
	if err != nil {
		t.Fatalf("RunBacktest(never): %v", err)
	}
	if len(out2) != 0 {
		t.Fatalf("placeholder-only run produced %d outcomes, want 0 (P0-1)", len(out2))
	}
}

// --- 9. DemoConditions compatibility ---

func TestDemoConditions_Compatibility(t *testing.T) {
	demo := DemoConditions()
	defaults := DefaultConditions()

	if len(demo) != len(defaults) {
		t.Fatalf("DemoConditions() has %d IDs, DefaultConditions() has %d conditions; want equal",
			len(demo), len(defaults))
	}
	for i := range demo {
		if string(demo[i]) != defaults[i].ID {
			t.Errorf("DemoConditions()[%d] = %q, DefaultConditions()[%d].ID = %q; want identical order",
				i, demo[i], i, defaults[i].ID)
		}
	}

	// The default set stays PIT-only: no fundamentals keyword in any ID.
	for _, c := range defaults {
		s := strings.ToLower(c.ID)
		for _, bad := range []string{"pe", "pb", "div", "yield", "value", "all-weather", "fundamental"} {
			if strings.Contains(s, bad) {
				t.Fatalf("default condition %q contains fundamentals keyword %q", c.ID, bad)
			}
		}
		if c.IsLiveObserveOnly() {
			t.Fatalf("default condition %q must not be live_observe_only", c.ID)
		}
	}
}
