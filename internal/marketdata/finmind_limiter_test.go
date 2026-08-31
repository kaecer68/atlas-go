package marketdata

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestNewFinMindRateLimiter_DefaultFreeTier - env unset -> free-tier 600/hr,
// burst 60 (the historical behavior).
func TestNewFinMindRateLimiter_DefaultFreeTier(t *testing.T) {
	t.Setenv("FINMIND_RATE_LIMIT_PER_HOUR", "")
	l := newFinMindRateLimiter()
	want := rate.Limit(600) / 3600
	if got := l.Limit(); got < want*0.99 || got > want*1.01 {
		t.Errorf("limit = %v, want ~%v", got, want)
	}
	if l.Burst() != 60 {
		t.Errorf("burst = %d, want 60", l.Burst())
	}
}

// TestNewFinMindRateLimiter_SponsorOverride - FINMIND_RATE_LIMIT_PER_HOUR=6000
// (Sponsor tier, #1742) raises the local budget so the auto_cycle_update
// startup stampede no longer self-throttles below the paid quota.
func TestNewFinMindRateLimiter_SponsorOverride(t *testing.T) {
	t.Setenv("FINMIND_RATE_LIMIT_PER_HOUR", "6000")
	l := newFinMindRateLimiter()
	want := rate.Limit(6000) / 3600
	if got := l.Limit(); got < want*0.99 || got > want*1.01 {
		t.Errorf("limit = %v, want ~%v", got, want)
	}
	if l.Burst() != 300 {
		t.Errorf("burst = %d, want 300 (capped)", l.Burst())
	}
}

// TestFinmindRateLimitPerHour_InvalidEnvFallsBack - garbage values fall back
// to the free-tier default instead of disabling the limiter.
func TestFinmindRateLimitPerHour_InvalidEnvFallsBack(t *testing.T) {
	for _, bad := range []string{"abc", "-5", "0"} {
		t.Setenv("FINMIND_RATE_LIMIT_PER_HOUR", bad)
		if got := finmindRateLimitPerHour(); got != finmindRateLimitFree {
			t.Errorf("env=%q -> %d, want default %d", bad, got, finmindRateLimitFree)
		}
	}
}

// TestFinMindClient_UsesConfiguredLimiter - the client constructor wires the
// env-configured limiter.
func TestFinMindClient_UsesConfiguredLimiter(t *testing.T) {
	t.Setenv("FINMIND_RATE_LIMIT_PER_HOUR", "6000")
	c := NewFinMindClient("test-key")
	l := c.RateLimiter()
	if l == nil {
		t.Fatal("RateLimiter() = nil")
	}
	if l.Burst() != 300 {
		t.Errorf("client burst = %d, want 300", l.Burst())
	}
	_ = time.Second
}
