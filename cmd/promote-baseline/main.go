package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
)

func main() {
	path := flag.String("result", "data/state/experiments/exec-growth-momentum-01-1774800459.json", "accepted experiment result json path")
	flag.Parse()

	cfg := config.Load()
	manager := baseline.NewManager(cfg.BaselinePolicyPath)
	policy, err := manager.PromoteResult(*path)
	if err != nil {
		log.Fatalf("promote baseline: %v", err)
	}

	fmt.Printf("baseline_policy: %s\n", cfg.BaselinePolicyPath)
	fmt.Printf("version: %d\n", policy.Version)
	fmt.Printf("prompt_overrides: %d\n", len(policy.PromptOverrides))
	fmt.Printf("promotions: %d\n", len(policy.Promotions))
	fmt.Printf("require_cro_pass: %t\n", policy.ExecutionPolicy.RequireCROPass)
	fmt.Printf("conviction_floor: %d\n", policy.ExecutionPolicy.ConvictionFloor)
}
