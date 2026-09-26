package marketdata

import (
	"encoding/json"
	"errors"
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
// # Scope of the ceiling: every process that shares this state file
//
// Until issue #2014 the tracker read the state file ONCE, in the constructor,
// and every AllowCall() overwrote the whole file with this process's private
// counter (last-writer-wins). With production running one long-lived atlas-go
// process plus seven atlas-cron-* containers that all bind-mount the same
// data/state directory, that made the ceiling a PER-PROCESS ceiling while
// looking like a global one: N processes each happily spend up to
// dailyLimit, and the file on disk ends up holding whichever process wrote
// last (so it under-reports the account total, sometimes by the full factor
// of N).
//
// The tracker now treats the state file as the single authority: every read
// happens under an exclusive cross-process lock (flock) and every increment is
// a locked read-modify-write, so the ceiling bounds the SUM over all processes
// that share the file. Measured on the production host (2026-09-26, three
// containers sharing one host directory, ceiling 200 each):
//
//	before: 3 × 200 calls actually spent, file said 200
//	after:   200 calls actually spent, file said 200
//
// The file lock is advisory and local to one filesystem: it covers exactly the
// processes that share the state directory (production: every atlas container
// on the host). A second host with its own copy of data/state is NOT covered —
// this is a same-shared-state guarantee, not an account-wide one. It also
// requires a filesystem that implements flock; if the platform cannot lock, the
// tracker fails closed instead of pretending to enforce a ceiling.
//
// # Two independent exhaustion signals are tracked
//
//  1. the LOCAL ceiling (dailyLimit) — the shared counter reached its budget;
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
//
// # Fail closed
//
// A quota guard that silently assumes "0 calls used" whenever it cannot read
// its own state is not a guard (issue #2014 requirement 3; same family as the
// #2009 "empty value passes" incident). When the on-disk truth cannot be
// established the tracker refuses calls, reports 0 remaining, and exposes the
// reason through StateErr() so callers can surface a distinct message:
//
//   - unreadable / uncreatable state directory or file  -> ErrQuotaStateUnavailable
//   - unparsable / empty / negative / undated state file -> ErrQuotaStateCorrupt
//
// Corruption is additionally made DURABLE: the bad file is renamed aside for
// forensics and an explicit marker (`quota_unknown: true`) is written for the
// current quota day, so a restart cannot turn "usage unknown" into "nothing
// used today". The marker clears on the next quota-day rollover.
type DailyQuotaTracker struct {
	mu         sync.Mutex
	provider   string
	stateFile  string
	lockFile   string
	dailyLimit int

	// callsToday / lastReset / upstream* mirror the last observed durable
	// state. They are a cache: every read path refreshes them from the file
	// under the cross-process lock, so they never go stale across processes.
	callsToday        int
	lastReset         time.Time
	upstreamExhausted bool
	upstreamReason    string
	upstreamAt        time.Time

	// stateErr is the last durable-state failure. Non-nil means the tracker is
	// failing closed.
	stateErr error
	// loggedErr deduplicates the operator-facing log line: the same failure is
	// logged once per distinct message instead of once per refused call.
	loggedErr string
}

// ErrQuotaStateUnavailable reports that the persisted quota counter could not
// be established (unreadable, uncreatable, or a platform without advisory file
// locking). Callers must treat it as "budget unknown" and fail closed.
var ErrQuotaStateUnavailable = errors.New("daily quota state unavailable")

// ErrQuotaStateCorrupt reports that the persisted quota counter exists but
// cannot be trusted (unparsable JSON, empty file, negative count, or missing
// quota day). Callers must treat it as "budget unknown" and fail closed.
var ErrQuotaStateCorrupt = errors.New("daily quota state corrupt")

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
	// QuotaUnknown marks a quota day whose usage could NOT be established
	// because the previous state file was corrupt (#2014 requirement 3). It is
	// written instead of a silent `calls_today: 0`, and it keeps the day closed
	// across restarts until the quota day rolls over.
	QuotaUnknown       bool   `json:"quota_unknown,omitempty"`
	QuotaUnknownReason string `json:"quota_unknown_reason,omitempty"`
}

// NewDailyQuotaTracker creates a new daily quota tracker.
// provider: e.g., "tej", "fugle"
// stateDir: directory to store state files
// dailyLimit: maximum calls per day, summed over every process sharing stateDir
func NewDailyQuotaTracker(provider, stateDir string, dailyLimit int) *DailyQuotaTracker {
	stateFile := filepath.Join(stateDir, fmt.Sprintf("%s_daily_quota.json", provider))
	return &DailyQuotaTracker{
		provider:   provider,
		stateFile:  stateFile,
		lockFile:   stateFile + ".lock",
		dailyLimit: dailyLimit,
	}
}

