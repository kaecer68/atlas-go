// validator_test.go — PR 2b three-layer capital-flow gateway tests (TDD).
//
// Covers: all-three-pass, each layer failing (foreign / institutional /
// retail), missing-layer skip (fail-open, 不可誤殺), all-layers-missing
// skip, OR semantics (raw magnitude OR z-score), disabled thresholds,
// per-condition layer override, thresholds read from
// configs/parameters.json (not hard-coded), defaults fallback for missing
// file/section, malformed-JSON error, nil-gateway fail-open, and
// CheckFromReport delegation.
package stockpicker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

// flowScore builds one capitalflow.ForceScore fixture for a layer.
func flowScore(force capitalflow.ForceName, raw, z float64, avail bool) capitalflow.ForceScore {
	return capitalflow.ForceScore{
		Force:         force,
		RawValue:      raw,
		ZScore:        z,
		DataAvailable: avail,
	}
}

// threeLayerForces returns one reading per gateway layer.
func threeLayerForces(foreign, inst, retail capitalflow.ForceScore) []capitalflow.ForceScore {
	return []capitalflow.ForceScore{foreign, inst, retail}
}

// passFixture is a reading set where every layer clears the default
// thresholds (foreign 1.0/0.5, institutional 0.3/0.5, retail 1.0/0.5).
func passFixture() []capitalflow.ForceScore {
	return threeLayerForces(
		flowScore(capitalflow.ForceForeign, 2.0, 1.0, true),
		flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true),
		flowScore(capitalflow.ForceRetail, 1.5, 0.6, true),
	)
}

func layerVerdict(v FlowVerdict, layer FlowLayer) (LayerVerdict, bool) {
	for _, lv := range v.Layers {
		if lv.Layer == layer {
			return lv, true
		}
	}
	return LayerVerdict{}, false
}

func defaultGateway() *FlowGateway {
	return NewFlowGateway(DefaultFlowGatewayParameters())
}

// --- 1. all three layers pass ---

func TestFlowGateway_AllLayersPass(t *testing.T) {
	g := defaultGateway()
	v := g.Check("2330", string(ConditionForeign3DNetBuy), passFixture())

	if !v.Pass {
		t.Fatalf("Pass=false, want true: %+v", v)
	}
	if v.Symbol != "2330" || v.ConditionID != string(ConditionForeign3DNetBuy) {
		t.Errorf("verdict identity = %q/%q, want 2330/foreign-3d-net-buy", v.Symbol, v.ConditionID)
	}
	if len(v.Layers) != 3 {
		t.Fatalf("len(Layers)=%d, want 3: %+v", len(v.Layers), v.Layers)
	}
	for _, layer := range AllFlowLayers {
		lv, ok := layerVerdict(v, layer)
		if !ok {
			t.Fatalf("missing layer verdict for %s", layer)
		}
		if lv.Status != LayerStatusPass {
			t.Errorf("%s status=%s, want pass (%s)", layer, lv.Status, lv.Reason)
		}
	}
}

// --- 2. each layer fails independently ---

func TestFlowGateway_EachLayerFails(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(foreign, inst, retail capitalflow.ForceScore) (capitalflow.ForceScore, capitalflow.ForceScore, capitalflow.ForceScore)
		failLayer FlowLayer
		wantInMsg string
	}{
		{
			name: "foreign_below_raw_and_z",
			mutate: func(f, i, r capitalflow.ForceScore) (capitalflow.ForceScore, capitalflow.ForceScore, capitalflow.ForceScore) {
				f.RawValue, f.ZScore = 0.2, 0.1 // below 1.0 and 0.5
				return f, i, r
			},
			failLayer: FlowLayerForeign,
			wantInMsg: "外資層不過",
		},
		{
			name: "institutional_below_raw_and_z",
			mutate: func(f, i, r capitalflow.ForceScore) (capitalflow.ForceScore, capitalflow.ForceScore, capitalflow.ForceScore) {
				i.RawValue, i.ZScore = 0.1, 0.05 // below 0.3 and 0.5
				return f, i, r
			},
			failLayer: FlowLayerInstitutional,
			wantInMsg: "投信層不過",
		},
		{
			name: "retail_below_raw_and_z",
			mutate: func(f, i, r capitalflow.ForceScore) (capitalflow.ForceScore, capitalflow.ForceScore, capitalflow.ForceScore) {
				r.RawValue, r.ZScore = 0.3, 0.2 // below 1.0 and 0.5
				return f, i, r
			},
			failLayer: FlowLayerRetail,
			wantInMsg: "散戶層不過",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf := passFixture()
			f, i, r := tc.mutate(pf[0], pf[1], pf[2])
			v := defaultGateway().Check("2330", "c", threeLayerForces(f, i, r))

			if v.Pass {
				t.Fatalf("Pass=true, want false: %+v", v)
			}
			lv, ok := layerVerdict(v, tc.failLayer)
			if !ok {
				t.Fatalf("missing verdict for %s", tc.failLayer)
			}
			if lv.Status != LayerStatusFail {
				t.Errorf("%s status=%s, want fail", tc.failLayer, lv.Status)
			}
			if !strings.Contains(lv.Reason, tc.wantInMsg) {
				t.Errorf("reason %q missing %q", lv.Reason, tc.wantInMsg)
			}
			// The other two layers must still report pass.
			for _, layer := range AllFlowLayers {
				if layer == tc.failLayer {
					continue
				}
				if lv2, _ := layerVerdict(v, layer); lv2.Status != LayerStatusPass {
					t.Errorf("%s status=%s, want pass (other layers unaffected)", layer, lv2.Status)
				}
			}
		})
	}
}

