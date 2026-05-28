package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// CalibratedParameter holds before/after values with calibration metadata.
type CalibratedParameter struct {
	Path             string
	Before           float64
	After            float64
	Method           string
	Confidence       float64
	Significant      bool
	SampleSize       int
	CalibrationNotes string
}

// CalibrationResult holds the outcome of calibrating one module.
type CalibrationResult struct {
	Module     string
	Parameters []CalibratedParameter
	Errors     []string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "calibrate-parameters: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("calibrate-parameters", flag.ContinueOnError)
	module := fs.String("module", "all", "Module to calibrate: garch, var, darwinian, factor, all")
	dryRun := fs.Bool("dry-run", false, "Preview changes without saving")
	dataPath := fs.String("data", "", "Path to replay data (CSV or JSONL). Defaults to ATLAS_REPLAY_DATA_PATH")
	verbose := fs.Bool("v", false, "Verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	workDir, _ := os.Getwd()
	replayPath := *dataPath
	if replayPath == "" {
		replayPath = config.GetReplayDataPath(workDir)
		if replayPath == "" {
			replayPath = filepath.Join(workDir, "data", "replay", "tw_extended_90days.csv")
		}
	}

	paramsPath := filepath.Join(workDir, "configs", "parameters.json")
	paramsCfg, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return fmt.Errorf("load parameters config: %w", err)
	}

	returns, n, err := loadReturns(replayPath)
	if err != nil {
		return fmt.Errorf("load returns: %w", err)
	}

	inference := config.NewInferenceEngine(paramsCfg)

	var results []CalibrationResult

	switch *module {
	case "garch":
		results = append(results, calibrateGARCH(inference, returns, n, paramsCfg))
	case "var":
		results = append(results, calibrateVaR(inference, returns, n, paramsCfg))
	case "darwinian":
		results = append(results, calibrateDarwinian(inference, n, paramsCfg))
	case "factor":
		results = append(results, calibrateFactor(inference, returns, n, paramsCfg))
	case "all":
		results = append(results, calibrateGARCH(inference, returns, n, paramsCfg))
		results = append(results, calibrateVaR(inference, returns, n, paramsCfg))
		results = append(results, calibrateDarwinian(inference, n, paramsCfg))
		results = append(results, calibrateFactor(inference, returns, n, paramsCfg))
	default:
		return fmt.Errorf("unknown module: %s (valid: garch, var, darwinian, factor, all)", *module)
	}

	printReport(results, *verbose)

	if *dryRun {
		fmt.Println("\n[DRY-RUN] No changes written.")
		return nil
	}

	now := time.Now()
	paramsCfg.UpdatedAt = now

	for _, res := range results {
		for _, p := range res.Parameters {
			if err := updateParameterMetadata(paramsCfg, p); err != nil {
				fmt.Fprintf(os.Stderr, "warn: update metadata for %s: %v\n", p.Path, err)
			}
		}
	}

	if err := paramsCfg.Save(paramsPath); err != nil {
		return fmt.Errorf("save parameters: %w", err)
	}

	fmt.Printf("\nSaved updated parameters to %s\n", paramsPath)
	return nil
}

func loadReturns(dataPath string) ([]float64, int, error) {
	ext := strings.ToLower(filepath.Ext(dataPath))

	var returns []float64

	if ext == ".jsonl" || ext == ".json" {
		returns, _ = loadReturnsFromJSONL(dataPath)
	}

	if len(returns) == 0 {
		var err error
		returns, _, err = loadReturnsFromCSV(dataPath)
		if err != nil {
			return nil, 0, fmt.Errorf("load from %s: %w", dataPath, err)
		}
	}

	if len(returns) < 30 {
		return nil, len(returns), fmt.Errorf("insufficient data: got %d returns, need at least 30", len(returns))
	}

	return returns, len(returns), nil
}

func loadReturnsFromJSONL(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	type Outcome struct {
		Return float64 `json:"return"`
	}

	var returns []float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var out Outcome
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			continue
		}
		if !math.IsNaN(out.Return) && !math.IsInf(out.Return, 0) {
			returns = append(returns, out.Return)
		}
	}
	return returns, nil
}

