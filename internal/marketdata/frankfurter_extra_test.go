package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Frankfurter returns ECB-published exchange rates.
// Reference: USD/JPY on 2026-04-29 = 156.42, prior business day 154.18.
// Day-over-day change: (156.42 - 154.18) / 154.18 * 100 = +1.453...%

func TestFrankfurterFXProvider_Name(t *testing.T) {
	if got := NewFrankfurterFXProvider().Name(); got != "frankfurter_fx" {
		t.Errorf("Name() = %q, want frankfurter_fx", got)
	}
}

func TestFrankfurterFXProvider_FetchSnapshot_Success(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/latest" || strings.Contains(r.URL.Path, "/latest"):
			w.Write([]byte(`{"date":"2026-04-29","base":"USD","rates":{"JPY":156.42}}`))
		default:
			// Historical date endpoint
			w.Write([]byte(`{"date":"2026-04-28","base":"USD","rates":{"JPY":154.18}}`))
		}
	}))
	defer ts.Close()

	// Redirect provider to test server via direct field override.
	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL
	p.latestURL = ts.URL + "/latest?from=USD&to=JPY"

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot error: %v", err)
	}
	if snap.JPY.Symbol != "JPY=X" {
		t.Errorf("JPY.Symbol = %q, want JPY=X", snap.JPY.Symbol)
	}
	if snap.JPY.Value != 156.42 {
		t.Errorf("JPY.Value = %v, want 156.42", snap.JPY.Value)
	}
	// (156.42 - 154.18) / 154.18 * 100 ≈ 1.453
	expectedPct := (156.42 - 154.18) / 154.18 * 100
	if diff := snap.JPY.ChangePct - expectedPct; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("JPY.ChangePct = %v, want %v (diff %v)", snap.JPY.ChangePct, expectedPct, diff)
	}
	if snap.JPY.Timestamp == 0 {
		t.Error("JPY.Timestamp should be populated")
	}
	if callCount < 1 {
		t.Errorf("expected at least 1 HTTP call, got %d", callCount)
	}
}

func TestFrankfurterFXProvider_FetchSnapshot_ZeroRate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"date":"2026-04-29","base":"USD","rates":{"JPY":0.0}}`))
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL
	p.latestURL = ts.URL + "/latest?from=USD&to=JPY"

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot should not error on zero rate (uses warning log), got %v", err)
	}
	if snap.JPY.Value != 0 {
		t.Errorf("JPY.Value = %v, want 0", snap.JPY.Value)
	}
	if snap.JPY.Symbol != "" {
		t.Errorf("JPY.Symbol should be empty for zero rate, got %q", snap.JPY.Symbol)
	}
}

func TestFrankfurterFXProvider_FetchSnapshot_NoPreviousRate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 500 for all endpoints to simulate outage
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL
	p.latestURL = ts.URL + "/latest?from=USD&to=JPY"

	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error when latest endpoint fails")
	}
}

func TestFrankfurterFXProvider_fetchRate_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL
	_, err := p.fetchRate(context.Background(), ts.URL+"/latest")
	if err == nil {
		t.Fatal("expected error for 502 response")
	}
}

func TestFrankfurterFXProvider_fetchRate_NoJPYKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"date":"2026-04-29","base":"USD","rates":{"EUR":0.92}}`))
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL
	_, err := p.fetchRate(context.Background(), ts.URL+"/latest")
	if err == nil {
		t.Fatal("expected error when JPY missing from rates")
	}
	if !strings.Contains(err.Error(), "JPY rate missing") {
		t.Errorf("error %q should mention JPY rate missing", err.Error())
	}
}

func TestFrankfurterFXProvider_fetchRate_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not-json`))
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL
	_, err := p.fetchRate(context.Background(), ts.URL+"/latest")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestPreviousBusinessDay(t *testing.T) {
	// 2026-04-30 is a Thursday
	thursday := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		now      time.Time
		daysBack int
		wantYMD  string
	}{
		{"Thursday→Wednesday", thursday, 1, "2026-04-29"},
		{"Thursday→Friday prior week", thursday, 3, "2026-04-27"},
		// P1-8: PreviousTradingDay now skips Taiwan public holidays, not just
		// weekends. 2026-05-01 is 勞動節 (Labor Day) — a Taiwan holiday — so
		// Saturday 05-02 / Sunday 05-03 with daysBack=1 lands on Thursday
		// 04-30, not the holiday Friday 05-01.
		{"Saturday skips to Friday", time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC), 1, "2026-04-30"},
		{"Sunday skips to Friday", time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC), 1, "2026-04-30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PreviousTradingDay(tt.now, tt.daysBack)
			if got.Format("2006-01-02") != tt.wantYMD {
				t.Errorf("got %s, want %s", got.Format("2006-01-02"), tt.wantYMD)
			}
			if got.Weekday() == time.Saturday || got.Weekday() == time.Sunday {
				t.Errorf("returned day is weekend: %s", got.Weekday())
			}
		})
	}
}

func TestFrankfurterFXProvider_fetchPreviousBusinessDayRate_AllDifferent(t *testing.T) {
	// All 7 historical endpoints return the same rate as current (pegged currency),
	// so firstRate is returned.
	currentRate := 156.42
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"date":"2026-04-29","base":"USD","rates":{"JPY":156.42}}`))
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL

	rate, date, err := p.fetchPreviousBusinessDayRate(context.Background(), currentRate)
	if err != nil {
		t.Fatalf("fetchPreviousBusinessDayRate: %v", err)
	}
	if rate != 156.42 {
		t.Errorf("rate = %v, want 156.42", rate)
	}
	if date == "" {
		t.Error("date should be populated")
	}
}

func TestFrankfurterFXProvider_fetchPreviousBusinessDayRate_FindsDifferent(t *testing.T) {
	// 3rd historical endpoint returns different rate (simulating yesterday's actual price)
	currentRate := 156.42
	var callCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// First 2 endpoints match current, 3rd differs
		if callCount >= 3 {
			w.Write([]byte(`{"date":"2026-04-25","base":"USD","rates":{"JPY":154.18}}`))
		} else {
			w.Write([]byte(`{"date":"2026-04-28","base":"USD","rates":{"JPY":156.42}}`))
		}
	}))
	defer ts.Close()

	p := NewFrankfurterFXProvider()
	p.baseURL = ts.URL

	rate, date, err := p.fetchPreviousBusinessDayRate(context.Background(), currentRate)
	if err != nil {
		t.Fatalf("fetchPreviousBusinessDayRate: %v", err)
	}
	if rate != 154.18 {
		t.Errorf("rate = %v, want 154.18 (first different rate)", rate)
	}
	if date == "" {
		t.Error("date should be populated")
	}
}
