package domain

//go:generate go run ../../cmd/gentags

import "time"

type Position struct {
	Symbol        string    `json:"symbol"`
	Quantity      int       `json:"quantity"`
	AverageCost   float64   `json:"average_cost"`
	CurrentPrice  float64   `json:"current_price"`
	MarketValue   float64   `json:"market_value"`
	UnrealizedPnL float64   `json:"unrealized_pnl"`
	EntryDate     time.Time `json:"entry_date"`
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
	DiscountedCommissionBps     float64 `json:"discounted_commission_bps"`
	CommissionDiscountThreshold float64 `json:"commission_discount_threshold"`
	SlippageBPS                 float64 `json:"slippage_bps"`
	ReserveCashFraction         float64 `json:"reserve_cash_fraction"`
	StopLossPct                 float64 `json:"stop_loss_pct"`   // sell when price drops below avgCost*(1+StopLossPct)
	TakeProfitPct               float64 `json:"take_profit_pct"` // sell when price rises above avgCost*(1+TakeProfitPct)
	MaxHoldingDays              int     `json:"max_holding_days"`
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
	RiskCommentary string        `json:"risk_commentary,omitempty"`
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

	// E3 P1: 七時期結構化欄位（PeriodDetector 接入後填充）
	MarketPeriod     string  `json:"market_period,omitempty"`     // period id: downturn/bull/plateau/...
	PeriodConfidence float64 `json:"period_confidence,omitempty"` // 0-1 detection confidence
	PeriodCashLevel  float64 `json:"period_cash_level,omitempty"` // recommended cash reserve (0-1)
	PeriodCondHit    int     `json:"period_cond_hit,omitempty"`   // conditions that actually triggered
	PeriodCondTotal  int     `json:"period_cond_total,omitempty"` // total conditions for this period
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
	NHISurchargeRate   float64 `json:"nhi_surcharge_rate"`
	IncludeNHI         bool    `json:"include_nhi"`
}

