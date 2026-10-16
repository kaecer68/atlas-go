package calibration

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalibrateModule dispatches to the requested calibrator(s).
//
// It is the store-less entry point: the darwinian calibrator then reads the flat
// JSONL mirror under <cwd>/data/state (see CalibrateDarwinianWithStore).
func CalibrateModule(module string, ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) ([]CalibrationResult, error) {
	return CalibrateModuleWithStore(module, ie, returns, n, cfg, nil)
}

// CalibrateModuleWithStore is CalibrateModule with an injected ledger outcome
// store.
//
// Only the darwinian calibrator reads the ledger (garch/var/factor are computed
// from the replay returns alone), so a nil store — or any module other than
// darwinian/all — behaves exactly as before this seam was introduced.
func CalibrateModuleWithStore(module string, ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig, store ledger.OutcomeStore) ([]CalibrationResult, error) {
	switch module {
	case "garch":
		return []CalibrationResult{CalibrateGARCH(ie, returns, n, cfg)}, nil
	case "var":
		return []CalibrationResult{CalibrateVaR(ie, returns, n, cfg)}, nil
	case "darwinian":
		return []CalibrationResult{CalibrateDarwinianWithStore(ie, n, cfg, store)}, nil
	case "factor":
		return []CalibrationResult{CalibrateFactor(ie, returns, n, cfg)}, nil
	case "all":
		return []CalibrationResult{
			CalibrateGARCH(ie, returns, n, cfg),
			CalibrateVaR(ie, returns, n, cfg),
			CalibrateDarwinianWithStore(ie, n, cfg, store),
			CalibrateFactor(ie, returns, n, cfg),
		}, nil
	default:
		return nil, fmt.Errorf("unknown module: %s (valid: garch, var, darwinian, factor, all)", module)
	}
}

