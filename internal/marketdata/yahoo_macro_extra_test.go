package marketdata

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// YahooFinanceMacroProvider fetches 9 macro tickers (DXY, US10Y, VIX, Oil, Gold, USDTWD, Silver, Copper).
// Reference snapshots (2026-04-29, illustrative):
//   DX-Y.NYB (DXY)      : 102.45 → 104.18 (+1.69%)
//   ^TNX (US 10Y yield) : 4.20 → 4.55 (+8.33%)
//   ^VIX                : 14.20 → 18.40 (+29.58%)
//   CL=F (WTI Crude)    : 78.00 → 82.50 (+5.77%)
//   GC=F (Gold)         : 2,340 → 2,510 (+7.26%)
//   USDTWD=X            : 32.10 → 32.45 (+1.09%)
//   SI=F (Silver)       : 27.50 → 31.20 (+13.45%)
//   HG=F (Copper)       : 4.55 → 4.80 (+5.49%)

func TestYahooFinanceMacroProvider_Name(t *testing.T) {
	if got := NewYahooFinanceMacroProvider().Name(); got != "yahoo_finance" {
		t.Errorf("Name() = %q, want yahoo_finance", got)
	}
}

func TestYahooFinanceMacroProvider_FetchSnapshot_AllSuccess(t *testing.T) {
	var requestPaths sync.Map // path → *atomic.Int32
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count, _ := requestPaths.LoadOrStore(r.URL.Path, new(atomic.Int32))
		count.(*atomic.Int32).Add(1)

		var closes [2]float64
		switch {
		case strings.Contains(r.URL.Path, "DX-Y.NYB"):
			closes = [2]float64{102.45, 104.18}
		case strings.Contains(r.URL.Path, "^TNX"):
			closes = [2]float64{4.20, 4.55}
		case strings.Contains(r.URL.Path, "^VIX"):
			closes = [2]float64{14.20, 18.40}
		case strings.Contains(r.URL.Path, "CL=F"):
			closes = [2]float64{78.00, 82.50}
		case strings.Contains(r.URL.Path, "GC=F"):
			closes = [2]float64{2340.0, 2510.0}
		case strings.Contains(r.URL.Path, "USDTWD=X"):
			closes = [2]float64{32.10, 32.45}
		case strings.Contains(r.URL.Path, "SI=F"):
			closes = [2]float64{27.50, 31.20}
		case strings.Contains(r.URL.Path, "HG=F"):
			closes = [2]float64{4.55, 4.80}
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body := fmt.Sprintf(`{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[%s,%s]}]}}]}}`,
			f64s(closes[0]), f64s(closes[1]))
		w.Write([]byte(body))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	snap, err := NewYahooFinanceMacroProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error: %v", err)
	}
	if snap.DXY.Value != 104.18 {
		t.Errorf("DXY.Value = %v, want 104.18", snap.DXY.Value)
	}
	if snap.US10Y.Value != 4.55 {
		t.Errorf("US10Y.Value = %v, want 4.55", snap.US10Y.Value)
	}
	if snap.VIX.Value != 18.40 {
		t.Errorf("VIX.Value = %v, want 18.40", snap.VIX.Value)
	}
	if snap.Oil.Value != 82.50 {
		t.Errorf("Oil.Value = %v, want 82.50", snap.Oil.Value)
	}
	if snap.Gold.Value != 2510.0 {
		t.Errorf("Gold.Value = %v, want 2510.0", snap.Gold.Value)
	}
	if snap.USD_TWD.Value != 32.45 {
		t.Errorf("USD_TWD.Value = %v, want 32.45", snap.USD_TWD.Value)
	}
	if snap.Silver.Value != 31.20 {
		t.Errorf("Silver.Value = %v, want 31.20", snap.Silver.Value)
	}
	if snap.Copper.Value != 4.80 {
		t.Errorf("Copper.Value = %v, want 4.80", snap.Copper.Value)
	}
}

