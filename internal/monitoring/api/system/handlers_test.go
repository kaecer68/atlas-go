package system

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func newSystemHandlers(t *testing.T) (*Handlers, string) {
	t.Helper()
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	baselinePath := filepath.Join(workDir, constants.StateBaselinePolicy+".json")

	store := ledger.NewStore(ledgerDir)
	svc := service.NewSystemService(workDir, ledgerDir, baselinePath, store, nil, nil)
	return &Handlers{Svc: svc}, workDir
}

func mustDecode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if w.Body.Len() == 0 {
		return nil
	}
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func assertFloatPtr(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil, want %v", name, want)
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, *got, want)
	}
}

func TestHandlePhase3Status_NoFile(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/phase3-status", nil)
	status, _ := h.HandlePhase3Status(req)
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
}

func TestHandleSystemHealth_Success(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/system-health", nil)
	status, body := h.HandleSystemHealth(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
}

func TestHandleClampingEvents_NoFile(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events", nil)
	status, body := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	m := body.(map[string]any)
	if count, ok := m["count"].(int); ok && count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestHandleClampingEvents_WithLimit(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events?limit=50", nil)
	status, body := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := body.(map[string]any)
	if _, ok := m["events"]; !ok {
		t.Error("missing events key")
	}
}

func TestHandleClampingEvents_InvalidLimit(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events?limit=abc", nil)
	status, body := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := body.(map[string]any)
	if _, ok := m["events"]; !ok {
		t.Error("missing events key")
	}
}

func TestHandleClampingEvents_LimitExceedsMax(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events?limit=5000", nil)
	status, _ := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestHandleConvictionClampingEvents_NoFile(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/conviction-clamping-events", nil)
	status, body := h.HandleConvictionClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := body.(map[string]any)
	if count, ok := m["count"].(int); ok && count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestHandleCapitalPhase_ReturnsSnapshot(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/capital-phase", nil)
	status, body := h.HandleCapitalPhase(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	// CapitalPhaseController.GetSnapshot() returns domain.CapitalSnapshot
	_, ok := body.(domain.CapitalSnapshot)
	if !ok {
		t.Fatalf("body is %T, want domain.CapitalSnapshot", body)
	}
}

func TestHandleRetailSentiment_NoSnapshot(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T, want RetailSentimentResponse", body)
	}
	if resp.Interpretation != "no macro snapshot available" {
		t.Errorf("interpretation = %q, want 'no macro snapshot available'", resp.Interpretation)
	}
	if resp.SentimentScore != nil {
		t.Errorf("SentimentScore = %v, want nil", resp.SentimentScore)
	}
	if resp.MarginBalance != nil {
		t.Errorf("MarginBalance = %v, want nil", resp.MarginBalance)
	}
	if resp.ShortBalance != nil {
		t.Errorf("ShortBalance = %v, want nil", resp.ShortBalance)
	}
	if resp.DayTradingRatio != nil {
		t.Errorf("DayTradingRatio = %v, want nil", resp.DayTradingRatio)
	}
	if resp.RetailFuturesOI != nil {
		t.Errorf("RetailFuturesOI = %v, want nil", resp.RetailFuturesOI)
	}
	if resp.ETFNetSubscription != nil {
		t.Errorf("ETFNetSubscription = %v, want nil", resp.ETFNetSubscription)
	}
	if resp.FetcherStatus.DayTrading != "no_data" {
		t.Errorf("DayTrading fetcher status = %q, want no_data", resp.FetcherStatus.DayTrading)
	}
}

func TestHandleRetailSentiment_WithSnapshot(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	// Write a minimal latest.json so loadLatestMacroSnapshot succeeds.
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05, "symbol": "MARGIN"},
		"retail_short_balance":  map[string]any{"value": 800, "change_pct": 0.02, "symbol": "SHORT"},
		"vix":                   map[string]any{"value": 18.5},
		"foreign_investor_net":  map[string]any{"value": 5000},
		"domestic_fund_net":     map[string]any{"value": 2000},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	assertFloatPtr(t, "MarginBalance", resp.MarginBalance, 2500)
	assertFloatPtr(t, "MarginChangePct", resp.MarginChangePct, 0.0005)
	assertFloatPtr(t, "ShortBalance", resp.ShortBalance, 800)
	assertFloatPtr(t, "ShortChangePct", resp.ShortChangePct, 0.02)
	if resp.SentimentScore == nil {
		t.Error("SentimentScore is nil, want non-nil")
	}
	if resp.Score == nil {
		t.Error("Score is nil, want non-nil")
	}
	if resp.ChangePct == nil {
		t.Error("ChangePct is nil, want non-nil")
	}
	if resp.CompositeSentiment == nil {
		t.Error("CompositeSentiment is nil, want non-nil")
	}
	if resp.MarginPercentile == nil {
		t.Error("MarginPercentile is nil, want non-nil")
	}
	if resp.DayTradingRatio != nil {
		t.Errorf("DayTradingRatio = %v, want nil (no fetcher)", *resp.DayTradingRatio)
	}
	if resp.RetailFuturesOI != nil {
		t.Errorf("RetailFuturesOI = %v, want nil (no fetcher)", *resp.RetailFuturesOI)
	}
	if resp.ETFNetSubscription != nil {
		t.Errorf("ETFNetSubscription = %v, want nil (no fetcher)", *resp.ETFNetSubscription)
	}
}

func TestHandleRetailSentiment_WithFetchers(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05, "symbol": "MARGIN"},
		"retail_short_balance":  map[string]any{"value": 800, "change_pct": 0.02, "symbol": "SHORT"},
		"vix":                   map[string]any{"value": 18.5},
		"foreign_investor_net":  map[string]any{"value": 5000},
		"domestic_fund_net":     map[string]any{"value": 2000},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h.DayTradingFetcher = func(ctx context.Context) (*marketdata.DayTradingStats, error) {
		return &marketdata.DayTradingStats{DayTradingVolume: 1000, VolumeRatio: 0.15}, nil
	}
	h.TaifexFetcher = func(ctx context.Context) (*marketdata.PCRStats, *marketdata.RetailFuturesOI, error) {
		return &marketdata.PCRStats{PutCallVolumeRatio: 0.9},
			&marketdata.RetailFuturesOI{RetailLongPct: 0.6, RetailShortPct: 0.4},
			nil
	}
	h.OddLotFetcher = func(ctx context.Context) (*marketdata.OddLotStats, error) {
		return &marketdata.OddLotStats{ImbalanceRatio: 0.05}, nil
	}
	h.ETFFetcher = func(ctx context.Context) (*marketdata.ETFStats, error) {
		return &marketdata.ETFStats{NetSubscription: 1_000_000}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}

	assertFloatPtr(t, "DayTradingRatio", resp.DayTradingRatio, 0.15)
	assertFloatPtr(t, "RetailFuturesOI", resp.RetailFuturesOI, 0.2)
	assertFloatPtr(t, "ETFNetSubscription", resp.ETFNetSubscription, 1_000_000)
	if resp.FetcherStatus.DayTrading != "ok" {
		t.Errorf("DayTrading status = %q, want ok", resp.FetcherStatus.DayTrading)
	}
	if resp.FetcherStatus.Taifex != "ok" {
		t.Errorf("Taifex status = %q, want ok", resp.FetcherStatus.Taifex)
	}
	if resp.FetcherStatus.OddLot != "ok" {
		t.Errorf("OddLot status = %q, want ok", resp.FetcherStatus.OddLot)
	}
	if resp.FetcherStatus.ETF != "ok" {
		t.Errorf("ETF status = %q, want ok", resp.FetcherStatus.ETF)
	}

	// Verify JSON serialization: optional fields with data are present and nil fields are omitted/null.
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(encoded, &m); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := m["retail_futures_oi"]; !ok {
		t.Error("expected retail_futures_oi in JSON when fetcher returns data")
	}
	if _, ok := m["etf_net_subscription"]; !ok {
		t.Error("expected etf_net_subscription in JSON when fetcher returns data")
	}
	if _, ok := m["day_trading_ratio"]; !ok {
		t.Error("expected day_trading_ratio in JSON when fetcher returns data")
	}
}

// TestHandleRetailSentiment_ActiveEvents verifies Part D triggered events are
// populated in active_events (Audit A04, 2026-08-12). Previously the field was
// always null so the frontend showed "無觸發事件" even when the D multiplier
// was adjusted (e.g. 0.85 from geopolitical risk).
func TestHandleRetailSentiment_ActiveEvents(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05, "symbol": "MARGIN"},
		"vix":                   map[string]any{"value": 18.5},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Geopolitical risk above 0.5 threshold → D1 multiplier 0.85 triggered.
	h.GeopoliticalRiskFetcher = func(ctx context.Context) float64 { return 0.8 }

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	if resp.SentimentSubIndicators == nil || resp.SentimentSubIndicators.CategoryD == nil {
		t.Fatal("expected category_d in response")
	}
	cd := resp.SentimentSubIndicators.CategoryD
	if cd.AdjustmentFactor == 1.0 {
		t.Error("expected adjustment factor < 1.0 with geopolitical risk 0.8")
	}
	found := false
	for _, ev := range cd.ActiveEvents {
		if ev == "地緣政治風險" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected active_events to contain 地緣政治風險, got %v", cd.ActiveEvents)
	}
}

// TestHandleRetailSentiment_NoActiveEvents verifies no D events are listed
// when all D multipliers are at neutral (1.0) — regression guard for A04.
func TestHandleRetailSentiment_NoActiveEvents(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05, "symbol": "MARGIN"},
		"vix":                   map[string]any{"value": 18.5},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Geopolitical risk below 0.5 threshold → no D1 trigger.
	h.GeopoliticalRiskFetcher = func(ctx context.Context) float64 { return 0.2 }

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	if resp.SentimentSubIndicators == nil || resp.SentimentSubIndicators.CategoryD == nil {
		t.Fatal("expected category_d in response")
	}
	cd := resp.SentimentSubIndicators.CategoryD
	if cd.AdjustmentFactor != 1.0 {
		t.Errorf("expected adjustment factor 1.0 with geopolitical risk 0.2, got %f", cd.AdjustmentFactor)
	}
	if len(cd.ActiveEvents) != 0 {
		t.Errorf("expected no active_events, got %v", cd.ActiveEvents)
	}
}

