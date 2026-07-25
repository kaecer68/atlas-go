package alertscanner

import (
	"context"
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// wave9EventTypes lists the Wave 9 observability event types that
// should be surfaced as alerts. These are events that indicate
// actionable conditions (warnings or errors), not routine observations.
var wave9EventTypes = []eventbus.EventType{
	eventbus.EventChannelIndividualHealth,
	eventbus.EventIngestionLagSpike,
	eventbus.EventDriftDetected,
	eventbus.EventFactorWeightRegression,
}

// Wave9Source subscribes to Wave 9 observability events from the
// eventbus and exposes them as alert records through the AlertSource
// interface. It maintains a bounded ring buffer of recent events;
// old events are evicted when the buffer is full.
//
// Lifecycle: Start() must be called to begin consuming events.
// Stop() unsubscribes and drains the buffer. After Stop(), ListActive
// returns the remaining buffered events.
type Wave9Source struct {
	bus eventbus.EventBus

	mu     sync.Mutex
	buffer []domain.AlertRecord
	subs   []eventbus.Subscription
	cap    int
}

// NewWave9Source creates a Wave 9 alert source with the given buffer
// capacity. Non-positive cap defaults to 256.
func NewWave9Source(bus eventbus.EventBus, cap int) *Wave9Source {
	if cap <= 0 {
		cap = 256
	}
	return &Wave9Source{
		bus:    bus,
		buffer: make([]domain.AlertRecord, 0, cap),
		cap:    cap,
	}
}

func (w *Wave9Source) Name() string { return "wave9" }

// Start subscribes to Wave 9 event types on the eventbus. Each incoming
// event is converted to a domain.AlertRecord and appended to the buffer.
// Returns an error if the bus is nil (no-op source) or if any subscription fails.
func (w *Wave9Source) Start() error {
	if w.bus == nil {
		return nil // no-op: no eventbus means no Wave9 events
	}
	for _, evType := range wave9EventTypes {
		sub := w.bus.Subscribe(evType, w.handleEvent)
		w.subs = append(w.subs, sub)
	}
	return nil
}

// Stop unsubscribes all Wave 9 event subscriptions. After Stop(),
// ListActive still returns buffered events but no new events arrive.
func (w *Wave9Source) Stop() {
	for _, sub := range w.subs {
		sub.Cancel()
	}
	w.subs = nil
}

// ListActive returns all buffered Wave 9 alerts. Only events with
// severity "warning" or "critical" are included; "info" events are
// informational and should not gate workflows.
func (w *Wave9Source) ListActive(_ context.Context) ([]domain.AlertRecord, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buffer) == 0 {
		return nil, nil
	}
	// Return a snapshot copy to avoid concurrent mutation.
	out := make([]domain.AlertRecord, len(w.buffer))
	copy(out, w.buffer)
	return out, nil
}

// handleEvent converts an incoming BusEvent into a domain.AlertRecord
// and appends it to the ring buffer.
func (w *Wave9Source) handleEvent(_ context.Context, ev eventbus.BusEvent) error {
	// Only surface warnings and errors (not info).
	if ev.Severity != "warning" && ev.Severity != "error" && ev.Severity != "critical" {
		return nil
	}
	// Enrich the event if description/severity are not yet populated.
	eventbus.EnrichEvent(&ev)

	record := domain.AlertRecord{
		ID:        fmt.Sprintf("wave9-%s-%d", ev.Type, ev.Timestamp.UnixNano()),
		Timestamp: ev.Timestamp,
		Rule:      string(ev.Type),
		Severity:  ev.Severity,
		Message:   ev.Description,
		Status:    domain.AlertStatusTriggered,
		LastSeen:  &ev.Timestamp,
	}

	w.mu.Lock()
	if len(w.buffer) >= w.cap {
		// Evict oldest half when buffer is full.
		keep := w.cap / 2
		w.buffer = append(w.buffer[len(w.buffer)-keep:], record)
	} else {
		w.buffer = append(w.buffer, record)
	}
	w.mu.Unlock()
	return nil
}
