package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("execute-experiment", flag.ContinueOnError)
	brief := fs.String("brief", "data/state/windows/window-20260326-20260327-mutation-brief.json", "mutation brief json path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
	}
	if _, err := baseline.Load(cfg.BaselinePolicyPath); err != nil {
		return fmt.Errorf("load baseline policy: %w", err)
	}
	executor := experiment.NewExecutor(ledger.NewStore(cfg.LedgerDir), cfg.BaselinePolicyPath)
	result, err := executor.Execute(*brief, cfg.ReplayDataPath)
	if err != nil {
		return fmt.Errorf("execute experiment: %w", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("candidate_prompt: %s\n", result.CandidatePrompt)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)
	fmt.Printf("policy_checks: %d\n", len(result.PolicyChecks))
	if result.DataMetadata != nil {
		fmt.Printf("data_range: %s to %s\n", result.DataMetadata.DateRangeStart.Format("2006-01-02"), result.DataMetadata.DateRangeEnd.Format("2006-01-02"))
		if result.DataMetadata.DaysDelayed > 2 {
			fmt.Printf("data_delay_warning: replay data is %d days delayed\n", result.DataMetadata.DaysDelayed)
		}
	}
	return nil
}
