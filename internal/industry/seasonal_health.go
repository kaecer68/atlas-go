package industry

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// DarwinianRange 為校準結果的合法 adjustment_factor 區間。
// 與 internal/monitoring 相關 guard 與 AGENTS.md「Darwinian 權重 [0.3, 2.5]」規範一致。
// 這是 adjustment_factor 值域的唯一權威常數：消費者一律透過
// ClampAdjustmentFactor / IsAdjustmentFactorInRange 使用，不得在別處再寫魔數。
const (
	DarwinianMinAdjustment = 0.3
	DarwinianMaxAdjustment = 2.5
)

// ClampAdjustmentFactor 把 pattern 的 adjustment_factor 夾進 Darwinian 合法區間。
//
// 為什麼需要（issue #1944 / I17）：configs/parameters.json 實測有 4 個 pattern
// 的 adjustment_factor 超出 [0.3, 2.5]（含負值），而 SeasonalEngine 的消費端
// 直接相乘。負值會讓季節調整變成負數（再被 adjustment_floor 打成 0.01），
// 3.44 則讓調整量爆走。超界值不得靜默相乘，因此所有乘法前一律先夾。
//
// 語意：
//   - NaN / -Inf / <= 0 → 1.0（中性，視為無可用訊號）。負值代表「方向被實測
//     否定」，不是單純倍率問題；若夾到下限 0.3，avoided 產業會拿到
//     1/0.3 = 3.33× 的加成，方向反而錯誤。
//   - 其餘（含 +Inf）→ 夾到 [DarwinianMinAdjustment, DarwinianMaxAdjustment]。
func ClampAdjustmentFactor(f float64) float64 {
	if math.IsNaN(f) || f <= 0 {
		return 1.0
	}
	return math.Min(math.Max(f, DarwinianMinAdjustment), DarwinianMaxAdjustment)
}

// IsAdjustmentFactorInRange 回報 adjustment_factor 是否落在 Darwinian 合法區間內。
// NaN 與 <=0 皆視為 out of range（與 ClampAdjustmentFactor 的判定一致）。
func IsAdjustmentFactorInRange(f float64) bool {
	return !math.IsNaN(f) && f >= DarwinianMinAdjustment && f <= DarwinianMaxAdjustment
}

// HealthStatus 為健康狀態的離散標籤。
type HealthStatus string

const (
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
	// HealthUnknown 表示「無法判定」。目前唯一成因是完全沒有 per-pattern
	// 校準觀測（TotalObservations == 0），因此「校準品質」本身不可知
	// （issue #1944 / I17）。這不是 critical：把「沒校準」報成 critical 會與
	// 「校準值壞了」混為一談，產生假訊號。值域問題不因此消失，仍由
	// AdjustmentFactorStatus / DarwinianViolations / OutOfRangePatterns 回報。
	HealthUnknown  HealthStatus = "unknown"
	HealthCritical HealthStatus = "critical"
)

// HealthReason 為 Health 欄位的成因說明（snake_case，供 dashboard 顯示）。
type HealthReason string

const (
	ReasonNoCalibrationData    HealthReason = "no_calibration_data"
	ReasonMalformedData        HealthReason = "malformed_calibration_data"
	ReasonNoCalibratedPatterns HealthReason = "no_calibrated_patterns"
	ReasonNoObservations       HealthReason = "no_observations"
	ReasonDarwinianViolations  HealthReason = "darwinian_violations"
	ReasonOutOfRangeValues     HealthReason = "out_of_range_values"
	ReasonInsufficientSamples  HealthReason = "insufficient_samples"
)

// CalibrationEvidence 標明 per-pattern 校準證據是否存在（機讀）。
// calibration_observations / calibration_verdict / calibration_timestamp 這三個
// per-pattern 欄位的 producer 是 `cmd/calibrate-seasonal --update`
// （`updateParametersFileAt`，需真 replay 資料才會寫；issue #1944 Batch 4 / I17）。
// 在 operator 跑過該工具之前，這三個欄位不存在 ⇒ 一律 CalibrationEvidenceNone。
const (
	CalibrationEvidenceNone    = "none"
	CalibrationEvidencePresent = "measured"
)

// SeasonalVerdict* 是 per-pattern 校準裁決的字彙表（機讀）。
// producer 是 cmd/calibrate-seasonal --update；consumer 是
// CalibrationHealthSummary.VerdictCounts（只計數，不驗值），因此這組常數是
// 兩端共用詞彙的單一權威。
const (
	// SeasonalVerdictCalibrated：有足夠觀測且值域合法，校準值已寫回 config。
	SeasonalVerdictCalibrated = "calibrated"
	// SeasonalVerdictInsufficientSamples：觀測數低於 --update-threshold，值未套用。
	SeasonalVerdictInsufficientSamples = "insufficient_samples"
	// SeasonalVerdictOutOfRange：觀測足夠但值域不合法（未通過守門），值未套用。
	SeasonalVerdictOutOfRange = "out_of_range"
	// SeasonalVerdictNoObservations：該 pattern 這次沒有任何觀測。
	SeasonalVerdictNoObservations = "no_observations"
)

