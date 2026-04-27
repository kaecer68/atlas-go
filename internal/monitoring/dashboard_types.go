package monitoring

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type AgentUniverseView struct {
	AgentID           string                   `json:"agent_id"`
	Name              string                   `json:"name"`
	Layer             string                   `json:"layer"`
	Universe          []string                 `json:"universe"`
	ScreeningCriteria domain.ScreeningCriteria `json:"screening_criteria"`
}

type SectorExposure struct {
	Sector      string  `json:"sector"`
	SectorLabel string  `json:"sector_label"`
	Weight      float64 `json:"weight"`
	EstValue    float64 `json:"est_value"`
}

type FactorExposureInline struct {
	Momentum float64 `json:"momentum"`
	Value    float64 `json:"value"`
	Quality  float64 `json:"quality"`
	Agent    float64 `json:"agent"`
	Total    float64 `json:"total"`
}

type PnLAttributionResponse struct {
	SnapshotTime      time.Time           `json:"snapshot_time"`
	SessionID         string              `json:"session_id"`
	StartingValue     float64             `json:"starting_value"`
	CurrentValue      float64             `json:"current_value"`
	CumulativePnL     float64             `json:"cumulative_pnl"`
	CumulativeRetPct  float64             `json:"cumulative_return_pct"`
	AgentAttribution  []AgentAttribution  `json:"agent_attribution"`
	SectorAttribution []SectorAttribution `json:"sector_attribution"`
	FactorAttribution FactorAttribution   `json:"factor_attribution"`
	SymbolAttribution []SymbolAttribution `json:"symbol_attribution"`
}

type AgentAttribution struct {
	AgentID     string  `json:"agent_id"`
	AgentName   string  `json:"agent_name"`
	Layer       string  `json:"layer"`
	TotalReturn float64 `json:"total_return"`
	Count       int     `json:"count"`
	AvgReturn   float64 `json:"avg_return"`
}

type SectorAttribution struct {
	Sector      string  `json:"sector"`
	SectorLabel string  `json:"sector_label"`
	TotalReturn float64 `json:"total_return"`
	Count       int     `json:"count"`
	AvgReturn   float64 `json:"avg_return"`
}

type FactorAttribution struct {
	Momentum FactorDetail `json:"momentum"`
	Value    FactorDetail `json:"value"`
	Quality  FactorDetail `json:"quality"`
	Agent    FactorDetail `json:"agent"`
	Total    FactorDetail `json:"total"`
}

type FactorDetail struct {
	AvgScore     float64 `json:"avg_score"`
	AvgReturn    float64 `json:"avg_return"`
	Contribution float64 `json:"contribution"`
}

type SymbolAttribution struct {
	Symbol      string  `json:"symbol"`
	TotalReturn float64 `json:"total_return"`
	Count       int     `json:"count"`
	AvgReturn   float64 `json:"avg_return"`
	Side        string  `json:"side"`
}

type RiskExposureResponse struct {
	SnapshotTime     time.Time               `json:"snapshot_time"`
	VaR95            float64                 `json:"var_95"`
	VaR99            float64                 `json:"var_99"`
	CVaR95           float64                 `json:"cvar_95"`
	MaxDrawdownPct   float64                 `json:"max_drawdown_pct"`
	PortfolioValue   float64                 `json:"portfolio_value"`
	CashRatio        float64                 `json:"cash_ratio"`
	PositionCount    int                     `json:"position_count"`
	SectorExposure   []SectorExposure        `json:"sector_exposure"`
	FactorExposure   FactorExposureInline    `json:"factor_exposure"`
	Concentration    []PositionConcentration `json:"concentration"`
	DataPoints       int                     `json:"data_points"`
	InsufficientData bool                    `json:"insufficient_data"`
}

type PositionConcentration struct {
	Symbol      string  `json:"symbol"`
	MarketValue float64 `json:"market_value"`
	Weight      float64 `json:"weight"`
}

type ExperimentInboxItem struct {
	ExperimentID    string                  `json:"experiment_id"`
	TargetAgentID   string                  `json:"target_agent_id"`
	Skill           string                  `json:"skill"`
	MutationType    string                  `json:"mutation_type"`
	MutationSummary string                  `json:"mutation_summary,omitempty"`
	Status          domain.ExperimentStatus `json:"status"`
	BaselineValue   float64                 `json:"baseline_value"`
	CandidateValue  float64                 `json:"candidate_value"`
	CandidatePath   string                  `json:"candidate_path"`
	RejectReason    string                  `json:"reject_reason,omitempty"`
	RecordedAt      time.Time               `json:"recorded_at"`
}

type ExperimentInboxResponse struct {
	PendingJudges   []ExperimentInboxItem `json:"pending_judges"`
	PendingPromotes []ExperimentInboxItem `json:"pending_promotes"`
	RecentHistory   []ExperimentInboxItem `json:"recent_history"`
	BaselineVersion int                   `json:"baseline_version"`
}
