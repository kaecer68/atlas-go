package marketdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyQuotaTracker tracks API calls per day with persistent storage.
// Resets automatically at midnight.
type DailyQuotaTracker struct {
	mu         sync.RWMutex
	provider   string
	stateFile  string
	dailyLimit int
	callsToday int
	lastReset  time.Time
}

// QuotaState is the persisted state format.
type QuotaState struct {
	CallsToday int       `json:"calls_today"`
	LastReset  time.Time `json:"last_reset"`
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

	// Check if we need to reset for a new day
	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	if today.After(t.lastReset) {
		t.callsToday = 0
		t.lastReset = today
	}

	if t.callsToday >= t.dailyLimit {
		return false
	}

	t.callsToday++
	t.save()
	return true
}

// Remaining returns the number of calls remaining today.
func (t *DailyQuotaTracker) Remaining() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	if today.After(t.lastReset) {
		return t.dailyLimit
	}

	remaining := t.dailyLimit - t.callsToday
	if remaining < 0 {
		return 0
	}
	return remaining
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

	// Only load if it's from today
	today := time.Now().Truncate(24 * time.Hour)
	stateDay := state.LastReset.Truncate(24 * time.Hour)
	if stateDay.Equal(today) {
		t.callsToday = state.CallsToday
		t.lastReset = state.LastReset
	}
}

func (t *DailyQuotaTracker) save() {
	state := QuotaState{
		CallsToday: t.callsToday,
		LastReset:  t.lastReset,
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
