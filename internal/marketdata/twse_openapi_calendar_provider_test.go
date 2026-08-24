package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTWSEOpenAPICalendarProvider_Name(t *testing.T) {
	p := NewTWSEOpenAPICalendarProvider()
	if p.Name() != "twse_openapi" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

// openAPICalendarStub serves the two OpenAPI v1 endpoints used by the
// provider: TWT48U_ALL (除權除息預告表) and t187ap41_L (股東會日期彙總表).
func openAPICalendarStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/exchangeReport/TWT48U_ALL":
			_, _ = w.Write([]byte(`[
  {"Date":"1150605","Code":"2330","Name":"台積電","Exdividend":"息","StockDividendRatio":"","CashDividend":"4.5"},
  {"Date":"1150615","Code":"2454","Name":"聯發科","Exdividend":"權","StockDividendRatio":"0.5","CashDividend":""},
  {"Date":"1150710","Code":"1101","Name":"台泥","Exdividend":"權息","StockDividendRatio":"0.2","CashDividend":"0.8"},
  {"Date":"1150915","Code":"9999","Name":"明年事件","Exdividend":"息","StockDividendRatio":"","CashDividend":"1"}
]`))
		case "/opendata/t187ap41_L":
			_, _ = w.Write([]byte(`[
  {"公司代號":"2330","公司名稱":"台積電","開會日期":"1150522","股東常(臨時)會":"常會"},
  {"公司代號":"2454","公司名稱":"聯發科","開會日期":"1150610","股東常(臨時)會":"臨時會"}
]`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTWSEOpenAPICalendarProvider_FetchEventsCurrentYear(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := openAPICalendarStub(t)
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	events, err := p.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents failed: %v", err)
	}
	// 3 ex-dividend events in 2026 (4th row is 2026-09 → still 2026, so 4)
	// + 2 shareholder meetings = 6 total.
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}

	var exDividend, meetings int
	for _, e := range events {
		if e.Source != "twse_openapi" {
			t.Errorf("event %s source = %q, want twse_openapi", e.Date, e.Source)
		}
		switch e.EventType {
		case "ex_dividend":
			exDividend++
		case "shareholder_meeting":
			meetings++
		default:
			t.Errorf("unexpected event type %q", e.EventType)
		}
	}
	if exDividend != 4 {
		t.Errorf("expected 4 ex_dividend events, got %d", exDividend)
	}
	if meetings != 2 {
		t.Errorf("expected 2 shareholder_meeting events, got %d", meetings)
	}
}

func TestTWSEOpenAPICalendarProvider_FetchEventsYearFilter(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := openAPICalendarStub(t)
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	// Request the current year but the stub emits a 1150915 (=2026-09-15)
	// row — still inside 2026, so it must be kept.
	events, err := p.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents failed: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Symbol == "9999" {
			found = true
			if e.Date != "2026-09-15" {
				t.Errorf("ROC date 1150915 mapped to %s, want 2026-09-15", e.Date)
			}
		}
	}
	if !found {
		t.Error("expected the 1150915 row to be kept (year 2026)")
	}
}

func TestTWSEOpenAPICalendarProvider_FetchEventsPastYearSkipped(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := openAPICalendarStub(t)
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	// OpenAPI v1 serves the current snapshot only — requesting 2024 must
	// return empty without error (documented limitation).
	events, err := p.FetchEvents(context.Background(), 2024)
	if err != nil {
		t.Fatalf("FetchEvents(2024) should not error, got: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events for a past year, got %d", len(events))
	}
}

func TestTWSEOpenAPICalendarProvider_ExDividendMapping(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`[
  {"Date":"1150605","Code":"2330","Name":"台積電","Exdividend":"息","StockDividendRatio":"","CashDividend":"4.5"},
  {"Date":"1150615","Code":"2454","Name":"聯發科","Exdividend":"權","StockDividendRatio":"0.5","CashDividend":""}
]`))
	}))
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	events, err := p.fetchExDividend(context.Background(), 2026)
	if err != nil {
		t.Fatalf("fetchExDividend failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Date != "2026-06-05" || events[0].Symbol != "2330" || events[0].EventType != "ex_dividend" {
		t.Errorf("unexpected first event: %+v", events[0])
	}
	if events[0].Direction != "mixed" {
		t.Errorf("ex_dividend direction = %s, want mixed", events[0].Direction)
	}
	// 息-only → weight 0.4; 權 → weight 0.35
	if events[0].Weight != 0.4 {
		t.Errorf("cash dividend weight = %v, want 0.4", events[0].Weight)
	}
	if events[1].Weight != 0.35 {
		t.Errorf("stock dividend weight = %v, want 0.35", events[1].Weight)
	}
}

func TestTWSEOpenAPICalendarProvider_MeetingMapping(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`[
  {"公司代號":"2330","公司名稱":"台積電","開會日期":"1150522","股東常(臨時)會":"常會"}
]`))
	}))
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	events, err := p.fetchShareholderMeetings(context.Background(), 2026)
	if err != nil {
		t.Fatalf("fetchShareholderMeetings failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Date != "2026-05-22" || e.Symbol != "2330" || e.EventType != "shareholder_meeting" {
		t.Errorf("unexpected event: %+v", e)
	}
	if e.Direction != "bullish" {
		t.Errorf("meeting direction = %s, want bullish", e.Direction)
	}
	if !strings.Contains(e.Description, "常會") {
		t.Errorf("description should include meeting type, got %q", e.Description)
	}
}

func TestTWSEOpenAPICalendarProvider_AllEndpointsFail(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html>404</html>`))
	}))
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	events, err := p.FetchEvents(context.Background(), 2026)
	if err == nil {
		t.Fatal("expected error when both endpoints fail")
	}
	if events != nil {
		t.Errorf("expected nil events on total failure, got %d", len(events))
	}
}

func TestTWSEOpenAPICalendarProvider_PartialFailure(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		switch r.URL.Path {
		case "/exchangeReport/TWT48U_ALL":
			_, _ = w.Write([]byte(`[{"Date":"1150605","Code":"2330","Name":"台積電","Exdividend":"息"}]`))
		case "/opendata/t187ap41_L":
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	p := NewTWSEOpenAPICalendarProvider()
	p.SetHTTPClient(server.Client())
	p.baseURL = server.URL
	p.SetNow(func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) })

	events, err := p.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("partial failure should not error, got: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 partial event, got %d", len(events))
	}
}

func TestNormalizeROCDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1150907", "2026-09-07"}, // ROC 115 = 2026
		{"1150522", "2026-05-22"},
		{"1100101", "2021-01-01"}, // ROC 110 = 2021
		{"", ""},
		{"   ", ""},
		{"115907", ""},   // too short
		{"11509071", ""}, // too long
		{"11513x1", ""},  // non-digit
		{"1151301", ""},  // month 13
		{"1150032", ""},  // day 32
	}
	for _, tc := range cases {
		if got := normalizeROCDate(tc.in); got != tc.want {
			t.Errorf("normalizeROCDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
