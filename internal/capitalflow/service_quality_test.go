package capitalflow

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type stubSnapshotProvider struct {
	calls int32
	snap  marketdata.MacroDataSnapshot
	err   error
	delay time.Duration
}

func (s *stubSnapshotProvider) Name() string { return "stub" }

func (s *stubSnapshotProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.snap, s.err
}

// resonanceToScore is the core mapping contract. Service-level cache tests
// avoid asserting on direction because the rolling-window z-score requires
// pre-primed baselines that are expensive to set up in unit tests.

func TestResonanceToScore_BullishAligned(t *testing.T) {
	r := ResonanceResult{Coefficient: 1.5, Direction: "bullish"}
	if got := resonanceToScore(r); got != 1.0 {
		t.Errorf("bullish aligned (coef 1.5): want +1.0, got %v", got)
	}
}

func TestResonanceToScore_BearishAligned(t *testing.T) {
	r := ResonanceResult{Coefficient: 1.5, Direction: "bearish"}
	if got := resonanceToScore(r); got != -1.0 {
		t.Errorf("bearish aligned (coef 1.5): want -1.0, got %v", got)
	}
}

func TestResonanceToScore_PartialBullish(t *testing.T) {
	r := ResonanceResult{Coefficient: 1.25, Direction: "bullish"}
	if got := resonanceToScore(r); got != 0.75 {
		t.Errorf("partial bullish (coef 1.25): want +0.75, got %v", got)
	}
}

func TestResonanceToScore_TypicalBullishStillScores(t *testing.T) {
	// Typical case where foreign/institutional align bullish but government
	// stays neutral → Coefficient=1.0, Direction=bullish. The mapping must
	// still emit a non-zero positive score so eventdriven picks up the signal.
	r := ResonanceResult{Coefficient: 1.0, Direction: "bullish"}
	if got := resonanceToScore(r); got != 0.5 {
		t.Errorf("typical bullish (coef 1.0): want +0.5 floor, got %v", got)
	}
}

func TestResonanceToScore_MixedReturnsZero(t *testing.T) {
	for _, dir := range []string{"mixed", "neutral", ""} {
		r := ResonanceResult{Coefficient: 0.5, Direction: dir}
		if got := resonanceToScore(r); got != 0 {
			t.Errorf("direction=%q must yield 0 regardless of coefficient, got %v", dir, got)
		}
	}
}

func TestService_QualityScore_CacheReducesFetchCalls(t *testing.T) {
	provider := &stubSnapshotProvider{snap: neutralSnapshot()}
	svc := NewService(provider, time.Second, nil)

	_ = svc.QualityScore()
	if got := atomic.LoadInt32(&provider.calls); got != 1 {
		t.Fatalf("first call must fetch once, got %d", got)
	}

	for i := 0; i < 5; i++ {
		_ = svc.QualityScore()
	}
	if got := atomic.LoadInt32(&provider.calls); got != 1 {
		t.Errorf("subsequent calls within QualityCacheTTL must not refetch, got %d", got)
	}
}

func TestService_QualityLabel_CacheReducesFetchCalls(t *testing.T) {
	provider := &stubSnapshotProvider{snap: neutralSnapshot()}
	svc := NewService(provider, time.Second, nil)

	_ = svc.QualityLabel()
	_ = svc.QualityLabel()
	_ = svc.QualityLabel()

	if got := atomic.LoadInt32(&provider.calls); got != 1 {
		t.Errorf("QualityLabel within TTL must reuse cache, got %d fetches", got)
	}
}

func TestService_QualityScore_ProviderErrorReturnsZeroInitially(t *testing.T) {
	svc := NewService(&stubSnapshotProvider{err: errors.New("boom")}, time.Second, nil)
	if got := svc.QualityScore(); got != 0 {
		t.Errorf("fresh cache + provider error: want 0, got %v", got)
	}
	if got := svc.QualityLabel(); got != "neutral" {
		t.Errorf("fresh cache + provider error: want neutral, got %q", got)
	}
}

func TestService_QualityScore_FailedRefreshKeepsStaleCache(t *testing.T) {
	provider := &stubSnapshotProvider{snap: neutralSnapshot()}
	svc := NewService(provider, time.Second, nil)
	first := svc.QualityScore()
	firstLabel := svc.QualityLabel()

	svc.mu.Lock()
	svc.cachedAt = time.Now().Add(-2 * QualityCacheTTL)
	svc.mu.Unlock()
	provider.err = errors.New("post-prime boom")

	if got := svc.QualityScore(); got != first {
		t.Errorf("stale cache should survive provider failure: want %v, got %v", first, got)
	}
	if got := svc.QualityLabel(); got != firstLabel {
		t.Errorf("stale cache label should survive provider failure: want %q, got %q", firstLabel, got)
	}
}

// neutralSnapshot produces a Direction=neutral Coefficient=1.0 resonance
// (single-push to windows means z-score = 0 → trend = neutral).
func neutralSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		RecordedAt:         time.Now().Unix(),
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 0},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: 0},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 0},
	}
}
