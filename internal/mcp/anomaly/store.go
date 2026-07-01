package anomaly

import (
	"sync"
	"time"
)

// Store is a thread-safe ring buffer of AnomalyEvent values.
type Store struct {
	mu       sync.RWMutex
	entries  []AnomalyEvent
	capacity int
}

// NewStore creates a Store with the given capacity. A capacity <= 0 defaults
// to 1000.
func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = 1000
	}
	return &Store{capacity: capacity}
}

// Add appends an event, dropping the oldest entry when capacity is exceeded.
func (s *Store) Add(e AnomalyEvent) {
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	if len(s.entries) > s.capacity {
		s.entries = s.entries[len(s.entries)-s.capacity:]
	}
}

// Recent returns the newest up to n events in descending chronological order
// (newest first). The returned slice is a copy.
func (s *Store) Recent(n int) []AnomalyEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 {
		return []AnomalyEvent{}
	}
	if n > len(s.entries) {
		n = len(s.entries)
	}
	out := make([]AnomalyEvent, n)
	for i := 0; i < n; i++ {
		out[i] = s.entries[len(s.entries)-1-i]
	}
	return out
}
