package eventquality

import (
	"fmt"
	"sync"
	"time"
)

// CrossSourceStatus indicates how many distinct sources have reported
// an event sharing the same (trigger_theme, symbol, effective_date)
// composite key.
type CrossSourceStatus string

const (
	StatusPending   CrossSourceStatus = "pending"   // 1 source
	StatusConfirmed CrossSourceStatus = "confirmed" // >= 2 sources
)

// sourceEntry records a single source report with its timestamp.
type sourceEntry struct {
	source string
	seenAt time.Time
}

// CrossSourceStore tracks unique sources per composite event key.
// Thread-safe; entries expire after a configurable TTL.
type CrossSourceStore struct {
	mu   sync.Mutex
	seen map[string][]sourceEntry
	ttl  time.Duration
	now  func() time.Time
}

// NewCrossSourceStore creates a store with the given TTL. A zero TTL
// defaults to 7 days.
func NewCrossSourceStore(ttl time.Duration) *CrossSourceStore {
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &CrossSourceStore{
		seen: make(map[string][]sourceEntry),
		ttl:  ttl,
		now:  time.Now,
	}
}

// SetClock replaces the clock for deterministic tests.
func (s *CrossSourceStore) SetClock(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

// dedupKey reuses the same composite-key format as Validator.
func crossSourceKey(triggerTheme, symbol string, effectiveDate time.Time) string {
	return fmt.Sprintf("%s|%s|%s",
		triggerTheme, symbol, effectiveDate.UTC().Format("2006-01-02"))
}

// Record registers a source for the given composite key and returns
// the current cross-source status. Calling Record for the same source
// more than once does not double-count.
//
// StatusPending  — only one distinct source has reported this event.
// StatusConfirmed — two or more distinct sources have reported.
func (s *CrossSourceStore) Record(source, triggerTheme, symbol string, effectiveDate time.Time) CrossSourceStatus {
	key := crossSourceKey(triggerTheme, symbol, effectiveDate)
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.gcLocked(now)

	entries, exists := s.seen[key]
	if exists {
		// Check if this source is already recorded.
		for i, e := range entries {
			if e.source == source {
				entries[i].seenAt = now
				s.seen[key] = entries
				if len(entries) >= 2 {
					return StatusConfirmed
				}
				return StatusPending
			}
		}
	}
	// New source for this key.
	s.seen[key] = append(entries, sourceEntry{source: source, seenAt: now})
	if len(s.seen[key]) >= 2 {
		return StatusConfirmed
	}
	return StatusPending
}

// Status returns the current cross-source status for the given composite
// key WITHOUT recording a new source. Returns empty string if the key
// has no entries at all.
func (s *CrossSourceStore) Status(triggerTheme, symbol string, effectiveDate time.Time) CrossSourceStatus {
	key := crossSourceKey(triggerTheme, symbol, effectiveDate)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.gcLocked(s.now())

	entries, ok := s.seen[key]
	if !ok || len(entries) == 0 {
		return ""
	}
	if len(entries) >= 2 {
		return StatusConfirmed
	}
	return StatusPending
}

// gcLocked removes entries whose TTL has expired. Caller must hold s.mu.
func (s *CrossSourceStore) gcLocked(now time.Time) {
	cutoff := now.Add(-s.ttl)
	for key, entries := range s.seen {
		var active []sourceEntry
		for _, e := range entries {
			if e.seenAt.After(cutoff) {
				active = append(active, e)
			}
		}
		if len(active) == 0 {
			delete(s.seen, key)
		} else {
			s.seen[key] = active
		}
	}
}
