// Package marketdata: captcha_cooldown_test.go
//
// Tests for CaptchaCooldown. The "連續 CAPTCHA → 不再嘗試" case required
// by the implementation prompt is TestCaptchaCooldown_ConsecutiveCaptchaSkips.
// All time-dependent tests use an injected clock so they run in <1ms.
package marketdata

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCaptchaCooldown_DefaultConstruct: defaults are sane.
func TestCaptchaCooldown_DefaultConstruct(t *testing.T) {
	c := NewCaptchaCooldown()
	if c.cooldown != DefaultCaptchaCooldownDuration {
		t.Fatalf("cooldown = %v, want %v", c.cooldown, DefaultCaptchaCooldownDuration)
	}
	if c.ShouldSkip("any") {
		t.Error("ShouldSkip should be false for a never-seen channel")
	}
}

// TestCaptchaCooldown_ConsecutiveCaptchaSkips: this is the W2 spec case.
// Scenario:
//  1. First tick hits CAPTCHA — ShouldSkip flips to true.
//  2. Second tick (while still inside cooldown) — ShouldSkip stays true,
//     the caller does NOT attempt another upstream fetch.
//  3. A simulated "succeed once" — RecordSuccess — and the third tick
//     is allowed through (ShouldSkip false again).
//  4. If a new CAPTCHA arrives after recovery, the cycle restarts
//     (ShouldSkip true again).
func TestCaptchaCooldown_ConsecutiveCaptchaSkips(t *testing.T) {
	var nowFake atomic.Int64
	nowFake.Store(time.Date(2026, 7, 31, 14, 30, 0, 0, time.UTC).UnixNano())
	clock := func() time.Time { return time.Unix(0, nowFake.Load()) }
	c := CaptchaCooldownWith(24*time.Hour, clock)

	ch := "government_broker"

	// Pre-condition: never seen, no skip.
	if c.ShouldSkip(ch) {
		t.Fatal("pre-condition violated: ShouldSkip should be false on cold start")
	}

	// Tick 1 at 14:30: aggregator hits CAPTCHA. Caller checks
	// IsCaptchaErr + RecordCaptcha, and would have skipped the rest of
	// the work. Assert that ShouldSkip now reports true.
	tick1Err := fmt.Errorf("%w 8060/20260730", ErrCaptchaRequired)
	if !c.IsCaptchaErr(tick1Err) {
		t.Fatalf("tick1 err %q should match ErrCaptchaRequired", tick1Err)
	}
	c.RecordCaptcha(ch)
	if !c.ShouldSkip(ch) {
		t.Fatal("after 1st CAPTCHA, ShouldSkip should be true")
	}

	// Advance 1h — still inside 24h cooldown. ShouldSkip stays true.
	nowFake.Store(nowFake.Load() + int64(time.Hour))
	if !c.ShouldSkip(ch) {
		t.Fatal("after 1h, still inside cooldown, ShouldSkip should be true")
	}
	if c.Until(ch).IsZero() {
		t.Fatal("Until should report a non-zero time during active cooldown")
	}

	// Advance to 23h+1h later — outside the 24h window. ShouldSkip
	// would naturally flip false here. (This proves the cooldown
	// is bounded — it does NOT extend on every CAPTCHA hit.)
	nowFake.Store(nowFake.Load() + int64(23*time.Hour))
	if c.ShouldSkip(ch) {
		t.Fatal("after 24h, cooldown should have expired, ShouldSkip should be false")
	}

	// Reset and re-test the consecutive-skip pattern with a fast clock
	// so the test stays in <1ms wall-time. Use a 100ms cooldown.
	nowFake.Store(time.Date(2026, 7, 31, 14, 30, 0, 0, time.UTC).UnixNano())
	c = CaptchaCooldownWith(100*time.Millisecond, clock)

	// Three consecutive CAPTCHA hits within 100ms: each one is the same
	// "ShouldSkip=true" state, the caller is expected to skip the
	// upstream fetch every time. This is the W2 spec scenario.
	c.RecordCaptcha(ch)
	for i := 0; i < 3; i++ {
		nowFake.Add(int64(10 * time.Millisecond))
		if !c.ShouldSkip(ch) {
			t.Fatalf("consecutive CAPTCHA tick %d/3: ShouldSkip should be true (now=%v, until=%v)", i+1, clock(), c.Until(ch))
		}
	}

	// After 100ms+ the cooldown expires. The next call gets through
	// and recovers. A new CAPTCHA in the recovery tick restarts the
	// cycle.
	nowFake.Add(int64(200 * time.Millisecond))
	if c.ShouldSkip(ch) {
		t.Fatal("post-cooldown: ShouldSkip should be false")
	}
	c.RecordSuccess(ch)
	if c.ShouldSkip(ch) {
		t.Fatal("post-RecordSuccess: ShouldSkip should be false")
	}
	c.RecordCaptcha(ch)
	if !c.ShouldSkip(ch) {
		t.Fatal("post-recovery-CAPTCHA: ShouldSkip should be true again (cycle restarted)")
	}
}