func TestYahooFinanceMacroProvider_FetchSnapshot_PartialFailure(t *testing.T) {
	// P1-14: use a 500 (not 429) for the failing ticker — a 429 now arms the
	// session-level negative cache and short-circuits the concurrent fetches
	// (the intended new behavior, covered by TestYahooNegativeCache_*). A 500
	// keeps this test's original intent: one upstream failure is tolerated
	// and the successful paths still populate the snapshot.
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "^VIX") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[100.0, 101.0]}]}}]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	snap, err := NewYahooFinanceMacroProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected partial failure error")
	}
	if snap.DXY.Value != 101.0 {
		t.Errorf("DXY.Value = %v, want 101.0 (success path still populates)", snap.DXY.Value)
	}
}

func TestYahooFinanceMacroProvider_fetchIndicator_RateLimited(t *testing.T) {
	y := NewYahooFinanceMacroProvider()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := y.fetchIndicator(ctx, "DX-Y.NYB")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestYahooFinanceMacroProvider_fetchIndicator_NoChartResult(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	y := NewYahooFinanceMacroProvider()
	_, err := y.fetchIndicator(context.Background(), "DX-Y.NYB")
	if err == nil {
		t.Fatal("expected error for no chart result")
	}
}

func TestYahooFinanceMacroProvider_fetchIndicator_NoClosePrices(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"indicators":{"quote":[{"close":[]}]}}]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	y := NewYahooFinanceMacroProvider()
	_, err := y.fetchIndicator(context.Background(), "DX-Y.NYB")
	if err == nil {
		t.Fatal("expected error for empty close prices")
	}
}

