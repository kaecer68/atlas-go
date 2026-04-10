package live

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// OrderManager 负责下单重试与事件稽核。
type OrderManager struct {
	broker       Broker
	eventBus     *ChannelEventBus
	maxRetries   int
	retryBackoff time.Duration
}

func NewOrderManager(broker Broker, eventBus *ChannelEventBus, maxRetries int, retryBackoff time.Duration) *OrderManager {
	if broker == nil {
		broker = NewDryRunBroker()
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	if retryBackoff < 0 {
		retryBackoff = 0
	}

	return &OrderManager{
		broker:       broker,
		eventBus:     eventBus,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
	}
}

func (m *OrderManager) Mode() string {
	if m.broker == nil {
		return "dry-run"
	}
	return m.broker.Mode()
}

func (m *OrderManager) Execute(ctx context.Context, order domain.Order) error {
	if m.broker == nil {
		m.broker = NewDryRunBroker()
	}

	attempts := m.maxRetries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := m.broker.SubmitOrder(ctx, order)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d/%d: %w", attempt, attempts, err)
			if attempt < attempts && m.retryBackoff > 0 {
				time.Sleep(m.retryBackoff)
			}
			continue
		}

		status := result.Status
		if status == "" {
			status = "placed"
		}

		eventOrder := order
		if status == "rejected" && result.Reason != "" {
			eventOrder.Reason = result.Reason
		}

		if m.eventBus != nil {
			if err := m.eventBus.PublishOrderEvent(eventOrder, result.OrderID, status, result.FillPrice); err != nil {
				return fmt.Errorf("publish order event: %w", err)
			}
		}

		if status == "rejected" {
			if result.Reason == "" {
				result.Reason = "broker rejected order"
			}
			return fmt.Errorf("broker rejected order %s: %s", order.Symbol, result.Reason)
		}

		return nil
	}

	if m.eventBus != nil {
		_ = m.eventBus.Publish(BusEvent{
			ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			Type:      EventSystemError,
			Timestamp: time.Now(),
			Payload: map[string]string{
				"error": fmt.Sprintf("submit order %s failed after %d attempts: %v", order.Symbol, attempts, lastErr),
			},
		})
	}

	return fmt.Errorf("submit order %s failed after %d attempts: %w", order.Symbol, attempts, lastErr)
}
