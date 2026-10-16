package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

type AutoExperimentMonitor interface {
	Alert(level string, category, message string, details map[string]any)
}

type AutoExperimentConfig struct {
	System   *orchestrator.System
	Config   config.Config
	Monitor  AutoExperimentMonitor
	Promoter *AutoJudgePromoter
	// Store is the experiments-family store (see
	// ledger.NewExperimentFamilyStore) used for every read
	// and write this pipeline performs. Optional seam: nil ⇒ built from Config,
	// which resolves cfg.Config.StoreBackend exactly like production does
	// (jsonl when unset, so tests and JSONL deployments keep the previous
	// behavior byte-for-byte). Injection exists so a test can pin that the
	// pipeline really routes through the supplied store.
	Store ledger.FullStore
}

func AutoExperiment(ctx context.Context, cfg AutoExperimentConfig) error {
	if cfg.System == nil {
		return fmt.Errorf("AutoExperiment: System must not be nil")
	}

	store, err := resolveAutoExperimentStore(cfg)
	if err != nil {
		// Fail loudly instead of continuing: silently falling back to JSONL on a
		// postgres deployment writes the lifecycle where no PG reader looks
		// (the failure mode scripts/ci/check_ledger_seam.py exists for).
		return err
	}
	cfg.Store = store

	// Drain hygiene: expire stale planned records before picking a candidate.
	// The ledger is append-only and historically never marked digested items,
	// so the backlog grew unbounded (fix manifest #D01).
	if expired, err := ExpireStalePlannedWithStore(store, cfg.Config.LedgerDir, stalePlannedTTL); err != nil {
		logging.Warn("experiment", "expire_stale_failed", logging.Err(err))
	} else if expired > 0 {
		logging.Info("experiment", "stale_planned_expired", "count", expired)
	}

	// Drain hygiene: expire abandoned "running" experiments whose latest
	// record is older than the TTL (audit A5: stuck running holds the
	// auto-judge pipeline hostage forever).
	if expired, err := ExpireStaleRunningWithStore(store, cfg.Config.LedgerDir, staleRunningTTL); err != nil {
		logging.Warn("experiment", "expire_stale_running_failed", logging.Err(err))
	} else if expired > 0 {
		logging.Info("experiment", "stale_running_expired", "count", expired)
	}

	// First, check if there are pending experiments from the daily pipeline that
	// haven't been tested yet. Process the oldest one to close the feedback loop.
	if pending, err := loadOldestPendingExperiment(store, cfg.Config.LedgerDir); err != nil {
		// A store read failure must not be answered with the frozen JSONL file
		// (that is the split this unit removes): report and skip the drain step,
		// then fall through to the normal candidate selection.
		logging.Warn("experiment", "pending_experiments_unreadable", logging.Err(err))
	} else if pending != nil {
		candidate := pending.toCandidate(cfg.System.GetRegistry())
		if candidate != nil {
			logging.Info("experiment", "processing_pending",
				"agent", candidate.Agent.ID, "experiment_id", pending.ID)
			return runExperimentForCandidate(ctx, cfg, candidate)
		}
	}

	candidate, err := cfg.System.NextExperimentCandidate()
	if err != nil {
		return fmt.Errorf("identify candidate: %w", err)
	}
	if candidate == nil {
		logging.Info("experiment", "no_candidate", "all agents currently healthy")
		return nil
	}

	logging.Info("experiment", "candidate_selected",
		"agent", candidate.Agent.ID,
		"skill", candidate.Agent.Skill,
		"sharpe", fmt.Sprintf("%.3f", candidate.Scorecard.SharpeLike))

	return runExperimentForCandidate(ctx, cfg, candidate)
}