func TestYahooFinanceMacroProvider_fetchIndicator_ZeroLatestPrice(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[0.0,0.0,0.0]}]}}]}}`))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	y := NewYahooFinanceMacroProvider()
	_, err := y.fetchIndicator(context.Background(), "^TNX")
	if err == nil {
		t.Fatal("expected error for zero latest price (^TNX)")
	}
	if !strings.Contains(err.Error(), "zero latest price") {
		t.Errorf("err = %v, want it to mention 'zero latest price'", err)
	}
}

func TestYahooFinanceMacroProvider_FetchSnapshot_ZeroValueExcluded(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		var body string
		switch {
		case strings.Contains(r.URL.Path, "^TNX"):
			body = `{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[0.0,0.0]}]}}]}}`
		case strings.Contains(r.URL.Path, "DX-Y.NYB"):
			body = `{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[102.45,104.18]}]}}]}}`
		default:
			body = `{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[1.0,1.0]}]}}]}}`
		}
		w.Write([]byte(body))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	snap, err := NewYahooFinanceMacroProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected partial failure error from ^TNX zero guard")
	}
	if snap.DXY.Value != 104.18 {
		t.Errorf("DXY.Value = %v, want 104.18 (success path should still populate)", snap.DXY.Value)
	}
	if snap.US10Y.Value != 0 {
		t.Errorf("US10Y.Value = %v, want 0 (zero guard should leave field empty)", snap.US10Y.Value)
	}
	if snap.US10Y.Symbol != "" {
		t.Errorf("US10Y.Symbol = %q, want empty (zero guard should leave field empty)", snap.US10Y.Symbol)
	}
}

func TestMockMacroProvider(t *testing.T) {
	want := MacroDataSnapshot{
		DXY: MacroDataPoint{Symbol: "DX-Y.NYB", Value: 104.18, ChangePct: 1.69},
	}
	m := &MockMacroProvider{Snapshot: want}
	if m.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", m.Name())
	}
	got, err := m.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error: %v", err)
	}
	if got.DXY.Value != 104.18 {
		t.Errorf("DXY.Value = %v, want 104.18", got.DXY.Value)
	}
	if got.DXY.ChangePct != 1.69 {
		t.Errorf("DXY.ChangePct = %v, want 1.69", got.DXY.ChangePct)
	}
}

// fetchIndicatorWithCloses spins up a fake Yahoo server that returns a chart
// with the given closes array, then runs fetchIndicator against it.
func fetchIndicatorWithCloses(t *testing.T, ticker string, closes []float64) (MacroDataPoint, error) {
	t.Helper()
	usCache.reset()
	var parts []string
	for _, c := range closes {
		parts = append(parts, f64s(c))
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body := fmt.Sprintf(`{"chart":{"result":[{"meta":{"regularMarketTime":1714500000},"indicators":{"quote":[{"close":[%s]}]}}]}}`,
			strings.Join(parts, ","))
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(ts.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(ts.Client())

	return NewYahooFinanceMacroProvider().fetchIndicator(context.Background(), ticker)
}

// TestYahooFinanceMacroProvider_fetchIndicator_ValidLatestZeroGap covers the
// DXY change_pct bug (user acceptance): Yahoo returns closes where the latest
// value is valid but the second-to-last entry is 0 (off-hours). The old code
// only checked closes[len-2], so prev fell back to latest → ChangePct=0.
// The fix walks back to the previous valid non-zero close.
func TestYahooFinanceMacroProvider_fetchIndicator_ValidLatestZeroGap(t *testing.T) {
	// [98.5, 0.0, 99.43]: latest=99.43 valid, closes[len-2]=0 → prev must be 98.5.
	point, err := fetchIndicatorWithCloses(t, "DX-Y.NYB", []float64{98.5, 0.0, 99.43})
	if err != nil {
		t.Fatalf("fetchIndicator error: %v", err)
	}
	if point.Value != 99.43 {
		t.Errorf("Value = %v, want 99.43", point.Value)
	}
	wantPct := (99.43 - 98.5) / 98.5 * 100
	if math.Abs(point.ChangePct-wantPct) > 1e-9 {
		t.Errorf("ChangePct = %.6f%%, want %.6f%% (prev must be 98.5, not latest)", point.ChangePct, wantPct)
	}
}

// TestYahooFinanceMacroProvider_fetchIndicator_SingleValidClose: only one
// close in the whole series — there is genuinely no previous day, so
// ChangePct=0 is the correct (and only possible) answer.
func TestYahooFinanceMacroProvider_fetchIndicator_SingleValidClose(t *testing.T) {
	point, err := fetchIndicatorWithCloses(t, "DX-Y.NYB", []float64{99.43})
	if err != nil {
		t.Fatalf("fetchIndicator error: %v", err)
	}
	if point.Value != 99.43 {
		t.Errorf("Value = %v, want 99.43", point.Value)
	}
	if point.ChangePct != 0 {
		t.Errorf("ChangePct = %v, want 0 (single close, no previous day)", point.ChangePct)
	}
}

// TestYahooFinanceMacroProvider_fetchIndicator_LeadingZerosSingleValid:
// closes=[0, 0, 99.43] contains only one valid value (99.43), so ChangePct=0
// is correct — the fix must not invent a prev from zeros.
func TestYahooFinanceMacroProvider_fetchIndicator_LeadingZerosSingleValid(t *testing.T) {
	point, err := fetchIndicatorWithCloses(t, "DX-Y.NYB", []float64{0.0, 0.0, 99.43})
	if err != nil {
		t.Fatalf("fetchIndicator error: %v", err)
	}
	if point.Value != 99.43 {
		t.Errorf("Value = %v, want 99.43", point.Value)
	}
	if point.ChangePct != 0 {
		t.Errorf("ChangePct = %v, want 0 (only one valid close; zeros are not data)", point.ChangePct)
	}
}

// TestYahooFinanceMacroProvider_fetchIndicator_TrailingZeroBeforeLatest:
// closes=[98.5, 0.0, 0.0, 99.43] — multiple trailing zeros between latest and
// the previous valid close; the fix must skip all of them.
func TestYahooFinanceMacroProvider_fetchIndicator_TrailingZeroBeforeLatest(t *testing.T) {
	point, err := fetchIndicatorWithCloses(t, "DX-Y.NYB", []float64{98.5, 0.0, 0.0, 99.43})
	if err != nil {
		t.Fatalf("fetchIndicator error: %v", err)
	}
	if point.Value != 99.43 {
		t.Errorf("Value = %v, want 99.43", point.Value)
	}
	wantPct := (99.43 - 98.5) / 98.5 * 100
	if math.Abs(point.ChangePct-wantPct) > 1e-9 {
		t.Errorf("ChangePct = %.6f%%, want %.6f%% (skip all trailing zeros)", point.ChangePct, wantPct)
	}
}

// f64s is a small helper to render floats deterministically for JSON payloads.
func f64s(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
