package capitalflow

// BK-15 — Date-keyed atomic persistence for the rolling-window
// reference used by ForceExtractor's Z-score computation (spec §8.5 /
// §8.2 / CF-INV-05).
//
// Until this task landed, the rolling window lived entirely in
// process memory (rollingWindow in types.go): every Extract() pushed
// a value, so two reads of the same snapshot drifted (spec §11.3).
// The store below moves the window onto disk with a date-keyed
// uniqueness guarantee, so that:
//
//   - one (dimension, trading_date) pair has at most one sample,
//   - the window survives process restart,
//   - History(...) returns samples with TradingDate strictly before
//     the given reference date, ascending, newest limit entries,
//   - write failures never leave a partial file behind.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Source-ID constants for the first-party source registry defined in
// docs/specs/capital-flow-seven-dimension-spec.md §5. These literals
// are referenced from tests and from production writers; renaming
// any of them is a breaking change for downstream consumers.
const (
	// SourceTWSET86 is the TWSE "三大法人買賣超日報" (T86) feed. This
	// is the only source currently emitted by the in-process Extract
	// path; the others are reserved for upcoming channel integrations
	// so the BK-15 registry does not need a follow-up rename.
	SourceTWSET86 = "SRC-TWSE-T86"

	SourceTAIFEXInst         = "SRC-TAIFEX-INST"
	SourceTWSEODDLOT         = "SRC-TWSE-MARGN"
	SourceGovernmentOperator = "SRC-OPERATOR-IMPORTED"
	SourceSECTSMC            = "SRC-SEC-TSMC"
	SourceYahoo              = "SRC-YAHOO-TSM-DERIVED"

	// SourceFinMindFutOI is the FinMind TaiwanFuturesInstitutionalInvestors
	// feed (FinMind's mirror of the TAIFEX institutional futures OI
	// report). Used by the history import for the ForceFutures dimension
	// when the snapshot source is the FinMind backfill channel.
	SourceFinMindFutOI = "SRC-FINMIND-FUT-OI"
)

// currentRollingStateVersion is the schema version this code emits
// and understands. Bumping it requires a migration path inside
// loadLocked so older on-disk files keep loading.
const currentRollingStateVersion = 1

// rollingStateFile is the on-disk shape of the persisted rolling
// window. Versioning lets us extend the schema without losing the
// samples stored under prior versions.
type rollingStateFile struct {
	Version int                           `json:"version"`
	Samples map[ForceName][]RollingSample `json:"samples"`
}

// RollingSampleStore is the date-keyed persistence contract for the
// rolling-window Z-score reference (spec §8.5).
//
// Invariants:
//
//   - At most one sample per (dimension, trading_date) (CF-INV-05).
//   - History returns a deep copy so callers cannot mutate persisted
//     state through the returned slice.
//   - History applies TradingDate < beforeDate strictly; the boundary
//     date is excluded so that "today's observation" never bleeds
//     into its own reference window (spec §8.4).
type RollingSampleStore interface {
	History(ctx context.Context, dimension ForceName, beforeDate string, limit int) ([]RollingSample, error)
	UpsertDay(ctx context.Context, tradingDate string, samples []RollingSample) error
	ImportHistory(ctx context.Context, samples []RollingSample) error
}

// ---------------------------------------------------------------------------
// FileRollingSampleStore
// ---------------------------------------------------------------------------

// FileRollingSampleStore persists the rolling window to a single JSON
// file. Writes go through a sibling ".tmp" file (mode 0o644) followed
// by os.Rename, so a crash mid-write either leaves the previous good
// state or — at worst — a stray tmp file that the next load ignores.
type FileRollingSampleStore struct {
	path     string
	capacity int
	mu       sync.Mutex
}

// NewFileRollingSampleStore constructs a store backed by the JSON
// file at path. capacity bounds the per-dimension sample count; the
// oldest sample is dropped when a new UpsertDay would exceed it.
//
// The file need not exist; an empty state is created on the first
// write. A capacity of 0 disables trimming (only recommended for
// tests; production wires a positive capacity through configuration).
func NewFileRollingSampleStore(path string, capacity int) *FileRollingSampleStore {
	return &FileRollingSampleStore{path: path, capacity: capacity}
}

