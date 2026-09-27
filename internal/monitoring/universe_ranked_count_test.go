package monitoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file pins the assertion that was missing until 2026-09-27 (audit gap D):
// nothing compared the size of the `ranked` array with the `symbols_ranked`
// count that is persisted beside it in the same artifact.
//
// The artifact carries both readings, and its readers split between them: the
// agents.json-compatible view and a human count entries, while the coverage
// check, `-build-universe status` and the verifier's L3 trust the count. A
// disagreement therefore means two readers of one file are looking at two
// different mother universes, and neither reading is wrong. The verifier printed
// the pair side by side without judging it; no rule or test asserted agreement.
//
// Two things are pinned here:
//   - the judgement (RankedCountConflict), including the nil-result case;
//   - the wiring: SaveUniverseSnapshot must actually leave the contradiction
//     visible (a log line an operator can grep) while still writing the file,
//     because the file is the only thing downstream readers have. Refusing to
//     write would turn a bookkeeping defect into an outage.

func TestRankedCountConflict(t *testing.T) {
	tests := []struct {
		name              string
		result            *UniverseBuildResult
		ranked            []RankedSymbol
		wantDeclared      int
		wantPersisted     int
		wantConflict      bool
		mutationDiffersAt bool
	}{
		{
			name:          "the invariant the pipeline maintains: count equals list length",
			result:        &UniverseBuildResult{SymbolsRanked: 3},
			ranked:        []RankedSymbol{{Symbol: "2330"}, {Symbol: "2317"}, {Symbol: "2454"}},
			wantDeclared:  3,
			wantPersisted: 3,
		},
		{
			name:              "the count claims more symbols than were persisted",
			result:            &UniverseBuildResult{SymbolsRanked: 3},
			ranked:            []RankedSymbol{{Symbol: "2330"}},
			wantDeclared:      3,
			wantPersisted:     1,
			wantConflict:      true,
			mutationDiffersAt: true,
		},
		{
			name:              "the list holds more symbols than the count admits",
			result:            &UniverseBuildResult{SymbolsRanked: 1},
			ranked:            []RankedSymbol{{Symbol: "2330"}, {Symbol: "2317"}},
			wantDeclared:      1,
			wantPersisted:     2,
			wantConflict:      true,
			mutationDiffersAt: true,
		},
		{
			name:          "a nil result with no list is not a contradiction",
			result:        nil,
			ranked:        nil,
			wantDeclared:  0,
			wantPersisted: 0,
		},
		{
			name:              "a nil result with a non-empty list cannot declare a count",
			result:            nil,
			ranked:            []RankedSymbol{{Symbol: "2330"}},
			wantDeclared:      0,
			wantPersisted:     1,
			wantConflict:      true,
			mutationDiffersAt: true,
		},
	}

	// mutated is the guard with its comparison deleted — the shape a future edit
	// would most plausibly produce (the function would keep compiling and keep
	// returning a plausible pair). The self-proof below asserts it disagrees with
	// the real guard on every row that carries a contradiction, which is what
	// makes this table a guard instead of a description.
	mutated := func(*UniverseBuildResult, []RankedSymbol) bool { return false }

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			declared, persisted, conflict := RankedCountConflict(tc.result, tc.ranked)
			if declared != tc.wantDeclared || persisted != tc.wantPersisted || conflict != tc.wantConflict {
				t.Fatalf("RankedCountConflict() = (%d, %d, %v), want (%d, %d, %v)",
					declared, persisted, conflict, tc.wantDeclared, tc.wantPersisted, tc.wantConflict)
			}
			if got := mutated(tc.result, tc.ranked); got == conflict && tc.mutationDiffersAt {
				t.Errorf("mutation not caught: deleting the comparison kept the answer %v", got)
			}
		})
	}
}

