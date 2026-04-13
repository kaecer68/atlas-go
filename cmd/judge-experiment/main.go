package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func findLatestExperiment(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" && name != "test-experiment.json" {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return filepath.Join(dir, files[0])
}

func run(args []string) error {
	defaultPath := findLatestExperiment("data/state/experiments")
	if defaultPath == "" {
		defaultPath = "data/state/experiments/exec-value-yield-01-1776084503.json"
	}

	fs := flag.NewFlagSet("judge-experiment", flag.ContinueOnError)
	path := fs.String("result", defaultPath, "experiment result json path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Load()
	judge := experiment.NewJudge(ledger.NewStore(cfg.LedgerDir), cfg.ReplayDataPath, cfg.BaselinePolicyPath)
	result, err := judge.Evaluate(*path)
	if err != nil {
		return fmt.Errorf("judge experiment: %w", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("baseline: %.6f\n", result.Experiment.BaselineValue)
	fmt.Printf("candidate: %.6f\n", result.Experiment.CandidateValue)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)
	return nil
}
