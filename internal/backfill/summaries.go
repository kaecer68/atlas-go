// Package backfill provides one-shot repair tools for ledger state that
// drifted from the canonical schema. The summaries subpackage reconstructs
// minimal summary.json for orphan sessions (those with only
// recommendation_outcomes.jsonl). Do not call from hot paths.
package backfill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// SummariesResult reports the counts from a backfill run.
type SummariesResult struct {
	Scanned       int
	Backfilled    int
	SkippedExists int
	SkippedEmpty  int
}

// BackfillSummaries scans sessionsDir and writes a minimal summary.json for
// every session that has recommendation_outcomes.jsonl but no summary.json.
// Existing summary.json is never overwritten. Sessions with neither file are
// skipped (no spurious zero-count summaries).
func BackfillSummaries(sessionsDir string, dryRun bool) (SummariesResult, error) {
	result := SummariesResult{}
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, fmt.Errorf("read sessions dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		sessionDir := filepath.Join(sessionsDir, sessionID)
		result.Scanned++

		summaryPath := filepath.Join(sessionDir, "summary.json")
		outcomesPath := filepath.Join(sessionDir, "recommendation_outcomes.jsonl")

		if _, err := os.Stat(summaryPath); err == nil {
			result.SkippedExists++
			continue
		}

		count := countNonEmptyLines(outcomesPath)
		if count == 0 {
			result.SkippedEmpty++
			continue
		}

		summary := domain.SessionSummary{
			SessionID:     sessionID,
			Regime:        domain.Regime(""),
			RecordedAt:    domain.SessionDateFromID(sessionID),
			OutcomeCount:  count,
			GuardOutcomes: nil,
		}

		if dryRun {
			logging.Info("backfill_summaries", "dry_run_would_write", "session", sessionID, "outcome_count", count)
			result.Backfilled++
			continue
		}

		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return result, fmt.Errorf("marshal summary for %s: %w", sessionID, err)
		}
		if err := os.WriteFile(summaryPath, data, 0o644); err != nil {
			return result, fmt.Errorf("write summary for %s: %w", sessionID, err)
		}
		logging.Info("backfill_summaries", "wrote_summary", "session", sessionID, "outcome_count", count)
		result.Backfilled++
	}

	return result, nil
}

func countNonEmptyLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	count := 0
	for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