// runExperimentForCandidate executes the full experiment pipeline for a candidate:
// build brief → run executor → judge → update ledger → promote if accepted.
func runExperimentForCandidate(_ context.Context, cfg AutoExperimentConfig, candidate *domain.Candidate) error {
	// Phase B1 replay freshness gate: when the replay lags the experiment
	// window by ≤1 day, defer the window to the replay's latest date instead of
	// failing with 數據不足; a lag of ≥2 days fails loudly for daily-replay-sync
	// triage (B4 alerting covers the standing failure case).
	windowStart, windowEnd, deferred, err := resolveExperimentWindow(time.Now(), cfg.Config.ReplayDataPath)
	if err != nil {
		return fmt.Errorf("replay freshness gate: %w", err)
	}
	if deferred {
		logging.Info("experiment", "window_deferred",
			"agent", candidate.Agent.ID,
			"window_start", windowStart.Format("2006-01-02"),
			"window_end", windowEnd.Format("2006-01-02"),
			"reason", "replay data lags experiment window by at most 1 day")
	}
	windowID := "window-" + windowStart.Format("20060102") + "-" + windowEnd.Format("20060102")
	brief := domain.BuildMutationBrief(windowID, candidate)

	briefDir := filepath.Join(cfg.Config.WorkDir, "data", "state", "windows")
	_ = os.MkdirAll(briefDir, 0o755)
	briefPath := filepath.Join(briefDir, "auto-brief-"+candidate.Agent.ID+".json")
	briefData, _ := json.MarshalIndent(brief, "", "  ")
	if err := os.WriteFile(briefPath, briefData, 0o644); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}

	store, err := resolveAutoExperimentStore(cfg)
	if err != nil {
		return err
	}
	executor := NewExecutor(store, cfg.Config.BaselinePolicyPath)
	result, runErr := executor.Run(briefPath, cfg.Config.ReplayDataPath)
	if runErr != nil {
		if cfg.Monitor != nil {
			cfg.Monitor.Alert("warning", "experiment",
				fmt.Sprintf("experiment failed: agent=%s err=%v", candidate.Agent.ID, runErr),
				map[string]any{"agent": candidate.Agent.ID, "error": runErr.Error()})
		}
		return fmt.Errorf("run experiment: %w", runErr)
	}

	expPath := FindLatestExperiment(filepath.Join(cfg.Config.WorkDir, "data", "state", "experiments"))
	if expPath == "" {
		return fmt.Errorf("experiment result not found for %s", result.Experiment.ID)
	}

	// #1774: materialize the experiment window's summary before judging. No
	// upstream writer ever produced data/state/windows/<windowID>.json for the
	// experiment's own trailing window — window_backtest and autobacktest write
	// windows with different spans (21d rolling / 1d), so judge.Evaluate
	// reliably failed with "open .../windows/window-YYYYMMDD-YYYYMMDD.json: no
	// such file or directory" (71 consecutive failures in prod, 2026-08-31).
	// Run the backtest over the experiment window on demand; the Runner records
	// the summary via ledger.RecordWindowSummary into the same base dir the
	// judge reads.
	baseDir := filepath.Dir(filepath.Dir(expPath))
	windowPath := filepath.Join(baseDir, "windows", windowID+".json")
	if _, statErr := os.Stat(windowPath); os.IsNotExist(statErr) {
		// Explicit JSONL backend, deliberately not the deployment's declared
		// backend: this summary is an experiments-internal artifact that
		// experiment.Judge reads back BY FILE PATH (judge.go windowSummaryPath
		// → loadWindowSummary), so it must be written into the directory the
		// judge looks in. Naming the backend (instead of relying on the
		// ledger.NewStore default) keeps a postgres deployment from silently
		// writing the window somewhere the judge cannot see — the same
		// convention as autobacktest.NewSignalEngineFromDir.
		//
		// Q2 batch 2 unit A (2026-10-16) made the experiments-writer decision
		// this note used to defer to, and kept the window summary on the file
		// contract: experiment.Judge still reads it by path, so converging this
		// sibling requires moving the judge's read in the same change. The gap is
		// registered as FU-20261016-09 (result file + window summary readers).
		windowStore, storeErr := ledger.NewFullStore(config.Config{
			StoreBackend: "jsonl",
			LedgerDir:    baseDir,
		})
		if storeErr != nil {
			return fmt.Errorf("experiment window store: %w", storeErr)
		}
		btRunner := backtest.NewRunner(cfg.Config, windowStore)
		summary, btErr := btRunner.Run(windowStart, windowEnd)
		if btErr != nil {
			return fmt.Errorf("materialize window summary %s: %w", windowID, btErr)
		}
		logging.Info("experiment", "window_summary_materialized",
			"window_id", summary.WindowID,
			"sessions", summary.SessionCount)
	}

	judge := NewJudge(store, cfg.Config.ReplayDataPath, cfg.Config.BaselinePolicyPath)
	judged, judgeErr := judge.Evaluate(expPath)
	if judgeErr != nil {
		return fmt.Errorf("judge experiment: %w", judgeErr)
	}

	status := judged.Experiment.Status
	logging.Info("experiment", "judged",
		"agent", candidate.Agent.ID,
		"status", status,
		"baseline", fmt.Sprintf("%.3f", judged.Experiment.BaselineValue),
		"candidate", fmt.Sprintf("%.3f", judged.Experiment.CandidateValue))

	// Close the planned→tested feedback loop: append the actual experiment result
	// to the ledger so the inbox shows real values instead of "待測試".
	//
	// WindowStart/WindowEnd are carried over from the judged result (the judge
	// fills them from the window summary). This write is the only place that
	// records the accepted/rejected transition, and the stores derive the
	// experiments row's `timestamp` column from WindowStart — leaving it zero
	// would make every transitioned row look like 0001-01-01 once the status
	// table is upserted rather than written once (Q2 batch 2 unit A).
	resultRec := domain.ExperimentRecord{
		ID:                   candidate.Experiment.ID,
		TargetAgentID:        candidate.Agent.ID,
		Skill:                candidate.Agent.Skill,
		MutationType:         candidate.Experiment.MutationType,
		Status:               status,
		BaselineValue:        judged.Experiment.BaselineValue,
		CandidateValue:       judged.Experiment.CandidateValue,
		BaselineMonetaryNTD:  judged.Experiment.BaselineMonetaryNTD,
		CandidateMonetaryNTD: judged.Experiment.CandidateMonetaryNTD,
		AcceptanceMetric:     candidate.Experiment.AcceptanceMetric,
		WindowStart:          judged.Experiment.WindowStart,
		WindowEnd:            judged.Experiment.WindowEnd,
	}
	if err := store.RecordExperiment(resultRec); err != nil {
		// ERROR, not WARN: this single write is what carries the accepted flag
		// into the status table. Losing it silently is how production ended up
		// reporting a 0% acceptance rate (FU-20261016-04). The pipeline still
		// continues — the promotion decision has already been made and recorded
		// elsewhere — but the failure must be visible.
		logging.Error("experiment", "ledger_update_failed",
			"experiment_id", resultRec.ID,
			"status", string(status),
			logging.Err(err))
	}

	if status == domain.ExperimentAccepted {
		if cfg.Promoter != nil {
			promoted, note, err := cfg.Promoter.TryPromoteFromPath(expPath)
			if err != nil {
				if cfg.Monitor != nil {
					cfg.Monitor.Alert("error", "experiment",
						fmt.Sprintf("promote failed: agent=%s err=%v", candidate.Agent.ID, err),
						map[string]any{"agent": candidate.Agent.ID, "error": err.Error()})
				}
				return fmt.Errorf("promote result: %w", err)
			}
			if !promoted {
				logging.Info("experiment", "promote_deferred",
					"agent", candidate.Agent.ID,
					"reason", note)
				if cfg.Monitor != nil {
					cfg.Monitor.Alert("info", "experiment",
						fmt.Sprintf("promote deferred: agent=%s reason=%s", candidate.Agent.ID, note),
						map[string]any{"agent": candidate.Agent.ID, "reason": note})
				}
				return nil
			}
		} else {
			mgr := baseline.NewManager(cfg.Config.BaselinePolicyPath)
			if _, err := mgr.PromoteResult(expPath); err != nil {
				if cfg.Monitor != nil {
					cfg.Monitor.Alert("error", "experiment",
						fmt.Sprintf("promote failed: agent=%s err=%v", candidate.Agent.ID, err),
						map[string]any{"agent": candidate.Agent.ID, "error": err.Error()})
				}
				return fmt.Errorf("promote result: %w", err)
			}
		}
		if cfg.Monitor != nil {
			cfg.Monitor.Alert("info", "experiment",
				fmt.Sprintf("strategy promoted: agent=%s (%s)", candidate.Agent.ID, candidate.Agent.Skill),
				map[string]any{
					"agent":     candidate.Agent.ID,
					"skill":     candidate.Agent.Skill,
					"status":    string(status),
					"baseline":  judged.Experiment.BaselineValue,
					"candidate": judged.Experiment.CandidateValue,
				})
		}
	} else {
		if cfg.Monitor != nil {
			cfg.Monitor.Alert("info", "experiment",
				fmt.Sprintf("experiment rejected: agent=%s (%s)", candidate.Agent.ID, candidate.Agent.Skill),
				map[string]any{
					"agent":     candidate.Agent.ID,
					"skill":     candidate.Agent.Skill,
					"status":    string(status),
					"baseline":  judged.Experiment.BaselineValue,
					"candidate": judged.Experiment.CandidateValue,
				})
		}
	}
	return nil
}

