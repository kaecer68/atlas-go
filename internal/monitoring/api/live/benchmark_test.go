package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func float64Ptr(v float64) *float64 { return &v }

func TestHandleBenchmarkComparison_SessionsWithData(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")

	session1 := domain.SessionSummary{
		SessionID:      "session-20260413-daily",
		PortfolioValue: 1000000.0,
		Regime:         domain.RegimeNeutral,
	}
	session2 := domain.SessionSummary{
		SessionID:      "session-20260414-daily",
		PortfolioValue: 1020000.0,
		Regime:         domain.RegimeNeutral,
	}
	session3 := domain.SessionSummary{
		SessionID:      "session-20260415-daily",
		PortfolioValue: 1050000.0,
		Regime:         domain.RegimeNeutral,
	}

	for _, s := range []domain.SessionSummary{session1, session2, session3} {
		dir := filepath.Join(sessionsDir, s.SessionID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("os.MkdirAll: %v", err)
		}
		b, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), b, 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
	}

	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/benchmark-comparison", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleBenchmarkComparison(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if result["session_count"] != float64(3) {
		t.Errorf("session_count = %v, want 3", result["session_count"])
	}

	if result["portfolio_return"] != 0.05 {
		t.Errorf("portfolio_return = %v, want 0.05", result["portfolio_return"])
	}

	curve, ok := result["equity_curve"].([]any)
	if !ok {
		t.Fatal("equity_curve is not an array")
	}
	if len(curve) != 3 {
		t.Fatalf("len(equity_curve) = %d, want 3", len(curve))
	}

	point0, ok := curve[0].(map[string]any)
	if !ok {
		t.Fatal("equity_curve[0] is not a map")
	}
	for _, key := range []string{"label", "portfolio", "benchmark", "outperf"} {
		if _, exists := point0[key]; !exists {
			t.Errorf("key %q missing from equity_curve[0]", key)
		}
	}
	if point0["portfolio"] != 100.0 {
		t.Errorf("portfolio[0] = %v, want 100.0", point0["portfolio"])
	}
	if point0["benchmark"] != 100.0 {
		t.Errorf("benchmark[0] = %v, want 100.0", point0["benchmark"])
	}
}

func TestHandleBenchmarkComparison_EmptySessions(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/benchmark-comparison", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleBenchmarkComparison(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal response: %v", err)
	}

	if result["session_count"] != float64(0) {
		t.Errorf("session_count = %v, want 0", result["session_count"])
	}
}

func TestHandleBenchmarkComparison_NoSessionsDir(t *testing.T) {
	tmpDir := t.TempDir()

	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/benchmark-comparison", nil)

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleBenchmarkComparison(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleBenchmarkComparison_MethodNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATLAS_API_KEY", "test-key")
	svc := service.NewLiveService(tmpDir, tmpDir)
	h := &Handlers{
		LedgerDir: tmpDir,
		WorkDir:   tmpDir,
		Svc:       svc,
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/benchmark-comparison", nil)
	req.Header.Set("X-API-Key", "test-key")

	adapted := shared.Get(func(r *http.Request) (int, any) {
		return h.HandleBenchmarkComparison(r)
	})
	adapted.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestBenchmarkComparisonResponse_JSONSerialization(t *testing.T) {
	now := time.Now()
	resp := BenchmarkComparisonResponse{
		SnapshotTime:    now,
		SessionCount:    5,
		PortfolioReturn: 0.12,
		TAIEXReturn:     float64Ptr(0.08),
		Outperformance:  float64Ptr(0.04),
		Alpha:           float64Ptr(0.03),
		Beta:            float64Ptr(1.15),
		TrackingError:   float64Ptr(0.02),
		SharpeRatio:     float64Ptr(1.85),
		InfoRatio:       float64Ptr(2.0),
		EquityCurve: []BenchmarkPoint{
			{Label: "04/13", Portfolio: 100.0, Benchmark: 100.0, Outperf: 0.0},
			{Label: "04/14", Portfolio: 102.0, Benchmark: 101.0, Outperf: 1.0},
			{Label: "04/15", Portfolio: 105.0, Benchmark: 102.0, Outperf: 3.0},
		},
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	for _, key := range []string{
		"snapshot_time", "session_count", "portfolio_return", "taiex_return",
		"outperformance", "alpha", "beta", "tracking_error", "sharpe_ratio",
		"info_ratio", "equity_curve",
	} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("expected JSON key %q not found in response", key)
		}
	}

	if decoded["portfolio_return"] != 0.12 {
		t.Errorf("portfolio_return = %v, want 0.12", decoded["portfolio_return"])
	}
	if decoded["taiex_return"] != 0.08 {
		t.Errorf("taiex_return = %v, want 0.08", decoded["taiex_return"])
	}

	curve, ok := decoded["equity_curve"].([]any)
	if !ok {
		t.Fatal("equity_curve is not an array")
	}
	if len(curve) != 3 {
		t.Fatalf("len(equity_curve) = %d, want 3", len(curve))
	}

	point1 := curve[1].(map[string]any)
	for _, key := range []string{"label", "portfolio", "benchmark", "outperf"} {
		if _, exists := point1[key]; !exists {
			t.Errorf("key %q missing from equity_curve[1]", key)
		}
	}
}

func TestBuildBenchmarkEquityCurve(t *testing.T) {
	tests := []struct {
		name        string
		points      []sessionPoint
		taiexReturn *float64
		wantLen     int
		wantFirst   BenchmarkPoint
	}{
		{
			name:        "empty points",
			points:      nil,
			taiexReturn: float64Ptr(0.05),
			wantLen:     0,
		},
		{
			name: "single point",
			points: []sessionPoint{
				{date: time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC), name: "session-20260413-daily", value: 1000000},
			},
			taiexReturn: float64Ptr(0.05),
			wantLen:     1,
			wantFirst:   BenchmarkPoint{Label: "04/13", Portfolio: 100.0, Benchmark: 100.0, Outperf: 0.0},
		},
		{
			name: "three points with positive return",
			points: []sessionPoint{
				{date: time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC), name: "session-20260413-daily", value: 1000000},
				{date: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), name: "session-20260414-daily", value: 1020000},
				{date: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), name: "session-20260415-daily", value: 1050000},
			},
			taiexReturn: float64Ptr(0.05),
			wantLen:     3,
			wantFirst:   BenchmarkPoint{Label: "04/13", Portfolio: 100.0, Benchmark: 100.0, Outperf: 0.0},
		},
		{
			name: "missing taiex return",
			points: []sessionPoint{
				{date: time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC), name: "session-20260413-daily", value: 1000000},
				{date: time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC), name: "session-20260414-daily", value: 1020000},
			},
			taiexReturn: nil,
			wantLen:     2,
			wantFirst:   BenchmarkPoint{Label: "04/13", Portfolio: 100.0, Benchmark: 100.0, Outperf: 0.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBenchmarkEquityCurve(tt.points, tt.taiexReturn)
			if len(got) != tt.wantLen {
				t.Errorf("len(equityCurve) = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 {
				if got[0].Label != tt.wantFirst.Label ||
					got[0].Portfolio != tt.wantFirst.Portfolio ||
					got[0].Benchmark != tt.wantFirst.Benchmark ||
					got[0].Outperf != tt.wantFirst.Outperf {
					t.Errorf("first point = %+v, want %+v", got[0], tt.wantFirst)
				}
			}
		})
	}
}
