package service

import (
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// periodMatrixCacheTTL mirrors the agent-observatory / overview cache cadence
// (PR #1813): the admin heatmap fires one request per page load over a
// possibly large outcome set, so a 60s TTL keeps repeated loads cheap while
// staying fresh enough for a read-only research view.
const periodMatrixCacheTTL = 60 * time.Second

// PeriodMatrixService serves the (agent × seven-period) performance matrix
// over the SSoT outcome store (capital-flow Phase 2 PR-2b).
//
// Read path mirrors SessionHistoryProvider /
// ledger.NewReportOutcomeStore (docs/decisions/2026-08-23-performance-report-ssot.md):
// PostgreSQL is the single source of truth; the JSONL ledger is used only as
// a degraded fallback when PG is unavailable. Source()/Degraded() label the
// response exactly like the performance report does.
type PeriodMatrixService struct {
	store ledger.OutcomeStore

	mu        sync.Mutex
	matrixAt  time.Time
	matrixHit *portfolio.PeriodPerformanceMatrix
}

// NewPeriodMatrixService builds the service from the normalized config.
func NewPeriodMatrixService(cfg config.Config) (*PeriodMatrixService, error) {
	store, err := ledger.NewReportOutcomeStore(cfg)
	if err != nil {
		return nil, err
	}
	return &PeriodMatrixService{store: store}, nil
}

// NewPeriodMatrixServiceWithStore wires an explicit outcome store (tests).
func NewPeriodMatrixServiceWithStore(store ledger.OutcomeStore) *PeriodMatrixService {
	return &PeriodMatrixService{store: store}
}

// Degraded reports whether the most recent read fell back to JSONL because
// the SSoT backend (PostgreSQL) was unavailable.
func (s *PeriodMatrixService) Degraded() bool {
	if s == nil || s.store == nil {
		return false
	}
	if d, ok := s.store.(interface{ Degraded() bool }); ok {
		return d.Degraded()
	}
	return false
}

// Source reports which backend actually served the most recent read.
func (s *PeriodMatrixService) Source() string {
	if s == nil || s.store == nil {
		return ""
	}
	if src, ok := s.store.(interface{ SourceBackend() string }); ok {
		return src.SourceBackend()
	}
	return ""
}

// Matrix returns the agent × period performance matrix with a 60s TTL.
func (s *PeriodMatrixService) Matrix() (*portfolio.PeriodPerformanceMatrix, error) {
	if s == nil || s.store == nil {
		return &portfolio.PeriodPerformanceMatrix{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.matrixHit != nil && time.Since(s.matrixAt) < periodMatrixCacheTTL {
		return s.matrixHit, nil
	}

	outcomes, err := s.store.LoadOutcomesFromSessions()
	if err != nil {
		logging.Warn("period_matrix", "load_outcomes_failed", logging.Err(err))
		return nil, err
	}
	matrix := portfolio.BuildPeriodPerformanceMatrix(outcomes, portfolio.PeriodMatrixMinSamplesDefault)
	s.matrixHit = &matrix
	s.matrixAt = time.Now()
	return s.matrixHit, nil
}
