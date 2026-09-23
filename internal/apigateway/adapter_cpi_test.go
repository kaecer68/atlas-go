package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestUSCPIChannelAdapter_Metadata(t *testing.T) {
	a := &USCPIChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "us_cpi" {
		t.Errorf("ChannelID = %q, want us_cpi", m.ChannelID)
	}
	if m.Country != "美國" {
		t.Errorf("Country = %q, want 美國", m.Country)
	}
	if m.Platform != "BLS" {
		t.Errorf("Platform = %q, want BLS", m.Platform)
	}
	if m.APIFormat != "REST JSON" {
		t.Errorf("APIFormat = %q, want REST JSON", m.APIFormat)
	}
	if m.Path != "api.bls.gov" {
		t.Errorf("Path = %q, want api.bls.gov", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestUSCPIChannelAdapter_RateLimit(t *testing.T) {
	a := NewUSCPIChannelAdapter(nil)
	if a == nil {
		t.Fatal("NewUSCPIChannelAdapter returned nil")
	}
	if a.RateLimit() == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

// --- Fetch behavior (limiter interplay + last-known-good) -----------------

// stubCPIProvider stands in for *marketdata.BLSCPIProvider so the limiter and
// last-known-good paths can be exercised without a network call.
type stubCPIProvider struct {
	snap  marketdata.MacroDataSnapshot
	err   error
	calls int
}

func (s *stubCPIProvider) FetchSnapshot(context.Context) (marketdata.MacroDataSnapshot, error) {
	s.calls++
	return s.snap, s.err
}

func cpiFixtureSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{CPIYoY: marketdata.MacroDataPoint{
		Symbol: "CUUR0000SA0", Value: 2.9, ChangePct: 0.1, Timestamp: 1758200000,
	}}
}

func newCPIAdapterWithStub(p *stubCPIProvider, window time.Duration) *USCPIChannelAdapter {
	return &USCPIChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Every(window), 1)}
}

// TestUSCPIChannelAdapter_Fetch_ThrottledServesLastKnownGood locks the fan-out
// safety property: once the channel's 1/hour token is spent, Fetch must return
// promptly (bounded by cpiLimiterMaxWait, not by the caller's ~30s fan-out
// budget) and must keep serving the last-known-good payload so
// MacroDataSnapshot.CPIYoY does not drop out of the runtime snapshot.
func TestUSCPIChannelAdapter_Fetch_ThrottledServesLastKnownGood(t *testing.T) {
	provider := &stubCPIProvider{snap: cpiFixtureSnapshot()}
	a := newCPIAdapterWithStub(provider, time.Hour)

	first, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	if first.Meta.Stale || first.Meta.Cached {
		t.Errorf("first Fetch must be a fresh upstream result, got %+v", first.Meta)
	}

	start := time.Now()
	second, err := a.Fetch(context.Background())
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("second Fetch (throttled) returned error, want last-known-good: %v", err)
	}
	if elapsed > cpiLimiterMaxWait {
		t.Fatalf("second Fetch blocked for %s, want <= %s (fan-out has no per-channel timeout)", elapsed, cpiLimiterMaxWait)
	}
	if provider.calls != 1 {
		t.Errorf("provider calls = %d, want 1: the throttled call must not reach BLS", provider.calls)
	}
	if !second.Meta.Cached || !second.Meta.Stale || !second.Meta.Fallback {
		t.Errorf("throttled serve must be marked cached/stale/fallback, got %+v", second.Meta)
	}

	var got marketdata.MacroDataSnapshot
	if err := json.Unmarshal(second.Data, &got); err != nil {
		t.Fatalf("unmarshal last-known-good payload: %v", err)
	}
	if got.CPIYoY.Symbol != "CUUR0000SA0" || got.CPIYoY.Value != 2.9 {
		t.Errorf("CPIYoY = %+v, want the last-known-good payload", got.CPIYoY)
	}
}

// TestUSCPIChannelAdapter_Fetch_ThrottledWithoutLastKnownGood covers the cold
// case: nothing to fall back on (the previous attempt consumed the token and
// then failed). The adapter must fail fast and non-alarmingly (ErrNoData maps
// to gateway RecordWaiting) instead of blocking for the refill window.
func TestUSCPIChannelAdapter_Fetch_ThrottledWithoutLastKnownGood(t *testing.T) {
	provider := &stubCPIProvider{err: errors.New("bls 503")}
	a := newCPIAdapterWithStub(provider, time.Hour)

	if _, err := a.Fetch(context.Background()); err == nil {
		t.Fatal("first Fetch (provider error) must fail")
	}

	start := time.Now()
	_, err := a.Fetch(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("second Fetch must fail: no last-known-good payload exists yet")
	}
	if !errors.Is(err, marketdata.ErrNoData) {
		t.Errorf("error = %v, want it to wrap marketdata.ErrNoData", err)
	}
	if elapsed > cpiLimiterMaxWait {
		t.Errorf("second Fetch blocked for %s, want <= %s", elapsed, cpiLimiterMaxWait)
	}
	if provider.calls != 1 {
		t.Errorf("provider calls = %d, want 1", provider.calls)
	}
}

// TestUSCPIChannelAdapter_Fetch_ShortWaitIsHonored keeps the bound honest: a
// token that is only shortly away must still be waited for (and then used for
// a real fetch), rather than being treated as throttled.
func TestUSCPIChannelAdapter_Fetch_ShortWaitIsHonored(t *testing.T) {
	provider := &stubCPIProvider{snap: cpiFixtureSnapshot()}
	a := newCPIAdapterWithStub(provider, 50*time.Millisecond)

	if _, err := a.Fetch(context.Background()); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	second, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	if provider.calls != 2 {
		t.Errorf("provider calls = %d, want 2 (delay %s <= %s must be waited for)", provider.calls, 50*time.Millisecond, cpiLimiterMaxWait)
	}
	if second.Meta.Stale {
		t.Error("a fetch that acquired a token must not be reported stale")
	}
}