// --- 3. missing layer → skip, never fail (不可誤殺) ---

func TestFlowGateway_MissingLayerSkip(t *testing.T) {
	foreign, inst, retail := passFixture()[0], passFixture()[1], passFixture()[2]
	foreign.DataAvailable = false // 外資 channel empty
	v := defaultGateway().Check("2330", "c", threeLayerForces(foreign, inst, retail))

	if !v.Pass {
		t.Fatalf("Pass=false, want true (missing layer must skip, not fail): %+v", v)
	}
	lv, ok := layerVerdict(v, FlowLayerForeign)
	if !ok {
		t.Fatal("missing foreign verdict")
	}
	if lv.Status != LayerStatusSkip {
		t.Errorf("foreign status=%s, want skip", lv.Status)
	}
	if !strings.Contains(lv.Reason, "缺資料") || !strings.Contains(lv.Reason, "skip") {
		t.Errorf("skip reason %q should annotate 缺資料 + skip", lv.Reason)
	}
	if lv2, _ := layerVerdict(v, FlowLayerInstitutional); lv2.Status != LayerStatusPass {
		t.Errorf("institutional status=%s, want pass", lv2.Status)
	}
	if lv2, _ := layerVerdict(v, FlowLayerRetail); lv2.Status != LayerStatusPass {
		t.Errorf("retail status=%s, want pass", lv2.Status)
	}
}

func TestFlowGateway_AllLayersMissingSkip(t *testing.T) {
	v := defaultGateway().Check("2330", "c", threeLayerForces(
		flowScore(capitalflow.ForceForeign, 0, 0, false),
		flowScore(capitalflow.ForceInstitutional, 0, 0, false),
		flowScore(capitalflow.ForceRetail, 0, 0, false),
	))
	if !v.Pass {
		t.Fatalf("Pass=false, want true (all layers missing → fail-open): %+v", v)
	}
	for _, layer := range AllFlowLayers {
		if lv, _ := layerVerdict(v, layer); lv.Status != LayerStatusSkip {
			t.Errorf("%s status=%s, want skip", layer, lv.Status)
		}
	}
}

func TestFlowGateway_ForceAbsentFromSliceIsSkip(t *testing.T) {
	// No foreign reading at all in the slice.
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)
	v := defaultGateway().Check("2330", "c", []capitalflow.ForceScore{inst, retail})

	if !v.Pass {
		t.Fatalf("Pass=false, want true (absent force → skip): %+v", v)
	}
	if lv, _ := layerVerdict(v, FlowLayerForeign); lv.Status != LayerStatusSkip {
		t.Errorf("foreign status=%s, want skip", lv.Status)
	}
}

func TestFlowGateway_FailAndSkipMixed(t *testing.T) {
	// Foreign fails AND retail is missing: verdict must fail on foreign
	// while retail is annotated as skip (not fail).
	foreign := flowScore(capitalflow.ForceForeign, 0.2, 0.1, true)
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 0, 0, false)
	v := defaultGateway().Check("2330", "c", threeLayerForces(foreign, inst, retail))

	if v.Pass {
		t.Fatal("Pass=true, want false (foreign layer fails)")
	}
	if lv, _ := layerVerdict(v, FlowLayerForeign); lv.Status != LayerStatusFail {
		t.Errorf("foreign status=%s, want fail", lv.Status)
	}
	if lv, _ := layerVerdict(v, FlowLayerRetail); lv.Status != LayerStatusSkip {
		t.Errorf("retail status=%s, want skip (missing data must not be counted as failure)", lv.Status)
	}
}

// --- 4. OR semantics: raw magnitude OR z-score clears a layer ---

