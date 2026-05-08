package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("backtest-window", flag.ContinueOnError)
	start := fs.String("start", "2026-03-26", "backtest window start date (YYYY-MM-DD)")
	end := fs.String("end", "2026-03-27", "backtest window end date (YYYY-MM-DD)")
	serve := fs.Bool("serve", false, "start dashboard API server after backtest completes")
	addr := fs.String("addr", ":8080", "dashboard API listen address (used with -serve)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		return fmt.Errorf("parse start date: %w", err)
	}
	endDate, err := time.Parse("2006-01-02", *end)
	if err != nil {
		return fmt.Errorf("parse end date: %w", err)
	}

	cfg := config.Load()
	runner := backtest.NewRunner(cfg, ledger.NewStore(cfg.LedgerDir))
	summary, err := runner.Run(startDate, endDate)
	if err != nil {
		return fmt.Errorf("run backtest window: %w", err)
	}

	fmt.Printf("window: %s\n", summary.WindowID)
	fmt.Printf("sessions: %d\n", summary.SessionCount)
	fmt.Printf("outcomes: %d\n", summary.OutcomeCount)
	fmt.Printf("worst_agent: %s\n", summary.WorstAgentID)
	fmt.Printf("worst_skill: %s\n", summary.WorstAgentSkill)
	fmt.Printf("worst_sharpe_like: %.6f\n", summary.WorstAgentSharpeLike)

	report, err := runner.GenerateReport(summary)
	if err != nil {
		log.Printf("warn: failed to generate report: %v", err)
	} else {
		fmt.Println(report)
	}

	if *serve {
		mux := http.NewServeMux()
		dashboard := monitoring.NewDashboardAPI(cfg.WorkDir, cfg.LedgerDir, nil)
		dashboard.RegisterRoutes(mux)
		dashboard.RegisterNarrativeRoutes(mux)
		dashboard.RegisterControlRoutes(mux)
		dashboard.RegisterMacroRoutes(mux)
		dashboard.RegisterExperimentRoutes(mux)
		dashboard.RegisterLiveRoutes(mux)
		dashboard.RegisterBacktestRoutes(mux)
		fmt.Printf("\nDashboard ready at http://localhost%s\n", *addr)
		fmt.Printf("Latest report: http://localhost%s/api/report/latest\n", *addr)
		if err := http.ListenAndServe(*addr, mux); err != nil {
			return fmt.Errorf("dashboard api server failed: %w", err)
		}
	}
	return nil
}
