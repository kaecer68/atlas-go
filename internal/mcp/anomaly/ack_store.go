package anomaly

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrEmptyAnomaly is returned by AnomalyStore.Save when the event is missing
// both tenant_id and anomaly_type. Refusing blank events prevents a
// programming bug from polluting the alert dashboard.
var ErrEmptyAnomaly = errors.New("anomaly: event missing tenant_id and anomaly_type")

// ErrAnomalyNotFound is returned by AnomalyStore.Ack and AnomalyStore.Get when
// the requested AnomalyID is unknown to the store.
var ErrAnomalyNotFound = errors.New("anomaly: id not found")

// AnomalyStore is the durable + ackable persistence surface for anomaly
// events. It is distinct from the ring-buffer Store (which is for the
// mcp_anomaly_get_recent tool's recent-events view). Implementations MUST be
// safe for concurrent use and survive long enough to outlast the detector's
// own eviction (so a user can ack an anomaly minutes after it was raised).
type AnomalyStore interface {
	// Save stores an anomaly and returns the assigned AnomalyID-wrapped
	// record. The caller (server wiring) supplies the AnomalyEvent from
	// the detector; the store assigns the AnomalyID.
	Save(ev AnomalyEvent) (StoredAnomaly, error)
	// Get returns the record for a given AnomalyID, or (zero, false).
	Get(anomalyID string) (StoredAnomaly, bool)
	// Ack marks the anomaly as acknowledged by userID at the current UTC
	// time. Idempotent: re-acking a known ID overwrites the previous
	// AckedBy/AckedAt.
	Ack(anomalyID, userID string) error
	// ListUnacked returns the most recent unacked anomalies, newest first,
	// capped at limit. Used by operator dashboards.
	ListUnacked(limit int) []StoredAnomaly
	// ListAll returns the most recent anomalies regardless of ack state,
	// newest first, capped at limit. Used by audit / behavior review.
	ListAll(limit int) []StoredAnomaly
}

// StoredAnomaly is the persistable view of an AnomalyEvent, augmented with
// the storage-assigned AnomalyID and the ack trail.
type StoredAnomaly struct {
	AnomalyID string       `json:"anomaly_id"`
	Event     AnomalyEvent `json:"event"`
	Acked     bool         `json:"acked"`
	AckedBy   string       `json:"acked_by,omitempty"`
	AckedAt   time.Time    `json:"acked_at,omitzero"`
	CreatedAt time.Time    `json:"created_at"`
}

// MemoryStore is the in-memory AnomalyStore implementation used by the MCP
// server when SQLite persistence is disabled (T1.4 default). It is goroutine
// safe via sync.RWMutex and applies the same FIFO eviction policy as the
// ring-buffer Store so dashboards see consistent data.
type MemoryStore struct {
	mu     sync.RWMutex
	cap    int
	items  []StoredAnomaly
	byID   map[string]int // index into items
	nextID uint64         // monotonic counter for compact ids
	now    func() time.Time
}

// NewMemoryStore builds a MemoryStore with the given capacity. A capacity
// <= 0 defaults to 1000 (matches the ring-buffer Store default).
func NewMemoryStore(capacity int) *MemoryStore {
	if capacity <= 0 {
		capacity = 1000
	}
	return &MemoryStore{
		cap:  capacity,
		byID: make(map[string]int),
		now:  time.Now,
	}
}

// Save stores an AnomalyEvent. It rejects fully-blank events (neither
// tenant_id nor anomaly_type set) and assigns a 16-hex-char AnomalyID.
func (m *MemoryStore) Save(ev AnomalyEvent) (StoredAnomaly, error) {
	if ev.TenantID == "" && ev.AnomalyType == "" {
		return StoredAnomaly{}, ErrEmptyAnomaly
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now().UTC()
	id := m.allocateID()
	sa := StoredAnomaly{
		AnomalyID: id,
		Event:     ev,
		CreatedAt: now,
	}

	if len(m.items) >= m.cap {
		// Evict the oldest entry: drop index 0 from items + byID.
		evicted := m.items[0]
		delete(m.byID, evicted.AnomalyID)
		m.items = m.items[1:]
		// Rebuild remaining byID indexes (shift left by 1).
		for k, v := range m.byID {
			m.byID[k] = v - 1
		}
	}
	m.byID[id] = len(m.items)
	m.items = append(m.items, sa)
	return sa, nil
}

// Get returns the StoredAnomaly for an AnomalyID, or (zero, false) if the
// id is unknown (already evicted or never existed).
func (m *MemoryStore) Get(anomalyID string) (StoredAnomaly, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	idx, ok := m.byID[anomalyID]
	if !ok {
		return StoredAnomaly{}, false
	}
	return m.items[idx], true
}

// Ack marks an anomaly as acknowledged by userID at the current UTC time.
// Returns ErrAnomalyNotFound for an unknown id; otherwise nil. Idempotent.
func (m *MemoryStore) Ack(anomalyID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, ok := m.byID[anomalyID]
	if !ok {
		return fmt.Errorf("%w: %q", ErrAnomalyNotFound, anomalyID)
	}
	sa := m.items[idx]
	sa.Acked = true
	sa.AckedBy = userID
	sa.AckedAt = m.now().UTC()
	m.items[idx] = sa
	return nil
}

// ListUnacked returns the most-recent unacked anomalies, newest first.
func (m *MemoryStore) ListUnacked(limit int) []StoredAnomaly {
	return m.listInternal(limit, true)
}

// ListAll returns the most-recent anomalies regardless of ack state.
func (m *MemoryStore) ListAll(limit int) []StoredAnomaly {
	return m.listInternal(limit, false)
}

func (m *MemoryStore) listInternal(limit int, unackedOnly bool) []StoredAnomaly {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 {
		return []StoredAnomaly{}
	}
	out := make([]StoredAnomaly, 0, limit)
	// Walk newest first.
	for i := len(m.items) - 1; i >= 0 && len(out) < limit; i-- {
		sa := m.items[i]
		if unackedOnly && sa.Acked {
			continue
		}
		out = append(out, sa)
	}
	return out
}

// allocateID produces a compact 16-hex-char id. We use a monotonic counter
// (no crypto/rand) to keep ids lexically sortable by creation time, which
// helps operator mental-model alignment. Crypto-strength uniqueness is not
// required for an in-process alert store.
func (m *MemoryStore) allocateID() string {
	m.nextID++
	var nonce [4]byte
	_, _ = rand.Read(nonce[:]) //nolint:errcheck // best-effort; if it fails we still have the counter
	buf := make([]byte, 0, 16)
	counter := make([]byte, 8)
	v := m.nextID
	for i := 7; i >= 0; i-- {
		counter[i] = byte(v & 0xff)
		v >>= 8
	}
	buf = append(buf, hex.EncodeToString(counter)...)
	buf = append(buf, hex.EncodeToString(nonce[:])...)
	return "anom-" + string(buf)
}
