package llm_annotator

import (
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// CircuitState is a Go 1.9+ type alias for apigateway.State. The
// Wave 4-era codebase compared against named constants (CircuitClosed /
// CircuitOpen / CircuitHalfOpen) rather than the integer values directly,
// so we keep both the alias and the named constants for backward compat.
type CircuitState = apigateway.State

// State constants — preserved from the pre-Phase 2 local CB so existing
// switch statements and test assertions keep compiling. They map 1:1 to
// the apigateway.State constants.
const (
	CircuitClosed   = apigateway.StateClosed
	CircuitOpen     = apigateway.StateOpen
	CircuitHalfOpen = apigateway.StateHalfOpen
)

// ErrCircuitOpen is returned when the breaker rejects a call. The
// pre-Phase 2 callers (e.g. monitoring/api/strategies) relied on
// errors.Is(err, ErrCircuitOpen); we keep the sentinel here for backward
// compat with downstream tools that grep on this exact message.
var ErrCircuitOpen = errors.New("circuit breaker open")

// CircuitBreaker is a thin facade over *apigateway.CircuitBreaker that
// preserves the Wave 4-era API surface that llm_annotator depended on:
// CircuitState type alias, ErrCircuitOpen sentinel, Allow(), Snapshot(),
// and the WithNowFunc clock-injection hook. All state mutations
// delegate to the canonical apigateway breaker — the wrapper exists only
// for backward compatibility and to keep the migration surface small.
type CircuitBreaker struct {
	cb *apigateway.CircuitBreaker
}

// newCircuitBreaker constructs a default-initialized breaker for use
// inside llm_annotator.Config. The breaker is wired with channel id
// "llm_annotator" so health endpoints can identify it.
func newCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		cb: apigateway.NewCircuitBreaker("llm_annotator"),
	}
}

// WithNowFunc injects a deterministic clock for recovery-timeout
// assertions. Test-only entry point — production code must leave the
// default clock in place. Returns the receiver for fluent chaining.
func (c *CircuitBreaker) WithNowFunc(now func() time.Time) *CircuitBreaker {
	c.cb.WithNowFunc(now)
	return c
}

// Call delegates to apigateway.CircuitBreaker.Call.
func (c *CircuitBreaker) Call(fn func() error) error {
	return c.cb.Call(fn)
}

// IsOpen reports whether the breaker is in the open state.
func (c *CircuitBreaker) IsOpen() bool {
	return c.cb.IsOpen()
}

// State returns the current breaker state as a CircuitState (alias for
// apigateway.State).
func (c *CircuitBreaker) State() CircuitState {
	return c.cb.State()
}

// ForceOpen delegates to apigateway.CircuitBreaker.ForceOpen.
func (c *CircuitBreaker) ForceOpen() {
	c.cb.ForceOpen()
}

// SetManualOverride delegates to apigateway.CircuitBreaker.SetManualOverride.
// Used by the budget-callback wiring in NewKimiClient to keep the breaker
// open across successful calls until an operator calls Reset.
func (c *CircuitBreaker) SetManualOverride(enabled bool) {
	c.cb.SetManualOverride(enabled)
}

// Reset delegates to apigateway.CircuitBreaker.Reset.
func (c *CircuitBreaker) Reset() {
	c.cb.Reset()
}

// Allow reports whether the next call would proceed without invoking a
// function. Returns (false, ErrCircuitOpen-wrapped) if the breaker would
// reject the call. The apigateway CB has no Allow() — this is a Wave 4-era
// helper that survives in the wrapper for backward compat.
func (c *CircuitBreaker) Allow() (bool, error) {
	err := c.cb.Call(func() error { return nil })
	if err == nil {
		return true, nil
	}
	return false, fmt.Errorf("%w: %v", ErrCircuitOpen, err)
}

// Snapshot returns the current breaker state as a string
// ("closed"/"open"/"half-open"/"unknown"). The apigateway CB exposes
// this indirectly via CircuitBreakerManager.Status; the wrapper offers a
// single-breaker view that mirrors the pre-Phase 2 API.
func (c *CircuitBreaker) Snapshot() string {
	switch c.cb.State() {
	case apigateway.StateClosed:
		return "closed"
	case apigateway.StateOpen:
		return "open"
	case apigateway.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}
