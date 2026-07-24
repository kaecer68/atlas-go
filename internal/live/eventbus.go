package live

import (
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// Backward-compatible re-exports of the eventbus package.
// All live package consumers should migrate to internal/eventbus directly.

type (
	EventType                              = eventbus.EventType
	BusEvent                               = eventbus.BusEvent
	EventHandler                           = eventbus.EventHandler
	Subscription                           = eventbus.Subscription
	MarketEventPayload                     = eventbus.MarketEventPayload
	RegimeEventPayload                     = eventbus.RegimeEventPayload
	PositionEventPayload                   = eventbus.PositionEventPayload
	RecommendationEventPayload             = eventbus.RecommendationEventPayload
	GuardOutcomeEventPayload               = eventbus.GuardOutcomeEventPayload
	OrderEventPayload                      = eventbus.OrderEventPayload
	RiskEventPayload                       = eventbus.RiskEventPayload
	OrderErrorEventPayload                 = eventbus.OrderErrorEventPayload
	ExperimentInsufficientDataEventPayload = eventbus.ExperimentInsufficientDataEventPayload
	EventBus                               = eventbus.EventBus
	ChannelEventBus                        = eventbus.ChannelEventBus
)

const (
	EventMarketSnapshot             = eventbus.EventMarketSnapshot
	EventMarketOpen                 = eventbus.EventMarketOpen
	EventMarketClose                = eventbus.EventMarketClose
	EventRegimeChange               = eventbus.EventRegimeChange
	EventPositionUpdate             = eventbus.EventPositionUpdate
	EventPortfolioPnL               = eventbus.EventPortfolioPnL
	EventAgentRecommendation        = eventbus.EventAgentRecommendation
	EventAgentEvaluation            = eventbus.EventAgentEvaluation
	EventOrderPlaced                = eventbus.EventOrderPlaced
	EventOrderFilled                = eventbus.EventOrderFilled
	EventOrderRejected              = eventbus.EventOrderRejected
	EventOrderError                 = eventbus.EventOrderError
	EventStopLossTriggered          = eventbus.EventStopLossTriggered
	EventTakeProfitTriggered        = eventbus.EventTakeProfitTriggered
	EventRiskAlert                  = eventbus.EventRiskAlert
	EventGuardOutcome               = eventbus.EventGuardOutcome
	EventSystemStart                = eventbus.EventSystemStart
	EventSystemError                = eventbus.EventSystemError
	EventExperimentInsufficientData = eventbus.EventExperimentInsufficientData
	EventTradeSlippage              = eventbus.EventTradeSlippage
)

func NewChannelEventBus(bufferSize int) *ChannelEventBus {
	return eventbus.NewChannelEventBus(bufferSize)
}
