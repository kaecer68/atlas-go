package apigateway

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// --- test seams -------------------------------------------------------------

// configWithFubonKey returns a minimal config whose only relevant field is the
// Fubon API key (RegisterChannelAdapters gates the fubon probe on it).
func configWithFubonKey(key string) config.Config {
	return config.Config{FubonAPIKey: key}
}

// stubFubonProbe replaces the package TCP probe for one test. The stub sees
// the candidate address so a test can make "only 127.0.0.1 answers" explicit.
func stubFubonProbe(t *testing.T, fn func(addr string) bool) {
	t.Helper()
	prev := fubonProxyProbe
	fubonProxyProbe = func(addr string, _ time.Duration) bool { return fn(addr) }
	t.Cleanup(func() { fubonProxyProbe = prev })
}

// stubFubonGlobalState restores the two package-global fubon settings plus the
// shared client that EnsureFubonAdapter touches, so arming the adapter in one
// test cannot leak into the next.
func stubFubonGlobalState(t *testing.T) {
	t.Helper()
	prevPort := fubonproxy.GetFubonProxyPort()
	t.Cleanup(func() {
		fubonproxy.SetProxyHost("fubon-proxy")
		fubonproxy.SetFubonProxyPort(prevPort)
		marketdata.ResetSharedFubonClient()
	})
}

// countingProvider records Fetch calls; used where a real channel body is not
// needed but "did the task actually fetch?" must be provable.
type countingProvider struct {
	channel string
	calls   atomic.Int64
}

func (p *countingProvider) Fetch(context.Context) (*FetchResult, error) {
	p.calls.Add(1)
	return &FetchResult{
		Data: []byte(`{"symbol":"2330"}`),
		Meta: FetchMetadata{ChannelID: p.channel},
	}, nil
}
func (p *countingProvider) HealthCheck(context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok"}, nil
}
func (p *countingProvider) RateLimit() *rate.Limiter { return rate.NewLimiter(rate.Inf, 0) }
func (p *countingProvider) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: p.channel}
}

// --- EnsureFubonAdapter -----------------------------------------------------

func TestEnsureFubonAdapter_AlreadyRegisteredSkipsProbe(t *testing.T) {
	g := newTestGateway(t)
	g.registry.Register("fubon", &mockProvider{name: "fubon"})

	probed := 0
	stubFubonProbe(t, func(string) bool {
		probed++
		return false
	})

	if !EnsureFubonAdapter(g) {
		t.Fatal("EnsureFubonAdapter() = false for an already-registered channel, want true")
	}
	if probed != 0 {
		t.Errorf("probe called %d times for an already-registered channel, want 0 (idempotent, no dial)", probed)
	}
}

func TestEnsureFubonAdapter_ProbeFailureLeavesChannelUnregistered(t *testing.T) {
	g := newTestGateway(t)
	stubFubonProbe(t, func(string) bool { return false })

	if EnsureFubonAdapter(g) {
		t.Fatal("EnsureFubonAdapter() = true with both probe addresses down, want false")
	}
	if g.HasChannel("fubon") {
		t.Error("fubon channel must NOT be registered when the proxy is unreachable")
	}
}

func TestEnsureFubonAdapter_ReachableOnBothAddrsRegisters(t *testing.T) {
	stubFubonGlobalState(t)
	g := newTestGateway(t)
	stubFubonProbe(t, func(string) bool { return true })

	if !EnsureFubonAdapter(g) {
		t.Fatal("EnsureFubonAdapter() = false when the proxy answers, want true")
	}
	if !g.HasChannel("fubon") {
		t.Fatal("fubon channel not registered after a successful probe")
	}
	if _, err := g.Provider("fubon"); err != nil {
		t.Fatalf("fubon provider not retrievable from the registry: %v", err)
	}
}

func TestEnsureFubonAdapter_LoopbackOnlyOverridesProxyHost(t *testing.T) {
	stubFubonGlobalState(t)
	g := newTestGateway(t)
	// 本機開發情境:127.0.0.1 可達、Docker DNS (fubon-proxy) 不可達。
	stubFubonProbe(t, func(addr string) bool { return strings.HasPrefix(addr, "127.0.0.1:") })

	if !EnsureFubonAdapter(g) {
		t.Fatal("EnsureFubonAdapter() = false for a loopback-only proxy, want true")
	}
	want := fmt.Sprintf("127.0.0.1:%d", fubonproxy.GetFubonProxyPort())
	if got := fubonproxy.ProxyHostPort(); got != want {
		t.Errorf("ProxyHostPort() = %q, want %q (loopback-only probe must override the proxy host)", got, want)
	}
}

