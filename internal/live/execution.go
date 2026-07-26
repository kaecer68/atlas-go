package live

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type ExecutionManager struct {
	broker         Broker
	orderMgr       *OrderManager
	circuitBreaker CircuitBreakerOps
	config         OrchestratorConfig
	metrics        MetricsRecorder
	eventBus       *ChannelEventBus
}

func NewExecutionManager(
	broker Broker,
	cb CircuitBreakerOps,
	config OrchestratorConfig,
	metrics MetricsRecorder,
) *ExecutionManager {
	return &ExecutionManager{
		broker:         broker,
		circuitBreaker: cb,
		config:         config,
		metrics:        metrics,
	}
}

func (e *ExecutionManager) SetEventBus(eb *ChannelEventBus) {
	e.eventBus = eb
}

func (e *ExecutionManager) SimulateExecution() {
	if e.broker == nil {
		e.broker = NewDryRunBroker()
	}
	if e.orderMgr == nil {
		retries := max(e.config.BrokerMaxRetries, 0)
		e.orderMgr = NewOrderManager(e.broker, e.eventBus, retries, 100*time.Millisecond, nil)
	}
	logging.Info("trading", "execution_channel_ready", "mode", e.orderMgr.Mode())
	if e.metrics != nil {
		e.metrics.RecordCounter("execution_cycles_total", 1, map[string]string{
			"broker_mode": e.broker.Mode(),
		})
	}
}

func (e *ExecutionManager) ExecuteOrder(ctx context.Context, order domain.Order) error {
	if e.circuitBreaker == nil {
		return fmt.Errorf("execution manager: circuit breaker not initialized")
	}
	if !e.circuitBreaker.CanPlaceOrder(order.Side) {
		if e.metrics != nil {
			e.metrics.RecordCounter("orders_blocked_total", 1, map[string]string{
				"symbol": order.Symbol,
				"side":   string(order.Side),
				"reason": string(e.circuitBreaker.State()),
			})
		}
		return fmt.Errorf("circuit breaker blocks %s order for %s (state=%s)", order.Side, order.Symbol, e.circuitBreaker.State())
	}
	if e.orderMgr == nil {
		if e.broker == nil {
			e.broker = NewDryRunBroker()
		}
		retries := max(e.config.BrokerMaxRetries, 0)
		e.orderMgr = NewOrderManager(e.broker, e.eventBus, retries, 100*time.Millisecond, nil)
	}
	if err := e.orderMgr.Run(ctx, order); err != nil {
		if e.metrics != nil {
			e.metrics.RecordCounter("orders_failed_total", 1, map[string]string{
				"symbol": order.Symbol,
				"side":   string(order.Side),
			})
		}
		return fmt.Errorf("execute order via manager: %w", err)
	}
	if e.metrics != nil {
		e.metrics.RecordOrder(order, "submitted")
	}
	return nil
}
