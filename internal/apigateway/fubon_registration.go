package apigateway

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FubonAdapterWatchInterval is the cadence of FubonAdapterWatchTask, the
// self-healing registration watch. 3m keeps the worst-case window between
// "fubon-proxy became reachable" and "fubon channel is registered again"
// short while the probe itself stays negligible (two 2s-bounded TCP dials).
const FubonAdapterWatchInterval = 3 * time.Minute

// fubonHealthProbeInterval matches the original cmd/atlas channel_health_fubon
// cadence (1h). The probe is also the channel's data refresh: Gateway.Fetch
// writes the fubon snapshot consumed downstream.
const fubonHealthProbeInterval = time.Hour

// fubonProxyDialTimeout bounds each reachability probe (unchanged from the
// original startup probe in RegisterChannelAdapters).
const fubonProxyDialTimeout = 2 * time.Second

// fubonGapRefreshInterval is how often the "adapter not registered" health
// record is refreshed while the proxy stays unreachable. It keeps the record's
// LastFetchAt current (Alerts() skips records older than 48h) without writing a
// fetch-log entry on every 3m watch tick.
const fubonGapRefreshInterval = time.Hour

// fubonProxyProbe is the TCP reachability probe. It is a package variable so
// unit tests can simulate "proxy up/down" without binding a real socket.
var fubonProxyProbe = func(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// FubonProxyAddrs returns the two candidate fubon-proxy endpoints:
// 127.0.0.1:<port> (local development, go run on the host) and
// fubon-proxy:<port> (Docker DNS, container deployments). Both come from the
// fubonproxy single source of truth, so they cannot drift from the port set by
// the cmd/atlas -fubon-port flag.
func FubonProxyAddrs() (localAddr, dockerAddr string) {
	return fmt.Sprintf("127.0.0.1:%d", fubonproxy.GetFubonProxyPort()), fubonproxy.ProxyHostPort()
}

// FubonKeyConfigured reports whether a Fubon API key is available: the config
// field first, then the ATLAS_FUBON_API_KEY secret (same precedence as the
// historical inline check in RegisterChannelAdapters).
//
// The adapter registration, the channel_health_fubon task wiring in cmd/atlas
// and the self-heal watch MUST use this one predicate. The cmd/atlas health
// task previously gated on cfg.FubonAPIKey alone, so a secret-only deployment
// registered the adapter but never its health probe (a silent second gap).
func FubonKeyConfigured(cfg config.Config) bool {
	return cfg.FubonAPIKey != "" || config.GetSecret("ATLAS_FUBON_API_KEY") != ""
}

// FubonRegistrationGapMessage explains why the fubon adapter is not in the
// registry. Shared by the startup log line and the outage health record so the
// operator sees the same addresses in both places.
func FubonRegistrationGapMessage() string {
	localAddr, dockerAddr := FubonProxyAddrs()
	return fmt.Sprintf("fubon-proxy not reachable on %s or %s", localAddr, dockerAddr)
}

// EnsureFubonAdapter registers the "fubon" channel adapter when the local
// fubon-proxy is reachable, and reports whether the channel is registered
// afterwards. Idempotent: an already-registered channel short-circuits without
// probing, so it is safe to call from a background watch.
//
// Why this function exists (2026-09-21 incident): registration used to be a
// single startup decision. atlas-go probed fubon-proxy 0.18s before the proxy
// container started listening, logged "fubon_proxy_not_reachable … skipping
// fubon adapter registration", and never retried — the channel stayed
// unregistered for the whole process lifetime, fubon data stopped flowing, and
// the channel health record kept reporting "ok" (no task was ever registered
// to notice). Making registration a repeatable, reachability-gated operation is
// what removes the "permanently disabled by one lost race" failure mode.
//
// Returns false when neither candidate address is reachable. The caller
// decides how to surface that: FubonAdapterWatchTask records a "warn" channel
// health record and retries on the next tick.
func EnsureFubonAdapter(g *Gateway) bool {
	if g == nil {
		return false
	}
	if g.HasChannel("fubon") {
		return true
	}

	localAddr, dockerAddr := FubonProxyAddrs()
	localOK := fubonProxyProbe(localAddr, fubonProxyDialTimeout)
	proxyOK := fubonProxyProbe(dockerAddr, fubonProxyDialTimeout)
	if !localOK && !proxyOK {
		return false
	}

	// 若只有本機 loopback 可達,覆寫 proxy host 為 127.0.0.1。順序很重要:
	// 必須在 GetSharedFubonClient() 之前,因為後者用 sync.Once 鎖定
	// fubonproxy.ProxyBaseURL() 的當下值。
	if localOK && !proxyOK {
		fubonproxy.SetProxyHost("127.0.0.1")
		logging.Info("apigateway", "fubon_proxy_host_override",
			"msg", "fubon-proxy only reachable on 127.0.0.1, overriding proxy host")
	}

	// The adapter gets the gateway's configured work dir: its Fetch persists the
	// L3 channel snapshot (see saveSnapshot), which must not follow the CWD.
	g.registry.Register("fubon", NewFubonChannelAdapter(marketdata.GetSharedFubonClient(), g.WorkDir()))
	logging.Info("apigateway", "adapter_registered", "channel", "fubon")
	return true
}

// FubonHealthProbeTask returns the periodic fubon channel health probe
// (task "channel_health_fubon"). Single definition: cmd/atlas registers it at
// startup and FubonAdapterWatchTask attaches it after a late self-heal, so the
// task name, ChannelID and interval cannot drift apart between the two paths.
func FubonHealthProbeTask(g *Gateway) *ScheduledTask {
	return &ScheduledTask{
		Name:      "channel_health_fubon",
		ChannelID: "fubon",
		Interval:  fubonHealthProbeInterval,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			_, err := g.Fetch(ctx, "fubon")
			return err
		},
	}
}

