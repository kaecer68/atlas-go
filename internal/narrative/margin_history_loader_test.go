package narrative

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestLoadMarginHistory(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"20260514_margin.json": `{"date":"20260514","margin_balance":5000,"change_pct":1.2}`,
		"20260513_margin.json": `{"date":"20260513","margin_balance":4900,"change_pct":-0.5}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}
	history, err := LoadMarginHistory(dir)
	if err != nil {
		t.Fatalf("LoadMarginHistory failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(history))
	}
	if history[0].Date != "20260513" || history[1].Date != "20260514" {
		t.Fatalf("expected sorted dates, got %+v", history)
	}
}

func TestComputeRollingPercentile(t *testing.T) {
	history := []MarginHistoryEntry{{MarginBalance: 1}, {MarginBalance: 2}, {MarginBalance: 3}, {MarginBalance: 4}, {MarginBalance: 5}}
	percentile, ok := ComputeRollingPercentile(history, 3, 5)
	if !ok {
		t.Fatalf("expected percentile to be available")
	}
	if percentile < 49 || percentile > 51 {
		t.Fatalf("expected percentile around 50, got %.2f", percentile)
	}
}

func TestComputeRollingAcceleration(t *testing.T) {
	history := []MarginHistoryEntry{{MarginBalance: 10}, {MarginBalance: 12}, {MarginBalance: 15}, {MarginBalance: 19}, {MarginBalance: 24}, {MarginBalance: 30}}
	accel, ok := ComputeRollingAcceleration(history, 5)
	if !ok {
		t.Fatalf("expected acceleration to be available")
	}
	if accel < 3.9 || accel > 4.1 {
		t.Fatalf("expected acceleration around 4, got %.2f", accel)
	}
}

func TestMarginHistoryInsufficientData(t *testing.T) {
	history := []MarginHistoryEntry{{MarginBalance: 1}, {MarginBalance: 2}}
	if _, ok := ComputeRollingPercentile(history, 2, 5); ok {
		t.Fatalf("expected percentile to be unavailable with sparse history")
	}
	if _, ok := ComputeRollingAcceleration(history, 5); ok {
		t.Fatalf("expected acceleration to be unavailable with sparse history")
	}
}

// marginMockTransport serves the provided TWSE response body and records
// requested dates. failFirst > 0 makes the first N requests fail with HTTP
// 500 so retry behavior can be exercised.
type marginMockTransport struct {
	mu        sync.Mutex
	body      []byte
	failFirst int
	requests  []string // requested "date|selectType"
}

func (m *marginMockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	date := r.URL.Query().Get("date")
	sel := r.URL.Query().Get("selectType")
	m.requests = append(m.requests, date+"|"+sel)
	if m.failFirst > 0 {
		m.failFirst--
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Body:       io.NopCloser(bytes.NewReader([]byte("boom"))),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewReader(m.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func (m *marginMockTransport) requestedDates() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.requests))
	copy(out, m.requests)
	return out
}

// newMarginMockBackfiller builds a MarginHistoryBackfiller whose provider
// talks to the mock transport, with an infinite rate limiter.
func newMarginMockBackfiller(t *testing.T, workDir string, mock *marginMockTransport) *MarginHistoryBackfiller {
	t.Helper()
	marginDir := filepath.Join(workDir, DefaultMarginHistoryDir)
	p := marketdata.NewTWSEMarginBalanceProvider(marginDir)
	p.SetHTTPClient(&http.Client{Transport: mock})
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 0))
	return &MarginHistoryBackfiller{
		WorkDir:      workDir,
		Provider:     p,
		LookbackDays: 30,
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return d
}

func listMarginDates(t *testing.T, workDir string) []string {
	t.Helper()
	dir := filepath.Join(workDir, DefaultMarginHistoryDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read margin dir: %v", err)
	}
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, "_margin.json") {
			continue
		}
		dates = append(dates, strings.TrimSuffix(name, "_margin.json"))
	}
	sort.Strings(dates)
	return dates
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return body
}

// TestMarginHistoryBackfiller_Parameterized exercises the Backfill window,
// existing-file skipping, weekend skipping, retry-on-failure and invalid
// window handling.
func TestMarginHistoryBackfiller_Parameterized(t *testing.T) {
	fixture := readFixture(t, filepath.Join("testdata", "twse_margin_ok.json"))

	tests := []struct {
		name        string
		start, end  string
		lookback    int
		seed        []string // existing files to pre-create (YYYYMMDD)
		failFirst   int
		wantDates   []string
		wantErr     bool
		wantRetries bool // expect date 20240701 requested more than once
	}{
		{
			name:      "full week window",
			start:     "2024-07-01",
			end:       "2024-07-05",
			wantDates: []string{"20240701", "20240702", "20240703", "20240704", "20240705"},
		},
		{
			name:      "weekend skipped",
			start:     "2024-07-05",
			end:       "2024-07-08",
			wantDates: []string{"20240705", "20240708"},
		},
		{
			name:      "existing files skipped",
			start:     "2024-07-01",
			end:       "2024-07-03",
			seed:      []string{"20240702"},
			wantDates: []string{"20240701", "20240702", "20240703"},
		},
		{
			name:        "retry after transient failure",
			start:       "2024-07-01",
			end:         "2024-07-01",
			failFirst:   7, // one full provider scan fails, then retry succeeds
			wantDates:   []string{"20240701"},
			wantRetries: true,
		},
		{
			name:      "retries exhausted",
			start:     "2024-07-01",
			end:       "2024-07-01",
			failFirst: 99,
			wantDates: []string{},
		},
		{
			name:    "start after end errors",
			start:   "2024-07-05",
			end:     "2024-07-01",
			wantErr: true,
		},
		{
			name:      "zero start uses lookback window",
			end:       "2024-07-05",
			lookback:  5,
			wantDates: []string{"20240701", "20240702", "20240703", "20240704", "20240705"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			// Pre-seed existing margin files.
			for _, d := range tc.seed {
				dir := filepath.Join(workDir, DefaultMarginHistoryDir)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				body, _ := json.Marshal(map[string]any{"date": d, "margin_balance": 1234.5})
				if err := os.WriteFile(filepath.Join(dir, d+"_margin.json"), body, 0o644); err != nil {
					t.Fatalf("seed %s: %v", d, err)
				}
			}

			mock := &marginMockTransport{body: fixture, failFirst: tc.failFirst}
			bf := newMarginMockBackfiller(t, workDir, mock)
			if tc.lookback > 0 {
				bf.LookbackDays = tc.lookback
			}
			if tc.start != "" {
				bf.StartDate = mustDate(t, tc.start)
			}
			if tc.end != "" {
				bf.EndDate = mustDate(t, tc.end)
			}

			err := bf.Backfill(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Backfill: %v", err)
			}

			got := listMarginDates(t, workDir)
			if !equalStrings(got, tc.wantDates) {
				t.Fatalf("margin dates = %v, want %v", got, tc.wantDates)
			}

			if tc.wantRetries {
				count := 0
				for _, r := range mock.requestedDates() {
					if r == "20240701|MS" {
						count++
					}
				}
				if count < 2 {
					t.Fatalf("expected date 20240701 requested multiple times, got %d", count)
				}
			}
		})
	}
}

// TestMarginHistoryBackfiller_Fixture verifies the on-disk file produced by
// Backfill matches the golden format (date, margin_balance, short_balance,
// change_pct, short_change_pct) when the provider serves a fixed fixture.
func TestMarginHistoryBackfiller_Fixture(t *testing.T) {
	fixture := readFixture(t, filepath.Join("testdata", "twse_margin_ok.json"))
	workDir := t.TempDir()
	mock := &marginMockTransport{body: fixture}
	bf := newMarginMockBackfiller(t, workDir, mock)
	bf.StartDate = mustDate(t, "2024-07-01")
	bf.EndDate = mustDate(t, "2024-07-01")

	if err := bf.Backfill(context.Background()); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	// 2024-07-01 is a Monday (trading day) — file must exist.
	path := filepath.Join(workDir, DefaultMarginHistoryDir, "20240701_margin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read margin file: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal margin file: %v", err)
	}

	if got["date"] != "20240701" {
		t.Errorf("date = %v, want 20240701", got["date"])
	}
	// Fixture 融資金額 今日餘額 120,000,000 仟元 / 1e5 = 1200.0
	if got["margin_balance"] != 1200.0 {
		t.Errorf("margin_balance = %v, want 1200.0", got["margin_balance"])
	}
	// Fixture 融券 今日餘額 239,205 / 1e5 = 2.39205
	if got["short_balance"] != 2.39205 {
		t.Errorf("short_balance = %v, want 2.39205", got["short_balance"])
	}
	// change_pct = (1200-1000)/1000*100 = 20
	if got["change_pct"] != 20.0 {
		t.Errorf("change_pct = %v, want 20.0", got["change_pct"])
	}
	if _, ok := got["short_change_pct"]; !ok {
		t.Errorf("short_change_pct missing from saved file")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
