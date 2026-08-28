// Package validator.go — PR 2b mixed-granularity capital-flow gateway.
//
// The stockpicker condition engine (conditions.go, PR 2a) decides WHEN a
// signal fires, but win rates built on "外資小幅賣超但勝率高" noise are not
// trustworthy. This file adds the gate in front of that: every candidate
// (symbol + condition) must sit on top of MEANINGFUL capital flow.
//
// The gate is a two-level design (PR 2b review fix for granularity):
//
//   - 個股層 (per-symbol): the foreign layer reads the CHECKED SYMBOL's own
//     T86 foreign net flow — FlowPoint.ForeignNet (backtest.go:38, units
//     shares/1e3 = 千股) — via the points map keyed by symbol. The symbol
//     argument is a real query key, not a label. The layer passes when
//     |ForeignNet| converted to 億股 (÷1e5) exceeds min_abs_net. Per-symbol
//     FlowPoint has no z-score channel, so the foreign layer is
//     magnitude-only.
//   - 市場 regime 層 (market level): institutional and retail have no
//     per-symbol data source, so they stay as market-regime layers backed by
//     the market-wide capitalflow ForceScore dimensions (ForceInstitutional
//     / ForceRetail). 文件明示: 無個股層級資料，僅供市場 regime 參考.
//
// Layer semantics: a market layer passes when the force reading is
// meaningful in ABSOLUTE terms — abs(RawValue) > min_abs_raw OR abs(ZScore)
// > min_abs_z (strict >; a threshold <= 0 disables that metric). Direction
// is the condition's job, not the gate's: the gate only rejects
// magnitude-noise, exactly the "小幅賣超但勝率高" failure mode.
//
// Missing data is handled per-layer fail-open: when a layer's data is absent
// (no FlowPoint for the symbol, force missing from the input, or
// DataAvailable=false — capitalflow's "source channel empty", spec §8.3 /
// CF-INV-06), the layer is SKIPPED and annotated — never treated as a
// failure, so a data gap cannot silently kill a symbol ("缺層 skip 並註記，
// 不可誤殺"). When EVERY enforced layer is skipped the verdict is
// AllSkipped=true with SkippedCount: with FailClosedWhenAllMissing=true (the
// default, live path) the gate fails closed — "全缺層不交易"; backtests may
// set false to keep evaluating on partial data.
//
// All thresholds live in configs/parameters.json → stockpicker.flow_gateway,
// read through the config singleton (config.GetParametersConfig().Stockpicker
// .FlowGateway, internal/config/parameters.go). The gate logic itself
// contains no magic numbers.
package stockpicker

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
)

// foreignNetToYiShares converts FlowPoint.ForeignNet (千股 = shares/1e3) to
// 億股 (hundred million shares): 1 億股 = 1e8 shares = 1e5 × 千股, so
// 億股 = ForeignNet / 1e5. All foreign per-symbol thresholds are expressed
// in 億股.
const foreignNetToYiShares = 1e5

// FlowLayer identifies one of the three gateway layers.
type FlowLayer string

const (
	// FlowLayerForeign is the 外資 layer — per-symbol (個股層), backed by
	// FlowPoint.ForeignNet.
	FlowLayerForeign FlowLayer = "foreign"
	// FlowLayerInstitutional is the 投信 layer — market regime (市場層),
	// backed by capitalflow.ForceInstitutional.
	FlowLayerInstitutional FlowLayer = "institutional"
	// FlowLayerRetail is the 散戶 layer — market regime (市場層), backed by
	// capitalflow.ForceRetail.
	FlowLayerRetail FlowLayer = "retail"
)

// AllFlowLayers is the canonical three-layer evaluation order. It is the
// default enforcement set when a condition declares no override.
var AllFlowLayers = []FlowLayer{FlowLayerForeign, FlowLayerInstitutional, FlowLayerRetail}

// DisplayName returns the Chinese display name for a layer.
func (l FlowLayer) DisplayName() string {
	switch l {
	case FlowLayerForeign:
		return "外資"
	case FlowLayerInstitutional:
		return "投信"
	case FlowLayerRetail:
		return "散戶"
	default:
		return string(l)
	}
}

