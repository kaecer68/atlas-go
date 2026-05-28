package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

type AutoExperimentMonitor interface {
	Alert(level string, category, message string, details map[string]any)
}

type AutoExperimentConfig struct {
	System  *orchestrator.System
	Config  config.Config
	Monitor AutoExperimentMonitor
}

func AutoExperiment(ctx context.Context, cfg AutoExperimentConfig) error {
	if cfg.System == nil {
		return fmt.Errorf("AutoExperiment: System must not be nil")
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

	windowID := "window-" + time.Now().Add(-7*24*time.Hour).Format("20060102") + "-" + time.Now().Format("20060102")
	brief := domain.BuildMutationBrief(windowID, candidate)

	briefDir := filepath.Join(cfg.Config.WorkDir, "data", "state", "windows")
	_ = os.MkdirAll(briefDir, 0o755)
	briefPath := filepath.Join(briefDir, "auto-brief-"+candidate.Agent.ID+".json")
	briefData, _ := json.MarshalIndent(brief, "", "  ")
	if err := os.WriteFile(briefPath, briefData, 0o644); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}

	store := ledger.NewStore(cfg.Config.LedgerDir)
	executor := NewExecutor(store.(ledger.FullStore), cfg.Config.BaselinePolicyPath)
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

	judge := NewJudge(store.(ledger.ExperimentStore), cfg.Config.ReplayDataPath, cfg.Config.BaselinePolicyPath)
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

	if status == domain.ExperimentAccepted {
		mgr := baseline.NewManager(cfg.Config.BaselinePolicyPath)
		if _, err := mgr.PromoteResult(expPath); err != nil {
			if cfg.Monitor != nil {
				cfg.Monitor.Alert("error", "experiment",
					fmt.Sprintf("promote failed: agent=%s err=%v", candidate.Agent.ID, err),
					map[string]any{"agent": candidate.Agent.ID, "error": err.Error()})
			}
			return fmt.Errorf("promote result: %w", err)
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
