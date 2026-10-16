package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ExperimentsJSONL reads every record in experiments.jsonl (best-effort;
// malformed lines are skipped). Shared by the auto-experiment drain logic and
// the orchestrator's backlog cap.
func ExperimentsJSONL(ledgerDir string) []domain.ExperimentRecord {
	path := filepath.Join(ledgerDir, "experiments.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var records []domain.ExperimentRecord
	for line := range strings.SplitSeq(string(data), "\n") {
		if line == "" {
			continue
		}
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			records = append(records, rec)
		}
	}
	return records
}

// LatestExperimentStatusByID folds an append-only record stream into the
// latest status per experiment ID (last write wins).
func LatestExperimentStatusByID(records []domain.ExperimentRecord) map[string]domain.ExperimentStatus {
	latest := make(map[string]domain.ExperimentStatus, len(records))
	for _, rec := range records {
		if rec.ID == "" {
			continue
		}
		latest[rec.ID] = rec.Status
	}
	return latest
}

// CountUnresolvedPlanned returns how many experiment IDs are still in
// "planned" state after folding duplicates (last write wins). Used to cap
// new planned-experiment creation (fix manifest #D01).
func CountUnresolvedPlanned(ledgerDir string) int {
	return CountUnresolvedPlannedIn(ExperimentsJSONL(ledgerDir))
}

// CountUnresolvedPlannedIn counts the IDs still in "planned" state after
// folding an already-loaded record stream (last write wins). Split out of
// CountUnresolvedPlanned so a store-backed reader can reuse the same fold
// instead of re-reading the file (Q2 batch 2 unit A).
func CountUnresolvedPlannedIn(records []domain.ExperimentRecord) int {
	n := 0
	for _, st := range LatestExperimentStatusByID(records) {
		if st == domain.ExperimentPlanned {
			n++
		}
	}
	return n
}

// ExperimentRecordReader is the read half of the experiments record stream.
// ledger.ExperimentStore satisfies it; the (file, store) pair is what
// LoadExperimentRecords switches on.
type ExperimentRecordReader interface {
	LoadExperiments() ([]domain.ExperimentRecord, error)
}

// LoadExperimentRecords reads the experiments family's record stream through
// the store when one is available, and through experiments.jsonl when it is
// not (store == nil ⇒ the pre-migration behavior, byte-for-byte).
//
// Why this exists (Q2 batch 2 unit A): the record half of the family is written
// by two different producers. The replay pipeline
// (internal/orchestrator/system.go) writes through the backend-aware
// ledger.NewOutcomeStore — PostgreSQL on production — while the auto-experiment
// drain/expiry used to read experiments.jsonl. Those two diverge: production's
// JSONL file stopped at 2026-10-13T23:39Z while PostgreSQL kept receiving rows
// (latest id suffix 2026-10-15T05:41Z), so the drain was selecting candidates
// from a frozen file. Reading through the same store the producer writes
// removes that split.
//
// Ordering: the two backends return opposite orders (the JSONL store returns
// file order = oldest first; SQLiteStore/PostgresLedgerStore both use
// ORDER BY id DESC = newest first). The stream is therefore re-sorted into a
// backend-independent OLDEST-FIRST order, keyed on the unix suffix embedded in
// the experiment id (the family's existing convention — see
// experimentRecordIsOld and experimentRecordTime). A record without a usable
// suffix sorts as the oldest, matching that convention. The sort is stable, so
// records that share an id (and therefore share their suffix) keep their input
// order — which is what the last-write-wins fold needs.
//
// Error handling: the store path returns its error instead of silently falling
// back to the file. A JSONL fallback would answer with the frozen file this
// function exists to bypass, i.e. the same silent-staleness mode as the T1
// signal-engine incident. Callers decide (log and skip the step).
func LoadExperimentRecords(store ExperimentRecordReader, ledgerDir string) ([]domain.ExperimentRecord, error) {
	var records []domain.ExperimentRecord
	if store != nil {
		loaded, err := store.LoadExperiments()
		if err != nil {
			return nil, fmt.Errorf("load experiment records: %w", err)
		}
		records = loaded
	} else {
		records = ExperimentsJSONL(ledgerDir)
	}
	return SortExperimentRecordsOldestFirst(records), nil
}

// SortExperimentRecordsOldestFirst returns records ordered by the unix suffix
// embedded in the experiment id, ascending, preserving input order on ties
// (stable). See LoadExperimentRecords for why this order is the canonical one.
func SortExperimentRecordsOldestFirst(records []domain.ExperimentRecord) []domain.ExperimentRecord {
	out := make([]domain.ExperimentRecord, len(records))
	copy(out, records)
	sort.SliceStable(out, func(i, j int) bool {
		return experimentIDSuffix(out[i].ID) < experimentIDSuffix(out[j].ID)
	})
	return out
}

// experimentIDSuffix parses the trailing "-<unix>" of an experiment id.
// Records without one (or with an implausible value) return 0 = oldest, the
// same "treat as old" convention as internal/experiment.experimentRecordIsOld.
func experimentIDSuffix(id string) int64 {
	i := strings.LastIndex(id, "-")
	if i < 0 || i+1 >= len(id) {
		return 0
	}
	ts, err := strconv.ParseInt(id[i+1:], 10, 64)
	if err != nil || ts <= 1_000_000_000 {
		return 0
	}
	return ts
}
