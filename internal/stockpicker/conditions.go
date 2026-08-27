// Package conditions.go — PR 2a configurable condition engine.
//
// Upgrades the PR 1c hardcoded demo condition set (backtest.go) into an
// ordered registry of configurable, point-in-time conditions. Every numeric
// parameter (window, threshold) is read from configs/parameters.json →
// stockpicker.conditions.<id>; conditions.go contains no magic numbers
// (P0-6). Fundamentals conditions stay live_observe_only placeholders
// (P0-1): data/fundamentals.json is a single dateless snapshot, so no
// historical evaluation exists and their Eval always returns false.
package stockpicker

import (
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// ConditionType classifies a condition by the point-in-time data channel it
// reads.
type ConditionType string

const (
	// ConditionTypePrice reads only price bars (PIT-safe).
	ConditionTypePrice ConditionType = "price"
	// ConditionTypeFlow reads only per-symbol T86 institutional flows
	// (PIT-safe).
	ConditionTypeFlow ConditionType = "flow"
	// ConditionTypeFundamentalLive is a live_observe_only placeholder:
	// fundamentals data has no historical series, so the condition can only
	// be observed against the live snapshot and is never backtested (P0-1).
	ConditionTypeFundamentalLive ConditionType = "fundamental-live"
)

// Condition.Params keys, shared by all registered conditions.
const (
	// ParamWindowDays is the lookback window in trading days.
	ParamWindowDays = "window_days"
	// ParamThreshold is the trigger threshold the window aggregate must
	// strictly exceed.
	ParamThreshold = "threshold"
)

// Condition is a configurable, point-in-time stock-selection rule.
//
// ID is the canonical key used in outcome source suffixes
// ("stockpicker-<id>"), CLI -conditions lists and parameters.json JSON keys.
// Params carries the numeric parameters read from configs/parameters.json;
// the evaluator reads them via Param, so a parameter change does not require
// recompiling conditions.go.
type Condition struct {
	ID     string
	Name   string
	Type   ConditionType
	Params map[string]float64

	eval func(c *Condition, bars []HistoricalBar, flows map[string]FlowPoint, flowDates []string, t time.Time) bool
}

// Eval reports whether the condition fires at trigger date t. Evaluators
// MUST read only data dated <= t (P0-1 PIT red line). A nil receiver, nil
// eval or a live_observe_only placeholder never fires (conservative).
func (c *Condition) Eval(bars []HistoricalBar, flows map[string]FlowPoint, flowDates []string, t time.Time) bool {
	if c == nil || c.eval == nil {
		return false
	}
	return c.eval(c, bars, flows, flowDates, t)
}

// Param returns the numeric parameter for key, or def when absent.
func (c *Condition) Param(key string, def float64) float64 {
	if c == nil || c.Params == nil {
		return def
	}
	if v, ok := c.Params[key]; ok {
		return v
	}
	return def
}

// IsLiveObserveOnly reports whether the condition is a fundamentals
// placeholder that cannot be backtested (P0-1).
func (c *Condition) IsLiveObserveOnly() bool {
	return c != nil && c.Type == ConditionTypeFundamentalLive
}

// ConditionRegistry is an ordered registry of named conditions.
type ConditionRegistry struct {
	byID  map[string]*Condition
	order []string
}

// NewConditionRegistry returns an empty registry.
func NewConditionRegistry() *ConditionRegistry {
	return &ConditionRegistry{byID: make(map[string]*Condition)}
}

// Register adds c to the registry. Duplicate and empty IDs are rejected.
func (r *ConditionRegistry) Register(c Condition) error {
	if c.ID == "" {
		return fmt.Errorf("stockpicker: condition ID must not be empty")
	}
	if _, ok := r.byID[c.ID]; ok {
		return fmt.Errorf("stockpicker: condition %q already registered", c.ID)
	}
	cp := c
	r.byID[cp.ID] = &cp
	r.order = append(r.order, cp.ID)
	return nil
}

// Lookup returns the condition registered under id.
func (r *ConditionRegistry) Lookup(id string) (*Condition, bool) {
	if r == nil {
		return nil, false
	}
	c, ok := r.byID[id]
	return c, ok
}

// IDs returns the registered condition IDs in registration order.
func (r *ConditionRegistry) IDs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// All returns copies of all registered conditions in registration order.
func (r *ConditionRegistry) All() []Condition {
	if r == nil {
		return nil
	}
	out := make([]Condition, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, *r.byID[id])
	}
	return out
}

