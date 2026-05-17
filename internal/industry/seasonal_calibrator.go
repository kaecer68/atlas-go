package industry

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"time"
)

// SeasonalCalibration holds the backtest-validated parameters for a single pattern.
type SeasonalCalibration struct {
	PatternID          string  `json:"pattern_id"`
	PatternName        string  `json:"pattern_name"`
	ObservedAccuracy   float64 `json:"observed_accuracy"`
	ObservedAvgReturn  float64 `json:"observed_avg_return"`
	ObservedAdjustment float64 `json:"observed_adjustment"`
	DeclaredAccuracy   float64 `json:"declared_accuracy"`
	DeclaredReturn     float64 `json:"declared_return"`
	DeclaredAdjustment float64 `json:"declared_adjustment"`
	ObservationCount   int     `json:"observation_count"`
	Verdict            string  `json:"verdict"` // "validated", "overstated", "understated"
}

// CalibratePatterns validates seasonal patterns against historical industry returns.
// industryReturns maps industryID → map of date string ("2006-01-02") → daily return.
// CalibratePatterns returns calibration results, total observation count, and any error.
func CalibratePatterns(engine *SeasonalEngine, industryReturns map[string]map[string]float64, startYear, endYear int) ([]SeasonalCalibration, error) {
	var results []SeasonalCalibration

	for _, pattern := range engine.GetAllPatterns() {
		cal := calibrateSinglePattern(pattern, industryReturns, startYear, endYear)
		results = append(results, cal)
	}
	return results, nil
}