// FubonAdapterWatchTask returns the task that repairs the fubon registration
// after a failed startup probe (see EnsureFubonAdapter) and attaches the
// channel_health_fubon probe once the channel exists.
//
// ChannelID is intentionally empty: Register rejects a ChannelID that is not
// yet in the gateway, and this task exists precisely for the window in which
// "fubon" is NOT registered. The task itself never fails on an unreachable
// proxy — a proxy that stays down is an expected state, and the operator
// signal lives in the channel health record (status "warn"), not in a
// task_failed every 3 minutes.
//
// When the watch attaches channel_health_fubon at runtime, that probe inherits
// the manager's startup stagger (bounded by interval/10, i.e. ≤6m for a 1h
// task), so the first fubon fetch after a self-heal happens within ~6 minutes.
func FubonAdapterWatchTask(taskMgr *BackgroundTaskManager, g *Gateway) *ScheduledTask {
	return &ScheduledTask{
		Name:     "fubon_adapter_watch",
		Interval: FubonAdapterWatchInterval,
		Enabled:  true,
		Task: func(context.Context) error {
			return watchFubonRegistration(taskMgr, g)
		},
	}
}

// watchFubonRegistration runs one self-heal tick.
func watchFubonRegistration(taskMgr *BackgroundTaskManager, g *Gateway) error {
	if g == nil {
		return nil
	}

	if !g.HasChannel("fubon") {
		if !EnsureFubonAdapter(g) {
			recordFubonRegistrationGap(g)
			return nil
		}
		logging.Info("apigateway", "fubon_adapter_selfhealed",
			"msg", "fubon-proxy became reachable after startup; fubon adapter registered by fubon_adapter_watch")
	}

	ensureFubonHealthProbeTask(taskMgr, g)
	return nil
}

// ensureFubonHealthProbeTask attaches channel_health_fubon when the channel is
// registered and no probe task exists yet.
//
// RegisterAndStart (not Register) is required here: BackgroundTaskManager.Start
// snapshots the registry once, so a task registered after start would sit in the
// map without ever running — the exact "health task registered on paper, never
// scheduled" shape this PR removes.
func ensureFubonHealthProbeTask(taskMgr *BackgroundTaskManager, g *Gateway) {
	if taskMgr == nil || g == nil || !g.HasChannel("fubon") {
		return
	}
	if _, exists := taskMgr.Get("channel_health_fubon"); exists {
		return
	}
	if err := taskMgr.RegisterAndStart(FubonHealthProbeTask(g)); err != nil {
		logging.Warn("apigateway", "fubon_health_task_register_failed", "err", err.Error())
		return
	}
	logging.Info("apigateway", "fubon_health_task_attached",
		"msg", "channel_health_fubon registered after fubon adapter became available")
}

// recordFubonRegistrationGap keeps an unreachable fubon-proxy visible on the
// channel page, so "adapter never registered" can no longer hide behind a
// stale "ok" record (the 2026-09-21 outage).
//
// Status "warn" is deliberate, not "error" and not "inactive":
//   - ChannelHealthStore.recordInternal passes "warn" through without
//     incrementing the failure streak, so an intentionally absent proxy does
//     not escalate into continuous ChannelHealthStatusError paging (the same
//     noise discipline the original startup probe was protecting).
//   - It is NOT filtered by Alerts() (only "ok" and "inactive" are), so the gap
//     stays visible to the operator and the alert evaluator.
//   - monitoring StatusText maps "warn" to 待更新 and the row carries
//     last_error = the probe addresses, so /admin/datachannels explains itself
//     instead of showing 未知.
//
// The record is (re)written at most once per fubonGapRefreshInterval instead of
// once per 3m tick: the fetch log stays clean, while LastFetchAt keeps moving
// so the record never ages out of the 48h window that Alerts() applies.
func recordFubonRegistrationGap(g *Gateway) {
	if g == nil {
		return
	}

	transition := true
	if rec := g.Health().Get("fubon"); rec != nil && rec.Status == "warn" {
		if ts, err := time.Parse(time.RFC3339, rec.LastFetchAt); err == nil && time.Since(ts) < fubonGapRefreshInterval {
			return
		}
		transition = false
	}

	msg := fmt.Sprintf("%s — adapter not registered; fubon_adapter_watch retries every %s",
		FubonRegistrationGapMessage(), FubonAdapterWatchInterval)
	if err := g.Health().Record("fubon", "warn", msg); err != nil {
		logging.Warn("apigateway", "fubon_registration_gap_record_failed", "err", err.Error())
	}
	if transition {
		// Log on the transition only; the hourly refresh above keeps the state
		// visible without repeating this line 480 times a day.
		logging.Warn("apigateway", "fubon_registration_gap", "msg", msg)
	}
}