// LayerThreshold holds the two numeric gates for one MARKET-REGIME layer
// (institutional / retail). A gate value <= 0 disables that metric
// (fail-open), so a layer with both gates at 0 always passes whenever its
// data is available.
type LayerThreshold struct {
	// MinAbsRaw is the minimum absolute RawValue (units follow the
	// capitalflow provenance row: 億股 for institutional, pct_composite for
	// retail).
	MinAbsRaw float64
	// MinAbsZ is the minimum absolute ZScore (capitalflow 60-day rolling).
	MinAbsZ float64
}

// ForeignThreshold is the per-symbol foreign layer gate (個股層).
type ForeignThreshold struct {
	// MinAbsNet is the minimum |foreign net buy| of the CHECKED SYMBOL in
	// 億股. The evaluator converts FlowPoint.ForeignNet (千股) by ÷1e5.
	// A value <= 0 disables the gate (fail-open).
	MinAbsNet float64
}

// FlowGatewayParameters is the evaluator's lean parameter table, converted
// from configs/parameters.json → stockpicker.flow_gateway (the authoritative
// config-package table with provenance metadata; see flowGatewayParamsFromConfig).
// ConditionLayers optionally narrows the enforced layer set per condition ID;
// a condition absent from the map enforces all three layers.
type FlowGatewayParameters struct {
	Foreign         ForeignThreshold
	Institutional   LayerThreshold
	Retail          LayerThreshold
	ConditionLayers map[string][]FlowLayer
	// FailClosedWhenAllMissing: true → when every enforced layer is skipped
	// (missing data), the verdict fails closed (no-decision). false → passes
	// with AllSkipped=true. Default true (live path: 全缺層不交易).
	FailClosedWhenAllMissing bool
}

// Layer returns the market-regime threshold for a layer. The foreign layer
// has its own threshold type (ForeignThreshold) and is handled separately by
// Check; this method returns a zero threshold for it.
func (p FlowGatewayParameters) Layer(l FlowLayer) LayerThreshold {
	switch l {
	case FlowLayerInstitutional:
		return p.Institutional
	case FlowLayerRetail:
		return p.Retail
	default:
		return LayerThreshold{}
	}
}

// LayersFor returns the layer set a condition enforces: the per-condition
// override when declared, otherwise all three layers. The returned slice is
// a copy; callers may not mutate the parameter table through it.
func (p FlowGatewayParameters) LayersFor(conditionID string) []FlowLayer {
	if ls, ok := p.ConditionLayers[conditionID]; ok && len(ls) > 0 {
		out := make([]FlowLayer, len(ls))
		copy(out, ls)
		return out
	}
	out := make([]FlowLayer, len(AllFlowLayers))
	copy(out, AllFlowLayers)
	return out
}

// DefaultFlowGatewayParameters returns the documented fallback thresholds.
// The values mirror configs/parameters.json → stockpicker.flow_gateway (and
// internal/config defaultFlowGatewayParameters) and are used only as the
// fallback when the config singleton is unavailable. The gate logic never
// reads these literals directly.
func DefaultFlowGatewayParameters() FlowGatewayParameters {
	return FlowGatewayParameters{
		Foreign: ForeignThreshold{
			MinAbsNet: 0.1, // 億股: meaningful per-symbol foreign net buy (≈1 萬張)
		},
		Institutional: LayerThreshold{
			MinAbsRaw: 0.3, // 億股: 投信 daily magnitudes are smaller than 外資
			MinAbsZ:   0.5, // capitalflow trendFor bullish/bearish boundary
		},
		Retail: LayerThreshold{
			MinAbsRaw: 1.0, // pct_composite: >1pct margin+short move is meaningful
			MinAbsZ:   0.5,
		},
		ConditionLayers:          map[string][]FlowLayer{},
		FailClosedWhenAllMissing: true,
	}
}