func calibrateSinglePattern(p SeasonalPattern, industryReturns map[string]map[string]float64, startYear, endYear int) SeasonalCalibration {
	cal := SeasonalCalibration{
		PatternID:          p.ID,
		PatternName:        p.Name,
		DeclaredAccuracy:   p.HistoricalAccuracy,
		DeclaredReturn:     p.AvgMarketReturn,
		DeclaredAdjustment: p.AdjustmentFactor,
	}

	var patternReturns []float64
	var baselineReturns []float64
	var correctPredictions int
	var totalObservations int

	for year := startYear; year <= endYear; year++ {
		startDate := time.Date(year, time.Month(p.StartMonth), p.StartDay, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(year, time.Month(p.EndMonth), p.EndDay, 0, 0, 0, 0, time.UTC)

		if p.StartMonth > p.EndMonth {
			endDate = endDate.AddDate(1, 0, 0)
		}

		favoredReturns := collectIndustryReturns(industryReturns, p.FavoredIndustries, startDate, endDate)
		avoidedReturns := collectIndustryReturns(industryReturns, p.AvoidedIndustries, startDate, endDate)

		if len(favoredReturns) == 0 || len(avoidedReturns) == 0 {
			continue
		}

		favAvg := average(favoredReturns)
		avdAvg := average(avoidedReturns)
		totalObservations++

		if favAvg > avdAvg {
			correctPredictions++
		}
		patternReturns = append(patternReturns, favAvg)
		baselineReturns = append(baselineReturns, avdAvg)
	}

	cal.ObservationCount = totalObservations

	if totalObservations > 0 {
		cal.ObservedAccuracy = float64(correctPredictions) / float64(totalObservations)
	}
	if len(patternReturns) > 0 {
		cal.ObservedAvgReturn = average(patternReturns)
	}
	if len(patternReturns) > 0 && len(baselineReturns) > 0 {
		cal.ObservedAdjustment = 1.0 + (average(patternReturns)-average(baselineReturns))/math.Max(math.Abs(average(baselineReturns)), 0.001)
	}

	accuracyGap := cal.ObservedAccuracy - cal.DeclaredAccuracy
	switch {
	case accuracyGap > 0.10:
		cal.Verdict = "understated"
	case accuracyGap < -0.10:
		cal.Verdict = "overstated"
	default:
		cal.Verdict = "validated"
	}

	return cal
}

func collectIndustryReturns(returns map[string]map[string]float64, industries []string, start, end time.Time) []float64 {
	var all []float64
	for _, ind := range industries {
		indReturns, ok := returns[ind]
		if !ok {
			continue
		}
		for dateStr, ret := range indReturns {
			d, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}
			if !d.Before(start) && !d.After(end) {
				all = append(all, ret)
			}
		}
	}
	return all
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// CalibrationReport returns a human-readable summary of calibration results.
func CalibrationReport(results []SeasonalCalibration) string {
	out := "季節性模式回測校準報告\n"
	out += "========================\n\n"

	for _, c := range results {
		icon := "✓"
		switch c.Verdict {
		case "overstated":
			icon = "⚠"
		case "understated":
			icon = "↑"
		}
		out += fmt.Sprintf("%s %s (%s)\n", icon, c.PatternName, c.PatternID)
		out += fmt.Sprintf("  觀察次數: %d\n", c.ObservationCount)
		out += fmt.Sprintf("  準確度: 宣稱 %.0f%% → 實測 %.0f%% (%s)\n",
			c.DeclaredAccuracy*100, c.ObservedAccuracy*100, c.Verdict)
		out += fmt.Sprintf("  平均回報: 宣稱 %.1f%% → 實測 %.1f%%\n",
			c.DeclaredReturn*100, c.ObservedAvgReturn*100)
		out += fmt.Sprintf("  調整因子: 宣稱 %.2f → 實測 %.2f\n",
			c.DeclaredAdjustment, c.ObservedAdjustment)
		out += "\n"
	}
	return out
}

// IndustryReturnAggregator computes aggregate industry returns from individual
// stock returns, using equal-weight averaging. A stock may belong to multiple
// sectors; its returns are distributed to all of them.
func IndustryReturnAggregator(stockReturns map[string]map[string]float64, stockIndustryMap map[string][]string) map[string]map[string]float64 {
	type accum struct {
		sum   float64
		count int
	}
	industryAccum := make(map[string]map[string]*accum)

	for symbol, dateReturns := range stockReturns {
		industryIDs := stockIndustryMap[symbol]
		if len(industryIDs) == 0 {
			continue
		}
		for _, industryID := range industryIDs {
			if industryAccum[industryID] == nil {
				industryAccum[industryID] = make(map[string]*accum)
			}
			for date, ret := range dateReturns {
				a, ok := industryAccum[industryID][date]
				if !ok {
					industryAccum[industryID][date] = &accum{sum: ret, count: 1}
				} else {
					a.sum += ret
					a.count++
				}
			}
		}
	}

	industryReturns := make(map[string]map[string]float64)
	for industryID, dateAccums := range industryAccum {
		industryReturns[industryID] = make(map[string]float64)
		for date, a := range dateAccums {
			industryReturns[industryID][date] = a.sum / float64(a.count)
		}
	}
	return industryReturns
}

// ValidateIndustryIDs checks that all pattern industry IDs exist in the returns map.
func ValidateIndustryIDs(patterns []SeasonalPattern, industryReturns map[string]map[string]float64) []string {
	seen := make(map[string]bool)
	var missing []string
	for _, p := range patterns {
		for _, id := range p.FavoredIndustries {
			if _, ok := industryReturns[id]; !ok && !seen[id] {
				seen[id] = true
				missing = append(missing, id)
			}
		}
		for _, id := range p.AvoidedIndustries {
			if _, ok := industryReturns[id]; !ok && !seen[id] && !slices.Contains(p.FavoredIndustries, id) {
				seen[id] = true
				missing = append(missing, id)
			}
		}
	}
	return missing
}

// LoadCalibrationEvidence reads calibration metadata from parameters.json.
// Returns nil if no calibration has been performed.
func LoadCalibrationEvidence(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil
	}
	industryCfg, ok := config["industry"].(map[string]any)
	if !ok {
		return nil
	}
	sp, ok := industryCfg["seasonal_patterns"].(map[string]any)
	if !ok {
		return nil
	}
	ts, hasTs := sp["calibration_timestamp"]
	src := sp["calibration_data_source"]
	if !hasTs {
		return nil
	}
	result := map[string]any{"calibrated": true}
	if ts != nil {
		result["timestamp"] = ts
	}
	if src != nil {
		result["data_source"] = src
	}
	return result
}