// pendingExperiment is a lightweight experiment record loaded from the ledger.
type pendingExperiment struct {
	ID            string
	TargetAgentID string
	Skill         string
	MutationType  string
	BaselineValue float64
}

func (p *pendingExperiment) toCandidate(registry domain.AgentRegistry) *domain.Candidate {
	for _, a := range registry.Agents {
		if a.ID == p.TargetAgentID && a.Enabled {
			return &domain.Candidate{
				Agent: a,
				Scorecard: domain.Scorecard{
					AgentID:     a.ID,
					SharpeLike:  p.BaselineValue,
					WindowCount: 1,
				},
				Experiment: domain.ExperimentRecord{
					ID:               p.ID,
					TargetAgentID:    a.ID,
					Skill:            a.Skill,
					MutationType:     p.MutationType,
					Status:           domain.ExperimentPlanned,
					BaselineValue:    p.BaselineValue,
					AcceptanceMetric: "sharpe_like",
				},
			}
		}
	}
	return nil
}

// stalePlannedTTL bounds how long a planned experiment may sit undigested
// before ExpireStalePlanned marks it expired.
const stalePlannedTTL = 30 * 24 * time.Hour

// staleRunningTTL bounds how long an experiment may sit in "running" status
// before ExpireStaleRunning marks it expired. It shares the 30-day TTL
// semantics of stalePlannedTTL so abandoned runs cannot hold the
// auto-judge pipeline hostage forever (audit A5: 13 stuck running).
const staleRunningTTL = stalePlannedTTL