// flowGatewayParamsFromConfig maps the config-package flow_gateway section
// (configs/parameters.json → stockpicker.flow_gateway, the authoritative
// parameter table) into the evaluator's lean parameter table. Threshold
// units are preserved verbatim: foreign min_abs_net is in 億股 (compared
// against FlowPoint.ForeignNet ÷1e5), institutional/retail min_abs_raw
// follow the capitalflow provenance row (億股 / pct_composite).
func flowGatewayParamsFromConfig(cfg config.FlowGatewayParameters) FlowGatewayParameters {
	p := DefaultFlowGatewayParameters()
	p.Foreign = ForeignThreshold{MinAbsNet: cfg.Layers.Foreign.MinAbsNet.Value}
	p.Institutional = LayerThreshold{MinAbsRaw: cfg.Layers.Institutional.MinAbsRaw.Value, MinAbsZ: cfg.Layers.Institutional.MinAbsZ.Value}
	p.Retail = LayerThreshold{MinAbsRaw: cfg.Layers.Retail.MinAbsRaw.Value, MinAbsZ: cfg.Layers.Retail.MinAbsZ.Value}
	p.FailClosedWhenAllMissing = cfg.FailClosedWhenAllMissing.Value
	p.ConditionLayers = map[string][]FlowLayer{}
	condLayers := map[string][]string{
		string(ConditionForeign3DNetBuy): cfg.Conditions.Foreign3DNetBuy.Layers.Value,
		string(ConditionMomentum20D):     cfg.Conditions.Momentum20DPosit.Layers.Value,
	}
	for id, names := range condLayers {
		if len(names) == 0 {
			continue
		}
		ls := make([]FlowLayer, 0, len(names))
		for _, n := range names {
			ls = append(ls, FlowLayer(n))
		}
		p.ConditionLayers[id] = ls
	}
	return p
}

// LayerStatus is the per-layer verdict of a gateway check.
type LayerStatus string

const (
	// LayerStatusPass means the layer's data was available and its
	// thresholds were met.
	LayerStatusPass LayerStatus = "pass"
	// LayerStatusFail means the layer's data was available but its
	// thresholds were not met — the symbol fails the gate.
	LayerStatusFail LayerStatus = "fail"
	// LayerStatusSkip means the layer's data was missing; the layer is
	// annotated and ignored (fail-open, never kills the symbol by itself).
	LayerStatusSkip LayerStatus = "skip"
)

// LayerVerdict is one layer's outcome inside a FlowVerdict.
type LayerVerdict struct {
	Layer  FlowLayer
	Status LayerStatus
	Reason string
}

// FlowVerdict is the result of a two-level gateway check for one
// (symbol, condition) candidate.
type FlowVerdict struct {
	Symbol      string
	ConditionID string
	Pass        bool
	Layers      []LayerVerdict
	// AllSkipped is true when every enforced layer was skipped (missing
	// data). With FailClosedWhenAllMissing=true the verdict then fails
	// closed (no-decision); with false it passes.
	AllSkipped bool
	// SkippedCount is the number of enforced layers that were skipped.
	SkippedCount int
	// Reason annotates the verdict when it fails closed on all-missing data.
	Reason string
}

// FlowGateway is the two-level capital-flow gate. It is a pure evaluator:
// thresholds come from the parameter table, per-symbol flows come from the
// caller (stockpicker backtest FlowPoint rows), market forces come from the
// caller (typically capitalflow.Service.LatestDaily → DailyReport).
type FlowGateway struct {
	params FlowGatewayParameters
}

// NewFlowGateway builds a gateway with the given thresholds.
func NewFlowGateway(params FlowGatewayParameters) *FlowGateway {
	return &FlowGateway{params: params}
}

// NewDefaultFlowGateway builds a gateway with the loaded
// stockpicker.flow_gateway parameters from the config singleton
// (config.GetParametersConfig().Stockpicker.FlowGateway —
// configs/parameters.json or the documented defaults). It never errors: the
// config singleton falls back to defaults when no file is present.
func NewDefaultFlowGateway() *FlowGateway {
	cfg := config.GetParametersConfig()
	if cfg == nil {
		return NewFlowGateway(DefaultFlowGatewayParameters())
	}
	return NewFlowGateway(flowGatewayParamsFromConfig(cfg.Stockpicker.FlowGateway))
}

