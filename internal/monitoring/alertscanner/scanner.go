// Package alertscanner provides a startup-time alert scanner that
// reloads in-flight alerts from the persistence store so that
// process restarts do not leave operators blind to existing alerts.
//
// # Route C of the 2026-07-25 channel architecture audit.
//
// Usage:
//
//	scanner := alertscanner.New(store)
//	alerts, err := scanner.Scan(ctx)
//	if err != nil { /* log and continue */ }
package alertscanner

import (
	"context"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Store is the minimal read-only alert persistence contract
// needed by Scanner. Both monitoring.AlertStore (JSONL) and
// repository.AlertStore (Postgres) satisfy this interface.
type Store interface {
	LoadAll() ([]domain.AlertRecord, error)
	LoadUnacknowledged() ([]domain.AlertRecord, error)
}

// Scanner loads and filters in-flight alerts from a Store.
type Scanner struct {
	store Store
}

// New returns a Scanner backed by store.
func New(store Store) *Scanner {
	return &Scanner{store: store}
}

// Scan returns all unacknowledged alerts from the store, ordered by
// timestamp descending (newest first). Call this at process startup
// to surface alerts that survived a restart.
//
// Scan is non-blocking and tolerant of store errors: if the store
// returns an error the caller should log it and continue — the
// scanner is a best-effort utility, not a process health gate.
func (s *Scanner) Scan(ctx context.Context) ([]domain.AlertRecord, error) {
	records, err := s.store.LoadUnacknowledged()
	if err != nil {
		return nil, err
	}
	sortByTimestampDesc(records)
	return records, nil
}

// ScanActive returns all non-resolved alerts (triggered + acknowledged
// + silenced). When no alerts exist or the store file is absent the
// result is empty, never nil.
func (s *Scanner) ScanActive(ctx context.Context) ([]domain.AlertRecord, error) {
	all, err := s.store.LoadAll()
	if err != nil {
		return nil, err
	}
	var active []domain.AlertRecord
	for _, r := range all {
		if r.Status != domain.AlertStatusResolved {
			active = append(active, r)
		}
	}
	sortByTimestampDesc(active)
	return active, nil
}

// CountBySeverity returns a summary of unreacknowledged alerts grouped
// by severity level.
func (s *Scanner) CountBySeverity(ctx context.Context) (map[string]int, error) {
	records, err := s.Scan(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{
		"critical": 0,
		"error":    0,
		"warning":  0,
		"info":     0,
	}
	for _, r := range records {
		counts[r.Severity]++
	}
	return counts, nil
}

// HasBlockers returns true when there are unacknowledged alerts at
// CRITICAL or ERROR severity — the levels that should gate code
// changes per c1-integrated-report §3.5.
func (s *Scanner) HasBlockers(ctx context.Context) (bool, error) {
	counts, err := s.CountBySeverity(ctx)
	if err != nil {
		return false, err
	}
	return counts["critical"]+counts["error"] > 0, nil
}

// Snapshot bundles scan results with metadata for delivery to an
// AI agent context or monitoring endpoint.
type Snapshot struct {
	ScannedAt  time.Time            `json:"scanned_at"`
	Total      int                  `json:"total"`
	BySeverity map[string]int       `json:"by_severity"`
	Blocked    bool                 `json:"blocked"` // HasBlockers result
	Alerts     []domain.AlertRecord `json:"alerts,omitempty"`
}

// Snapshot runs Scan + CountBySeverity + HasBlockers and returns a
// self-contained snapshot suitable for MCP tool output or agent context.
func (s *Scanner) Snapshot(ctx context.Context) (*Snapshot, error) {
	records, err := s.Scan(ctx)
	if err != nil {
		return nil, err
	}
	counts, _ := s.CountBySeverity(ctx)
	blocked, _ := s.HasBlockers(ctx)

	return &Snapshot{
		ScannedAt:  time.Now().UTC(),
		Total:      len(records),
		BySeverity: counts,
		Blocked:    blocked,
		Alerts:     records,
	}, nil
}

func sortByTimestampDesc(records []domain.AlertRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Timestamp.After(records[j].Timestamp)
	})
}
