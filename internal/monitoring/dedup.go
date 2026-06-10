package monitoring

import (
	"sync"
	"time"
)

// DedupResult contains the outcome of a dedup check.
type DedupResult struct {
	// Skip indicates the alert is a duplicate within the window and should not be saved.
	Skip bool
	// ExistingAlertID is the ID of the existing alert that was dedup-matched.
	ExistingAlertID string
	// NewCount is the updated count if the existing alert was incremented.
	NewCount int
}

// AlertDeduplicator prevents duplicate alerts within a configurable time window.
type AlertDeduplicator struct {
	mu         sync.Mutex
	window     time.Duration
	recent     map[string]time.Time // dedupKey → last seen timestamp
	alertStore *AlertStore
}

// NewAlertDeduplicator creates a deduplicator with the given window duration.
func NewAlertDeduplicator(window time.Duration, store *AlertStore) *AlertDeduplicator {
	return &AlertDeduplicator{
		window:     window,
		recent:     make(map[string]time.Time),
		alertStore: store,
	}
}

// Check returns a DedupResult indicating whether the dedupKey is a duplicate.
//   - If found within window: Skip=true, ExistingAlertID set, NewCount = existing count + 1
//   - If not found: Skip=false
func (d *AlertDeduplicator) Check(dedupKey string) (DedupResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	if lastSeen, ok := d.recent[dedupKey]; ok && now.Sub(lastSeen) < d.window {
		if d.alertStore != nil {
			existing, err := d.alertStore.FindByDedupKey(dedupKey)
			if err == nil && existing != nil {
				return DedupResult{
					Skip:            true,
					ExistingAlertID: existing.ID,
					NewCount:        existing.Count + 1,
				}, nil
			}
		}
		return DedupResult{Skip: true}, nil
	}

	return DedupResult{Skip: false}, nil
}

// Track records a dedup key with the current timestamp for future dedup checks.
func (d *AlertDeduplicator) Track(dedupKey string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recent[dedupKey] = time.Now()
}

// cleanup removes stale entries from the in-memory map.
func (d *AlertDeduplicator) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	cutoff := time.Now().Add(-d.window)
	for key, ts := range d.recent {
		if ts.Before(cutoff) {
			delete(d.recent, key)
		}
	}
}