// ObservationStatus 為校準觀測量的機讀狀態。
const (
	ObservationNoObservations = "no_observations"
	ObservationInsufficient   = "insufficient"
	ObservationSufficient     = "sufficient"
)

// AdjustmentFactorStatus 為 adjustment_factor 值域的機讀狀態。
// 即使 TotalObservations == 0（Health == unknown）也照實回報，避免超界事實被
// unknown 掩蓋（issue #1944 / I17）。
const (
	AdjustmentFactorInRange    = "in_range"
	AdjustmentFactorOutOfRange = "out_of_range"
)

// seasonalDataStatus 表達 parameters.json 中 seasonal_patterns 結構的健度。
type seasonalDataStatus int

const (
	seasonalDataOK seasonalDataStatus = iota
	seasonalDataMissingSection
	seasonalDataMalformedValue
)

// CalibrationHealthSummary 為 parameters.json 中季節性模式校準品質的快照。
// 設計為 dashboard 區塊的單一資料來源。
type CalibrationHealthSummary struct {
	LastCalibratedAt    *time.Time     `json:"last_calibrated_at,omitempty"`
	PatternCount        int            `json:"pattern_count"`
	TotalObservations   int            `json:"total_observations"`
	VerdictCounts       map[string]int `json:"verdict_counts"`
	DarwinianViolations int            `json:"darwinian_violations"`
	OutOfRangeCount     int            `json:"out_of_range_count"`
	Health              HealthStatus   `json:"health"`
	Reason              HealthReason   `json:"reason,omitempty"`

	// CalibrationEvidence：none | measured。none 表示沒有任何 per-pattern
	// 校準證據（目前為常態，因為沒有 producer）。
	CalibrationEvidence string `json:"calibration_evidence"`
	// ObservationStatus：no_observations | insufficient | sufficient。
	ObservationStatus string `json:"observation_status"`
	// AdjustmentFactorStatus：in_range | out_of_range。這是值域軸的機讀旗標；
	// 超界的 pattern id 列在 OutOfRangePatterns。
	AdjustmentFactorStatus string `json:"adjustment_factor_status"`
	// OutOfRangePatterns 列出 adjustment_factor 缺失或超界的 pattern id
	// （機讀，供告警指名；不得只靠 health 這個聚合標籤）。
	OutOfRangePatterns []string `json:"out_of_range_patterns,omitempty"`
}

// SummarizeCalibrationHealth 從 parameters.json 解析 seasonal_patterns 並產生健康摘要。
// 不需要先跑校準；直接讀檔即可。
//
// 健度分級（由嚴重到輕）：
//   - critical + reason="no_calibration_data"：parameters.json 缺少 industry.seasonal_patterns 區塊
//   - critical + reason="malformed_calibration_data"：value 不是 array 結構
//   - degraded + reason="no_calibrated_patterns"：value 為空陣列
//   - unknown + reason="no_observations"：完全沒有 per-pattern 校準觀測
//     （TotalObservations == 0）⇒ 校準品質不可知。**刻意不報 critical**
//     （issue #1944 / I17：不得製造假訊號）。此時值域問題仍完整回報於
//     AdjustmentFactorStatus / DarwinianViolations / OutOfRangePatterns。
//   - critical + reason="darwinian_violations"：有觀測且 pattern 違反 Darwinian 邊界
//   - critical + reason="out_of_range_values"：有觀測且 pattern 欄位（accuracy / return）超出合法區間
//   - degraded + reason="insufficient_samples"：有觀測但平均觀察樣本 < 3
//   - healthy：以上皆未觸發
func SummarizeCalibrationHealth(paramsPath string) (*CalibrationHealthSummary, error) {
	raw, err := os.ReadFile(paramsPath)
	if err != nil {
		return nil, fmt.Errorf("read parameters.json: %w", err)
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse parameters.json: %w", err)
	}

	summary := &CalibrationHealthSummary{
		VerdictCounts: make(map[string]int),
	}

	patterns, dataStatus := extractSeasonalPatterns(root)
	summary.PatternCount = len(patterns)

	switch dataStatus {
	case seasonalDataMissingSection:
		summary.Health = HealthCritical
		summary.Reason = ReasonNoCalibrationData
		return summary, nil
	case seasonalDataMalformedValue:
		summary.Health = HealthCritical
		summary.Reason = ReasonMalformedData
		return summary, nil
	}

	if summary.PatternCount == 0 {
		summary.Health = HealthDegraded
		summary.Reason = ReasonNoCalibratedPatterns
		return summary, nil
	}

	var latest *time.Time
	outOfRangePatterns := make([]string, 0)
	for i, p := range patterns {
		observations, _ := p["calibration_observations"].(float64)
		summary.TotalObservations += int(observations)

		verdict, _ := p["calibration_verdict"].(string)
		if verdict == "" {
			verdict = "unknown"
		}
		summary.VerdictCounts[verdict]++

		ts, _ := p["calibration_timestamp"].(string)
		if ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				if latest == nil || t.After(*latest) {
					latest = &t
				}
			}
		}

		// adjustment_factor 缺失與超界都算違規：缺失會讓 Go 端讀到 0，
		// 若不處理會被當成「0× 加成」靜默消滅該 pattern 的調整量。
		adj, hasAdj := p["adjustment_factor"].(float64)
		if !hasAdj || !IsAdjustmentFactorInRange(adj) {
			summary.DarwinianViolations++
			outOfRangePatterns = append(outOfRangePatterns, patternLabel(p, i))
		}

		acc, _ := p["historical_accuracy"].(float64)
		ret, _ := p["avg_market_return"].(float64)
		if acc < 0 || acc > 1 || ret < -1 || ret > 1 {
			summary.OutOfRangeCount++
		}
	}
	summary.LastCalibratedAt = latest
	summary.OutOfRangePatterns = outOfRangePatterns
	summary.AdjustmentFactorStatus = AdjustmentFactorInRange
	if len(outOfRangePatterns) > 0 {
		summary.AdjustmentFactorStatus = AdjustmentFactorOutOfRange
	}
	summary.CalibrationEvidence = CalibrationEvidenceNone
	summary.ObservationStatus = ObservationNoObservations
	if summary.TotalObservations > 0 {
		summary.CalibrationEvidence = CalibrationEvidencePresent
		summary.ObservationStatus = ObservationSufficient
		if summary.TotalObservations < summary.PatternCount*3 {
			summary.ObservationStatus = ObservationInsufficient
		}
	}
	summary.Health, summary.Reason = deriveHealth(summary)
	return summary, nil
}

