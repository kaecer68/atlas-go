package capitalflow

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// QualityCacheTTL bounds cache reuse for QualityScore/Label. Kept short
// because the event-driven predictor calls these on every request and
// longer TTLs risk predictions reflecting pre-news resonance.
const QualityCacheTTL = 60 * time.Second

// Service exposes capital-flow aggregation as a callable interface so
// downstream consumers (e.g. internal/recommender, internal/eventdriven)
// can reuse the same pipeline the HTTP handler runs, without going
// through *http.Request.
//
// The pipeline (FetchSnapshot → Extract → ComputeResonance → GenerateDailyReport)
// is purely data-driven and HTTP-agnostic.
type Service struct {
	provider  marketdata.MacroDataProvider
	extractor *ForceExtractor
	timeout   time.Duration

	mu              sync.RWMutex
	cachedResonance ResonanceResult
	cachedAt        time.Time
}

// NewService constructs a Service backed by the given macrodata provider.
// Pass timeout=0 to use the default 15s context timeout.
func NewService(p marketdata.MacroDataProvider, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Service{provider: p, extractor: NewForceExtractor(), timeout: timeout}
}

// QualityScore returns a signed score in [-1, 1] derived from the latest
// cached ResonanceResult. Mapping:
//
//	score = (coefficient - 1.0) * 2.0 * sign(direction)
//
// so bullish alignment (coefficient 1.5, dir bullish) → +1,
// bearish alignment (coefficient 1.5, dir bearish) → -1,
// mixed / neutral → 0.
//
// Returns 0 if no successful resonance has been observed yet. Auto-refreshes
// when the cache is older than QualityCacheTTL.
func (s *Service) QualityScore() float64 {
	return resonanceToScore(s.refreshIfStale())
}

// QualityLabel returns the direction label for the latest cached resonance
// ("bullish" / "bearish" / "mixed" / "neutral"). Auto-refreshes when stale.
// Returns "neutral" when no successful resonance has been observed yet.
func (s *Service) QualityLabel() string {
	r := s.refreshIfStale()
	if r.Direction == "" {
		return "neutral"
	}
	return r.Direction
}

func resonanceToScore(r ResonanceResult) float64 {
	switch r.Direction {
	case "bullish":
		return math.Max(0.5, r.Coefficient-0.5)
	case "bearish":
		return -math.Max(0.5, r.Coefficient-0.5)
	default:
		return 0
	}
}

// refreshIfStale returns the cached ResonanceResult, refreshing it when
// older than QualityCacheTTL or when the cache has never been populated.
// Concurrent callers serialize on the write lock. A failed refresh leaves
// the previous cached value intact so stale-but-better-than-nothing wins
// over zeros during provider outages.
func (s *Service) refreshIfStale() ResonanceResult {
	s.mu.RLock()
	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < QualityCacheTTL {
		r := s.cachedResonance
		s.mu.RUnlock()
		return r
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cachedAt.IsZero() && time.Since(s.cachedAt) < QualityCacheTTL {
		return s.cachedResonance
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	snap, err := s.provider.FetchSnapshot(ctx)
	if err != nil {
		return s.cachedResonance
	}
	forces := s.extractor.Extract(snap)
	s.cachedResonance = ComputeResonance(forces)
	s.cachedAt = time.Now()
	return s.cachedResonance
}

// LatestDaily runs the same FetchSnapshot → Extract → ComputeResonance →
// GenerateDailyReport pipeline as Handler.HandleDaily but as a Go call.
func (s *Service) LatestDaily(ctx context.Context) (DailyReport, error) {
	cctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	snap, err := s.provider.FetchSnapshot(cctx)
	if err != nil {
		return DailyReport{}, err
	}
	date := time.Unix(snap.RecordedAt, 0)
	forces := s.extractor.Extract(snap)
	resonance := ComputeResonance(forces)
	return GenerateDailyReport(date, forces, resonance), nil
}

// Summary returns the latest summary report by reusing LatestDaily's
// FetchSnapshot → Extract → ComputeResonance pipeline. It exists to give
// non-HTTP consumers (background jobs, internal adapters such as
// internal/recommender) a SummaryReport without routing through
// Handler.HandleSummary (which requires *http.Request).
//
// Caller cost: a single provider fetch + Extract + ComputeResonance,
// shared with LatestDaily if both are called on the same snapshot.
// SummaryReport is derived deterministically from the same
// (date, forces, resonance) tuple that feeds DailyReport.
func (s *Service) Summary(ctx context.Context) (SummaryReport, error) {
	daily, err := s.LatestDaily(ctx)
	if err != nil {
		return SummaryReport{}, fmt.Errorf("capitalflow: build summary from latest daily: %w", err)
	}
	return GenerateSummaryReport(daily.Date, daily.Forces, daily.Resonance), nil
}
