package ledger

// screened_symbols_rejects_only_test.go — pins the SEMANTICS of the
// sessions/<id>/screened_symbols.jsonl artifact (issue #1944 I36/T2).
//
// The filename says "screened symbols"; the file actually holds screening
// REJECTS only. That gap misled a live investigation (a T2 read of "this symbol
// is absent" was compatible with both "never screened" and "screened and
// passed"), so the semantics get a test rather than a comment: a comment can be
// deleted in a refactor without any test failing, and then the next reader is
// misled again.
//
// Two properties are pinned:
//  1. NEGATIVE — a symbol that was not rejected must not appear in the file at
//     all. This is what makes "absent ⇒ not rejected" true.
//  2. An empty reject list still creates an EMPTY file (the direct writer
//     truncates unconditionally), so "file exists" must not be read as
//     "something was rejected". The transaction path differs on purpose and is
//     pinned in transaction_test.go (no file at all when there are no rejects).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestScreenedSymbolsArtifact_IsRejectsOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	const sessionID = "session-20261001-daily"

	// Fixture: 1301.TW FAILED the momentum floor for mining-desk-01 (so it is a
	// reject and must be written); 2330.TW was screened by the same agent and
	// PASSED (so it was never in the reject slice and must NOT be written).
	rejected := domain.ScreeningReject{
		SessionID:      sessionID,
		Symbol:         "1301.TW",
		AgentID:        "mining-desk-01",
		Skill:          "mining_desk",
		Criterion:      "momentum_20d_min",
		CriterionLabel: "20-day momentum",
		Threshold:      "-0.50",
		ActualValue:    "-1.00",
		RecordedAt:     time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC),
	}
	if err := store.RecordSessionScreeningRejects(sessionID, []domain.ScreeningReject{rejected}); err != nil {
		t.Fatalf("RecordSessionScreeningRejects: %v", err)
	}

	// Positive half: the reject round-trips with the evidence an audit needs.
	loaded, err := store.LoadSessionScreeningRejects(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d rejects, want 1", len(loaded))
	}
	if loaded[0].Symbol != "1301.TW" || loaded[0].AgentID != "mining-desk-01" {
		t.Errorf("loaded reject = %+v, want the 1301.TW / mining-desk-01 record", loaded[0])
	}
	if loaded[0].Criterion != "momentum_20d_min" || loaded[0].Threshold != "-0.50" || loaded[0].ActualValue != "-1.00" {
		t.Errorf("criterion/threshold/actual must survive the round trip, got %q/%q/%q",
			loaded[0].Criterion, loaded[0].Threshold, loaded[0].ActualValue)
	}

	// NEGATIVE half (the pin): a symbol that passed screening must not be in the
	// artifact. If this ever fails, the file stopped being rejects-only and every
	// "absent ⇒ not rejected" reading in the codebase becomes wrong.
	raw, err := os.ReadFile(filepath.Join(dir, "sessions", sessionID, "screened_symbols.jsonl"))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	body := string(raw)
	if strings.Contains(body, "2330.TW") {
		t.Errorf("a symbol that PASSED screening appears in the rejects-only artifact:\n%s", body)
	}
	if !strings.Contains(body, "1301.TW") {
		t.Errorf("the rejected symbol is missing from the artifact:\n%s", body)
	}
	// One JSON object per line, one line here: the artifact is a stream of
	// rejects, and an accidental extra record would be a silent miscount.
	if lines := len(strings.Split(strings.TrimSpace(body), "\n")); lines != 1 {
		t.Errorf("artifact has %d lines, want 1 (one JSON object per reject)", lines)
	}
}

func TestScreenedSymbolsArtifact_EmptyRejectsStillCreatesAnEmptyFile(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir).(*Store)
	const sessionID = "session-20261002-daily"

	if err := store.RecordSessionScreeningRejects(sessionID, nil); err != nil {
		t.Fatalf("RecordSessionScreeningRejects(nil): %v", err)
	}

	path := filepath.Join(dir, "sessions", sessionID, "screened_symbols.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the direct writer must create the artifact even with no rejects: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("artifact size = %d, want 0 (nothing was rejected)", info.Size())
	}
	loaded, err := store.LoadSessionScreeningRejects(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionScreeningRejects on an empty artifact: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("loaded %d rejects from an empty artifact, want 0", len(loaded))
	}
}
