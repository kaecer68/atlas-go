package strategy_ranker

// StrategyReport 為單一策略的完整回測驗證績效報告。
type StrategyReport struct {
	StrategyID   string `json:"strategy_id"`
	StrategyName string `json:"strategy_name"`

	// 核心績效指標；無法計算時以 nil 輸出 null，避免前端把 0 誤判為真實數值。
	AnnualizedReturn *float64 `json:"annualized_return,omitempty"` // 年化報酬（%）
	MaxDrawdown      *float64 `json:"max_drawdown,omitempty"`      // 最大回撤（%）
	SharpeRatio      *float64 `json:"sharpe_ratio,omitempty"`      // 年化 Sharpe（TWSE 交易日頻率）
	WinRate          float64  `json:"win_rate"`                    // 勝率（0~1）
	TaiexCorrelation *float64 `json:"taiex_correlation,omitempty"` // 與加權指數的 Pearson 相關係數

	// 輔助資訊
	TotalReturn *float64 `json:"total_return,omitempty"` // 總累積報酬（%）
	SampleDays  int      `json:"sample_days"`            // 樣本交易日數
	AlphaScore  *float64 `json:"alpha_score,omitempty"`  // Alpha 近似值：年化報酬 − 加權指數年化報酬

	// 策略方向（來自 StrategyFrame.Direction），用於資金流共鳴偏置
	Direction string `json:"direction,omitempty"`
}

// ValidationConfig 為驗證器的可配置參數。
type ValidationConfig struct {
	RiskFreeRate float64 // 無風險利率（預設 0.01，即 1%）
	MinSamples   int     // 最少樣本數（少於此數不計算 Sharpe）
}

// DefaultValidationConfig 回傳建議的預設驗證參數。
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		RiskFreeRate: 0.01,
		MinSamples:   20,
	}
}

// RankedReport 為附帶排名與分層資訊的策略報告。
type RankedReport struct {
	StrategyReport
	Rank  int     `json:"rank"`  // 綜合排名（1 為最佳）
	Tier  string  `json:"tier"`  // 分層標籤：free, registered, premium
	Score float64 `json:"score"` // 綜合評分（0~100）
}

// BatchReport 為一次完整驗證批次的所有策略報告集合。
type BatchReport struct {
	BenchmarkID string         `json:"benchmark_id"` // 基準指數（如 TAIEX）
	StartDate   string         `json:"start_date"`
	EndDate     string         `json:"end_date"`
	Reports     []RankedReport `json:"reports"`
}
