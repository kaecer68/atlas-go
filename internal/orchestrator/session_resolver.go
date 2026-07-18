package orchestrator

import (
	"errors"
	"fmt"
	"time"
)

// ErrSessionUnavailable is returned by TradingSessionResolver when
// the next effective trading session cannot be determined (e.g.
// no replay dataset loaded, or non-replay path without explicit
// session override). Callers MUST treat this as a hard failure —
// no policy may be produced without a known effective_from date.
var ErrSessionUnavailable = errors.New("next trading session unavailable")

// TradingSessionResolver determines the next effective trading
// session date for a given as-of timestamp. This is the authoritative
// source of truth for the effective_from field in sector allocation
// snapshots — no calendar arithmetic, no AddDate(0,0,1) guessing.
type TradingSessionResolver interface {
	// NextTradingSession returns the next trading session date strictly
	// after asOf. Returns ErrSessionUnavailable when:
	//   - No replay dataset loaded (replay is the only path that
	//     can determine future trading dates reliably)
	//   - asOf is past the last session in the dataset
	NextTradingSession(asOf time.Time) (time.Time, error)
}

// ReplayNextSessionResolver derives the next trading session from a
// replay dataset iterator. It calls dataset.NextDate() to get the
// date AFTER the current asOf — never lookahead beyond the replay's
// known sequence.
type ReplayNextSessionResolver struct {
	getNext func() (time.Time, bool)
}

// NewReplayNextSessionResolver creates a resolver that uses fn to
// determine the next date. fn should return the next trading date
// and true if available, or zero time and false if exhausted.
func NewReplayNextSessionResolver(fn func() (time.Time, bool)) *ReplayNextSessionResolver {
	return &ReplayNextSessionResolver{getNext: fn}
}

func (r *ReplayNextSessionResolver) NextTradingSession(asOf time.Time) (time.Time, error) {
	next, ok := r.getNext()
	if !ok {
		return time.Time{}, fmt.Errorf("%w: replay dataset exhausted (as_of=%s)", ErrSessionUnavailable, asOf.Format("2006-01-02"))
	}
	if !next.After(asOf) {
		return time.Time{}, fmt.Errorf("%w: resolved next %s is not after as_of %s",
			ErrSessionUnavailable, next.Format("2006-01-02"), asOf.Format("2006-01-02"))
	}
	return next, nil
}

// NoOpNextSessionResolver always returns ErrSessionUnavailable.
// Used for non-replay paths (live, auto_experiment) where lookahead
// is forbidden and no trading calendar is available.
type NoOpNextSessionResolver struct{}

func (r *NoOpNextSessionResolver) NextTradingSession(_ time.Time) (time.Time, error) {
	return time.Time{}, ErrSessionUnavailable
}