// AllowCall spends one call from the shared daily budget.
//
// It is a locked read-modify-write against the state file, so concurrent
// processes cannot both observe "budget left" and both increment past the
// ceiling (#2014). Returns false when the call must not be made: ceiling
// reached, upstream latch set, or the state is unusable (fail closed).
func (t *DailyQuotaTracker) AllowCall() bool {
	allowed, _ := t.update(func(st *QuotaState) bool {
		// Upstream latch first: the provider already told us today's budget is
		// gone, so there is nothing left to spend and nothing to learn from
		// trying again before the day rolls over.
		if st.UpstreamExhausted {
			return false
		}
		if st.CallsToday >= t.dailyLimit {
			return false
		}
		st.CallsToday++
		return true
	})
	return allowed
}

// Remaining returns the number of calls remaining today across every process
// sharing this state file.
//
// It reports 0 while the upstream latch is set (the account has no usable
// budget left, whatever the local counter says) and 0 when the state is
// unusable (fail closed). This is what makes every reserve/floor check built on
// Remaining() (auto_quote_backfill's 1,500-call floor, the sbl/tdcc 500-call
// reserve) stop immediately after an upstream refusal — including right after a
// restart.
func (t *DailyQuotaTracker) Remaining() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.refreshLocked(); err != nil {
		return 0
	}
	if t.upstreamExhausted {
		return 0
	}
	if remaining := t.dailyLimit - t.callsToday; remaining > 0 {
		return remaining
	}
	return 0
}

// CallsToday returns the number of calls made today across every process
// sharing this state file, as of the last successful read. When the state is
// unusable it returns the last known value (never a silent 0) and StateErr()
// explains why.
func (t *DailyQuotaTracker) CallsToday() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	_ = t.refreshLocked()
	return t.callsToday
}

// StateErr returns the reason the tracker is failing closed, or nil when the
// shared state is healthy. Callers must surface it: a refused call caused by an
// unreadable counter must not look like an ordinary "quota exhausted".
func (t *DailyQuotaTracker) StateErr() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateErr
}

// StateFile returns the path of the counter file this tracker treats as the
// single authority (diagnostics, channel health, ops scripts).
func (t *DailyQuotaTracker) StateFile() string { return t.stateFile }

// MarkUpstreamExhausted latches "the provider itself said today's quota is
// gone" until the quota day rolls over, and persists it.
//
// reason is stored for operators (channel health, error text). Callers MUST
// pass an already-sanitized string: it is written to the state file on disk
// and echoed in errors, so upstream token fragments must never reach it.
func (t *DailyQuotaTracker) MarkUpstreamExhausted(reason string) {
	first := false
	// Persist regardless of the return value: latching is not a budget spend.
	_, err := t.update(func(st *QuotaState) bool {
		first = !st.UpstreamExhausted
		st.UpstreamExhausted = true
		if reason != "" {
			st.UpstreamReason = reason
		}
		st.UpstreamAt = time.Now().UTC().Format(time.RFC3339)
		return true
	})
	if err != nil {
		// The latch is best-effort once the state is already unusable: the
		// tracker is failing closed anyway, and `update` has logged why.
		return
	}
	if first {
		logging.Warn("marketdata", "daily_quota_upstream_exhausted",
			"provider", t.provider,
			"calls_today", t.CallsToday(),
			"reason", reason,
		)
	}
}

// UpstreamExhausted reports whether the provider has told us that today's
// quota is gone. The latch clears when the quota day rolls over.
func (t *DailyQuotaTracker) UpstreamExhausted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	_ = t.refreshLocked()
	return t.upstreamExhausted
}

// UpstreamExhaustion returns the latch together with the sanitized upstream
// reason and the time it was observed, so callers can explain WHY every call
// is being refused (instead of a bare "quota exhausted").
func (t *DailyQuotaTracker) UpstreamExhaustion() (bool, string, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	_ = t.refreshLocked()
	return t.upstreamExhausted, t.upstreamReason, t.upstreamAt
}

// SetLimit updates the daily limit (e.g., when tier changes).
//
// The ceiling is per-process configuration (it comes from
// marketdata.FinMindDailyLimit / the constructor argument) and is deliberately
// NOT persisted: every process sharing the counter must resolve the same
// number from its own config. A process with a lower ceiling stops spending
// first; it cannot raise another process's ceiling.
func (t *DailyQuotaTracker) SetLimit(limit int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.dailyLimit = limit
}

