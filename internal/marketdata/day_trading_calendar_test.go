package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestRecentTradingDays_WeekendSkip covers the #1767 calendar scan: from a
// Sunday the expected trading days must be Fri/Thu/Wed — no weekend days.
func TestRecentTradingDays_WeekendSkip(t *testing.T) {
	sun := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC) // Sunday
	days := RecentTradingDays(sun, 3)
	want := []string{"2026-08-28", "2026-08-27", "2026-08-26"} // Fri, Thu, Wed
	for i, w := range want {
		if got := days[i].Format("2006-01-02"); got != w {
			t.Errorf("days[%d] = %v (%s), want %s", i, days[i], days[i].Weekday(), w)
		}
	}

	// Monday includes itself first, then skips the weekend.
	mon := time.Date(2026, time.August, 31, 1, 0, 0, 0, time.UTC)
	days = RecentTradingDays(mon, 3)
	if got := days[0].Format("2006-01-02"); got != "2026-08-31" {
		t.Errorf("days[0] = %v, want Monday 8/31", days[0])
	}
	if got := days[1].Format("2006-01-02"); got != "2026-08-28" {
		t.Errorf("days[1] = %v, want Friday 8/28", days[1])
	}
}

// TestDayTradingProvider_FetchLatest_BoundedAttemptsAndLastError — the scan
// must give up after maxAttempts (3) and surface the last underlying error.
func TestDayTradingProvider_FetchLatest_BoundedAttemptsAndLastError(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	p := NewDayTradingProvider()
	p.SetHTTPClient(srv.Client())
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p.SetBaseURL(srv.URL)

	_, err := p.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("FetchLatest err = nil, want error")
	}
	if !strings.Contains(err.Error(), "last error") {
		t.Errorf("err missing last-error detail: %v", err)
	}
	if got := hits.Load(); got > 3 {
		t.Errorf("attempted %d fetches, want <= 3", got)
	}
}
