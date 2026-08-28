// validator_test.go — PR 2b two-level capital-flow gateway tests (TDD).
//
// Covers: all-layers-pass, each layer failing (foreign / institutional /
// retail), per-symbol foreign unit conversion (千股 → 億股 ÷1e5), strict->
// equality boundary (== threshold fails), missing-layer skip (fail-open,
// 不可誤殺), all-layers-missing skip vs fail-closed (AllSkipped /
// SkippedCount / fail_closed_when_all_missing), OR semantics (raw magnitude
// OR z-score), disabled thresholds, per-condition layer override (real
// narrowing), thresholds from a hermetic config fixture (not the real
// configs/parameters.json), defaults fallback for missing section,
// malformed-JSON error, layer-name typo → load error, nil-gateway fail-open,
// and CheckFromReport delegation.
package stockpicker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
)

// --- fixtures ---

// flowScore builds one capitalflow.ForceScore fixture for a layer.
func flowScore(force capitalflow.ForceName, raw, z float64, avail bool) capitalflow.ForceScore {
	return capitalflow.ForceScore{
		Force:         force,
		RawValue:      raw,
		ZScore:        z,
		DataAvailable: avail,
	}
}

// marketPassForces returns the market-regime readings (institutional +
// retail) that clear the default thresholds (institutional 0.3/0.5, retail
// 1.0/0.5). The foreign layer is per-symbol and comes from points, not here.
func marketPassForces() []capitalflow.ForceScore {
	return []capitalflow.ForceScore{
		flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true),
		flowScore(capitalflow.ForceRetail, 1.5, 0.6, true),
	}
}

// symbolPoint returns a per-symbol FlowPoint map for symbol with the given
// ForeignNet (千股 = shares/1e3; 億股 = ForeignNet / 1e5).
func symbolPoint(symbol string, foreignNet float64) map[string]FlowPoint {
	return map[string]FlowPoint{symbol: {ForeignNet: foreignNet}}
}

// passPoint is a ForeignNet that clears the default min_abs_net (0.1 億股):
// 2e4 千股 = 0.2 億股.
func passPoint() map[string]FlowPoint { return symbolPoint("2330", 2e4) }

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

// writeFixture writes body to a temp parameters.json and returns its path.
func writeFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// loadFlowGatewayFixture loads body through the canonical config pipeline
// (unmarshal → mergeAllDefaults → Validate) and returns the loaded config.
// Hermetic: never reads the real configs/parameters.json.
func loadFlowGatewayFixture(t *testing.T, body string) *config.ParametersConfig {
	t.Helper()
	cfg, err := config.LoadParametersConfig(writeFixture(t, body))
	if err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	return cfg
}

// validFlowGatewayFixture mirrors the real flow_gateway section shape with
// the PR 2b two-level design (per-symbol foreign min_abs_net + market regime
// layers + real momentum narrowing).
const validFlowGatewayFixture = `{
  "version": "1.0",
  "stockpicker": {
    "flow_gateway": {
      "fail_closed_when_all_missing": {"value": true, "rationale": "r", "source": "heuristic"},
      "layers": {
        "foreign": {"min_abs_net": {"value": 0.1, "rationale": "r", "source": "heuristic"}},
        "institutional": {"min_abs_raw": {"value": 0.3, "rationale": "r", "source": "heuristic"}, "min_abs_z": {"value": 0.5, "rationale": "r", "source": "heuristic"}},
        "retail": {"min_abs_raw": {"value": 1.0, "rationale": "r", "source": "heuristic"}, "min_abs_z": {"value": 0.5, "rationale": "r", "source": "heuristic"}}
      },
      "conditions": {
        "foreign-3d-net-buy": {"layers": {"value": ["foreign", "institutional", "retail"], "rationale": "r", "source": "heuristic"}},
        "momentum-20d-positive": {"layers": {"value": ["foreign", "institutional"], "rationale": "r", "source": "heuristic"}}
      }
    }
  }
}`

