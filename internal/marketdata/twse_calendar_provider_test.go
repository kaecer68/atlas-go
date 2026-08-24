package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTWSECalendarProvider_Name(t *testing.T) {
	p := NewTWSECalendarProvider()
	if p.Name() != "twse_calendar" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestTWSECalendarProvider_FetchExDividendMonth(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	// Mock TWSE exRight endpoint that returns ex-dividend data.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260501",
  "title": "除權息預告表",
  "fields": ["股票代號", "股票名稱", "除權息日期", "種類", "除權息前收盤價", "除權息參考價", "權值", "息值", "權值+息值", "漲停價", "跌停價", "開始交易日期", "現金股利發放日"],
  "data": [
    ["2330", "台積電", "20260615", "除息", "950.00", "938.00", "0", "12", "12", "1031.00", "844.00", "20260616", "20260710"],
    ["2454", "聯發科", "20260620", "除息", "1200.00", "1180.00", "0", "20", "20", "1298.00", "1062.00", "20260621", "20260715"]
  ]
}`))
	}))
	defer server.Close()

	p := NewTWSECalendarProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())

	events, err := p.fetchExDividendMonth(context.Background(), "20260501")
	if err != nil {
		t.Fatalf("fetchExDividendMonth failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Check TSMC event
	e0 := events[0]
	if e0.Symbol != "2330" {
		t.Fatalf("expected symbol 2330, got %s", e0.Symbol)
	}
	if e0.EventType != "ex_dividend" {
		t.Fatalf("expected event_type ex_dividend, got %s", e0.EventType)
	}
	if e0.Date != "2026-06-15" {
		t.Fatalf("expected date 2026-06-15, got %s", e0.Date)
	}
	if e0.Source != "twse" {
		t.Fatalf("expected source twse, got %s", e0.Source)
	}

	// Check Mediatek event
	e1 := events[1]
	if e1.Symbol != "2454" {
		t.Fatalf("expected symbol 2454, got %s", e1.Symbol)
	}
}

func TestTWSECalendarProvider_FetchShareholderMeetingMonth(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	// Mock TWSE meeting endpoint that returns shareholder meeting data.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
  "stat": "OK",
  "date": "20260601",
  "title": "股東會公告",
  "fields": ["公司代號", "公司名稱", "股東會日期", "最後過戶日", "停止過戶起始日期", "停止過戶截止日期", "紀念品代號", "紀念品名稱", "開會地點", "備註", "是否發放紀念品"],
  "data": [
    ["2330", "台積電", "20260605", "20260601", "20260602", "20260606", "", "", "新竹科學園區", "", "否"],
    ["2454", "聯發科", "20260610", "20260606", "20260607", "20260611", "", "", "新竹", "", "否"]
  ]
}`))
	}))
	defer server.Close()

	p := NewTWSECalendarProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())

	events, err := p.fetchMeetingMonth(context.Background(), "20260601")
	if err != nil {
		t.Fatalf("fetchMeetingMonth failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	e0 := events[0]
	if e0.Symbol != "2330" {
		t.Fatalf("expected symbol 2330, got %s", e0.Symbol)
	}
	if e0.EventType != "shareholder_meeting" {
		t.Fatalf("expected event_type shareholder_meeting, got %s", e0.EventType)
	}
	if e0.Date != "2026-06-05" {
		t.Fatalf("expected date 2026-06-05, got %s", e0.Date)
	}
	if e0.Direction != "bullish" {
		t.Fatalf("expected direction bullish, got %s", e0.Direction)
	}
}

func TestNormalizeTWDate_YYYYMMDD(t *testing.T) {
	result := normalizeTWDate("20260513")
	if result != "2026-05-13" {
		t.Fatalf("expected 2026-05-13, got %s", result)
	}
}

func TestNormalizeTWDate_ROC(t *testing.T) {
	// 115年 = 2026 (115 + 1911 = 2026)
	result := normalizeTWDate("115/05/13")
	if result != "2026-05-13" {
		t.Fatalf("expected 2026-05-13, got %s", result)
	}
}

func TestNormalizeTWDate_Empty(t *testing.T) {
	if normalizeTWDate("") != "" {
		t.Fatal("expected empty string")
	}
	if normalizeTWDate("  ") != "" {
		t.Fatal("expected empty string")
	}
}

func TestTWSECalendarProvider_FetchExDividendMonth_HTMLResponseDeprecated(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	// TWSE deprecated exRight endpoint (2026-06): returns 302 → /page-not-found.html
	// with text/html body. Provider must return empty events gracefully, not error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>404 Not Found</title></head><body>...</body></html>`))
	}))
	defer server.Close()

	p := NewTWSECalendarProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())

	events, err := p.fetchExDividendMonth(context.Background(), "20260501")
	if err != nil {
		t.Fatalf("expected nil error for HTML response (graceful deprecation), got: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events for deprecated endpoint, got %d events", len(events))
	}
}

