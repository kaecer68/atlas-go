package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
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

	fs := flag.NewFlagSet("promote-baseline", flag.ContinueOnError)
	path := fs.String("result", defaultPath, "accepted experiment result json path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Load()
	manager := baseline.NewManager(cfg.BaselinePolicyPath)
	policy, err := manager.PromoteResult(*path)
	if err != nil {
		return fmt.Errorf("promote baseline: %w", err)
	}

	fmt.Printf("baseline_policy: %s\n", cfg.BaselinePolicyPath)
	fmt.Printf("version: %d\n", policy.Version)
	fmt.Printf("prompt_overrides: %d\n", len(policy.PromptOverrides))
	fmt.Printf("promotions: %d\n", len(policy.Promotions))
	fmt.Printf("require_cro_pass: %t\n", policy.ExecutionPolicy.RequireCROPass)
	fmt.Printf("conviction_floor: %d\n", policy.ExecutionPolicy.ConvictionFloor)
	return nil
}