func loadReturnsFromCSV(path string) ([]float64, int, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(path)
	if err != nil {
		return nil, 0, err
	}

	var bestSym string
	var bestCount int
	if len(ds.Dates) > 0 {
		for sym := range ds.ByDate[ds.Dates[0].Format("2006-01-02")] {
			count := 0
			for _, d := range ds.Dates {
				bar, ok := ds.ByDate[d.Format("2006-01-02")][sym]
				if ok && bar.Close > 0 {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestSym = sym
			}
		}
	}

	if bestSym == "" {
		return nil, 0, fmt.Errorf("no valid bars found")
	}

	bestCount = 0
	for _, d := range ds.Dates {
		bar, ok := ds.ByDate[d.Format("2006-01-02")][bestSym]
		if ok && bar.Close > 0 {
			bestCount++
		}
	}

	var returns []float64
	var prevClose float64
	for _, date := range ds.Dates {
		bar, ok := ds.ByDate[date.Format("2006-01-02")][bestSym]
		if !ok || bar.Close == 0 {
			continue
		}
		if prevClose > 0 {
			ret := (bar.Close - prevClose) / prevClose
			if !math.IsNaN(ret) && !math.IsInf(ret, 0) {
				returns = append(returns, ret)
			}
		}
		prevClose = bar.Close
	}

	return returns, bestCount, nil
}

// ---- Module calibrators ----

func calibrateGARCH(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "garch"}

	garch, err := ie.InferGARCH(returns)
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	beforeAlpha := cfg.GARCH.Alpha.Value
	beforeBeta := cfg.GARCH.Beta.Value
	beforeOmega := cfg.GARCH.Omega.Value
	beforeSum := beforeAlpha + beforeBeta
	afterSum := garch.Alpha + garch.Beta
	significant := beforeSum > 0 && math.Abs(afterSum-beforeSum)/beforeSum > 0.05

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:        "garch.omega",
		Before:      beforeOmega,
		After:       garch.Omega,
		Method:      "MLE_grid_search",
		Confidence:  0.95,
		Significant: significant,
		SampleSize:  n,
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:        "garch.alpha",
		Before:      beforeAlpha,
		After:       garch.Alpha,
		Method:      "MLE_grid_search",
		Confidence:  0.95,
		Significant: significant,
		SampleSize:  n,
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:        "garch.beta",
		Before:      beforeBeta,
		After:       garch.Beta,
		Method:      "MLE_grid_search",
		Confidence:  0.95,
		Significant: significant,
		SampleSize:  n,
	})

	_ = ie.SetParameter("garch_omega", garch.Omega)
	_ = ie.SetParameter("garch_alpha", garch.Alpha)
	_ = ie.SetParameter("garch_beta", garch.Beta)

	return res
}

func calibrateVaR(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "var"}

	var95, err := ie.EstimateVaR(returns, 0.95)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("95%% VaR: %w", err).Error())
		return res
	}

	var99, err := ie.EstimateVaR(returns, 0.99)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("99%% VaR: %w", err).Error())
		return res
	}

	empiricalVol := math.Abs(var95.VaR) / 1.645
	empiricalMaxDD := math.Abs(var99.VaR) * 1.2

	beforeTargetVol := cfg.Sizing.TargetVolatility.Value
	beforeMaxDD := cfg.Sizing.MaxDrawdownLimit.Value

	sigTargetVol := math.Abs(empiricalVol-beforeTargetVol)/beforeTargetVol > 0.10
	sigMaxDD := math.Abs(empiricalMaxDD-beforeMaxDD)/beforeMaxDD > 0.10

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "sizing.target_volatility",
		Before:           beforeTargetVol,
		After:            math.Min(empiricalVol, 0.30),
		Method:           "VaR_inference",
		Confidence:       0.90,
		Significant:      sigTargetVol,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("VaR(95%%)=%.4f from %d observations", var95.VaR, n),
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "sizing.max_drawdown_limit",
		Before:           beforeMaxDD,
		After:            math.Min(empiricalMaxDD, 0.20),
		Method:           "VaR_inference",
		Confidence:       0.90,
		Significant:      sigMaxDD,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("VaR(99%%)=%.4f from %d observations", var99.VaR, n),
	})

	if empiricalVol > 0 && empiricalVol < 0.30 {
		_ = ie.SetParameter("sizing_target_volatility", math.Min(empiricalVol, 0.30))
	}
	if empiricalMaxDD > 0 && empiricalMaxDD < 0.20 {
		_ = ie.SetParameter("sizing_max_drawdown_limit", math.Max(0.05, math.Min(empiricalMaxDD, 0.20)))
	}

	return res
}

