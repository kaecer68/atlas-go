package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestAdjustRegimeFromNarrative_NoEvents(t *testing.T) {
	tests := []struct {
		name string
		base domain.Regime
		want domain.Regime
	}{
		{"risk on stays", domain.RegimeRiskOn, domain.RegimeRiskOn},
		{"risk off stays", domain.RegimeRiskOff, domain.RegimeRiskOff},
		{"neutral stays", domain.RegimeNeutral, domain.RegimeNeutral},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AdjustRegimeFromNarrative(tt.base, nil)
			if got != tt.want {
				t.Errorf("AdjustRegimeFromNarrative() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdjustRegimeFromNarrative_RiskOffEvents(t *testing.T) {
	riskOffEvents := []narrative.NarrativeEvent{
		{Theme: "US_rates_up", Confidence: 0.8},
		{Theme: "geopolitical_risk_spike", Confidence: 0.7},
		{Theme: "oil_price_shock", Confidence: 0.6},
		{Theme: "JPY_carry_unwind", Confidence: 0.5},
	}
	for _, ev := range riskOffEvents {
		t.Run(ev.Theme, func(t *testing.T) {
			for _, base := range []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral, domain.RegimeRiskOff} {
				got := AdjustRegimeFromNarrative(base, []narrative.NarrativeEvent{ev})
				if got != domain.RegimeRiskOff {
					t.Errorf("base=%v with %v: got %v, want RISK_OFF", base, ev.Theme, got)
				}
			}
		})
	}
}

func TestAdjustRegimeFromNarrative_RiskOnEvents(t *testing.T) {
	events := []narrative.NarrativeEvent{{Theme: "AI_capex_surge", Confidence: 0.9}}

	t.Run("neutral to risk on", func(t *testing.T) {
		got := AdjustRegimeFromNarrative(domain.RegimeNeutral, events)
		if got != domain.RegimeRiskOn {
			t.Errorf("got %v, want RISK_ON", got)
		}
	})

	t.Run("risk off to neutral", func(t *testing.T) {
		got := AdjustRegimeFromNarrative(domain.RegimeRiskOff, events)
		if got != domain.RegimeNeutral {
			t.Errorf("got %v, want NEUTRAL", got)
		}
	})

	t.Run("risk on stays", func(t *testing.T) {
		got := AdjustRegimeFromNarrative(domain.RegimeRiskOn, events)
		if got != domain.RegimeRiskOn {
			t.Errorf("got %v, want RISK_ON", got)
		}
	})
}

func TestAdjustRegimeFromNarrative_MixedEvents(t *testing.T) {
	// Risk-off themes should dominate when mixed with risk-on themes
	events := []narrative.NarrativeEvent{
		{Theme: "AI_capex_surge", Confidence: 0.9},
		{Theme: "US_rates_up", Confidence: 0.8},
	}
	got := AdjustRegimeFromNarrative(domain.RegimeRiskOn, events)
	if got != domain.RegimeRiskOff {
		t.Errorf("mixed events: got %v, want RISK_OFF", got)
	}
}

// TestBuildParameterSnapshot verifies that buildParameterSnapshot() returns
// a populated ParameterSnapshot with fields from the current config.
func TestBuildParameterSnapshot(t *testing.T) {
	snap := buildParameterSnapshot()
	if snap == nil {
		t.Fatal("expected non-nil ParameterSnapshot")
	}
	if snap.ConfigVersion == "" {
		t.Error("expected ConfigVersion to be non-empty")
	}
	if len(snap.FactorWeights) == 0 {
		t.Error("expected FactorWeights to be non-empty")
	}
	if len(snap.NarrativeHitRates) == 0 {
		t.Error("expected NarrativeHitRates to be non-empty")
	}
	if snap.CapturedAt.IsZero() {
		t.Error("expected CapturedAt to be set")
	}
	t.Logf("Snapshot: version=%s, weights=%d, hitrates=%d, phases=%d",
		snap.ConfigVersion, len(snap.FactorWeights), len(snap.NarrativeHitRates), len(snap.IndustryPhaseScores))
}