// ExpireStalePlanned appends an "expired" record for every planned
// experiment whose oldest record is older than maxAge. Append-only semantics
// are preserved (no rewrite); readers folding by ID see the new status.
//
// store == nil keeps the pre-migration behavior (read experiments.jsonl); the
// live pipeline passes the deployment's store instead, so it expires the same
// records it will later write the transition to. See
// ledger.LoadExperimentRecords for why a store-backed read is the only one that
// sees the live stream on a postgres deployment.
func ExpireStalePlanned(ledgerDir string, maxAge time.Duration) (int, error) {
	return ExpireStalePlannedWithStore(nil, ledgerDir, maxAge)
}

// ExpireStalePlannedWithStore is ExpireStalePlanned through an optional store.
// The write half goes through the same store: writing an expiry to JSONL while
// the pipeline reads PostgreSQL (or the reverse) would leave the transition
// invisible to the reader that needs it.
func ExpireStalePlannedWithStore(store ledger.FullStore, ledgerDir string, maxAge time.Duration) (int, error) {
	records, err := ledger.LoadExperimentRecords(store, ledgerDir)
	if err != nil {
		return 0, err
	}
	latest := ledger.LatestExperimentStatusByID(records)
	firstRec := make(map[string]domain.ExperimentRecord)
	for _, rec := range records {
		if rec.ID == "" {
			continue
		}
		if _, ok := firstRec[rec.ID]; !ok {
			firstRec[rec.ID] = rec
		}
	}
	cutoff := time.Now().Add(-maxAge)
	var toExpire []domain.ExperimentRecord
	for id, st := range latest {
		if st != domain.ExperimentPlanned {
			continue
		}
		rec := firstRec[id]
		// Records carry no explicit timestamp field; fall back to ID-embedded
		// unix suffix when present, else expire conservatively (treat as old).
		if !experimentRecordIsOld(rec, cutoff) {
			continue
		}
		toExpire = append(toExpire, domain.ExperimentRecord{
			ID:            rec.ID,
			TargetAgentID: rec.TargetAgentID,
			Skill:         rec.Skill,
			MutationType:  rec.MutationType,
			Status:        domain.ExperimentExpired,
		})
	}
	if len(toExpire) == 0 {
		return 0, nil
	}
	writer, err := expireWriter(store, ledgerDir)
	if err != nil {
		return 0, err
	}
	for _, rec := range toExpire {
		if err := writer.RecordExperiment(rec); err != nil {
			return 0, fmt.Errorf("expire %s: %w", rec.ID, err)
		}
	}
	return len(toExpire), nil
}