// History returns a deep-copied, ascending-ordered slice of samples
// for dimension whose TradingDate is strictly less than beforeDate,
// capped at the newest limit entries.
func (s *FileRollingSampleStore) History(_ context.Context, dimension ForceName, beforeDate string, limit int) ([]RollingSample, error) {
	if err := validateBeforeDate(beforeDate); err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("rolling_store: history limit %d < 0", limit)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	return filterAndCopy(state.Samples[dimension], beforeDate, limit), nil
}

// UpsertDay replaces any existing sample whose TradingDate equals
// tradingDate for each dimension present in samples, appends the new
// observation, then sorts and trims each affected dimension before
// writing the whole state back atomically.
//
// Per spec §8.2 / CF-INV-05 the "last write wins" rule collapses
// duplicates on the same trading date into a single sample.
func (s *FileRollingSampleStore) UpsertDay(_ context.Context, tradingDate string, samples []RollingSample) error {
	if err := validateTradingDate(tradingDate); err != nil {
		return err
	}
	for i := range samples {
		if samples[i].TradingDate != tradingDate {
			return fmt.Errorf("rolling_store: sample trading_date %q != %q", samples[i].TradingDate, tradingDate)
		}
		if err := validateDimension(samples[i].Dimension); err != nil {
			return fmt.Errorf("rolling_store: sample[%d]: %w", i, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	applyUpsert(state, tradingDate, samples, s.capacity)
	return s.persistLocked(state)
}

// ImportHistory bulk-loads historical samples into the store with a
// single atomic write (CAL-1). Every sample follows UpsertDay
// semantics: the (dimension, trading_date) pair is deduplicated
// ("last write wins" per CF-INV-05), the dimension slice is re-sorted
// ascending by TradingDate, and trimmed to capacity. Unlike UpsertDay
// (one trading date per call) the batch may span many trading dates;
// the whole batch is validated first, then applied under a single lock
// and persisted exactly once, so a failure mid-import can never leave
// a partially-updated file behind.
func (s *FileRollingSampleStore) ImportHistory(_ context.Context, samples []RollingSample) error {
	if len(samples) == 0 {
		return nil
	}
	for i := range samples {
		if err := validateTradingDate(samples[i].TradingDate); err != nil {
			return fmt.Errorf("rolling_store: import sample[%d]: %w", i, err)
		}
		if err := validateDimension(samples[i].Dimension); err != nil {
			return fmt.Errorf("rolling_store: import sample[%d]: %w", i, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return err
	}
	applyImport(state, samples, s.capacity)
	return s.persistLocked(state)
}

// loadLocked reads the persisted JSON state. Missing file is not an
// error — it returns an empty state with the current schema version
// so the first UpsertDay can write through cleanly. Callers must
// hold s.mu.
func (s *FileRollingSampleStore) loadLocked() (*rollingStateFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &rollingStateFile{
				Version: currentRollingStateVersion,
				Samples: make(map[ForceName][]RollingSample),
			}, nil
		}
		return nil, fmt.Errorf("rolling_store: read %s: %w", s.path, err)
	}
	var state rollingStateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("rolling_store: parse %s: %w", s.path, err)
	}
	if state.Version == 0 {
		// Pre-versioning files are still v1 in spirit; we only wrote
		// Version explicitly from this task onward, but be lenient on
		// older files that may have been hand-rolled in development.
		state.Version = currentRollingStateVersion
	}
	if state.Samples == nil {
		state.Samples = make(map[ForceName][]RollingSample)
	}
	return &state, nil
}

// persistLocked writes state to a sibling tmp file (mode 0o644),
// closes it, then renames it over the canonical path. Any error
// before the rename removes the tmp file so a partial write cannot
// poison subsequent loads. Callers must hold s.mu.
func (s *FileRollingSampleStore) persistLocked(state *rollingStateFile) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("rolling_store: marshal state: %w", err)
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("rolling_store: create tmp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Default: clean up the tmp file if anything goes wrong before
	// the rename lands. The successful branch flips this flag so a
	// panic between tmp.Close and os.Rename still leaves the
	// canonical file intact.
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpName)
		}
	}()
	// Best-effort chmod; on platforms / filesystems where the
	// CreateTemp default already matches (0600 in a 0644-umask dir),
	// chmod failure is non-fatal — record it but continue so a
	// permission oddity never blocks the rolling-window writer.
	if err := tmp.Chmod(0o644); err != nil && !errors.Is(err, fs.ErrPermission) {
		_ = tmp.Close()
		return fmt.Errorf("rolling_store: chmod %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("rolling_store: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("rolling_store: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rolling_store: rename %s -> %s: %w", tmpName, s.path, err)
	}
	cleanupTmp = false
	return nil
}

