package strategy_techniques

// Layer represents the 5-layer framework for Taiwan stock market analysis.
// Use as StrategyFrame.Layer to categorize each investment technique.
type Layer string

// Five canonical layers, ordered by macro→micro precedence.
const (
	LayerL1GlobalLiquidity   Layer = "L1" // 全球流動性 (Fed policy, US10Y, JPY, DXY)
	LayerL2ForeignBehavior   Layer = "L2" // 外資行為 (ForeignInvestorNet, 期貨淨多空)
	LayerL3IndustryCatalysts Layer = "L3" // 產業催化 (NVDA, TSM ADR, SOX, 台積電法說)
	LayerL4FXAndChips        Layer = "L4" // 匯率籌碼 (USD_TWD, 融資, 大戶)
	LayerL5Geopolitics       Layer = "L5" // 地緣政治 (台海, 關稅, 中美, 半導體禁令)
)

// AllLayers is the canonical 5-layer framework ordered by macro→micro precedence.
// Use to enumerate layers for dashboards, even if some are empty (count=0).
// Exposed as a public package variable so handlers can render the full L1~L5 board
// regardless of how many techniques exist for each layer.
var AllLayers = []Layer{
	LayerL1GlobalLiquidity,
	LayerL2ForeignBehavior,
	LayerL3IndustryCatalysts,
	LayerL4FXAndChips,
	LayerL5Geopolitics,
}

// IsValid reports whether the layer is one of the 5 canonical values.
func (l Layer) IsValid() bool {
	switch l {
	case LayerL1GlobalLiquidity, LayerL2ForeignBehavior,
		LayerL3IndustryCatalysts, LayerL4FXAndChips, LayerL5Geopolitics:
		return true
	}
	return false
}

// String returns the canonical string representation.
// Invalid values return "unknown" rather than panic, for safe logging.
func (l Layer) String() string {
	if l.IsValid() {
		return string(l)
	}
	return "unknown"
}

// Status represents the lifecycle status of a strategy frame.
// Mirrors the legacy EventRule.Status semantic for migration compatibility.
type Status string

const (
	StatusActive   Status = "active"   // 命中率高於閾值，持續啟用
	StatusDegraded Status = "degraded" // 命中率下滑，待回歸驗證
	StatusExpired  Status = "expired"  // 已失效，需手動覆核
)

// IsValid reports whether the status is one of the 3 canonical values.
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusDegraded, StatusExpired:
		return true
	}
	return false
}

// String returns the canonical string representation.
// Invalid values return "unknown" for safe logging.
func (s Status) String() string {
	if s.IsValid() {
		return string(s)
	}
	return "unknown"
}

// AttributionMode represents how a rule's failure is attributed.
// Hybrid mode (Q5 decision): rule-based classifier + LLM annotation.
type AttributionMode string

const (
	AttributionModeRuleBased    AttributionMode = "rule_based"    // 規則分類器（regime shift / 政策衝擊 / 結構斷裂 / 數據異常 / 季節性 / 流動性 / 板塊輪動 / 未知）
	AttributionModeLLMAnnotated AttributionMode = "llm_annotated" // LLM natural-language 歸因（Wave 2 接入 narrative engine）
)

// IsValid reports whether the attribution mode is one of the 2 canonical values.
func (a AttributionMode) IsValid() bool {
	switch a {
	case AttributionModeRuleBased, AttributionModeLLMAnnotated:
		return true
	}
	return false
}

// String returns the canonical string representation.
// Invalid values return "unknown" for safe logging.
func (a AttributionMode) String() string {
	if a.IsValid() {
		return string(a)
	}
	return "unknown"
}

// Direction represents the expected market direction when a StrategyFrame fires.
// Mirrors the legacy EventRule.Direction semantic.
type Direction string

const (
	DirectionUp       Direction = "up"       // 預期上漲
	DirectionDown     Direction = "down"     // 預期下跌
	DirectionVolatile Direction = "volatile" // 預期高波動
)

// IsValid reports whether the direction is one of the 3 canonical values.
func (d Direction) IsValid() bool {
	switch d {
	case DirectionUp, DirectionDown, DirectionVolatile:
		return true
	}
	return false
}

// String returns the canonical string representation.
// Invalid values return "unknown" for safe logging.
func (d Direction) String() string {
	if d.IsValid() {
		return string(d)
	}
	return "unknown"
}

// Risk represents the geopolitical / structural risk tier of a StrategyFrame.
// Used by the attribution engine to flag high-Risk frames for human review.
type Risk string

const (
	RiskLow    Risk = "low"    // 結構穩定，地緣敏感度低
	RiskMedium Risk = "medium" // 中度敏感，需監控
	RiskHigh   Risk = "high"   // 高地緣/結構風險，需風控覆核
)

// IsValid reports whether the risk tier is one of the 3 canonical values.
func (r Risk) IsValid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

// String returns the canonical string representation.
// Invalid values return "unknown" for safe logging.
func (r Risk) String() string {
	if r.IsValid() {
		return string(r)
	}
	return "unknown"
}

// Source represents the provenance of a StrategyFrame.
// Mirrors the legacy EventRule.ConfidenceSource semantic.
type Source string

const (
	SourceBacktest       Source = "backtest"        // 從歷史回測自動產生
	SourceManual         Source = "manual"          // 研究員/管理員手動建立
	SourceAutoDiscovered Source = "auto_discovered" // PatternDetector 自動發現
)

// IsValid reports whether the source is one of the 3 canonical values.
func (s Source) IsValid() bool {
	switch s {
	case SourceBacktest, SourceManual, SourceAutoDiscovered:
		return true
	}
	return false
}

// String returns the canonical string representation.
// Invalid values return "unknown" for safe logging.
func (s Source) String() string {
	if s.IsValid() {
		return string(s)
	}
	return "unknown"
}