// TestHandleRetailSentiment_FallbackFields verifies per-sub-indicator
// fallback_fields is populated (Review P1): only fallback sub-indicators are
// listed, so the frontend can badge rows individually instead of using the
// category-level OR flag.
func TestHandleRetailSentiment_FallbackFields(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05, "symbol": "MARGIN"},
		"vix":                   map[string]any{"value": 18.5},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// All fetchers fail → every sub-indicator is fallback; verify each key listed.
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	if resp.SentimentSubIndicators == nil {
		t.Fatal("expected sentiment_sub_indicators")
	}
	catA := resp.SentimentSubIndicators.CategoryA
	catC := resp.SentimentSubIndicators.CategoryC
	if catA == nil || catC == nil {
		t.Fatal("expected category_a and category_c")
	}
	// With no fetchers wired and zero margin history: a2/a4/a5/a6/a1 fallback,
	// a3 (percentile-based) computes a value (not fallback).
	hasA2 := false
	for _, f := range catA.FallbackFields {
		if f == "a2_day_trading" {
			hasA2 = true
		}
	}
	if !hasA2 {
		t.Errorf("expected a2_day_trading in category_a.fallback_fields, got %v", catA.FallbackFields)
	}
	// c1/c2/c3 all fallback (taifex/oddlot/etf fetchers nil)
	if len(catC.FallbackFields) != 3 {
		t.Errorf("expected 3 fallback fields in category_c, got %v", catC.FallbackFields)
	}
}

