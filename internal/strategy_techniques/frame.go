package strategy_techniques

import (
	"errors"
	"fmt"
	"time"
)

// StrategyFrame 是投資心法庫的核心資料結構，對應到 dashboard / decision-chain
// / portfolio 設定檔之間的傳輸格式。取代舊 internal/eventlogic.EventRule。
//
// 5 層框架：Layer (L1~L5)
// 自我修正：Attribution[] + AttributionMode (rule_based | llm_annotated)
// 跨週期驗證：HitRate / TotalTests / TotalHits + 多時間尺度由 detector 提供
type StrategyFrame struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Layer           Layer           `json:"layer"`
	Summary         string          `json:"summary"`
	Rationale       string          `json:"rationale"`
	Conditions      []Condition     `json:"conditions"`
	Themes          []string        `json:"themes"`
	Regimes         []string        `json:"regimes"`
	Sectors         []string        `json:"sectors"`
	Direction       Direction       `json:"direction"`
	Risk            Risk            `json:"risk"`
	Source          Source          `json:"source"`
	Status          Status          `json:"status"`
	AttributionMode AttributionMode `json:"attribution_mode"`
	Attribution     []string        `json:"attribution"`
	HitRate         float64         `json:"hit_rate"`
	TotalTests      int             `json:"total_tests"`
	TotalHits       int             `json:"total_hits"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Validate 檢查 StrategyFrame 的所有必填欄位、enum 合法性、數值邊界。
// 回傳 nil 表示通過；error 中應含失敗欄位名稱以利除錯。
func (f StrategyFrame) Validate() error {
	if f.ID == "" {
		return errors.New("ID is required")
	}
	if f.Name == "" {
		return errors.New("name is required")
	}
	if f.Summary == "" {
		return errors.New("summary is required")
	}
	if len(f.Conditions) == 0 {
		return errors.New("conditions must contain at least one element")
	}
	if !f.Layer.IsValid() {
		return fmt.Errorf("Layer %q is not a valid value", f.Layer)
	}
	if !f.Direction.IsValid() {
		return fmt.Errorf("Direction %q is not a valid value", f.Direction)
	}
	if !f.Risk.IsValid() {
		return fmt.Errorf("Risk %q is not a valid value", f.Risk)
	}
	if !f.Source.IsValid() {
		return fmt.Errorf("Source %q is not a valid value", f.Source)
	}
	if !f.Status.IsValid() {
		return fmt.Errorf("Status %q is not a valid value", f.Status)
	}
	if !f.AttributionMode.IsValid() {
		return fmt.Errorf("AttributionMode %q is not a valid value", f.AttributionMode)
	}
	if f.HitRate < 0 || f.HitRate > 1 {
		return fmt.Errorf("HitRate %f out of range [0, 1]", f.HitRate)
	}
	if f.TotalTests < 0 {
		return fmt.Errorf("TotalTests %d must be >= 0", f.TotalTests)
	}
	if f.TotalHits < 0 {
		return fmt.Errorf("TotalHits %d must be >= 0", f.TotalHits)
	}
	if f.TotalHits > f.TotalTests {
		return fmt.Errorf("TotalHits %d cannot exceed TotalTests %d", f.TotalHits, f.TotalTests)
	}
	return nil
}
