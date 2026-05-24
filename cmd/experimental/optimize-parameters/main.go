package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/config"
)

type sessionData struct {
	Summary  sessionSummary
	Outcomes []outcome
}

type sessionSummary struct {
	PortfolioValue float64 `json:"portfolio_value"`
	EndingCash     float64 `json:"ending_cash"`
}

type outcome struct {
	Symbol     string  `json:"symbol"`
	Conviction int     `json:"conviction"`
	Side       string  `json:"side"`
	Price      float64 `json:"price"`
}

func main() {
	help := flag.Bool("help", false, "show usage")
	stateDir := flag.String("state", "data/state", "path to state directory")
	flag.Parse()

	if *help {
		fmt.Println("Usage: go run ./cmd/experimental/optimize-parameters [--state <path>]")
		fmt.Println()
		fmt.Println("Runs Bayesian optimization on risk gate parameters against historical session data.")
		fmt.Println("Scores each parameter set by counting the fraction of trades that would pass")
		fmt.Println("through the PreTradeGate with a balanced intercept profile.")
		fmt.Println()
		fmt.Println("Flags:")
		fmt.Println("  --state   path to state directory (default: data/state)")
		os.Exit(0)
	}

	sessions := loadSessions(*stateDir)
	if len(sessions) == 0 {
		fmt.Fprintf(os.Stderr, "No sessions found. Run a simulation first: go run ./cmd/atlas -api\n")
		os.Exit(1)
	}

	ie := config.NewInferenceEngine(config.GetParametersConfig())

	evaluator := func(cfg *config.ParametersConfig) (float64, error) {
		return evaluateGateEffectiveness(sessions, cfg)
	}

	paramNames := []string{
		"risk_max_position_size",
		"risk_max_daily_loss_pct",
	}

	result, err := ie.OptimizeBayesian(paramNames, evaluator, config.DefaultOptimizerConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "optimization failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Println("  Bayesian Parameter Optimization — Results")
	fmt.Println("═══════════════════════════════════════════════════")
	fmt.Printf("  Evaluations:              %d\n", result.Observations)
	fmt.Printf("  Best score:               %.4f\n", result.BestScore)
	fmt.Println()
	fmt.Println("  ─── Optimized Parameters ───")
	for name, val := range result.ParamValues {
		current, _ := ie.GetParameter(name)
		delta := (val - current) / current * 100
		fmt.Printf("  %-40s current=%.4f → best=%.4f (%+.1f%%)\n", name, current, val, delta)
	}
	fmt.Println()
	fmt.Println("  TIP: Apply optimized values by updating configs/parameters.json")
	fmt.Println("       with the best values above, then restart the server.")
}

func loadSessions(stateDir string) []sessionData {
	pattern := filepath.Join(stateDir, "sessions", "session-*")
	dirs, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var sessions []sessionData
	for _, d := range dirs {
		info, statErr := os.Stat(d)
		if statErr != nil || !info.IsDir() {
			continue
		}
		sd := loadSessionDir(d)
		if sd.Summary.PortfolioValue > 0 {
			sessions = append(sessions, sd)
		}
	}
	return sessions
}

func loadSessionDir(dir string) sessionData {
	sd := sessionData{}
	data, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err == nil {
		_ = json.Unmarshal(data, &sd.Summary)
	}
	data, err = os.ReadFile(filepath.Join(dir, "recommendation_outcomes.jsonl"))
	if err == nil {
		sd.Outcomes = parseOutcomes(data)
	}
	return sd
}

func parseOutcomes(data []byte) []outcome {
	var out []outcome
	start := 0
	for i, b := range data {
		if b != '\n' {
			continue
		}
		if i > start {
			var o outcome
			if json.Unmarshal(data[start:i], &o) == nil && o.Symbol != "" {
				out = append(out, o)
			}
		}
		start = i + 1
	}
	if start < len(data) {
		var o outcome
		if json.Unmarshal(data[start:], &o) == nil && o.Symbol != "" {
			out = append(out, o)
		}
	}
	return out
}

func evaluateGateEffectiveness(sessions []sessionData, params *config.ParametersConfig) (float64, error) {
	total := 0
	blocked := 0
	allowedButBad := 0

	totalValue := 0.0
	for _, s := range sessions {
		totalValue += s.Summary.PortfolioValue
	}
	if len(sessions) > 0 {
		totalValue /= float64(len(sessions))
	}
	if totalValue <= 0 {
		totalValue = 3_000_000
	}

	maxPosition := params.Risk.MaxPositionSize.Value
	maxDailyLoss := params.Risk.MaxDailyLossPct.Value

	for _, s := range sessions {
		for _, o := range s.Outcomes {
			total++
			notional := totalValue * 0.05
			pct := notional / totalValue
			lossPct := notional * 1.5 / totalValue

			blockedByPosition := pct > maxPosition
			blockedByDailyLoss := lossPct > maxDailyLoss
			isBlocked := blockedByPosition || blockedByDailyLoss

			if isBlocked {
				blocked++
			} else {
				if o.Conviction < 30 {
					allowedButBad++
				}
			}
		}
	}

	if total == 0 {
		return 0, nil
	}

	interceptRate := float64(blocked) / float64(total)
	badRate := float64(allowedButBad) / float64(total)

	if interceptRate > 0.5 {
		return -1.0, nil
	}
	if interceptRate < 0.01 {
		return -0.5, nil
	}

	score := interceptRate - badRate
	return score, nil
}