// loadOutcomesFromSessions reads recommendation outcomes from all session directories.
// This aggregates per-session outcome data (which is rich: per-agent, per-symbol, with
// forward returns) rather than the sparse global outcome file.
func loadOutcomesFromSessions(sessionsDir string) []domain.RecommendationOutcome {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}
	var allOutcomes []domain.RecommendationOutcome
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		outcomePath := filepath.Join(sessionsDir, entry.Name(), "recommendation_outcomes.jsonl")
		f, err := os.Open(outcomePath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var outcome domain.RecommendationOutcome
			if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
				continue
			}
			allOutcomes = append(allOutcomes, outcome)
		}
		_ = f.Close()
	}
	return allOutcomes
}

func calibrateDarwinian(ie *config.InferenceEngine, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "darwinian"}

	workDir, _ := os.Getwd()
	sessionsDir := filepath.Join(workDir, "data", "state", "sessions")
	outcomes := loadOutcomesFromSessions(sessionsDir)
	if len(outcomes) < 10 {
		res.Errors = append(res.Errors, "insufficient outcome data for darwinian calibration, using defaults")
		res.Parameters = darwinianHeuristicDefaults(n, cfg)
		return res
	}

	agentStats := make(map[string]struct{ hits, total int })
	for _, o := range outcomes {
		stats := agentStats[o.AgentID]
		stats.total++
		if o.Hit {
			stats.hits++
		}
		agentStats[o.AgentID] = stats
	}

	hitRates := make([]float64, 0, len(agentStats))
	for _, stats := range agentStats {
		if stats.total > 0 {
			hitRates = append(hitRates, float64(stats.hits)/float64(stats.total))
		}
	}

	if len(hitRates) < 3 {
		res.Errors = append(res.Errors, fmt.Sprintf("only %d agents with hit-rate data, need at least 3", len(hitRates)))
		res.Parameters = darwinianHeuristicDefaults(n, cfg)
		return res
	}

	sort.Float64s(hitRates)
	highHitRate := hitRates[int(float64(len(hitRates))*0.75)]
	lowHitRate := hitRates[int(float64(len(hitRates))*0.25)]

	beforeHighHR := cfg.Darwinian.HitRateHighThreshold.Value
	beforeLowHR := cfg.Darwinian.HitRateLowThreshold.Value

	sigHighHR := math.Abs(highHitRate-beforeHighHR)/beforeHighHR > 0.10
	sigLowHR := math.Abs(lowHitRate-beforeLowHR)/beforeLowHR > 0.10

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "darwinian.hit_rate_high_threshold",
		Before:           beforeHighHR,
		After:            highHitRate,
		Method:           "percentile_based",
		Confidence:       0.85,
		Significant:      sigHighHR,
		SampleSize:       len(hitRates),
		CalibrationNotes: fmt.Sprintf("75th percentile of %d agent hit-rates", len(hitRates)),
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "darwinian.hit_rate_low_threshold",
		Before:           beforeLowHR,
		After:            lowHitRate,
		Method:           "percentile_based",
		Confidence:       0.85,
		Significant:      sigLowHR,
		SampleSize:       len(hitRates),
		CalibrationNotes: fmt.Sprintf("25th percentile of %d agent hit-rates", len(hitRates)),
	})

	if sigHighHR {
		_ = ie.SetParameter("darwinian_hit_rate_high_threshold", highHitRate)
	}
	if sigLowHR {
		_ = ie.SetParameter("darwinian_hit_rate_low_threshold", lowHitRate)
	}

	// Extended calibration: compute agent-level Sharpe and volatility from forward returns
	agentReturns := make(map[string][]float64)
	for _, o := range outcomes {
		agentReturns[o.AgentID] = append(agentReturns[o.AgentID], o.ForwardReturn)
	}

	agentSharpes := make([]float64, 0, len(agentReturns))
	agentVols := make([]float64, 0, len(agentReturns))
	for _, returns := range agentReturns {
		if len(returns) < int(cfg.Darwinian.SharpeMinSampleSize.Value) {
			continue
		}
		mean := 0.0
		for _, r := range returns {
			mean += r
		}
		mean /= float64(len(returns))
		if mean <= 0 {
			continue
		}
		variance := 0.0
		for _, r := range returns {
			diff := r - mean
			variance += diff * diff
		}
		stdDev := math.Sqrt(variance / float64(len(returns)-1))
		if stdDev > 0 {
			sharpe := (mean / stdDev) * math.Sqrt(252)
			agentSharpes = append(agentSharpes, sharpe)
			agentVols = append(agentVols, stdDev*math.Sqrt(252))
		}
	}

	if len(agentSharpes) >= 3 {
		sort.Float64s(agentSharpes)
		sort.Float64s(agentVols)

		topSharpe := agentSharpes[int(float64(len(agentSharpes))*0.67)]
		midSharpe := agentSharpes[int(float64(len(agentSharpes))*0.5)]

		// Tier multipliers: how much better are top agents vs median?
		if midSharpe > 0 {
			observedTopBoost := topSharpe / midSharpe
			topMult := math.Max(1.02, math.Min(1.15, observedTopBoost))
			beforeTop := cfg.Darwinian.TopQuartileMultiplier.Value
			if math.Abs(topMult-beforeTop) > 0.01 {
				_ = ie.SetParameter("darwinian_top_quartile_multiplier", topMult)
				res.Parameters = append(res.Parameters, CalibratedParameter{
					Path:       "darwinian.top_quartile_multiplier",
					Before:     beforeTop,
					After:      topMult,
					Method:     "sharpe_ratio_based",
					Confidence: 0.80,
					SampleSize: len(agentSharpes),
				})
			}
		}

		// Volatility penalty threshold: median annualized volatility
		medianVol := agentVols[len(agentVols)/2]
		volThreshold := math.Max(0.05, math.Min(0.30, medianVol))
		beforeVol := cfg.Darwinian.VolatilityPenaltyThreshold.Value
		if math.Abs(volThreshold-beforeVol) > 0.01 {
			_ = ie.SetParameter("darwinian_volatility_penalty_threshold", volThreshold)
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path:       "darwinian.volatility_penalty_threshold",
				Before:     beforeVol,
				After:      volThreshold,
				Method:     "median_volatility",
				Confidence: 0.75,
				SampleSize: len(agentVols),
			})
		}

		// LookbackDays: validate against return autocorrelation
		// If returns have low autocorrelation, shorter lookback is more responsive
		avgLag1Corr := 0.0
		count := 0
		for _, returns := range agentReturns {
			if len(returns) >= 20 {
				mean := 0.0
				for _, r := range returns {
					mean += r
				}
				mean /= float64(len(returns))
				cov := 0.0
				var1, var2 := 0.0, 0.0
				for i := 0; i < len(returns)-1; i++ {
					d1 := returns[i] - mean
					d2 := returns[i+1] - mean
					cov += d1 * d2
					var1 += d1 * d1
					var2 += d2 * d2
				}
				if var1 > 0 && var2 > 0 {
					avgLag1Corr += cov / math.Sqrt(var1*var2)
					count++
				}
			}
		}
		if count > 0 {
			avgLag1Corr /= float64(count)
			// Low autocorrelation → shorter lookback is OK
			optimalLookback := 20
			if avgLag1Corr < 0.1 {
				optimalLookback = 15
			} else if avgLag1Corr > 0.3 {
				optimalLookback = 30
			}
			beforeLookback := float64(cfg.Darwinian.LookbackDays.Value)
			if float64(optimalLookback) != beforeLookback {
				res.Parameters = append(res.Parameters, CalibratedParameter{
					Path:       "darwinian.lookback_days",
					Before:     beforeLookback,
					After:      float64(optimalLookback),
					Method:     "autocorrelation_based",
					Confidence: 0.70,
					SampleSize: count,
				})
			}
		}

		// Bottom quartile multiplier: how much worse are bottom vs median?
		botSharpe := agentSharpes[int(float64(len(agentSharpes))*0.25)]
		if midSharpe > 0 {
			observedBotRatio := botSharpe / midSharpe
			botMult := math.Max(0.85, math.Min(0.98, observedBotRatio))
			beforeBot := cfg.Darwinian.BottomQuartileMultiplier.Value
			if math.Abs(botMult-beforeBot) > 0.01 {
				_ = ie.SetParameter("darwinian_bottom_quartile_multiplier", botMult)
				res.Parameters = append(res.Parameters, CalibratedParameter{
					Path: "darwinian.bottom_quartile_multiplier", Before: beforeBot, After: botMult,
					Method: "sharpe_ratio_based", Confidence: 0.80, SampleSize: len(agentSharpes),
				})
			}
		}

		// EMA alpha: from return persistence (higher autocorrelation → lower alpha)
		optimalAlpha := 0.3
		if avgLag1Corr > 0.2 {
			optimalAlpha = 0.2
		} else if avgLag1Corr < 0.05 {
			optimalAlpha = 0.4
		}
		beforeAlpha := cfg.Darwinian.EMAAlpha.Value
		if math.Abs(optimalAlpha-beforeAlpha) > 0.01 {
			_ = ie.SetParameter("darwinian_ema_alpha", optimalAlpha)
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path: "darwinian.ema_alpha", Before: beforeAlpha, After: optimalAlpha,
				Method: "autocorrelation_based", Confidence: 0.65, SampleSize: count,
			})
		}

		// Volatility penalty multiplier: high-vol agents vs low-vol agents performance
		if len(agentVols) >= 4 {
			highVolIdx := int(float64(len(agentVols)) * 0.75)
			highVolAgents := make(map[string]bool)
			for aid, returns := range agentReturns {
				if len(returns) >= 5 {
					m := 0.0
					for _, r := range returns {
						m += r
					}
					m /= float64(len(returns))
					v := 0.0
					for _, r := range returns {
						d := r - m
						v += d * d
					}
					av := math.Sqrt(v/float64(len(returns)-1)) * math.Sqrt(252)
					if av > agentVols[highVolIdx] {
						highVolAgents[aid] = true
					}
				}
			}
			if len(highVolAgents) > 0 {
				highVolSharpeSum := 0.0
				highVolCount := 0
				for aid := range highVolAgents {
					returns := agentReturns[aid]
					if len(returns) >= 5 {
						m := 0.0
						for _, r := range returns {
							m += r
						}
						m /= float64(len(returns))
						v := 0.0
						for _, r := range returns {
							d := r - m
							v += d * d
						}
						sd := math.Sqrt(v / float64(len(returns)-1))
						if sd > 0 {
							highVolSharpeSum += (m / sd) * math.Sqrt(252)
							highVolCount++
						}
					}
				}
				if highVolCount > 0 {
					avgHighVolSharpe := highVolSharpeSum / float64(highVolCount)
					avgAllSharpe := 0.0
					for _, s := range agentSharpes {
						avgAllSharpe += s
					}
					avgAllSharpe /= float64(len(agentSharpes))
					if avgAllSharpe > 0 {
						volPenalty := math.Max(0.7, math.Min(0.95, avgHighVolSharpe/avgAllSharpe))
						beforeVolPen := cfg.Darwinian.VolatilityPenaltyMultiplier.Value
						if math.Abs(volPenalty-beforeVolPen) > 0.02 {
							_ = ie.SetParameter("darwinian_volatility_penalty_multiplier", volPenalty)
							res.Parameters = append(res.Parameters, CalibratedParameter{
								Path: "darwinian.volatility_penalty_multiplier", Before: beforeVolPen, After: volPenalty,
								Method: "volatility_weighted_sharpe", Confidence: 0.70, SampleSize: highVolCount,
							})
						}
					}
				}
			}
		}

		// Risk volatility threshold: top quartile of agent volatility (daily, not annualized)
		// RollingVolatility in the code is daily std dev, so calibrate daily value
		dailyRiskVol := agentVols[int(float64(len(agentVols))*0.75)] / math.Sqrt(252)
		beforeRiskVol := cfg.Darwinian.RiskVolatilityThreshold.Value
		if math.Abs(dailyRiskVol-beforeRiskVol) > 0.001 {
			_ = ie.SetParameter("darwinian_risk_volatility_threshold", dailyRiskVol)
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path: "darwinian.risk_volatility_threshold", Before: beforeRiskVol, After: dailyRiskVol,
				Method: "percentile_based", Confidence: 0.75, SampleSize: len(agentVols),
			})
		}

		// Max performance bonus: from max observed Sharpe (cap at 30%)
		maxSharpe := agentSharpes[len(agentSharpes)-1]
		maxBonus := math.Min(0.30, maxSharpe*0.02)
		beforeMaxBonus := cfg.Darwinian.MaxPerformanceBonusPct.Value
		if math.Abs(maxBonus-beforeMaxBonus) > 0.01 {
			_ = ie.SetParameter("darwinian_max_performance_bonus_pct", maxBonus)
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path: "darwinian.max_performance_bonus_pct", Before: beforeMaxBonus, After: maxBonus,
				Method: "sharpe_capped", Confidence: 0.65, SampleSize: len(agentSharpes),
			})
		}

		// Middle tier multipliers: from agent hit-rate distribution
		if len(hitRates) >= 6 {
			midHighHR := hitRates[int(float64(len(hitRates))*0.6)]
			midLowHR := hitRates[int(float64(len(hitRates))*0.4)]
			midBoost := 1.0 + (midHighHR-highHitRate)*0.1
			midCut := 1.0 - (lowHitRate-midLowHR)*0.1
			midBoost = math.Max(1.005, math.Min(1.03, midBoost))
			midCut = math.Max(0.97, math.Min(0.995, midCut))

			beforeMidBoost := cfg.Darwinian.MiddleTierBoostMultiplier.Value
			if math.Abs(midBoost-beforeMidBoost) > 0.002 {
				_ = ie.SetParameter("darwinian_middle_tier_boost_multiplier", midBoost)
				res.Parameters = append(res.Parameters, CalibratedParameter{
					Path: "darwinian.middle_tier_boost_multiplier", Before: beforeMidBoost, After: midBoost,
					Method: "hit_rate_based", Confidence: 0.60, SampleSize: len(hitRates),
				})
			}
			beforeMidCut := cfg.Darwinian.MiddleTierCutMultiplier.Value
			if math.Abs(midCut-beforeMidCut) > 0.002 {
				_ = ie.SetParameter("darwinian_middle_tier_cut_multiplier", midCut)
				res.Parameters = append(res.Parameters, CalibratedParameter{
					Path: "darwinian.middle_tier_cut_multiplier", Before: beforeMidCut, After: midCut,
					Method: "hit_rate_based", Confidence: 0.60, SampleSize: len(hitRates),
				})
			}
		}
	}

	return res
}

