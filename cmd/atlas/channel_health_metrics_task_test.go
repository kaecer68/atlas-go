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
			"twse_capital_flow": {
				Status:     "ok",
				LastDataAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
				LatencyMs:  250,
			},
			"finmind": {
				Status:      "error",
				LastFetchAt: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339),
				LatencyMs:   5000,
			},
			"us_yahoo": {
				Status:     "error",
				LastDataAt: now.Add(-3 * 24 * time.Hour).Format(time.RFC3339),
				LatencyMs:  5000,
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
	if err := exportChannelHealthMetrics(dir, collector, now, nil); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	body := rec.Body.String()

	mustContain := []string{
		`atlas_channel_health_status{channel="twse_capital_flow"} 0`,
		`atlas_channel_fetch_latency_seconds{channel="twse_capital_flow"} 0.25`,
		`atlas_channel_data_staleness_seconds{channel="twse_capital_flow"} 1800`,
		`atlas_channel_health_status{channel="finmind"} 2`,
		`atlas_channel_fetch_latency_seconds{channel="finmind"} 5`,
		`atlas_channel_data_staleness_seconds{channel="finmind"} 172800`,
		`atlas_channel_data_staleness_seconds{channel="us_yahoo"} 259200`,
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
			// 2026-09-07: us10y 改為 derived（us_yahoo 欄位,見
			// TestExportChannelHealthMetrics_DerivedIndicatorChannelsSkipped）。
			// 「預設 48h 窗」範例改用 fugle（契約 FreshnessWindow=0 → 繼承
			// StaleDataThreshold 48h）。2026-09-08 R3: 通道名一律用
			// channelIDs() 內的註冊 ID（未註冊 ID 會被 janitor 標 derived 而跳過）。
			"fugle": {
				Status:     "ok",
				LastDataAt: now.Add(-27 * time.Hour).Format(time.RFC3339),
			},
			"twse_replay": {
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
	if err := exportChannelHealthMetrics(dir, collector, now, nil); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	body := rec.Body.String()

	// twse_replay: 6d staleness - 72h 契約窗 = 72h overage。
	wantOverage := `atlas_channel_staleness_overage_seconds{channel="twse_replay"} 259200`
	if !strings.Contains(body, wantOverage) {
		t.Fatalf("missing overage series %q\n--- full body ---\n%s", wantOverage, body)
	}
	// 2026-09-05 修復: overage 永遠輸出（窗口內 = 0）——若條件消失就跳過,
	// MetricsCollector 的 last-write-wins gauge 會凍結舊樣本,Prometheus
	// 永遠看到 >0,alert 永遠 firing（實證: twse_replay_sync 恢復後仍每小時
	// 重複通知）。
	for _, want := range []string{
		`atlas_channel_staleness_overage_seconds{channel="fugle"} 0`,
		`atlas_channel_staleness_overage_seconds{channel="tdcc_equity_dispersion"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing zeroed overage series %q\n--- full body ---\n%s", want, body)
		}
	}
	// raw staleness gauge 仍輸出（dashboard 需要）。
	if !strings.Contains(body, `atlas_channel_data_staleness_seconds{channel="fugle"} 97200`) {
		t.Fatalf("raw staleness gauge for fugle missing\n--- full body ---\n%s", body)
	}
}

// TestExportChannelHealthMetrics_ExpiredOkExportsStaleNotOk —
// 2026-09-24 channel-status-truth: the status gauge used to export the raw
// record status, so a channel whose last fetch was 17 days old still exported
// atlas_channel_health_status 0 (ok) while every other surface called it stale.
// The gauge now carries the same contract-aware verdict (stale → warning value
// 1; the alert rules only match == 2, so nothing pages differently).
func TestExportChannelHealthMetrics_ExpiredOkExportsStaleNotOk(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)
	wrapper := struct {
		Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
	}{
		Channels: map[string]*apigateway.ChannelHealthRecord{
			// Registered channel, record says ok, data 17 days old.
			"twse_oddlot": {
				Status:      "ok",
				LastFetchAt: now.Add(-17 * 24 * time.Hour).Format(time.RFC3339),
			},
			// Registered channel, freshly fetched: stays ok.
			"twse_capital_flow": {
				Status:      "ok",
				LastFetchAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
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
	if err := exportChannelHealthMetrics(dir, collector, now, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	body := rec.Body.String()

	for _, want := range []string{
		`atlas_channel_health_status{channel="twse_oddlot"} 1`,
		`atlas_channel_health_status{channel="twse_capital_flow"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", want, body)
		}
	}
	if strings.Contains(body, `atlas_channel_health_status{channel="twse_oddlot"} 0`) {
		t.Fatal("expired ok channel must not be exported as ok")
	}
}

func TestExportChannelHealthMetrics_NoCollectorIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := exportChannelHealthMetrics(dir, nil, time.Now(), nil); err != nil {
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

// TestExportChannelHealthMetrics_DerivedIndicatorChannelsSkipped —
// 2026-09-07: vix/us10y 是 us_yahoo 批次通道的指標欄位,不是獨立通道。
// channel_health.json 的孤兒紀錄（crossmarket callback 只在轉換時寫一次）
// 讓 staleness 凍結時間戳無限增長 → ChannelDataStale 對健康管線反覆誤報
// （#1843 之後仍復發）。staleness/latency/overage 序列必須跳過;
// status gauge 保留（dashboard 顯示用）。
// 2026-09-08 R3: 跳過機制從 hardcode 白名單改為 provenance — load 時
// janitor 對未註冊 ID 自動標 derived（見 channel_health.go load()）。
func TestExportChannelHealthMetrics_DerivedIndicatorChannelsSkipped(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 14, 0, 0, 0, time.UTC)
	wrapper := struct {
		Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
	}{
		Channels: map[string]*apigateway.ChannelHealthRecord{
			"vix": {
				Status:      "ok",
				LastFetchAt: now.Add(-60 * time.Hour).Format(time.RFC3339),
			},
			"us10y": {
				Status:      "ok",
				LastFetchAt: now.Add(-60 * time.Hour).Format(time.RFC3339),
			},
			"us_yahoo": {
				Status:     "ok",
				LastDataAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
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
	if err := exportChannelHealthMetrics(dir, collector, now, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, req)
	body := rec.Body.String()

	// status gauge 仍輸出
	mustContain := []string{
		`atlas_channel_health_status{channel="vix"} 0`,
		`atlas_channel_health_status{channel="us10y"} 0`,
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", want, body)
		}
	}
	// staleness/latency/overage 序列不得輸出（誤報源頭）
	mustAbsent := []string{
		`atlas_channel_data_staleness_seconds{channel="vix"}`,
		`atlas_channel_staleness_overage_seconds{channel="vix"}`,
		`atlas_channel_data_staleness_seconds{channel="us10y"}`,
		`atlas_channel_staleness_overage_seconds{channel="us10y"}`,
	}
	for _, absent := range mustAbsent {
		if strings.Contains(body, absent) {
			t.Fatalf("unexpected /metrics line %q for derived indicator channel\n--- full body ---\n%s", absent, body)
		}
	}
	// 真通道 us_yahoo 仍正常輸出
	if !strings.Contains(body, `atlas_channel_data_staleness_seconds{channel="us_yahoo"} 600`) {
		t.Fatalf("us_yahoo staleness missing\n--- full body ---\n%s", body)
	}
}

// TestHealthStatusValue pins the atlas_channel_health_status gauge vocabulary.
//
// E29-3 (2026-09-27): "degraded" used to fall through to 4 (other/unmapped) — a
// defined non-ok verdict exported as "unmapped" — while "stale" already mapped to
// warn. warn/stale/degraded stay below the page threshold on purpose: the alert
// rules on this gauge match == 2 only, and a channel that is degraded past its
// freshness window no longer stops here (DeriveChannelStatus escalates it to
// error, i.e. 2).
func TestHealthStatusValue(t *testing.T) {
	cases := map[string]float64{
		"ok":             0,
		"warn":           1,
		"stale":          1,
		"degraded":       1,
		"error":          2,
		"inactive":       3,
		"expected_delay": 4,
		"":               4,
	}
	for status, want := range cases {
		if got := healthStatusValue(status); got != want {
			t.Errorf("healthStatusValue(%q) = %v, want %v", status, got, want)
		}
	}
}

// TestExportChannelHealthMetrics_TwseOddlotRetirementClosesTheAlertLoop —
// issue #2134. The production alert ChannelHealthStatusError{channel="twse_oddlot"}
// was firing because the leftover record was status="degraded" and
// DeriveChannelStatus escalates a degraded record to error once its data is
// older than the 48h contract window (E29-3 rule 2b): the gauge became 2, which
// is the only value the alert rules match. This test pins both ends through the
// real export path, so "remove the registration and hope" cannot silently
// regress into a permanent page again.
func TestExportChannelHealthMetrics_TwseOddlotRetirementClosesTheAlertLoop(t *testing.T) {
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)

	exportedStatus := func(t *testing.T, rec *apigateway.ChannelHealthRecord) float64 {
		t.Helper()
		dir := t.TempDir()
		stateDir := filepath.Join(dir, "data", "state")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatal(err)
		}
		wrapper := struct {
			Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
		}{
			Channels: map[string]*apigateway.ChannelHealthRecord{"twse_oddlot": rec},
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
		rec2 := httptest.NewRecorder()
		monitoring.PrometheusHandler(collector).ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/metrics", nil))

		const prefix = `atlas_channel_health_status{channel="twse_oddlot"} `
		for _, line := range strings.Split(rec2.Body.String(), "\n") {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			// Prometheus text format renders gauges as floats ("2.000000").
			value, parseErr := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, prefix)), 64)
			if parseErr != nil {
				t.Fatalf("unparseable gauge value in %q: %v", line, parseErr)
			}
			return value
		}
		t.Fatalf("no %s line in /metrics:\n%s", prefix, rec2.Body.String())
		return -1
	}

	t.Run("pre-fix production record exports 2 (the firing condition)", func(t *testing.T) {
		// Measured in production 2026-09-29.
		got := exportedStatus(t, &apigateway.ChannelHealthRecord{
			Status:        apigateway.StatusDegraded,
			LastFetchAt:   "2026-09-27T12:59:28Z",
			LastSuccessAt: "2026-09-07T00:18:11Z",
			LastError:     "twse_oddlot: 上游回傳空/停用資料（stale payload）",
		})
		if got != 2 {
			t.Fatalf("pre-fix record exported status %v, want 2 (that is the state the retirement removes)", got)
		}
	})

	t.Run("retired record exports 3 (no alert rule matches)", func(t *testing.T) {
		got := exportedStatus(t, &apigateway.ChannelHealthRecord{
			Status:        apigateway.StatusInactive,
			LastFetchAt:   "2026-09-29T06:40:00Z",
			LastSuccessAt: "2026-09-07T00:18:11Z",
			LastError:     "BFI84U 上游已由 TWSE 移除（2026-08）⇒ 本 channel 永久退役，不再抓取；零售商零股失衡輸入改由 twse_capital_flow 代理",
		})
		if got != 3 {
			t.Fatalf("retired record exported status %v, want 3 (inactive; ChannelHealthStatusError matches == 2 only)", got)
		}
		if healthStatusValue(apigateway.StatusInactive) != 3 {
			t.Fatal("healthStatusValue(inactive) must stay 3: the retirement depends on it")
		}
	})
}

