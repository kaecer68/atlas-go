package marketdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// DailyQuotaTracker tracks API calls per day with persistent storage.
// Resets automatically at midnight.
//
// Two independent exhaustion signals are tracked:
//
//  1. the LOCAL ceiling (dailyLimit) — this process counted its own calls;
//  2. the UPSTREAM latch (upstreamExhausted) — the provider itself refused a
//     call because the account's quota is gone (FinMind: HTTP 402
//     "Requests reach the upper limit").
//
// (2) exists because (1) is only as accurate as the ceiling constant. On
// 2026-09-26 FinMind started refusing at ~12,500 calls while the local
// ceiling still said 14,400: the platform kept knocking on a closed door,
// and — worse — a service restart reloaded a counter below the ceiling and
// restarted the flood (auto_quote_backfill). The latch is persisted, so a
// restart cannot forget that today's budget is gone.
type DailyQuotaTracker struct {
	mu         sync.RWMutex
	provider   string
	stateFile  string
	dailyLimit int
	callsToday int
	lastReset  time.Time
	// upstreamExhausted is the latched upstream refusal. It is cleared only
	// when the quota day rolls over (see rolloverLocked).
	upstreamExhausted bool
	upstreamReason    string
	upstreamAt        time.Time
}

// QuotaState is the persisted state format.
type QuotaState struct {
	CallsToday int       `json:"calls_today"`
	LastReset  time.Time `json:"last_reset"`
	// UpstreamExhausted is the durable latch written when the PROVIDER said
	// today's quota is gone. Persisting it is the whole point: without it a
	// restart silently resets the local view and the backfill runs into the
	// same wall again (2026-09-26 production incident).
	UpstreamExhausted bool   `json:"upstream_exhausted,omitempty"`
	UpstreamReason    string `json:"upstream_reason,omitempty"`
	UpstreamAt        string `json:"upstream_at,omitempty"` // RFC3339 UTC
}

// NewDailyQuotaTracker creates a new daily quota tracker.
// provider: e.g., "tej", "fugle"
// stateDir: directory to store state files
// dailyLimit: maximum calls per day
func NewDailyQuotaTracker(provider, stateDir string, dailyLimit int) *DailyQuotaTracker {
	stateFile := filepath.Join(stateDir, fmt.Sprintf("%s_daily_quota.json", provider))
	t := &DailyQuotaTracker{
		provider:   provider,
		stateFile:  stateFile,
		dailyLimit: dailyLimit,
		lastReset:  time.Now().Truncate(24 * time.Hour),
	}
	t.load()
	return t
}

// AllowCall checks if a call is allowed and increments the counter if so.
// Returns true if the call is within quota.
func (t *DailyQuotaTracker) AllowCall() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rolloverLocked(time.Now())

	// Upstream latch first: the provider already told us today's budget is
	// gone, so there is nothing left to spend and nothing to learn from
	// trying again before the day rolls over.
	if t.upstreamExhausted {
		return false
	}

	if t.callsToday >= t.dailyLimit {
		return false
	}

	t.callsToday++
	t.save()
	return true
}

// Remaining returns the number of calls remaining today.
//
// It reports 0 while the upstream latch is set: the account has no usable
// budget left, whatever the local counter says. This is what makes every
// reserve/floor check built on Remaining() (auto_quote_backfill's 1,500-call
// floor, the sbl/tdcc 500-call reserve) stop immediately after an upstream
// refusal — including right after a restart.
func (t *DailyQuotaTracker) Remaining() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rolloverLocked(time.Now())
	if t.upstreamExhausted {
		return 0
	}

	remaining := t.dailyLimit - t.callsToday
	if remaining < 0 {
		return 0
	}
	return remaining
}

