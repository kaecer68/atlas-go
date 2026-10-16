package main

// Hermetic tests for the JSONL-side experiment-family backfill (-experiments).
//
// No database is involved: every rule that decides what lands in PG — the
// append-only folding, the file classification, the skip accounting, the column
// mapping and the source-path defaults — is a pure function, and the PG handle
// only carries those values. The test that exercises the real upserts (and their
// idempotency) is experiments_integration_test.go, behind the integration tag.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// testExperimentIDPrefix marks every id this suite writes, so the integration
// test's cleanup can never touch real experiment rows.
const testExperimentIDPrefix = "exp-q2b2ebf-"

// experimentRecordFixture mirrors domain.ExperimentRecord as the producer writes
// it (ledger.Store.RecordExperiment → one experiments.jsonl line per status
// transition). marker lands in RevertReason, so two lines with the same id are
// distinguishable inside the stored JSON blob.
func experimentRecordFixture(id string, status domain.ExperimentStatus, marker string) domain.ExperimentRecord {
	return domain.ExperimentRecord{
		ID:               id,
		ProposalID:       "prop-" + id,
		CommitID:         "commit-" + id,
		TargetAgentID:    "migratetest-agent",
		Skill:            "growth_momentum",
		Hypothesis:       "tighten the exit rule",
		MutationType:     "prompt_tightening",
		AcceptanceGates:  []string{"improve_sharpe_like"},
		WindowStart:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:        time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		AcceptanceMetric: "sharpe_like",
		BaselineValue:    1.0,
		CandidateValue:   1.5,
		Status:           status,
		RevertReason:     marker,
	}
}

// promptResultFixture mirrors domain.PromptExperimentResult as written to
// <LedgerDir>/experiments/<experiment_id>.json.
func promptResultFixture(id string, status domain.ExperimentStatus) domain.PromptExperimentResult {
	return domain.PromptExperimentResult{
		Experiment:            experimentRecordFixture(id, status, "evaluated"),
		CandidatePrompt:       "candidate prompt for " + id,
		EvaluationMode:        "replay",
		PolicyChecks:          []string{"no_live_trading"},
		Notes:                 []string{"hermetic fixture"},
		BaselineObservations:  40,
		CandidateObservations: 40,
		RecordedAt:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
	}
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return data
}

// writeExperimentJSONLFixture writes one line per record plus any raw lines, in
// the order given (raw lines let a test place a malformed or blank line at an
// exact position).
func writeExperimentJSONLFixture(t *testing.T, path string, records []domain.ExperimentRecord, raw []string) {
	t.Helper()
	var buf bytes.Buffer
	for _, rec := range records {
		buf.Write(mustJSONBytes(t, rec))
		buf.WriteByte('\n')
	}
	for _, line := range raw {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write jsonl fixture %s: %v", path, err)
	}
}

