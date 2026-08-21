package experiment

// B4 — evolution health alerting.
//
// EvolutionHealthChecker-style helpers that watch the four evolution
// pillars (proposal / judge / promote / revert) for 24h of silence and
// report staleness so the daily scheduler can raise monitor alerts.
// Audit root cause: the evolution loop ran unmonitored for 4 months with
// nobody noticing it had stalled (see phaseBC_execution_design.md §B4).
//
// Data sources (all filesystem, no new infra):
//   proposal: <ledgerDir>/experiments.jsonl — newest record timestamp
//             (ID-embedded unix suffix, else WindowStart; mirrors
//             experimentRecordIsOld in auto.go).
//   judge:    <ledgerDir>/experiments/*.json — PromptExperimentResult
//             outcomes with status accepted/rejected (RecordedAt).
//             Falls back to accepted/rejected records in experiments.jsonl
//             for deployments that only persist the ledger.
//   promote:  baseline_policy.json promotions[].promoted_at (last entry).
//   revert:   baseline_policy.json revert_history[].reverted_at (last entry).

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// Pillar names one of the four evolution activity pillars.
type Pillar string

const (
	PillarProposal Pillar = "proposal"
	PillarJudge    Pillar = "judge"
	PillarPromote  Pillar = "promote"
	PillarRevert   Pillar = "revert"
)

// AllPillars is the canonical, stable ordering of the four pillars.
var AllPillars = []Pillar{PillarProposal, PillarJudge, PillarPromote, PillarRevert}

// DefaultEvolutionHealthWindow is the activity window for a healthy loop.
const DefaultEvolutionHealthWindow = 24 * time.Hour

// DefaultReplayMaxDelay is the replay freshness threshold (≥2 days behind
// triggers the stale alert).
const DefaultReplayMaxDelay = 48 * time.Hour

// EvolutionHealthConfig configures CheckEvolutionHealth. Zero values pick
// production defaults (24h window, 48h replay delay, time.Now clock).
type EvolutionHealthConfig struct {
	LedgerDir          string // dir containing experiments.jsonl + experiments/ outcome files
	BaselinePolicyPath string // baseline_policy.json
	ReplayDataPath     string // replay CSV (optional; empty disables the replay check)
	Window             time.Duration
	ReplayMaxDelay     time.Duration
	Now                time.Time // injectable clock for tests; zero → time.Now()
}

// EvolutionHealthResult is the outcome of a single evolution health check.
type EvolutionHealthResult struct {
	CheckedAt     time.Time
	Window        time.Duration
	Stale         []Pillar
	LastActivity  map[Pillar]time.Time
	AllStale      bool
	ReplayFresh   bool
	ReplayDaysOld int
	ReplayErr     error
}

// CheckEvolutionHealth checks activity in the last Window (default 24h)
// across the four evolution pillars. A pillar is stale when its last
// observed activity predates now-Window (or was never observed).
// AllStale is true when all four pillars are stale (→ error alert).
func CheckEvolutionHealth(cfg EvolutionHealthConfig) EvolutionHealthResult {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}
	window := cfg.Window
	if window <= 0 {
		window = DefaultEvolutionHealthWindow
	}
	res := EvolutionHealthResult{
		CheckedAt:    now,
		Window:       window,
		LastActivity: make(map[Pillar]time.Time, len(AllPillars)),
	}
	res.LastActivity[PillarProposal] = lastProposalActivity(cfg.LedgerDir)
	res.LastActivity[PillarJudge] = lastJudgeActivity(cfg.LedgerDir)
	res.LastActivity[PillarPromote] = lastPolicyActivity(cfg.BaselinePolicyPath, true)
	res.LastActivity[PillarRevert] = lastPolicyActivity(cfg.BaselinePolicyPath, false)

	cutoff := now.Add(-window)
	for _, p := range AllPillars {
		if res.LastActivity[p].Before(cutoff) {
			res.Stale = append(res.Stale, p)
		}
	}
	res.AllStale = len(res.Stale) == len(AllPillars)

	if cfg.ReplayDataPath != "" {
		res.ReplayFresh, res.ReplayDaysOld, res.ReplayErr = replayFreshness(cfg.ReplayDataPath, now, cfg.ReplayMaxDelay)
	} else {
		res.ReplayFresh = true // check disabled
	}
	return res
}

// lastProposalActivity returns the newest timestamp among all records in
// experiments.jsonl. Timestamp priority: ID-embedded unix suffix (the
// canonical source; status-transition records reuse the original ID so they
// never read as fresh), else WindowStart. Records with neither are skipped.
func lastProposalActivity(ledgerDir string) time.Time {
	if ledgerDir == "" {
		return time.Time{}
	}
	records := ledger.ExperimentsJSONL(ledgerDir)
	var latest time.Time
	for _, rec := range records {
		ts := experimentRecordTime(rec)
		if !ts.IsZero() && ts.After(latest) {
			latest = ts
		}
	}
	return latest
}

// experimentRecordTime extracts the best-available timestamp for a ledger
// record. Mirrors experimentRecordIsOld (auto.go) for the ID suffix.
func experimentRecordTime(rec domain.ExperimentRecord) time.Time {
	if i := strings.LastIndex(rec.ID, "-"); i >= 0 && i+1 < len(rec.ID) {
		if ts, err := strconv.ParseInt(rec.ID[i+1:], 10, 64); err == nil && ts > 1_000_000_000 {
			return time.Unix(ts, 0)
		}
	}
	if !rec.WindowStart.IsZero() {
		return rec.WindowStart
	}
	return time.Time{}
}

