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
	// MetricChannelStalenessOverageSeconds reports how much the data
	// staleness EXCEEDS the channel's contract FreshnessWindow (0 = within
	// contract). The ChannelDataStale alert keys on this instead of the raw
	// staleness gauge: raw staleness fires daily false positives for
	// channels whose upstream data timestamp legitimately lags the fetch
	// time (2026-09-04 盤查: us10y/vix data-as-of 落後 ~27h 慢性超過 24h 門檻)
	// and for low-frequency snapshots (tdcc_equity_dispersion 週快照).
	MetricChannelStalenessOverageSeconds = "atlas_channel_staleness_overage_seconds"
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
			return exportChannelHealthMetrics(d.cfg.WorkDir, d.collector, time.Now())
		},
	})
	log.Printf("[Gateway] registered channel_health_metrics_export background task (5m interval)")
}

// exportChannelHealthMetrics loads channel_health.json and emits gauges.
func exportChannelHealthMetrics(workDir string, collector *monitoring.MetricsCollector, now time.Time) error {
	if collector == nil {
		return nil
	}
	store := apigateway.NewChannelHealthStore(filepath.Join(workDir, "data/state"))
	records := store.All()
	for channelID, rec := range records {
		collector.RecordGauge(MetricChannelHealthStatus, healthStatusValue(rec.Status), map[string]string{"channel": channelID})
		// 告警降噪（2026-09-03 盤查）：known-issue 通道（twse_oddlot /
		// twse_etf / taifex-daily 等，見 monitoring/known_issues.go）的上游
		// 已停用或遷移，資料永遠不會刷新——staleness/latency gauge 只會讓
		// ChannelDataStale / ChannelFetchLatencyHigh 每 5m 誤報一次（實證：
		// twse_oddlot 資料 >24h 卻被當成異常）。status gauge 仍輸出（dashboard
		// 需要它顯示 known-issue badge），但不再對 known-issue 通道輸出
		// staleness/latency 序列。
		if monitoring.LookupKnownIssue(channelID) != nil {
			continue
		}
		if rec.LatencyMs > 0 {
			collector.RecordGauge(MetricChannelFetchLatencySeconds, float64(rec.LatencyMs)/1000.0, map[string]string{"channel": channelID})
		}
		staleSec, err := computeChannelStalenessSeconds(rec, now)
		if err == nil {
			collector.RecordGauge(MetricChannelDataStalenessSeconds, staleSec, map[string]string{"channel": channelID})
			// Contract-aware overage: only emitted when staleness exceeds
			// the contract FreshnessWindow, so the ChannelDataStale alert
			// (expr: overage > 0) fires on real pipeline breakage, not on
			// legitimate data-timestamp lag or low-frequency snapshots.
			window := apigateway.ChannelContracts().Contract(channelID).FreshnessWindow
			if window <= 0 {
				window = apigateway.StaleDataThreshold
			}
			if overage := staleSec - window.Seconds(); overage > 0 {
				collector.RecordGauge(MetricChannelStalenessOverageSeconds, overage, map[string]string{"channel": channelID})
			}
		}
	}
	return nil
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

func healthStatusValue(status string) float64 {
	switch status {
	case "ok":
		return 0
	case "warn":
		return 1
	case "error":
		return 2
	case "inactive":
		return 3
	default:
		return 4
	}
}
