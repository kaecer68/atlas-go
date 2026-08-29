package industry

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// DarwinianRange 為校準結果的合法 adjustment_factor 區間。
// 與 internal/monitoring 相關 guard 與 AGENTS.md「Darwinian 權重 [0.3, 2.5]」規範一致。
const (
	DarwinianMinAdjustment = 0.3
	DarwinianMaxAdjustment = 2.5
)

// HealthStatus 為健康狀態的離散標籤。
type HealthStatus string

const (
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
	HealthCritical HealthStatus = "critical"
)

// HealthReason 為 Health 欄位的成因說明（snake_case，供 dashboard 顯示）。
type HealthReason string

const (
	ReasonNoCalibrationData    HealthReason = "no_calibration_data"
	ReasonMalformedData        HealthReason = "malformed_calibration_data"
	ReasonNoCalibratedPatterns HealthReason = "no_calibrated_patterns"
	ReasonDarwinianViolations  HealthReason = "darwinian_violations"
	ReasonOutOfRangeValues     HealthReason = "out_of_range_values"
	ReasonInsufficientSamples  HealthReason = "insufficient_samples"
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
}

// SummarizeCalibrationHealth 從 parameters.json 解析 seasonal_patterns 並產生健康摘要。
// 不需要先跑校準；直接讀檔即可。
//
// 健度分級（由嚴重到輕）：
//   - critical + reason="no_calibration_data"：parameters.json 缺少 industry.seasonal_patterns 區塊
//   - critical + reason="malformed_calibration_data"：value 不是 array 結構
//   - degraded + reason="no_calibrated_patterns"：value 為空陣列
//   - critical + reason="darwinian_violations"：有 pattern 違反 Darwinian 邊界
//   - critical + reason="out_of_range_values"：有 pattern 欄位（accuracy / return）超出合法區間
//   - degraded + reason="insufficient_samples"：平均觀察樣本 < 3
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
	for _, p := range patterns {
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

		adj, _ := p["adjustment_factor"].(float64)
		if adj < DarwinianMinAdjustment || adj > DarwinianMaxAdjustment {
			summary.DarwinianViolations++
		}

		acc, _ := p["historical_accuracy"].(float64)
		ret, _ := p["avg_market_return"].(float64)
		if acc < 0 || acc > 1 || ret < -1 || ret > 1 {
			summary.OutOfRangeCount++
		}
	}
	summary.LastCalibratedAt = latest
	summary.Health, summary.Reason = deriveHealth(summary)
	return summary, nil
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
