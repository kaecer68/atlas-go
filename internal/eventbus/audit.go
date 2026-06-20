// Package eventbus — PD-2: JSONL audit trail subscriber for all bus events.
package eventbus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// AuditTrailSubscriber subscribes to every event on a ChannelEventBus and
// appends each event as a single JSON line to {ledgerDir}/events.jsonl.
// It can be enabled/disabled at runtime via atomic flags.
type AuditTrailSubscriber struct {
	bus       *ChannelEventBus
	ledgerDir string

	enabled   atomic.Bool
	persistMu sync.Mutex
}

// NewAuditTrailSubscriber creates a new global audit subscriber and registers
// it with SubscribeAll on the provided bus.
func NewAuditTrailSubscriber(bus *ChannelEventBus, ledgerDir string) *AuditTrailSubscriber {
	a := &AuditTrailSubscriber{
		bus:       bus,
		ledgerDir: ledgerDir,
	}
	bus.SubscribeAll(a.handle)
	return a
}

// Enable turns event persistence on.
func (a *AuditTrailSubscriber) Enable() {
	a.enabled.Store(true)
}

// Disable turns event persistence off.
func (a *AuditTrailSubscriber) Disable() {
	a.enabled.Store(false)
}

func (a *AuditTrailSubscriber) handle(_ context.Context, ev BusEvent) error {
	if !a.enabled.Load() {
		return nil
	}
	a.persistEvent(ev)
	return nil
}

func (a *AuditTrailSubscriber) persistEvent(ev BusEvent) {
	path := filepath.Join(a.ledgerDir, "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logging.Warn("eventbus_audit", "mkdir_failed",
			logging.FStr("path", filepath.Dir(path)),
			logging.Err(err))
		return
	}

	persisted := ev
	EnrichEvent(&persisted)

	a.persistMu.Lock()
	defer a.persistMu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logging.Warn("eventbus_audit", "open_failed",
			logging.FStr("path", path),
			logging.Err(err))
		return
	}
	defer func() { _ = f.Close() }()

	if err := json.NewEncoder(f).Encode(persisted); err != nil {
		logging.Warn("eventbus_audit", "encode_failed",
			logging.FStr("path", path),
			logging.Err(err))
	}
}
