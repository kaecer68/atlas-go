package eventbus

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// EventType 事件类型
type EventType string

const (
	// 市场数据事件
	EventMarketSnapshot EventType = "market.snapshot"
	EventMarketOpen     EventType = "market.open"
	EventMarketClose    EventType = "market.close"

	// 市场状态事件
	EventRegimeChange EventType = "market.regime.change"

	// 投资组合事件
	EventPositionUpdate EventType = "portfolio.position.update"
	EventPortfolioPnL   EventType = "portfolio.pnl.update"

	// Agent 事件
	EventAgentRecommendation EventType = "agent.recommendation"
	EventAgentEvaluation     EventType = "agent.evaluation"

	// 订单事件
	EventOrderPlaced   EventType = "order.placed"
	EventOrderFilled   EventType = "order.filled"
	EventOrderRejected EventType = "order.rejected"

	// 风险事件
	EventStopLossTriggered   EventType = "risk.stoploss.triggered"
	EventTakeProfitTriggered EventType = "risk.takeprofit.triggered"
	EventRiskAlert           EventType = "risk.alert"

	// 控制层事件
	EventGuardOutcome EventType = "guard.outcome"

	// 演化权重事件
	EventDarwinianClamping EventType = "darwinian.clamping"

	// Agent 健康状态事件
	EventAgentHealthChange EventType = "agent.health.change"

	// Conviction 夹制事件
	EventConvictionClamping EventType = "darwinian.conviction_clamping"

	// 系统事件
	EventSimulationStart    EventType = "simulation.start"
	EventSimulationComplete EventType = "simulation.complete"
	EventSystemStart        EventType = "system.start"
	EventSystemError        EventType = "system.error"

	// 实验事件
	EventExperimentInsufficientData EventType = "experiment.insufficient_data"
	EventExperimentAccepted         EventType = "experiment.accepted"
	EventExperimentRejected         EventType = "experiment.rejected"

	// 后台任务事件 (Decision 4)
	EventBackgroundTaskSustainedFailure EventType = "background_task.sustained_failure"
	EventBackgroundTaskRecovered        EventType = "background_task.recovered"

	// 叙事事件 (MacroIngestor 生成)
	EventNarrative EventType = "narrative.event"

	// 订单错误事件
	EventOrderError EventType = "order.error"

	// 自动监控事件
	EventDrawdownBreach      EventType = "monitor.drawdown.breach"
	EventConcentrationBreach EventType = "portfolio.concentration.breach"

	// 风险闸门事件
	EventRiskGateRejected   EventType = "monitor.risk_gate.rejected"
	EventRiskGateAllowed    EventType = "monitor.risk_gate.allowed"
	EventRiskGateOverridden EventType = "monitor.risk_gate.overridden"

	// 产业日历事件
	EventIndustryCalendar EventType = "industry.calendar.event"

	EventHealthAlert          EventType = "monitor.health.alert"
	EventPromotionRecorded    EventType = "experiment.promotion_recorded"
	EventBacktestCompleted    EventType = "experiment.backtest_completed"
	EventCalibrationCompleted EventType = "experiment.calibration_completed"
	EventTradeSlippage        EventType = "trade.slippage"

	// Wave 9 YELLOW 觀測性擴展（forward-compat 設計，只讀既有 public API）
	EventChannelIndividualHealth EventType = "monitor.channel.health.individual"
	EventRegimeChangeConfirmed   EventType = "market.regime.confirmed"
	EventFactorWeightRegression  EventType = "portfolio.factor.regression"
	EventDriftDetected           EventType = "portfolio.drift.detected"
	EventIngestionLagSpike       EventType = "apigateway.ingestion.lag.spike"

	// MCP observability (Phase 4 T1.4) — anomaly detector 偵測到異常時發布
	EventMCPAnomalyDetected EventType = "mcp.anomaly.detected"
)

// MarketEventPayload 市场事件载荷
type MarketEventPayload struct {
	Symbol    string       `json:"symbol"`
	Quote     domain.Quote `json:"quote"`
	Timestamp time.Time    `json:"timestamp"`
}

// RegimeEventPayload 市场状态事件载荷
type RegimeEventPayload struct {
	OldRegime    domain.Regime `json:"old_regime"`
	NewRegime    domain.Regime `json:"new_regime"`
	Confidence   float64       `json:"confidence"`
	DeterminedBy string        `json:"determined_by"`
}

// PositionEventPayload 持仓事件载荷
type PositionEventPayload struct {
	Symbol     string          `json:"symbol"`
	Position   domain.Position `json:"position"`
	ChangeType string          `json:"change_type"` // "added", "updated", "removed"
}

// RecommendationEventPayload 推荐事件载荷
type RecommendationEventPayload struct {
	Agent           string                  `json:"agent"`
	Recommendations []domain.Recommendation `json:"recommendations"`
}

// GuardOutcomeEventPayload 控制层过滤结果事件载荷
type GuardOutcomeEventPayload struct {
	SessionID string                `json:"session_id"`
	Outcomes  []domain.GuardOutcome `json:"outcomes"`
}

// DarwinianClampingEventPayload 演化权重夹制事件载荷
type DarwinianClampingEventPayload struct {
	ClampingEvents []ClampingEventPayload `json:"clamping_events"`
}

