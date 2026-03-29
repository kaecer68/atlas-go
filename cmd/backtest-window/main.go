package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
)

func main() {
	start := flag.String("start", "2026-03-26", "backtest window start date (YYYY-MM-DD)")
	end := flag.String("end", "2026-03-27", "backtest window end date (YYYY-MM-DD)")
	flag.Parse()

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		log.Fatalf("parse start date: %v", err)
	}
	endDate, err := time.Parse("2006-01-02", *end)
	if err != nil {
		log.Fatalf("parse end date: %v", err)
	}

	runner := backtest.NewRunner(config.Load())
	summary, err := runner.Run(startDate, endDate)
	if err != nil {
		log.Fatalf("run backtest window: %v", err)
	}

	fmt.Printf("window: %s\n", summary.WindowID)
	fmt.Printf("sessions: %d\n", summary.SessionCount)
	fmt.Printf("outcomes: %d\n", summary.OutcomeCount)
	fmt.Printf("worst_agent: %s\n", summary.WorstAgentID)
	fmt.Printf("worst_skill: %s\n", summary.WorstAgentSkill)
	fmt.Printf("worst_sharpe_like: %.6f\n", summary.WorstAgentSharpeLike)
}
