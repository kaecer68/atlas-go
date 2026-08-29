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

		favPeriod := periodReturn(favoredReturns)
		avdPeriod := periodReturn(avoidedReturns)
		totalObservations++

		if favPeriod > avdPeriod {
			correctPredictions++
		}
		patternReturns = append(patternReturns, favPeriod)
		baselineReturns = append(baselineReturns, avdPeriod)
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

// periodReturn 計算一段期間的累積複利報酬（geometric compound return）。
// 輸入為該期間內逐筆的日報酬小數（例如 0.001 = 0.1%），輸出為整段期間的累積漲跌幅。
// 例如 30 個交易日每天 +0.1% → (1.001^30) - 1 ≈ 0.0304（≈3.04%）。
// 這與 SeasonalPattern.AvgMarketReturn 欄位的語義一致：「該季節典型累積報酬」。
//
// NaN / ±Inf 輸入回傳 0：JSON 無法表示 NaN，讓它流入 parameters.json 會污染
// 下次讀取，因此寧可放棄該期資料。
func periodReturn(dailyReturns []float64) float64 {
	if len(dailyReturns) == 0 {
		return 0
	}
	compounded := 1.0
	for _, r := range dailyReturns {
		if math.IsNaN(r) || math.IsInf(r, 0) {
			return 0
		}
		compounded *= (1.0 + r)
	}
	return compounded - 1.0
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
// Compatible with both the raw JSON format (calibration_timestamp) written
// by cmd/calibrate-seasonal and the Go struct format (last_calibrated) written
// by ParametersConfig.Save().
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

	// Check for timestamp in either format:
	//   - Go struct: last_calibrated (from ParameterMetadata.LastCalibrated)
	//   - Raw JSON:  calibration_timestamp (from cmd/calibrate-seasonal --update)
	ts, hasTs := sp["last_calibrated"]
	if !hasTs || ts == nil {
		if v, ok := sp["calibration_timestamp"]; ok && v != nil && v != "" {
			ts = v
			hasTs = true
		}
	}
	if !hasTs || ts == nil || ts == "" {
		return nil
	}

	// Determine data source:
	//   - calibration_data_source (preferred, from cmd/calibrate-seasonal --update)
	//   - citation.source_reference (fallback, from ParameterMetadata.Citation)
	var src any
	if s, ok := sp["calibration_data_source"]; ok && s != nil {
		src = s
	} else if cite, ok := sp["citation"].(map[string]any); ok {
		if sr, ok := cite["source_reference"]; ok && sr != nil {
			src = sr
		}
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

// ValidationResult captures the A/B holdout test outcome for a single pattern.
type ValidationResult struct {
	PatternID       string  `json:"pattern_id"`
	TrainAccuracy   float64 `json:"train_accuracy"`
	TestAccuracy    float64 `json:"test_accuracy"`
	Degradation     float64 `json:"degradation"`
	Pass            bool    `json:"pass"`
	TrainSampleSize int     `json:"train_sample_size"`
	TestSampleSize  int     `json:"test_sample_size"`
	Margin          float64 `json:"margin"`
}

func makeYears(startYear, endYear int) []int {
	years := make([]int, 0, endYear-startYear+1)
	for y := startYear; y <= endYear; y++ {
		years = append(years, y)
	}
	return years
}

func evaluatePatternYears(p SeasonalPattern, industryReturns map[string]map[string]float64, years []int) (int, int) {
	var correct, total int
	for _, year := range years {
		startDate := time.Date(year, time.Month(p.StartMonth), p.StartDay, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(year, time.Month(p.EndMonth), p.EndDay, 0, 0, 0, 0, time.UTC)
		if p.StartMonth > p.EndMonth {
			endDate = endDate.AddDate(1, 0, 0)
		}
		favored := collectIndustryReturns(industryReturns, p.FavoredIndustries, startDate, endDate)
		avoided := collectIndustryReturns(industryReturns, p.AvoidedIndustries, startDate, endDate)
		if len(favored) == 0 || len(avoided) == 0 {
			continue
		}
		total++
		if periodReturn(favored) > periodReturn(avoided) {
			correct++
		}
	}
	return correct, total
}

// ValidateCalibration runs an out-of-sample A/B holdout test on a pattern.
// It splits [startYear, endYear] into train (first (1-testFraction)) and test
// (last testFraction) sub-ranges, counts fav>avd occurrences in each, and
// reports degradation. The pattern passes when TestAccuracy >= TrainAccuracy
// - margin. Defaults: testFraction=0.2, margin=0.05.
func ValidateCalibration(p SeasonalPattern, industryReturns map[string]map[string]float64, startYear, endYear int, testFraction, margin float64) ValidationResult {
	if testFraction <= 0 || testFraction >= 1 {
		testFraction = 0.2
	}
	if margin < 0 {
		margin = 0.05
	}
	years := makeYears(startYear, endYear)
	if len(years) < 2 {
		return ValidationResult{PatternID: p.ID, Margin: margin}
	}
	splitIdx := max(int(float64(len(years))*(1.0-testFraction)), 1)
	if splitIdx >= len(years) {
		splitIdx = len(years) - 1
	}
	trainCorrect, trainTotal := evaluatePatternYears(p, industryReturns, years[:splitIdx])
	testCorrect, testTotal := evaluatePatternYears(p, industryReturns, years[splitIdx:])
	result := ValidationResult{
		PatternID:       p.ID,
		TrainSampleSize: trainTotal,
		TestSampleSize:  testTotal,
		Margin:          margin,
	}
	if trainTotal > 0 {
		result.TrainAccuracy = float64(trainCorrect) / float64(trainTotal)
	}
	if testTotal > 0 {
		result.TestAccuracy = float64(testCorrect) / float64(testTotal)
	}
	result.Degradation = result.TrainAccuracy - result.TestAccuracy
	result.Pass = result.TestAccuracy >= result.TrainAccuracy-margin
	return result
}

func ValidateAllPatterns(engine *SeasonalEngine, industryReturns map[string]map[string]float64, startYear, endYear int, testFraction, margin float64) []ValidationResult {
	out := make([]ValidationResult, 0)
	for _, p := range engine.GetAllPatterns() {
		out = append(out, ValidateCalibration(p, industryReturns, startYear, endYear, testFraction, margin))
	}
	return out
}