// --- 1. all layers pass ---

func TestFlowGateway_AllLayersPass(t *testing.T) {
	g := defaultGateway()
	v := g.Check("2330", string(ConditionForeign3DNetBuy), passPoint(), marketPassForces())

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
	if v.AllSkipped || v.SkippedCount != 0 {
		t.Errorf("AllSkipped=%v SkippedCount=%d, want false/0", v.AllSkipped, v.SkippedCount)
	}
}

// --- 2. per-symbol foreign unit conversion (千股 → 億股 ÷1e5) ---

func TestFlowGateway_ForeignUnitConversion(t *testing.T) {
	// FlowPoint.ForeignNet is 千股 (shares/1e3); min_abs_net is 億股.
	// 億股 = ForeignNet / 1e5: 2e4 千股 = 0.2 億股 > 0.1 → pass;
	// 5e3 千股 = 0.05 億股 < 0.1 → fail.
	g := defaultGateway()
	forces := marketPassForces()

	if v := g.Check("2330", "c", symbolPoint("2330", 2e4), forces); !v.Pass {
		t.Fatalf("2e4 千股 (=0.2 億股) must pass min_abs_net=0.1 億股: %+v", v)
	}
	if v := g.Check("2330", "c", symbolPoint("2330", 5e3), forces); v.Pass {
		t.Fatal("5e3 千股 (=0.05 億股) must fail min_abs_net=0.1 億股")
	}
	// The symbol argument is a real query key: a point under another symbol
	// must NOT satisfy the foreign layer for 2330.
	if v := g.Check("2330", "c", symbolPoint("0050", 2e4), forces); v.Pass {
		t.Fatal("foreign flow of another symbol must not satisfy 2330's foreign layer")
	}
}

// --- 3. each layer fails independently ---

func TestFlowGateway_EachLayerFails(t *testing.T) {
	cases := []struct {
		name      string
		point     map[string]FlowPoint
		forces    []capitalflow.ForceScore
		failLayer FlowLayer
		wantInMsg string
	}{
		{
			name:      "foreign_below_min_abs_net",
			point:     symbolPoint("2330", 5e3), // 0.05 億股 < 0.1
			forces:    marketPassForces(),
			failLayer: FlowLayerForeign,
			wantInMsg: "外資層不過",
		},
		{
			name:  "institutional_below_raw_and_z",
			point: passPoint(),
			forces: []capitalflow.ForceScore{
				flowScore(capitalflow.ForceInstitutional, 0.1, 0.05, true), // below 0.3 and 0.5
				flowScore(capitalflow.ForceRetail, 1.5, 0.6, true),
			},
			failLayer: FlowLayerInstitutional,
			wantInMsg: "投信層不過",
		},
		{
			name:  "retail_below_raw_and_z",
			point: passPoint(),
			forces: []capitalflow.ForceScore{
				flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true),
				flowScore(capitalflow.ForceRetail, 0.3, 0.2, true), // below 1.0 and 0.5
			},
			failLayer: FlowLayerRetail,
			wantInMsg: "散戶層不過",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := defaultGateway().Check("2330", "c", tc.point, tc.forces)

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
			// The other layers must still report pass.
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

// --- 4. strict > boundary: value == threshold must fail ---

func TestFlowGateway_EqualThresholdFails(t *testing.T) {
	g := defaultGateway()
	point := passPoint()

	t.Run("foreign_net_equal_min_abs_net", func(t *testing.T) {
		// 1e4 千股 = 0.1 億股 == min_abs_net → strict > → fail.
		v := g.Check("2330", "c", symbolPoint("2330", 1e4), marketPassForces())
		if v.Pass {
			t.Fatal("ForeignNet == min_abs_net must fail (strict > semantics)")
		}
	})
	t.Run("market_raw_equal_min_abs_raw", func(t *testing.T) {
		inst := flowScore(capitalflow.ForceInstitutional, 0.3, 0.0, true) // raw == 0.3, z == 0
		retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)
		v := g.Check("2330", "c", point, []capitalflow.ForceScore{inst, retail})
		if v.Pass {
			t.Fatal("raw == min_abs_raw must fail (strict > semantics)")
		}
	})
	t.Run("market_z_equal_min_abs_z", func(t *testing.T) {
		inst := flowScore(capitalflow.ForceInstitutional, 0.0, 0.5, true) // raw == 0, z == 0.5
		retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)
		v := g.Check("2330", "c", point, []capitalflow.ForceScore{inst, retail})
		if v.Pass {
			t.Fatal("z == min_abs_z must fail (strict > semantics)")
		}
	})
}