// Check evaluates the two-level gateway for symbol + condition against the
// symbol's per-symbol foreign flow (points[symbol], 個股層) and the supplied
// market-wide capitalflow force readings (市場 regime 層). The forces are
// usually report.Forces from capitalflow.Service.LatestDaily (see
// CheckFromReport).
//
// Verdict semantics:
//
//   - A layer whose data is absent from the input (no FlowPoint for symbol,
//     force missing from the slice, or DataAvailable=false) is SKIPPED with
//     an annotated reason (fail-open, 不可誤殺).
//   - The foreign layer fails when abs(ForeignNet 億股) <= min_abs_net
//     (strict >; threshold <= 0 disables).
//   - A market layer fails when abs(RawValue) <= min_abs_raw AND
//     abs(ZScore) <= min_abs_z (strict > for each enabled metric).
//   - Pass is true iff no enforced layer failed; skipped layers do not
//     affect the outcome. If EVERY enforced layer was skipped, AllSkipped is
//     true and Pass follows FailClosedWhenAllMissing.
//
// A nil receiver returns Pass=true (no gate configured → fail-open).
func (g *FlowGateway) Check(symbol string, conditionID string, points map[string]FlowPoint, forces []capitalflow.ForceScore) FlowVerdict {
	verdict := FlowVerdict{Symbol: symbol, ConditionID: conditionID}
	if g == nil {
		verdict.Pass = true
		return verdict
	}
	byForce := make(map[capitalflow.ForceName]capitalflow.ForceScore, len(forces))
	for _, f := range forces {
		byForce[f.Force] = f
	}
	layers := g.params.LayersFor(conditionID)
	pass := true
	skipped := 0
	for _, layer := range layers {
		switch layer {
		case FlowLayerForeign:
			point, ok := points[symbol]
			if !ok {
				if len(points) == 0 {
					// Data source entirely absent (e.g. pre-backfill): skip,
					// fail-open — a data gap must not kill a symbol.
					skipped++
					verdict.Layers = append(verdict.Layers, LayerVerdict{
						Layer:  layer,
						Status: LayerStatusSkip,
						Reason: fmt.Sprintf("%s層缺個股 flow 資料（無任何 FlowPoint 輸入），skip 不誤殺", layer.DisplayName()),
					})
					continue
				}
				// Data exists for OTHER symbols but not THIS one: the symbol
				// has no foreign backing — fail, exactly the "無外資背書"
				// noise this gate exists to filter.
				pass = false
				verdict.Layers = append(verdict.Layers, LayerVerdict{
					Layer:  layer,
					Status: LayerStatusFail,
					Reason: fmt.Sprintf("%s層不過: 無 %s 的個股外資 flow 資料（其他 symbol 有資料，本 symbol 無外資背書）", layer.DisplayName(), symbol),
				})
				continue
			}
			netYi := math.Abs(point.ForeignNet) / foreignNetToYiShares
			th := g.params.Foreign.MinAbsNet
			if th <= 0 || netYi > th {
				verdict.Layers = append(verdict.Layers, LayerVerdict{
					Layer:  layer,
					Status: LayerStatusPass,
					Reason: fmt.Sprintf("%s層通過（個股淨買超 %.2f 億股達標）", layer.DisplayName(), netYi),
				})
			} else {
				pass = false
				verdict.Layers = append(verdict.Layers, LayerVerdict{
					Layer:  layer,
					Status: LayerStatusFail,
					Reason: fmt.Sprintf("%s層不過: abs(個股淨買超)=%.2f 億股 ≤ min_abs_net=%.2f 億股（個股外資幅度不足）", layer.DisplayName(), netYi, th),
				})
			}
		case FlowLayerInstitutional, FlowLayerRetail:
			score, ok := byForce[forceNameForLayer(layer)]
			if !ok || !score.DataAvailable {
				skipped++
				verdict.Layers = append(verdict.Layers, LayerVerdict{
					Layer:  layer,
					Status: LayerStatusSkip,
					Reason: fmt.Sprintf("%s層缺資料（capitalflow data unavailable），skip 不誤殺", layer.DisplayName()),
				})
				continue
			}
			th := g.params.Layer(layer)
			if reason := layerFailReason(layer, score, th); reason != "" {
				pass = false
				verdict.Layers = append(verdict.Layers, LayerVerdict{
					Layer:  layer,
					Status: LayerStatusFail,
					Reason: reason,
				})
			} else {
				verdict.Layers = append(verdict.Layers, LayerVerdict{
					Layer:  layer,
					Status: LayerStatusPass,
					Reason: fmt.Sprintf("%s層通過（市場 regime 資金流幅度達標）", layer.DisplayName()),
				})
			}
		default:
			// Unknown layer names are rejected at config load
			// (config.validateFlowGateway); defensively skip any that still
			// reach the evaluator so it can never mis-fail a symbol.
			skipped++
			verdict.Layers = append(verdict.Layers, LayerVerdict{
				Layer:  layer,
				Status: LayerStatusSkip,
				Reason: fmt.Sprintf("未知層 %q，skip 不誤殺", layer),
			})
		}
	}
	verdict.SkippedCount = skipped
	verdict.AllSkipped = skipped == len(layers)
	if verdict.AllSkipped && g.params.FailClosedWhenAllMissing {
		pass = false
		verdict.Reason = fmt.Sprintf("全缺層 no-decision：%d 個強制層皆缺資料，fail-closed 不交易", skipped)
	}
	verdict.Pass = pass
	return verdict
}