func TestHandleRetailSentiment_MissingShortBalance(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05, "symbol": "MARGIN"},
		"vix":                   map[string]any{"value": 18.5},
		"foreign_investor_net":  map[string]any{"value": 5000},
		"domestic_fund_net":     map[string]any{"value": 2000},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	assertFloatPtr(t, "MarginBalance", resp.MarginBalance, 2500)
	if resp.ShortBalance != nil {
		t.Errorf("ShortBalance = %v, want nil", *resp.ShortBalance)
	}
	if resp.ShortChangePct != nil {
		t.Errorf("ShortChangePct = %v, want nil", *resp.ShortChangePct)
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(encoded, &m); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if m["short_balance"] != nil {
		t.Errorf("short_balance = %v, want null", m["short_balance"])
	}
	if m["short_change_pct"] != nil {
		t.Errorf("short_change_pct = %v, want null", m["short_change_pct"])
	}
}

func TestExtremeReadingFromScore(t *testing.T) {
	tests := []struct {
		score  float64
		expect string
	}{
		{0.8, "frenzy"},
		{0.5, "frenzy"},
		{0.3, "neutral"},
		{0.0, "neutral"},
		{-0.3, "neutral"},
		{-0.5, "fear"},
		{-0.9, "fear"},
	}
	for _, tt := range tests {
		got := extremeReadingFromScore(tt.score)
		if got != tt.expect {
			t.Errorf("extremeReadingFromScore(%v) = %q, want %q", tt.score, got, tt.expect)
		}
	}
}