// --- 5. missing layer → skip, never fail (不可誤殺) ---

func TestFlowGateway_MissingLayerSkip(t *testing.T) {
	// No FlowPoint for the symbol → foreign skipped; market layers pass.
	v := defaultGateway().Check("2330", "c", nil, marketPassForces())

	if !v.Pass {
		t.Fatalf("Pass=false, want true (missing layer must skip, not fail): %+v", v)
	}
	if v.AllSkipped || v.SkippedCount != 1 {
		t.Errorf("AllSkipped=%v SkippedCount=%d, want false/1", v.AllSkipped, v.SkippedCount)
	}
	lv, ok := layerVerdict(v, FlowLayerForeign)
	if !ok {
		t.Fatal("missing foreign verdict")
	}
	if lv.Status != LayerStatusSkip {
		t.Errorf("foreign status=%s, want skip", lv.Status)
	}
	if !strings.Contains(lv.Reason, "缺個股 flow 資料") || !strings.Contains(lv.Reason, "skip") {
		t.Errorf("skip reason %q should annotate 缺個股 flow 資料 + skip", lv.Reason)
	}
	if lv2, _ := layerVerdict(v, FlowLayerInstitutional); lv2.Status != LayerStatusPass {
		t.Errorf("institutional status=%s, want pass", lv2.Status)
	}
	if lv2, _ := layerVerdict(v, FlowLayerRetail); lv2.Status != LayerStatusPass {
		t.Errorf("retail status=%s, want pass", lv2.Status)
	}
}

func TestFlowGateway_ForceAbsentFromSliceIsSkip(t *testing.T) {
	// Institutional reading absent entirely from the slice; foreign passes.
	retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)
	v := defaultGateway().Check("2330", "c", passPoint(), []capitalflow.ForceScore{retail})

	if !v.Pass {
		t.Fatalf("Pass=false, want true (absent force → skip): %+v", v)
	}
	if v.SkippedCount != 1 {
		t.Errorf("SkippedCount=%d, want 1", v.SkippedCount)
	}
	if lv, _ := layerVerdict(v, FlowLayerInstitutional); lv.Status != LayerStatusSkip {
		t.Errorf("institutional status=%s, want skip", lv.Status)
	}
}

func TestFlowGateway_FailAndSkipMixed(t *testing.T) {
	// Foreign fails AND retail is missing: verdict must fail on foreign
	// while retail is annotated as skip (not fail).
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 0, 0, false)
	v := defaultGateway().Check("2330", "c", symbolPoint("2330", 5e3), []capitalflow.ForceScore{inst, retail})

	if v.Pass {
		t.Fatal("Pass=true, want false (foreign layer fails)")
	}
	if v.AllSkipped {
		t.Error("AllSkipped=true, want false (foreign + institutional have data)")
	}
	if lv, _ := layerVerdict(v, FlowLayerForeign); lv.Status != LayerStatusFail {
		t.Errorf("foreign status=%s, want fail", lv.Status)
	}
	if lv, _ := layerVerdict(v, FlowLayerRetail); lv.Status != LayerStatusSkip {
		t.Errorf("retail status=%s, want skip (missing data must not be counted as failure)", lv.Status)
	}
}

// --- 6. all layers missing: fail-open (flag false) vs fail-closed (flag true) ---

