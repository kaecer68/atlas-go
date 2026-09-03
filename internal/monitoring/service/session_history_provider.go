package service

import (
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// HistoryPoint is one dated point of the L-cold session history (net value,
// cash and tax from a session summary). It is the shared currency of every
// monitoring endpoint that previously re-scanned sessions/*/summary.json by
// hand (SSOT spec §1 violation list).
type HistoryPoint struct {
	SessionID      string
	Date           time.Time
	RecordedAt     time.Time
	PortfolioValue float64
	EndingCash     float64
	TotalTaxPaid   float64
}

// BuildHistoryPoints converts raw session summaries into a date-sorted
// history series, skipping summaries that carry no equity data (zero
// PortfolioValue — legacy 0-value backfill rows that would otherwise
// collapse drawdown/VaR math to nonsense). Session ordering is by the
// session date parsed from the SessionID, matching the equity-curve
// semantics the performance report already locks in its golden tests.
func BuildHistoryPoints(summaries []domain.SessionSummary) []HistoryPoint {
	points := make([]HistoryPoint, 0, len(summaries))
	for _, s := range summaries {
		if s.SessionID == "" || s.PortfolioValue == 0 {
			continue
		}
		points = append(points, HistoryPoint{
			SessionID:      s.SessionID,
			Date:           domain.SessionDateFromID(s.SessionID),
			RecordedAt:     s.RecordedAt,
			PortfolioValue: s.PortfolioValue,
			EndingCash:     s.EndingCash,
			TotalTaxPaid:   s.TotalTaxPaid,
		})
	}
	slices.SortFunc(points, func(a, b HistoryPoint) int {
		return a.Date.Compare(b.Date)
	})
	return points
}

// PortfolioValues extracts the net-value series of a history (in date order).
func PortfolioValues(points []HistoryPoint) []float64 {
	out := make([]float64, len(points))
	for i, p := range points {
		out[i] = p.PortfolioValue
	}
	return out
}

// SessionReturns derives the daily (per-session) return series of a history.
// A point whose predecessor has no positive value contributes no return.
func SessionReturns(points []HistoryPoint) []float64 {
	out := make([]float64, 0, max(0, len(points)-1))
	for i := 1; i < len(points); i++ {
		if points[i-1].PortfolioValue > 0 {
			out = append(out, (points[i].PortfolioValue-points[i-1].PortfolioValue)/points[i-1].PortfolioValue)
		}
	}
	return out
}

// SessionHistoryProvider is the shared L-cold read path for monitoring
// endpoints that need session summaries / trades / outcomes (SSOT plan P1-1).
//
// It wraps ledger.NewReportOutcomeStore — the same PG-first factory that
// backs the performance report (docs/decisions/2026-08-23-performance-report-ssot.md):
// PostgreSQL is the single source of truth; the JSONL ledger is used only as
// a degraded fallback when PG is unavailable. Non-postgres backends keep
// their native store semantics. Providers expose Source()/Degraded() so
// handlers can label their responses (P1-3).
type SessionHistoryProvider struct {
	store ledger.OutcomeStore
}

// NewSessionHistoryProvider builds the provider from the normalized config.
func NewSessionHistoryProvider(cfg config.Config) (*SessionHistoryProvider, error) {
	store, err := ledger.NewReportOutcomeStore(cfg)
	if err != nil {
		return nil, err
	}
	return &SessionHistoryProvider{store: store}, nil
}

// Store exposes the underlying backend-aware outcome store.
func (p *SessionHistoryProvider) Store() ledger.OutcomeStore {
	if p == nil {
		return nil
	}
	return p.store
}

// Degraded reports whether the most recent read fell back to JSONL because
// the SSoT backend (PostgreSQL) was unavailable. False for stores that do
// not track degradation.
func (p *SessionHistoryProvider) Degraded() bool {
	if p == nil {
		return false
	}
	if d, ok := p.store.(interface{ Degraded() bool }); ok {
		return d.Degraded()
	}
	return false
}

// Source reports which backend actually served the most recent read:
// "postgres" normally, "jsonl" when degraded, "" for stores that do not
// report a source.
func (p *SessionHistoryProvider) Source() string {
	if p == nil {
		return ""
	}
	if s, ok := p.store.(interface{ SourceBackend() string }); ok {
		return s.SourceBackend()
	}
	return ""
}

// HistoryPoints loads the date-sorted session history from the SSoT backend.
func (p *SessionHistoryProvider) HistoryPoints() ([]HistoryPoint, error) {
	if p == nil || p.store == nil {
		return nil, nil
	}
	summaries, err := p.store.LoadSessionSummaries()
	if err != nil {
		logging.Warn("session_history", "load_summaries_failed", logging.Err(err))
		return nil, err
	}
	return BuildHistoryPoints(summaries), nil
}

// Trades loads all executed trades from the SSoT backend (PG trades table on
// production; JSONL trades.jsonl otherwise).
func (p *SessionHistoryProvider) Trades() ([]domain.TradeRecord, error) {
	if p == nil || p.store == nil {
		return nil, nil
	}
	return p.store.LoadAllSessionTrades()
}
