package eventdriven

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// TestComputeHistoricalHitRate_DisclosesNeutralBasis is the N-U6 fix
// (#1944 Batch 4): the production rate (12.1%) was built from reconciled rows
// whose predicted direction was neutral. Those rows can never be directional
// hits, so the payload must disclose how many of the samples cannot be judged
// and which definition the ratio uses — otherwise the number reads as
// "directionally wrong ~88% of the time".
func TestComputeHistoricalHitRate_DisclosesNeutralBasis(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	rec := reconTime()
	h.SetPredictionStore(&fakePredictionStore{records: []PredictionRecord{
		{DirectionSign: 0.5, ActualSign: 0.6, ActualCapturedAt: &rec},   // inflow hit
		{DirectionSign: 0, ActualSign: 0.4, ActualCapturedAt: &rec},     // neutral prediction → not judgeable
		{DirectionSign: 0, ActualSign: 0, ActualCapturedAt: &rec},       // neutral prediction, neutral actual
		{DirectionSign: -0.5, ActualSign: -0.4, ActualCapturedAt: &rec}, // outflow hit
		{DirectionSign: 0.5, ActualSign: -0.2, ActualCapturedAt: &rec},  // miss
	}})

	got := h.computeHistoricalHitRate()
	if got == nil {
		t.Fatal("expected non-nil hit rate")
	}
	if got.Samples != 5 {
		t.Fatalf("Samples = %d, want 5 (neutral predictions stay in the denominator)", got.Samples)
	}
	if got.Hits != 2 {
		t.Fatalf("Hits = %d, want 2", got.Hits)
	}
	if got.NeutralSamples != 2 {
		t.Fatalf("NeutralSamples = %d, want 2", got.NeutralSamples)
	}
	if got.DirectionalSamples != 3 {
		t.Fatalf("DirectionalSamples = %d, want 3", got.DirectionalSamples)
	}
	if got.HitRateBasis != HitRateBasisDirectionSign {
		t.Fatalf("HitRateBasis = %q, want %q", got.HitRateBasis, HitRateBasisDirectionSign)
	}
	if got.HitRate != 2.0/5.0 {
		t.Fatalf("HitRate = %v, want 2/5", got.HitRate)
	}
}

// TestComputeHistoricalHitRate_BasisIsSetOnEveryShape pins that the basis label
// is always present, including the zero-sample and object-marshalling shapes,
// so a consumer can never have to guess the definition.
func TestComputeHistoricalHitRate_BasisIsSetOnEveryShape(t *testing.T) {
	rec := reconTime()

	unreconciled := NewHandler(industry.NewEventCalendar())
	unreconciled.SetPredictionStore(&fakePredictionStore{records: []PredictionRecord{
		{DirectionSign: 0.5, ActualSign: 0}, // no ActualCapturedAt → not yet reconciled
	}})
	if got := unreconciled.computeHistoricalHitRate(); got == nil || got.HitRateBasis != HitRateBasisDirectionSign {
		t.Fatalf("zero-reconciled shape must carry the basis, got %+v", got)
	}

	unwired := NewHandler(industry.NewEventCalendar())
	unwired.SetPredictionStore(&fakePredictionStore{records: []PredictionRecord{
		{DirectionSign: 0.5, ActualSign: 0.6, ActualCapturedAt: &rec},
	}})
	got := unwired.computeHistoricalHitRate()
	if got == nil || got.NeutralSamples != 0 || got.DirectionalSamples != 1 {
		t.Fatalf("all-directional shape = %+v, want neutral 0 / directional 1", got)
	}
	if got.HitRateBasis != HitRateBasisDirectionSign {
		t.Fatalf("basis = %q, want %q", got.HitRateBasis, HitRateBasisDirectionSign)
	}
}