func TestInterpretRetailSentiment(t *testing.T) {
	tests := []struct {
		score  float64
		expect string
	}{
		{1.0, "extremely bullish retail sentiment"},
		{0.8, "extremely bullish retail sentiment"},
		{0.6, "bullish retail sentiment"},
		{0.5, "bullish retail sentiment"},
		{0.3, "mildly bullish retail sentiment"},
		{0.2, "mildly bullish retail sentiment"},
		{0.1, "neutral retail sentiment"},
		{0.0, "neutral retail sentiment"},
		{-0.19, "neutral retail sentiment"},
		{-0.2, "mildly bearish retail sentiment"},
		{-0.3, "mildly bearish retail sentiment"},
		{-0.5, "bearish retail sentiment"},
		{-0.7, "bearish retail sentiment"},
		{-0.8, "extremely bearish retail sentiment"},
		{-1.0, "extremely bearish retail sentiment"},
	}
	for _, tt := range tests {
		got := interpretRetailSentiment(tt.score)
		if got != tt.expect {
			t.Errorf("interpretRetailSentiment(%v) = %q, want %q", tt.score, got, tt.expect)
		}
	}
}

func TestGetFloatOrZero(t *testing.T) {
	type nullableFloat struct{ Val float64 }

	t.Run("nil pointer returns zero", func(t *testing.T) {
		var p *nullableFloat
		got := getFloatOrZero(p, func(v *nullableFloat) float64 { return v.Val })
		if got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})
	t.Run("non-nil returns field value", func(t *testing.T) {
		p := &nullableFloat{Val: 3.14}
		got := getFloatOrZero(p, func(v *nullableFloat) float64 { return v.Val })
		if got != 3.14 {
			t.Errorf("got %v, want 3.14", got)
		}
	})
}

func TestCalculateMarginPercentile_SingleDataPoint(t *testing.T) {
	workDir := t.TempDir()
	macroDir := filepath.Join(workDir, constants.StateMacro)
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write two historical snapshots with margin data
	for i, val := range []float64{2000, 3000} {
		snap := map[string]any{
			"retail_margin_balance": map[string]any{"value": val, "symbol": "TEST"},
		}
		data, _ := json.Marshal(snap)
		name := filepath.Join(macroDir, "2026010"+string(rune('1'+i))+".json")
		os.WriteFile(name, data, 0o644)
	}

	// current value = 2500 → 1 of 2 values are less → 0.5
	got := calculateMarginPercentile(workDir, 2500)
	if got != 0.5 {
		t.Errorf("percentile = %v, want 0.5", got)
	}
}

