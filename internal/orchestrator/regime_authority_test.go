package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// #1785: when an authoritative regime source is attached (regime_history /
// stress index), inferRegime must adopt it verbatim — the 4-layer evidence
// becomes advisory and the home dashboard vs session radar contradiction
// disappears at the source.
func TestInferRegime_AuthorityOverridesEvidence(t *testing.T) {
	// Evidence that alone would clearly say RISK_OFF (high VIX, all quotes down).
	quotes := map[string]domain.Quote{
		"^VIX": {Symbol: "^VIX", Last: 45},
		"2330": {Symbol: "2330", Last: 500, Open: 600, Volume: 1000},
		"2454": {Symbol: "2454", Last: 900, Open: 1000, Volume: 1000},
		"2317": {Symbol: "2317", Last: 90, Open: 100, Volume: 1000},
		"6505": {Symbol: "6505", Last: 1000, Open: 1100, Volume: 1000},
	}
	authority := func() (domain.Regime, float64, bool) {
		return domain.RegimeRiskOn, 5.576, true
	}

	got := inferRegime(emptyRegistry(), quotes, nil, nil, nil, nil, "s-auth", authority)
	if got != domain.RegimeRiskOn {
		t.Errorf("authority override: got %s, want RISK_ON (authoritative value must win)", got)
	}
}

func TestInferRegime_AuthorityUnavailableFallsBackToEvidence(t *testing.T) {
	quotes := map[string]domain.Quote{
		"^VIX": {Symbol: "^VIX", Last: 45},
	}
	authority := func() (domain.Regime, float64, bool) {
		return "", 0, false // store down / no rows
	}

	got := inferRegime(emptyRegistry(), quotes, nil, nil, nil, nil, "s-fallback", authority)
	if got != domain.RegimeRiskOff {
		t.Errorf("fallback: got %s, want RISK_OFF (evidence inference)", got)
	}
}

// #1785: the technical layer must not let a tiny quote sample outvote the
// macro layer. 4 down-ticks used to produce score=-1.2 with full confidence
// 0.4; now the score is a bounded ratio and confidence scales with coverage.
func TestTechnicalEvidence_SmallSampleLowConfidence(t *testing.T) {
	src := NewTechnicalEvidenceSource()
	small := map[string]domain.Quote{
		"2330": {Symbol: "2330", Last: 500, Open: 600, Volume: 1000},
		"2454": {Symbol: "2454", Last: 900, Open: 1000, Volume: 1000},
		"2317": {Symbol: "2317", Last: 90, Open: 100, Volume: 1000},
		"6505": {Symbol: "6505", Last: 1000, Open: 1100, Volume: 1000},
	}
	ev := src.Evidence(small, nil)
	if ev.Score != -1.0 {
		t.Errorf("score = %.3f, want -1.0 (all-down bounded ratio)", ev.Score)
	}
	if ev.Confidence > 0.4*4/20+1e-9 {
		t.Errorf("confidence = %.3f, want coverage-scaled (<= %.3f) so 4 symbols cannot dominate", ev.Confidence, 0.4*4/20)
	}

	// A volumed sample of 20+ reaches full confidence.
	large := map[string]domain.Quote{}
	for i := 0; i < 25; i++ {
		sym := "S" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		large[sym] = domain.Quote{Symbol: sym, Last: 100, Open: 95, Volume: 1000}
	}
	evFull := src.Evidence(large, nil)
	if evFull.Confidence != 0.4 {
		t.Errorf("full-coverage confidence = %.3f, want 0.4", evFull.Confidence)
	}
	if evFull.Score != 1.0 {
		t.Errorf("all-up score = %.3f, want 1.0", evFull.Score)
	}
}
