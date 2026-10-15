package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// testWindowIDPrefix marks every row this suite writes to PG (integration
// test) so cleanup can never touch real window data.
const testWindowIDPrefix = "window-q2b2wbf-"

// mustJSON marshals a fixture the way the ledger writers do.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(data)
}

// windowSummaryFixture mirrors domain.BacktestWindowSummary as the producer
// writes it (internal/ledger.Store.RecordWindowSummary → <windowID>.json).
func windowSummaryFixture(windowID string) domain.BacktestWindowSummary {
	return domain.BacktestWindowSummary{
		WindowID:     windowID,
		StartDate:    time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		SessionCount: 5,
		OutcomeCount: 12,
		GeneratedAt:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}
}

// mutationBriefFixture mirrors domain.MutationBrief as the producer writes it
// (<windowID>-mutation-brief.json).
func mutationBriefFixture(windowID string) domain.MutationBrief {
	return domain.MutationBrief{
		ContractVersion:  domain.MutationBriefContractVersion,
		ProposalID:       "prop-window-q2b2wbf",
		WindowID:         windowID,
		TargetAgentID:    "migratetest-agent",
		TargetSkill:      "growth_momentum",
		MutationType:     "prompt_tightening",
		FailurePattern:   "weak_momentum",
		Hypothesis:       "tighten the exit rule",
		AcceptanceMetric: "sharpe_like",
		AcceptanceGates:  []string{"improve_sharpe_like"},
	}
}

func artifactWindowIDs(artifacts []windowArtifact) []string {
	ids := make([]string, 0, len(artifacts))
	for _, a := range artifacts {
		ids = append(ids, a.WindowID)
	}
	return ids
}

func artifactFileNames(skips []windowSkip) []string {
	names := make([]string, 0, len(skips))
	for _, s := range skips {
		names = append(names, s.FileName)
	}
	return names
}

// TestClassifyWindowFile pins the name-based classification, including the
// names that must NOT be treated as window artifacts (write leftovers, temp
// files, ids that cannot address one artifact).
func TestClassifyWindowFile(t *testing.T) {
	cases := []struct {
		name     string
		wantKind windowFileKind
		wantID   string
	}{
		{"window-20260401-20260430.json", windowFileSummary, "window-20260401-20260430"},
		{"window-20260401-20260430-mutation-brief.json", windowFileMutationBrief, "window-20260401-20260430"},
		// A brief name ends in .json too — the brief suffix must win.
		{"window-a-mutation-brief.json", windowFileMutationBrief, "window-a"},
		// Outside the family.
		{"notes.txt", windowFileIgnored, ""},
		{"window-a.json.tmp", windowFileIgnored, ""},
		{".DS_Store", windowFileIgnored, ""},
		{".json", windowFileIgnored, ""},
		{"-mutation-brief.json", windowFileIgnored, ""},
		{".hidden.json", windowFileIgnored, ""},
		{"", windowFileIgnored, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, id := classifyWindowFile(tc.name)
			if kind != tc.wantKind {
				t.Fatalf("classifyWindowFile(%q) kind = %v, want %v", tc.name, kind, tc.wantKind)
			}
			if id != tc.wantID {
				t.Fatalf("classifyWindowFile(%q) id = %q, want %q", tc.name, id, tc.wantID)
			}
		})
	}
}

