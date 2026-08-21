package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ---- fixtures -------------------------------------------------------------

func writeHealthJSONL(t *testing.T, path string, records []domain.ExperimentRecord) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write(b)
		_, _ = f.Write([]byte("\n"))
	}
}

func writeHealthOutcome(t *testing.T, dir, name string, res domain.PromptExperimentResult) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func writeBaselinePolicy(t *testing.T, path string, promotions, reverts []time.Time) {
	t.Helper()
	type promRec struct {
		ExperimentID string    `json:"experiment_id"`
		PromotedAt   time.Time `json:"promoted_at"`
		Status       string    `json:"status"`
		VersionAfter int       `json:"version_after"`
	}
	type revRec struct {
		FromVersion int       `json:"from_version"`
		ToVersion   int       `json:"to_version"`
		Reason      string    `json:"reason"`
		RevertedAt  time.Time `json:"reverted_at"`
	}
	proms := make([]promRec, 0, len(promotions))
	for i, ts := range promotions {
		proms = append(proms, promRec{
			ExperimentID: "exp-promote",
			PromotedAt:   ts,
			Status:       "accepted",
			VersionAfter: i + 1,
		})
	}
	revs := make([]revRec, 0, len(reverts))
	for _, ts := range reverts {
		revs = append(revs, revRec{FromVersion: 2, ToVersion: 1, Reason: "test", RevertedAt: ts})
	}
	policy := map[string]any{
		"version":          2,
		"prompt_overrides": map[string]string{},
		"promotions":       proms,
		"revert_history":   revs,
	}
	b, _ := json.MarshalIndent(policy, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write baseline policy: %v", err)
	}
}

func writeReplayCSV(t *testing.T, path, latestDate string) {
	t.Helper()
	content := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" +
		"2026-03-20,0050,Test,1000000,100,110,90,105\n" +
		latestDate + ",0050,Test,1000000,105,115,95,110\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write replay csv: %v", err)
	}
}

// ---- tests ----------------------------------------------------------------

func TestCheckEvolutionHealth_AllActive(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// proposal: fresh ledger record with ID-embedded unix timestamp
	writeHealthJSONL(t, filepath.Join(dir, "experiments.jsonl"), []domain.ExperimentRecord{
		{ID: "auto-propose-desk-01-" + ts(now.Add(-time.Hour)), Status: domain.ExperimentPlanned},
	})
	// judge: fresh accepted outcome
	writeHealthOutcome(t, filepath.Join(dir, "experiments"), "exec-desk-01-1.json", domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exec-desk-01-1", Status: domain.ExperimentAccepted},
		RecordedAt: now.Add(-2 * time.Hour),
	})
	// promote + revert: fresh baseline timestamps
	writeBaselinePolicy(t, filepath.Join(dir, "baseline_policy.json"),
		[]time.Time{now.Add(-3 * time.Hour)}, []time.Time{now.Add(-4 * time.Hour)})

	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:          dir,
		BaselinePolicyPath: filepath.Join(dir, "baseline_policy.json"),
		Now:                now,
	})
	if len(res.Stale) != 0 {
		t.Fatalf("expected no stale pillars, got %v (lastActivity=%v)", res.Stale, res.LastActivity)
	}
	if res.AllStale {
		t.Fatal("expected AllStale=false")
	}
}

func TestCheckEvolutionHealth_NoActivity(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// empty dir → no activity anywhere
	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:          dir,
		BaselinePolicyPath: filepath.Join(dir, "baseline_policy.json"),
		Now:                now,
	})
	if len(res.Stale) != 4 {
		t.Fatalf("expected all 4 pillars stale, got %v", res.Stale)
	}
	if !res.AllStale {
		t.Fatal("expected AllStale=true")
	}
}

func TestCheckEvolutionHealth_PartialActivity(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// only proposal is fresh; judge/promote/revert are stale
	writeHealthJSONL(t, filepath.Join(dir, "experiments.jsonl"), []domain.ExperimentRecord{
		{ID: "auto-propose-desk-01-" + ts(now.Add(-time.Hour)), Status: domain.ExperimentPlanned},
	})
	writeHealthOutcome(t, filepath.Join(dir, "experiments"), "exec-desk-01-old.json", domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exec-desk-01-old", Status: domain.ExperimentAccepted},
		RecordedAt: now.Add(-72 * time.Hour), // judge happened 3 days ago
	})

	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:          dir,
		BaselinePolicyPath: filepath.Join(dir, "baseline_policy.json"), // missing → stale
		Now:                now,
	})
	if len(res.Stale) != 3 {
		t.Fatalf("expected 3 stale pillars (judge,promote,revert), got %v", res.Stale)
	}
	if res.AllStale {
		t.Fatal("expected AllStale=false")
	}
	for _, p := range res.Stale {
		if p == PillarProposal {
			t.Fatalf("proposal should NOT be stale, got %v", res.Stale)
		}
	}
}