func writeExperimentResultFixture(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

// TestFoldExperimentRecordsLastWriteWins is the folding contract: three lines
// for one id (planned → running → accepted) must produce ONE row whose blob and
// accepted flag come from the LAST line.
func TestFoldExperimentRecordsLastWriteWins(t *testing.T) {
	id := testExperimentIDPrefix + "fold"
	lines := []experimentLineRecord{
		{Line: 1, Record: experimentRecordFixture(id, domain.ExperimentPlanned, "planned")},
		{Line: 2, Record: experimentRecordFixture(id, domain.ExperimentRunning, "running")},
		{Line: 3, Record: experimentRecordFixture(id, domain.ExperimentAccepted, "accepted")},
	}

	folded, skipped := foldExperimentRecords(lines)
	if len(skipped) != 0 {
		t.Fatalf("valid records must not be skipped, got %+v", skipped)
	}
	if len(folded) != 1 {
		t.Fatalf("folded len = %d, want 1 (one row per experiment id)", len(folded))
	}
	if folded[0].Status != domain.ExperimentAccepted {
		t.Fatalf("folded status = %q, want %q (last write wins)", folded[0].Status, domain.ExperimentAccepted)
	}

	row, err := experimentRowValues(folded[0])
	if err != nil {
		t.Fatalf("experimentRowValues: %v", err)
	}
	if row.ExperimentID != id {
		t.Fatalf("row experiment_id = %q, want %q", row.ExperimentID, id)
	}
	if row.Accepted != 1 {
		t.Fatalf("row accepted = %d, want 1 (status %q)", row.Accepted, folded[0].Status)
	}
	if row.Timestamp != "2026-04-01T00:00:00Z" {
		t.Fatalf("row timestamp = %q, want the WindowStart RFC3339 value", row.Timestamp)
	}

	// mutation_brief_json must carry the LAST line, not the first.
	var got domain.ExperimentRecord
	if err := json.Unmarshal([]byte(row.MutationBriefJSON), &got); err != nil {
		t.Fatalf("mutation_brief_json must be a JSON ExperimentRecord: %v", err)
	}
	if got.Status != domain.ExperimentAccepted || got.RevertReason != "accepted" {
		t.Fatalf("mutation_brief_json = %s/%s, want the last line (accepted/accepted)", got.Status, got.RevertReason)
	}

	// Control: folding the SAME three lines first-write-wins yields a visibly
	// different row (planned, accepted=0, the first line's blob). Without this
	// control every assertion above would also pass on a fold that just kept
	// line 1 — it proves the assertions can tell the two semantics apart.
	firstRow, err := experimentRowValues(lines[0].Record)
	if err != nil {
		t.Fatalf("control experimentRowValues: %v", err)
	}
	if firstRow.Accepted != 0 {
		t.Fatalf("control accepted = %d, want 0", firstRow.Accepted)
	}
	if firstRow.MutationBriefJSON == row.MutationBriefJSON {
		t.Fatal("control blob must differ from the last-wins blob; the test would not detect first-wins")
	}
}

// TestFoldExperimentRecordsOrderAndMissingID pins the two remaining fold rules:
// a later line updates a record in place (first-appearance order is stable) and
// a record without an id is reported as skipped rather than written to an
// unaddressable NOT NULL UNIQUE column.
func TestFoldExperimentRecordsOrderAndMissingID(t *testing.T) {
	a := testExperimentIDPrefix + "a"
	b := testExperimentIDPrefix + "b"
	c := testExperimentIDPrefix + "c"
	lines := []experimentLineRecord{
		{Line: 1, Record: experimentRecordFixture(a, domain.ExperimentPlanned, "a-planned")},
		{Line: 2, Record: experimentRecordFixture(b, domain.ExperimentPlanned, "b-planned")},
		{Line: 3, Record: experimentRecordFixture(a, domain.ExperimentRejected, "a-rejected")},
		{Line: 4, Record: experimentRecordFixture("", domain.ExperimentRunning, "no-id")},
		{Line: 5, Record: experimentRecordFixture(c, domain.ExperimentExpired, "c-expired")},
	}

	folded, skipped := foldExperimentRecords(lines)
	if got := []string{folded[0].ID, folded[1].ID, folded[2].ID}; !slices.Equal(got, []string{a, b, c}) {
		t.Fatalf("folded order = %v, want first-appearance order [%s %s %s]", got, a, b, c)
	}
	if folded[0].Status != domain.ExperimentRejected {
		t.Fatalf("folded[0] status = %q, want %q (last write wins, in place)", folded[0].Status, domain.ExperimentRejected)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped = %+v, want exactly the id-less record", skipped)
	}
	if !strings.Contains(skipped[0].Source, "line 4") {
		t.Fatalf("skip source = %q, want it to name line 4", skipped[0].Source)
	}
	if !strings.Contains(skipped[0].Reason, "without an id") {
		t.Fatalf("skip reason = %q, want it to explain the missing id", skipped[0].Reason)
	}
}

// TestScanExperimentJSONL pins the per-line handling: blank lines are ignored
// (not counted), a malformed line is skipped with its physical line number, and
// the lines around it still parse.
func TestScanExperimentJSONL(t *testing.T) {
	id := testExperimentIDPrefix + "jsonl"
	path := filepath.Join(t.TempDir(), "experiments.jsonl")
	writeExperimentJSONLFixture(t, path,
		[]domain.ExperimentRecord{experimentRecordFixture(id, domain.ExperimentPlanned, "one")},
		[]string{"", "{ this is not json", mustString(mustJSONBytes(t, experimentRecordFixture(id, domain.ExperimentAccepted, "two")))})

	scan, err := scanExperimentJSONL(path)
	if err != nil {
		t.Fatalf("scanExperimentJSONL: %v", err)
	}
	if !scan.Found {
		t.Fatal("Found = false for an existing file")
	}
	if scan.Lines != 3 {
		t.Fatalf("Lines = %d, want 3 non-blank lines", scan.Lines)
	}
	if len(scan.Records) != 2 {
		t.Fatalf("Records = %d, want 2 (the malformed line must not abort the scan)", len(scan.Records))
	}
	if got := []int{scan.Records[0].Line, scan.Records[1].Line}; !slices.Equal(got, []int{1, 4}) {
		t.Fatalf("record line numbers = %v, want [1 4] (physical lines)", got)
	}
	if len(scan.Skipped) != 1 {
		t.Fatalf("Skipped = %+v, want exactly the malformed line", scan.Skipped)
	}
	if !strings.Contains(scan.Skipped[0].Source, "line 3") {
		t.Fatalf("skip source = %q, want it to name line 3", scan.Skipped[0].Source)
	}
	if !strings.Contains(scan.Skipped[0].Reason, "unmarshal experiment record") {
		t.Fatalf("skip reason = %q, want the unmarshal failure", scan.Skipped[0].Reason)
	}
}

// TestScanExperimentJSONLMissingFile: a deployment that never ran an experiment
// has no experiments.jsonl. That is an empty scan, not an error.
func TestScanExperimentJSONLMissingFile(t *testing.T) {
	scan, err := scanExperimentJSONL(filepath.Join(t.TempDir(), "experiments.jsonl"))
	if err != nil {
		t.Fatalf("missing file must not error, got: %v", err)
	}
	if scan.Found || scan.Lines != 0 || len(scan.Records) != 0 || len(scan.Skipped) != 0 {
		t.Fatalf("missing file must scan empty, got %+v", scan)
	}
}

// TestClassifyExperimentResultFile pins the name-based classifier, including the
// names that must NOT be treated as result files.
func TestClassifyExperimentResultFile(t *testing.T) {
	cases := []struct {
		name   string
		wantID string
		wantOK bool
	}{
		{"exp-20260401-1.json", "exp-20260401-1", true},
		// The writer's temp name before os.Rename: a crashed write leaves it
		// behind, and it must never be ingested as a half-written result.
		{"exp-20260401-1.json.tmp", "", false},
		{"notes.txt", "", false},
		{".json", "", false},
		{".hidden.json", "", false},
		{"", "", false},
		{"sub.json.bak", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := classifyExperimentResultFile(tc.name)
			if ok != tc.wantOK || id != tc.wantID {
				t.Fatalf("classifyExperimentResultFile(%q) = (%q, %v), want (%q, %v)",
					tc.name, id, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}

// TestScanExperimentResults covers the directory scan: valid files become rows
// keyed by their file name, a corrupt file is skipped (with a reason) without
// aborting, the temp leftover and the foreign name are ignored outright, a
// subdirectory is skipped, and each row carries the file's mtime for created_at.
func TestScanExperimentResults(t *testing.T) {
	idA := testExperimentIDPrefix + "a"
	idB := testExperimentIDPrefix + "b"
	dir := t.TempDir()
	writeExperimentResultFixture(t, dir, idA+".json", mustJSONBytes(t, promptResultFixture(idA, domain.ExperimentAccepted)))
	writeExperimentResultFixture(t, dir, idB+".json", mustJSONBytes(t, promptResultFixture(idB, domain.ExperimentRunning)))
	writeExperimentResultFixture(t, dir, testExperimentIDPrefix+"corrupt.json", []byte("{ truncated"))
	writeExperimentResultFixture(t, dir, idA+".json.tmp", mustJSONBytes(t, promptResultFixture(idA, domain.ExperimentRunning)))
	writeExperimentResultFixture(t, dir, "notes.txt", []byte("not a result"))
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	results, skipped, err := scanExperimentResults(dir)
	if err != nil {
		t.Fatalf("scanExperimentResults: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d rows, want 2 (corrupt/leftover/foreign/sub must not add rows)", len(results))
	}
	if got := []string{results[0].ExperimentID, results[1].ExperimentID}; !slices.Equal(got, []string{idA, idB}) {
		t.Fatalf("result ids = %v, want [%s %s] (directory order, keyed by file name)", got, idA, idB)
	}
	if results[0].Result.Experiment.ID != idA || results[0].Result.CandidatePrompt != "candidate prompt for "+idA {
		t.Fatalf("result payload mismatch: %+v", results[0].Result)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped = %+v, want exactly the corrupt file", skipped)
	}
	if skipped[0].Source != testExperimentIDPrefix+"corrupt.json" {
		t.Fatalf("skip source = %q, want the corrupt file name", skipped[0].Source)
	}
	if !strings.Contains(skipped[0].Reason, "unmarshal prompt experiment result") {
		t.Fatalf("skip reason = %q, want the unmarshal failure", skipped[0].Reason)
	}

	// created_at comes from the file mtime: it must be the real mtime, not zero
	// (a zero would silently degrade the row to time.Now()).
	info, err := os.Stat(filepath.Join(dir, idA+".json"))
	if err != nil {
		t.Fatalf("stat fixture: %v", err)
	}
	if results[0].ModTime.IsZero() {
		t.Fatal("ModTime must be populated from the directory entry")
	}
	if !results[0].ModTime.Equal(info.ModTime()) {
		t.Fatalf("ModTime = %v, want the file mtime %v", results[0].ModTime, info.ModTime())
	}
}

// TestScanExperimentResultsMissingDir: no experiments directory is the normal
// state of a deployment that never evaluated a candidate — empty, not an error.
func TestScanExperimentResultsMissingDir(t *testing.T) {
	results, skipped, err := scanExperimentResults(filepath.Join(t.TempDir(), "experiments"))
	if err != nil {
		t.Fatalf("missing directory must not error, got: %v", err)
	}
	if len(results) != 0 || len(skipped) != 0 {
		t.Fatalf("missing directory must scan empty, got %d results / %d skips", len(results), len(skipped))
	}
}

// TestScanExperimentResultsUnreadableDir: a path that is a regular file is a real
// error and must surface, not be reported as "no results".
func TestScanExperimentResultsUnreadableDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "experiments.txt")
	if err := os.WriteFile(path, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := scanExperimentResults(path); err == nil {
		t.Fatal("a non-directory path must error")
	} else if !strings.Contains(err.Error(), "read experiments dir") {
		t.Fatalf("error must name the failed read, got: %v", err)
	}
}

// TestScanExperimentFamilyCounters is the composed plan: the counters the run
// report prints must count what the writer will actually attempt, and the skip
// list must keep one entry per dropped source (jsonl line, id-less record,
// result file) for the warnings.
func TestScanExperimentFamilyCounters(t *testing.T) {
	idA := testExperimentIDPrefix + "a"
	idB := testExperimentIDPrefix + "b"
	stateDir := t.TempDir()
	jsonlPath := filepath.Join(stateDir, "experiments.jsonl")
	writeExperimentJSONLFixture(t, jsonlPath,
		[]domain.ExperimentRecord{
			experimentRecordFixture(idA, domain.ExperimentPlanned, "a-planned"),
			experimentRecordFixture(idA, domain.ExperimentAccepted, "a-accepted"),
			experimentRecordFixture(idB, domain.ExperimentRunning, "b-running"),
			experimentRecordFixture("", domain.ExperimentRunning, "no-id"),
		},
		[]string{"", "not json at all"})

	dir := filepath.Join(stateDir, "experiments")
	writeExperimentResultFixture(t, dir, idA+".json", mustJSONBytes(t, promptResultFixture(idA, domain.ExperimentAccepted)))
	writeExperimentResultFixture(t, dir, idB+".json", mustJSONBytes(t, promptResultFixture(idB, domain.ExperimentRunning)))
	writeExperimentResultFixture(t, dir, testExperimentIDPrefix+"corrupt.json", []byte("nope"))
	writeExperimentResultFixture(t, dir, "notes.txt", []byte("ignored"))

	scan, err := scanExperimentFamily(jsonlPath, dir)
	if err != nil {
		t.Fatalf("scanExperimentFamily: %v", err)
	}
	if !scan.JSONLFound {
		t.Fatal("JSONLFound = false for an existing experiments.jsonl")
	}
	// 5 non-blank lines: 4 parsed (3 with an id → 2 folded rows, 1 id-less
	// skipped) plus the malformed line.
	if scan.JSONLLines != 5 {
		t.Fatalf("JSONLLines = %d, want 5 non-blank lines", scan.JSONLLines)
	}
	if scan.JSONLSkipped != 2 {
		t.Fatalf("JSONLSkipped = %d, want 2 (malformed line + id-less record)", scan.JSONLSkipped)
	}
	if len(scan.Experiments) != 2 {
		t.Fatalf("Experiments = %d, want 2 folded ids", len(scan.Experiments))
	}
	if scan.Experiments[0].ID != idA || scan.Experiments[0].Status != domain.ExperimentAccepted {
		t.Fatalf("folded[0] = %s/%s, want %s/%s", scan.Experiments[0].ID, scan.Experiments[0].Status, idA, domain.ExperimentAccepted)
	}
	if scan.ResultFiles != 2 {
		t.Fatalf("ResultFiles = %d, want 2 parsed files", scan.ResultFiles)
	}
	if scan.ResultsSkipped != 1 {
		t.Fatalf("ResultsSkipped = %d, want 1 corrupt file", scan.ResultsSkipped)
	}
	if len(scan.Skipped) != 3 {
		t.Fatalf("Skipped = %d entries, want 3 (1 malformed line, 1 id-less record, 1 corrupt file): %+v",
			len(scan.Skipped), scan.Skipped)
	}
}

// TestScanExperimentFamilyNoSources: with neither source present the scan is
// empty and the counters are zero, which is what makes migrateExperimentsData
// report "nothing to do" instead of failing.
func TestScanExperimentFamilyNoSources(t *testing.T) {
	stateDir := t.TempDir()
	scan, err := scanExperimentFamily(filepath.Join(stateDir, "experiments.jsonl"), filepath.Join(stateDir, "experiments"))
	if err != nil {
		t.Fatalf("scanExperimentFamily: %v", err)
	}
	if scan.JSONLFound || scan.JSONLLines != 0 || len(scan.Experiments) != 0 || scan.ResultFiles != 0 || len(scan.Skipped) != 0 {
		t.Fatalf("empty sources must scan empty, got %+v", scan)
	}
}

// TestExperimentRowValues mirrors the column mapping of
// ledger.PostgresLedgerStore.RecordExperiment: only ExperimentAccepted sets
// accepted=1, and the timestamp keeps the WindowStart zone offset instead of
// being silently converted.
func TestExperimentRowValues(t *testing.T) {
	id := testExperimentIDPrefix + "row"
	for _, tc := range []struct {
		status       domain.ExperimentStatus
		wantAccepted int
	}{
		{domain.ExperimentPlanned, 0},
		{domain.ExperimentRunning, 0},
		{domain.ExperimentRejected, 0},
		{domain.ExperimentExpired, 0},
		{domain.ExperimentAccepted, 1},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			row, err := experimentRowValues(experimentRecordFixture(id, tc.status, "m"))
			if err != nil {
				t.Fatalf("experimentRowValues: %v", err)
			}
			if row.Accepted != tc.wantAccepted {
				t.Fatalf("accepted = %d for status %q, want %d", row.Accepted, tc.status, tc.wantAccepted)
			}
			if row.ExperimentID != id {
				t.Fatalf("experiment_id = %q, want %q", row.ExperimentID, id)
			}
		})
	}

	// Zone offset must survive: the ledger writes
	// WindowStart.Format("2006-01-02T15:04:05Z07:00") and readers compare the
	// stored text, so a silently UTC-normalized timestamp would change the value.
	rec := experimentRecordFixture(id, domain.ExperimentRunning, "m")
	rec.WindowStart = time.Date(2026, 4, 1, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	row, err := experimentRowValues(rec)
	if err != nil {
		t.Fatalf("experimentRowValues: %v", err)
	}
	if row.Timestamp != "2026-04-01T09:30:00+08:00" {
		t.Fatalf("timestamp = %q, want the WindowStart offset preserved", row.Timestamp)
	}
}

// TestUpsertSQLShapes pins the two properties the idempotency contract rests on,
// because they are invisible to a hermetic test that never reaches PG:
//
//  1. both halves upsert on experiment_id — DO NOTHING would silently keep an
//     OLDER row, so a re-run could never refresh a folded status, and a bare
//     INSERT would raise a duplicate key on every re-run;
//  2. the experiments half protects a session_id that is already stored: this
//     backfill writes NULL, and a plain assignment would erase the attribution
//     RecordSessionExperiment had established (FU-20261016-03).
func TestUpsertSQLShapes(t *testing.T) {
	for name, sql := range map[string]string{
		"experiments":               upsertExperimentSQL,
		"prompt_experiment_results": upsertPromptExperimentResultSQL,
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(sql, "ON CONFLICT (experiment_id) DO UPDATE") {
				t.Fatalf("%s must upsert on experiment_id, got: %s", name, sql)
			}
			if strings.Contains(sql, "DO NOTHING") {
				t.Fatalf("%s must not use DO NOTHING (a re-run could never refresh the row): %s", name, sql)
			}
		})
	}
	if !strings.Contains(upsertExperimentSQL, "COALESCE(experiments.session_id, EXCLUDED.session_id)") {
		t.Fatalf("experiments upsert must keep an existing session_id, got: %s", upsertExperimentSQL)
	}
	if !strings.Contains(upsertPromptExperimentResultSQL, "data_json = EXCLUDED.data_json") {
		t.Fatalf("prompt_experiment_results upsert must refresh data_json, got: %s", upsertPromptExperimentResultSQL)
	}
}

// TestRequireExperimentsTargetAssertion covers the fail-closed gate added for
// -experiments: without -expect-db the run aborts and names both the flag and the
// missing assertion (windows_test.go covers the stockpicker/-windows helper).
func TestRequireExperimentsTargetAssertion(t *testing.T) {
	cases := []struct {
		name        string
		experiments bool
		expectDB    string
		wantErr     bool
	}{
		{name: "experiments without expect-db aborts", experiments: true, wantErr: true},
		{name: "experiments with expect-db passes", experiments: true, expectDB: "atlas"},
		{name: "other modes are unaffected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireExperimentsTargetAssertion(tc.experiments, tc.expectDB)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an abort naming -experiments")
			}
			if !strings.Contains(err.Error(), "-experiments") {
				t.Fatalf("error must name -experiments, got: %v", err)
			}
			if !strings.Contains(err.Error(), "-expect-db") {
				t.Fatalf("error must name -expect-db, got: %v", err)
			}
		})
	}
}

