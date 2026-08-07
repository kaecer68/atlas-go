package userstate

import "time"

// UserSignalState tracks a user's per-signal read/acknowledged status — the
// minimal "追蹤" primitive. A signal (e.g. foreign-3day-inflow) is shown in
// the dashboard; the investor marks it 已讀 (acknowledged) to fold it into
// their own discipline loop without being re-notified.
type UserSignalState struct {
	// UserID is the subscription.User.ID this state belongs to.
	UserID int64 `json:"user_id"`
	// SignalKey is the canonical signal id (strategy_techniques naming).
	SignalKey string `json:"signal_key"`
	// AcknowledgedAt is set when the user first marks the signal 已讀.
	// nil = not yet acknowledged (still "new" in the UI).
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	// Dismissed is true when the user chose to stop seeing this signal
	// entirely (hard dismiss vs soft acknowledge).
	Dismissed bool `json:"dismissed"`
	// UpdatedAt is the last mutation timestamp (for sync ordering).
	UpdatedAt time.Time `json:"updated_at"`
}

// UserWatchlist is a user's curated list of signals/symbols they want the
// platform to re-check. Distinct from universe_watchlist (D6 simulation
// pool — ops semantics, see Gap 3 audit §3 warning against semantic reuse).
type UserWatchlist struct {
	UserID    int64     `json:"user_id"`
	Symbols   []string  `json:"symbols"`    // Taiwan stock symbols, e.g. "2330"
	SignalKey string    `json:"signal_key"` // "" = whole-market watch
	CreatedAt time.Time `json:"created_at"`
}

// UserJournal is a free-form discipline record: what the investor observed,
// decided, and what happened. The "紀律" layer — retrospective over 行為覆盤.
type UserJournal struct {
	UserID    int64     `json:"user_id"`
	EntryID   string    `json:"entry_id"` // uuid or ULID
	SignalKey string    `json:"signal_key,omitempty"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}