func TestFlowGateway_OrSemantics(t *testing.T) {
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)

	t.Run("raw_meaningful_z_zero_passes", func(t *testing.T) {
		foreign := flowScore(capitalflow.ForceForeign, 5.0, 0.0, true) // raw > 1.0
		v := defaultGateway().Check("2330", "c", threeLayerForces(foreign, inst, retail))
		if !v.Pass {
			t.Fatalf("Pass=false, want true (raw magnitude alone must pass): %+v", v)
		}
	})
	t.Run("z_meaningful_raw_small_passes", func(t *testing.T) {
		foreign := flowScore(capitalflow.ForceForeign, 0.2, 2.0, true) // z > 0.5
		v := defaultGateway().Check("2330", "c", threeLayerForces(foreign, inst, retail))
		if !v.Pass {
			t.Fatalf("Pass=false, want true (z-score alone must pass): %+v", v)
		}
	})
	t.Run("both_below_fails", func(t *testing.T) {
		foreign := flowScore(capitalflow.ForceForeign, 0.2, 0.1, true) // neither
		v := defaultGateway().Check("2330", "c", threeLayerForces(foreign, inst, retail))
		if v.Pass {
			t.Fatalf("Pass=true, want false (raw and z both below): %+v", v)
		}
	})
}

// --- 5. disabled thresholds (<= 0) make the layer fail-open ---

func TestFlowGateway_ThresholdDisabled(t *testing.T) {
	params := DefaultFlowGatewayParameters()
	params.Foreign = LayerThreshold{} // both metrics disabled
	g := NewFlowGateway(params)

	foreign := flowScore(capitalflow.ForceForeign, 0.2, 0.1, true) // tiny reading
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)
	v := g.Check("2330", "c", threeLayerForces(foreign, inst, retail))

	if !v.Pass {
		t.Fatalf("Pass=false, want true (disabled thresholds must not reject): %+v", v)
	}
	if lv, _ := layerVerdict(v, FlowLayerForeign); lv.Status != LayerStatusPass {
		t.Errorf("foreign status=%s, want pass", lv.Status)
	}
}

// --- 6. per-condition layer override ---

func TestFlowGateway_ConditionLayerOverride(t *testing.T) {
	params := DefaultFlowGatewayParameters()
	params.ConditionLayers["momentum-20d-positive"] = []FlowLayer{FlowLayerForeign}
	g := NewFlowGateway(params)

	foreign := flowScore(capitalflow.ForceForeign, 2.0, 1.0, true)
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 0.3, 0.2, true) // would fail retail layer

	// Condition with override: only foreign enforced → passes.
	v := g.Check("2330", "momentum-20d-positive", threeLayerForces(foreign, inst, retail))
	if !v.Pass {
		t.Fatalf("Pass=false, want true (override narrows to foreign): %+v", v)
	}
	if len(v.Layers) != 1 || v.Layers[0].Layer != FlowLayerForeign {
		t.Errorf("Layers=%+v, want exactly [foreign]", v.Layers)
	}

	// Unknown condition: all three enforced → retail fails.
	v2 := g.Check("2330", "some-other-condition", threeLayerForces(foreign, inst, retail))
	if v2.Pass {
		t.Fatalf("Pass=true, want false (unlisted condition enforces all layers): %+v", v2)
	}
	if len(v2.Layers) != 3 {
		t.Errorf("len(Layers)=%d, want 3 for unlisted condition", len(v2.Layers))
	}
}

// --- 7. thresholds are configurable, not hard-coded ---

func TestFlowGateway_ThresholdsAreConfigurable(t *testing.T) {
	// Same fixture passes with defaults but must fail when thresholds are
	// raised — proving the gate reads the parameter table.
	strict := DefaultFlowGatewayParameters()
	strict.Foreign = LayerThreshold{MinAbsRaw: 100, MinAbsZ: 50}
	g := NewFlowGateway(strict)

	if v := defaultGateway().Check("2330", "c", passFixture()); !v.Pass {
		t.Fatalf("default thresholds must pass the pass fixture: %+v", v)
	}
	v := g.Check("2330", "c", passFixture())
	if v.Pass {
		t.Fatal("Pass=true, want false (raised thresholds must reject the same fixture)")
	}
	if lv, _ := layerVerdict(v, FlowLayerForeign); lv.Status != LayerStatusFail {
		t.Errorf("foreign status=%s, want fail under strict thresholds", lv.Status)
	}
}

// --- 8. parameters from configs/parameters.json ---