// TestScanWindowArtifacts is the table-driven scan/classify contract: which
// files become summaries, which become briefs, which are skipped with a reason
// and which are ignored outright.
func TestScanWindowArtifacts(t *testing.T) {
	prefix := testWindowIDPrefix
	summaryA := mustJSON(t, windowSummaryFixture(prefix+"a"))
	summaryB := mustJSON(t, windowSummaryFixture(prefix+"b"))
	briefA := mustJSON(t, mutationBriefFixture(prefix+"a"))

	cases := []struct {
		name               string
		files              map[string]string
		dirs               []string
		wantSummaries      []string
		wantSummaryDataIDs []string
		wantBriefs         []string
		wantSkipped        []string
		wantSkipReason     string
	}{
		{
			name:               "summary only",
			files:              map[string]string{prefix + "a.json": summaryA},
			wantSummaries:      []string{prefix + "a"},
			wantSummaryDataIDs: []string{prefix + "a"},
		},
		{
			name:               "brief is not parsed as a summary",
			files:              map[string]string{prefix + "a-mutation-brief.json": briefA},
			wantSummaryDataIDs: nil,
			wantBriefs:         []string{prefix + "a"},
		},
		{
			name: "both halves of one window",
			files: map[string]string{
				prefix + "a.json":                summaryA,
				prefix + "a-mutation-brief.json": briefA,
			},
			wantSummaries:      []string{prefix + "a"},
			wantSummaryDataIDs: []string{prefix + "a"},
			wantBriefs:         []string{prefix + "a"},
		},
		{
			name: "two summaries keep directory order",
			files: map[string]string{
				prefix + "a.json": summaryA,
				prefix + "b.json": summaryB,
			},
			wantSummaries:      []string{prefix + "a", prefix + "b"},
			wantSummaryDataIDs: []string{prefix + "a", prefix + "b"},
		},
		{
			name: "corrupt summary skipped, valid summary kept",
			files: map[string]string{
				prefix + "a.json":   summaryA,
				prefix + "bad.json": "{ this is not json",
			},
			wantSummaries:      []string{prefix + "a"},
			wantSummaryDataIDs: []string{prefix + "a"},
			wantSkipped:        []string{prefix + "bad.json"},
			wantSkipReason:     "unmarshal window summary",
		},
		{
			name: "corrupt brief skipped",
			files: map[string]string{
				prefix + "a-mutation-brief.json": "not json at all",
			},
			wantSkipped:    []string{prefix + "a-mutation-brief.json"},
			wantSkipReason: "unmarshal mutation brief",
		},
		{
			name: "files outside the family are ignored, not skipped",
			files: map[string]string{
				"notes.txt":                "hello",
				prefix + "a.json.tmp":      summaryA,
				".DS_Store":                "",
				".json":                    summaryA,
				"-mutation-brief.json":     briefA,
				prefix + "hidden.json.bak": "x",
			},
			wantSummaryDataIDs: nil,
		},
		{
			name:               "subdirectory ignored",
			dirs:               []string{"sub"},
			files:              map[string]string{filepath.Join("sub", prefix+"a.json"): summaryA},
			wantSummaryDataIDs: nil,
		},
		{
			name:               "summary without window_id falls back to the file name",
			files:              map[string]string{prefix + "fallback.json": `{"session_count":7}`},
			wantSummaries:      []string{prefix + "fallback"},
			wantSummaryDataIDs: []string{prefix + "fallback"},
		},
		{
			name: "brief row key comes from the file name, not the data field",
			files: map[string]string{
				prefix + "a-mutation-brief.json": mustJSON(t, mutationBriefFixture("window-somewhere-else")),
			},
			wantSummaryDataIDs: nil,
			wantBriefs:         []string{prefix + "a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, sub := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", sub, err)
				}
			}
			for name, content := range tc.files {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("mkdir for %s: %v", name, err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			scan, err := scanWindowArtifacts(dir)
			if err != nil {
				t.Fatalf("scanWindowArtifacts: %v", err)
			}
			if got := artifactWindowIDs(scan.Summaries); !slices.Equal(got, tc.wantSummaries) {
				t.Fatalf("summary ids = %v, want %v", got, tc.wantSummaries)
			}
			if got := artifactWindowIDs(scan.Briefs); !slices.Equal(got, tc.wantBriefs) {
				t.Fatalf("brief ids = %v, want %v", got, tc.wantBriefs)
			}
			// The summary's own WindowID is what RecordWindowSummary keys the
			// row on, so the file-name fallback must land in the struct too.
			dataIDs := make([]string, 0, len(scan.Summaries))
			for _, s := range scan.Summaries {
				dataIDs = append(dataIDs, s.Summary.WindowID)
			}
			if !slices.Equal(dataIDs, tc.wantSummaryDataIDs) {
				t.Fatalf("summary data window ids = %v, want %v", dataIDs, tc.wantSummaryDataIDs)
			}
			if got := artifactFileNames(scan.Skipped); !slices.Equal(got, tc.wantSkipped) {
				t.Fatalf("skipped files = %v, want %v", got, tc.wantSkipped)
			}
			if tc.wantSkipReason != "" {
				for _, skip := range scan.Skipped {
					if !strings.Contains(skip.Reason, tc.wantSkipReason) {
						t.Fatalf("skip reason for %s = %q, want it to contain %q",
							skip.FileName, skip.Reason, tc.wantSkipReason)
					}
				}
			}
		})
	}
}

