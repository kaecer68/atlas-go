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
	calls atomic.Int32
	snap  marketdata.MacroDataSnapshot
	err   error
	delay time.Duration
}

func (s *stubSnapshotProvider) Name() string { return "stub" }

func (s *stubSnapshotProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.snap, s.err
}

// resonanceToScore is the core mapping contract. Service-level cache tests
// avoid asserting on direction because the rolling-window z-score requires
// pre-primed baselines that are expensive to set up in unit tests.
//
// M5 mapping (audit 2026-09-04): the implementation
// sign(direction) * max(0.5, coefficient-0.5) is the locked contract —
// eventdriven's scaleQualityScoreToBaseline depends on the ±0.5 floor for
// coefficient 1.0. Table rows below anchor every acceptance case:
// 1.5→±1.0, 1.0→±0.5, 0.5→±0.5 (floor), mixed/neutral/"" → 0.
func TestResonanceToScore_MappingTable(t *testing.T) {
	cases := []struct {
		name string
		coef float64
		dir  string
		want float64
	}{
		{"bullish_aligned_1.5", 1.5, "bullish", 1.0},
		{"bearish_aligned_1.5", 1.5, "bearish", -1.0},
		{"bullish_partial_1.25", 1.25, "bullish", 0.75},
		{"bullish_typical_1.0", 1.0, "bullish", 0.5},
		{"bearish_typical_1.0", 1.0, "bearish", -0.5},
		{"bullish_min_coef_0.5", 0.5, "bullish", 0.5},
		{"bearish_min_coef_0.5", 0.5, "bearish", -0.5},
		{"mixed_1.0", 1.0, "mixed", 0},
		{"mixed_0.5", 0.5, "mixed", 0},
		{"neutral_0.5", 0.5, "neutral", 0},
		{"empty_direction", 0.5, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := ResonanceResult{Coefficient: c.coef, Direction: c.dir}
			if got := resonanceToScore(r); got != c.want {
				t.Errorf("resonanceToScore(coef=%v, dir=%q) = %v, want %v", c.coef, c.dir, got, c.want)
			}
		})
	}
}

func TestService_QualityScore_CacheReducesFetchCalls(t *testing.T) {
	provider := &stubSnapshotProvider{snap: neutralSnapshot()}
	svc := NewService(provider, time.Second, nil)

	_ = svc.QualityScore()
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("first call must fetch once, got %d", got)
	}

	for range 5 {
		_ = svc.QualityScore()
	}
	if got := provider.calls.Load(); got != 1 {
		t.Errorf("subsequent calls within QualityCacheTTL must not refetch, got %d", got)
	}
}

func TestService_QualityLabel_CacheReducesFetchCalls(t *testing.T) {
	provider := &stubSnapshotProvider{snap: neutralSnapshot()}
	svc := NewService(provider, time.Second, nil)

	_ = svc.QualityLabel()
	_ = svc.QualityLabel()
	_ = svc.QualityLabel()

	if got := provider.calls.Load(); got != 1 {
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