func TestFlowGateway_ParamsFromConfigFile(t *testing.T) {
	params, err := LoadFlowGatewayParameters()
	if err != nil {
		t.Fatalf("LoadFlowGatewayParameters: %v", err)
	}
	if params.Foreign.MinAbsRaw != 1.0 {
		t.Errorf("foreign.min_abs_raw=%v, want 1.0 (from parameters.json)", params.Foreign.MinAbsRaw)
	}
	if params.Foreign.MinAbsZ != 0.5 {
		t.Errorf("foreign.min_abs_z=%v, want 0.5 (from parameters.json)", params.Foreign.MinAbsZ)
	}
	if params.Institutional.MinAbsRaw != 0.3 {
		t.Errorf("institutional.min_abs_raw=%v, want 0.3 (from parameters.json)", params.Institutional.MinAbsRaw)
	}
	if params.Institutional.MinAbsZ != 0.5 {
		t.Errorf("institutional.min_abs_z=%v, want 0.5 (from parameters.json)", params.Institutional.MinAbsZ)
	}
	if params.Retail.MinAbsRaw != 1.0 {
		t.Errorf("retail.min_abs_raw=%v, want 1.0 (from parameters.json)", params.Retail.MinAbsRaw)
	}
	if params.Retail.MinAbsZ != 0.5 {
		t.Errorf("retail.min_abs_z=%v, want 0.5 (from parameters.json)", params.Retail.MinAbsZ)
	}
	ls, ok := params.ConditionLayers[string(ConditionForeign3DNetBuy)]
	if !ok {
		t.Fatalf("flow_gateway.conditions missing %q", ConditionForeign3DNetBuy)
	}
	if len(ls) != 3 || ls[0] != FlowLayerForeign || ls[1] != FlowLayerInstitutional || ls[2] != FlowLayerRetail {
		t.Errorf("foreign-3d-net-buy layers=%v, want [foreign institutional retail]", ls)
	}
}

// --- 9. defaults fallback + malformed file ---

func TestFlowGateway_DefaultsWhenFileMissing(t *testing.T) {
	params, err := loadFlowGatewayParameters(filepath.Join(t.TempDir(), "nope", "parameters.json"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	def := DefaultFlowGatewayParameters()
	if params.Foreign != def.Foreign || params.Institutional != def.Institutional || params.Retail != def.Retail {
		t.Errorf("missing file must return defaults, got %+v", params)
	}
}

func TestFlowGateway_DefaultsWhenSectionMissing(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "parameters.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.0","stockpicker":{"costs":{"round_trip_pct":{"value":0.00585}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	params, err := loadFlowGatewayParameters(path)
	if err != nil {
		t.Fatalf("section missing must not error: %v", err)
	}
	def := DefaultFlowGatewayParameters()
	if params.Foreign != def.Foreign || params.Institutional != def.Institutional || params.Retail != def.Retail {
		t.Errorf("missing section must return defaults, got %+v", params)
	}
}

func TestFlowGateway_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "parameters.json")
	if err := os.WriteFile(path, []byte(`{"stockpicker": {`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFlowGatewayParameters(path); err == nil {
		t.Fatal("malformed JSON must return an error")
	}
}

func TestFlowGateway_NewDefaultGateway(t *testing.T) {
	g, err := NewDefaultFlowGateway()
	if err != nil {
		t.Fatalf("NewDefaultFlowGateway: %v", err)
	}
	v := g.Check("2330", string(ConditionForeign3DNetBuy), passFixture())
	if !v.Pass {
		t.Fatalf("Pass=false with default gateway + pass fixture: %+v", v)
	}
}

// --- 10. nil gateway → fail-open ---

func TestFlowGateway_NilGateway(t *testing.T) {
	var g *FlowGateway
	v := g.Check("2330", "c", nil)
	if !v.Pass {
		t.Fatalf("nil gateway must fail open (Pass=true), got %+v", v)
	}
	if v.Symbol != "2330" || v.ConditionID != "c" {
		t.Errorf("nil gateway verdict identity lost: %+v", v)
	}
}

// --- 11. CheckFromReport delegation ---

func TestFlowGateway_CheckFromReport(t *testing.T) {
	g := defaultGateway()
	report := &capitalflow.DailyReport{Forces: passFixture()}
	v := g.CheckFromReport("2330", "c", report)
	if !v.Pass {
		t.Fatalf("Pass=false, want true: %+v", v)
	}

	nilReport := g.CheckFromReport("2330", "c", nil)
	if !nilReport.Pass {
		t.Fatalf("nil report must fail open (all layers skipped): %+v", nilReport)
	}
	for _, layer := range AllFlowLayers {
		if lv, _ := layerVerdict(nilReport, layer); lv.Status != LayerStatusSkip {
			t.Errorf("%s status=%s, want skip for nil report", layer, lv.Status)
		}
	}
}