func TestFlowGateway_AllLayersMissingSkip(t *testing.T) {
	// Fail-open mode (backtest): all skipped → Pass=true + AllSkipped=true.
	params := DefaultFlowGatewayParameters()
	params.FailClosedWhenAllMissing = false
	g := NewFlowGateway(params)

	v := g.Check("2330", "c", nil, nil)
	if !v.Pass {
		t.Fatalf("Pass=false, want true (all layers missing + fail-open): %+v", v)
	}
	if !v.AllSkipped {
		t.Error("AllSkipped=false, want true")
	}
	if v.SkippedCount != len(AllFlowLayers) {
		t.Errorf("SkippedCount=%d, want %d", v.SkippedCount, len(AllFlowLayers))
	}
	for _, layer := range AllFlowLayers {
		if lv, _ := layerVerdict(v, layer); lv.Status != LayerStatusSkip {
			t.Errorf("%s status=%s, want skip", layer, lv.Status)
		}
	}
}

func TestFlowGateway_AllLayersMissingFailClosed(t *testing.T) {
	// Default (live path, flag true): all skipped → Pass=false (no-decision)
	// + AllSkipped=true.
	g := defaultGateway()

	v := g.Check("2330", "c", nil, nil)
	if v.Pass {
		t.Fatalf("Pass=true, want false (all layers missing must fail closed): %+v", v)
	}
	if !v.AllSkipped {
		t.Error("AllSkipped=false, want true")
	}
	if v.SkippedCount != len(AllFlowLayers) {
		t.Errorf("SkippedCount=%d, want %d", v.SkippedCount, len(AllFlowLayers))
	}
	if !strings.Contains(v.Reason, "no-decision") {
		t.Errorf("Reason %q should annotate no-decision", v.Reason)
	}
}

func TestFlowGateway_FailClosedWithPartialData(t *testing.T) {
	// Foreign passes, both market forces missing → not all skipped → pass
	// (fail-closed only fires when EVERY enforced layer is missing).
	g := defaultGateway()
	v := g.Check("2330", "c", passPoint(), nil)

	if !v.Pass {
		t.Fatalf("Pass=false, want true (foreign data present, market layers skip): %+v", v)
	}
	if v.AllSkipped {
		t.Error("AllSkipped=true, want false")
	}
	if v.SkippedCount != 2 {
		t.Errorf("SkippedCount=%d, want 2", v.SkippedCount)
	}
	for _, layer := range []FlowLayer{FlowLayerInstitutional, FlowLayerRetail} {
		if lv, _ := layerVerdict(v, layer); lv.Status != LayerStatusSkip {
			t.Errorf("%s status=%s, want skip", layer, lv.Status)
		}
	}
}

// --- 7. OR semantics: raw magnitude OR z-score clears a market layer ---

func TestFlowGateway_OrSemantics(t *testing.T) {
	point := passPoint()
	retail := flowScore(capitalflow.ForceRetail, 1.5, 0.6, true)

	t.Run("raw_meaningful_z_zero_passes", func(t *testing.T) {
		inst := flowScore(capitalflow.ForceInstitutional, 5.0, 0.0, true) // raw > 0.3
		v := defaultGateway().Check("2330", "c", point, []capitalflow.ForceScore{inst, retail})
		if !v.Pass {
			t.Fatalf("Pass=false, want true (raw magnitude alone must pass): %+v", v)
		}
	})
	t.Run("z_meaningful_raw_small_passes", func(t *testing.T) {
		inst := flowScore(capitalflow.ForceInstitutional, 0.2, 2.0, true) // z > 0.5
		v := defaultGateway().Check("2330", "c", point, []capitalflow.ForceScore{inst, retail})
		if !v.Pass {
			t.Fatalf("Pass=false, want true (z-score alone must pass): %+v", v)
		}
	})
	t.Run("both_below_fails", func(t *testing.T) {
		inst := flowScore(capitalflow.ForceInstitutional, 0.2, 0.1, true) // neither
		v := defaultGateway().Check("2330", "c", point, []capitalflow.ForceScore{inst, retail})
		if v.Pass {
			t.Fatalf("Pass=true, want false (raw and z both below): %+v", v)
		}
	})
}