// NewDefaultConditionRegistry builds a registry containing the default
// backtest-eligible conditions (foreign-3d-net-buy, momentum-20d-positive)
// parameterized from params. When params is nil the loaded parameters config
// (configs/parameters.json, or its defaults) is used.
func NewDefaultConditionRegistry(params *config.StockpickerConditionsParameters) *ConditionRegistry {
	cp := config.StockpickerConditionsParameters{}
	if params != nil {
		cp = *params
	} else {
		cp = config.GetParametersConfig().Stockpicker.Conditions
	}
	reg := NewConditionRegistry()
	_ = reg.Register(newForeign3DNetBuy(cp.Foreign3DNetBuy))
	_ = reg.Register(newMomentum20D(cp.Momentum20DPosit))
	return reg
}

// DefaultConditions returns the default backtest-eligible condition set —
// the PR 1c demo conditions rebuilt as configurable conditions (3d foreign
// window, 20d momentum, threshold 0 from parameters.json). Fundamentals
// placeholders are excluded (P0-1).
func DefaultConditions() []Condition {
	return NewDefaultConditionRegistry(nil).All()
}

// NewFundamentalPlaceholder returns the P0-1 fundamentals placeholder
// (value / all_weather PE/PB/div-yield). data/fundamentals.json is a single
// dateless snapshot, so no point-in-time evaluation exists: Eval always
// returns false and the condition is marked live_observe_only. It is never
// part of the default backtest condition set.
func NewFundamentalPlaceholder() Condition {
	return Condition{
		ID:   "fundamental-value",
		Name: "Value fundamentals (live observe only)",
		Type: ConditionTypeFundamentalLive,
		eval: evalFundamentalLiveObserveOnly,
	}
}

// newForeign3DNetBuy builds the foreign-3d-net-buy condition.
func newForeign3DNetBuy(p config.StockpickerConditionWindow) Condition {
	return Condition{
		ID:     string(ConditionForeign3DNetBuy),
		Name:   "Foreign net buy over window",
		Type:   ConditionTypeFlow,
		Params: map[string]float64{ParamWindowDays: p.WindowDays.Value, ParamThreshold: p.Threshold.Value},
		eval:   evalForeign3DNetBuy,
	}
}

// newMomentum20D builds the momentum-20d-positive condition.
func newMomentum20D(p config.StockpickerConditionWindow) Condition {
	return Condition{
		ID:     string(ConditionMomentum20D),
		Name:   "Momentum over window positive",
		Type:   ConditionTypePrice,
		Params: map[string]float64{ParamWindowDays: p.WindowDays.Value, ParamThreshold: p.Threshold.Value},
		eval:   evalMomentum20D,
	}
}

// evalForeign3DNetBuy fires when the cumulative foreign net buy over the
// most recent window_days flow dates dated <= t exceeds threshold.
// SearchStrings returns the first index with date >= t; everything before it
// is dated <= t (PIT). Fewer than window flow dates <= t → no trigger
// (fail-open, conservative PIT semantics).
func evalForeign3DNetBuy(c *Condition, _ []HistoricalBar, flows map[string]FlowPoint, flowDates []string, t time.Time) bool {
	window := int(c.Param(ParamWindowDays, 3))
	threshold := c.Param(ParamThreshold, 0)
	if window <= 0 {
		return false
	}
	idx := sort.SearchStrings(flowDates, t.Format("2006-01-02"))
	if idx < window {
		return false
	}
	var sum float64
	for _, d := range flowDates[idx-window : idx] {
		sum += flows[d].ForeignNet
	}
	return sum > threshold
}

// evalMomentum20D fires when the window-day momentum
// close[t]/close[t-window] - 1 exceeds threshold. bars is the prefix of
// bars dated <= t (the engine passes bars[:i+1]); only the last window+1
// closes are read (PIT).
func evalMomentum20D(c *Condition, bars []HistoricalBar, _ map[string]FlowPoint, _ []string, _ time.Time) bool {
	window := int(c.Param(ParamWindowDays, 20))
	threshold := c.Param(ParamThreshold, 0)
	if window <= 0 || len(bars) < window+1 {
		return false
	}
	base := bars[len(bars)-1-window].Close
	if base <= 0 {
		return false
	}
	return bars[len(bars)-1].Close/base-1 > threshold
}

// evalFundamentalLiveObserveOnly is the P0-1 PIT red-line placeholder:
// fundamentals.json is a single dateless snapshot, so no point-in-time
// evaluation exists. The condition is registered for live observation only
// and never fires in a backtest.
func evalFundamentalLiveObserveOnly(_ *Condition, _ []HistoricalBar, _ map[string]FlowPoint, _ []string, _ time.Time) bool {
	return false
}