// update runs fn against the authoritative on-disk state with the exclusive
// cross-process lock held, persists the mutation when fn reports it consumed
// budget, and mirrors the result into the in-memory cache.
//
// It returns (allowed, nil) when the operation may proceed, (false, nil) when
// the budget is exhausted, and (false, err) when the state is unusable — in
// which case the tracker fails closed and err carries the reason.
func (t *DailyQuotaTracker) update(fn func(st *QuotaState) bool) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	unlock, err := t.lockState()
	if err != nil {
		t.recordStateErrLocked(err)
		return false, err
	}
	defer unlock()

	st, err := t.readStateLocked()
	if err != nil {
		if errors.Is(err, ErrQuotaStateCorrupt) {
			// Durable fail-closed: quarantine the unparsable file and leave an
			// explicit marker for the current quota day so that a restart (or
			// the next process) cannot read "no file" as "0 calls used".
			if qErr := t.quarantineCorruptStateLocked(err); qErr != nil {
				err = errors.Join(err, qErr)
			}
		}
		t.recordStateErrLocked(err)
		return false, err
	}

	normalizeQuotaState(&st, time.Now())
	if st.QuotaUnknown {
		reason := st.QuotaUnknownReason
		if reason == "" {
			reason = "usage for this quota day is unknown"
		}
		stateErr := fmt.Errorf("%w: %s (%s)", ErrQuotaStateCorrupt, reason, t.stateFile)
		t.applyLocked(st)
		t.recordStateErrLocked(stateErr)
		return false, stateErr
	}

	allowed := fn(&st)
	if allowed {
		if err := t.writeStateLocked(st); err != nil {
			// We already decided to spend, but we cannot record it: refusing is
			// the only option that keeps the ceiling honest. It is also the
			// difference between "counter broken" and "counter silently 0".
			t.recordStateErrLocked(err)
			return false, err
		}
	}
	// A rollover observed while refusing is deliberately not written back: the
	// reset is idempotent and the next writer persists it.

	t.applyLocked(st)
	t.clearStateErrLocked()
	return allowed, nil
}

// refreshLocked re-reads the authoritative state so getters never report
// another process's stale view. It is read-only: repairing a corrupt file is
// done by update (the path that matters for actually spending budget).
func (t *DailyQuotaTracker) refreshLocked() error {
	unlock, err := t.lockState()
	if err != nil {
		t.recordStateErrLocked(err)
		return err
	}
	defer unlock()

	st, err := t.readStateLocked()
	if err != nil {
		t.recordStateErrLocked(err)
		return err
	}
	normalizeQuotaState(&st, time.Now())
	t.applyLocked(st)
	if st.QuotaUnknown {
		reason := st.QuotaUnknownReason
		if reason == "" {
			reason = "usage for this quota day is unknown"
		}
		stateErr := fmt.Errorf("%w: %s (%s)", ErrQuotaStateCorrupt, reason, t.stateFile)
		t.recordStateErrLocked(stateErr)
		return stateErr
	}
	t.clearStateErrLocked()
	return nil
}

// lockState takes the exclusive cross-process lock for this provider's state
// file and returns its release function. The lock lives in a dedicated
// `<state>.lock` file so the state file itself can keep using the crash-safe
// tmp+rename write (renaming the locked file would silently drop the lock).
//
// The lock is advisory, so it binds only the processes that cooperate — which
// is exactly the atlas containers sharing one data/state directory.
func (t *DailyQuotaTracker) lockState() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(t.stateFile), 0o755); err != nil {
		return nil, fmt.Errorf("%w: create state dir %s: %v", ErrQuotaStateUnavailable, filepath.Dir(t.stateFile), err)
	}
	f, err := os.OpenFile(t.lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("%w: open lock file %s: %v", ErrQuotaStateUnavailable, t.lockFile, err)
	}
	if err := lockFileExclusive(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w: lock %s: %v", ErrQuotaStateUnavailable, t.lockFile, err)
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}

// readStateLocked reads the persisted state. Callers must hold the lock.
//
// A missing state file on an existing state directory is an honest fresh start
// (0 calls used). Anything else that prevents the on-disk truth from being
// established is an error — never a silent zero.
func (t *DailyQuotaTracker) readStateLocked() (QuotaState, error) {
	raw, err := os.ReadFile(t.stateFile)
	if errors.Is(err, os.ErrNotExist) {
		dir := filepath.Dir(t.stateFile)
		if _, statErr := os.Stat(dir); statErr != nil {
			// WorkDir is wrong / the runtime state volume is not mounted.
			// Treating that as "no calls used today" would disable the guard
			// exactly when the deployment is broken.
			return QuotaState{}, fmt.Errorf("%w: state dir %s is not usable: %v",
				ErrQuotaStateUnavailable, dir, statErr)
		}
		return QuotaState{CallsToday: 0, LastReset: quotaDay(time.Now())}, nil
	}
	if err != nil {
		return QuotaState{}, fmt.Errorf("%w: read %s: %v", ErrQuotaStateUnavailable, t.stateFile, err)
	}

	var st QuotaState
	if err := json.Unmarshal(raw, &st); err != nil {
		return QuotaState{}, fmt.Errorf("%w: parse %s (%d bytes): %v",
			ErrQuotaStateCorrupt, t.stateFile, len(raw), err)
	}
	if st.LastReset.IsZero() {
		return QuotaState{}, fmt.Errorf("%w: %s has no last_reset (cannot tell which quota day it describes)",
			ErrQuotaStateCorrupt, t.stateFile)
	}
	if st.CallsToday < 0 {
		return QuotaState{}, fmt.Errorf("%w: %s has negative calls_today (%d)",
			ErrQuotaStateCorrupt, t.stateFile, st.CallsToday)
	}
	return st, nil
}

