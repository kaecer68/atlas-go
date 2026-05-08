package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// EventType 事件类型
type EventType string

const (
	// 市场数据事件
	EventMarketSnapshot EventType = "market.snapshot"
	EventMarketTick     EventType = "market.tick"
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
	EventSystemStart EventType = "system.start"
	EventSystemError EventType = "system.error"

	// 实验事件
	EventExperimentInsufficientData EventType = "experiment.insufficient_data"

	// 订单错误事件
	EventOrderError EventType = "order.error"
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

// BusEvent 总线事件
type BusEvent struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

// EventHandler 事件处理器函数类型
type EventHandler func(ctx context.Context, event BusEvent) error

// EventBus 事件总线接口
type EventBus interface {
	Publish(event BusEvent) error
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
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

type subscriber struct {
	id      string
	handler EventHandler
}

// NewChannelEventBus 创建新的事件总线
func NewChannelEventBus(bufferSize int) *ChannelEventBus {
	ctx, cancel := context.WithCancel(context.Background())
	bus := &ChannelEventBus{
		subscribers: make(map[EventType][]*subscriber),
		eventChan:   make(chan BusEvent, bufferSize),
		ctx:         ctx,
		cancel:      cancel,
	}

	// 启动事件分发器
	bus.wg.Add(1)
	go bus.dispatcher()

	return bus
}

// Publish 发布事件
func (b *ChannelEventBus) Publish(event BusEvent) error {
	select {
	case b.eventChan <- event:
		return nil
	case <-b.ctx.Done():
		return fmt.Errorf("event bus closed")
	default:
		return fmt.Errorf("event channel full")
	}
}

// PublishMarketSnapshot 发布市场快照事件（便捷方法）
func (b *ChannelEventBus) PublishMarketSnapshot(quote domain.Quote) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventMarketSnapshot,
		Timestamp: time.Now(),
		Payload: MarketEventPayload{
			Symbol:    quote.Symbol,
			Quote:     quote,
			Timestamp: time.Now(),
		},
	})
}

// PublishRegimeChange 发布市场状态变更事件
func (b *ChannelEventBus) PublishRegimeChange(oldRegime, newRegime domain.Regime, confidence float64, determinedBy string) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventRegimeChange,
		Timestamp: time.Now(),
		Payload: RegimeEventPayload{
			OldRegime:    oldRegime,
			NewRegime:    newRegime,
			Confidence:   confidence,
			DeterminedBy: determinedBy,
		},
	})
}

// PublishPositionUpdate 发布持仓更新事件
func (b *ChannelEventBus) PublishPositionUpdate(symbol string, position domain.Position, changeType string) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventPositionUpdate,
		Timestamp: time.Now(),
		Payload: PositionEventPayload{
			Symbol:     symbol,
			Position:   position,
			ChangeType: changeType,
		},
	})
}

// PublishRecommendation 发布 Agent 推荐事件
func (b *ChannelEventBus) PublishRecommendation(agent string, recommendations []domain.Recommendation) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventAgentRecommendation,
		Timestamp: time.Now(),
		Payload: RecommendationEventPayload{
			Agent:           agent,
			Recommendations: recommendations,
		},
	})
}

// PublishGuardOutcomes 发布控制层过滤结果事件
func (b *ChannelEventBus) PublishGuardOutcomes(sessionID string, outcomes []domain.GuardOutcome) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventGuardOutcome,
		Timestamp: time.Now(),
		Payload: GuardOutcomeEventPayload{
			SessionID: sessionID,
			Outcomes:  outcomes,
		},
	})
}

// PublishDarwinianClamping 发布演化权重夹制事件
func (b *ChannelEventBus) PublishDarwinianClamping(events []ClampingEventPayload) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventDarwinianClamping,
		Timestamp: time.Now(),
		Payload: DarwinianClampingEventPayload{
			ClampingEvents: events,
		},
	})
}

// PublishAgentHealthChange 发布 Agent 健康状态变更事件
func (b *ChannelEventBus) PublishAgentHealthChange(agentID, oldStatus, newStatus, reason string) error {
	return b.Publish(BusEvent{
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
	})
}

// PublishConvictionClamping 发布 Conviction 夹制事件
func (b *ChannelEventBus) PublishConvictionClamping(events []ConvictionClampingEventPayload) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventConvictionClamping,
		Timestamp: time.Now(),
		Payload:   events,
	})
}

// PublishOrderEvent 发布订单事件
func (b *ChannelEventBus) PublishOrderEvent(order domain.Order, orderID, status string, fillPrice float64) error {
	payload := OrderEventPayload{
		OrderID: orderID,
		Order:   order,
		Status:  status,
	}
	if status == "filled" {
		payload.FillPrice = fillPrice
		payload.FillTime = time.Now()
	}

	eventType := EventOrderPlaced
	if status == "filled" {
		eventType = EventOrderFilled
	} else if status == "rejected" {
		eventType = EventOrderRejected
	}

	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// PublishRiskEvent 发布风险事件
func (b *ChannelEventBus) PublishRiskEvent(eventType EventType, symbol string, position domain.Position, triggerType string, triggerPrice float64) error {
	return b.Publish(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
		Payload: RiskEventPayload{
			Symbol:       symbol,
			Position:     position,
			TriggerType:  triggerType,
			TriggerPrice: triggerPrice,
		},
	})
}

// PublishExperimentInsufficientData 发布实验数据不足事件
func (b *ChannelEventBus) PublishExperimentInsufficientData(experimentID string, baselineObs, candidateObs, requiredObs int, maturityLevel string, usedFallback bool) error {
	return b.Publish(BusEvent{
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
	})
}

// PublishOrderError 发布订单错误事件
func (b *ChannelEventBus) PublishOrderError(orderID, symbol, side string, price float64, quantity int, errorCode, errorMessage string, attempts int, lastStatus string) error {
	return b.Publish(BusEvent{
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

// handleEvent 处理单个事件
func (b *ChannelEventBus) handleEvent(sub *subscriber, event BusEvent) {
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	if err := sub.handler(ctx, event); err != nil {
		// 记录错误但不中断其他处理器
		fmt.Printf("[EventBus] Handler error for %s: %v\n", sub.id, err)
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

	return stats
}
