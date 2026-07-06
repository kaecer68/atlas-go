package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/repository"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func findLatestExperiment(dir string) string {
	return experiment.FindLatestExperiment(dir)
}

func run(args []string) error {
	defaultPath := findLatestExperiment(constants.StateExperiments)
	if defaultPath == "" {
		defaultPath = "data/state/experiments/exec-value-yield-01-1776084503.json"
	}

	fs := flag.NewFlagSet("judge-experiment", flag.ContinueOnError)
	path := fs.String("result", defaultPath, "experiment result json path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = constants.ReplayCSVPath
	}
	store := ledger.NewStore(cfg.LedgerDir)
	judge := experiment.NewJudge(store.(ledger.ExperimentStore), cfg.ReplayDataPath, cfg.BaselinePolicyPath)
	result, err := judge.Evaluate(*path)
	if err != nil {
		return fmt.Errorf("judge experiment: %w", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("baseline: %.6f\n", result.Experiment.BaselineValue)
	fmt.Printf("candidate: %.6f\n", result.Experiment.CandidateValue)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pool, poolErr := db.Init(context.Background(), dsn, filepath.Join(filepath.Dir(os.Args[0]), "..", "..", "sql", "migrations"))
		if poolErr == nil {
			defer pool.Close()
			collector := monitoring.NewMetricsCollector()
			snap := collector.GetMetricsSnapshot()
			repoSnap := repository.MetricsSnapshot{
				ScreeningTotal:     snap.ScreeningTotal,
				ScreeningPassed:    snap.ScreeningPassed,
				ScreeningRate:      snap.ScreeningRate,
				AlertsTriggered:    snap.AlertsTriggered,
				AlertsAcknowledged: snap.AlertsAcknowledged,
				AlertsByType:       snap.AlertsByType,
				Timestamp:          time.Now(),
			}
			repo := repository.NewDualWriteRepository(pool, nil, nil, nil, nil, nil, nil)
			if err := repo.SaveSnapshot(context.Background(), &repoSnap); err != nil {
				log.Printf("[Metrics] snapshot save failed: %v", err)
			} else {
				log.Printf("[Metrics] snapshot saved")
			}
		}
	}
	return nil
}