// ---------------------------------------------------------------------------
// MemoryRollingSampleStore
// ---------------------------------------------------------------------------

// MemoryRollingSampleStore is the non-persistent in-memory counterpart
// of FileRollingSampleStore. It exists for short-lived processes and
// for callers (tests, simulation runs) that need the same contract
// without touching disk.
type MemoryRollingSampleStore struct {
	capacity int
	mu       sync.Mutex
	samples  map[ForceName][]RollingSample
}

// NewMemoryRollingSampleStore returns an empty in-memory store with
// the given per-dimension capacity. capacity<=0 disables trimming.
func NewMemoryRollingSampleStore(capacity int) *MemoryRollingSampleStore {
	return &MemoryRollingSampleStore{
		capacity: capacity,
		samples:  make(map[ForceName][]RollingSample),
	}
}

// History returns a deep-copied, ascending-ordered slice of in-memory
// samples for dimension with TradingDate strictly before beforeDate,
// capped at the newest limit entries.
func (s *MemoryRollingSampleStore) History(_ context.Context, dimension ForceName, beforeDate string, limit int) ([]RollingSample, error) {
	if err := validateBeforeDate(beforeDate); err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, fmt.Errorf("rolling_store: history limit %d < 0", limit)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return filterAndCopy(s.samples[dimension], beforeDate, limit), nil
}

