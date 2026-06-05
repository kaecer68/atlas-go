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
}

// SummarizeCalibrationHealth 從 parameters.json 解析 seasonal_patterns 並產生健康摘要。
// 不需要先跑校準;直接讀檔即可。任何欄位缺失會回退為零值而非錯誤,便於監控穩健性。
func SummarizeCalibrationHealth(paramsPath string) (*CalibrationHealthSummary, error) {
	raw, err := os.ReadFile(paramsPath)
	if err != nil {
		return nil, fmt.Errorf("read parameters.json: %w", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse parameters.json: %w", err)
	}

	summary := &CalibrationHealthSummary{
		VerdictCounts: make(map[string]int),
	}

	patterns := extractSeasonalPatterns(root)
	summary.PatternCount = len(patterns)

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
	summary.Health = deriveHealth(summary)
	return summary, nil
}

func extractSeasonalPatterns(root map[string]interface{}) []map[string]interface{} {
	industry, ok := root["industry"].(map[string]interface{})
	if !ok {
		return nil
	}
	sp, ok := industry["seasonal_patterns"].(map[string]interface{})
	if !ok {
		return nil
	}
	arr, ok := sp["value"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// 健康判定啟發式:
//   - critical:DarwinianViolations > 0 或 OutOfRangeCount > 0
//   - degraded:TotalObservations < PatternCount * 3 (平均觀察樣本不足)
//   - healthy:其他
func deriveHealth(s *CalibrationHealthSummary) HealthStatus {
	if s.DarwinianViolations > 0 || s.OutOfRangeCount > 0 {
		return HealthCritical
	}
	if s.PatternCount > 0 && s.TotalObservations < s.PatternCount*3 {
		return HealthDegraded
	}
	return HealthHealthy
}
