package marketdata

import (
	"sort"
	"sync"
	"time"
)

// QuotaEntry is a single provider's quota snapshot for a given day.
// Fields are read-only; callers should treat the value as immutable.
type QuotaEntry struct {
	Provider  string `json:"provider"`             // e.g. "finmind", "fugle"
	Used      int    `json:"used"`                 // calls made today
	Limit     int    `json:"limit"`                // configured daily ceiling
	Remaining int    `json:"remaining"`            // Limit - Used (clamped to >= 0)
	StateFile string `json:"state_file,omitempty"` // path to the persistent counter file
	UpdatedAt string `json:"updated_at"`           // RFC3339 timestamp of last AllowCall
	Exhausted bool   `json:"exhausted"`            // true when Used >= Limit
}

// QuotaSnapshot is a unified read-only view across every registered provider.
// Returned by QuotaRegistry.Snapshot() so the dashboard, channel-health page,
// and alerting pipeline all see one consistent picture. This is the file that
// addresses kaecer's 2026-08-04 feedback ("改了 A 讓 B 壞掉"): a single
// registry means changing FinMind's gate cannot silently shift Fugle's
// budget view, because both read from the same per-day counter.
type QuotaSnapshot struct {
	GeneratedAt string       `json:"generated_at"` // RFC3339 — when Snapshot() was called
	Entries     []QuotaEntry `json:"entries"`      // sorted by Provider for stable output
}

// QuotaRegistry is the shared cross-provider quota view. It does NOT own the
// per-provider DailyQuotaTracker — those live inside FinMindClient / FugleClient
// — but it knows about them so a single Snapshot() call returns everything
// the dashboard needs.
//
// Why a registry and not just read each tracker independently?
//   - One HTTP endpoint to populate the dashboard
//   - One alerting surface (so the channel-health page cannot disagree with
//     the alert rules about whether Fugle is "ok" vs "warn")
//   - Future paid-tier upgrades can register new providers without touching
//     the dashboard code
type QuotaRegistry struct {
	mu        sync.RWMutex
	providers map[string]*DailyQuotaTracker // provider name → tracker
}

// NewQuotaRegistry creates an empty registry. Provider trackers are added
// via Register(); tests use this constructor with empty state.
func NewQuotaRegistry() *QuotaRegistry {
	return &QuotaRegistry{
		providers: make(map[string]*DailyQuotaTracker),
	}
}

var (
	// globalQuotaRegistry is the process-wide singleton. Both
	// FinMindClient.GetSharedFinMindClient and FugleClient.GetSharedFugleClient
	// register their trackers here at construction time, so the dashboard
	// sees both without any explicit wiring at the call site.
	globalQuotaRegistry     *QuotaRegistry
	globalQuotaRegistryOnce sync.Once
)

// GlobalQuotaRegistry returns the process-wide registry. Safe to call
// concurrently — the first call initializes; subsequent calls return the same
// instance. New providers (TEJ revival, paid FinMind upgrade, custom
// backends) should register their DailyQuotaTracker here in their
// constructor so the dashboard automatically picks them up.
func GlobalQuotaRegistry() *QuotaRegistry {
	globalQuotaRegistryOnce.Do(func() {
		globalQuotaRegistry = NewQuotaRegistry()
	})
	return globalQuotaRegistry
}

// Register adds a provider's tracker. Safe to call multiple times for the
// same provider — the latest registration wins (useful for tests that
// reset the shared client).
func (r *QuotaRegistry) Register(provider string, tracker *DailyQuotaTracker) {
	if r == nil || tracker == nil || provider == "" {
		return
	}
	r.mu.Lock()
	r.providers[provider] = tracker
	r.mu.Unlock()
}

// Snapshot returns a stable, sorted view of every registered provider's
// current quota usage. The snapshot is a value type (copy on return) so
// callers can iterate freely without holding the registry lock.
func (r *QuotaRegistry) Snapshot() QuotaSnapshot {
	if r == nil {
		return QuotaSnapshot{GeneratedAt: time.Now().Format(time.RFC3339)}
	}
	r.mu.RLock()
	entries := make([]QuotaEntry, 0, len(r.providers))
	for name, t := range r.providers {
		entries = append(entries, quotaEntryFromTracker(name, t))
	}
	r.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Provider < entries[j].Provider
	})
	return QuotaSnapshot{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Entries:     entries,
	}
}

// quotaEntryFromTracker converts a tracker into a snapshot row. Kept as a
// free function (not a method) so tests can build entries directly without
// going through a live tracker.
func quotaEntryFromTracker(provider string, t *DailyQuotaTracker) QuotaEntry {
	used := t.CallsToday()
	remaining := t.Remaining()
	limit := t.dailyLimit
	return QuotaEntry{
		Provider:  provider,
		Used:      used,
		Limit:     limit,
		Remaining: remaining,
		UpdatedAt: time.Now().Format(time.RFC3339),
		Exhausted: remaining == 0,
	}
}

// IsProviderExhausted returns true if the named provider's daily quota is
// gone. Used by callers (e.g. the channel-health dashboard) that want a
// fast boolean check without iterating the full snapshot.
func (r *QuotaRegistry) IsProviderExhausted(provider string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.providers[provider]
	if !ok {
		return false
	}
	return t.Remaining() == 0
}
