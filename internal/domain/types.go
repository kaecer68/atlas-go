package domain

import "time"

type Regime string

const (
	RegimeRiskOn  Regime = "RISK_ON"
	RegimeRiskOff Regime = "RISK_OFF"
	RegimeNeutral Regime = "NEUTRAL"
)

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type Quote struct {
	Symbol     string    `json:"symbol"`
	Last       float64   `json:"last"`
	Open       float64   `json:"open"`
	High       float64   `json:"high"`
	Low        float64   `json:"low"`
	Volume     int64     `json:"volume"`
	Market     string    `json:"market"`
	AsOf       time.Time `json:"as_of"`
	IsTradable bool      `json:"is_tradable"`
	Source     string    `json:"source"`
}

type FactorScoreItem struct {
	Score      float64            `json:"score"`
	Weight     float64            `json:"weight,omitempty"`
	Formula    string             `json:"formula"`
	RawInputs  map[string]float64 `json:"raw_inputs"`
	IsFallback bool               `json:"is_fallback"`
}

type FactorScoreBreakdown struct {
	Momentum               FactorScoreItem `json:"momentum"`
	Value                  FactorScoreItem `json:"value"`
	Quality                FactorScoreItem `json:"quality"`
	Agent                  FactorScoreItem `json:"agent"`
	InstitutionalSentiment FactorScoreItem `json:"institutional_sentiment"`
	Liquidity              FactorScoreItem `json:"liquidity"`
	Total                  FactorScoreItem `json:"total"`
}

type FactorScores struct {
	Momentum               float64               `json:"momentum"`
	Value                  float64               `json:"value"`
	Quality                float64               `json:"quality"`
	Agent                  float64               `json:"agent"`
	InstitutionalSentiment float64               `json:"institutional_sentiment"`
	Liquidity              float64               `json:"liquidity"`
	Total                  float64               `json:"total"`
	Breakdown              *FactorScoreBreakdown `json:"breakdown,omitempty"`
}

type ConvictionStep struct {
	Rule   string `json:"rule"`
	Delta  int    `json:"delta"`
	Reason string `json:"reason"`
}

type ConvictionBreakdown struct {
	Base  int              `json:"base"`
	Floor int              `json:"floor"`
	Final int              `json:"final"`
	Steps []ConvictionStep `json:"steps"`
}

type Recommendation struct {
	Agent               string
	Skill               string
	Layer               AgentLayer
	Symbol              string
	Side                Side
	Conviction          int
	TargetPrice         float64
	StopLossPrice       float64
	Reason              string
	ReasoningChain      []string             `json:"reasoning_chain,omitempty"`
	SupportingEvents    []string             `json:"supporting_events,omitempty"`
	FactorScores        FactorScores         `json:"factor_scores,omitempty"`
	ConvictionBreakdown *ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
}

type Position struct {
	Symbol        string
	Quantity      int
	AverageCost   float64
	CurrentPrice  float64
	MarketValue   float64
	UnrealizedPnL float64
}

type Order struct {
	Symbol   string
	Side     Side
	Quantity int
	Price    float64
	Reason   string
}

type SimulationConstraints struct {
	StartingCash                float64
	MaxPositionWeight           float64
	MaxOpenPositions            int
	MinTradableVolume           int64
	MinRecommendationConviction int
	RequireCROPass              bool
	TransactionCostBPS          float64
	SlippageBPS                 float64
	ReserveCashFraction         float64
	StopLossPct                 float64 // sell when price drops below avgCost*(1+StopLossPct)
	TakeProfitPct               float64 // sell when price rises above avgCost*(1+TakeProfitPct)
}

type ExecutionPolicy struct {
	ConvictionFloor               int
	RequireCROPass                bool
	MomentumCrashProtection       bool
	EnableConvictionNormalization bool
}

type RiskSnapshot struct {
	VaR95          float64 `json:"var_95"`
	VaR99          float64 `json:"var_99"`
	CVaR95         float64 `json:"cvar_95"`
	MaxDrawdownPct float64 `json:"max_drawdown_pct"`
}

type SimulationResult struct {
	Regime         Regime
	Orders         []Order
	Positions      []Position
	EndingCash     float64
	PortfolioValue float64
	GuardOutcomes  []GuardOutcome
	RiskSnapshot   *RiskSnapshot `json:"risk_snapshot,omitempty"`
	TaxSnapshots   []TaxSnapshot `json:"tax_snapshots,omitempty"`
	BeforeTaxPnL   float64       `json:"before_tax_pnl"`
	AfterTaxPnL    float64       `json:"after_tax_pnl"`
	TotalTaxPaid   float64       `json:"total_tax_paid"`
}

type ReportSection struct {
	Title    string `json:"title"`
	Content  string `json:"content"`
	Priority int    `json:"priority"`
}

