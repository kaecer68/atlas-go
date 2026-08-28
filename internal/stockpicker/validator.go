// Package validator.go — PR 2b three-layer capital-flow gateway.
//
// The stockpicker condition engine (conditions.go, PR 2a) decides WHEN a
// signal fires, but win rates built on "外資小幅賣超但勝率高" noise are not
// trustworthy. This file adds the gate in front of that: every candidate
// (symbol + condition) must sit on top of MEANINGFUL capital flow.
//
// The gate has three layers, each backed by one capitalflow ForceScore
// dimension (never guessed — read from internal/capitalflow/types.go and
// forces.go):
//
//	foreign       → capitalflow.ForceForeign       (外資, official_actor)
//	institutional → capitalflow.ForceInstitutional (投信, official_actor)
//	retail        → capitalflow.ForceRetail        (散戶, behavioral_proxy)
//
// Layer semantics (per the PR 2b contract): a layer passes when the force
// reading is meaningful in ABSOLUTE terms — abs(RawValue) above min_abs_raw
// (e.g. "abs 淨買超 > X 億") OR abs(ZScore) above min_abs_z ("z-score > Y").
// Direction is the condition's job, not the gate's: the gate only rejects
// magnitude-noise, exactly the "小幅賣超但勝率高" failure mode.
//
// Missing data is handled fail-open: when a layer's force is absent from the
// input or reports DataAvailable=false (capitalflow's "source channel empty",
// spec §8.3 / CF-INV-06), the layer is SKIPPED and annotated — never treated
// as a failure, so a data gap cannot silently kill a symbol ("缺層 skip 並註
// 記，不可誤殺").
//
// All thresholds live in configs/parameters.json → stockpicker.flow_gateway
// (mirrored by DefaultFlowGatewayParameters as the documented fallback for
// missing files, following the repo-wide parameters_defaults.go convention).
// The gate logic itself contains no magic numbers.
package stockpicker

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

// FlowLayer identifies one of the three gateway layers.
type FlowLayer string

