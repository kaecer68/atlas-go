package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
)

func TestNewTWSEMarginBalanceProvider(t *testing.T) {
	p := NewTWSEMarginBalanceProvider("")
	if p.Name() != "twse_margin_balance" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
	if p.storageDir != "" {
		t.Fatalf("expected empty storageDir, got %s", p.storageDir)
	}
}

func TestTWSEMarginBalanceProvider_SaveMargin(t *testing.T) {
	dir := t.TempDir()
	p := NewTWSEMarginBalanceProvider(dir)

	if err := p.saveMargin("20260513", 3500.5, 120.5, 1.25, -0.75); err != nil {
		t.Fatalf("saveMargin failed: %v", err)
	}

	fpath := filepath.Join(dir, "20260513_margin.json")
	data, err := os.ReadFile(fpath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if result["date"] != "20260513" {
		t.Fatalf("unexpected date: %v", result["date"])
	}
	if result["margin_balance"] != 3500.5 {
		t.Fatalf("unexpected margin_balance: %v", result["margin_balance"])
	}
	if result["change_pct"] != 1.25 {
		t.Fatalf("unexpected change_pct: %v", result["change_pct"])
	}
	if result["short_balance"] != 120.5 {
		t.Fatalf("unexpected short_balance: %v", result["short_balance"])
	}
	if result["short_change_pct"] != -0.75 {
		t.Fatalf("unexpected short_change_pct: %v", result["short_change_pct"])
	}
}

func TestTWSEMarginBalanceProvider_SaveMargin_EmptyDir(t *testing.T) {
	p := NewTWSEMarginBalanceProvider("")

	if err := p.saveMargin("20260513", 3500.5, 120.5, 1.25, -0.75); err != nil {
		t.Fatalf("saveMargin with empty dir should not error: %v", err)
	}
}

func TestTWSEMarginBalanceProvider_FetchDateExpanded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("date") != "20260513" {
			t.Fatalf("unexpected date: %s", r.URL.Query().Get("date"))
		}
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260513",
  "tables": [
    {
      "title": "115年05月13日 信用交易統計",
      "fields": ["項目", "買進", "賣出", "現金(券)償還", "前日餘額", "今日餘額"],
      "data": [
        ["融資(交易單位)", "429,048", "495,782", "5,934", "9,149,157", "9,076,489"],
        ["融券(交易單位)", "23,361", "25,191", "2,062", "239,437", "239,205"],
        ["融資金額(仟元)", "33,268,274", "37,431,433", "569,362", "100,000,000", "120,000,000"]
      ]
    }
  ]
}`))
	}))
	defer server.Close()

	p := NewTWSEMarginBalanceProvider("")
	p.baseURL = server.URL
	p.client = server.Client()
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	margin, short, marginChange, shortChange, err := p.fetchDateExpanded(context.Background(), "20260513")
	if err != nil {
		t.Fatalf("fetchDateExpanded failed: %v", err)
	}

	// 融資金額: 120,000,000 仟元 / 1e5 = 1200.0
	if margin != 1200.0 {
		t.Fatalf("unexpected margin: %v (want 1200.0)", margin)
	}
	// 融券: 239,205 交易單位 / 1e5 = 2.39205
	if short != 2.39205 {
		t.Fatalf("unexpected short: %v (want 2.39205)", short)
	}
	// marginChange: (1200-1000)/1000*100 = 20%
	if marginChange < 19.9 || marginChange > 20.1 {
		t.Fatalf("unexpected marginChange: %v (want ~20)", marginChange)
	}
	// shortChange: (2.39205-2.39437)/2.39437*100 ≈ -0.097% (239437/1e5 = 2.39437)
	if shortChange <= -1.0 || shortChange >= 1.0 {
		t.Fatalf("unexpected shortChange: %v (want near 0)", shortChange)
	}
}

func TestTWSEMarginBalanceProvider_FetchSnapshotIncludesShortBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260513",
  "tables": [
    {
      "title": "115年05月13日 信用交易統計",
      "fields": ["項目", "買進", "賣出", "現金(券)償還", "前日餘額", "今日餘額"],
      "data": [
        ["融資(交易單位)", "429,048", "495,782", "5,934", "9,149,157", "9,076,489"],
        ["融券(交易單位)", "23,361", "25,191", "2,062", "239,437", "239,205"],
        ["融資金額(仟元)", "33,268,274", "37,431,433", "569,362", "100,000,000", "120,000,000"]
      ]
    }
  ]
}`))
	}))
	defer server.Close()

	p := NewTWSEMarginBalanceProvider("")
	p.baseURL = server.URL
	p.client = server.Client()
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}
	if snap.RetailMarginBalance.Symbol != "TAIWAN_MARGIN_BALANCE" {
		t.Fatalf("unexpected margin symbol: %s", snap.RetailMarginBalance.Symbol)
	}
	if snap.RetailShortBalance.Symbol != "TAIWAN_SHORT_BALANCE" {
		t.Fatalf("unexpected short symbol: %s", snap.RetailShortBalance.Symbol)
	}
	if snap.RetailShortBalance.Value <= 0 {
		t.Fatalf("expected positive short_balance, got %v", snap.RetailShortBalance.Value)
	}
}

