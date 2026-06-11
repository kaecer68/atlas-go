package strategy_techniques

import (
	"errors"
	"fmt"
)

// Condition 描述心法的觸發條件。一個 StrategyFrame 可有多個 Condition（AND 運算）。
//
// 觸發條件的單一原子表達。新增 Timeframe（多時間尺度）與 Source（數據追溯）以支援
// L1~L5 不同時間維度的策略組合。
// Operator 支援：eq, neq, gt, lt, gte, lte, cross_above, cross_below。
type Condition struct {
	Field       string  `json:"field"`        // 例如 "DXY", "ForeignInvestorNet", "TSMADR.ChangePct"
	Operator    string  `json:"operator"`     // eq/neq/gt/lt/gte/lte/cross_above/cross_below
	Value       float64 `json:"value"`        // 比較門檻值
	StringValue string  `json:"string_value"` // 當比較對象為字串（如 Regimes="NOVEL"）時使用
	Timeframe   string  `json:"timeframe"`    // 1D/3D/5D/20D（時間窗口）
	Source      string  `json:"source"`       // 數據來源 channel ID（用於追溯）
}

// validOperators 是允許的運算子集合。Validate 會檢查 Condition.Operator 是否在表內。
var validOperators = map[string]struct{}{
	"eq":          {},
	"neq":         {},
	"gt":          {},
	"lt":          {},
	"gte":         {},
	"lte":         {},
	"cross_above": {},
	"cross_below": {},
}

// Validate 檢查 Condition 欄位合法性。
func (c Condition) Validate() error {
	if c.Field == "" {
		return errors.New("Condition.Field is required")
	}
	if _, ok := validOperators[c.Operator]; !ok {
		return fmt.Errorf("Condition.Operator %q is not a valid value (allowed: eq/neq/gt/lt/gte/lte/cross_above/cross_below)", c.Operator)
	}
	if c.Timeframe != "" {
		switch c.Timeframe {
		case "1D", "3D", "5D", "20D", "60D":
			// valid
		default:
			return fmt.Errorf("Condition.Timeframe %q is not a valid value (allowed: 1D/3D/5D/20D/60D)", c.Timeframe)
		}
	}
	return nil
}

// validTimeframes 匯出供 registry 與 detector 複用。
var ValidTimeframes = []string{"1D", "3D", "5D", "20D", "60D"}
