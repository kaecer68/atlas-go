package main

// Channel-health metrics export task.
//
// BackgroundTaskManager task that periodically loads channel_health.json and
// emits Prometheus gauges for per-channel data staleness, fetch latency, and
// health status. These gauges power the per-channel latency/staleness alert
// rules in monitoring/rules/channel_health_latent_staleness.yml.

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

const (
	MetricChannelDataStalenessSeconds = "atlas_channel_data_staleness_seconds"
	MetricChannelFetchLatencySeconds  = "atlas_channel_fetch_latency_seconds"
	MetricChannelHealthStatus         = "atlas_channel_health_status"
	// MetricChannelConsecutiveFailures reports the failed-attempt streak
	// behind the derived status (k3 audit R1, 2026-09-08): lets dashboards
	// see a channel approaching the error escalation before it pages.
	MetricChannelConsecutiveFailures = "atlas_channel_consecutive_failures"
	// MetricChannelStalenessOverageSeconds reports how much the data
	// staleness EXCEEDS the channel's contract FreshnessWindow (0 = within
	// contract). The ChannelDataStale alert keys on this instead of the raw
	// staleness gauge: raw staleness fires daily false positives for
	// channels whose upstream data timestamp legitimately lags the fetch
	// time (2026-09-04 盤查: us10y/vix data-as-of 落後 ~27h 慢性超過 24h 門檻)
	// and for low-frequency snapshots (tdcc_equity_dispersion 週快照).
	MetricChannelStalenessOverageSeconds = "atlas_channel_staleness_overage_seconds"
	// MetricChannelGovernanceOverdue reports, for a channel whose entry declares
	// a PERMANENT upstream removal (KnownIssue.UpstreamRemovedAt), whether the
	// "must be retired or fixed" criterion (issue #2138) currently holds:
	// 1 = yes, 0 = no. Only known-issue entries take part, so the series set is
	// bounded by the registry (a channel with no known issue never produces one)
	// — the same cardinality discipline the rest of this family follows.
	//
	// It does NOT replace ChannelHealthStatusError and no alert rule reads it:
	// the criterion's paging pressure comes from the existing status gauge, while
	// this series (plus the one-shot log line below) is the governance signal an
	// operator looks at when deciding retire-or-fix. Any future rule on it must
	// be calibrated against real data first — the measured baseline right after
	// the #2136/#2134 deploy is 0 overdue channels.
	MetricChannelGovernanceOverdue = "atlas_channel_governance_overdue"
)

// registerChannelHealthMetricsTask wires the channel_health_metrics_export
// background task into the BackgroundTaskManager.
func registerChannelHealthMetricsTask(d backfillDeps) {
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "channel_health_metrics_export",
		ChannelID: "",
		Interval:  5 * time.Minute,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			// The notifier is process-scoped on purpose: it is what turns "overdue"
			// into ONE reminder instead of a line every 5 minutes.
			return exportChannelHealthMetrics(d.cfg.WorkDir, d.collector, time.Now(), governanceNotifier)
		},
	})
	log.Printf("[Gateway] registered channel_health_metrics_export background task (5m interval)")
}

// governanceNotifier is the process-scoped "remind once" state for the #2138
// governance criterion (see monitoring.GovernanceNotifier).
var governanceNotifier = monitoring.NewGovernanceNotifier()

