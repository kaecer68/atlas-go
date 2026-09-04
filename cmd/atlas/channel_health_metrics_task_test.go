package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestExportChannelHealthMetrics_EmitsStalenessLatencyStatus(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	wrapper := struct {
		Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
	}{
		Channels: map[string]*apigateway.ChannelHealthRecord{
			"capital_flow": {
				Status:     "ok",
				LastDataAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
				LatencyMs:  250,
			},
			"finmind": {
				Status:      "error",
				LastFetchAt: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339),
				LatencyMs:   5000,
			},
			// Known-issue channel (upstream removed — see
			// internal/monitoring/known_issues.go): must NOT emit
			// staleness/latency series (they only feed false
			// ChannelDataStale / ChannelFetchLatencyHigh alerts), but the
			// status gauge stays for the dashboard known-issue badge.
			"twse_oddlot": {
				Status:     "error",
				LastDataAt: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
				LatencyMs:  5000,
			},
		},
	}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "channel_health.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	collector := monitoring.NewMetricsCollector()
	if err := exportChannelHealthMetrics(dir, collector, now); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	body := rec.Body.String()

	mustContain := []string{
		`atlas_channel_health_status{channel="capital_flow"} 0`,
		`atlas_channel_fetch_latency_seconds{channel="capital_flow"} 0.25`,
		`atlas_channel_data_staleness_seconds{channel="capital_flow"} 1800`,
		`atlas_channel_health_status{channel="finmind"} 2`,
		`atlas_channel_fetch_latency_seconds{channel="finmind"} 5`,
		`atlas_channel_data_staleness_seconds{channel="finmind"} 172800`,
		// known-issue 通道仍輸出 status gauge（dashboard badge 需要），
		`atlas_channel_health_status{channel="twse_oddlot"} 2`,
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", want, body)
		}
	}
	// 但不得輸出 staleness/latency 序列（2026-09-03 告警降噪：這些序列只會
	// 讓 ChannelDataStale / ChannelFetchLatencyHigh 對已停用上游誤報）。
	mustAbsent := []string{
		`atlas_channel_fetch_latency_seconds{channel="twse_oddlot"}`,
		`atlas_channel_data_staleness_seconds{channel="twse_oddlot"}`,
	}
	for _, absent := range mustAbsent {
		if strings.Contains(body, absent) {
			t.Fatalf("unexpected /metrics line %q for known-issue channel\n--- full body ---\n%s", absent, body)
		}
	}
}

// TestExportChannelHealthMetrics_StalenessOverageRespectsContract —
// 2026-09-04 告警降噪: atlas_channel_staleness_overage_seconds 只在
// staleness 超出該通道契約 FreshnessWindow 時輸出。
//   - us10y（無契約 → 預設 48h 窗）staleness 27h: 資料時間戳合法落後 → 不得輸出
//     （舊 raw >24h 規則對它日日誤報,實證 2026-09-03/04）。
//   - twse_replay_sync（無契約 → 預設 48h 窗）staleness 6d: 真斷軌 → 必須輸出
//     overage = 6d-48h。
//   - tdcc_equity_dispersion（契約窗 8d,週快照）staleness 3d: 不得輸出。
func TestExportChannelHealthMetrics_StalenessOverageRespectsContract(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 4, 1, 30, 0, 0, time.UTC)
	wrapper := struct {
		Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
	}{
		Channels: map[string]*apigateway.ChannelHealthRecord{
			"us10y": {
				Status:     "ok",
				LastDataAt: now.Add(-27 * time.Hour).Format(time.RFC3339),
			},
			"twse_replay_sync": {
				Status:      "ok",
				LastFetchAt: now.Add(-6 * 24 * time.Hour).Format(time.RFC3339),
			},
			"tdcc_equity_dispersion": {
				Status:      "ok",
				LastFetchAt: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
			},
		},
	}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "channel_health.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	collector := monitoring.NewMetricsCollector()
	if err := exportChannelHealthMetrics(dir, collector, now); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	body := rec.Body.String()

	// twse_replay_sync: 6d staleness - 48h window = 96h overage。
	wantOverage := `atlas_channel_staleness_overage_seconds{channel="twse_replay_sync"} 345600`
	if !strings.Contains(body, wantOverage) {
		t.Fatalf("missing overage series %q\n--- full body ---\n%s", wantOverage, body)
	}
	for _, absent := range []string{
		`atlas_channel_staleness_overage_seconds{channel="us10y"}`,
		`atlas_channel_staleness_overage_seconds{channel="tdcc_equity_dispersion"}`,
	} {
		if strings.Contains(body, absent) {
			t.Fatalf("unexpected overage series %q (within contract window)\n--- full body ---\n%s", absent, body)
		}
	}
	// raw staleness gauge 仍輸出（dashboard 需要）。
	if !strings.Contains(body, `atlas_channel_data_staleness_seconds{channel="us10y"} 97200`) {
		t.Fatalf("raw staleness gauge for us10y missing\n--- full body ---\n%s", body)
	}
}

func TestExportChannelHealthMetrics_NoCollectorIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := exportChannelHealthMetrics(dir, nil, time.Now()); err != nil {
		t.Fatalf("expected nil collector to be no-op, got %v", err)
	}
}

func TestRegisterBackfillTasks_ChannelHealthMetricsRegistered(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerBackfillTasks(backfillDeps{
		taskMgr: mgr,
		cfg:     config.Config{WorkDir: t.TempDir()},
	})
	if _, ok := mgr.Get("channel_health_metrics_export"); !ok {
		t.Fatal("channel_health_metrics_export task was not registered")
	}
}
