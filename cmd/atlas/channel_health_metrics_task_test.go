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
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", want, body)
		}
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
