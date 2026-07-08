package recommender

// 本檔定義 recommender 對外整合的 service interfaces。
// Sprint 2 T7 引入：handler 可注入這些 deps，但目前僅 NIL-safe (保持 T8-T12 stub)。
//
// 設計原則: interface 由 consumer (recommender) 定義, 而不是 producer (narrative/capitalflow/...)
// — 遵循 Go 的「accept interfaces」原則, 防止循環依賴。

// NarrativeProvider 供 recommender 查詢當前 regime + Taiwan stress index。
// 對應實作: internal/monitoring/service/narrative.go::NarrativeService。
type NarrativeProvider interface {
	GetCurrentStressIndex(ctx interface{}) (StressIndexInfo, error)
	BuildMarketNarrativeData(ctx interface{}) (MarketNarrativeInfo, error)
}

// CapitalFlowProvider 供 recommender 查詢當日七大資金勢力 summary。
// 對應實作: internal/capitalflow.Handler。
type CapitalFlowProvider interface {
	LatestDaily(ctx interface{}) (CapitalFlowDailyInfo, error)
}

// EventPredictor 供 recommender 查詢當日事件 + 5 日預測。
// 對應實作: internal/eventdriven.Predictor。
type EventPredictor interface {
	PredictToday(ctx interface{}) ([]EventPredictionInfo, error)
}

// ComparisonEngine 供 recommender 計算策略 EntrySignal/StopLoss。
// 對應實作: internal/strategy.ComparisonEngine。
type ComparisonEngine interface {
	GetScore(strategyID string) (StrategyScoreInfo, error)
}

// 下面是 consumer-side 的 value types (推薦器市場信號的最小 surface):
// 這些型別獨立於 producer, 之後可手動 mapping 從真實 service types。

// StressIndexInfo 對應 narrative.TaiwanStressIndex 的關鍵欄位。
type StressIndexInfo struct {
	Value   float64
	Regime  string
	HasData bool
}

// MarketNarrativeInfo 對應 narrative.MarketNarrativeData 的關鍵欄位。
type MarketNarrativeInfo struct {
	Events []string
	Chains []string
}

// CapitalFlowDailyInfo 對應 capitalflow.DailySnapshot 的高階摘要。
type CapitalFlowDailyInfo struct {
	Summary  string
	Resonance float64
}

// EventPredictionInfo 對應 eventdriven.DailyPrediction。
type EventPredictionInfo struct {
	Date       string
	Direction  string
	Magnitude  float64
	Confidence float64
}

// StrategyScoreInfo 對應 strategy.ComparisonEngine.GetScore 的輸出。
type StrategyScoreInfo struct {
	Score      float64
	EntrySignal string
	StopLoss   float64
	TakeProfit float64
}