// ─── #1924: margin maintenance ratio comes from FinMind, not TWSE ───

// twseMarginBalanceStub serves the MI_MARGN balance table (selectType=MS).
// It fails the test if the provider ever asks for selectType=ALL again — that
// probe read the per-stock 融資融券彙總 table and always failed on the name
// column (#1924).
func twseMarginBalanceStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("selectType"); got == "ALL" {
			t.Errorf("provider must no longer query selectType=ALL (issue #1924), got %q", got)
		}
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260922",
  "tables": [
    {
      "title": "115年09月22日 信用交易統計",
      "fields": ["項目", "買進", "賣出", "現金(券)償還", "前日餘額", "今日餘額"],
      "data": [
        ["融資(交易單位)", "429,048", "495,782", "5,934", "9,149,157", "9,076,489"],
        ["融券(交易單位)", "23,361", "25,191", "2,062", "239,437", "239,205"],
        ["融資金額(仟元)", "33,268,274", "37,431,433", "569,362", "100,000,000", "120,000,000"]
      ]
    }
  ]
}`))
	}))
}

// marginTestProvider builds a provider pointed at the stub in fast-mode.
func marginTestProvider(t *testing.T, twse *httptest.Server) *TWSEMarginBalanceProvider {
	t.Helper()
	p := NewTWSEMarginBalanceProvider("")
	p.SetBaseURL(twse.URL)
	p.SetHTTPClient(twse.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	return p
}

// marginFinMindStub builds a FinMind client pointed at body.
func marginFinMindStub(t *testing.T, calls *atomic.Int64, body string) *FinMindClient {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		if got := r.URL.Query().Get("dataset"); got != "TaiwanTotalExchangeMarginMaintenance" {
			t.Errorf("dataset = %q, want TaiwanTotalExchangeMarginMaintenance", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)

	fc := NewFinMindClientWithStateDir("test-key", t.TempDir())
	fc.SetBaseURL(ts.URL)
	fc.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	return fc
}

// captureMarketdataLogs swaps the global logger for a debug-level buffer and
// restores it after the test.
func captureMarketdataLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return &buf
}

// TestTWSEMarginBalanceProvider_FillsMaintenanceRatioFromFinMind covers the
// #1924 fix: with a FinMind client wired, the TWSE margin snapshot carries
// TSE_MARGIN_MAINT (value = latest whole-market row) so the period_detector
// 融資維持率 rule can fire; the balance fields keep their TWSE source, and a
// second fetch on the same trading day does not spend another API call.
func TestTWSEMarginBalanceProvider_FillsMaintenanceRatioFromFinMind(t *testing.T) {
	twse := twseMarginBalanceStub(t)
	defer twse.Close()

	var calls atomic.Int64
	fc := marginFinMindStub(t, &calls, `{"msg":"success","status":200,"data":[
	  {"date":"2026-09-21","TotalExchangeMarginMaintenance":191.448},
	  {"date":"2026-09-22","TotalExchangeMarginMaintenance":193.131}
	]}`)

	p := marginTestProvider(t, twse)
	p.SetFinMindClient(fc)

	// 2026-09-23 is a Wednesday: the provider asks TWSE for that date first.
	date := time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC)
	snap, err := p.FetchSnapshotForDate(context.Background(), date)
	if err != nil {
		t.Fatalf("FetchSnapshotForDate failed: %v", err)
	}
	if snap.MarginMaintenanceRatio.Symbol != "TSE_MARGIN_MAINT" {
		t.Fatalf("MarginMaintenanceRatio.Symbol = %q, want TSE_MARGIN_MAINT (snapshot %+v)",
			snap.MarginMaintenanceRatio.Symbol, snap.MarginMaintenanceRatio)
	}
	if snap.MarginMaintenanceRatio.Value != 193.131 {
		t.Errorf("MarginMaintenanceRatio.Value = %v, want 193.131 (latest FinMind row)",
			snap.MarginMaintenanceRatio.Value)
	}
	if snap.MarginMaintenanceRatio.Timestamp == 0 {
		t.Errorf("MarginMaintenanceRatio.Timestamp must be set")
	}
	// The TWSE balance path is untouched (120,000,000 仟元 / 1e5 = 1200).
	if snap.RetailMarginBalance.Value != 1200.0 {
		t.Errorf("RetailMarginBalance.Value = %v, want 1200.0 (TWSE path unchanged)",
			snap.RetailMarginBalance.Value)
	}
	if snap.RetailShortBalance.Value <= 0 {
		t.Errorf("RetailShortBalance.Value = %v, want > 0", snap.RetailShortBalance.Value)
	}

	if _, err := p.FetchSnapshotForDate(context.Background(), date); err != nil {
		t.Fatalf("second FetchSnapshotForDate failed: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("FinMind calls = %d, want 1 (1 call/day budget)", n)
	}
}

// TestTWSEMarginBalanceProvider_MaintenanceRatioNoDataIsSilent covers the
// noise requirement of #1924: FinMind not publishing a row yet (weekend,
// holiday, or before the evening release) must NOT raise the fetch to an
// error, must NOT log a WARN, and must leave the other fields intact.
func TestTWSEMarginBalanceProvider_MaintenanceRatioNoDataIsSilent(t *testing.T) {
	twse := twseMarginBalanceStub(t)
	defer twse.Close()

	var calls atomic.Int64
	fc := marginFinMindStub(t, &calls, `{"msg":"success","status":200,"data":[]}`)

	buf := captureMarketdataLogs(t)

	p := marginTestProvider(t, twse)
	p.SetFinMindClient(fc)

	snap, err := p.FetchSnapshotForDate(context.Background(), time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ratio no-data must not fail the snapshot: %v", err)
	}
	if snap.MarginMaintenanceRatio.Symbol != "" {
		t.Errorf("MarginMaintenanceRatio should stay empty, got %+v", snap.MarginMaintenanceRatio)
	}
	if snap.RetailMarginBalance.Value != 1200.0 || snap.RetailShortBalance.Value <= 0 {
		t.Errorf("TWSE fields must survive a missing ratio, got margin=%v short=%v",
			snap.RetailMarginBalance.Value, snap.RetailShortBalance.Value)
	}
	if calls.Load() != 1 {
		t.Errorf("FinMind calls = %d, want 1 (retry allowed while unpublished)", calls.Load())
	}

	out := buf.String()
	if strings.Contains(out, "level=WARN") {
		t.Errorf("no-data ratio path must not log WARN; log was:\n%s", out)
	}
	if strings.Contains(out, "maintenance_ratio_fetch_failed") {
		t.Errorf("the removed TWSE probe event reappeared:\n%s", out)
	}
}

// TestTWSEMarginBalanceProvider_MaintenanceRatioQuotaQuiet covers the other
// expected-no-noise case: FinMind's free-tier "please update your level"
// answer (HTTP 200 + empty data) is a budget condition (ErrQuotaExhausted),
// not an outage, so it must not log a WARN either.
func TestTWSEMarginBalanceProvider_MaintenanceRatioQuotaQuiet(t *testing.T) {
	twse := twseMarginBalanceStub(t)
	defer twse.Close()

	fc := marginFinMindStub(t, nil,
		`{"msg":"Your level is free. Please update your user level.","status":200,"data":[]}`)

	buf := captureMarketdataLogs(t)

	p := marginTestProvider(t, twse)
	p.SetFinMindClient(fc)

	if _, err := p.FetchSnapshotForDate(context.Background(), time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("quota condition must not fail the snapshot: %v", err)
	}
	if out := buf.String(); strings.Contains(out, "level=WARN") {
		t.Errorf("quota condition must not log WARN; log was:\n%s", out)
	}
}

// TestTWSEMarginBalanceProvider_MaintenanceRatioUpstreamFailureWarns keeps the
// other half of the contract: a genuine upstream failure is still surfaced.
func TestTWSEMarginBalanceProvider_MaintenanceRatioUpstreamFailureWarns(t *testing.T) {
	twse := twseMarginBalanceStub(t)
	defer twse.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"msg":"bad request"}`))
	}))
	defer ts.Close()

	fc := NewFinMindClientWithStateDir("test-key", t.TempDir())
	fc.SetBaseURL(ts.URL)
	fc.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))

	buf := captureMarketdataLogs(t)

	p := marginTestProvider(t, twse)
	p.SetFinMindClient(fc)

	snap, err := p.FetchSnapshotForDate(context.Background(), time.Date(2026, 9, 23, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("upstream ratio failure must not fail the snapshot: %v", err)
	}
	if snap.RetailMarginBalance.Value != 1200.0 {
		t.Errorf("TWSE fields must survive a ratio outage, got %v", snap.RetailMarginBalance.Value)
	}
	if !strings.Contains(buf.String(), "maintenance_ratio_source_failed") {
		t.Errorf("genuine upstream failure should log WARN maintenance_ratio_source_failed; log was:\n%s", buf.String())
	}
}
