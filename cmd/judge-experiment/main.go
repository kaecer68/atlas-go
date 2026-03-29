package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func main() {
	path := flag.String("result", "data/state/experiments/exec-growth-momentum-01-1774800459.json", "experiment result json path")
	flag.Parse()

	cfg := config.Load()
	judge := experiment.NewJudge(ledger.NewStore(cfg.LedgerDir), cfg.ReplayDataPath, cfg.BaselinePolicyPath)
	result, err := judge.Evaluate(*path)
	if err != nil {
		log.Fatalf("judge experiment: %v", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("baseline: %.6f\n", result.Experiment.BaselineValue)
	fmt.Printf("candidate: %.6f\n", result.Experiment.CandidateValue)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)
}