func DefaultTaiwanTaxConfig() TaxConfig {
	return TaxConfig{
		DividendTaxRate:    0.28,
		TransactionTaxRate: 0.003,
		NHISurchargeRate:   0.0211,
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

type AlertBreakdown struct {
	Rule        string  `json:"rule"`
	Description string  `json:"description"`
	Current     float64 `json:"current"`
	Threshold   float64 `json:"threshold"`
	Delta       float64 `json:"delta"`
	Formula     string  `json:"formula"`
}

// AlertStatus represents the lifecycle state of an alert.
type AlertStatus string

const (
	AlertStatusTriggered    AlertStatus = "triggered"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
	AlertStatusSilenced     AlertStatus = "silenced"
)

type AlertRecord struct {
	ID             string          `json:"id"`
	Timestamp      time.Time       `json:"timestamp"`
	Rule           string          `json:"rule"`
	Severity       string          `json:"severity"`
	Message        string          `json:"message"`
	Value          float64         `json:"value"`
	Threshold      float64         `json:"threshold"`
	Breakdown      *AlertBreakdown `json:"breakdown,omitempty"`
	Acknowledged   bool            `json:"acknowledged"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string          `json:"acknowledged_by,omitempty"`

	// Lifecycle fields (Phase 2A)
	Status        AlertStatus `json:"status"`
	DedupKey      string      `json:"dedup_key,omitempty"`
	FirstSeen     *time.Time  `json:"first_seen,omitempty"`
	LastSeen      *time.Time  `json:"last_seen,omitempty"`
	Count         int         `json:"count"`
	ResolvedAt    *time.Time  `json:"resolved_at,omitempty"`
	ResolvedBy    string      `json:"resolved_by,omitempty"`
	SilencedUntil *time.Time  `json:"silenced_until,omitempty"`

	// Decision 9 (alert-redesign-v2.md Part 3.7): SLA tracking. Pointer
	// because legacy alerts saved before this field existed will not have
	// it; nil = legacy/no data, > 0 = latency in seconds from emit to ack.
	AcknowledgedWithinSec *int `json:"acknowledged_within_sec,omitempty"`
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
	MarginBalance          float64             `json:"margin_balance"`
	MarginChangePct        float64             `json:"margin_change_pct"`
	DayTradingRatio        float64             `json:"day_trading_ratio"`
	MarginPercentile       float64             `json:"margin_percentile"`
	Timestamp              time.Time           `json:"timestamp"`
	RetailFuturesOI        float64             `json:"retail_futures_oi,omitempty"`        // 小台指散戶未平倉比例
	ETINetSubscription     float64             `json:"etf_net_subscription,omitempty"`     // ETF 淨申購 — ⚠️ 資料源已移除(TWT44U→404)，恆為 0
	CompositeSentiment     float64             `json:"composite_sentiment"`                // RSI-tw 綜合指數 -1.0 to 1.0
	SentimentSubIndicators *RSITwSubIndicators `json:"sentiment_sub_indicators,omitempty"` // 子指標明細
}

// RSITwSubIndicators holds the sub-indicator breakdown for RSI-tw composite sentiment.
type RSITwSubIndicators struct {
	CategoryA *RSITwCategoryA `json:"category_a,omitempty"`
	CategoryC *RSITwCategoryC `json:"category_c,omitempty"`
	CategoryD *RSITwCategoryD `json:"category_d,omitempty"`
}

// RSITwCategoryA covers margin & market-sentiment proxies (40% weight).
type RSITwCategoryA struct {
	MarginMaintenanceZ float64  `json:"margin_maintenance_z"` // 維持率 Z-score
	DayTradingZ        float64  `json:"day_trading_z"`        // 當沖 Z-score
	MarginBalanceZ     float64  `json:"margin_balance_z"`     // 融資餘額 Z-score
	VIXRiskScore       float64  `json:"vix_risk_score"`       // VIX 風險分數 0-1
	WeeklyPCR          float64  `json:"weekly_pcr"`           // 週選擇權 PCR
	OddLotImbalance    float64  `json:"odd_lot_imbalance"`    // 零股交易失衡
	AScore             float64  `json:"a_score"`              // Part A 綜合分數
	IsFallback         bool     `json:"is_fallback"`          // true if any sub-indicator is fallback
	FallbackFields     []string `json:"fallback_fields"`      // per-sub-indicator fallback keys (a1_margin_z, a2_day_trading, ...)
}

// RSITwCategoryC covers institutional / futures / ETF flow proxies (25% weight).
type RSITwCategoryC struct {
	FuturesRetailOI      float64  `json:"futures_retail_oi"`      // 散戶期貨 OI
	BrokerFlowScore      float64  `json:"broker_flow_score"`      // 券商分點流向
	ETFSubscriptionScore float64  `json:"etf_subscription_score"` // ETF 申購分數
	CScore               float64  `json:"c_score"`                // Part C 綜合分數
	IsFallback           bool     `json:"is_fallback"`            // true if any sub-indicator is fallback
	FallbackFields       []string `json:"fallback_fields"`        // per-sub-indicator fallback keys (c1_futures_oi, c2_inst_flow, c3_etf_sub)
}

// RSITwCategoryD captures the event-driven adjustment multiplier.
type RSITwCategoryD struct {
	AdjustmentFactor float64  `json:"adjustment_factor"` // 事件調整倍數 0.8-1.2
	ActiveEvents     []string `json:"active_events"`     // 目前觸發的事件
	DMultiplier      float64  `json:"d_multiplier"`      // 最終乘數
	IsFallback       bool     `json:"is_fallback"`       // true if any sub-factor is fallback
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

// DataQuality carries per-response metadata about data freshness,
// provenance, and trustworthiness (#1265 unified metadata envelope).
// When Available=false, the response value is a semantic zero —
// the consumer MUST NOT interpret it as a real measurement.
type DataQuality struct {
	Available      bool      `json:"available"`
	Source         string    `json:"source,omitempty"`
	AsOf           time.Time `json:"as_of,omitempty"`
	SampleCount    int       `json:"sample_count"`
	SampleUnit     string    `json:"sample_unit,omitempty"`
	IsFallback     bool      `json:"is_fallback"`
	FallbackReason string    `json:"fallback_reason,omitempty"`
	Provenance     string    `json:"provenance,omitempty"`
}

func (d DataQuality) DataStatus() string {
	if !d.Available {
		return "unavailable"
	}
	if d.IsFallback {
		return "degraded"
	}
	return "ok"
}

func (d DataQuality) ToMap() map[string]any {
	m := map[string]any{
		"available": d.Available,
	}
	if d.Source != "" {
		m["source"] = d.Source
	}
	if !d.AsOf.IsZero() {
		m["as_of"] = d.AsOf.Format(time.RFC3339)
	}
	if d.SampleCount > 0 {
		m["sample_count"] = d.SampleCount
	}
	if d.SampleUnit != "" {
		m["sample_unit"] = d.SampleUnit
	}
	if d.IsFallback {
		m["is_fallback"] = true
	}
	if d.FallbackReason != "" {
		m["fallback_reason"] = d.FallbackReason
	}
	if d.Provenance != "" {
		m["provenance"] = d.Provenance
	}
	return m
}