// TestSaveUniverseSnapshot_MismatchIsReportedAndStillWritten is the wiring half.
// A judgement that no caller consults is not an assertion, and a writer that
// refuses to write is not an option here.
func TestSaveUniverseSnapshot_MismatchIsReportedAndStillWritten(t *testing.T) {
	t.Run("consistent pair writes silently", func(t *testing.T) {
		buf := captureLogs(t)
		workDir := t.TempDir()
		result := &UniverseBuildResult{SymbolsRanked: 2, SymbolsBuilt: 1599}
		ranked := []RankedSymbol{{Symbol: "2330"}, {Symbol: "2317"}}
		if err := SaveUniverseSnapshot(workDir, result, ranked); err != nil {
			t.Fatalf("SaveUniverseSnapshot: %v", err)
		}
		if strings.Contains(buf.String(), "universe_ranked_count_mismatch") {
			t.Errorf("a consistent pair produced a mismatch warning: %s", buf.String())
		}
		snap, err := LoadUniverseSnapshot(workDir)
		if err != nil {
			t.Fatalf("LoadUniverseSnapshot: %v", err)
		}
		if len(snap.Ranked) != result.SymbolsRanked {
			t.Errorf("round trip lost symbols: ranked=%d declared=%d", len(snap.Ranked), result.SymbolsRanked)
		}
	})

	t.Run("contradictory pair is reported, and still written", func(t *testing.T) {
		buf := captureLogs(t)
		workDir := t.TempDir()
		result := &UniverseBuildResult{SymbolsRanked: 3, SymbolsBuilt: 1599}
		ranked := []RankedSymbol{{Symbol: "2330"}} // one entry, declared three

		if err := SaveUniverseSnapshot(workDir, result, ranked); err != nil {
			t.Fatalf("SaveUniverseSnapshot must not fail on a contradiction (the file is what readers have): %v", err)
		}

		logged := buf.String()
		if !strings.Contains(logged, "universe_ranked_count_mismatch") {
			t.Fatalf("the contradiction left no trace for an operator: %s", logged)
		}
		for _, want := range []string{"declared_symbols_ranked=3", "persisted_ranked_entries=1"} {
			if !strings.Contains(logged, want) {
				t.Errorf("the warning must carry %q so the two readings can be compared, got: %s", want, logged)
			}
		}
		if !strings.Contains(logged, UniverseSnapshotPath(workDir)) {
			t.Errorf("the warning must name the artifact it is about, got: %s", logged)
		}

		// Non-fatal means the artifact exists and still contains the
		// contradiction: the guard reports, it does not silently rewrite data.
		snap, err := LoadUniverseSnapshot(workDir)
		if err != nil {
			t.Fatalf("the artifact was not written: %v", err)
		}
		if len(snap.Ranked) != 1 || snap.Result == nil || snap.Result.SymbolsRanked != 3 {
			t.Errorf("the artifact must be written as handed over: ranked=%d symbols_ranked=%v",
				len(snap.Ranked), snap.Result)
		}
		if _, err := os.Stat(filepath.Join(workDir, "data", "state", "universe_snapshot.json")); err != nil {
			t.Errorf("canonical artifact missing: %v", err)
		}
	})
}

// TestUniverseRegistryAgentsMatchPersistedRanked pins the same invariant for the
// SECOND artifact, whose shape is derived from the ranked list rather than
// carrying its own count. It is a cheap cross-check that the registry cannot
// describe a different mother universe than the snapshot written in the same
// step — the pairing that had no detector until the registry gauge existed.
func TestUniverseRegistryAgentsMatchPersistedRanked(t *testing.T) {
	workDir := t.TempDir()
	ranked := []RankedSymbol{
		{Symbol: "2330", Score: 91, Industry: "Semiconductor"},
		{Symbol: "2317", Score: 88, Industry: "Electronics"},
	}
	result := &UniverseBuildResult{SymbolsRanked: len(ranked), Timestamp: time.Now()}
	if err := SaveUniverseSnapshot(workDir, result, ranked); err != nil {
		t.Fatalf("SaveUniverseSnapshot: %v", err)
	}
	if err := WriteUniverseRegistry(UniverseRegistryPath(workDir), result, ranked, 1); err != nil {
		t.Fatalf("WriteUniverseRegistry: %v", err)
	}
	reg, err := LoadUniverseRegistry(UniverseRegistryPath(workDir))
	if err != nil {
		t.Fatalf("LoadUniverseRegistry: %v", err)
	}
	snap, err := LoadUniverseSnapshot(workDir)
	if err != nil {
		t.Fatalf("LoadUniverseSnapshot: %v", err)
	}
	if len(reg.Agents) != len(snap.Ranked) {
		t.Fatalf("registry describes %d symbols, snapshot describes %d", len(reg.Agents), len(snap.Ranked))
	}
	for i := range snap.Ranked {
		if len(reg.Agents[i].Universe) == 0 {
			t.Errorf("registry agent %d (%s) carries no universe", i, reg.Agents[i].ID)
		}
	}
}
