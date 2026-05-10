package domain

//go:generate go run ../../cmd/gentags

import "time"

type Position struct {
	Symbol        string  `json:"symbol"`
	Quantity      int     `json:"quantity"`
	AverageCost   float64 `json:"average_cost"`
	CurrentPrice  float64 `json:"current_price"`
	MarketValue   float64 `json:"market_value"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
}

type Order struct {
	Symbol   string
	Side     Side
	Quantity int
	Price    float64
	Reason   string
}

type SimulationConstraints struct {
	StartingCash                float64 `json:"starting_cash"`
	MaxPositionWeight           float64 `json:"max_position_weight"`
	MaxOpenPositions            int     `json:"max_open_positions"`
	MinTradableVolume           int64   `json:"min_tradable_volume"`
	MinRecommendationConviction int     `json:"min_recommendation_conviction"`
	RequireCROPass              bool    `json:"require_cro_pass"`
	TransactionCostBPS          float64 `json:"transaction_cost_bps"`
	SlippageBPS                 float64 `json:"slippage_bps"`
	ReserveCashFraction         float64 `json:"reserve_cash_fraction"`
	StopLossPct                 float64 `json:"stop_loss_pct"`   // sell when price drops below avgCost*(1+StopLossPct)
	TakeProfitPct               float64 `json:"take_profit_pct"` // sell when price rises above avgCost*(1+TakeProfitPct)
}

type ExecutionPolicy struct {
	ConvictionFloor               int  `json:"conviction_floor"`
	RequireCROPass                bool `json:"require_cro_pass"`
	MomentumCrashProtection       bool `json:"momentum_crash_protection"`
	EnableConvictionNormalization bool `json:"enable_conviction_normalization"`
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
	Trades         []TradeRecord
	Positions      []Position
	EndingCash     float64
	PortfolioValue float64
	GuardOutcomes  []GuardOutcome
	RiskSnapshot   *RiskSnapshot `json:"risk_snapshot,omitempty"`
	TaxSnapshots   []TaxSnapshot `json:"tax_snapshots,omitempty"`
	BeforeTaxPnL   float64       `json:"before_tax_pnl"`
	AfterTaxPnL    float64       `json:"after_tax_pnl"`
	TotalTaxPaid   float64       `json:"total_tax_paid"`
	FallbackEvents []string      `json:"fallback_events,omitempty"`
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

type DividendRecord struct {
	Symbol         string  `json:"symbol"`
	Year           int     `json:"year"`
	CashDividend   float64 `json:"cash_dividend"`
	StockDividend  float64 `json:"stock_dividend"`
	ExDividendDate string  `json:"ex_dividend_date"`
	PaymentDate    string  `json:"payment_date"`
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
	CurrentPhase         CapitalPhase       `json:"current_phase"`
	PhaseStartDate       time.Time          `json:"phase_start_date"`
	MinDaysPerPhase      int                `json:"min_days_per_phase"`
	MaxDrawdownLimit     float64            `json:"max_drawdown_limit"`
	SharpeThreshold      float64            `json:"sharpe_threshold"`
	CapitalLimits        map[string]float64 `json:"capital_limits"`
	MaxConsecutiveLosses int                `json:"max_consecutive_losses"`
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
		MaxConsecutiveLosses: 5,
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
