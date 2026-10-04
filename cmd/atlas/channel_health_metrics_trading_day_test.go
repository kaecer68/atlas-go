package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// TestExportChannelHealthMetrics_TradingDayChannelsNoWeekendOverage — F58
// (2026-10-04 週末假陽性)：匯出層的驗收。
//
// 生產實證形狀：twse_sbl / government_broker 最後成功在台北週五 15:2x
// （10-02T07:20Z / 10-02T07:18Z），週日晚上 raw age 已 55h8m > 預設窗口 48h。
// 修前 ⇒ atlas_channel_staleness_overage_seconds > 0 ⇒ ChannelDataStale firing
// 每個週末。修後（契約宣告 PublishCalendarTWTradingDay）⇒ overage = 0，
// 但 raw staleness gauge 照舊輸出（可觀測性不得被藏起來）。
//
// 對照組：同一份 channel_health.json 裡、**沒有**宣告交易日曆的通道
// （fugle，54h > 48h 窗口）必須照舊 > 0 ⇒ 這不是全體靜默。
func TestExportChannelHealthMetrics_TradingDayChannelsNoWeekendOverage(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 4, 14, 28, 0, 0, time.UTC) // 台北週日 22:28
	lastSuccess := time.Date(2026, 10, 2, 7, 20, 0, 0, time.UTC).Format(time.RFC3339)
	wrapper := struct {
		Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
	}{
		Channels: map[string]*apigateway.ChannelHealthRecord{
			"twse_sbl":          {Status: "ok", LastFetchAt: lastSuccess},
			"government_broker": {Status: "ok", LastFetchAt: lastSuccess},
			// 對照組：日曆無關的通道（契約 48h），相同 age ⇒ 仍必須過窗。
			"fugle": {Status: "ok", LastDataAt: lastSuccess},
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
	if err := exportChannelHealthMetrics(dir, collector, now, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	// raw staleness 照舊（55h8m = 198480s）——不是把問題藏起來，而是判定改曆法。
	const rawAge = 198480
	for _, want := range []string{
		`atlas_channel_staleness_overage_seconds{channel="twse_sbl"} 0`,
		`atlas_channel_staleness_overage_seconds{channel="government_broker"} 0`,
		`atlas_channel_data_staleness_seconds{channel="twse_sbl"} ` + strconv.Itoa(rawAge),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in /metrics body", want)
		}
	}
	// 對照組必須仍過窗（198480 - 172800 = 25680）。
	if want := `atlas_channel_staleness_overage_seconds{channel="fugle"} 25680`; !strings.Contains(body, want) {
		t.Errorf("missing %q ⇒ 修法可能把日曆無關的通道一起靜默了", want)
	}
}