// expireWriter resolves the store the expiry transition is appended to.
// store == nil falls back to an EXPLICIT jsonl factory for the caller's ledger
// directory — the same JSONL file ledger.NewStore(ledgerDir) used to target, so
// the no-store callers keep their behavior exactly, while the backend is named
// instead of left to a default (the ledger seam guard's contract).
func expireWriter(store ledger.FullStore, ledgerDir string) (ledger.FullStore, error) {
	if store != nil {
		return store, nil
	}
	jsonlStore, err := ledger.NewFullStore(config.Config{
		StoreBackend: "jsonl",
		LedgerDir:    ledgerDir,
	})
	if err != nil {
		return nil, fmt.Errorf("expire experiments (jsonl): %w", err)
	}
	return jsonlStore, nil
}

// ExpireStaleRunning appends an "expired" record for every experiment whose
// latest ledger status is "running" and whose oldest record is older than
// maxAge. Mirror of ExpireStalePlanned for abandoned runs: an executor that
// died mid-run must not pin its experiment in "running" forever (audit A5).
// Append-only semantics are preserved (no rewrite); readers folding by ID
// see the new status.
func ExpireStaleRunning(ledgerDir string, maxAge time.Duration) (int, error) {
	return ExpireStaleRunningWithStore(nil, ledgerDir, maxAge)
}

// ExpireStaleRunningWithStore is ExpireStaleRunning through an optional store
// (read and write halves both). See ExpireStalePlannedWithStore.
func ExpireStaleRunningWithStore(store ledger.FullStore, ledgerDir string, maxAge time.Duration) (int, error) {
	records, err := ledger.LoadExperimentRecords(store, ledgerDir)
	if err != nil {
		return 0, err
	}
	latest := ledger.LatestExperimentStatusByID(records)
	firstRec := make(map[string]domain.ExperimentRecord)
	for _, rec := range records {
		if rec.ID == "" {
			continue
		}
		if _, ok := firstRec[rec.ID]; !ok {
			firstRec[rec.ID] = rec
		}
	}
	cutoff := time.Now().Add(-maxAge)
	var toExpire []domain.ExperimentRecord
	for id, st := range latest {
		if st != domain.ExperimentRunning {
			continue
		}
		rec := firstRec[id]
		// Records carry no explicit timestamp field; fall back to ID-embedded
		// unix suffix when present, else expire conservatively (treat as old).
		if !experimentRecordIsOld(rec, cutoff) {
			continue
		}
		toExpire = append(toExpire, domain.ExperimentRecord{
			ID:            rec.ID,
			TargetAgentID: rec.TargetAgentID,
			Skill:         rec.Skill,
			MutationType:  rec.MutationType,
			Status:        domain.ExperimentExpired,
			RevertReason:  "expired: stuck in running beyond TTL",
		})
	}
	if len(toExpire) == 0 {
		return 0, nil
	}
	writer, err := expireWriter(store, ledgerDir)
	if err != nil {
		return 0, err
	}
	for _, rec := range toExpire {
		if err := writer.RecordExperiment(rec); err != nil {
			return 0, fmt.Errorf("expire running %s: %w", rec.ID, err)
		}
	}
	return len(toExpire), nil
}