// writeStateLocked persists state atomically (tmp + rename). Callers must hold
// the lock. The tmp name is shared by every process, which is safe precisely
// because all writers hold the lock: only one of them is writing at a time.
func (t *DailyQuotaTracker) writeStateLocked(st QuotaState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("%w: marshal quota state: %v", ErrQuotaStateUnavailable, err)
	}
	if err := os.MkdirAll(filepath.Dir(t.stateFile), 0o755); err != nil {
		return fmt.Errorf("%w: create state dir for %s: %v", ErrQuotaStateUnavailable, t.stateFile, err)
	}
	tmpPath := t.stateFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("%w: write %s: %v", ErrQuotaStateUnavailable, tmpPath, err)
	}
	if err := os.Rename(tmpPath, t.stateFile); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: rename %s: %v", ErrQuotaStateUnavailable, t.stateFile, err)
	}
	return nil
}

// quarantineCorruptStateLocked moves an unparsable state file aside and writes
// the explicit "usage unknown" marker for the current quota day. Callers must
// hold the lock and must have failed readStateLocked with ErrQuotaStateCorrupt.
func (t *DailyQuotaTracker) quarantineCorruptStateLocked(cause error) error {
	quarantinePath := fmt.Sprintf("%s.corrupt-%s", t.stateFile, time.Now().UTC().Format("20060102T150405Z"))
	if err := os.Rename(t.stateFile, quarantinePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: quarantine %s: %v", ErrQuotaStateUnavailable, t.stateFile, err)
	}
	marker := QuotaState{
		CallsToday:         0,
		LastReset:          quotaDay(time.Now()),
		QuotaUnknown:       true,
		QuotaUnknownReason: clampForError(cause.Error(), 240),
	}
	if err := t.writeStateLocked(marker); err != nil {
		return err
	}
	logging.Error("marketdata", "daily_quota_state_corrupt",
		"provider", t.provider,
		"state_file", t.stateFile,
		"quarantined_to", quarantinePath,
		"error", clampForError(cause.Error(), 240),
		"action", "day latched as quota-unknown (fail closed); fix/remove the file to restore service",
	)
	return nil
}

// applyLocked mirrors a durable state into the in-memory cache.
func (t *DailyQuotaTracker) applyLocked(st QuotaState) {
	t.callsToday = st.CallsToday
	t.lastReset = st.LastReset
	t.upstreamExhausted = st.UpstreamExhausted
	t.upstreamReason = st.UpstreamReason
	t.upstreamAt = time.Time{}
	if st.UpstreamAt != "" {
		if at, err := time.Parse(time.RFC3339, st.UpstreamAt); err == nil {
			t.upstreamAt = at
		}
	}
}

// recordStateErrLocked remembers a fail-closed condition and logs it once per
// distinct message (a refused call happens on every probe; the operator does
// not need the same line every 5 seconds).
func (t *DailyQuotaTracker) recordStateErrLocked(err error) {
	if err == nil {
		return
	}
	t.stateErr = err
	msg := err.Error()
	if t.loggedErr == msg {
		return
	}
	t.loggedErr = msg
	logging.Error("marketdata", "daily_quota_state_unusable",
		"provider", t.provider,
		"state_file", t.stateFile,
		"error", clampForError(msg, 240),
	)
}

func (t *DailyQuotaTracker) clearStateErrLocked() {
	t.stateErr = nil
	t.loggedErr = ""
}

// quotaDay is the quota-day boundary an instant belongs to: a UTC-aligned 24h
// window (time.Truncate on a UTC clock). Production containers run TZ-unset, so
// the boundary is 00:00Z = 08:00 Taipei, matching the upstream's own daily
// reset as observed in data/state/finmind_daily_quota.json.
func quotaDay(now time.Time) time.Time { return now.Truncate(24 * time.Hour) }

// normalizeQuotaState resets the per-day state when the quota day has advanced,
// and reports whether it changed anything.
func normalizeQuotaState(st *QuotaState, now time.Time) bool {
	today := quotaDay(now)
	if today.After(st.LastReset) {
		// A state file from an earlier quota day is intentionally ignored in
		// full — including the upstream latch and a quota-unknown marker.
		*st = QuotaState{CallsToday: 0, LastReset: today}
		return true
	}
	return false
}