// TestCaptchaCooldown_RecordSuccessClearsImmediate: a single successful
// tick after a CAPTCHA should clear the cooldown right away, NOT wait
// for the full 24h to expire. This is the "成功一次後重置" rule.
func TestCaptchaCooldown_RecordSuccessClearsImmediate(t *testing.T) {
	var nowFake atomic.Int64
	nowFake.Store(time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC).UnixNano())
	clock := func() time.Time { return time.Unix(0, nowFake.Load()) }
	c := CaptchaCooldownWith(24*time.Hour, clock)
	c.RecordCaptcha("government_broker")
	if !c.ShouldSkip("government_broker") {
		t.Fatal("expected ShouldSkip true right after RecordCaptcha")
	}
	// 1 minute later, still inside 24h, but the upstream fetch succeeded.
	nowFake.Add(int64(time.Minute))
	c.RecordSuccess("government_broker")
	if c.ShouldSkip("government_broker") {
		t.Error("RecordSuccess should clear cooldown immediately, not wait 24h")
	}
	if !c.Until("government_broker").IsZero() {
		t.Errorf("Until should be zero after RecordSuccess, got %v", c.Until("government_broker"))
	}
}

// TestCaptchaCooldown_ChannelIsolation: a CAPTCHA on channel A must
// not silence channel B.
func TestCaptchaCooldown_ChannelIsolation(t *testing.T) {
	var nowFake atomic.Int64
	nowFake.Store(time.Date(2026, 7, 31, 14, 30, 0, 0, time.UTC).UnixNano())
	clock := func() time.Time { return time.Unix(0, nowFake.Load()) }
	c := CaptchaCooldownWith(24*time.Hour, clock)
	c.RecordCaptcha("government_broker")
	if !c.ShouldSkip("government_broker") {
		t.Fatal("A should be in cooldown (24h default; injected clock; just after RecordCaptcha)")
	}
	if c.ShouldSkip("taiex_index") {
		t.Error("B should NOT be in cooldown just because A was")
	}
}

// TestCaptchaCooldown_NilSafe: nil receivers are no-ops, so call sites
// can short-circuit a cooldown check with one branchless call.
func TestCaptchaCooldown_NilSafe(t *testing.T) {
	var c *CaptchaCooldown
	if c.ShouldSkip("any") {
		t.Error("nil.ShouldSkip should return false")
	}
	if !c.Until("any").IsZero() {
		t.Error("nil.Until should return zero time")
	}
	c.RecordCaptcha("any") // must not panic
	c.RecordSuccess("any")
	if c.IsCaptchaErr(nil) {
		t.Error("nil.IsCaptchaErr(nil) should return false")
	}
	if c.IsCaptchaErr(errors.New("unrelated")) {
		t.Error("nil.IsCaptchaErr(other) should return false")
	}
}

// TestCaptchaCooldown_IsCaptchaErr_MatchesWrapped: the typed sentinel
// is detectable through fmt.Errorf("%w ...", ErrCaptchaRequired, ...)
// (the wrap format used in government_broker_aggregator.go).
func TestCaptchaCooldown_IsCaptchaErr_MatchesWrapped(t *testing.T) {
	c := NewCaptchaCooldown()
	if !c.IsCaptchaErr(ErrCaptchaRequired) {
		t.Error("bare sentinel should match")
	}
	wrapped := fmt.Errorf("%w 8060/20260730", ErrCaptchaRequired)
	if !c.IsCaptchaErr(wrapped) {
		t.Errorf("wrapped error %q should match ErrCaptchaRequired via errors.Is", wrapped)
	}
	other := errors.New("connection reset")
	if c.IsCaptchaErr(other) {
		t.Error("non-CAPTCHA err should not match")
	}
}

// TestCaptchaCooldown_ConcurrentSafety: many goroutines hammering
// RecordCaptcha / ShouldSkip / RecordSuccess must not race.
// Run with `go test -race` to catch data races.
func TestCaptchaCooldown_ConcurrentSafety(t *testing.T) {
	c := NewCaptchaCooldown()
	const goroutines = 32
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			ch := "channel"
			if id%2 == 0 {
				ch = "other"
			}
			for i := 0; i < iterations; i++ {
				switch i % 3 {
				case 0:
					c.RecordCaptcha(ch)
				case 1:
					_ = c.ShouldSkip(ch)
					_ = c.Until(ch)
				case 2:
					c.RecordSuccess(ch)
				}
			}
		}(g)
	}
	wg.Wait()
}