// --- 8. disabled thresholds (<= 0) make a layer fail-open ---

func TestFlowGateway_ThresholdDisabled(t *testing.T) {
	params := DefaultFlowGatewayParameters()
	params.Foreign = ForeignThreshold{}     // per-symbol gate disabled
	params.Institutional = LayerThreshold{} // both metrics disabled
	params.Retail = LayerThreshold{}        // both metrics disabled
	g := NewFlowGateway(params)

	// Tiny readings everywhere: disabled thresholds must not reject.
	inst := flowScore(capitalflow.ForceInstitutional, 0.01, 0.01, true)
	retail := flowScore(capitalflow.ForceRetail, 0.01, 0.01, true)
	v := g.Check("2330", "c", symbolPoint("2330", 5e3), []capitalflow.ForceScore{inst, retail})

	if !v.Pass {
		t.Fatalf("Pass=false, want true (disabled thresholds must not reject): %+v", v)
	}
	for _, layer := range AllFlowLayers {
		if lv, _ := layerVerdict(v, layer); lv.Status != LayerStatusPass {
			t.Errorf("%s status=%s, want pass", layer, lv.Status)
		}
	}
}

// --- 9. per-condition layer override ---

func TestFlowGateway_ConditionLayerOverride(t *testing.T) {
	params := DefaultFlowGatewayParameters()
	params.ConditionLayers["momentum-20d-positive"] = []FlowLayer{FlowLayerForeign}
	g := NewFlowGateway(params)

	point := passPoint()
	inst := flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true)
	retail := flowScore(capitalflow.ForceRetail, 0.3, 0.2, true) // would fail retail layer
	forces := []capitalflow.ForceScore{inst, retail}

	// Condition with override: only foreign enforced → passes even with a
	// failing retail reading.
	v := g.Check("2330", "momentum-20d-positive", point, forces)
	if !v.Pass {
		t.Fatalf("Pass=false, want true (override narrows to foreign): %+v", v)
	}
	if len(v.Layers) != 1 || v.Layers[0].Layer != FlowLayerForeign {
		t.Errorf("Layers=%+v, want exactly [foreign]", v.Layers)
	}

	// Unknown condition: all three enforced → retail fails.
	v2 := g.Check("2330", "some-other-condition", point, forces)
	if v2.Pass {
		t.Fatalf("Pass=true, want false (unlisted condition enforces all layers): %+v", v2)
	}
	if len(v2.Layers) != 3 {
		t.Errorf("len(Layers)=%d, want 3 for unlisted condition", len(v2.Layers))
	}
}

// --- 10. thresholds are configurable, not hard-coded ---

func TestFlowGateway_ThresholdsAreConfigurable(t *testing.T) {
	// Same fixture passes with defaults but must fail when thresholds are
	// raised — proving the gate reads the parameter table.
	strict := DefaultFlowGatewayParameters()
	strict.Foreign = ForeignThreshold{MinAbsNet: 100} // 100 億股 per symbol
	strict.Institutional = LayerThreshold{MinAbsRaw: 100, MinAbsZ: 50}
	g := NewFlowGateway(strict)

	if v := defaultGateway().Check("2330", "c", passPoint(), marketPassForces()); !v.Pass {
		t.Fatalf("default thresholds must pass the pass fixture: %+v", v)
	}
	v := g.Check("2330", "c", passPoint(), marketPassForces())
	if v.Pass {
		t.Fatal("Pass=true, want false (raised thresholds must reject the same fixture)")
	}
	if lv, _ := layerVerdict(v, FlowLayerForeign); lv.Status != LayerStatusFail {
		t.Errorf("foreign status=%s, want fail under strict thresholds", lv.Status)
	}
}

// --- 11. parameters from a hermetic config fixture ---

