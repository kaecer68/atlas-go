package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func resetTAIEXTwseTargetDate(t *testing.T) {
	t.Helper()
	orig := twseTAIEXTargetDate
	twseTAIEXTargetDate = func() time.Time { return time.Date(2026, 7, 29, 9, 0, 0, 0, twseLocation) }
	t.Cleanup(func() { twseTAIEXTargetDate = orig })
}

func TestTAIEXIndexProvider_Name(t *testing.T) {
	if got := NewTAIEXIndexProvider().Name(); got != "taiex_index" {
		t.Errorf("Name() = %q, want taiex_index", got)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_Success(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()
	// Pin the fallback clock to a trading day (2026-07-29 is a Wednesday) so the
	// weekend/holiday gate does not redirect to TWSE regardless of wall clock.
	resetTAIEXTwseTargetDate(t)

	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 23000.0},
					"indicators": {"quote": [{"close": [22800.0, 23000.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "^TWII") {
			t.Errorf("unexpected path: %s, expected ^TWII", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.TAIEX.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.TAIEX.Symbol)
	}
	if snap.TAIEX.Value != 23000.0 {
		t.Errorf("Value = %v, want 23000.0", snap.TAIEX.Value)
	}
	expectedPct := 0.88 // (23000-22800)/22800*100 = 0.877..., rounds to 0.88
	if snap.TAIEX.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.TAIEX.ChangePct, expectedPct)
	}
	if snap.TAIEX.Timestamp != 1714500000 {
		t.Errorf("Timestamp = %v, want 1714500000", snap.TAIEX.Timestamp)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_NoChartResult_FallsBackThenFails(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()
	resetTAIEXTwseTargetDate(t)

	// Yahoo returns an empty chart result.
	yahooSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer yahooSrv.Close()

	// TWSE fallback also fails, so the provider must return an error.
	twseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer twseSrv.Close()

	origTWSEURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = twseSrv.URL + "/exchangeReport/MI_INDEX"
	defer func() { taiexTWSEBaseURL = origTWSEURL }()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(yahooSrv.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(yahooSrv.Client())

	_, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("FetchSnapshot() expected error when both Yahoo and TWSE fallback fail")
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_TWSEFallbackSuccess(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()
	resetTAIEXTwseTargetDate(t)

	// Yahoo server always fails.
	yahooSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("yahoo down"))
	}))
	defer yahooSrv.Close()

	// TWSE server returns the requested date (2026-07-29, ROC 115/07/29).
	twseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"stat":"OK","tables":[{"title":"115年07月29日 價格指數(臺灣證券交易所)","fields":["指數","收盤指數","漲跌(+/-)","漲跌點數","漲跌百分比(%)","特殊處理註記"],"data":[["發行量加權股價指數","43,654.84","<p style ='color:green'>-</p>","1,195.97","-2.67",""]]}]}`))
	}))
	defer twseSrv.Close()

	origTWSEURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = twseSrv.URL + "/exchangeReport/MI_INDEX"
	defer func() { taiexTWSEBaseURL = origTWSEURL }()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(yahooSrv.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(yahooSrv.Client())

	snap, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.TAIEX.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.TAIEX.Symbol)
	}
	if snap.TAIEX.Value != 43654.84 {
		t.Errorf("Value = %v, want 43654.84", snap.TAIEX.Value)
	}
	if snap.TAIEX.ChangePct != -2.67 {
		t.Errorf("ChangePct = %v, want -2.67", snap.TAIEX.ChangePct)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_TWSEFallbackRejectsStaleDate(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()
	resetTAIEXTwseTargetDate(t)

	yahooSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer yahooSrv.Close()

	// TWSE response title date (2026-07-24) does not match requested date (2026-07-29).
	twseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"stat":"OK","tables":[{"title":"115年07月24日 價格指數(臺灣證券交易所)","fields":["指數","收盤指數","漲跌(+/-)","漲跌點數","漲跌百分比(%)","特殊處理註記"],"data":[["發行量加權股價指數","43,654.84","-","1,195.97","-2.67",""]]}]}`))
	}))
	defer twseSrv.Close()

	origTWSEURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = twseSrv.URL + "/exchangeReport/MI_INDEX"
	defer func() { taiexTWSEBaseURL = origTWSEURL }()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(yahooSrv.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(yahooSrv.Client())

	_, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when TWSE fallback returns stale date")
	}
	if !strings.Contains(err.Error(), "stale") && !strings.Contains(err.Error(), "reported date") {
		t.Fatalf("expected stale/reported-date error, got %v", err)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_BothSourcesFail(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()
	resetTAIEXTwseTargetDate(t)

	yahooSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer yahooSrv.Close()

	twseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer twseSrv.Close()

	origTWSEURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = twseSrv.URL + "/exchangeReport/MI_INDEX"
	defer func() { taiexTWSEBaseURL = origTWSEURL }()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(yahooSrv.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(yahooSrv.Client())

	_, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when both Yahoo and TWSE fail")
	}
	if !strings.Contains(err.Error(), "taiex_index") {
		t.Errorf("expected wrapped taiex_index error, got %v", err)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_WeekendBypassesYahoo(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	// 2026-07-25 is a Saturday. The weekend gate must bypass Yahoo and serve the
	// previous trading day (Friday 2026-07-24) from TWSE without tripping the
	// channel circuit breaker.
	orig := twseTAIEXTargetDate
	twseTAIEXTargetDate = func() time.Time { return time.Date(2026, 7, 25, 3, 0, 0, 0, twseLocation) }
	t.Cleanup(func() { twseTAIEXTargetDate = orig })

	yahooCalled := false
	yahooSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		yahooCalled = true
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer yahooSrv.Close()

	// TWSE returns Friday's report, matching the rolled-back request date.
	twseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "date=20260724") {
			t.Errorf("expected rolled-back TWSE date 20260724, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"stat":"OK","tables":[{"title":"115年07月24日 價格指數(臺灣證券交易所)","fields":["指數","收盤指數","漲跌(+/-)","漲跌點數","漲跌百分比(%)","特殊處理註記"],"data":[["發行量加權股價指數","43,654.84","-","1,195.97","-2.67",""]]}]}`))
	}))
	defer twseSrv.Close()

	origTWSEURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = twseSrv.URL + "/exchangeReport/MI_INDEX"
	defer func() { taiexTWSEBaseURL = origTWSEURL }()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(yahooSrv.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(yahooSrv.Client())

	snap, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() on weekend should serve previous trading day, got error = %v", err)
	}
	if yahooCalled {
		t.Error("weekend FetchSnapshot should bypass Yahoo entirely")
	}
	if snap.TAIEX.Value != 43654.84 {
		t.Errorf("Value = %v, want 43654.84 (previous trading day close)", snap.TAIEX.Value)
	}
	if snap.TAIEX.ChangePct != -2.67 {
		t.Errorf("ChangePct = %v, want -2.67", snap.TAIEX.ChangePct)
	}
}

func TestFetchTWSETAIEXFallback_RejectsMismatchedDate(t *testing.T) {
	resetTAIEXTwseTargetDate(t)

	twseSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"stat":"OK","tables":[{"title":"115年07月24日 價格指數(臺灣證券交易所)","fields":["指數","收盤指數","漲跌(+/-)","漲跌點數","漲跌百分比(%)","特殊處理註記"],"data":[["發行量加權股價指數","43,654.84","-","1,195.97","-2.67",""]]}]}`))
	}))
	defer twseSrv.Close()

	origTWSEURL := taiexTWSEBaseURL
	taiexTWSEBaseURL = twseSrv.URL + "/exchangeReport/MI_INDEX"
	defer func() { taiexTWSEBaseURL = origTWSEURL }()

	_, err := fetchTWSETAIEXFallback(context.Background())
	if err == nil {
		t.Fatal("expected error for mismatched date")
	}
	if !strings.Contains(err.Error(), "reported date") {
		t.Fatalf("expected reported-date mismatch error, got %v", err)
	}
}
