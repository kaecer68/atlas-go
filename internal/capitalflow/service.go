package capitalflow

import (
	"context"
	"fmt"
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
