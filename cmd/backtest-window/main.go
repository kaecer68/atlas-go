package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/reporting"
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

	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
	}
	runner := backtest.NewRunner(cfg)
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

	// Generate markdown report
	store := ledger.NewStore(cfg.LedgerDir)
	scorecards, _, err := store.LoadAllSessionScorecards()
	if err != nil {
		log.Printf("warn: failed to load scorecards: %v", err)
	}

	sessionSummaries, err := store.LoadSessionSummaries()
	if err != nil {
		log.Printf("warn: failed to load session summaries: %v", err)
	}

	equityCurve := make([]float64, 0, len(sessionSummaries))
	regimeCounts := make(map[string]int)
	for _, s := range sessionSummaries {
		pv := s.PortfolioValue
		if pv == 0 {
			pv = s.EndingCash
		}
		equityCurve = append(equityCurve, pv)
		regimeCounts[string(s.Regime)]++
	}

	reportData := reporting.BacktestReportData{
		WindowID:        summary.WindowID,
		StartDate:       summary.StartDate,
		EndDate:         summary.EndDate,
		SessionCount:    summary.SessionCount,
		OutcomeCount:    summary.OutcomeCount,
		EquityCurve:     equityCurve,
		AgentRows:       reporting.BuildAgentRows(scorecards, nil),
		MutationStats:   reporting.MutationStats{}, // TODO: populate from experiment results
		WorstAgentID:    summary.WorstAgentID,
		WorstAgentSkill: summary.WorstAgentSkill,
		WorstSharpeLike: summary.WorstAgentSharpeLike,
		RegimeCounts:    regimeCounts,
	}

	report := reporting.RenderMarkdown(reportData)
	reportDir := "reports"
	_ = os.MkdirAll(reportDir, 0o755)
	reportPath := filepath.Join(reportDir, fmt.Sprintf("backtest_%s.md", summary.WindowID))
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		log.Printf("warn: failed to write report: %v", err)
	} else {
		fmt.Printf("report written to: %s\n", reportPath)
	}
}
