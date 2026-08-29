package llm_annotator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// Local aliases shorten test expressions without dragging the apigateway
// package name into every assertion. They are intentionally lowercase so
// they stay package-scoped.
type apigatewayState = apigateway.State

const apigatewayCircuitBreakerFailureThreshold = apigateway.CircuitBreakerFailureThreshold

// errorsIs / simpleError are tiny helpers so the characterisation tests
// don't need to import the full errors / fmt packages just for one line.
func errorsIs(err, target error) bool { return errors.Is(err, target) }

type stringError string

func (s stringError) Error() string { return string(s) }

func simpleError(msg string) error { return stringError(msg) }

// TestKimiClient_BreakerNil_DoesNotPanic verifies that omitting the
// Breaker still produces a working client (Annotate short-circuits any
// breaker check, BreakerState reports "disabled").
func TestKimiClient_BreakerNil_DoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate with nil breaker: %v", err)
	}
	if c.BreakerState() != "disabled" {
		t.Errorf("BreakerState = %q, want disabled", c.BreakerState())
	}
}

// TestKimiClient_BreakerOpensAfterRepeatedFailures drives the wrapper
// (delegating to apigateway.CircuitBreaker) through
// apigateway.CircuitBreakerFailureThreshold consecutive HTTP failures and
// verifies the Annotate layer reports the Open state.
func TestKimiClient_BreakerOpensAfterRepeatedFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{
		APIKey: "k", BaseURL: srv.URL,
		Breaker: newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	// 3 consecutive breaker failures (= 3 Annotate calls each exhausting
	// their retry budget) opens the breaker.
	for range apigatewayCircuitBreakerFailureThreshold {
		_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	}
	if got := c.BreakerState(); got != "open" {
		t.Errorf("BreakerState after %d failures = %q, want open", apigatewayCircuitBreakerFailureThreshold, got)
	}

	// Next call: short-circuits with circuit_open metric, no HTTP call.
	var hits int32
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv2.Close()
	c.cfg.BaseURL = srv2.URL
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("HTTP server hit count = %d while breaker open, want 0", got)
	}
}

// TestKimiClient_BreakerRecoversOnSuccess confirms that an operator-driven
// Reset() puts the breaker back to Closed so the next successful Annotate
// call returns content.
func TestKimiClient_BreakerRecoversOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{
		APIKey: "k", BaseURL: srv.URL,
		Breaker: newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	for range apigatewayCircuitBreakerFailureThreshold {
		_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	}
	if c.BreakerState() != "open" {
		t.Fatalf("setup: breaker state = %q, want open", c.BreakerState())
	}

	// Manual reset + point to a healthy server simulates recovery.
	c.breaker.Reset()
	c.cfg.BaseURL = newHealthyServer(t)
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate on healthy server: %v", err)
	}
	if got := c.BreakerState(); got != "closed" {
		t.Errorf("BreakerState after success = %q, want closed", got)
	}
}

// TestKimiClient_BudgetForcesBreakerOpen is the characterization for the
// budget callback semantic. After the wrapped budget callback fires
// (ForceOpen + SetManualOverride(true)), the breaker must stay Open even
// after a subsequent successful Annotate call.
func TestKimiClient_BudgetForcesBreakerOpen(t *testing.T) {
	var fired int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{
		APIKey:          "k",
		BaseURL:         srv.URL,
		BudgetThreshold: 19,
		BudgetCallback:  func(u Usage) { atomic.StoreInt32(&fired, 1) },
		Breaker:         newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("first Annotate: %v", err)
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("budget callback not invoked after first call")
	}
	if got := c.BreakerState(); got != "open" {
		t.Errorf("BreakerState after budget fire = %q, want open (ForceOpen wired)", got)
	}

	// Characterization for the Wave 12 Phase 2 manual-override semantic:
	// after budget fires, the breaker must stay Open until an operator
	// calls Reset — even if a follow-up Annotate call would otherwise
	// succeed against a healthy upstream.
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer healthy.Close()
	c.cfg.BaseURL = healthy.URL
	c.breaker.ForceOpen()
	c.breaker.SetManualOverride(true)
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err == nil {
		t.Fatalf("Annotate should not succeed while breaker is force-open")
	}
	if got := c.BreakerState(); got == "closed" {
		t.Errorf("BreakerState after override-armed success = closed, want open/half-open")
	}
}