// TestExperimentSourcePaths pins the documented -experiments sources: the
// LedgerDir convention by default (same shape as <LedgerDir>/windows), explicit
// flags win when given.
func TestExperimentSourcePaths(t *testing.T) {
	ledgerDir := filepath.Join(t.TempDir(), "state")
	dir, jsonlPath := experimentSourcePaths(ledgerDir, "", "")
	if dir != filepath.Join(ledgerDir, "experiments") {
		t.Fatalf("default dir = %q, want <LedgerDir>/experiments", dir)
	}
	if jsonlPath != filepath.Join(ledgerDir, "experiments.jsonl") {
		t.Fatalf("default jsonl = %q, want <LedgerDir>/experiments.jsonl", jsonlPath)
	}

	dir, jsonlPath = experimentSourcePaths(ledgerDir, "/src/dir", "/src/experiments.jsonl")
	if dir != "/src/dir" || jsonlPath != "/src/experiments.jsonl" {
		t.Fatalf("explicit flags ignored: got (%q, %q)", dir, jsonlPath)
	}
}

// TestExperimentsWiringInMainGo is the source-level guard for the two wiring
// rules that a hermetic test cannot observe (mirrors
// cmd/atlas TestMainGoCallsWireStage3): -experiments must stay OUT of -all, and
// the fail-closed gate must be wired. Without the first, `-all` would start
// backfilling experiments on deployments that never asked for it; without the
// second, a forgotten -expect-db would write production rows anywhere
// DATABASE_URL points.
func TestExperimentsWiringInMainGo(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !bytes.Contains(src, []byte("if *migrateExperiments {")) {
		t.Fatal("main.go must dispatch -experiments with `if *migrateExperiments {` (an explicit backfill, not part of -all)")
	}
	// Word-bounded on purpose: "*migrateAll || *migrateExperimentsSQLite" is a
	// different flag (-experiments-sqlite) and must keep its -all membership.
	allGated := regexp.MustCompile(`\*migrateAll\s*\|\|\s*\*migrateExperiments\b`)
	if allGated.Match(src) {
		t.Fatal("-experiments must NOT be part of -all (mirrors the -windows decision)")
	}
	if !bytes.Contains(src, []byte("requireExperimentsTargetAssertion(*migrateExperiments, *expectDB)")) {
		t.Fatal("main.go must assert the target for -experiments (requireExperimentsTargetAssertion)")
	}
	for _, flagName := range []string{`flag.Bool("experiments"`, `flag.String("experiments-dir"`, `flag.String("experiments-jsonl"`} {
		if !bytes.Contains(src, []byte(flagName)) {
			t.Fatalf("main.go must declare %s", flagName)
		}
	}
}

// mustString converts fixture bytes to the string form tests splice into raw
// jsonl lines.
func mustString(b []byte) string { return string(b) }