// lastJudgeActivity returns the newest judge outcome timestamp. Primary
// source: <ledgerDir>/experiments/*.json PromptExperimentResult files with
// status accepted/rejected (RecordedAt is refreshed at judge time).
// Fallback: accepted/rejected records in experiments.jsonl.
func lastJudgeActivity(ledgerDir string) time.Time {
	if ledgerDir == "" {
		return time.Time{}
	}
	var latest time.Time
	dir := filepath.Join(ledgerDir, "experiments")
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(filepath.Base(f), "_metadata.json") {
				continue
			}
			data, rerr := os.ReadFile(f)
			if rerr != nil {
				continue
			}
			var res domain.PromptExperimentResult
			if json.Unmarshal(data, &res) != nil {
				continue
			}
			if res.Experiment.Status != domain.ExperimentAccepted &&
				res.Experiment.Status != domain.ExperimentRejected {
				continue
			}
			if !res.RecordedAt.IsZero() && res.RecordedAt.After(latest) {
				latest = res.RecordedAt
			}
		}
	}
	// Fallback: ledger records that carry an explicit judge outcome.
	for _, rec := range ledger.ExperimentsJSONL(ledgerDir) {
		if rec.Status != domain.ExperimentAccepted && rec.Status != domain.ExperimentRejected {
			continue
		}
		if ts := experimentRecordTime(rec); !ts.IsZero() && ts.After(latest) {
			latest = ts
		}
	}
	return latest
}

// lastPolicyActivity returns the newest promoted_at (promote=true) or
// reverted_at (promote=false) across the baseline policy file. A missing
// or unparsable file yields the zero time (→ stale, which is correct: no
// promote/revert has ever happened).
func lastPolicyActivity(path string, promote bool) time.Time {
	if path == "" {
		return time.Time{}
	}
	policy, err := baseline.LoadStrict(path)
	if err != nil {
		return time.Time{}
	}
	var latest time.Time
	if promote {
		for _, rec := range policy.Promotions {
			if rec.PromotedAt.After(latest) {
				latest = rec.PromotedAt
			}
		}
	} else {
		for _, rec := range policy.RevertHistory {
			if rec.RevertedAt.After(latest) {
				latest = rec.RevertedAt
			}
		}
	}
	return latest
}

// replayFreshness reports whether the replay CSV's latest date is within
// maxDelay of now. Unreadable/missing replay is treated as not fresh (the
// caller surfaces ReplayErr in the alert details).
func replayFreshness(replayPath string, now time.Time, maxDelay time.Duration) (fresh bool, daysOld int, err error) {
	if maxDelay <= 0 {
		maxDelay = DefaultReplayMaxDelay
	}
	latest, err := latestReplayDate(replayPath)
	if err != nil {
		return false, 0, err
	}
	daysOld = int(now.Sub(latest).Hours() / 24)
	return now.Sub(latest) < maxDelay, daysOld, nil
}

// latestReplayDate scans the first column of the replay CSV and returns the
// newest 2006-01-02 date found. Mirrors getLatestReplayDate in
// cmd/atlas/bootstrap_helpers.go (kept local so the checker stays
// self-contained in package experiment).
func latestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	var latest time.Time
	_, _ = reader.Read() // header
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if len(row) == 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		if d.After(latest) {
			latest = d
		}
	}
	if latest.IsZero() {
		return time.Time{}, fmt.Errorf("no valid dates found in %s", csvPath)
	}
	return latest, nil
}

// RaiseEvolutionHealthAlerts maps a health result onto the existing monitor
// alert channel (AutoExperimentMonitor, implemented by experimentMonitorAdapter
// in cmd/atlas). Levels:
//   - every stale pillar → warning "no_<pillar>_activity_in_24h"
//   - all four pillars stale → error "evolution_loop_inactive_24h"
//     (raised INSTEAD of the four per-pillar warnings to avoid noise)
//   - replay behind ≥2 days → warning "replay_data_stale"
//   - replay unreadable → warning "replay_data_unavailable"
//
// Replay alerts are independent and always raised.
func RaiseEvolutionHealthAlerts(monitor AutoExperimentMonitor, res EvolutionHealthResult) {
	if monitor == nil {
		return
	}
	if res.AllStale {
		monitor.Alert("error", "evolution",
			"evolution_loop_inactive_24h: no proposal/judge/promote/revert activity in the last 24h",
			map[string]any{"pillars": pillarNames(res.Stale)})
	} else {
		for _, p := range res.Stale {
			last := res.LastActivity[p]
			lastStr := "never"
			if !last.IsZero() {
				lastStr = last.Format(time.RFC3339)
			}
			monitor.Alert("warning", "evolution",
				fmt.Sprintf("no_%s_activity_in_24h", p),
				map[string]any{"pillar": string(p), "last_activity": lastStr, "window": res.Window.String()})
		}
	}
	if res.ReplayErr != nil {
		monitor.Alert("warning", "evolution", "replay_data_unavailable",
			map[string]any{"error": res.ReplayErr.Error()})
	} else if !res.ReplayFresh {
		monitor.Alert("warning", "evolution", "replay_data_stale",
			map[string]any{"days_behind": res.ReplayDaysOld})
	}
}

func pillarNames(pillars []Pillar) []string {
	out := make([]string, len(pillars))
	for i, p := range pillars {
		out[i] = string(p)
	}
	return out
}
