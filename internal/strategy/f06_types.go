package strategy

import "time"

// EvaluationModeShadow marks every observation produced by the shadow evaluator.
const EvaluationModeShadow = "shadow"

// StrategyDailyObservation records a single strategy's daily benchmark-relative result.
type StrategyDailyObservation struct {
	TradingDate     string  `json:"trading_date"`
	StrategyID      string  `json:"strategy_id"`
	EvaluationMode  string  `json:"evaluation_mode"`
	DailyReturn     float64 `json:"daily_return"`
	BenchmarkReturn float64 `json:"benchmark_return"`
	Outperformance  float64 `json:"outperformance"`
	OutcomeCount    int     `json:"outcome_count"`
}

// BenchmarkObservation captures the benchmark (TAIEX) daily state.
type BenchmarkObservation struct {
	TradingDate string  `json:"trading_date"`
	SourceID    string  `json:"source_id"`
	ReasonCode  string  `json:"reason_code"`
	Return      float64 `json:"return"`
	Available   bool    `json:"available"`
}

// WarmingUpState informs consumers that not enough history exists.
type WarmingUpState struct {
	Status            string `json:"status"`
	LastTradingDate   string `json:"last_trading_date"`
	ReasonCode        string `json:"reason_code"`
	SampleDays        int    `json:"sample_days"`
	MinHistoryDays    int    `json:"min_history_days"`
	DaysUntilEligible int    `json:"days_until_eligible"`
}

// RankedStrategy is a single strategy ranking entry.
type RankedStrategy struct {
	Rank           int     `json:"rank"`
	SampleDays     int     `json:"sample_days"`
	StrategyID     string  `json:"strategy_id"`
	EvaluationMode string  `json:"evaluation_mode"`
	Score          float64 `json:"score"`
}

// RankingSnapshot is the full shadow ranking state for a given trading date.
type RankingSnapshot struct {
	AsOfTradingDate string               `json:"as_of_trading_date"`
	WarmingUp       WarmingUpState       `json:"warming_up"`
	Ranked          []RankedStrategy     `json:"ranked"`
	DeployedMix     map[string]float64   `json:"deployed_mix"`
	Benchmark       BenchmarkObservation `json:"benchmark"`
}

// ComparisonDay is a single day of shadow comparison data (persisted by FileComparisonStore).
type ComparisonDay struct {
	TradingDate  string                     `json:"trading_date"`
	Benchmark    BenchmarkObservation       `json:"benchmark"`
	Observations []StrategyDailyObservation `json:"observations"`
	DeployedMix  map[string]float64         `json:"deployed_mix"`
}

// ComparisonStore persists and retrieves ComparisonDay entries.
type ComparisonStore interface {
	Load(ctx interface{}) ([]ComparisonDay, error)
	Upsert(ctx interface{}, day ComparisonDay) error
}

// ShadowRankingProvider exposes the latest ranking snapshot to consumers.
type ShadowRankingProvider interface {
	RankingSnapshot(asOf time.Time) RankingSnapshot
}
