package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func main() {
	brief := flag.String("brief", "data/state/windows/window-20260326-20260327-mutation-brief.json", "mutation brief json path")
	flag.Parse()

	cfg := config.Load()
	if _, err := baseline.Load(cfg.BaselinePolicyPath); err != nil {
		log.Fatalf("load baseline policy: %v", err)
	}
	executor := experiment.NewExecutor(ledger.NewStore(cfg.LedgerDir))
	result, err := executor.Execute(*brief)
	if err != nil {
		log.Fatalf("execute experiment: %v", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("candidate_prompt: %s\n", result.CandidatePrompt)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)
	fmt.Printf("policy_checks: %d\n", len(result.PolicyChecks))
}
