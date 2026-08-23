package dailyreport

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestGenerate(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	rep := gen.Generate()
	if rep.Date == "" {
		t.Error("report date should not be empty")
	}
	if rep.Global.Summary == "" {
		t.Error("report should have global summary")
	}
	if rep.Capital.Resonance != 1.0 {
		t.Errorf("expected resonance 1.0, got %.2f", rep.Capital.Resonance)
	}
}

func TestHandleLatest(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	h := NewHandler(gen, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/latest", nil)
	rec := httptest.NewRecorder()
	h.HandleLatest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleArchive(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	gen.Generate()

	h := NewHandler(gen, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/archive?date=2025-01-01", nil)
	rec := httptest.NewRecorder()
	h.HandleArchive(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown date, got %d", rec.Code)
	}
}

func TestHandleSubscribe(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	h := NewHandler(gen, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/subscribe", nil)
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleSubscribe_GetRejected(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	h := NewHandler(gen, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/subscribe", nil)
	rec := httptest.NewRecorder()
	h.HandleSubscribe(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET, got %d", rec.Code)
	}
}

func TestHandleArchive_Success(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	rep := gen.Generate()

	h := NewHandler(gen, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/reports/archive?date="+rep.Date, nil)
	rec := httptest.NewRecorder()
	h.HandleArchive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleArchive_MissingDate(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	h := NewHandler(gen, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/archive", nil)
	rec := httptest.NewRecorder()
	h.HandleArchive(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestReport_Markdown(t *testing.T) {
	r := &Report{
		Date:      "2026-07-20",
		Generated: time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
		Summary:   "盤勢偏多（RISK_ON）：風險偏好回升",
		Global: GlobalOverview{
			BondYield: "4.25%", USDIndex: "104.5", JPY: "150.2", VIX: "14.3",
			Status: "RISK_ON", Summary: "盤勢偏多（RISK_ON）：風險偏好回升",
		},
		Capital: CapitalSection{
			Foreign: "淨買超50億", Institutional: "淨買超20億", Dealer: "淨賣超10億",
			Government: "中性", Retail: "淨賣超5億",
			Resonance: 1.5, Quality: "moderate_inflow",
		},
		Events: EventsSection{
			Tomorrow: []string{"FOMC會議"},
			ThisWeek: []string{"月營收公告", "MSCI調整"},
			Count:    3,
		},
		Strategy: StrategySection{
			Active: "all_weather", EntryCond: "等待回測", Direction: "偏多",
		},
		Risk: RiskSection{
			StressIndex: 0.35, DrawdownAlert: false, RiskLevel: "moderate",
		},
	}

	md := r.Markdown()

	checks := []string{
		"台股每日市場報告 — 2026-07-20",
		"美債殖利率：4.25%",
		"美元指數：104.5",
		"外資：淨買超50億",
		"投信：淨買超20億",
		"自營商：淨賣超10億",
		"FOMC會議",
		"MSCI調整",
		"all_weather",
		"壓力指數：0.35",
		"風險等級：moderate",
	}
	for _, check := range checks {
		if !strings.Contains(md, check) {
			t.Errorf("Markdown missing expected content: %s", check)
		}
	}
}

func TestReport_MarkdownWithWarning(t *testing.T) {
	r := &Report{
		Date:      "2026-07-20",
		Generated: time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
		Global:    GlobalOverview{BondYield: "4.25%", USDIndex: "104.5", JPY: "150.2", VIX: "14.3"},
		Capital:   CapitalSection{Resonance: 0.5, Quality: "neutral"},
		Risk: RiskSection{
			StressIndex: 0.8, DrawdownAlert: true, RiskLevel: "high",
			Warning: "警戒：外資連續三日淨賣超",
		},
	}

	md := r.Markdown()
	if !strings.Contains(md, "警戒：外資連續三日淨賣超") {
		t.Error("Markdown should include warning text")
	}
}

func TestSetProvider(t *testing.T) {
	gen := NewGenerator("")
	p := &mockProvider{}
	gen.SetProvider(p)
	if gen.provider != p {
		t.Error("SetProvider did not set provider")
	}
}

func TestSetRegimeProvider(t *testing.T) {
	gen := NewGenerator("")
	fn := func() domain.Regime { return domain.RegimeRiskOn }
	gen.SetRegimeProvider(fn)
	if gen.regimeGetter == nil {
		t.Error("SetRegimeProvider did not set regimeGetter")
	}
}

func TestResolveRegime_WithRegimeGetter(t *testing.T) {
	gen := NewGenerator("")
	gen.SetRegimeProvider(func() domain.Regime { return domain.RegimeRiskOff })

	got := gen.resolveRegime()
	if got != domain.RegimeRiskOff {
		t.Errorf("resolveRegime = %s, want RISK_OFF", got)
	}
}

func TestResolveRegime_WithoutRegimeGetter(t *testing.T) {
	gen := NewGenerator("")

	got := gen.resolveRegime()
	if got != domain.RegimeNeutral {
		t.Errorf("resolveRegime = %s, want NEUTRAL", got)
	}
}

func TestSummarizeGlobalStatus(t *testing.T) {
	tests := []struct {
		status string
		wantFn func(string) bool
	}{
		{string(domain.RegimeRiskOn), func(s string) bool { return strings.Contains(s, "偏多") }},
		{string(domain.RegimeRiskOff), func(s string) bool { return strings.Contains(s, "偏空") }},
		{string(domain.RegimeNeutral), func(s string) bool { return strings.Contains(s, "中性") }},
		{string(domain.RegimeNeutral), func(s string) bool { return strings.Contains(s, "中性") }},
		{"", func(s string) bool { return strings.Contains(s, "中性") }},
	}
	for _, tt := range tests {
		got := summarizeGlobalStatus(tt.status)
		if !tt.wantFn(got) {
			t.Errorf("summarizeGlobalStatus(%q) = %q, want match", tt.status, got)
		}
	}
}

func TestGenerate_RegimeOverridesStatus(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	gen.SetRegimeProvider(func() domain.Regime { return domain.RegimeRiskOff })

	rep := gen.Generate()
	if rep.Global.Status != "RISK_OFF" {
		t.Errorf("Global.Status = %s, want RISK_OFF", rep.Global.Status)
	}
	if rep.Summary != rep.Global.Summary {
		t.Error("Report.Summary should mirror Global.Summary")
	}
	if !strings.Contains(rep.Global.Summary, "偏空") {
		t.Errorf("Global.Summary = %q, should contain 偏空", rep.Global.Summary)
	}
}

func TestGetByDate(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	gen.Generate()

	date := time.Now().Format("2006-01-02")
	rep := gen.GetByDate(date)
	if rep == nil {
		t.Fatal("GetByDate should find today's report")
	}
	if rep.Date != date {
		t.Errorf("Date = %s, want %s", rep.Date, date)
	}

	missing := gen.GetByDate("2020-01-01")
	if missing != nil {
		t.Error("GetByDate should return nil for unknown date")
	}
}

func TestRegisterRoutes(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rpt-test")
	defer os.RemoveAll(dir)

	gen := NewGenerator(dir)
	trk := NewTracker(dir, filepath.Join(dir, "replay"))
	mux := http.NewServeMux()
	RegisterRoutes(mux, gen, trk)

	// ServeMux doesn't expose registered patterns directly;
	// verify by making requests that would 404 if not registered.
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		method, path string
		wantCode     int
	}{
		{"GET", "/api/reports/latest", 200},
		{"GET", "/api/reports/archive?date=2099-01-01", 404},
		{"POST", "/api/reports/subscribe", 200},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tt.wantCode {
			t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.wantCode)
		}
	}
}

type mockProvider struct{}

func (m *mockProvider) FetchMacro() (GlobalOverview, error) {
	return GlobalOverview{BondYield: "5.0%", USDIndex: "105.0", Status: "TEST"}, nil
}

func (m *mockProvider) FetchCapital() (CapitalSection, error) {
	return CapitalSection{Foreign: "測試偏多", Quality: "test"}, nil
}

func (m *mockProvider) FetchEvents(_ time.Time) (EventsSection, error) {
	return EventsSection{Tomorrow: []string{"測試事件"}, Count: 1}, nil
}