// exportChannelHealthMetrics loads channel_health.json and emits gauges.
//
// gov may be nil: the gauges are still exported, only the one-shot reminder log
// is skipped (callers that only want metrics pass nil).
func exportChannelHealthMetrics(workDir string, collector *monitoring.MetricsCollector, now time.Time, gov *monitoring.GovernanceNotifier) error {
	if collector == nil {
		return nil
	}
	store := apigateway.NewChannelHealthStore(filepath.Join(workDir, "data/state"))
	records := store.All()
	contracts := apigateway.ChannelContracts()
	for channelID, rec := range records {
		// Status gauge carries the contract-aware verdict, not the raw record
		// status (2026-09-24 channel-status-truth): a channel whose last fetch
		// is older than its freshness window must not be exported as ok while
		// the admin page and the health summary call it stale. 0=ok,1=warn,
		// 2=error,3=inactive,4=other — the alert rules only match ==2, so the
		// stale value (warn) neither pages nor changes any existing rule.
		status := apigateway.DeriveChannelStatus(&rec, contracts.Contract(channelID), now)
		collector.RecordGauge(MetricChannelHealthStatus, healthStatusValue(status), map[string]string{"channel": channelID})
		if rec.ConsecutiveFailures > 0 {
			collector.RecordGauge(MetricChannelConsecutiveFailures, float64(rec.ConsecutiveFailures), map[string]string{"channel": channelID})
		}
		// 告警降噪（2026-09-03 盤查）：known-issue 通道（twse_oddlot /
		// twse_etf / taifex-daily 等，見 monitoring/known_issues.go）的上游
		// 已停用或遷移，資料永遠不會刷新——staleness/latency gauge 只會讓
		// ChannelDataStale / ChannelFetchLatencyHigh 每 5m 誤報一次（實證：
		// twse_oddlot 資料 >24h 卻被當成異常）。status gauge 仍輸出（dashboard
		// 需要它顯示 known-issue badge），但不再對 known-issue 通道輸出
		// staleness/latency 序列。
		if issue := monitoring.LookupKnownIssue(channelID); issue != nil {
			// Governance (issue #2138): only entries that declare a PERMANENT
			// upstream removal take part, so the series set stays bounded by the
			// registry. The verdict is computed from the live record, the
			// channel's own contract window, and the registry deadline.
			if dl := monitoring.EvaluateGovernanceDeadline(channelID, *issue, now); dl.AvailabilityCase {
				decision := monitoring.EvaluateChannelGovernance(issue, &rec, contracts.Contract(channelID), now)
				value := 0.0
				if decision.ShouldRetire || decision.Overdue {
					value = 1
				}
				collector.RecordGauge(MetricChannelGovernanceOverdue, value, map[string]string{"channel": channelID})
				// The reminder goes through governanceReminderLine — the SAME
				// function the test drives. An inline log.Printf here would make
				// the tested function production-dead code and would let a future
				// edit of this line change the shipped behavior with no test
				// failing (the "green test, different production path" trap the
				// third-party review caught on 2026-09-30).
				if line, emit := governanceReminderLine(gov, channelID, decision, now); emit {
					log.Print(line)
				}
			}
			continue
		}
		// R3 (k3 audit): derived indicator records (provenance=derived —
		// unregistered IDs like vix/us10y, tagged at write/janitor time)
		// keep the status gauge but skip staleness/latency/overage gauges.
		if rec.Provenance == apigateway.ProvenanceDerived {
			continue
		}
		if rec.LatencyMs > 0 {
			collector.RecordGauge(MetricChannelFetchLatencySeconds, float64(rec.LatencyMs)/1000.0, map[string]string{"channel": channelID})
		}
		staleSec, err := computeChannelStalenessSeconds(rec, now)
		if err == nil {
			collector.RecordGauge(MetricChannelDataStalenessSeconds, staleSec, map[string]string{"channel": channelID})
			// Contract-aware overage: how much staleness EXCEEDS the
			// contract FreshnessWindow, so the ChannelDataStale alert
			// (expr: overage > 0) fires on real pipeline breakage, not on
			// legitimate data-timestamp lag or low-frequency snapshots.
			//
			// 2026-09-05 修復（實證: twse_replay_sync 恢復後 alert 仍每小時
			// 重複通知）: overage 必須永遠輸出（窗口內 = 0）。MetricsCollector
			// 的 gauge 是 last-write-wins 且不清序列——若條件消失就跳過輸出,
			// 舊樣本會永久凍結在 /metrics,Prometheus 永遠看到 >0,alert
			// 永遠 firing。輸出 0 讓序列持續更新,alert 自然 resolved。
			window := apigateway.ChannelContracts().Contract(channelID).FreshnessWindow
			if window <= 0 {
				window = apigateway.StaleDataThreshold
			}
			overage := staleSec - window.Seconds()
			if overage < 0 {
				overage = 0
			}
			collector.RecordGauge(MetricChannelStalenessOverageSeconds, overage, map[string]string{"channel": channelID})
		}
	}
	return nil
}

// governanceReminderLine decides whether this observation must produce a
// reminder, and formats it. It is a separate function for one reason: the
// "once, not continuously" property is the whole point of the reminder, and a
// property that can only be observed by capturing the process log is a property
// that will not be tested. With the notifier and `now` injected, the transition
// is a pure function.
func governanceReminderLine(gov *monitoring.GovernanceNotifier, channelID string, decision monitoring.GovernanceDecision, now time.Time) (string, bool) {
	overdue := decision.ShouldRetire || decision.Overdue
	if !gov.Note(channelID, overdue, now) {
		return "", false
	}
	return fmt.Sprintf(
		"[Gateway] channel_governance_overdue channel=%s status=%s data_age_windows=%.1f "+
			"deadline_passed=%t should_retire=%t reasons=%v — a permanently unavailable channel with a "+
			"replacement input must be retired or fixed (issue #2138); widening a window or suppressing the "+
			"alert is concealment, not a fix",
		channelID, decision.Status, decision.DataAgeWindows, decision.Overdue, decision.ShouldRetire, decision.Reasons), true
}

func computeChannelStalenessSeconds(rec apigateway.ChannelHealthRecord, now time.Time) (float64, error) {
	// Prefer LastDataAt (when the upstream data itself was produced), fall back
	// to LastFetchAt (when we last tried to refresh it).
	timeField := rec.LastDataAt
	if timeField == "" {
		timeField = rec.LastFetchAt
	}
	if timeField == "" {
		return 0, fmt.Errorf("no timestamp available")
	}
	t, err := time.Parse(time.RFC3339, timeField)
	if err != nil {
		return 0, err
	}
	staleSec := now.Sub(t).Seconds()
	if staleSec < 0 {
		staleSec = 0
	}
	return staleSec, nil
}

// healthStatusValue maps a channel status to the atlas_channel_health_status
// gauge value: 0=ok, 1=warn, 2=error, 3=inactive, 4=other/unmapped.
//
// "stale" (derived: last fetch older than the contract freshness window) maps
// to warn: it is a real warning an operator should see, but it must not page —
// every alert rule on this gauge matches == 2 (error), and the
// ok-but-untouched-for-weeks case is already reported by
// atlas_channel_staleness_overage_seconds.
//
// "degraded" (the fetch succeeded while the payload was empty/stale/partial)
// maps to warn for the same reason (E29-3, 2026-09-27). It used to fall through
// to 4 "other/unmapped": no alert rule matches 1 or 4 either way, but 4 made the
// series unreadable (a defined non-ok verdict exported as "unmapped"). A degraded
// channel that outlives its freshness window no longer stops here — DeriveChannelStatus
// escalates it to error (→ 2), which is what actually pages.
func healthStatusValue(status string) float64 {
	switch status {
	case "ok":
		return 0
	case "warn", "stale", "degraded":
		return 1
	case "error":
		return 2
	case "inactive":
		return 3
	default:
		return 4
	}
}