func calibrateFactor(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "factor"}

	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	p90 := sorted[int(float64(len(sorted))*0.90)]

	autocorr := computeAutocorrelation(returns, 5)
	var momentumLookback int
	if autocorr > 0.5 {
		momentumLookback = 20
	} else if autocorr > 0.2 {
		momentumLookback = 10
	} else {
		momentumLookback = 5
	}

	beforeStddevDiv := cfg.Factor.MomentumStdDevDivisor.Value
	beforeLookback := cfg.Factor.MomentumLookbackDays.Value

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "factor.momentum_stddev_divisor",
		Before:           beforeStddevDiv,
		After:            math.Max(p90*0.5, 0.05),
		Method:           "return_distribution",
		Confidence:       0.80,
		Significant:      true,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("P90=%.4f, autocorrelation=%.3f", p90, autocorr),
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "factor.momentum_lookback_days",
		Before:           float64(beforeLookback),
		After:            float64(momentumLookback),
		Method:           "autocorrelation",
		Confidence:       0.75,
		Significant:      beforeLookback != momentumLookback,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("autocorrelation=%.3f suggests %d-day window", autocorr, momentumLookback),
	})

	if p90 > 0.05 {
		_ = ie.SetParameter("factor_momentum_stddev_divisor", math.Max(p90*0.5, 0.05))
	}
	_ = ie.SetParameter("factor_momentum_lookback_days", float64(momentumLookback))

	return res
}

