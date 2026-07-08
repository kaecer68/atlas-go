package capitalflow

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// Service exposes capital-flow aggregation as a callable interface so
// downstream consumers (e.g. internal/recommender) can reuse the same
// pipeline the HTTP handler runs, without going through *http.Request.
//
// The pipeline (FetchSnapshot → Extract → ComputeResonance → GenerateDailyReport)
// is purely data-driven and HTTP-agnostic.
type Service struct {
	provider  marketdata.MacroDataProvider
	extractor *ForceExtractor
	timeout   time.Duration
}

// NewService constructs a Service backed by the given macrodata provider.
// Pass timeout=0 to use the default 15s context timeout.
func NewService(p marketdata.MacroDataProvider, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Service{provider: p, extractor: NewForceExtractor(), timeout: timeout}
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