// TestRegisterChannelAdapters_FubonRegistrationFollowsProbe locks the startup
// behaviour: the fubon adapter follows probe reachability but a failed probe is
// no longer a permanent verdict — the same EnsureFubonAdapter call is what
// FubonAdapterWatchTask retries later.
func TestRegisterChannelAdapters_FubonRegistrationFollowsProbe(t *testing.T) {
	t.Run("proxy down", func(t *testing.T) {
		t.Setenv("ATLAS_FUBON_API_KEY", "")
		g := newTestGateway(t)
		stubFubonProbe(t, func(string) bool { return false })
		cfg := configWithFubonKey("test-fubon-key")

		if err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil); err != nil {
			t.Fatalf("RegisterChannelAdapters failed: %v", err)
		}
		if g.HasChannel("fubon") {
			t.Error("fubon channel should not be registered while the proxy is unreachable")
		}
	})

	t.Run("proxy up", func(t *testing.T) {
		t.Setenv("ATLAS_FUBON_API_KEY", "")
		stubFubonGlobalState(t)
		g := newTestGateway(t)
		stubFubonProbe(t, func(string) bool { return true })
		cfg := configWithFubonKey("test-fubon-key")

		if err := RegisterChannelAdapters(g, t.TempDir(), cfg, nil, nil); err != nil {
			t.Fatalf("RegisterChannelAdapters failed: %v", err)
		}
		if !g.HasChannel("fubon") {
			t.Error("fubon channel should be registered when the proxy answers the probe")
		}
	})

	t.Run("no key", func(t *testing.T) {
		t.Setenv("ATLAS_FUBON_API_KEY", "")
		g := newTestGateway(t)
		probed := 0
		stubFubonProbe(t, func(string) bool {
			probed++
			return true
		})

		if err := RegisterChannelAdapters(g, t.TempDir(), configWithFubonKey(""), nil, nil); err != nil {
			t.Fatalf("RegisterChannelAdapters failed: %v", err)
		}
		if g.HasChannel("fubon") {
			t.Error("fubon channel should not be registered without a key")
		}
		if probed != 0 {
			t.Errorf("probe called %d times without a fubon key, want 0", probed)
		}
	})
}

// --- FubonAdapterWatchTask --------------------------------------------------

func TestFubonAdapterWatchTask_UnreachableRecordsWarnOnce(t *testing.T) {
	g := newTestGateway(t)
	mgr := NewBackgroundTaskManager(g)
	stubFubonProbe(t, func(string) bool { return false })

	task := FubonAdapterWatchTask(mgr, g)
	if task.Name != "fubon_adapter_watch" {
		t.Fatalf("watch task name = %q, want fubon_adapter_watch", task.Name)
	}
	if task.ChannelID != "" {
		t.Errorf("watch task ChannelID = %q, want empty (the channel is NOT registered yet by design)", task.ChannelID)
	}
	if task.Interval != FubonAdapterWatchInterval {
		t.Errorf("watch task interval = %v, want %v", task.Interval, FubonAdapterWatchInterval)
	}

	// First outage tick. The record clock starts two hours in the past so the
	// refresh behaviour below can be exercised deterministically.
	store := g.Health().store
	store.WithRecordClock(func() time.Time { return time.Now().Add(-2 * time.Hour) })
	runWatchTick(t, task)
	store.WithRecordClock(nil)

	if g.HasChannel("fubon") {
		t.Error("watch tick must not register the fubon adapter while the proxy is down")
	}
	if _, ok := mgr.Get("channel_health_fubon"); ok {
		t.Error("channel_health_fubon must NOT be registered while the fubon channel is missing (Register would be rejected)")
	}

	rec := g.Health().Get("fubon")
	if rec == nil {
		t.Fatal("expected a health record for fubon so the outage is visible on the channel page")
	}
	if rec.Status != "warn" {
		t.Errorf("fubon health status = %q, want warn (visible on /admin/datachannels as 待更新, not filtered by Alerts())", rec.Status)
	}
	if !strings.Contains(rec.LastError, "not reachable") {
		t.Errorf("fubon health LastError = %q, want it to explain the unreachable proxy", rec.LastError)
	}
	afterFirst := countChannelFetches(g, "fubon")
	if afterFirst != 1 {
		t.Fatalf("fubon fetch-log entries = %d after the first outage tick, want 1", afterFirst)
	}

	// The aged record (> fubonGapRefreshInterval) is refreshed so the gap keeps a
	// current LastFetchAt instead of ageing out of the 48h window Alerts()
	// applies — the exact blind spot the 2026-09-21 outage fell into.
	runWatchTick(t, task)
	if after := countChannelFetches(g, "fubon"); after != afterFirst+1 {
		t.Errorf("fubon fetch-log entries = %d after the refresh tick, want %d", after, afterFirst+1)
	}
	if rec := g.Health().Get("fubon"); rec == nil || rec.Status != "warn" {
		t.Errorf("fubon status after the refresh tick = %+v, want warn", rec)
	}

	// A further tick inside the refresh window must not add another entry: one
	// record per 3m tick would flood the fetch log over a long outage.
	before := countChannelFetches(g, "fubon")
	runWatchTick(t, task)
	if after := countChannelFetches(g, "fubon"); after != before {
		t.Errorf("fubon fetch-log entries = %d after a tick inside the refresh window, want %d", after, before)
	}
	if rec := g.Health().Get("fubon"); rec == nil || rec.Status != "warn" {
		t.Errorf("fubon status after a repeated failing tick = %+v, want warn", rec)
	}
}