// experimentRecordIsOld reports whether the record predates cutoff. Since
// ExperimentRecord has no timestamp, age is inferred from a unix-timestamp
// suffix in the ID when present; otherwise the record is treated as old.
func experimentRecordIsOld(rec domain.ExperimentRecord, cutoff time.Time) bool {
	if i := strings.LastIndex(rec.ID, "-"); i >= 0 && i+1 < len(rec.ID) {
		if ts, err := strconv.ParseInt(rec.ID[i+1:], 10, 64); err == nil && ts > 1_000_000_000 {
			return time.Unix(ts, 0).Before(cutoff)
		}
	}
	return true
}

// loadOldestPendingExperiment reads the experiments ledger to find the oldest
// planned experiment that hasn't been tested yet. Resolution is last-write-wins
// per experiment ID, so digested items are never re-picked (fix manifest #D01).
//
// store == nil keeps the pre-migration behavior (experiments.jsonl); the live
// pipeline passes the deployment's store, because the planned records are
// written by the replay pipeline through the backend-aware store
// (internal/orchestrator/system.go) and are therefore in PostgreSQL on
// production, not in the file (FU-20261016-04: the file stopped at
// 2026-10-13T23:39Z while PostgreSQL kept receiving rows).
func loadOldestPendingExperiment(store ledger.FullStore, ledgerDir string) (*pendingExperiment, error) {
	records, err := ledger.LoadExperimentRecords(store, ledgerDir)
	if err != nil {
		return nil, err
	}
	latest := ledger.LatestExperimentStatusByID(records)
	for _, rec := range records {
		if rec.Status != domain.ExperimentPlanned || rec.TargetAgentID == "" {
			continue
		}
		if latest[rec.ID] != domain.ExperimentPlanned {
			continue // resolved by a later record
		}
		return &pendingExperiment{
			ID:            rec.ID,
			TargetAgentID: rec.TargetAgentID,
			Skill:         rec.Skill,
			MutationType:  rec.MutationType,
			BaselineValue: rec.BaselineValue,
		}, nil
	}
	return nil, nil
}

// resolveAutoExperimentStore returns the injected store when the caller set
// one, and builds the family store from cfg.Config otherwise (Config.StoreBackend
// unset ⇒ jsonl, i.e. the previous behavior).
func resolveAutoExperimentStore(cfg AutoExperimentConfig) (ledger.FullStore, error) {
	if cfg.Store != nil {
		return cfg.Store, nil
	}
	return ledger.NewExperimentFamilyStore(cfg.Config)
}

// LoadPendingExperiments returns all pending experiment results from the
// data/state/experiments directory. Pending = Status not in {Accepted, Rejected}.
// Files that fail to parse are skipped (logged at Warn) so partial corruption
// does not block the auto-judge-promoter wire.
func LoadPendingExperiments(workDir string) []experiment.PromptExperimentResult {
	expDir := filepath.Join(workDir, "data", "state", "experiments")
	files, err := filepath.Glob(filepath.Join(expDir, "*.json"))
	if err != nil {
		logging.Warn("auto_judge", "load_pending_glob_failed", "dir", expDir, "err", err.Error())
		return nil
	}
	var pending []experiment.PromptExperimentResult
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			logging.Warn("auto_judge", "load_pending_read_failed", "file", filepath.Base(f), "err", err.Error())
			continue
		}
		var r experiment.PromptExperimentResult
		if err := json.Unmarshal(data, &r); err != nil {
			logging.Warn("auto_judge", "load_pending_parse_failed", "file", filepath.Base(f), "err", err.Error())
			continue
		}
		switch r.Experiment.Status {
		case domain.ExperimentAccepted, domain.ExperimentRejected, domain.ExperimentExpired:
			continue
		}
		// Stuck-running TTL: a result left in "running" beyond the TTL is
		// abandoned (executor died). Skip it so it never blocks the
		// auto-judge wire again (audit A5). The ledger-level expiry
		// (ExpireStaleRunning) records the transition.
		if r.Experiment.Status == domain.ExperimentRunning &&
			!r.RecordedAt.IsZero() &&
			time.Since(r.RecordedAt) > staleRunningTTL {
			logging.Warn("auto_judge", "stuck_running_skipped",
				"file", filepath.Base(f),
				"experiment_id", r.Experiment.ID,
				"recorded_at", r.RecordedAt.Format(time.RFC3339))
			continue
		}
		pending = append(pending, r)
	}
	return pending
}