// ---- Helpers ----

func computeAutocorrelation(returns []float64, lag int) float64 {
	if len(returns) < lag+1 {
		return 0
	}
	var mean, sumCorr, sumSq float64
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	for i := 0; i < len(returns)-lag; i++ {
		sumCorr += (returns[i] - mean) * (returns[i+lag] - mean)
	}
	for i := 0; i < len(returns); i++ {
		dx := returns[i] - mean
		sumSq += dx * dx
	}
	if sumSq == 0 {
		return 0
	}
	return sumCorr / sumSq
}

func darwinianHeuristicDefaults(n int, cfg *config.ParametersConfig) []CalibratedParameter {
	confidence := 0.6
	return []CalibratedParameter{
		{
			Path:        "darwinian.hit_rate_high_threshold",
			Before:      cfg.Darwinian.HitRateHighThreshold.Value,
			After:       cfg.Darwinian.HitRateHighThreshold.Value,
			Method:      "heuristic_fallback",
			Confidence:  confidence,
			Significant: false,
			SampleSize:  n,
		},
		{
			Path:        "darwinian.hit_rate_low_threshold",
			Before:      cfg.Darwinian.HitRateLowThreshold.Value,
			After:       cfg.Darwinian.HitRateLowThreshold.Value,
			Method:      "heuristic_fallback",
			Confidence:  confidence,
			Significant: false,
			SampleSize:  n,
		},
	}
}