const (
	// FlowLayerForeign is the 外資 layer (capitalflow.ForceForeign).
	FlowLayerForeign FlowLayer = "foreign"
	// FlowLayerInstitutional is the 投信 layer (capitalflow.ForceInstitutional).
	FlowLayerInstitutional FlowLayer = "institutional"
	// FlowLayerRetail is the 散戶 layer (capitalflow.ForceRetail).
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

// LayerThreshold holds the two numeric gates for one layer. A gate value
// <= 0 disables that metric (fail-open), so a layer with both gates at 0
// always passes whenever its data is available.
type LayerThreshold struct {
	// MinAbsRaw is the minimum absolute RawValue (units follow the
	// capitalflow provenance row: 億股 for foreign/institutional,
	// pct_composite for retail).
	MinAbsRaw float64
	// MinAbsZ is the minimum absolute ZScore (capitalflow 60-day rolling).
	MinAbsZ float64
}

// FlowGatewayParameters is the stockpicker.flow_gateway section of
// configs/parameters.json. ConditionLayers optionally narrows the enforced
// layer set per condition ID; a condition absent from the map enforces all
// three layers.
type FlowGatewayParameters struct {
	Foreign         LayerThreshold
	Institutional   LayerThreshold
	Retail          LayerThreshold
	ConditionLayers map[string][]FlowLayer
}

// Layer returns the threshold for a layer. Unknown layers get a zero
// threshold (both metrics disabled → pass whenever data is available).
func (p FlowGatewayParameters) Layer(l FlowLayer) LayerThreshold {
	switch l {
	case FlowLayerForeign:
		return p.Foreign
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
// The values mirror configs/parameters.json → stockpicker.flow_gateway and
// are used only when the file or section is absent (same convention as
// internal/config/parameters_defaults.go). The gate logic never reads these
// literals directly.
func DefaultFlowGatewayParameters() FlowGatewayParameters {
	return FlowGatewayParameters{
		Foreign: LayerThreshold{
			MinAbsRaw: 1.0, // 億股: meaningful market-wide foreign net buy
			MinAbsZ:   0.5, // capitalflow trendFor bullish/bearish boundary
		},
		Institutional: LayerThreshold{
			MinAbsRaw: 0.3, // 億股: 投信 daily magnitudes are smaller than 外資
			MinAbsZ:   0.5,
		},
		Retail: LayerThreshold{
			MinAbsRaw: 1.0, // pct_composite: >1pct margin+short move is meaningful
			MinAbsZ:   0.5,
		},
		ConditionLayers: map[string][]FlowLayer{},
	}
}

// LoadFlowGatewayParameters reads the stockpicker.flow_gateway section from
// configs/parameters.json. The path resolves from ATLAS_PARAMETERS_CONFIG_PATH,
// ATLAS_PARAMETERS_CONFIG, or a repo-root walk-up from the working directory
// (same default as internal/config). A missing file or section returns the
// documented defaults with no error; malformed JSON returns an error so
// callers can decide whether to fall back.
func LoadFlowGatewayParameters() (FlowGatewayParameters, error) {
	path := resolveParametersConfigPath()
	if path == "" {
		return DefaultFlowGatewayParameters(), nil
	}
	return loadFlowGatewayParameters(path)
}

// loadFlowGatewayParameters is the injectable loader used by tests.
func loadFlowGatewayParameters(path string) (FlowGatewayParameters, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultFlowGatewayParameters(), nil
		}
		return FlowGatewayParameters{}, fmt.Errorf("stockpicker: read flow_gateway config %s: %w", path, err)
	}
	var file struct {
		Stockpicker struct {
			FlowGateway *flowGatewayFileSection `json:"flow_gateway"`
		} `json:"stockpicker"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return FlowGatewayParameters{}, fmt.Errorf("stockpicker: parse flow_gateway config %s: %w", path, err)
	}
	if file.Stockpicker.FlowGateway == nil {
		return DefaultFlowGatewayParameters(), nil
	}
	return file.Stockpicker.FlowGateway.toParams(), nil
}

// resolveParametersConfigPath locates configs/parameters.json: env override
// first, then walk up from the working directory (tests and jobs run from
// the repo root; walking up covers nested invocation dirs). Returns "" when
// no file is found so callers fall back to defaults.
func resolveParametersConfigPath() string {
	for _, env := range []string{"ATLAS_PARAMETERS_CONFIG_PATH", "ATLAS_PARAMETERS_CONFIG"} {
		if p := os.Getenv(env); p != "" {
			return p
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return "configs/parameters.json"
	}
	for range 10 {
		candidate := filepath.Join(dir, "configs", "parameters.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// flowGatewayFileSection mirrors the JSON shape of
// stockpicker.flow_gateway (parameter blocks use the repo-wide
// {"value": ..., "rationale": ..., "source": ...} convention; rationale and
// source are documentation only and are intentionally not decoded).
type flowGatewayFileSection struct {
	Layers     map[string]flowLayerFile     `json:"layers"`
	Conditions map[string]flowConditionFile `json:"conditions,omitempty"`
}

type flowParamValue struct {
	Value float64 `json:"value"`
}

type flowLayerFile struct {
	MinAbsRaw flowParamValue `json:"min_abs_raw"`
	MinAbsZ   flowParamValue `json:"min_abs_z"`
}

type flowConditionFile struct {
	Layers []string `json:"layers"`
}

// toParams merges the file section over the documented defaults: a layer
// key present in the file replaces that layer's thresholds wholesale (a
// missing metric inside the block decodes to 0 = metric disabled); a
// condition key maps its declared layer set. Layers/conditions absent from
// the file keep the defaults.
func (s *flowGatewayFileSection) toParams() FlowGatewayParameters {
	p := DefaultFlowGatewayParameters()
	if l, ok := s.Layers["foreign"]; ok {
		p.Foreign = LayerThreshold{MinAbsRaw: l.MinAbsRaw.Value, MinAbsZ: l.MinAbsZ.Value}
	}
	if l, ok := s.Layers["institutional"]; ok {
		p.Institutional = LayerThreshold{MinAbsRaw: l.MinAbsRaw.Value, MinAbsZ: l.MinAbsZ.Value}
	}
	if l, ok := s.Layers["retail"]; ok {
		p.Retail = LayerThreshold{MinAbsRaw: l.MinAbsRaw.Value, MinAbsZ: l.MinAbsZ.Value}
	}
	for id, cond := range s.Conditions {
		if len(cond.Layers) == 0 {
			continue
		}
		layers := make([]FlowLayer, 0, len(cond.Layers))
		for _, ls := range cond.Layers {
			layers = append(layers, FlowLayer(ls))
		}
		p.ConditionLayers[id] = layers
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
	// annotated and ignored (fail-open, never kills the symbol).
	LayerStatusSkip LayerStatus = "skip"
)

// LayerVerdict is one layer's outcome inside a FlowVerdict.
type LayerVerdict struct {
	Layer  FlowLayer
	Status LayerStatus
	Reason string
}

// FlowVerdict is the result of a three-layer gateway check for one
// (symbol, condition) candidate.
type FlowVerdict struct {
	Symbol      string
	ConditionID string
	Pass        bool
	Layers      []LayerVerdict
}

// FlowGateway is the three-layer capital-flow gate. It is a pure
// evaluator: thresholds come from the parameters table, force readings come
// from the caller (typically capitalflow.Service.LatestDaily → DailyReport).
type FlowGateway struct {
	params FlowGatewayParameters
}

// NewFlowGateway builds a gateway with the given thresholds.
func NewFlowGateway(params FlowGatewayParameters) *FlowGateway {
	return &FlowGateway{params: params}
}

// NewDefaultFlowGateway builds a gateway with the loaded
// stockpicker.flow_gateway parameters (configs/parameters.json or the
// documented defaults).
func NewDefaultFlowGateway() (*FlowGateway, error) {
	params, err := LoadFlowGatewayParameters()
	if err != nil {
		return nil, err
	}
	return NewFlowGateway(params), nil
}

// Check evaluates the three-layer gateway for symbol + condition against
// the supplied capitalflow force readings. The forces are usually
// report.Forces from capitalflow.Service.LatestDaily (see CheckFromReport).
//
// Verdict semantics:
//
//   - A layer whose force is absent from the input or has DataAvailable=false
//     is SKIPPED with an annotated reason (fail-open, 不可誤殺).
//   - A layer whose data is available fails when abs(RawValue) <= min_abs_raw
//     AND abs(ZScore) <= min_abs_z (with either metric disabled when its
//     threshold is <= 0).
//   - Pass is true iff no enforced layer failed; skipped layers do not
//     affect the outcome.
//
// A nil receiver returns Pass=true (no gate configured → fail-open).
func (g *FlowGateway) Check(symbol string, conditionID string, forces []capitalflow.ForceScore) FlowVerdict {
	verdict := FlowVerdict{Symbol: symbol, ConditionID: conditionID}
	if g == nil {
		verdict.Pass = true
		return verdict
	}
	byForce := make(map[capitalflow.ForceName]capitalflow.ForceScore, len(forces))
	for _, f := range forces {
		byForce[f.Force] = f
	}
	pass := true
	for _, layer := range g.params.LayersFor(conditionID) {
		score, ok := byForce[forceNameForLayer(layer)]
		if !ok || !score.DataAvailable {
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
				Reason: fmt.Sprintf("%s層通過（資金流幅度達標）", layer.DisplayName()),
			})
		}
	}
	verdict.Pass = pass
	return verdict
}

// CheckFromReport evaluates the gateway against a capitalflow.DailyReport —
// the canonical read path output of capitalflow.Service.LatestDaily. A nil
// report evaluates with no forces: every enforced layer is skipped and the
// verdict is Pass=true (fail-open, 不可誤殺).
func (g *FlowGateway) CheckFromReport(symbol string, conditionID string, report *capitalflow.DailyReport) FlowVerdict {
	if report == nil {
		return g.Check(symbol, conditionID, nil)
	}
	return g.Check(symbol, conditionID, report.Forces)
}

// forceNameForLayer maps a gateway layer to its capitalflow dimension.
// Unknown layers map to the empty ForceName, which never matches a real
// reading → the layer is skipped.
func forceNameForLayer(l FlowLayer) capitalflow.ForceName {
	switch l {
	case FlowLayerForeign:
		return capitalflow.ForceForeign
	case FlowLayerInstitutional:
		return capitalflow.ForceInstitutional
	case FlowLayerRetail:
		return capitalflow.ForceRetail
	default:
		return capitalflow.ForceName("")
	}
}

// layerFailReason returns "" when the layer passes, or a Chinese reason
// identifying why the layer fails. A layer passes when abs(RawValue) >
// min_abs_raw OR abs(ZScore) > min_abs_z (a threshold <= 0 disables that
// metric); both below → fail ("外資小幅賣超但勝率高" noise gate).
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