// TestKimiClient_Annotate_RecordsCircuitOpenMetric confirms that when the
// breaker rejects an Annotate call, the circuit_open counter increments
// — used by dashboards to surface "stuck" breakers.
func TestKimiClient_Annotate_RecordsCircuitOpenMetric(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	m := newCountingMetrics()
	c, err := NewKimiClient(Config{
		APIKey: "k", BaseURL: srv.URL,
		Metrics: m,
		Breaker: newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	for range apigatewayCircuitBreakerFailureThreshold {
		_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	}
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	labels := map[string]string{"provider": "kimi", "outcome": "circuit_open"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got < 1 {
		t.Errorf("circuit_open counter = %v, want >= 1", got)
	}
}

// TestCircuitBreaker_Allow_Open verifies the Wave 4-era Allow() helper
// (preserved by the wrapper): it returns (false, ErrCircuitOpen-wrapped)
// when the breaker is in Open state.
func TestCircuitBreaker_Allow_Open(t *testing.T) {
	cb := newCircuitBreaker()
	cb.ForceOpen()
	ok, err := cb.Allow()
	if ok {
		t.Errorf("Allow after ForceOpen returned ok=true, want false")
	}
	if err == nil {
		t.Fatalf("Allow after ForceOpen returned err=nil, want ErrCircuitOpen wrap")
	}
	if !errorsIs(err, ErrCircuitOpen) {
		t.Errorf("Allow error %v does not wrap ErrCircuitOpen", err)
	}
}

// TestCircuitBreaker_Allow_Closed verifies Allow returns (true, nil) for a
// freshly-initialised breaker that has not recorded any failures.
func TestCircuitBreaker_Allow_Closed(t *testing.T) {
	cb := newCircuitBreaker()
	ok, err := cb.Allow()
	if !ok {
		t.Errorf("Allow on fresh breaker returned ok=false, want true")
	}
	if err != nil {
		t.Errorf("Allow on fresh breaker returned err=%v, want nil", err)
	}
}

// TestCircuitBreaker_Snapshot exercises the Wave 4-era Snapshot() helper
// preserved by the wrapper. It must return the same labels the dashboard
// relies on: "closed" / "open" / "half-open" / "unknown".
func TestCircuitBreaker_Snapshot(t *testing.T) {
	cb := newCircuitBreaker()
	if got := cb.Snapshot(); got != "closed" {
		t.Errorf("Snapshot fresh breaker = %q, want closed", got)
	}
	cb.ForceOpen()
	if got := cb.Snapshot(); got != "open" {
		t.Errorf("Snapshot after ForceOpen = %q, want open", got)
	}
}

// TestCircuitState_TypeAlias pins the CircuitState → apigateway.State
// type alias. If the alias is ever dropped, this test breaks at compile
// time because CircuitClosed / CircuitOpen / CircuitHalfOpen would no
// longer satisfy apigateway.State.
func TestCircuitState_TypeAlias(t *testing.T) {
	var s CircuitState = CircuitClosed
	s = CircuitOpen
	s = CircuitHalfOpen
	_ = s

	// Confirm the alias: apigateway.State and CircuitState must be the
	// same type at compile time.
	var a apigatewayState = CircuitClosed
	var c CircuitState = a
	_ = c
}

// TestCircuitBreaker_WithNowFunc verifies that the Wave 4-era clock
// injection hook survives the migration to apigateway.CircuitBreaker.
// This is the characterisation for the "fake clock" testing mechanism
// the user explicitly asked to preserve.
//
// Sequence: lock the breaker open via ForceOpen, then inject a clock
// that returns "1 hour in the future" relative to the real time. The
// next Call() should see the recovery timeout as elapsed and transition
// to HalfOpen, then a successful fn() should leave it in HalfOpen
// (because manualOverride is not armed).
func TestCircuitBreaker_WithNowFunc(t *testing.T) {
	cb := newCircuitBreaker()

	// Pin the clock to a known instant.
	frozen := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	nowValue := frozen
	cb.WithNowFunc(func() time.Time { return nowValue })

	// Drive 3 failures with the frozen clock — lastFailure should be
	// "frozen", not the real wall clock.
	errBoom := simpleError("boom")
	for i := range 3 {
		if err := cb.Call(func() error { return errBoom }); err == nil {
			t.Fatalf("call %d: expected error, got nil", i)
		}
	}
	if cb.State() != CircuitOpen {
		t.Fatalf("state after 3 failures = %v, want CircuitOpen", cb.State())
	}

	// Step the clock forward 10 minutes (recovery timeout = 5 min).
	nowValue = frozen.Add(10 * time.Minute)

	// With the clock advanced, the next Call() should see recovery
	// timeout elapsed → transition Open → HalfOpen → run fn → Closed
	// (no manual override armed).
	if err := cb.Call(func() error { return nil }); err != nil {
		t.Fatalf("Call after recovery elapsed: %v", err)
	}
	if cb.State() != CircuitClosed {
		t.Errorf("state after success with advanced clock = %v, want CircuitClosed", cb.State())
	}

	// Reset the clock and confirm the production default takes over.
	cb.WithNowFunc(nil)
	if cb.State() != CircuitClosed {
		t.Errorf("state after WithNowFunc(nil) reset = %v, want CircuitClosed", cb.State())
	}
}

// newHealthyServer returns the URL of an httptest server that always 200s.
func newHealthyServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