func updateParameterMetadata(cfg *config.ParametersConfig, p CalibratedParameter) error {
	pathParts := strings.Split(p.Path, ".")
	if len(pathParts) != 2 {
		return fmt.Errorf("invalid path format: %s", p.Path)
	}
	section, key := pathParts[0], pathParts[1]
	now := time.Now()

	switch section {
	case "garch":
		switch key {
		case "omega":
			cfg.GARCH.Omega.Source = config.SourceCalibrated
			cfg.GARCH.Omega.CalibrationMethod = p.Method
			cfg.GARCH.Omega.LastCalibrated = &now
		case "alpha":
			cfg.GARCH.Alpha.Source = config.SourceCalibrated
			cfg.GARCH.Alpha.CalibrationMethod = p.Method
			cfg.GARCH.Alpha.LastCalibrated = &now
		case "beta":
			cfg.GARCH.Beta.Source = config.SourceCalibrated
			cfg.GARCH.Beta.CalibrationMethod = p.Method
			cfg.GARCH.Beta.LastCalibrated = &now
		}
	case "sizing":
		switch key {
		case "target_volatility":
			cfg.Sizing.TargetVolatility.Source = config.SourceCalibrated
			cfg.Sizing.TargetVolatility.CalibrationMethod = p.Method
			cfg.Sizing.TargetVolatility.LastCalibrated = &now
		case "max_drawdown_limit":
			cfg.Sizing.MaxDrawdownLimit.Source = config.SourceCalibrated
			cfg.Sizing.MaxDrawdownLimit.CalibrationMethod = p.Method
			cfg.Sizing.MaxDrawdownLimit.LastCalibrated = &now
		}
	case "darwinian":
		switch key {
		case "hit_rate_high_threshold":
			cfg.Darwinian.HitRateHighThreshold.Source = config.SourceCalibrated
			cfg.Darwinian.HitRateHighThreshold.CalibrationMethod = p.Method
			cfg.Darwinian.HitRateHighThreshold.LastCalibrated = &now
		case "hit_rate_low_threshold":
			cfg.Darwinian.HitRateLowThreshold.Source = config.SourceCalibrated
			cfg.Darwinian.HitRateLowThreshold.CalibrationMethod = p.Method
			cfg.Darwinian.HitRateLowThreshold.LastCalibrated = &now
		}
	case "factor":
		switch key {
		case "momentum_stddev_divisor":
			cfg.Factor.MomentumStdDevDivisor.Source = config.SourceCalibrated
			cfg.Factor.MomentumStdDevDivisor.CalibrationMethod = p.Method
			cfg.Factor.MomentumStdDevDivisor.LastCalibrated = &now
		case "momentum_lookback_days":
			cfg.Factor.MomentumLookbackDays.Source = config.SourceCalibrated
			cfg.Factor.MomentumLookbackDays.CalibrationMethod = p.Method
			cfg.Factor.MomentumLookbackDays.LastCalibrated = &now
		}
	}
	return nil
}