func TestCheckEvolutionHealth_ProposalTimestampPriority(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// record with WindowStart (executor path) but no parseable ID suffix
	writeHealthJSONL(t, filepath.Join(dir, "experiments.jsonl"), []domain.ExperimentRecord{
		{ID: "exec-desk-01", WindowStart: now.Add(-time.Hour), Status: domain.ExperimentRunning},
	})
	res := CheckEvolutionHealth(EvolutionHealthConfig{LedgerDir: dir, Now: now})
	if len(res.Stale) != 3 {
		t.Fatalf("expected 3 stale (judge,promote,revert), got %v", res.Stale)
	}
	found := false
	for _, p := range res.Stale {
		if p == PillarProposal {
			found = true
		}
	}
	if found {
		t.Fatalf("proposal should be fresh via WindowStart, got %v", res.Stale)
	}
}

func TestCheckEvolutionHealth_ReplayStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	replayPath := filepath.Join(dir, "replay.csv")
	// latest replay date is 3 days behind → stale
	writeReplayCSV(t, replayPath, now.AddDate(0, 0, -3).Format("2006-01-02"))

	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:          dir,
		BaselinePolicyPath: filepath.Join(dir, "baseline_policy.json"),
		ReplayDataPath:     replayPath,
		Now:                now,
	})
	if res.ReplayFresh {
		t.Fatalf("expected replay stale (daysOld=%d)", res.ReplayDaysOld)
	}
	if res.ReplayDaysOld != 3 {
		t.Fatalf("expected ReplayDaysOld=3, got %d", res.ReplayDaysOld)
	}
	if res.ReplayErr != nil {
		t.Fatalf("unexpected replay error: %v", res.ReplayErr)
	}
}

func TestCheckEvolutionHealth_ReplayFresh(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	replayPath := filepath.Join(dir, "replay.csv")
	writeReplayCSV(t, replayPath, now.Format("2006-01-02"))

	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:      dir,
		ReplayDataPath: replayPath,
		Now:            now,
	})
	if !res.ReplayFresh {
		t.Fatalf("expected replay fresh, err=%v", res.ReplayErr)
	}
}

func TestCheckEvolutionHealth_ReplayUnreadable(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:      dir,
		ReplayDataPath: filepath.Join(dir, "missing.csv"),
		Now:            now,
	})
	if res.ReplayFresh {
		t.Fatal("expected replay not fresh for unreadable file")
	}
	if res.ReplayErr == nil {
		t.Fatal("expected ReplayErr for unreadable file")
	}
}

// ---- alert raising --------------------------------------------------------

type recordingMonitor struct {
	alerts []recordedAlert
}

type recordedAlert struct {
	level    string
	category string
	message  string
	details  map[string]any
}

func (m *recordingMonitor) Alert(level string, category, message string, details map[string]any) {
	m.alerts = append(m.alerts, recordedAlert{level: level, category: category, message: message, details: details})
}

func TestRaiseEvolutionHealthAlerts_AllStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	res := CheckEvolutionHealth(EvolutionHealthConfig{LedgerDir: dir, Now: now})

	mon := &recordingMonitor{}
	RaiseEvolutionHealthAlerts(mon, res)

	if len(mon.alerts) != 1 {
		t.Fatalf("expected exactly 1 alert for all-stale, got %d: %+v", len(mon.alerts), mon.alerts)
	}
	a := mon.alerts[0]
	if a.level != "error" {
		t.Errorf("expected error level, got %q", a.level)
	}
	if a.category != "evolution" {
		t.Errorf("expected category evolution, got %q", a.category)
	}
	if a.details["pillars"] == nil {
		t.Errorf("expected pillar details, got %+v", a.details)
	}
}

func TestRaiseEvolutionHealthAlerts_Partial(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	// proposal fresh only
	writeHealthJSONL(t, filepath.Join(dir, "experiments.jsonl"), []domain.ExperimentRecord{
		{ID: "auto-propose-desk-01-" + ts(now.Add(-time.Hour)), Status: domain.ExperimentPlanned},
	})
	res := CheckEvolutionHealth(EvolutionHealthConfig{LedgerDir: dir, Now: now})

	mon := &recordingMonitor{}
	RaiseEvolutionHealthAlerts(mon, res)

	if len(mon.alerts) != 3 {
		t.Fatalf("expected 3 warning alerts (judge,promote,revert), got %d: %+v", len(mon.alerts), mon.alerts)
	}
	for _, a := range mon.alerts {
		if a.level != "warning" {
			t.Errorf("expected warning level, got %q", a.level)
		}
	}
}

func TestRaiseEvolutionHealthAlerts_ReplayStale(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	replayPath := filepath.Join(dir, "replay.csv")
	writeReplayCSV(t, replayPath, now.AddDate(0, 0, -3).Format("2006-01-02"))

	res := CheckEvolutionHealth(EvolutionHealthConfig{
		LedgerDir:      dir,
		ReplayDataPath: replayPath,
		Now:            now,
	})
	mon := &recordingMonitor{}
	RaiseEvolutionHealthAlerts(mon, res)

	var found bool
	for _, a := range mon.alerts {
		if a.message == "replay_data_stale" {
			found = true
			if a.details["days_behind"] != 3 {
				t.Errorf("expected days_behind=3, got %v", a.details["days_behind"])
			}
		}
	}
	if !found {
		t.Fatalf("expected replay_data_stale alert, got %+v", mon.alerts)
	}
}

// ts renders a time as the unix suffix used in experiment IDs.
func ts(t time.Time) string {
	return fmt.Sprintf("%d", t.Unix())
}