func TestTWSECalendarProvider_FetchMeetingMonth_HTMLResponseDeprecated(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	// TWSE deprecated meeting endpoint (2026-06): same HTML fallback behavior.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte(`<html><body>Not Found</body></html>`))
	}))
	defer server.Close()

	p := NewTWSECalendarProvider()
	p.baseURL = server.URL
	p.SetHTTPClient(server.Client())

	events, err := p.fetchMeetingMonth(context.Background(), "20260601")
	if err != nil {
		t.Fatalf("expected nil error for HTML response (graceful deprecation), got: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events for deprecated endpoint, got %d events", len(events))
	}
}

func TestIsHTMLContentType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"json app", "application/json", false},
		{"json app with charset", "application/json; charset=UTF-8", false},
		{"html lowercase", "text/html", true},
		{"html uppercase", "text/HTML; charset=utf-8", true},
		{"html mixed case", "Text/Html", true},
		{"xhtml", "application/xhtml+xml", false},
		{"malformed", "this is not a mime type", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHTMLContentType(tt.in); got != tt.want {
				t.Errorf("isHTMLContentType(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// mockCalendarProvider is a simple in-memory provider for testing.
type mockCalendarProvider struct {
	name   string
	events []CalendarProviderData
	err    error
}

func (m *mockCalendarProvider) Name() string { return m.name }

func (m *mockCalendarProvider) FetchEvents(_ context.Context, _ int) ([]CalendarProviderData, error) {
	return m.events, m.err
}

func TestCompositeCalendarProvider_MergeLastWriteWins(t *testing.T) {
	p1 := &mockCalendarProvider{
		name: "provider_a",
		events: []CalendarProviderData{
			{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Name: "TSMC Div A", Source: "a"},
			{Date: "2026-06-10", EventType: "shareholder_meeting", Symbol: "2330", Name: "TSMC Meeting A", Source: "a"},
		},
	}
	p2 := &mockCalendarProvider{
		name: "provider_b",
		events: []CalendarProviderData{
			{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Name: "TSMC Div B", Source: "b"},
			{Date: "2026-06-20", EventType: "ex_dividend", Symbol: "2454", Name: "Mediatek Div", Source: "b"},
		},
	}

	composite := NewCompositeCalendarProvider(p1, p2)
	events, err := composite.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 unique events, got %d", len(events))
	}

	// The duplicate (2330, ex_dividend, 2026-06-15) should come from provider_b (last write wins)
	found := false
	for _, e := range events {
		if e.Symbol == "2330" && e.EventType == "ex_dividend" {
			found = true
			if e.Source != "b" {
				t.Fatalf("expected source 'b' (last write wins), got %s", e.Source)
			}
			if e.Name != "TSMC Div B" {
				t.Fatalf("expected name 'TSMC Div B', got %s", e.Name)
			}
		}
	}
	if !found {
		t.Fatal("expected to find TSMC ex_dividend event")
	}
}

func TestCompositeCalendarProvider_AddProvider(t *testing.T) {
	p1 := &mockCalendarProvider{
		name:   "provider_a",
		events: []CalendarProviderData{{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Source: "a"}},
	}
	p2 := &mockCalendarProvider{
		name:   "provider_b",
		events: []CalendarProviderData{{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Source: "b"}},
	}

	composite := NewCompositeCalendarProvider(p1)
	composite.AddProvider(p2)

	events, err := composite.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Source != "b" {
		t.Fatalf("expected source 'b', got %s", events[0].Source)
	}
}

func TestCompositeCalendarProvider_PartialFailure(t *testing.T) {
	p1 := &mockCalendarProvider{
		name: "provider_a",
		events: []CalendarProviderData{
			{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Source: "a"},
		},
	}
	p2 := &mockCalendarProvider{
		name: "provider_b",
		err:  context.DeadlineExceeded,
	}

	composite := NewCompositeCalendarProvider(p1, p2)
	events, err := composite.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents should not error on partial failure: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event from provider_a, got %d", len(events))
	}
}

func TestCompositeCalendarProvider_AllFail(t *testing.T) {
	p1 := &mockCalendarProvider{name: "a", err: context.DeadlineExceeded}
	p2 := &mockCalendarProvider{name: "b", err: context.DeadlineExceeded}

	composite := NewCompositeCalendarProvider(p1, p2)
	_, err := composite.FetchEvents(context.Background(), 2026)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestCompositeCalendarProvider_Name(t *testing.T) {
	composite := NewCompositeCalendarProvider()
	if composite.Name() != "composite_calendar" {
		t.Fatalf("expected composite_calendar, got %s", composite.Name())
	}
}
