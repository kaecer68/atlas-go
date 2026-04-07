package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
)

func main() {
	var (
		toVersion    = flag.Int("to-version", 0, "revert to specific version number (0 = revert last promotion)")
		toExperiment = flag.String("to-experiment", "", "revert to state before specific experiment ID")
		reason       = flag.String("reason", "", "reason for revert (required)")
		dryRun       = flag.Bool("dry-run", false, "show what would be reverted without making changes")
		listHistory  = flag.Bool("list", false, "show promotion history and exit")
	)
	flag.Parse()

	cfg := config.Load()
	manager := baseline.NewManager(cfg.BaselinePolicyPath)

	// List mode: show history and exit
	if *listHistory {
		if err := showPromotionHistory(manager); err != nil {
			log.Fatalf("failed to show history: %v", err)
		}
		return
	}

	// Validate required flags
	if *reason == "" && !*dryRun {
		fmt.Fprintf(os.Stderr, "Error: --reason is required for revert (use --dry-run to preview)\n")
		flag.Usage()
		os.Exit(1)
	}

	// Determine revert target
	var target baseline.RevertTarget
	if *toVersion > 0 {
		target = baseline.RevertTarget{
			Type:    baseline.RevertToVersion,
			Version: *toVersion,
		}
	} else if *toExperiment != "" {
		target = baseline.RevertTarget{
			Type:         baseline.RevertToExperiment,
			ExperimentID: *toExperiment,
		}
	} else {
		// Default: revert last promotion
		target = baseline.RevertTarget{
			Type: baseline.RevertLast,
		}
	}

	// Execute revert
	result, err := manager.Revert(target, *reason, *dryRun)
	if err != nil {
		log.Fatalf("revert failed: %v", err)
	}

	// Output results
	if *dryRun {
		fmt.Println("=== DRY RUN - No changes made ===")
	}
	fmt.Printf("baseline_policy: %s\n", cfg.BaselinePolicyPath)
	fmt.Printf("reverted_from_version: %d\n", result.FromVersion)
	fmt.Printf("reverted_to_version: %d\n", result.ToVersion)
	fmt.Printf("reverted_experiments: %d\n", len(result.RevertedExperiments))
	fmt.Printf("reason: %s\n", result.Reason)
	fmt.Printf("reverted_at: %s\n", result.RevertedAt.Format("2006-01-02T%H:%M:%S"))

	if len(result.RevertedExperiments) > 0 {
		fmt.Println("\nreverted_experiment_ids:")
		for _, expID := range result.RevertedExperiments {
			fmt.Printf("  - %s\n", expID)
		}
	}

	if result.DryRun {
		fmt.Println("\n=== Use without --dry-run to apply revert ===")
	}
}

func showPromotionHistory(manager *baseline.Manager) error {
	history, err := manager.GetPromotionHistory()
	if err != nil {
		return err
	}

	if len(history) == 0 {
		fmt.Println("No promotion history found.")
		return nil
	}

	fmt.Println("Promotion History:")
	fmt.Println("==================")
	fmt.Printf("%-5s %-20s %-15s %-20s %-10s\n", "VER", "EXPERIMENT", "SKILL", "TIME", "STATUS")
	fmt.Println(string(make([]byte, 80)))

	for _, record := range history {
		fmt.Printf("%-5d %-20s %-15s %-20s %-10s\n",
			record.Version,
			truncate(record.ExperimentID, 18),
			truncate(record.TargetSkill, 13),
			record.PromotedAt.Format("2006-01-02 %H:%M"),
			record.Status,
		)
	}

	fmt.Printf("\nTotal promotions: %d\n", len(history))
	fmt.Printf("Current version: %d\n", history[len(history)-1].Version)

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
