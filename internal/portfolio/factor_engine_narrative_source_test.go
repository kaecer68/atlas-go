package portfolio

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestNarrativeFactorFormulaCarriesHitRateSource pins item I23 (#1944 Batch 3)
// on the outward factor breakdown: the narrative factor presents an average hit
// rate over narrative events, and that average must never appear without its
// provenance. Today every contributing event carries a hand-authored prior, so
// the formula must say so.
func TestNarrativeFactorFormulaCarriesHitRateSource(t *testing.T) {
	fe := NewFactorEngine()
	fe.WithNarrativeProvider(func(symbol string) *domain.NarrativeFactorScore {
		if symbol != "TEST.TW" {
			return nil
		}
		return &domain.NarrativeFactorScore{
			Score:         0.75,
			Theme:         "AI_capex_surge",
			HitRate:       0.81,
			HitRateSource: "handwritten_prior",
			Confidence:    0.90,
		}
	})

	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}
	breakdown, _ := fe.CalculateAllScoresWithBreakdown("TEST.TW", quotes, nil, nil, nil)

	if !strings.Contains(breakdown.Narrative.Formula, "hit_rate_source=handwritten_prior") {
		t.Errorf("narrative formula %q must carry the hit-rate provenance", breakdown.Narrative.Formula)
	}
	if !strings.Contains(breakdown.Narrative.Formula, "hit_rate=0.81") {
		t.Errorf("narrative formula %q must still carry the hit rate", breakdown.Narrative.Formula)
	}
}

// TestNarrativeFactorFormulaMarksUnknownSource pins the defensive branch: a
// provider that predates the provenance field must not produce a bare hit rate.
func TestNarrativeFactorFormulaMarksUnknownSource(t *testing.T) {
	fe := NewFactorEngine()
	fe.WithNarrativeProvider(func(symbol string) *domain.NarrativeFactorScore {
		if symbol != "TEST.TW" {
			return nil
		}
		return &domain.NarrativeFactorScore{Score: 0.5, Theme: "x", HitRate: 0.5, Confidence: 0.5}
	})
	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}
	breakdown, _ := fe.CalculateAllScoresWithBreakdown("TEST.TW", quotes, nil, nil, nil)
	if !strings.Contains(breakdown.Narrative.Formula, "hit_rate_source=unknown") {
		t.Errorf("narrative formula %q must mark an absent provenance as unknown", breakdown.Narrative.Formula)
	}
}