// ---- Reporting ----

func printReport(results []CalibrationResult, verbose bool) {
	fmt.Println("=== Parameter Calibration Report ===")
	fmt.Printf("Generated: %s\n\n", time.Now().Format(time.RFC3339))

	totalParams := 0
	for _, res := range results {
		fmt.Printf("Module: %s\n", res.Module)
		fmt.Println(strings.Repeat("-", 60))

		if len(res.Errors) > 0 {
			fmt.Printf("  [ERRORS]\n")
			for _, e := range res.Errors {
				fmt.Printf("    - %s\n", e)
			}
			fmt.Println()
			continue
		}

		for _, p := range res.Parameters {
			totalParams++
			sigMark := ""
			if p.Significant {
				sigMark = " *"
			}
			fmt.Printf("  %-35s  before=%-10.6f  after=%-10.6f%s\n", p.Path, p.Before, p.After, sigMark)
			fmt.Printf("    method=%-20s confidence=%-5.2f  n=%d\n", p.Method, p.Confidence, p.SampleSize)
			if p.CalibrationNotes != "" {
				fmt.Printf("    notes: %s\n", p.CalibrationNotes)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total parameters calibrated: %d\n", totalParams)
	if verbose {
		fmt.Println("\n[*] indicates statistically significant change (>10% relative)")
	}
}
