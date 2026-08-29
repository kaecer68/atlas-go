package live

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// OrderEvent records a single event in an order's lifecycle.
type OrderEvent struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	FillPrice float64   `json:"fill_price,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// OrderRecord is a complete order record with lifecycle events.
type OrderRecord struct {
	OrderID    string       `json:"order_id"`
	Symbol     string       `json:"symbol"`
	Side       string       `json:"side"`
	Quantity   int          `json:"quantity"`
	Price      float64      `json:"price"`
	Status     string       `json:"status"`
	CreatedAt  time.Time    `json:"created_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	BrokerMode string       `json:"broker_mode"`
	FillPrice  float64      `json:"fill_price,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Events     []OrderEvent `json:"events"`
}

// OrderFilter filters the order list.
type OrderFilter struct {
	Status   string
	Symbol   string
	Side     string
	DateFrom time.Time
	DateTo   time.Time
	Page     int
	PageSize int
}

// OrderManager 负责下单重试与事件稽核。
type OrderManager struct {
	broker       Broker
	eventBus     *ChannelEventBus
	maxRetries   int
	retryBackoff time.Duration
	riskGate     *RiskGate

	mu               sync.RWMutex
	orders           map[string]OrderRecord
	portfolioValueFn func() float64
}

func NewOrderManager(broker Broker, eventBus *ChannelEventBus, maxRetries int, retryBackoff time.Duration, riskGate *RiskGate) *OrderManager {
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
		riskGate:     riskGate,
		orders:       make(map[string]OrderRecord),
	}
}

func (m *OrderManager) Mode() string {
	if m.broker == nil {
		return "dry-run"
	}
	return m.broker.Mode()
}

func (m *OrderManager) RecordOrder(order OrderRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[order.OrderID] = order
}

// SetPortfolioValueProvider registers a function that returns the current
// portfolio notional. The OrderManager calls it on each fill to compute the
// loss fraction reported to the RiskGate.
func (m *OrderManager) SetPortfolioValueProvider(fn func() float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.portfolioValueFn = fn
}

func (m *OrderManager) recordFillToRiskGate(order domain.Order, result BrokerResult) {
	if m.riskGate == nil {
		return
	}
	m.mu.RLock()
	provider := m.portfolioValueFn
	m.mu.RUnlock()
	if provider == nil {
		return
	}
	m.riskGate.RecordFill(order.Side, order.Price, result.FillPrice, float64(order.Quantity), provider())
}

// checkRiskGate validates the order against the current risk gate state. It
// is invoked on every retry attempt so a halt triggered by a circuit
// transition or daily-loss breach between attempts will short-circuit the
// retry loop instead of leaking the order to the broker.
func (m *OrderManager) checkRiskGate(ctx context.Context, order domain.Order, attempt int) error {
	if m.riskGate == nil {
		return nil
	}
	if err := m.riskGate.Check(ctx, order); err != nil {
		if m.eventBus != nil {
			m.eventBus.PublishOrderError(
				"",
				order.Symbol,
				string(order.Side),
				order.Price,
				order.Quantity,
				"risk_gate_blocked",
				err.Error(),
				attempt,
				"blocked",
			)
		}
		return fmt.Errorf("risk gate blocked order for %s: %w", order.Symbol, err)
	}
	return nil
}

func (m *OrderManager) Run(ctx context.Context, order domain.Order) error {
	if m.broker == nil {
		m.broker = NewDryRunBroker()
	}

	attempts := m.maxRetries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := m.checkRiskGate(ctx, order, attempt); err != nil {
			return err
		}

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
			m.eventBus.PublishOrderEvent(eventOrder, result.OrderID, status, result.FillPrice)
		}

		if status == "filled" {
			m.recordFillToRiskGate(order, result)
			if m.eventBus != nil {
				slippageBPS := 0.0
				if order.Price > 0 {
					slippageBPS = math.Abs(result.FillPrice-order.Price) / order.Price * 10000
				}
				slippageCost := math.Abs(result.FillPrice-order.Price) * float64(order.Quantity)
				m.eventBus.PublishTradeSlippage(eventbus.TradeSlippageEventPayload{
					OrderID:       result.OrderID,
					Symbol:        order.Symbol,
					Side:          string(order.Side),
					Quantity:      order.Quantity,
					ExpectedPrice: order.Price,
					FillPrice:     result.FillPrice,
					SlippageBPS:   slippageBPS,
					SlippageCost:  slippageCost,
					BrokerMode:    m.Mode(),
					Timestamp:     time.Now(),
				})
			}
		}

		if status == "rejected" {
			if result.Reason == "" {
				result.Reason = "broker rejected order"
			}
			if m.eventBus != nil {
				m.eventBus.PublishOrderError(
					result.OrderID,
					order.Symbol,
					string(order.Side),
					order.Price,
					order.Quantity,
					"rejected",
					result.Reason,
					attempt,
					status,
				)
			}
			return fmt.Errorf("broker rejected order %s: %s", order.Symbol, result.Reason)
		}

		return nil
	}

	if m.eventBus != nil {
		m.eventBus.PublishOrderError(
			"",
			order.Symbol,
			string(order.Side),
			order.Price,
			order.Quantity,
			"retry_exhausted",
			lastErr.Error(),
			attempts,
			"error",
		)
	}

	return fmt.Errorf("submit order %s failed after %d attempts: %w", order.Symbol, attempts, lastErr)
}

func (m *OrderManager) GetOrders(filter OrderFilter) ([]OrderRecord, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	var all []OrderRecord
	for _, rec := range m.orders {
		if filter.Symbol != "" && !strings.EqualFold(rec.Symbol, filter.Symbol) {
			continue
		}
		if filter.Side != "" && !strings.EqualFold(rec.Side, filter.Side) {
			continue
		}
		if filter.Status != "" && !strings.EqualFold(rec.Status, filter.Status) {
			continue
		}
		if !filter.DateFrom.IsZero() && rec.CreatedAt.Before(filter.DateFrom) {
			continue
		}
		if !filter.DateTo.IsZero() && rec.CreatedAt.After(filter.DateTo) {
			continue
		}
		all = append(all, rec)
	}

	slices.SortFunc(all, func(a, b OrderRecord) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})

	total := len(all)
	start := (filter.Page - 1) * filter.PageSize
	if start >= total {
		return []OrderRecord{}, total, nil
	}
	end := min(start+filter.PageSize, total)
	return all[start:end], total, nil
}

func (m *OrderManager) GetOrder(orderID string) (*OrderRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if rec, ok := m.orders[orderID]; ok {
		return &rec, nil
	}
	return nil, fmt.Errorf("order not found: %s", orderID)
}