func TestFlowGateway_ParamsFromConfigFile(t *testing.T) {
	cfg := loadFlowGatewayFixture(t, validFlowGatewayFixture)
	p := flowGatewayParamsFromConfig(cfg.Stockpicker.FlowGateway)

	if p.Foreign.MinAbsNet != 0.1 {
		t.Errorf("foreign.min_abs_net=%v, want 0.1 (from fixture)", p.Foreign.MinAbsNet)
	}
	if p.Institutional.MinAbsRaw != 0.3 || p.Institutional.MinAbsZ != 0.5 {
		t.Errorf("institutional=%+v, want raw 0.3 / z 0.5 (from fixture)", p.Institutional)
	}
	if p.Retail.MinAbsRaw != 1.0 || p.Retail.MinAbsZ != 0.5 {
		t.Errorf("retail=%+v, want raw 1.0 / z 0.5 (from fixture)", p.Retail)
	}
	if !p.FailClosedWhenAllMissing {
		t.Error("fail_closed_when_all_missing=false, want true (from fixture)")
	}
	ls, ok := p.ConditionLayers[string(ConditionForeign3DNetBuy)]
	if !ok {
		t.Fatalf("flow_gateway.conditions missing %q", ConditionForeign3DNetBuy)
	}
	if len(ls) != 3 || ls[0] != FlowLayerForeign || ls[1] != FlowLayerInstitutional || ls[2] != FlowLayerRetail {
		t.Errorf("foreign-3d-net-buy layers=%v, want [foreign institutional retail]", ls)
	}
	// Real narrowing: momentum does NOT enforce the retail regime layer.
	mom, ok := p.ConditionLayers[string(ConditionMomentum20D)]
	if !ok {
		t.Fatalf("flow_gateway.conditions missing %q", ConditionMomentum20D)
	}
	if len(mom) != 2 || mom[0] != FlowLayerForeign || mom[1] != FlowLayerInstitutional {
		t.Errorf("momentum-20d-positive layers=%v, want [foreign institutional] (real narrowing)", mom)
	}
}

// --- 12. defaults fallback + malformed file + layer-name validation ---

func TestFlowGateway_DefaultsWhenSectionMissing(t *testing.T) {
	// Section absent → mergeStockpickerDefaults fills the documented
	// two-level gate defaults.
	body := `{"version":"1.0","stockpicker":{"costs":{"round_trip_pct":{"value":0.00585,"rationale":"r","source":"heuristic"}}}}`
	cfg := loadFlowGatewayFixture(t, body)
	p := flowGatewayParamsFromConfig(cfg.Stockpicker.FlowGateway)

	def := DefaultFlowGatewayParameters()
	if p.Foreign != def.Foreign || p.Institutional != def.Institutional || p.Retail != def.Retail {
		t.Errorf("missing section must return defaults, got %+v", p)
	}
	if p.FailClosedWhenAllMissing != def.FailClosedWhenAllMissing {
		t.Errorf("FailClosedWhenAllMissing=%v, want default %v", p.FailClosedWhenAllMissing, def.FailClosedWhenAllMissing)
	}
	if ls := p.ConditionLayers[string(ConditionMomentum20D)]; len(ls) != 2 {
		t.Errorf("momentum layers after default merge=%v, want 2 (foreign+institutional)", ls)
	}
}

func TestFlowGateway_InvalidJSON(t *testing.T) {
	path := writeFixture(t, `{"stockpicker": {`)
	if _, err := config.LoadParametersConfig(path); err == nil {
		t.Fatal("malformed JSON must return an error")
	}
}