// patternLabel 回傳 pattern 的機讀識別：優先 id，缺 id 時退回 index，
// 讓 OutOfRangePatterns 永遠能指名到具體 pattern。
func patternLabel(p map[string]any, index int) string {
	if id, ok := p["id"].(string); ok && id != "" {
		return id
	}
	return fmt.Sprintf("pattern[%d]", index)
}

// extractSeasonalPatterns 從 root 中取出 seasonal_patterns.value 陣列，
// 並回傳其健度狀態以區分「結構不存在」、「結構錯誤」、「結構正常」三種情境。
// 容忍陣列中個別元素不是 object（會被略過），但陣列本身必須存在且型別正確。
func extractSeasonalPatterns(root map[string]any) ([]map[string]any, seasonalDataStatus) {
	if root == nil {
		return nil, seasonalDataMissingSection
	}
	industry, ok := root["industry"].(map[string]any)
	if !ok {
		return nil, seasonalDataMissingSection
	}
	sp, ok := industry["seasonal_patterns"].(map[string]any)
	if !ok {
		return nil, seasonalDataMissingSection
	}
	rawValue, exists := sp["value"]
	if !exists {
		return nil, seasonalDataMissingSection
	}
	arr, ok := rawValue.([]any)
	if !ok {
		return nil, seasonalDataMalformedValue
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, seasonalDataOK
}

// deriveHealth 套用健康判定啟發式。已假設 PatternCount > 0（caller 應在
// 進入此函式前先處理 missing / malformed / empty 等前置情境）。
func deriveHealth(s *CalibrationHealthSummary) (HealthStatus, HealthReason) {
	// 「無觀測」優先於值域判定（issue #1944 / I17）：
	// calibration_observations / calibration_verdict / calibration_timestamp 的
	// producer 是 cmd/calibrate-seasonal --update（需真 replay 與足夠觀測）。
	// 在 operator 跑過之前，TotalObservations == 0 ⇒「校準品質」本身不可知。
	// 此時若沿用值域判定把它報成 critical，dashboard 只會看到 critical 卻無法
	// 分辨「沒校準」與「校準值壞了」——這就是假訊號。
	//
	// 值域問題不會被掩蓋：AdjustmentFactorStatus（out_of_range）、
	// DarwinianViolations 與 OutOfRangePatterns 都照實回報，消費者可依需要
	// 對值域軸單獨告警。有觀測時仍照舊以值域判定為 critical。
	if s.TotalObservations == 0 {
		return HealthUnknown, ReasonNoObservations
	}
	if s.DarwinianViolations > 0 {
		return HealthCritical, ReasonDarwinianViolations
	}
	if s.OutOfRangeCount > 0 {
		return HealthCritical, ReasonOutOfRangeValues
	}
	if s.TotalObservations < s.PatternCount*3 {
		return HealthDegraded, ReasonInsufficientSamples
	}
	return HealthHealthy, ""
}