// UpsertDay replaces any existing sample on tradingDate for each
// dimension present in samples, appends the new one, and trims to
// capacity. Identical semantics to FileRollingSampleStore.UpsertDay
// minus the disk write.
func (s *MemoryRollingSampleStore) UpsertDay(_ context.Context, tradingDate string, samples []RollingSample) error {
	if err := validateTradingDate(tradingDate); err != nil {
		return err
	}
	for i := range samples {
		if samples[i].TradingDate != tradingDate {
			return fmt.Errorf("rolling_store: sample trading_date %q != %q", samples[i].TradingDate, tradingDate)
		}
		if err := validateDimension(samples[i].Dimension); err != nil {
			return fmt.Errorf("rolling_store: sample[%d]: %w", i, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	applyUpsert(&rollingStateFile{Samples: s.samples}, tradingDate, samples, s.capacity)
	return nil
}

// ImportHistory bulk-loads historical samples with the same per-date
// UpsertDay semantics as FileRollingSampleStore.ImportHistory, minus
// the disk write: same-day dedup per dimension, ascending sort, and
// capacity trim. The whole batch is validated first and applied under
// a single lock so concurrent readers never observe a partial import.
func (s *MemoryRollingSampleStore) ImportHistory(_ context.Context, samples []RollingSample) error {
	if len(samples) == 0 {
		return nil
	}
	for i := range samples {
		if err := validateTradingDate(samples[i].TradingDate); err != nil {
			return fmt.Errorf("rolling_store: import sample[%d]: %w", i, err)
		}
		if err := validateDimension(samples[i].Dimension); err != nil {
			return fmt.Errorf("rolling_store: import sample[%d]: %w", i, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	applyImport(&rollingStateFile{Samples: s.samples}, samples, s.capacity)
	return nil
}

// ---------------------------------------------------------------------------
// Shared helpers (used by both store implementations)
// ---------------------------------------------------------------------------

// applyUpsert mutates state in place so that, for every sample:
//
//  1. any pre-existing sample on the same trading date is dropped
//     ("last write wins" per CF-INV-05),
//  2. the new sample is appended,
//  3. the dimension's slice is re-sorted ascending by TradingDate,
//  4. the slice is trimmed to the most-recent capacity entries.
//
// capacity<=0 disables the trim step.
func applyUpsert(state *rollingStateFile, tradingDate string, samples []RollingSample, capacity int) {
	for _, sample := range samples {
		existing := state.Samples[sample.Dimension]
		filtered := make([]RollingSample, 0, len(existing)+1)
		for _, e := range existing {
			if e.TradingDate != tradingDate {
				filtered = append(filtered, e)
			}
		}
		filtered = append(filtered, sample)
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].TradingDate < filtered[j].TradingDate
		})
		if capacity > 0 && len(filtered) > capacity {
			filtered = filtered[len(filtered)-capacity:]
		}
		state.Samples[sample.Dimension] = filtered
	}
}

// applyImport applies a bulk batch to state in place, preserving the
// exact UpsertDay semantics per trading date: for each date the
// samples are deduplicated per dimension ("last write wins" per
// CF-INV-05), the dimension slice is re-sorted ascending by
// TradingDate, and trimmed to the most-recent capacity entries.
//
// Dates are processed in ascending order and the input order is
// preserved within a date, so when a batch contains two samples for
// the same (dimension, trading_date) the later one deterministically
// wins — identical to calling UpsertDay once per date in order.
func applyImport(state *rollingStateFile, samples []RollingSample, capacity int) {
	byDate := make(map[string][]RollingSample, len(samples))
	dates := make([]string, 0, len(samples))
	for _, s := range samples {
		if _, ok := byDate[s.TradingDate]; !ok {
			dates = append(dates, s.TradingDate)
		}
		byDate[s.TradingDate] = append(byDate[s.TradingDate], s)
	}
	sort.Strings(dates)
	for _, d := range dates {
		applyUpsert(state, d, byDate[d], capacity)
	}
}

// filterAndCopy returns a deep copy of samples with TradingDate
// strictly less than beforeDate, sorted ascending, capped at the
// newest limit entries. limit<=0 or beforeDate=="" returns nil.
//
// The copy matters: callers should never observe mutations of the
// store's internal slice when they read through History.
func filterAndCopy(samples []RollingSample, beforeDate string, limit int) []RollingSample {
	if beforeDate == "" || limit <= 0 || len(samples) == 0 {
		return nil
	}
	filtered := make([]RollingSample, 0, len(samples))
	for _, s := range samples {
		if s.TradingDate < beforeDate {
			filtered = append(filtered, s)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].TradingDate < filtered[j].TradingDate
	})
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	out := make([]RollingSample, len(filtered))
	copy(out, filtered)
	return out
}

// validateTradingDate checks that s parses as YYYY-MM-DD. The shape
// matches the TradingDate format used everywhere in spec §6 / §8.
func validateTradingDate(s string) error {
	if s == "" {
		return fmt.Errorf("rolling_store: trading_date is empty")
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("rolling_store: trading_date %q is not YYYY-MM-DD: %w", s, err)
	}
	return nil
}

// validateBeforeDate mirrors validateTradingDate but allows the
// empty string as "no upper bound". That convention lets internal
// callers request every persisted sample without inventing a far-
// future sentinel date.
func validateBeforeDate(s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("rolling_store: before_date %q is not YYYY-MM-DD: %w", s, err)
	}
	return nil
}

// validateDimension ensures the dimension is one of the seven
// known capital forces (spec §6 / §5 source registry). Rejecting
// unknown dimensions here keeps a typo at the writer from
// silently growing the JSON file with phantom keys.
func validateDimension(d ForceName) error {
	switch d {
	case ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail:
		return nil
	}
	return fmt.Errorf("rolling_store: unknown dimension %q", string(d))
}
