package orchestrator

import (
	"reflect"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestTechniquesLayerActive_IsFalse pins the inert-by-design status of the
// L1-L5 心法 (strategy techniques) layer. Issue #1944 Batch 1: the plugin is
// registered in production, so "the plugin exists" must not be read as "the
// 心法 layer is effective". Flip this expectation only together with the
// detector/corrector wiring and evidence that recommendations actually change.
func TestTechniquesLayerActive_IsFalse(t *testing.T) {
	if TechniquesLayerActive {
		t.Fatal("TechniquesLayerActive must be false while ProcessRecommendations is a pass-through")
	}
}

// TestStrategyTechniquesPlugin_ProcessRecommendations_IsPassThrough asserts the
// documented invariant: the plugin returns the SAME slice (no copy, no filter,
// no reorder) for every regime.
func TestStrategyTechniquesPlugin_ProcessRecommendations_IsPassThrough(t *testing.T) {
	recs := []domain.Recommendation{
		{Agent: "a1", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "a2", Symbol: "2317.TW", Side: domain.SideSell, Conviction: 60},
	}
	for _, regime := range []domain.Regime{domain.RegimeRiskOn, domain.RegimeRiskOff, domain.RegimeNeutral} {
		in := make([]domain.Recommendation, len(recs))
		copy(in, recs)

		p := &strategyTechniquesPlugin{}
		out := p.ProcessRecommendations(regime, in)

		if len(out) != len(in) {
			t.Fatalf("regime %s: len(out)=%d, want %d (pass-through must not filter)", regime, len(out), len(in))
		}
		if &out[0] != &in[0] {
			t.Fatalf("regime %s: plugin copied the slice; pass-through must return the same backing array", regime)
		}
		for i := range out {
			if !reflect.DeepEqual(out[i], in[i]) {
				t.Fatalf("regime %s: rec[%d] mutated: %+v != %+v", regime, i, out[i], in[i])
			}
		}
	}
}

// TestStrategyTechniquesPlugin_ProcessRecommendations_EmptyAndNil covers the
// degenerate inputs the pass-through must also hand back untouched.
func TestStrategyTechniquesPlugin_ProcessRecommendations_EmptyAndNil(t *testing.T) {
	p := &strategyTechniquesPlugin{}

	if out := p.ProcessRecommendations(domain.RegimeNeutral, nil); out != nil {
		t.Fatalf("nil input: got %v, want nil", out)
	}

	empty := []domain.Recommendation{}
	out := p.ProcessRecommendations(domain.RegimeNeutral, empty)
	if out == nil || len(out) != 0 {
		t.Fatalf("empty input: got %v (len=%d), want empty non-nil", out, len(out))
	}
}

// TestStrategyTechniquesPlugin_Name pins the plugin identifier used by
// PluginHost routing and by main.go logging — operators rely on this name to
// recognise the inert 心法 plugin in logs.
func TestStrategyTechniquesPlugin_Name(t *testing.T) {
	p := &strategyTechniquesPlugin{}
	if got := p.Name(); got != "strategy_techniques" {
		t.Fatalf("Name() = %q, want %q", got, "strategy_techniques")
	}
}

// TestStrategyTechniquesPlugin_PostSimulation_NilRegistry is a smoke test for
// the second documented no-op path (no detector/corrector yet): PostSimulation
// must not panic and must not mutate anything when no registry was injected.
func TestStrategyTechniquesPlugin_PostSimulation_NilRegistry(t *testing.T) {
	p := &strategyTechniquesPlugin{}
	p.PostSimulation(nil, domain.RegimeNeutral, time.Now())
}