func TestFlowGateway_UnknownLayerNameFailsLoad(t *testing.T) {
	// "foregin" typo must be a config-load error, not a silent skip.
	body := `{
  "version": "1.0",
  "stockpicker": {
    "flow_gateway": {
      "fail_closed_when_all_missing": {"value": true, "rationale": "r", "source": "heuristic"},
      "layers": {
        "foreign": {"min_abs_net": {"value": 0.1, "rationale": "r", "source": "heuristic"}},
        "institutional": {"min_abs_raw": {"value": 0.3, "rationale": "r", "source": "heuristic"}, "min_abs_z": {"value": 0.5, "rationale": "r", "source": "heuristic"}},
        "retail": {"min_abs_raw": {"value": 1.0, "rationale": "r", "source": "heuristic"}, "min_abs_z": {"value": 0.5, "rationale": "r", "source": "heuristic"}}
      },
      "conditions": {
        "foreign-3d-net-buy": {"layers": {"value": ["foregin", "institutional", "retail"], "rationale": "r", "source": "heuristic"}}
      }
    }
  }
}`
	path := writeFixture(t, body)
	_, err := config.LoadParametersConfig(path)
	if err == nil {
		t.Fatal("typo'd layer name (foregin) must fail config load, not silently skip")
	}
	if !strings.Contains(err.Error(), "unknown layer") {
		t.Errorf("error %q should mention the unknown layer", err)
	}
}

// --- 13. NewDefaultGateway reads the config singleton (hermetic) ---

func TestFlowGateway_NewDefaultGateway(t *testing.T) {
	origPath := config.GetParametersConfigPath()
	config.SetParametersConfigPath(writeFixture(t, validFlowGatewayFixture))
	config.ResetParametersConfig()
	t.Cleanup(func() {
		config.SetParametersConfigPath(origPath)
		config.ResetParametersConfig()
	})

	g := NewDefaultFlowGateway()
	v := g.Check("2330", string(ConditionForeign3DNetBuy), passPoint(), marketPassForces())
	if !v.Pass {
		t.Fatalf("Pass=false with default gateway + pass fixture: %+v", v)
	}
	// momentum narrows to [foreign institutional] via the fixture.
	v2 := g.Check("2330", string(ConditionMomentum20D), passPoint(), []capitalflow.ForceScore{
		flowScore(capitalflow.ForceInstitutional, 1.0, 0.8, true),
		flowScore(capitalflow.ForceRetail, 0.3, 0.2, true), // would fail retail if enforced
	})
	if !v2.Pass {
		t.Fatalf("momentum override must narrow to [foreign institutional], got fail: %+v", v2)
	}
}

// --- 14. nil gateway → fail-open ---

func TestFlowGateway_NilGateway(t *testing.T) {
	var g *FlowGateway
	v := g.Check("2330", "c", nil, nil)
	if !v.Pass {
		t.Fatalf("nil gateway must fail open (Pass=true), got %+v", v)
	}
	if v.Symbol != "2330" || v.ConditionID != "c" {
		t.Errorf("nil gateway verdict identity lost: %+v", v)
	}
	if v.AllSkipped || v.SkippedCount != 0 {
		t.Errorf("nil gateway AllSkipped=%v SkippedCount=%d, want false/0", v.AllSkipped, v.SkippedCount)
	}
}

// --- 15. CheckFromReport delegation ---

func TestFlowGateway_CheckFromReport(t *testing.T) {
	g := defaultGateway()
	point := passPoint()
	report := &capitalflow.DailyReport{Forces: marketPassForces()}
	v := g.CheckFromReport("2330", "c", point, report)
	if !v.Pass {
		t.Fatalf("Pass=false, want true: %+v", v)
	}

	// Nil report → no market forces: market layers skip, foreign passes.
	nilReport := g.CheckFromReport("2330", "c", point, nil)
	if !nilReport.Pass {
		t.Fatalf("nil report must fail open when foreign data is present: %+v", nilReport)
	}
	if nilReport.SkippedCount != 2 {
		t.Errorf("SkippedCount=%d, want 2 (market layers skipped)", nilReport.SkippedCount)
	}
	for _, layer := range []FlowLayer{FlowLayerInstitutional, FlowLayerRetail} {
		if lv, _ := layerVerdict(nilReport, layer); lv.Status != LayerStatusSkip {
			t.Errorf("%s status=%s, want skip for nil report", layer, lv.Status)
		}
	}
}