type DailySummaryReport struct {
	Date           string           `json:"date"`
	Sections       []ReportSection  `json:"sections"`
	TopPicks       []Recommendation `json:"top_picks"`
	RiskLevel      string           `json:"risk_level"`
	NarrativeCount int              `json:"narrative_count"`
}

type TaxSnapshot struct {
	Symbol             string  `json:"symbol"`
	DividendTaxRate    float64 `json:"dividend_tax_rate"`
	TransactionTaxRate float64 `json:"transaction_tax_rate"`
	DividendTax        float64 `json:"dividend_tax"`
	TransactionTax     float64 `json:"transaction_tax"`
	TotalTax           float64 `json:"total_tax"`
	AfterTaxPnL        float64 `json:"after_tax_pnl"`
}

type TaxConfig struct {
	DividendTaxRate    float64 `json:"dividend_tax_rate"`
	TransactionTaxRate float64 `json:"transaction_tax_rate"`
	IncludeNHI         bool    `json:"include_nhi"`
}

func DefaultTaiwanTaxConfig() TaxConfig {
	return TaxConfig{
		DividendTaxRate:    0.28,
		TransactionTaxRate: 0.003,
		IncludeNHI:         true,
	}
}

type AlertRecord struct {
	ID             string     `json:"id"`
	Timestamp      time.Time  `json:"timestamp"`
	Rule           string     `json:"rule"`
	Severity       string     `json:"severity"`
	Message        string     `json:"message"`
	Value          float64    `json:"value"`
	Threshold      float64    `json:"threshold"`
	Acknowledged   bool       `json:"acknowledged"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
}

type AlertChannelConfig struct {
	TelegramBotToken string            `json:"telegram_bot_token,omitempty"`
	TelegramChatID   string            `json:"telegram_chat_id,omitempty"`
	EmailSMTPHost    string            `json:"email_smtp_host,omitempty"`
	EmailSMTPPort    int               `json:"email_smtp_port,omitempty"`
	EmailFrom        string            `json:"email_from,omitempty"`
	EmailTo          []string          `json:"email_to,omitempty"`
	EmailPassword    string            `json:"email_password,omitempty"`
	WebhookURL       string            `json:"webhook_url,omitempty"`
	WebhookHeaders   map[string]string `json:"webhook_headers,omitempty"`
}

type CapitalPhase string

const (
	PhaseSimulation CapitalPhase = "simulation"
	PhasePaper      CapitalPhase = "paper"
	PhaseLive       CapitalPhase = "live"
	PhaseFull       CapitalPhase = "full"
)

type CapitalPhaseConfig struct {
	CurrentPhase     CapitalPhase       `json:"current_phase"`
	PhaseStartDate   time.Time          `json:"phase_start_date"`
	MinDaysPerPhase  int                `json:"min_days_per_phase"`
	MaxDrawdownLimit float64            `json:"max_drawdown_limit"`
	SharpeThreshold  float64            `json:"sharpe_threshold"`
	CapitalLimits    map[string]float64 `json:"capital_limits"`
}

func DefaultCapitalPhaseConfig() CapitalPhaseConfig {
	return CapitalPhaseConfig{
		CurrentPhase:     PhaseSimulation,
		PhaseStartDate:   time.Now(),
		MinDaysPerPhase:  30,
		MaxDrawdownLimit: 0.10,
		SharpeThreshold:  1.0,
		CapitalLimits: map[string]float64{
			string(PhaseSimulation): 1.0,
			string(PhasePaper):      0.10,
			string(PhaseLive):       0.30,
			string(PhaseFull):       1.0,
		},
	}
}

type CapitalSnapshot struct {
	Phase             CapitalPhase `json:"phase"`
	PhaseStartDate    time.Time    `json:"phase_start_date"`
	DaysInPhase       int          `json:"days_in_phase"`
	TotalCapital      float64      `json:"total_capital"`
	DeployedCapital   float64      `json:"deployed_capital"`
	ReserveCash       float64      `json:"reserve_cash"`
	RollingSharpe     float64      `json:"rolling_sharpe"`
	MaxDrawdown       float64      `json:"max_drawdown"`
	ConsecutiveLosses int          `json:"consecutive_losses"`
	CanAdvance        bool         `json:"can_advance"`
	AdvanceReason     string       `json:"advance_reason,omitempty"`
}

type RetailSentimentSnapshot struct {
	MarginBalance    float64   `json:"margin_balance"`
	MarginChangePct  float64   `json:"margin_change_pct"`
	DayTradingRatio  float64   `json:"day_trading_ratio"`
	MarginPercentile float64   `json:"margin_percentile"`
	Timestamp        time.Time `json:"timestamp"`
}

func (s RetailSentimentSnapshot) CalculateSentimentScore() float64 {
	return (s.MarginPercentile - 0.5) * 2
}

func (s RetailSentimentSnapshot) ExtremeReading() string {
	if s.MarginPercentile >= 0.9 {
		return "frenzy"
	}
	if s.MarginPercentile <= 0.1 {
		return "fear"
	}
	return "neutral"
}