// AgentHealthChangeEventPayload Agent 健康状态变更事件载荷
type AgentHealthChangeEventPayload struct {
	AgentID   string    `json:"agent_id"`
	OldStatus string    `json:"old_status"`
	NewStatus string    `json:"new_status"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

type ConvictionClampingEventPayload struct {
	AgentID         string    `json:"agent_id"`
	Symbol          string    `json:"symbol"`
	RawConviction   int       `json:"raw_conviction"`
	FinalConviction int       `json:"final_conviction"`
	Weight          float64   `json:"weight"`
	Boundary        string    `json:"boundary"`
	Timestamp       time.Time `json:"timestamp"`
}

// ClampingEventPayload 单个夹制事件载荷
type ClampingEventPayload struct {
	AgentID     string    `json:"agent_id"`
	RawWeight   float64   `json:"raw_weight"`
	FinalWeight float64   `json:"final_weight"`
	Boundary    string    `json:"boundary"`
	Timestamp   time.Time `json:"timestamp"`
}

// OrderEventPayload 订单事件载荷
type OrderEventPayload struct {
	OrderID   string       `json:"order_id"`
	Order     domain.Order `json:"order"`
	Status    string       `json:"status"` // "placed", "filled", "rejected"
	FillPrice float64      `json:"fill_price,omitempty"`
	FillTime  time.Time    `json:"fill_time"`
}

// RiskEventPayload 风险事件载荷
type RiskEventPayload struct {
	Symbol       string          `json:"symbol"`
	Position     domain.Position `json:"position"`
	TriggerType  string          `json:"trigger_type"` // "stop_loss", "take_profit", "max_loss"
	TriggerPrice float64         `json:"trigger_price"`
}

// RiskGateEventPayload 风险闸门决策事件载荷
type RiskGateEventPayload struct {
	Phase                string    `json:"phase"`   // pre_trade, in_trade, post_trade
	Verdict              string    `json:"verdict"` // ALLOW, REDUCE, BLOCK, HALT, ALERT_ONLY
	Reason               string    `json:"reason"`
	ActionType           string    `json:"action_type"`        // SELL, REDUCE, FREEZE, LIQUIDATE, NOTIFY (空字串 if no action)
	ActionDescription    string    `json:"action_description"` // 人類可讀描述 (空字串 if no action)
	Mode                 string    `json:"mode"`               // NORMAL, CAUTIOUS, DEFENSIVE, SUSPENDED
	Symbol               string    `json:"symbol"`
	Timestamp            time.Time `json:"timestamp"`
	ConfidenceCommentary string    `json:"confidence_commentary,omitempty"` // LLM 生成信心註釋
}

// IndustryCalendarEventPayload 产业日历事件载荷
type IndustryCalendarEventPayload struct {
	EventID             string    `json:"event_id"`
	Name                string    `json:"name"`
	NameEN              string    `json:"name_en,omitempty"`
	EventType           string    `json:"event_type"`
	Description         string    `json:"description"`
	Direction           string    `json:"direction"` // bullish / bearish / mixed / neutral
	BaseWeight          float64   `json:"base_weight"`
	Active              bool      `json:"active"`
	StartDate           time.Time `json:"start_date"`
	EndDate             time.Time `json:"end_date"`
	PeakDate            time.Time `json:"peak_date"`
	DecayDays           int       `json:"decay_days"`
	AffectedIndustries  []string  `json:"affected_industries"`
	SentimentAdjustment float64   `json:"sentiment_adjustment"`
	DataSource          string    `json:"data_source"`      // default_rules / twse_provider / finmind_provider / mops_provider
	EvidenceQuality     string    `json:"evidence_quality"` // backtested / estimated / unverified / realtime
	GeneratedAt         time.Time `json:"generated_at"`
}

// BacktestCompletedEventPayload 自动回测完成事件载荷
type BacktestCompletedEventPayload struct {
	WindowID              string    `json:"window_id"`
	StartDate             time.Time `json:"start_date"`
	EndDate               time.Time `json:"end_date"`
	SessionCount          int       `json:"session_count"`
	OutcomeCount          int       `json:"outcome_count"`
	WorstAgentID          string    `json:"worst_agent_id"`
	WorstAgentSkill       string    `json:"worst_agent_skill"`
	WorstAgentLayer       string    `json:"worst_agent_layer"`
	WorstAgentWindowCount int       `json:"worst_agent_window_count"`
	WorstAgentSharpeLike  float64   `json:"worst_agent_sharpe_like"`
	GeneratedAt           time.Time `json:"generated_at"`
	TargetDate            time.Time `json:"target_date"`
	SyncSucceeded         bool      `json:"sync_succeeded"`
}

// CalibrationCompletedEventPayload 参数校准完成事件载荷
type CalibrationCompletedEventPayload struct {
	Module            string    `json:"module"`
	CalibratorName    string    `json:"calibrator_name"`
	ParamCount        int       `json:"param_count"`
	BaselineScore     float64   `json:"baseline_score"`
	OptimizedScore    float64   `json:"optimized_score"`
	Verdict           string    `json:"verdict"`
	ChangeCount       int       `json:"change_count"`
	TopChangeParam    string    `json:"top_change_param"`
	TopChangeDeltaPct float64   `json:"top_change_delta_pct"`
	GeneratedAt       time.Time `json:"generated_at"`
	SyncSucceeded     bool      `json:"sync_succeeded"`
}

// TradeSlippageEventPayload 滑价事件载荷 — emitted per order fill.
type TradeSlippageEventPayload struct {
	OrderID       string    `json:"order_id"`
	Symbol        string    `json:"symbol"`
	Side          string    `json:"side"`
	Quantity      int       `json:"quantity"`
	ExpectedPrice float64   `json:"expected_price"`
	FillPrice     float64   `json:"fill_price"`
	SlippageBPS   float64   `json:"slippage_bps"`
	SlippageCost  float64   `json:"slippage_cost"`
	BrokerMode    string    `json:"broker_mode"`
	Timestamp     time.Time `json:"timestamp"`
}

// DrawdownBreachPayload portfolio drawdown breach event payload.
// Emitted by internal/portfolio/risk_manager.go when currentDrawdown exceeds
// maxDrawdownPct. Consumed by monitoring/drawdown_consumer.go (Decision 8 PR-B)
// to surface the alert on the dashboard.
type DrawdownBreachPayload struct {
	CurrentDrawdown float64   `json:"current_drawdown"`
	MaxDrawdownPct  float64   `json:"max_drawdown_pct"`
	PortfolioValue  float64   `json:"portfolio_value"`
	PeakValue       float64   `json:"peak_value"`
	Timestamp       time.Time `json:"timestamp"`
}

// ConcentrationBreachPayload portfolio concentration breach event payload.
// Emitted by internal/risk/concentration_alert_emitter.go when per-position
// weight, positions count, or sector weight exceeds the configured thresholds.
// Consumed by a future monitoring consumer (Decision 7 follow-up).
type ConcentrationBreachPayload struct {
	Type      string    `json:"type"`      // "position" | "count" | "sector"
	Symbol    string    `json:"symbol"`    // for Type=="position"
	Sector    string    `json:"sector"`    // for Type=="sector"
	Value     float64   `json:"value"`     // breached metric (weight / count / exposure)
	Threshold float64   `json:"threshold"` // configured threshold
	Timestamp time.Time `json:"timestamp"`
}

// ExperimentInsufficientDataEventPayload 实验数据不足事件载荷
type ExperimentInsufficientDataEventPayload struct {
	ExperimentID  string `json:"experiment_id"`
	BaselineObs   int    `json:"baseline_observations"`
	CandidateObs  int    `json:"candidate_observations"`
	RequiredObs   int    `json:"required_observations"`
	MaturityLevel string `json:"maturity_level"`
	UsedFallback  bool   `json:"used_fallback_window"`
}

// OrderErrorEventPayload 订单错误事件载荷
type OrderErrorEventPayload struct {
	OrderID      string    `json:"order_id"`
	Symbol       string    `json:"symbol"`
	Side         string    `json:"side"`
	Price        float64   `json:"price"`
	Quantity     int       `json:"quantity"`
	ErrorCode    string    `json:"error_code"`
	ErrorMessage string    `json:"error_message"`
	Attempts     int       `json:"attempts"`
	LastStatus   string    `json:"last_status"`
	Timestamp    time.Time `json:"timestamp"`
}

// ExperimentLifecyclePayload experiment lifecycle event payload (Decision 5).
// Emitted by internal/experiment/lifecycle_publisher.go when an experiment
// transitions to Accepted (info) or Rejected (error). Consumed by a future
// monitoring consumer to surface baseline promotion/rejection on the dashboard.
type ExperimentLifecyclePayload struct {
	ExperimentID  string    `json:"experiment_id"`
	ProposalID    string    `json:"proposal_id"`
	TargetAgentID string    `json:"target_agent_id"`
	Skill         string    `json:"skill"`
	RevertReason  string    `json:"revert_reason,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// BackgroundTaskPayload background task lifecycle event payload (Decision 4).
// Emitted by internal/monitoring/background_task_tracker.go when a
// background task reaches the consecutive-failure threshold
// (SustainedFailure) or recovers after sustained failures (Recovered).
type BackgroundTaskPayload struct {
	TaskName            string    `json:"task_name"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	Threshold           int       `json:"threshold"`
	Timestamp           time.Time `json:"timestamp"`
}

// NarrativeEventPayload 叙事事件载荷
type NarrativeEventPayload struct {
	EventID              string  `json:"event_id"`
	Theme                string  `json:"theme"`
	Region               string  `json:"region"`
	Sentiment            float64 `json:"sentiment"`
	SentimentText        string  `json:"sentiment_text"`
	Confidence           float64 `json:"confidence"`
	ConfidenceSource     string  `json:"confidence_source"`
	HitRate              float64 `json:"hit_rate"`
	CapitalFlow          string  `json:"capital_flow"`
	TimeWindow           string  `json:"time_window"`
	Description          string  `json:"description"`
	Explanation          string  `json:"explanation,omitempty"`
	SentimentExplanation string  `json:"sentiment_explanation,omitempty"`
}

// HealthAlertPayload carries a single system health alert for downstream
// consumers (dashboard, Telegram, email, etc.).
type HealthAlertPayload struct {
	Severity        string    `json:"severity"`
	Category        string    `json:"category"`
	Message         string    `json:"message"`
	Value           float64   `json:"value"`
	Threshold       float64   `json:"threshold"`
	SuggestedAction string    `json:"suggested_action"`
	Timestamp       time.Time `json:"timestamp"`
}

// PromotionRecordedPayload is emitted when AutoRollback.RecordPromotion
// snapshots pre-promotion state for an experiment.
type PromotionRecordedPayload struct {
	ExperimentID       string    `json:"experiment_id"`
	PrePromotionSharpe float64   `json:"pre_promotion_sharpe"`
	Timestamp          time.Time `json:"timestamp"`
}

// MCPAnomalyEventPayload carries the structured fields of a detected MCP
// anomaly from the detector to SSE subscribers and downstream handlers.
// The field set matches alerting.AnomalyEvent 1:1 (minus the DetectedAt
// naming) so a single JSON-marshal path serves both the alert webhook and
// the event bus.
type MCPAnomalyEventPayload struct {
	AnomalyID   string  `json:"anomaly_id"`
	TenantID    string  `json:"tenant_id"`
	AnomalyType string  `json:"anomaly_type"`
	Tool        string  `json:"tool,omitempty"`
	Score       float64 `json:"score"`
	Severity    string  `json:"severity"`
	DetectedAt  string  `json:"detected_at"`
}

// BusEvent 总线事件
type BusEvent struct {
	ID            string    `json:"id"`
	Type          EventType `json:"type"`
	Timestamp     time.Time `json:"timestamp"`
	Payload       any       `json:"payload"`
	Description   string    `json:"description,omitempty"`
	Severity      string    `json:"severity,omitempty"` // "info", "warning", "error"
	SchemaVersion int       `json:"schema_version"`     // PD-1：事件 schema 版本，零值視為 v1 向後相容
}

// EnrichEvent populates Description and Severity on an event that lacks them,
// using the event type and payload to produce human-readable text suitable for
// the dashboard "即時事件流".
func EnrichEvent(ev *BusEvent) {
	if ev.Description != "" {
		return
	}
	ev.Description, ev.Severity = describeEvent(ev.Type, ev.Payload)
}

type eventDesc struct{ desc, severity string }

var eventDescriptions = map[EventType]eventDesc{
	EventSimulationStart:            {"模擬開始執行，系統正在調用 AI Agent 生成投資推薦", "info"},
	EventSimulationComplete:         {"模擬執行完成，推薦已生成並通過風控審查", "info"},
	EventSystemStart:                {"Atlas 系統啟動，開始監控市場與執行回測", "info"},
	EventSystemError:                {"系統發生錯誤，請檢查日誌", "error"},
	EventRegimeChange:               {"市場體制轉變，策略權重將自動調整", "warning"},
	EventAgentRecommendation:        {"AI Agent 生成新的投資推薦", "info"},
	EventAgentEvaluation:            {"AI Agent 績效評估完成", "info"},
	EventAgentHealthChange:          {"AI Agent 健康狀態改變", "warning"},
	EventGuardOutcome:               {"風控層（Guard）審查結果出爐", "info"},
	EventDarwinianClamping:          {"Darwinian 權重夾制觸發，部分 Agent 權重被限制", "warning"},
	EventConvictionClamping:         {"Conviction 信念夾制觸發，極端推薦被抑制", "warning"},
	EventOrderPlaced:                {"訂單已提交至券商", "info"},
	EventOrderFilled:                {"訂單成交", "info"},
	EventOrderRejected:              {"訂單被拒絕", "error"},
	EventOrderError:                 {"訂單處理發生錯誤", "error"},
	EventStopLossTriggered:          {"停損觸發！部位已強制平倉", "error"},
	EventTakeProfitTriggered:        {"停利觸發，部位已獲利了結", "info"},
	EventRiskAlert:                  {"風險警報！請注意市場異常", "warning"},
	EventPositionUpdate:             {"投資組合部位已更新", "info"},
	EventPortfolioPnL:               {"投資組合損益更新", "info"},
	EventMarketSnapshot:             {"市場快照已擷取", "info"},
	EventMarketOpen:                 {"市場開盤，開始接收即時報價", "info"},
	EventMarketClose:                {"市場收盤，停止即時交易", "info"},
	EventExperimentInsufficientData: {"實驗數據不足，無法進行統計比較", "warning"},
	EventNarrative:                  {"偵測到宏觀敘事事件", "warning"},
	EventHealthAlert:                {"系統健康監控警報觸發", "warning"},
	EventIndustryCalendar:           {"產業日曆事件：當前台股市場日曆事件（除權息、MSCI 調整、財報季等）", "info"},
	EventRiskGateRejected:           {"風控閘門拒絕交易，部位操作已被中止", "warning"},
	EventRiskGateAllowed:            {"風控閘門允許交易，部位操作已通過", "info"},
	EventRiskGateOverridden:         {"風控閘門決策被覆寫（REDUCE 或 ALERT_ONLY），仍記錄但不阻擋", "info"},
	EventBacktestCompleted:          {"自動回測完成，投組快照與風險訊號已記錄", "info"},
	EventCalibrationCompleted:       {"參數校準完成，模組參數已更新或保持不變", "info"},
	EventTradeSlippage:              {"訂單成交滑價計算：期望價與實際成交價之差（BPS），用於監控執行品質", "info"},
	EventChannelIndividualHealth:    {"個別監控通道健康狀態變化（per-channel error rate 變化）", "info"},
	EventRegimeChangeConfirmed:      {"市場體制轉變穩定確認（新 regime 持續 30 秒未變動）", "info"},
	EventFactorWeightRegression:     {"因子權重回歸偵測（regime 變化後權重位移超過閾值 0.5）", "info"},
	EventDriftDetected:              {"投資組合部位漂移偵測（單一持倉集中度 > 25% 或週轉率 > 15%）", "info"},
	EventIngestionLagSpike:          {"API Gateway ingestion p99 latency 超過 5 秒閾值", "warning"},
	EventMCPAnomalyDetected:         {"MCP 異常偵測：agent 行為或錯誤率超過閾值", "warning"},
}

var narrativeThemeLabels = map[string]string{
	"US_rates_up":                     "美國公債殖利率上升，資金可能回流美元資產，台股面臨外資流出壓力",
	"JPY_carry_unwind":                "日圓套利平倉潮，全球風險偏好下降，注意高槓桿部位",
	"geopolitical_risk_spike":         "地緣政治風險升溫，市場避險情緒上升，防禦型資產受青睞",
	"oil_price_shock":                 "油價劇烈波動，影響通膨預期與運輸成本，注意相關產業衝擊",
	"USD_TWD_volatility":              "台幣匯率波動加劇，反映出口競爭力變化與外資動向",
	"semiconductor_downturn":          "半導體產業景氣放緩訊號，注意科技股風險",
	"AI_capex_surge":                  "AI 資本支出持續強勁，科技供應鏈展望正面",
	"retail_frenzy":                   "散戶融資餘額飆升，市場過熱風險升高，注意回檔風險",
	"retail_fear":                     "散戶融資餘額低迷，市場情緒悲觀，可能是逢低佈局時機",
	"retail_institutional_divergence": "散戶與法人方向分歧，市場可能即將轉向",
}

func describeEvent(t EventType, payload any) (string, string) {
	if d, ok := eventDescriptions[t]; ok {
		desc := enrichWithPayload(d.desc, t, payload)
		return desc, d.severity
	}
	return string(t), "info"
}

func enrichWithPayload(base string, t EventType, payload any) string {
	m, ok := payload.(map[string]any)
	if !ok {
		return base
	}
	switch t {
	case EventRegimeChange:
		if from, _ := m["from"].(string); from != "" {
			if to, _ := m["to"].(string); to != "" {
				return "市場體制轉變：" + from + " → " + to + "，策略權重將自動調整"
			}
		}
	case EventNarrative:
		if theme, _ := m["theme"].(string); theme != "" {
			if label, ok := narrativeThemeLabels[theme]; ok {
				return label
			}
		}
		if theme, _ := m["Theme"].(string); theme != "" {
			if label, ok := narrativeThemeLabels[theme]; ok {
				return label
			}
		}
	}
	return base
}

// EventHandler 事件处理器函数类型
type EventHandler func(ctx context.Context, event BusEvent) error

type HandlerError struct {
	EventType    EventType
	SubscriberID string
	Err          error
}

// EventBus 事件总线接口
type EventBus interface {
	// Publish 发布事件（fire-and-forget — 失败时内部记录日志，不回传 error）
	Publish(event BusEvent)
	Subscribe(eventType EventType, handler EventHandler) Subscription
	SubscribeAll(handler EventHandler) Subscription
	Close() error
}

// Subscription 订阅句柄
type Subscription struct {
	ID        string
	EventType EventType
	Cancel    func()
}

// ChannelEventBus 基于 Channel 的事件总线实现
type ChannelEventBus struct {
	subscribers    map[EventType][]*subscriber
	allSubscribers []*subscriber
	mutex          sync.RWMutex
	eventChan      chan BusEvent
	criticalErrCh  chan HandlerError
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup

	// Observable counters (atomic)
	publishDropped  int64
	handlerTimeouts int64

	// PD-3: Throttling
	throttleMu       sync.RWMutex
	throttleConfigs  map[EventType]*throttleEntry
	publishThrottled int64 // atomic
}

type subscriber struct {
	id       string
	handler  EventHandler
	critical bool
}

type throttleEntry struct {
	lastAllowed time.Time
	minInterval time.Duration
}

// NewChannelEventBus 创建新的事件总线
func NewChannelEventBus(bufferSize int) *ChannelEventBus {
	ctx, cancel := context.WithCancel(context.Background())
	bus := &ChannelEventBus{
		subscribers:   make(map[EventType][]*subscriber),
		eventChan:     make(chan BusEvent, bufferSize),
		criticalErrCh: make(chan HandlerError, 10),
		ctx:           ctx,
		cancel:        cancel,
	}

	// 启动事件分发器
	bus.wg.Add(1)
	go bus.dispatcher()

	return bus
}

func (b *ChannelEventBus) Publish(event BusEvent) {
	// PD-3: Throttle check
	if !b.allowEvent(event.Type) {
		atomic.AddInt64(&b.publishThrottled, 1)
		return
	}

	select {
	case b.eventChan <- event:
		return
	case <-b.ctx.Done():
		atomic.AddInt64(&b.publishDropped, 1)
		logging.Warn("eventbus", "publish_dropped",
			logging.FStr("event_id", event.ID),
			logging.FStr("event_type", string(event.Type)),
			logging.FStr("reason", "bus_closed"))
	default:
		atomic.AddInt64(&b.publishDropped, 1)
		logging.Warn("eventbus", "publish_dropped",
			logging.FStr("event_id", event.ID),
			logging.FStr("event_type", string(event.Type)),
			logging.FStr("reason", "channel_full"))
	}
}

// allowEvent 檢查事件類型是否超過節流設定；若無設定則一律放行。
func (b *ChannelEventBus) allowEvent(et EventType) bool {
	b.throttleMu.Lock()
	defer b.throttleMu.Unlock()

	entry := b.throttleConfigs[et]
	if entry == nil {
		return true
	}

	if time.Since(entry.lastAllowed) < entry.minInterval {
		return false
	}

	entry.lastAllowed = time.Now()
	return true
}

// SetEventThrottle 設定每秒最大事件數；maxPerSecond <= 0 時視為不限制。
func (b *ChannelEventBus) SetEventThrottle(eventType EventType, maxPerSecond int) {
	b.throttleMu.Lock()
	defer b.throttleMu.Unlock()

	if b.throttleConfigs == nil {
		b.throttleConfigs = make(map[EventType]*throttleEntry)
	}

	minInterval := time.Duration(0)
	if maxPerSecond > 0 {
		minInterval = time.Second / time.Duration(maxPerSecond)
	}
	b.throttleConfigs[eventType] = &throttleEntry{minInterval: minInterval}
}

// PublishMarketSnapshot 发布市场快照事件（便捷方法）
func (b *ChannelEventBus) PublishMarketSnapshot(quote domain.Quote) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventMarketSnapshot,
		Timestamp: time.Now(),
		Payload: MarketEventPayload{
			Symbol:    quote.Symbol,
			Quote:     quote,
			Timestamp: time.Now(),
		},
		SchemaVersion: 1,
	})
}

// PublishSimulationStart 发布模擬開始事件
func (b *ChannelEventBus) PublishSimulationStart(sessionID string, asOf time.Time) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSimulationStart,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"session_id": sessionID,
			"as_of":      asOf.Format("2006-01-02"),
		},
		SchemaVersion: 1,
	})
}

// PublishSimulationComplete 发布模擬完成事件
func (b *ChannelEventBus) PublishSimulationComplete(sessionID string, portfolioValue float64, orderCount, positionCount int) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSimulationComplete,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"session_id":      sessionID,
			"portfolio_value": portfolioValue,
			"order_count":     orderCount,
			"position_count":  positionCount,
		},
		SchemaVersion: 1,
	})
}

// PublishRegimeChange 发布市场状态变更事件
func (b *ChannelEventBus) PublishRegimeChange(oldRegime, newRegime domain.Regime, confidence float64, determinedBy string) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventRegimeChange,
		Timestamp: time.Now(),
		Payload: RegimeEventPayload{
			OldRegime:    oldRegime,
			NewRegime:    newRegime,
			Confidence:   confidence,
			DeterminedBy: determinedBy,
		},
		SchemaVersion: 1,
	})
}

// PublishPositionUpdate 发布持仓更新事件
func (b *ChannelEventBus) PublishPositionUpdate(symbol string, position domain.Position, changeType string) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventPositionUpdate,
		Timestamp: time.Now(),
		Payload: PositionEventPayload{
			Symbol:     symbol,
			Position:   position,
			ChangeType: changeType,
		},
		SchemaVersion: 1,
	})
}

// PublishRecommendation 发布 Agent 推荐事件
func (b *ChannelEventBus) PublishRecommendation(agent string, recommendations []domain.Recommendation) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventAgentRecommendation,
		Timestamp: time.Now(),
		Payload: RecommendationEventPayload{
			Agent:           agent,
			Recommendations: recommendations,
		},
		SchemaVersion: 1,
	})
}

// PublishGuardOutcomes 发布控制层过滤结果事件
func (b *ChannelEventBus) PublishGuardOutcomes(sessionID string, outcomes []domain.GuardOutcome) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventGuardOutcome,
		Timestamp: time.Now(),
		Payload: GuardOutcomeEventPayload{
			SessionID: sessionID,
			Outcomes:  outcomes,
		},
		SchemaVersion: 1,
	})
}

// PublishDarwinianClamping 发布演化权重夹制事件
func (b *ChannelEventBus) PublishDarwinianClamping(events []ClampingEventPayload) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventDarwinianClamping,
		Timestamp: time.Now(),
		Payload: DarwinianClampingEventPayload{
			ClampingEvents: events,
		},
		SchemaVersion: 1,
	})
}

// PublishAgentHealthChange 发布 Agent 健康状态变更事件
func (b *ChannelEventBus) PublishAgentHealthChange(agentID, oldStatus, newStatus, reason string) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventAgentHealthChange,
		Timestamp: time.Now(),
		Payload: AgentHealthChangeEventPayload{
			AgentID:   agentID,
			OldStatus: oldStatus,
			NewStatus: newStatus,
			Reason:    reason,
			Timestamp: time.Now(),
		},
		SchemaVersion: 1,
	})
}

// PublishConvictionClamping 发布 Conviction 夹制事件
func (b *ChannelEventBus) PublishConvictionClamping(events []ConvictionClampingEventPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventConvictionClamping,
		Timestamp:     time.Now(),
		Payload:       events,
		SchemaVersion: 1,
	})
}

func (b *ChannelEventBus) PublishOrderEvent(order domain.Order, orderID, status string, fillPrice float64) {
	payload := OrderEventPayload{
		OrderID: orderID,
		Order:   order,
		Status:  status,
	}
	if status == "filled" {
		payload.FillPrice = fillPrice
		payload.FillTime = time.Now()
	}

	var eventType EventType
	switch status {
	case "filled":
		eventType = EventOrderFilled
	case "rejected":
		eventType = EventOrderRejected
	default:
		eventType = EventOrderPlaced
	}

	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          eventType,
		Timestamp:     time.Now(),
		Payload:       payload,
		SchemaVersion: 1,
	})
}

// PublishRiskEvent 发布风险事件
func (b *ChannelEventBus) PublishRiskEvent(eventType EventType, symbol string, position domain.Position, triggerType string, triggerPrice float64) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
		Payload: RiskEventPayload{
			Symbol:       symbol,
			Position:     position,
			TriggerType:  triggerType,
			TriggerPrice: triggerPrice,
		},
		SchemaVersion: 1,
	})
}

// PublishExperimentInsufficientData 发布实验数据不足事件
func (b *ChannelEventBus) PublishExperimentInsufficientData(experimentID string, baselineObs, candidateObs, requiredObs int, maturityLevel string, usedFallback bool) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventExperimentInsufficientData,
		Timestamp: time.Now(),
		Payload: ExperimentInsufficientDataEventPayload{
			ExperimentID:  experimentID,
			BaselineObs:   baselineObs,
			CandidateObs:  candidateObs,
			RequiredObs:   requiredObs,
			MaturityLevel: maturityLevel,
			UsedFallback:  usedFallback,
		},
		SchemaVersion: 1,
	})
}

// PublishExperimentLifecycle publishes an experiment lifecycle event
// (Accepted or Rejected). Fired by internal/experiment/lifecycle_publisher.go
// when an experiment status transitions to Accepted (severity "info")
// or Rejected (severity "error"). The caller specifies the event type
// and severity so this single method serves both lifecycle transitions.
func (b *ChannelEventBus) PublishExperimentLifecycle(eventType EventType, payload ExperimentLifecyclePayload, severity string) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          eventType,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      severity,
		SchemaVersion: 1,
	})
}

// PublishBackgroundTaskSustainedFailure publishes when a background task
// has failed consecutively >= threshold times. Decision 4
// (alert-redesign-v2.md Part 3.2): 1-2 failures are transient, threshold+
// failures are systematic and worth paging.
func (b *ChannelEventBus) PublishBackgroundTaskSustainedFailure(payload BackgroundTaskPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventBackgroundTaskSustainedFailure,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "error",
		SchemaVersion: 1,
	})
}

// PublishBackgroundTaskRecovered publishes when a background task that
// previously hit the threshold has succeeded — auto-resolve signal.
func (b *ChannelEventBus) PublishBackgroundTaskRecovered(payload BackgroundTaskPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventBackgroundTaskRecovered,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "info",
		SchemaVersion: 1,
	})
}

// PublishOrderError 发布订单错误事件
func (b *ChannelEventBus) PublishOrderError(orderID, symbol, side string, price float64, quantity int, errorCode, errorMessage string, attempts int, lastStatus string) {
	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventOrderError,
		Timestamp: time.Now(),
		Payload: OrderErrorEventPayload{
			OrderID:      orderID,
			Symbol:       symbol,
			Side:         side,
			Price:        price,
			Quantity:     quantity,
			ErrorCode:    errorCode,
			ErrorMessage: errorMessage,
			Attempts:     attempts,
			LastStatus:   lastStatus,
			Timestamp:    time.Now(),
		},
		SchemaVersion: 1,
	})
}

// PublishNarrativeEvent 发布叙事事件 (MacroIngestor 生成)
func (b *ChannelEventBus) PublishNarrativeEvent(eventID, theme, region string, sentiment, confidence float64, confidenceSource, hitRate, capitalFlow, timeWindow, explanation, sentimentExplanation string) {
	sentimentText := "中立"
	if sentiment > 0.3 {
		sentimentText = "利多"
	} else if sentiment < -0.3 {
		sentimentText = "利空"
	}

	themeDescriptions := map[string]string{
		"US_rates_up":                     "美國公債殖利率上升，可能引發資金流向調整",
		"JPY_carry_unwind":                "日圓套利平倉，顯示全球流動性收緊",
		"geopolitical_risk_spike":         "地緣政治風險攀升，市場避險情緒升溫",
		"oil_price_shock":                 "油價劇烈波動，影響通膨預期",
		"USD_TWD_volatility":              "美元兌台幣波動，反映台灣出口競爭力變化",
		"semiconductor_downturn":          "半導體出口下滑，景氣放緩訊號",
		"AI_capex_surge":                  "AI資本支出強勁，科技股展望正面",
		"retail_frenzy":                   "散戶融資餘額飆升，市場過熱風險",
		"retail_fear":                     "散戶融資餘額低迷，市場情緒低迷",
		"retail_institutional_divergence": "散戶與法人方向分歧，可能出現轉向",
	}

	description := themeDescriptions[theme]
	if description == "" {
		description = fmt.Sprintf("%s 區域發生 %s 事件，%s 信號", region, theme, sentimentText)
	}

	b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventNarrative,
		Timestamp: time.Now(),
		Payload: NarrativeEventPayload{
			EventID:              eventID,
			Theme:                theme,
			Region:               region,
			Sentiment:            sentiment,
			SentimentText:        sentimentText,
			Confidence:           confidence,
			ConfidenceSource:     confidenceSource,
			HitRate:              parseFloat(hitRate),
			CapitalFlow:          capitalFlow,
			TimeWindow:           timeWindow,
			Description:          description,
			Explanation:          explanation,
			SentimentExplanation: sentimentExplanation,
		},
		SchemaVersion: 1,
	})
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// PublishHealthAlert publishes a system health alert to the event bus.
func (b *ChannelEventBus) PublishHealthAlert(alert HealthAlertPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventHealthAlert,
		Timestamp:     time.Now(),
		Payload:       alert,
		Severity:      alert.Severity,
		SchemaVersion: 1,
	})
}

// PublishRiskGateEvent publishes a risk gate decision to the event bus.
//
// Auto-routing logic (Wave 8.2):
//   - BLOCK / HALT    → EventRiskGateRejected   (阻擋類)
//   - REDUCE / ALERT_ONLY → EventRiskGateOverridden (覆寫路徑)
//   - ALLOW           → EventRiskGateAllowed    (純通過)
//
// The three-way split preserves the semantic distinction between "fully allowed",
// "modified after override" (e.g. partial reduction or alert-only warning), and
// "blocked entirely". Frontend subscribers can render each category with its own
// badge color without parsing payload.Verdict themselves.
func (b *ChannelEventBus) PublishRiskGateEvent(payload RiskGateEventPayload) {
	eventType := EventRiskGateAllowed
	switch payload.Verdict {
	case "BLOCK", "HALT":
		eventType = EventRiskGateRejected
	case "REDUCE", "ALERT_ONLY":
		eventType = EventRiskGateOverridden
	}
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          eventType,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "warning",
		SchemaVersion: 1,
	})
}

// PublishIndustryCalendarEvent publishes a Taiwan market calendar event to the event bus.
func (b *ChannelEventBus) PublishIndustryCalendarEvent(payload IndustryCalendarEventPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventIndustryCalendar,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "info",
		SchemaVersion: 1,
	})
}

// PublishBacktestCompleted publishes an auto-backtest completion event.
// Fired by internal/autobacktest.Runner after RunAndStore succeeds and live store is synced.
func (b *ChannelEventBus) PublishBacktestCompleted(payload BacktestCompletedEventPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventBacktestCompleted,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "info",
		SchemaVersion: 1,
	})
}

// PublishCalibrationCompleted publishes a parameter calibration completion event.
// Fired by cmd/atlas/main.go linkage_calibrate task after CalibrateParameters succeeds.
func (b *ChannelEventBus) PublishCalibrationCompleted(payload CalibrationCompletedEventPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventCalibrationCompleted,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "info",
		SchemaVersion: 1,
	})
}

// PublishTradeSlippage publishes a per-order-fill slippage event.
// Fired by internal/live/order_manager.go on every order fill (status == "filled").
func (b *ChannelEventBus) PublishTradeSlippage(payload TradeSlippageEventPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventTradeSlippage,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "info",
		SchemaVersion: 1,
	})
}

// PublishDrawdownBreach publishes a portfolio drawdown breach event.
// Fired by internal/portfolio/risk_manager.go when currentDrawdown exceeds
// maxDrawdownPct. Severity is "critical" because drawdown breaches represent
// active capital loss requiring immediate attention.
func (b *ChannelEventBus) PublishDrawdownBreach(payload DrawdownBreachPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventDrawdownBreach,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "critical",
		SchemaVersion: 1,
	})
}

// PublishConcentrationBreach publishes a portfolio concentration breach
// event. Fired by internal/risk/concentration_alert_emitter.go when
// per-position weight, positions count, or sector weight exceeds the
// configured threshold. Severity is determined by the caller via the
// BusEvent.Severity field (position/sector breach → "error", count
// breach → "warning") — the payload Type field tells the consumer which
// metric was breached.
func (b *ChannelEventBus) PublishConcentrationBreach(payload ConcentrationBreachPayload, severity string) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventConcentrationBreach,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      severity,
		SchemaVersion: 1,
	})
}

func (b *ChannelEventBus) PublishPromotionRecorded(payload PromotionRecordedPayload) {
	b.Publish(BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          EventPromotionRecorded,
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "info",
		SchemaVersion: 1,
	})
}

// Subscribe 订阅特定类型事件
func (b *ChannelEventBus) Subscribe(eventType EventType, handler EventHandler) Subscription {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	sub := &subscriber{id: id, handler: handler}

	b.subscribers[eventType] = append(b.subscribers[eventType], sub)

	return Subscription{
		ID:        id,
		EventType: eventType,
		Cancel: func() {
			b.unsubscribe(eventType, id)
		},
	}
}

// SubscribeAll 订阅所有事件
func (b *ChannelEventBus) SubscribeAll(handler EventHandler) Subscription {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	id := fmt.Sprintf("sub-all-%d", time.Now().UnixNano())
	sub := &subscriber{id: id, handler: handler}

	b.allSubscribers = append(b.allSubscribers, sub)

	return Subscription{
		ID:        id,
		EventType: "",
		Cancel: func() {
			b.unsubscribeAll(id)
		},
	}
}

func (b *ChannelEventBus) SubscribeCritical(eventType EventType, handler EventHandler) (Subscription, <-chan error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	id := fmt.Sprintf("sub-crit-%d", time.Now().UnixNano())
	sub := &subscriber{id: id, handler: handler, critical: true}

	b.subscribers[eventType] = append(b.subscribers[eventType], sub)

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		for he := range b.criticalErrCh {
			if he.SubscriberID == id {
				errCh <- he.Err
			}
		}
	}()

	return Subscription{
		ID:        id,
		EventType: eventType,
		Cancel: func() {
			b.unsubscribe(eventType, id)
		},
	}, errCh
}

// dispatcher 事件分发器协程
func (b *ChannelEventBus) dispatcher() {
	defer b.wg.Done()

	for {
		select {
		case event := <-b.eventChan:
			b.dispatch(event)
		case <-b.ctx.Done():
			return
		}
	}
}

// dispatch 分发单个事件
func (b *ChannelEventBus) dispatch(event BusEvent) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	// 分发给特定类型订阅者
	if subs, ok := b.subscribers[event.Type]; ok {
		for _, sub := range subs {
			go b.handleEvent(sub, event)
		}
	}

	// 分发给所有事件订阅者
	for _, sub := range b.allSubscribers {
		go b.handleEvent(sub, event)
	}
}

func (b *ChannelEventBus) handleEvent(sub *subscriber, event BusEvent) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("eventbus", "handler_panic", "subscriber_id", sub.id, "panic", r)
		}
	}()

	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := sub.handler(ctx, event); err != nil {
			logging.Error("eventbus", "handler_error", "subscriber_id", sub.id, logging.Err(err))

			if sub.critical {
				select {
				case b.criticalErrCh <- HandlerError{
					EventType:    event.Type,
					SubscriberID: sub.id,
					Err:          err,
				}:
				default:
					logging.Error("eventbus", "critical_err_ch_full", "subscriber_id", sub.id)
				}
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
		atomic.AddInt64(&b.handlerTimeouts, 1)
		logging.Error("eventbus", "handler_timeout", "subscriber_id", sub.id)
	}
}

// unsubscribe 取消订阅
func (b *ChannelEventBus) unsubscribe(eventType EventType, id string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if subs, ok := b.subscribers[eventType]; ok {
		newSubs := make([]*subscriber, 0, len(subs))
		for _, sub := range subs {
			if sub.id != id {
				newSubs = append(newSubs, sub)
			}
		}
		b.subscribers[eventType] = newSubs
	}
}

// unsubscribeAll 取消全事件订阅
func (b *ChannelEventBus) unsubscribeAll(id string) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	newSubs := make([]*subscriber, 0, len(b.allSubscribers))
	for _, sub := range b.allSubscribers {
		if sub.id != id {
			newSubs = append(newSubs, sub)
		}
	}
	b.allSubscribers = newSubs
}

// Close 关闭事件总线
func (b *ChannelEventBus) Close() error {
	b.cancel()
	close(b.eventChan)
	b.wg.Wait()
	close(b.criticalErrCh)
	return nil
}

// Stats 获取统计信息
func (b *ChannelEventBus) Stats() map[string]any {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	stats := make(map[string]any)
	subscriberCount := len(b.allSubscribers)
	for _, subs := range b.subscribers {
		subscriberCount += len(subs)
	}

	stats["subscribers_total"] = subscriberCount
	stats["subscribers_by_type"] = len(b.subscribers)
	stats["channel_capacity"] = cap(b.eventChan)
	stats["channel_length"] = len(b.eventChan)
	stats["publish_dropped"] = atomic.LoadInt64(&b.publishDropped)
	stats["publish_throttled"] = atomic.LoadInt64(&b.publishThrottled)
	stats["handler_timeouts"] = atomic.LoadInt64(&b.handlerTimeouts)

	return stats
}