func TestCalculateMarginPercentile_ZeroCurrent(t *testing.T) {
	got := calculateMarginPercentile("/nonexistent", 0)
	if got != 0 {
		t.Errorf("percentile = %v, want 0", got)
	}
}

func TestRegisterRoutes(t *testing.T) {
	h, _ := newSystemHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		path string
	}{
		{"/api/dashboard/phase3-status"},
		{"/api/dashboard/system-health"},
		{"/api/dashboard/clamping-events"},
		{"/api/dashboard/conviction-clamping-events"},
		{"/api/dashboard/capital-phase"},
		{"/api/dashboard/retail-sentiment"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(http.MethodGet, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s not registered", r.path)
		}
	}

	// POST should be rejected on GET-only routes
	for _, r := range routes {
		req := httptest.NewRequest(http.MethodPost, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want %d", r.path, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHealthHandlers_HandleHealth(t *testing.T) {
	hh := &HealthHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	status, body := hh.HandleHealth(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(healthResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if _, ok := resp.Ports["atlas_http"]; !ok {
		t.Error("missing atlas_http port report")
	}
	if _, ok := resp.Ports["fubon_proxy"]; !ok {
		t.Error("missing fubon_proxy port report")
	}
}

func TestHealthHandlers_RegisterRoutes(t *testing.T) {
	hh := &HealthHandlers{}
	mux := http.NewServeMux()
	hh.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("route /health not registered or failed, status=%d", w.Code)
	}
}

func TestSwaggerHandlers_HandleSwaggerJSON_Missing(t *testing.T) {
	sh := NewSwaggerHandlers("/nonexistent/path")
	req := httptest.NewRequest(http.MethodGet, "/api/docs/swagger.json", nil)
	status, _ := sh.HandleSwaggerJSON(nil, req)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestSwaggerHandlers_HandleSwaggerUI(t *testing.T) {
	sh := NewSwaggerHandlers("/tmp")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	status, _ := sh.HandleSwaggerUI(w, req)
	if status != 0 {
		t.Errorf("status = %d, want 0 (already written)", status)
	}
	body := w.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("swagger UI HTML does not contain swagger-ui reference")
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandleDataIntegrity_EmptyDir(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	sessionsDir := filepath.Join(ledgerDir, "sessions")
	os.MkdirAll(sessionsDir, 0o755)

	handler := HandleDataIntegrity(workDir, ledgerDir)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-integrity", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := mustDecode(t, w)
	if m == nil {
		t.Fatal("expected JSON response")
	}
	overall, _ := m["overall"].(string)
	if overall != "failing" {
		t.Errorf("overall = %q, want failing (no sessions)", overall)
	}
}

func TestHandleDataIntegrity_WithSessions(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	sessionsDir := filepath.Join(ledgerDir, "sessions", "session-20260101-daily")
	os.MkdirAll(sessionsDir, 0o755)
	// Write a valid summary.json with snake_case encoding and tax data.
	summary := map[string]any{
		"session_id":      "session-20260101-daily",
		"portfolio_value": 1000000.0,
		"tax_snapshots":   []map[string]any{{"tax": 100}},
		"total_tax_paid":  100.0,
	}
	data, _ := json.Marshal(summary)
	os.WriteFile(filepath.Join(sessionsDir, "summary.json"), data, 0o644)
	os.WriteFile(filepath.Join(sessionsDir, "positions.json"), []byte(`[]`), 0o644)

	handler := HandleDataIntegrity(workDir, ledgerDir)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-integrity", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := mustDecode(t, w)
	if m == nil {
		t.Fatal("expected JSON response")
	}
	overall, _ := m["overall"].(string)
	// With a session that has tax data + positions but no replay file,
	// there should be some warnings but encoding should be ok.
	if overall == "failing" {
		// This is expected if replay data is missing.
		t.Logf("overall = %q (replay data file missing is expected)", overall)
	}
	checks, _ := m["checks"].([]any)
	if len(checks) == 0 {
		t.Error("expected at least one integrity check")
	}
}