// MarkUpstreamExhausted latches "the provider itself said today's quota is
// gone" until the quota day rolls over, and persists it.
//
// reason is stored for operators (channel health, error text). Callers MUST
// pass an already-sanitized string: it is written to the state file on disk
// and echoed in errors, so upstream token fragments must never reach it.
func (t *DailyQuotaTracker) MarkUpstreamExhausted(reason string) {
	t.mu.Lock()
	t.rolloverLocked(time.Now())
	first := !t.upstreamExhausted
	t.upstreamExhausted = true
	if reason != "" {
		t.upstreamReason = reason
	}
	t.upstreamAt = time.Now().UTC()
	t.save()
	provider, storedReason, calls := t.provider, t.upstreamReason, t.callsToday
	t.mu.Unlock()

	if first {
		logging.Warn("marketdata", "daily_quota_upstream_exhausted",
			"provider", provider,
			"calls_today", calls,
			"reason", storedReason,
		)
	}
}

// UpstreamExhausted reports whether the provider has told us that today's
// quota is gone. The latch clears when the quota day rolls over.
func (t *DailyQuotaTracker) UpstreamExhausted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rolloverLocked(time.Now())
	return t.upstreamExhausted
}

// UpstreamExhaustion returns the latch together with the sanitized upstream
// reason and the time it was observed, so callers can explain WHY every call
// is being refused (instead of a bare "quota exhausted").
func (t *DailyQuotaTracker) UpstreamExhaustion() (bool, string, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rolloverLocked(time.Now())
	return t.upstreamExhausted, t.upstreamReason, t.upstreamAt
}

// rolloverLocked resets the per-day state when the quota day has advanced.
// The quota day is a UTC-aligned 24h window (time.Truncate on a UTC clock) —
// production containers run TZ-unset, so the boundary is 00:00Z = 08:00
// Taipei, matching the upstream's own daily reset as observed in
// data/state/finmind_daily_quota.json (last_reset=2026-09-24T00:00:00Z).
// Callers must hold t.mu.
func (t *DailyQuotaTracker) rolloverLocked(now time.Time) {
	today := now.Truncate(24 * time.Hour)
	if today.After(t.lastReset) {
		t.callsToday = 0
		t.lastReset = today
		t.upstreamExhausted = false
		t.upstreamReason = ""
		t.upstreamAt = time.Time{}
	}
}

// CallsToday returns the number of calls made today.
func (t *DailyQuotaTracker) CallsToday() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.callsToday
}

// SetLimit updates the daily limit (e.g., when tier changes).
func (t *DailyQuotaTracker) SetLimit(limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dailyLimit = limit
}

func (t *DailyQuotaTracker) load() {
	data, err := os.ReadFile(t.stateFile)
	if err != nil {
		return // File doesn't exist yet
	}

	var state QuotaState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	// Only load if it's from today. A state file from an earlier quota day
	// is intentionally ignored in full — including the upstream latch — and
	// the tracker starts the new day clean.
	today := time.Now().Truncate(24 * time.Hour)
	stateDay := state.LastReset.Truncate(24 * time.Hour)
	if !stateDay.Equal(today) {
		return
	}
	t.callsToday = state.CallsToday
	t.lastReset = state.LastReset
	// Restore the upstream latch so a restart cannot restart the flood that
	// the provider already refused (2026-09-26: auto_quote_backfill re-ran
	// after a restart and walked straight back into the 402 wall).
	t.upstreamExhausted = state.UpstreamExhausted
	t.upstreamReason = state.UpstreamReason
	if state.UpstreamAt != "" {
		if at, err := time.Parse(time.RFC3339, state.UpstreamAt); err == nil {
			t.upstreamAt = at
		}
	}
}

func (t *DailyQuotaTracker) save() {
	state := QuotaState{
		CallsToday:        t.callsToday,
		LastReset:         t.lastReset,
		UpstreamExhausted: t.upstreamExhausted,
		UpstreamReason:    t.upstreamReason,
	}
	if !t.upstreamAt.IsZero() {
		state.UpstreamAt = t.upstreamAt.UTC().Format(time.RFC3339)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	// P1-12: atomic write (tmp + rename, matching government_broker). A
	// direct WriteFile could leave a truncated/corrupt state file if the
	// process dies mid-write — the next process would then silently start
	// at calls_today=0 and blow the daily budget.
	if err := os.MkdirAll(filepath.Dir(t.stateFile), 0o755); err != nil {
		return
	}
	tmpPath := t.stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmpPath, t.stateFile); err != nil {
		_ = os.Remove(tmpPath)
	}
}