// CheckFromReport evaluates the gateway against a symbol's per-symbol flow
// point and a capitalflow.DailyReport — the canonical read path output of
// capitalflow.Service.LatestDaily. A nil report evaluates with no market
// forces: market-regime layers are skipped and only the per-symbol foreign
// layer can pass (fail-open, 不可誤殺).
func (g *FlowGateway) CheckFromReport(symbol string, conditionID string, points map[string]FlowPoint, report *capitalflow.DailyReport) FlowVerdict {
	if report == nil {
		return g.Check(symbol, conditionID, points, nil)
	}
	return g.Check(symbol, conditionID, points, report.Forces)
}

// forceNameForLayer maps a gateway market-regime layer to its capitalflow
// dimension. The foreign layer is per-symbol (FlowPoint) and never uses this
// mapping; unknown layers map to the empty ForceName, which never matches a
// real reading → the layer is skipped.
func forceNameForLayer(l FlowLayer) capitalflow.ForceName {
	switch l {
	case FlowLayerInstitutional:
		return capitalflow.ForceInstitutional
	case FlowLayerRetail:
		return capitalflow.ForceRetail
	default:
		return capitalflow.ForceName("")
	}
}

// layerFailReason returns "" when the market-regime layer passes, or a
// Chinese reason identifying why it fails. A layer passes when
// abs(RawValue) > min_abs_raw OR abs(ZScore) > min_abs_z (strict >; a
// threshold <= 0 disables that metric); both at-or-below → fail ("外資小幅
// 賣超但勝率高" noise gate).
func layerFailReason(layer FlowLayer, score capitalflow.ForceScore, th LayerThreshold) string {
	rawOk := th.MinAbsRaw <= 0 || math.Abs(score.RawValue) > th.MinAbsRaw
	zOk := th.MinAbsZ <= 0 || math.Abs(score.ZScore) > th.MinAbsZ
	if rawOk || zOk {
		return ""
	}
	return fmt.Sprintf(
		"%s層不過: abs(淨值)=%.2f ≤ min_abs_raw=%.2f 且 abs(z-score)=%.2f ≤ min_abs_z=%.2f（資金流幅度不足）",
		layer.DisplayName(), math.Abs(score.RawValue), th.MinAbsRaw, math.Abs(score.ZScore), th.MinAbsZ)
}
