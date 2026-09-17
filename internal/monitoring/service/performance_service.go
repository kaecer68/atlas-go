package service

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/reporting"
)

// defaultPerformanceReportTTL bounds how long a generated performance report is
// reused. The report is derived from the ledger + window summaries, so a minute
// of staleness is harmless — while regenerating it per request is not: the
// production ledger holds 271,359 recommendation outcomes (644 MB of metadata),
// and one generation allocated gigabytes, OOM-killing the container in a
// restart loop (2026-09-17).
const defaultPerformanceReportTTL = 60 * time.Second

// perfCacheEntry holds one generated report for its TTL window.
type perfCacheEntry struct {
	report    *reporting.PerformanceReport
	expiresAt time.Time
}

// PerformanceService provides performance report generation operations.
type PerformanceService struct {
	store     ledger.OutcomeStore
	ledgerDir string

	// ttl bounds report reuse; 0 → defaultPerformanceReportTTL.
	ttl   time.Duration
	nowFn func() time.Time

	mu    sync.Mutex
	cache map[string]*perfCacheEntry
	sf    singleflight.Group
}

// NewPerformanceService creates a new PerformanceService backed by the given
// outcome store. The store is created by the caller via
// ledger.NewReportOutcomeStore(cfg): postgres backend reads PG first with a
// JSONL fallback + degraded marker (SSoT decision
// docs/decisions/2026-08-23-performance-report-ssot.md); other backends keep
// NewOutcomeStore semantics (perf-report-zero audit BL-01).
func NewPerformanceService(store ledger.OutcomeStore, ledgerDir string) *PerformanceService {
	return &PerformanceService{
		store:     store,
		ledgerDir: ledgerDir,
		cache:     make(map[string]*perfCacheEntry),
	}
}

// WithReportTTL overrides the report cache TTL (tests, tuning).
func (s *PerformanceService) WithReportTTL(d time.Duration) *PerformanceService {
	s.ttl = d
	return s
}

func (s *PerformanceService) clock() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

func (s *PerformanceService) reportTTL() time.Duration {
	if s.ttl > 0 {
		return s.ttl
	}
	return defaultPerformanceReportTTL
}

func (s *PerformanceService) cachedReport(period string, now time.Time) *reporting.PerformanceReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.cache[period]
	if entry == nil || !now.Before(entry.expiresAt) {
		return nil
	}
	return entry.report
}

// GetPerformanceReport generates (or serves from cache) a performance report for
// the given period. Supported periods: "30d", "90d", "1y", "all".
//
// Concurrent callers share a single generation, and the result is reused for the
// TTL. GetAgentContributions and GetRegimeBreakdown route through this method, so
// one dashboard page load no longer triggers three full expensive passes.
func (s *PerformanceService) GetPerformanceReport(period string) (*reporting.PerformanceReport, error) {
	if cached := s.cachedReport(period, s.clock()); cached != nil {
		return cached, nil
	}

	v, err, _ := s.sf.Do("perf:"+period, func() (any, error) {
		if cached := s.cachedReport(period, s.clock()); cached != nil {
			return cached, nil
		}
		report, err := reporting.GenerateReport(s.store, s.ledgerDir, period)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.cache == nil {
			s.cache = make(map[string]*perfCacheEntry)
		}
		s.cache[period] = &perfCacheEntry{report: report, expiresAt: s.clock().Add(s.reportTTL())}
		s.mu.Unlock()
		return report, nil
	})
	if err != nil {
		return nil, err
	}
	report, ok := v.(*reporting.PerformanceReport)
	if !ok || report == nil {
		return nil, fmt.Errorf("generate performance report: unexpected cache value %T", v)
	}
	return report, nil
}

// GetAgentContributions returns the top agent contributions for the given period.
func (s *PerformanceService) GetAgentContributions(period string) ([]reporting.AgentContribution, error) {
	report, err := s.GetPerformanceReport(period)
	if err != nil {
		return nil, err
	}
	return report.TopAgents, nil
}

// GetRegimeBreakdown returns the regime breakdown for the given period.
func (s *PerformanceService) GetRegimeBreakdown(period string) (*reporting.RegimeBreakdown, error) {
	report, err := s.GetPerformanceReport(period)
	if err != nil {
		return nil, err
	}
	return &report.RegimeBreakdown, nil
}
