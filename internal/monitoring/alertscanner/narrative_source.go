package alertscanner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// narrativeAlertTTL is the dedup window for narrative theme alerts: the
// same theme firing again within this window updates LastSeen instead of
// creating a duplicate record.
const narrativeAlertTTL = 20 * time.Second

// narrativeCriticalThemes is the number of distinct themes firing within
// the dedup window that escalates to a CRITICAL alert (multi-theme
// narrative resonance — several macro narratives at once).
const narrativeCriticalThemes = 3

// NarrativeAlertSource surfaces narrative detector events as
// domain.AlertRecord through the unified MultiScanner. It subscribes to
// EventNarrative on the eventbus (published by the MacroIngestor when a
// macro narrative event is detected) and maintains a bounded buffer of
// recent theme alerts, deduplicated by theme within a TTL window.
//
// Severity mapping:
//   - Every detected narrative theme → WARNING alert
//     (rule "narrative_theme_detected", message includes the theme).
//   - When ≥ narrativeCriticalThemes distinct themes fire within the
//     dedup window, the newest record escalates to CRITICAL
//     (multi-theme resonance signals regime-level risk).
//
// Lifecycle: Start() must be called to begin consuming events.
// Stop() unsubscribes and drains the buffer.
type NarrativeAlertSource struct {
	bus eventbus.EventBus

	mu       sync.Mutex
	buffer   []domain.AlertRecord
	lastSeen map[string]time.Time // theme → last alert time
	subs     []eventbus.Subscription
	cap      int
}

// NewNarrativeSource creates a narrative alert source with the given
// buffer capacity. Non-positive cap defaults to 256.
func NewNarrativeSource(bus eventbus.EventBus, cap int) *NarrativeAlertSource {
	if cap <= 0 {
		cap = 256
	}
	return &NarrativeAlertSource{
		bus:      bus,
		buffer:   make([]domain.AlertRecord, 0, cap),
		lastSeen: make(map[string]time.Time),
		cap:      cap,
	}
}

func (n *NarrativeAlertSource) Name() string { return "narrative" }

// Start subscribes to narrative detection events on the eventbus.
// Returns an error if the bus is nil (no-op source).
func (n *NarrativeAlertSource) Start() error {
	if n.bus == nil {
		return nil // no-op: no eventbus means no narrative events
	}
	sub := n.bus.Subscribe(eventbus.EventNarrative, n.handleEvent)
	n.subs = append(n.subs, sub)
	return nil
}

// Stop unsubscribes all narrative event subscriptions. After Stop(),
// ListActive still returns buffered alerts but no new events arrive.
func (n *NarrativeAlertSource) Stop() {
	for _, sub := range n.subs {
		sub.Cancel()
	}
	n.subs = nil
}

// ListActive returns all buffered narrative alerts as a snapshot copy.
func (n *NarrativeAlertSource) ListActive(_ context.Context) ([]domain.AlertRecord, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.buffer) == 0 {
		return nil, nil
	}
	out := make([]domain.AlertRecord, len(n.buffer))
	copy(out, n.buffer)
	return out, nil
}

// handleEvent converts an incoming EventNarrative bus event into a
// domain.AlertRecord, deduplicated by theme within narrativeAlertTTL.
func (n *NarrativeAlertSource) handleEvent(_ context.Context, ev eventbus.BusEvent) error {
	payload, ok := ev.Payload.(eventbus.NarrativeEventPayload)
	if !ok || payload.Theme == "" {
		return nil
	}

	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	theme := payload.Theme
	dedupKey := "narrative:" + theme

	n.mu.Lock()
	defer n.mu.Unlock()

	// Dedup: same theme within TTL → update LastSeen on the existing record.
	nowTs := now
	for i := range n.buffer {
		rec := &n.buffer[i]
		if rec.DedupKey == dedupKey && rec.LastSeen != nil && nowTs.Sub(*rec.LastSeen) < narrativeAlertTTL {
			rec.Count++
			rec.LastSeen = &nowTs
			n.lastSeen[theme] = nowTs
			return nil
		}
	}

	severity := "warning"
	message := fmt.Sprintf("敘事主題 %s 觸發（嚴重度 %s）", theme, severity)
	// Multi-theme resonance: count distinct themes active within the TTL
	// window and escalate to CRITICAL when the threshold is reached.
	activeThemes := 1
	for t, last := range n.lastSeen {
		if t == theme {
			continue
		}
		if nowTs.Sub(last) < narrativeAlertTTL {
			activeThemes++
		}
	}
	if activeThemes >= narrativeCriticalThemes {
		severity = "critical"
		message = fmt.Sprintf("多個敘事主題共振觸發（%d themes）：%s", activeThemes, theme)
	}
	n.lastSeen[theme] = nowTs

	record := domain.AlertRecord{
		ID:        fmt.Sprintf("narrative-%s-%d", theme, now.UnixNano()),
		Timestamp: now,
		Rule:      "narrative_theme_detected",
		Severity:  severity,
		Message:   message,
		Status:    domain.AlertStatusTriggered,
		DedupKey:  dedupKey,
		FirstSeen: &nowTs,
		LastSeen:  &nowTs,
		Count:     1,
	}

	if len(n.buffer) >= n.cap {
		keep := n.cap / 2
		n.buffer = append(n.buffer[len(n.buffer)-keep:], record)
	} else {
		n.buffer = append(n.buffer, record)
	}
	return nil
}