func TestFubonAdapterWatchTask_SelfHealsAndAttachesHealthProbe(t *testing.T) {
	stubFubonGlobalState(t)
	g := newTestGateway(t)
	mgr := NewBackgroundTaskManager(g)

	// Startup: proxy down → no adapter.
	var reachable atomic.Bool
	stubFubonProbe(t, func(string) bool { return reachable.Load() })
	task := FubonAdapterWatchTask(mgr, g)
	runWatchTick(t, task)
	if g.HasChannel("fubon") {
		t.Fatal("precondition failed: fubon channel registered while the proxy is down")
	}

	// Proxy comes up after startup → the next watch tick repairs the channel
	// and attaches the health probe.
	reachable.Store(true)
	runWatchTick(t, task)

	if !g.HasChannel("fubon") {
		t.Fatal("fubon channel not registered after the proxy became reachable (self-heal failed)")
	}
	healthTask, ok := mgr.Get("channel_health_fubon")
	if !ok {
		t.Fatal("channel_health_fubon was not attached after the self-heal — the channel would have no health probe again")
	}
	if healthTask.ChannelID != "fubon" {
		t.Errorf("attached health task ChannelID = %q, want fubon", healthTask.ChannelID)
	}

	// Further ticks must not duplicate the registration.
	runWatchTick(t, task)
	if got := len(mgr.List()); got != 1 {
		t.Errorf("registered tasks after repeated ticks = %v, want exactly [channel_health_fubon]", mgr.List())
	}
}

func TestFubonAdapterWatchTask_AttachesHealthProbeWhenChannelAlreadyRegistered(t *testing.T) {
	g := newTestGateway(t)
	mgr := NewBackgroundTaskManager(g)
	g.registry.Register("fubon", &mockProvider{name: "fubon"})

	runWatchTick(t, FubonAdapterWatchTask(mgr, g))

	if _, ok := mgr.Get("channel_health_fubon"); !ok {
		t.Fatal("watch tick must attach channel_health_fubon when the fubon channel exists but the task is missing")
	}
}

// --- FubonHealthProbeTask ---------------------------------------------------

func TestFubonHealthProbeTask_ShapeAndFetch(t *testing.T) {
	g := newTestGateway(t)
	provider := &countingProvider{channel: "fubon"}
	g.registry.Register("fubon", provider)

	task := FubonHealthProbeTask(g)
	if task.Name != "channel_health_fubon" {
		t.Errorf("task name = %q, want channel_health_fubon", task.Name)
	}
	if task.ChannelID != "fubon" {
		t.Errorf("task ChannelID = %q, want fubon", task.ChannelID)
	}
	if task.Interval != time.Hour {
		t.Errorf("task interval = %v, want 1h", task.Interval)
	}
	if !task.Enabled {
		t.Error("task must be enabled")
	}

	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task.Task() = %v, want nil", err)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Errorf("gateway fetch calls = %d, want 1 (the probe must fetch through Gateway.Fetch)", got)
	}
}

// --- helpers ----------------------------------------------------------------

func runWatchTick(t *testing.T, task *ScheduledTask) {
	t.Helper()
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("watch tick returned %v, want nil (an unreachable proxy is an expected state, not a task failure)", err)
	}
}

func countChannelFetches(g *Gateway, channelID string) int {
	n := 0
	for _, e := range g.Health().store.RecentFetches(200) {
		if e.Channel == channelID {
			n++
		}
	}
	return n
}
