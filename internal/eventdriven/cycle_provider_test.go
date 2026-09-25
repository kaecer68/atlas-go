package eventdriven

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// stubCycleSource lets the provider's evidence filter be tested without a real
// CycleTracker (whose evidence tiers depend on config-seeded startup state).
type stubCycleSource struct {
	tiers  map[string]string
	scores map[string]float64
	reads  int
}

func (s *stubCycleSource) EvidenceTier(industryID string) string {
	if tier, ok := s.tiers[industryID]; ok {
		return tier
	}
	return "insufficient"
}

func (s *stubCycleSource) GetContinuousPhaseScore(industryID string) float64 {
	s.reads++
	return s.scores[industryID]
}

// TestMeasuredCycleProvider_DropsSeedOnlyIndustries is the guard for the I4
// cycle half (#1944 Batch 4): a seeded ("estimated") or data-less
// ("insufficient") industry must keep the neutral 0.0 that the unwired state
// produced, so injecting the tracker cannot present a config seed as a
// measurement.
func TestMeasuredCycleProvider_DropsSeedOnlyIndustries(t *testing.T) {
	src := &stubCycleSource{
		tiers: map[string]string{
			"semiconductor": "empirical",
			"financials":    "estimated",
			"shipping":      "insufficient",
		},
		scores: map[string]float64{
			"semiconductor": 0.62,
			"financials":    0.40,
			"shipping":      0.30,
		},
	}
	p := NewMeasuredCycleProvider(src)

	if got := p.GetContinuousPhaseScore("semiconductor"); got != 0.62 {
		t.Errorf("empirical industry score = %v, want 0.62 (delegated)", got)
	}
	if got := p.GetContinuousPhaseScore("financials"); got != 0.0 {
		t.Errorf("seed-only industry score = %v, want 0.0 (a seed must not be presented as a measurement)", got)
	}
	if got := p.GetContinuousPhaseScore("shipping"); got != 0.0 {
		t.Errorf("industry without data score = %v, want 0.0", got)
	}
	if src.reads != 1 {
		t.Errorf("source queried %d times, want 1: seed-only industries must not even be read", src.reads)
	}
}

// TestMeasuredCycleProvider_NilSource pins the defensive path.
func TestMeasuredCycleProvider_NilSource(t *testing.T) {
	if got := NewMeasuredCycleProvider(nil).GetContinuousPhaseScore("semiconductor"); got != 0.0 {
		t.Errorf("nil source score = %v, want 0.0", got)
	}
	var p *MeasuredCycleProvider
	if got := p.GetContinuousPhaseScore("semiconductor"); got != 0.0 {
		t.Errorf("nil provider score = %v, want 0.0", got)
	}
}

// TestSectorPredictionStatusWiresCycleProvider proves the handler consumes the
// injected provider: the status flips and the `cycle_position` driver appears,
// so the wiring is observable end to end instead of merely stored.
func TestSectorPredictionStatusWiresCycleProvider(t *testing.T) {
	src := &stubCycleSource{
		tiers:  map[string]string{"semiconductor": "empirical", "foundry": "empirical", "server_assembly": "empirical"},
		scores: map[string]float64{"semiconductor": 0.9, "foundry": 0.9, "server_assembly": 0.9},
	}

	h := NewHandler(industry.NewEventCalendar())
	h.SetMacroProvider(stubMacroProvider{snap: staleSnapshot()})
	h.SetSectorCycleProvider(NewMeasuredCycleProvider(src))
	report := predictOnce(t, h)

	st := report.SectorPredictionStatus
	if st == nil {
		t.Fatal("SectorPredictionStatus must be populated")
	}
	if !st.CycleProviderWired {
		t.Error("status.CycleProviderWired must be true after SetSectorCycleProvider")
	}

	seen := false
	for _, day := range report.SectorPredictions {
		for _, s := range day.Sectors {
			for _, d := range s.Drivers {
				if d == "cycle_position" {
					seen = true
				}
			}
		}
	}
	if !seen {
		t.Error("no cycle_position driver was reported although a measured cycle provider is wired")
	}
}

// TestSectorPredictionStatusCycleProviderSurvivesRebuild mirrors the prior
// regression: the provider must be re-applied on every predictor the handler
// rebuilds, not only on the first request.
func TestSectorPredictionStatusCycleProviderSurvivesRebuild(t *testing.T) {
	src := &stubCycleSource{
		tiers:  map[string]string{"semiconductor": "empirical"},
		scores: map[string]float64{"semiconductor": 0.9},
	}
	h := NewHandler(industry.NewEventCalendar())
	h.SetMacroProvider(stubMacroProvider{snap: staleSnapshot()})
	h.SetSectorCycleProvider(NewMeasuredCycleProvider(src))

	first := predictOnce(t, h)
	second := predictOnce(t, h)
	for name, report := range map[string]PredictionReport{"first": first, "second": second} {
		if report.SectorPredictionStatus == nil || !report.SectorPredictionStatus.CycleProviderWired {
			t.Fatalf("%s request: CycleProviderWired = false, want true (provider must survive the cache-miss rebuild)", name)
		}
	}
}