func CalibrateGARCH(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
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

func CalibrateVaR(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
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

// Source labels reported by darwinian_ledger_source. They are stable strings so
// an operator can tell from the log alone which backend actually served the
// calibration input.
const (
	darwinianSourceInjectedStore = "injected-store"
	darwinianSourceJSONLMirror   = "explicit-jsonl-mirror"
)

// darwinianMirrorRelDir is the flat JSONL ledger directory the darwinian
// calibrator has always read, relative to the process working directory.
var darwinianMirrorRelDir = filepath.Join("data", "state")

// darwinianOutcomes resolves the outcome data the darwinian calibration runs on
// and logs which source actually served it.
//
// Order: the injected store first; the flat JSONL mirror only as a fallback when
// the store read ERRORED. The mirror is the file the session_summary_reconcile
// job backfills from PostgreSQL (every 12h), so it is a possibly-lagging copy —
// which is why an injected store always wins when one is wired.
//
// Read failures keep the pre-seam tolerance: no outcomes at all, and the caller
// then reports "insufficient outcome data" and uses the heuristic defaults.
func darwinianOutcomes(workDir string, store ledger.OutcomeStore) []domain.RecommendationOutcome {
	source := darwinianSourceInjectedStore
	defer func() {
		logging.Info("calibration", "darwinian_ledger_source",
			logging.FStr("source", source),
			logging.FStr("store", fmt.Sprintf("%T", store)))
	}()

	if store != nil {
		outcomes, err := store.LoadOutcomesFromSessions()
		if err == nil {
			return outcomes
		}
		logging.Warn("calibration", "darwinian_store_read_failed",
			logging.Err(err),
			logging.FStr("fallback", darwinianSourceJSONLMirror))
	}
	source = darwinianSourceJSONLMirror

	// Explicit JSONL backend, deliberately not the deploy-time backend: the
	// darwinian calibrator reads the PER-SESSION files under
	// <workDir>/data/state/sessions/*, which is what the mirror holds. Naming the
	// backend keeps a postgres deployment from resolving some other directory
	// (same convention as internal/experiment/auto.go).
	mirror, err := ledger.NewOutcomeStore(config.Config{
		StoreBackend: "jsonl",
		LedgerDir:    filepath.Join(workDir, darwinianMirrorRelDir),
	})
	if err != nil {
		return nil
	}
	outcomes, err := mirror.LoadOutcomesFromSessions()
	if err != nil {
		return nil
	}
	return outcomes
}

// CalibrateDarwinian calibrates the darwinian parameters from the ledger
// outcomes found in the flat JSONL mirror under <cwd>/data/state.
//
// Callers that already hold a backend-aware ledger store should use
// CalibrateDarwinianWithStore so the calibration reads the source of truth
// instead of the mirror.
func CalibrateDarwinian(ie *config.InferenceEngine, n int, cfg *config.ParametersConfig) CalibrationResult {
	return CalibrateDarwinianWithStore(ie, n, cfg, nil)
}

// CalibrateDarwinianWithStore is CalibrateDarwinian with an injected ledger
// outcome store.
//
// A non-nil store is the source of truth (in production: PostgreSQL). The flat
// JSONL mirror is only read when no store was injected or when the store itself
// failed — a store that legitimately returns zero outcomes is NOT an error and
// must never be second-guessed here.
func CalibrateDarwinianWithStore(ie *config.InferenceEngine, n int, cfg *config.ParametersConfig, store ledger.OutcomeStore) CalibrationResult {
	res := CalibrationResult{Module: "darwinian"}

	workDir, _ := os.Getwd()
	outcomes := darwinianOutcomes(workDir, store)
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

	agentReturns := make(map[string][]float64)
	for _, o := range outcomes {
		agentReturns[o.AgentID] = append(agentReturns[o.AgentID], o.ForwardReturn)
	}

	agentSharpes := make([]float64, 0, len(agentReturns))
	agentVols := make([]float64, 0, len(agentReturns))
	for _, returns := range agentReturns {
		if len(returns) < cfg.Darwinian.SharpeMinSampleSize.Value {
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

		// Sample adequacy (FU-20260930-09). The enclosing floor for this whole
		// section is len(agentSharpes) >= 3, and the upper quartile below is
		// taken at nearest-rank index int(0.75*n) of the sorted vols — which IS
		// the largest sample for n <= 4. An estimate that equals the sample
		// maximum is not a percentile: a single agent's volatility alone sets
		// the threshold, and installing it means no agent at or below the
		// sample maximum is above it. Measured on a four-agent fixture, moving
		// ONE agent's daily volatility 0.008 -> 0.050 moved the written
		// threshold 0.0082 -> 0.0513 (and the row stayed inside the band below,
		// so the band does not cover this case).
		//
		// The floor is derived from the estimator (the estimated rank must have
		// at least one observation above it), not picked as a policy: any
		// stricter statistical adequacy floor is a threshold-policy decision for
		// the owner and is deliberately NOT applied here.
		riskVolRank := int(float64(len(agentVols)) * 0.75)
		dailyRiskVol := agentVols[riskVolRank] / math.Sqrt(252)
		beforeRiskVol := cfg.Darwinian.RiskVolatilityThreshold.Value
		sampleSupportsRank := riskVolRank < len(agentVols)-1

		// Sane band for a DAILY volatility threshold (issue #2169).
		//
		// The consumer compares this against DarwinianAgentWeight.RollingVolatility,
		// which is a daily standard deviation (portfolio/darwinian_weights.go), and
		// the shipped default is 0.08 with the rationale "8% daily volatility is
		// extreme" (config/defaults_portfolio.go). Every sibling value in this
		// function is clamped (top/bottom quartile multipliers, volatility penalty
		// multiplier, performance bonus); this one was not, so a scale error in the
		// input series or a degenerate sample could write an arbitrary number —
		// measured on 2026-06-13 the same method wrote an annualized-scale 1.355
		// (18.6x the daily-scale value) and on 2026-09-30 it wrote 0.0729.
		//
		// The band is derived from the documented intent, not from the observed
		// outlier: 0.25/day is >3x the documented "extreme" 0.08/day, and it is far
		// below the annualized scale (annualized = daily*sqrt(252)), so an
		// annualized or otherwise mis-scaled input is rejected. The lower bound
		// (0.05%/day) rejects a degenerate near-flat series.
		const (
			minDailyRiskVolatility = 0.0005
			maxDailyRiskVolatility = 0.25
		)
		outOfBand := dailyRiskVol < minDailyRiskVolatility || dailyRiskVol > maxDailyRiskVolatility
		writeRiskVol := sampleSupportsRank && !outOfBand && math.Abs(dailyRiskVol-beforeRiskVol) > 0.001

		if !sampleSupportsRank {
			// Do NOT write. Same contract as the band skip below: the overlay
			// persists only CHANGED leaves, so Before == After leaves the SSOT
			// and the overlay untouched while the reason stays visible in the
			// calibration report and in the log. Writing the value instead would
			// publish the most volatile agent's own volatility as the risk
			// threshold.
			logging.Warn("calibration", "darwinian_risk_volatility_insufficient_sample",
				logging.FInt("agents", len(agentVols)),
				logging.FInt("percentile_rank", riskVolRank),
				logging.FStr("computed_daily_volatility", fmt.Sprintf("%.6f", dailyRiskVol)),
				logging.FStr("action", "value NOT written; the upper-quartile rank is the sample maximum"))
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path: "darwinian.risk_volatility_threshold", Before: beforeRiskVol, After: beforeRiskVol,
				Method: "skipped_insufficient_sample", Confidence: 0, SampleSize: len(agentVols),
				CalibrationNotes: fmt.Sprintf("upper-quartile rank %d of %d agents is the sample maximum (computed daily volatility %.6f); a percentile needs at least one observation above its rank; not written",
					riskVolRank, len(agentVols), dailyRiskVol),
			})
		} else if outOfBand {
			// Do NOT write. Nothing is lost by recording the value here: the
			// overlay persists only CHANGED leaves, so Before == After keeps the
			// SSOT and the overlay untouched while the reason stays visible in the
			// calibration report and in the log.
			logging.Warn("calibration", "darwinian_risk_volatility_out_of_band",
				logging.FStr("computed_daily_volatility", fmt.Sprintf("%.6f", dailyRiskVol)),
				logging.FStr("band", fmt.Sprintf("[%.4f, %.2f] per day", minDailyRiskVolatility, maxDailyRiskVolatility)),
				logging.FInt("agents", len(agentVols)),
				logging.FStr("action", "value NOT written; check input series scale (annualized vs daily) or sample size"))
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path: "darwinian.risk_volatility_threshold", Before: beforeRiskVol, After: beforeRiskVol,
				Method: "skipped_out_of_band", Confidence: 0, SampleSize: len(agentVols),
				CalibrationNotes: fmt.Sprintf("computed daily volatility %.6f outside sane band [%.4f, %.2f]; not written",
					dailyRiskVol, minDailyRiskVolatility, maxDailyRiskVolatility),
			})
		} else if writeRiskVol {
			_ = ie.SetParameter("darwinian_risk_volatility_threshold", dailyRiskVol)
			res.Parameters = append(res.Parameters, CalibratedParameter{
				Path: "darwinian.risk_volatility_threshold", Before: beforeRiskVol, After: dailyRiskVol,
				Method: "percentile_based", Confidence: 0.75, SampleSize: len(agentVols),
			})
		}

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

func CalibrateFactor(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "factor"}

	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	p90 := 0.0
	if len(sorted) > 0 {
		p90 = sorted[int(float64(len(sorted))*0.90)]
	}

	autocorr := ComputeAutocorrelation(returns, 5)
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

func ComputeAutocorrelation(returns []float64, lag int) float64 {
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
	for i := range returns {
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
