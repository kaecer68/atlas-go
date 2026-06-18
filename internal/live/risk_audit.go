package live

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const riskGateBlockedCode = "risk_gate_blocked"

// RiskAuditEntry records a single order that was blocked by the risk gate.
type RiskAuditEntry struct {
	OrderID      string
	Symbol       string
	Side         string
	Price        float64
	Quantity     int
	Attempts     int
	ErrorMessage string
	Timestamp    time.Time
}

// RiskAuditLog is an in-memory audit log of risk-gate-blocked orders. It
// subscribes to the channel event bus and records every EventOrderError with
// ErrorCode == "risk_gate_blocked". Operators inspect Entries() after a
// trading session to review which orders were refused and why.
type RiskAuditLog struct {
	mu      sync.RWMutex
	entries []RiskAuditEntry
}

func NewRiskAuditLog() *RiskAuditLog {
	return &RiskAuditLog{entries: make([]RiskAuditEntry, 0)}
}

// Subscribe registers the audit log as a subscriber on the given event bus.
// The returned cancel function detaches the subscription when called.
func (l *RiskAuditLog) Subscribe(bus *ChannelEventBus) eventbus.Subscription {
	if bus == nil {
		return eventbus.Subscription{}
	}
	sub := bus.SubscribeAll(l.handle)
	return sub
}

func (l *RiskAuditLog) handle(_ context.Context, event eventbus.BusEvent) error {
	if event.Type != eventbus.EventOrderError {
		return nil
	}
	payload, ok := event.Payload.(eventbus.OrderErrorEventPayload)
	if !ok {
		return nil
	}
	if payload.ErrorCode != riskGateBlockedCode {
		return nil
	}
	entry := RiskAuditEntry{
		OrderID:      payload.OrderID,
		Symbol:       payload.Symbol,
		Side:         payload.Side,
		Price:        payload.Price,
		Quantity:     payload.Quantity,
		Attempts:     payload.Attempts,
		ErrorMessage: payload.ErrorMessage,
		Timestamp:    payload.Timestamp,
	}
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	l.mu.Unlock()
	return nil
}

// Entries returns a snapshot copy of all recorded audit entries.
func (l *RiskAuditLog) Entries() []RiskAuditEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]RiskAuditEntry, len(l.entries))
	copy(out, l.entries)
	return out
}
