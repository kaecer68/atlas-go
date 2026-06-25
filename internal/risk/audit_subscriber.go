package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type AuditSubscriber struct {
	bus       *eventbus.ChannelEventBus
	ledgerDir string

	mu        sync.RWMutex
	sessionID string
	persistMu sync.Mutex
}

// auditSubscribers is a process-wide registry keyed by bus pointer that
// prevents double-registration of an AuditSubscriber on the same bus.
// ChannelEventBus.Subscribe is a pure append, so a second call on the same
// bus would cause every risk event to be persisted to JSONL twice — an
// audit-log integrity violation for compliance-sensitive data.
var (
	auditSubscribersMu sync.Mutex
	auditSubscribers   = make(map[*eventbus.ChannelEventBus]*AuditSubscriber)
)

func NewAuditSubscriber(bus *eventbus.ChannelEventBus) *AuditSubscriber {
	auditSubscribersMu.Lock()
	defer auditSubscribersMu.Unlock()
	if existing, ok := auditSubscribers[bus]; ok {
		// Double-registration on the same bus would cause every risk event
		// to be logged + persisted twice. Return the existing subscriber
		// and surface the issue so the caller can fix the wiring.
		logging.Warn("risk", "audit_subscriber_already_registered",
			"detail", "returning existing subscriber; refactor caller to reuse it")
		return existing
	}
	a := &AuditSubscriber{
		bus:       bus,
		ledgerDir: config.Load().LedgerDir,
	}
	bus.Subscribe(eventbus.EventStopLossTriggered, a.log)
	bus.Subscribe(eventbus.EventTakeProfitTriggered, a.log)
	bus.Subscribe(eventbus.EventRiskAlert, a.log)
	bus.Subscribe(eventbus.EventOrderFilled, a.log)
	bus.Subscribe(eventbus.EventOrderRejected, a.log)
	bus.Subscribe(eventbus.EventOrderPlaced, a.log)
	auditSubscribers[bus] = a
	return a
}

func (a *AuditSubscriber) SetSessionID(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessionID = sessionID
}

func (a *AuditSubscriber) log(_ context.Context, ev eventbus.BusEvent) error {
	logging.Info(
		"risk_audit", "risk_event",
		logging.FStr("event_type", string(ev.Type)),
		logging.FStr("event_id", ev.ID),
		logging.FStr("payload", fmt.Sprintf("%+v", ev.Payload)),
	)
	if err := a.persistEvent(ev); err != nil {
		return fmt.Errorf("persist risk event: %w", err)
	}
	return nil
}

func (a *AuditSubscriber) persistEvent(ev eventbus.BusEvent) error {
	sessionID := a.getSessionID()
	if sessionID == "" {
		return nil
	}

	path := filepath.Join(a.ledgerDir, "sessions", sessionID, "risk_events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir session risk dir: %w", err)
	}

	persisted := ev
	eventbus.EnrichEvent(&persisted)

	a.persistMu.Lock()
	defer a.persistMu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open session risk events file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := json.NewEncoder(f).Encode(persisted); err != nil {
		return fmt.Errorf("encode session risk event: %w", err)
	}
	return nil
}

func (a *AuditSubscriber) getSessionID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionID
}