// TestScanWindowArtifactsMissingDir: a deployment that never ran a backtest
// window has no windows directory. That is an empty result, not an error — the
// migration must not fail on it.
func TestScanWindowArtifactsMissingDir(t *testing.T) {
	scan, err := scanWindowArtifacts(filepath.Join(t.TempDir(), "windows"))
	if err != nil {
		t.Fatalf("missing directory must not error, got: %v", err)
	}
	if len(scan.Summaries) != 0 || len(scan.Briefs) != 0 || len(scan.Skipped) != 0 {
		t.Fatalf("missing directory must scan empty, got %+v", scan)
	}
}

// TestScanWindowArtifactsEmptyDir: an existing but empty directory is also an
// empty result.
func TestScanWindowArtifactsEmptyDir(t *testing.T) {
	scan, err := scanWindowArtifacts(t.TempDir())
	if err != nil {
		t.Fatalf("empty directory must not error, got: %v", err)
	}
	if len(scan.Summaries) != 0 || len(scan.Briefs) != 0 || len(scan.Skipped) != 0 {
		t.Fatalf("empty directory must scan empty, got %+v", scan)
	}
}

// TestScanWindowArtifactsUnreadableDir: an unreadable path (a regular file
// instead of a directory) is a real error and must surface, not be reported as
// "no windows".
func TestScanWindowArtifactsUnreadableDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "windows.txt")
	if err := os.WriteFile(path, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := scanWindowArtifacts(path); err == nil {
		t.Fatal("a non-directory path must error")
	} else if !strings.Contains(err.Error(), "read windows dir") {
		t.Fatalf("error must name the failed read, got: %v", err)
	}
}

// TestMutationBriefWouldUnmarshalAsSummary pins the reason the classifier keys
// on the file name: the brief JSON carries window_id, so unmarshalling it into
// a window summary SUCCEEDS and yields a half-empty summary. There is no error
// to detect — only the name tells the two halves apart.
func TestMutationBriefWouldUnmarshalAsSummary(t *testing.T) {
	windowID := testWindowIDPrefix + "a"
	var summary domain.BacktestWindowSummary
	if err := json.Unmarshal([]byte(mustJSON(t, mutationBriefFixture(windowID))), &summary); err != nil {
		t.Fatalf("precondition changed: brief no longer unmarshals as a summary: %v", err)
	}
	if summary.WindowID != windowID {
		t.Fatalf("precondition: brief window_id must carry over, got %q", summary.WindowID)
	}
	if summary.SessionCount != 0 || summary.OutcomeCount != 0 {
		t.Fatalf("precondition: brief has no summary counters, got %+v", summary)
	}
}

// TestRequireMigrateTargetAssertion covers the generalized fail-closed gate:
// both production-writing modes refuse to run without -expect-db, and the
// read-only modes keep the legacy behavior.
func TestRequireMigrateTargetAssertion(t *testing.T) {
	cases := []struct {
		name                 string
		stockpicker, windows bool
		expectDB             string
		wantErrFlag          string
	}{
		{name: "windows without expect-db aborts", windows: true, wantErrFlag: "-windows"},
		{name: "stockpicker without expect-db aborts", stockpicker: true, wantErrFlag: "-stockpicker"},
		{name: "both without expect-db aborts", stockpicker: true, windows: true, wantErrFlag: "-expect-db"},
		{name: "windows with expect-db passes", windows: true, expectDB: "atlas"},
		{name: "stockpicker with expect-db passes", stockpicker: true, expectDB: "atlas"},
		{name: "legacy mode without expect-db passes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := requireMigrateTargetAssertion(tc.stockpicker, tc.windows, tc.expectDB)
			if tc.wantErrFlag == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an abort naming %s", tc.wantErrFlag)
			}
			if !strings.Contains(err.Error(), tc.wantErrFlag) {
				t.Fatalf("error must name %s, got: %v", tc.wantErrFlag, err)
			}
			if !strings.Contains(err.Error(), "-expect-db") {
				t.Fatalf("error must name -expect-db, got: %v", err)
			}
		})
	}
}
