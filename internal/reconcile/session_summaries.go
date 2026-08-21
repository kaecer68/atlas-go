// Package reconcile provides the one-key reconciliation (B6) between the two
// session-summary backends: PostgreSQL (session_summaries table) and the JSONL
// ledger (sessions/<id>/summary.json).
//
// Dual-write can split silently: the audited production state had PG holding
// 07-02..07-23 while JSONL held 07-24..08-21. The write path now warns and
// counts one-sided failures (see repository.SessionSummaryReconcilePendingTotal),
// and this package is the durable fix: it loads both sides, computes the
// symmetric difference, and (with Apply) backfills the one-sided gaps.
//
// Trade-off documented: backfill only repairs SESSION-ID gaps (a session
// present on one side, missing on the other). Sessions present on BOTH sides
// with drifted content are reported as conflicts and never auto-overwritten —
// a human reviews them (the merge read path already resolves reads via
// newer-RecordedAt-wins / PG tiebreak).
package reconcile

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PGRepo is the PostgreSQL side of a session-summary reconcile.
type PGRepo interface {
	LoadAllSessionSummaries(ctx context.Context) ([]domain.SessionSummary, error)
	SaveSessionSummary(ctx context.Context, summary domain.SessionSummary) error
}

// JSONLRepo is the JSONL/ledger side of a session-summary reconcile.
type JSONLRepo interface {
	LoadSessionSummaries() ([]domain.SessionSummary, error)
	RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error
}

// Options controls a reconcile run.
type Options struct {
	// Apply backfills one-sided gaps. false = dry-run (report only).
	Apply bool
}

// Conflict is a session present on both sides whose summaries differ.
type Conflict struct {
	SessionID string
	PG        domain.SessionSummary
	JSONL     domain.SessionSummary
}

// Diff is the symmetric difference between the two backends.
type Diff struct {
	// OnlyPG are session IDs present in PG and missing from JSONL.
	OnlyPG []string
	// OnlyJSONL are session IDs present in JSONL and missing from PG.
	OnlyJSONL []string
	// Conflicts are sessions present on both sides with drifted content.
	Conflicts []Conflict
	// PGCount / JSONLCount are the total summaries read from each side.
	PGCount    int
	JSONLCount int
}

// Empty reports whether the two sides are perfectly consistent (no gaps, no
// content drift).
func (d Diff) Empty() bool {
	return len(d.OnlyPG) == 0 && len(d.OnlyJSONL) == 0 && len(d.Conflicts) == 0
}

// Result is the outcome of a reconcile run.
type Result struct {
	Diff Diff

	// BackfilledToPG / BackfilledToJSONL count one-sided gaps repaired by an
	// Apply run.
	BackfilledToPG    int
	BackfilledToJSONL int

	// Errors collects per-backfill failures (Apply continues past them).
	Errors []string
}

// Run loads both sides, computes the symmetric difference, and — when
// opts.Apply is set — backfills the one-sided gaps (JSONL-missing into PG,
// PG-missing into JSONL). Conflicts are never auto-resolved.
func Run(ctx context.Context, pg PGRepo, jsonl JSONLRepo, opts Options) (Result, error) {
	var res Result

	pgSummaries, err := pg.LoadAllSessionSummaries(ctx)
	if err != nil {
		return res, fmt.Errorf("load PG summaries: %w", err)
	}
	jsonlSummaries, err := jsonl.LoadSessionSummaries()
	if err != nil {
		return res, fmt.Errorf("load JSONL summaries: %w", err)
	}

	res.Diff = Compare(pgSummaries, jsonlSummaries)

	if !opts.Apply {
		return res, nil
	}

	pgByID := indexBySessionID(pgSummaries)
	jsonlByID := indexBySessionID(jsonlSummaries)

	for _, id := range res.Diff.OnlyJSONL {
		if err := pg.SaveSessionSummary(ctx, jsonlByID[id]); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("backfill PG %s: %v", id, err))
			continue
		}
		res.BackfilledToPG++
	}
	for _, id := range res.Diff.OnlyPG {
		if err := jsonl.RecordSessionSummary(domain.ReplaySession{ID: id}, pgByID[id]); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("backfill JSONL %s: %v", id, err))
			continue
		}
		res.BackfilledToJSONL++
	}
	return res, nil
}

// Compare indexes both sides by SessionID and computes the symmetric
// difference plus content conflicts. SessionID is the identity key; the
// newest duplicate wins when a side lists the same ID twice.
func Compare(pg, jsonl []domain.SessionSummary) Diff {
	pgByID := indexBySessionID(pg)
	jsonlByID := indexBySessionID(jsonl)

	var d Diff
	d.PGCount = len(pgByID)
	d.JSONLCount = len(jsonlByID)

	for id := range pgByID {
		if _, ok := jsonlByID[id]; !ok {
			d.OnlyPG = append(d.OnlyPG, id)
		}
	}
	for id := range jsonlByID {
		if _, ok := pgByID[id]; !ok {
			d.OnlyJSONL = append(d.OnlyJSONL, id)
		}
	}
	for id, p := range pgByID {
		j, ok := jsonlByID[id]
		if !ok {
			continue
		}
		if summariesConflict(p, j) {
			d.Conflicts = append(d.Conflicts, Conflict{SessionID: id, PG: p, JSONL: j})
		}
	}

	sort.Strings(d.OnlyPG)
	sort.Strings(d.OnlyJSONL)
	sort.Slice(d.Conflicts, func(i, j int) bool { return d.Conflicts[i].SessionID < d.Conflicts[j].SessionID })
	return d
}

func indexBySessionID(summaries []domain.SessionSummary) map[string]domain.SessionSummary {
	out := make(map[string]domain.SessionSummary, len(summaries))
	for _, s := range summaries {
		// Skip empty session IDs: the JSONL loader already drops them (they
		// are corrupted rows), and backfilling "" would write
		// sessions//summary.json outside any session directory.
		if s.SessionID == "" {
			continue
		}
		out[s.SessionID] = s // newest duplicate wins by insertion order
	}
	return out
}

// summariesConflict reports whether two summaries for the same session carry
// different content (timestamp compared via time.Equal, other fields via
// DeepEqual after zeroing the timestamp).
func summariesConflict(a, b domain.SessionSummary) bool {
	if !a.RecordedAt.Equal(b.RecordedAt) {
		return true
	}
	a.RecordedAt, b.RecordedAt = time.Time{}, time.Time{}
	return !reflect.DeepEqual(a, b)
}
