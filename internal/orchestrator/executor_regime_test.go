package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func emptyRegistry() domain.AgentRegistry {
	return recommendation.AgentRegistry{}
}

func TestSequentialRegime_CounterExampleA(t *testing.T) {
	// Retail-chasing scenario: no macro data (no VIX quote), but many
	// stocks show last > open. Sequential pipeline should remain neutral,
	// not produce RISK_ON from pure retail-chasing signals.
	quotes := map[string]domain.Quote{
		"2330": {Symbol: "2330", Last: 600, Open: 590},
		"2454": {Symbol: "2454", Last: 1100, Open: 1080},
		"2317": {Symbol: "2317", Last: 105, Open: 102},
	}

	regime := inferRegime(emptyRegistry(), quotes, nil, nil, nil, nil, "", nil)
	if regime != domain.RegimeNeutral {
		t.Errorf("counter-A: got %s, want %s — retail chasing without macro must be neutral", regime, domain.RegimeNeutral)
	}
}

func TestSequentialRegime_CounterExampleB(t *testing.T) {
	// Layer conflict scenario: macro (VIX crash) = RISK_OFF, but technical
	// (many stocks up) = RISK_ON. Sequential pipeline reduces technical
	// confidence so the result is RISK_OFF, not RISK_ON.
	quotes := map[string]domain.Quote{
		"^VIX": {Symbol: "^VIX", Last: 42},
		"2330": {Symbol: "2330", Last: 600, Open: 590},
		"2454": {Symbol: "2454", Last: 1100, Open: 1080},
	}

	regime := inferRegime(emptyRegistry(), quotes, nil, nil, nil, nil, "", nil)
	if regime == domain.RegimeRiskOn {
		t.Errorf("counter-B: got RISK_ON, want not RISK_ON — layer_0 VIX crash must dominate")
	}
}

func TestSequentialRegime_CounterExampleC(t *testing.T) {
	quotes := map[string]domain.Quote{
		"^VIX": {Symbol: "^VIX", Last: 18},
	}
	events := []narrative.NarrativeEvent{
		{Theme: "geopolitical_risk_spike", Severity: "high", Confidence: 0.6, HitRate: 0.8},
	}
	regime := inferRegime(emptyRegistry(), quotes, nil, nil, events, nil, "", nil)
	if regime != domain.RegimeRiskOff && regime != domain.RegimeNeutral {
		t.Errorf("counter-C: got %s, want RISK_OFF or NEUTRAL (geopolitical risk)", regime)
	}
	t.Logf("counter-C: regime=%s", regime)
}

func TestSequentialRegime_LayerIDTraces(t *testing.T) {
	quotes := map[string]domain.Quote{
		"^VIX": {Symbol: "^VIX", Last: 25},
		"2330": {Symbol: "2330", Last: 600, Open: 590},
	}

	scratchpad := NewScratchpad("test-session", t.TempDir())
	regime := inferRegime(emptyRegistry(), quotes, nil, nil, nil, scratchpad, "test-session", nil)

	traces := scratchpad.Traces()
	if len(traces) == 0 {
		t.Fatal("expected at least one trace")
	}
	t.Logf("regime=%s traces=%d", regime, len(traces))

	layersFound := make(map[string]bool)
	for _, tr := range traces {
		// Only check layer evidence traces (not the final detect_regime trace)
		if tr.Action != "layer_evidence" {
			continue
		}
		if tr.LayerID == "" {
			t.Errorf("trace without LayerID: action=%s component=%s", tr.Action, tr.Component)
		}
		layersFound[tr.LayerID] = true
		t.Logf("  layer=%s parent=%s conf=%.3f", tr.LayerID, tr.LayerParentID, tr.Confidence)
	}
	for _, want := range []string{"layer_0", "layer_4", "layer_7", "layer_root"} {
		if !layersFound[want] {
			t.Errorf("%s not found in traces", want)
		}
	}
}

func TestSequentialRegime_Regression(t *testing.T) {
	tests := []struct {
		name   string
		quotes map[string]domain.Quote
		want   domain.Regime
	}{
		{"empty quotes → neutral", map[string]domain.Quote{}, domain.RegimeNeutral},
		{
			"high VIX → risk_off",
			map[string]domain.Quote{"^VIX": {Symbol: "^VIX", Last: 45}},
			domain.RegimeRiskOff,
		},
		{
			"no VIX + stocks up → neutral",
			map[string]domain.Quote{
				"2330": {Symbol: "2330", Last: 600, Open: 590},
			},
			domain.RegimeNeutral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferRegime(emptyRegistry(), tt.quotes, nil, nil, nil, nil, "", nil)
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
