package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
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
	addr := fs.String("addr", constants.AdminHTTPPort, "dashboard API listen address (used with -serve)")
	paramOverride := arrayFlags{}
	fs.Var(&paramOverride, "param-override", "override a ParametersConfig value (repeatable: -param-override name=value)")
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

	tempCleanup := func() {}
	if len(paramOverride) > 0 {
		ie := config.NewInferenceEngine(config.GetParametersConfig())
		if err := applyParamOverrides(ie, paramOverride); err != nil {
			return fmt.Errorf("param-override: %w", err)
		}
		overridePath, err := materializeParamConfig(ie)
		if err != nil {
			return fmt.Errorf("materialize param-override: %w", err)
		}
		cfg.ParametersConfigPath = overridePath
		tempCleanup = func() { _ = os.RemoveAll(overridePath) }
	}
	defer tempCleanup()

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
		srv := &http.Server{
			Addr:         *addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil {
			return fmt.Errorf("dashboard api server failed: %w", err)
		}
	}
	return nil
}

// arrayFlags implements flag.Value to accept repeated string flags.
type arrayFlags []string

func (a *arrayFlags) String() string { return strings.Join(*a, ",") }
func (a *arrayFlags) Set(v string) error {
	*a = append(*a, v)
	return nil
}

// parseParamOverride splits "name=value" and parses the value as float64.
// It rejects malformed entries and unknown parameter names.
func parseParamOverride(input string) (name string, value float64, err error) {
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("param-override: malformed %q (want name=value)", input)
	}
	name = strings.TrimSpace(parts[0])
	valStr := strings.TrimSpace(parts[1])
	if name == "" {
		return "", 0, fmt.Errorf("param-override: empty name in %q", input)
	}
	if valStr == "" {
		return "", 0, fmt.Errorf("param-override: empty value for %q", name)
	}
	value, err = strconv.ParseFloat(valStr, 64)
	if err != nil {
		return "", 0, fmt.Errorf("param-override: non-numeric value for %q: %w", name, err)
	}
	// Validate the parameter name is registered in the parameter table.
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())
	if _, ok := ie.GetParameter(name); !ok {
		return "", 0, fmt.Errorf("param-override: unknown parameter %q", name)
	}
	return name, value, nil
}

// applyParamOverrides parses each override string and applies it to the
// InferenceEngine via SetParameter.
func applyParamOverrides(ie *config.InferenceEngine, overrides []string) error {
	for _, o := range overrides {
		name, value, err := parseParamOverride(o)
		if err != nil {
			return err
		}
		if err := ie.SetParameter(name, value); err != nil {
			return fmt.Errorf("param-override: SetParameter(%q, %v): %w", name, value, err)
		}
	}
	return nil
}

// materializeParamConfig serializes the ParametersConfig from the
// InferenceEngine to a temporary JSON file and returns its path.
func materializeParamConfig(ie *config.InferenceEngine) (string, error) {
	tmpDir, err := os.MkdirTemp("", "backtest-params-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	path := tmpDir + "/parameters.json"

	data, err := json.MarshalIndent(ie.Parameters(), "", "  ")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("marshal params: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write params: %w", err)
	}
	return path, nil
}