// TestExportChannelHealthMetrics_GovernanceGauge — issue #2138. Two properties
// are pinned here, and both are cardinality/one-shot discipline rather than
// arithmetic:
//  1. the governance gauge exists ONLY for channels whose registry entry declares
//     a permanent upstream removal (bounded series set — a channel with no known
//     issue must never produce one);
//  2. the reminder fires ONCE per occurrence, not once per 5-minute tick.
func TestExportChannelHealthMetrics_GovernanceGauge(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 2026-09-29 07:00Z: the instant the production alert was firing on.
	now := time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)
	wrapper := struct {
		Channels map[string]*apigateway.ChannelHealthRecord `json:"channels"`
	}{
		Channels: map[string]*apigateway.ChannelHealthRecord{
			// The pre-retirement production shape: degraded, last real data 22 days
			// old, permanent upstream removal + replacement on record.
			"twse_oddlot": {
				Status:        apigateway.StatusDegraded,
				LastFetchAt:   "2026-09-27T12:59:28Z",
				LastSuccessAt: "2026-09-07T00:18:11Z",
				LastError:     "twse_oddlot: 上游回傳空/停用資料（stale payload）",
			},
			// A healthy channel WITH a known issue that is not an availability case
			// (dead alias): it must not produce a governance series either.
			"taifex-daily": {
				Status:        apigateway.StatusError,
				LastFetchAt:   "2026-06-04T01:11:48Z",
				LastSuccessAt: "2026-06-04T01:11:48Z",
			},
			// No known issue at all: the bounded-set assertion.
			"finmind": {
				Status:      apigateway.StatusError,
				LastFetchAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
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
	notifier := monitoring.NewGovernanceNotifier()
	for i := range 3 { // three ticks in a row
		if err := exportChannelHealthMetrics(dir, collector, now.Add(time.Duration(i)*5*time.Minute), notifier); err != nil {
			t.Fatal(err)
		}
	}

	rec := httptest.NewRecorder()
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `atlas_channel_governance_overdue{channel="twse_oddlot"} 1`) {
		t.Fatalf("missing governance gauge for the pre-retirement twse_oddlot shape\n--- body ---\n%s", body)
	}
	for _, absent := range []string{
		`atlas_channel_governance_overdue{channel="finmind"}`,      // no known issue ⇒ no series
		`atlas_channel_governance_overdue{channel="taifex-daily"}`, // declared NOT an availability case
	} {
		if strings.Contains(body, absent) {
			t.Errorf("unbounded series: %s must not exist (the gauge set is bounded by the registry's availability cases)", absent)
		}
	}

	// One-shot: the first tick reminded, the following two must not.
	decision := monitoring.EvaluateChannelGovernance(
		monitoring.LookupKnownIssue("twse_oddlot"),
		&apigateway.ChannelHealthRecord{Status: apigateway.StatusDegraded, LastSuccessAt: "2026-09-07T00:18:11Z"},
		apigateway.ChannelContracts().Contract("twse_oddlot"), now)
	fresh := monitoring.NewGovernanceNotifier()
	if line, emit := governanceReminderLine(fresh, "twse_oddlot", decision, now); !emit || line == "" {
		t.Fatalf("the first observation of an overdue channel must remind (emit=%t)", emit)
	}
	for i := 1; i <= 3; i++ {
		if _, emit := governanceReminderLine(fresh, "twse_oddlot", decision, now.Add(time.Duration(i)*5*time.Minute)); emit {
			t.Fatalf("tick %d reminded again: a repeating reminder is a second paging channel", i)
		}
	}
	// A nil notifier (gauge-only caller) must not panic and must not remind.
	if _, emit := governanceReminderLine(nil, "twse_oddlot", decision, now); emit {
		t.Error("a nil notifier must never emit a reminder")
	}
}
