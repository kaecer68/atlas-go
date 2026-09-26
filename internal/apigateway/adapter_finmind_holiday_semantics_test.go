package apigateway

// Issue #1999 regression suite — the finmind channel's non-trading-day /
// "target session not published yet" semantics, proven end to end through
// Gateway.Fetch (the production path: cmd/atlas registers
// channel_health_finmind as a 1h gatewayChannelFetch task).
//
// Production evidence being reproduced (snapshot taken 2026-09-26T01:01Z;
// the live counters kept climbing afterwards — channel_health
// consecutive_failures reached 4 and task_liveness channel_health_finmind 13):
// at 2026-09-26T01:01Z (= Saturday 09:01 Taipei) channel_health showed
//
//	finmind | error | consecutive_failures=2
//	last_error: "finmind fetch: finmind: no price data for 2330 on 2026-09-25: no data for symbol"
//
// 2026-09-25 was 中秋節 (a 休市日 — asserted against internal/taiwanholidays
// below), so the probe date itself was the bug. Three directions are pinned:
//
//	休市日 / 交易日盤前 (calendar-explained, and unreachable for the probe date)
//	        → "ok" with no error at all: the 1h probe task succeeds, so neither
//	          ChannelHealthStatusError nor the background_task alert fires.
//	a trading day with no row (NOT calendar-explained)
//	        → "warn": visible on the channel page, never a page, streak untouched.
//	a real upstream fault (HTTP 5xx / auth / schema)
//	        → "error" as before.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

const (
	finmindRowJSON   = `{"msg":"success","status":200,"data":[{"close":600.0,"open":595.0,"max":605.0,"min":590.0,"Trading_Volume":10000.0}]}`
	finmindEmptyJSON = `{"msg":"success","status":200,"data":[]}`
)

// finmindFakeUpstream is a fake api.finmindtrade.com that records the
// start_date of every request, so a test can prove WHICH day the probe asked
// for (the whole point of #1999).
type finmindFakeUpstream struct {
	server *httptest.Server

	mu      sync.Mutex
	dates   []string
	symbols []string
}

func newFinmindFakeUpstream(t *testing.T, respond func(w http.ResponseWriter, date string)) *finmindFakeUpstream {
	t.Helper()
	u := &finmindFakeUpstream{}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("start_date")
		symbol := r.URL.Query().Get("data_id")
		u.mu.Lock()
		u.dates = append(u.dates, date)
		u.symbols = append(u.symbols, symbol)
		u.mu.Unlock()
		respond(w, date)
	}))
	t.Cleanup(u.server.Close)
	return u
}

func (u *finmindFakeUpstream) requestedDates() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.dates...)
}

func (u *finmindFakeUpstream) requestedSymbols() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.symbols...)
}

func respondFinMindRow(w http.ResponseWriter, _ string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(finmindRowJSON))
}

// newFinmindAdapter builds the real finmind channel adapter against the fake
// upstream, with the probe clock frozen at now.
func newFinmindAdapter(t *testing.T, now time.Time, u *finmindFakeUpstream) *FinMindChannelAdapter {
	t.Helper()
	writeParametersJSON(t, nil)
	marketdata.ResetSharedFinMindClient()
	client := marketdata.NewFinMindClient("test-key")
	client.SetHTTPClient(withClientMockTransport(u.server, "api.finmindtrade.com"))

	adapter := NewFinMindChannelAdapter(client)
	adapter.now = func() time.Time { return now }
	return adapter
}

// newFinmindChannelGateway wires that adapter into a fresh Gateway on the
// "finmind" channel — the same registry slot production uses — so the tests
// exercise Gateway.Fetch (cache, breaker, health recording) end to end.
func newFinmindChannelGateway(t *testing.T, now time.Time, u *finmindFakeUpstream) *Gateway {
	t.Helper()
	g := newTestGateway(t)
	g.registry.Register("finmind", newFinmindAdapter(t, now, u))
	return g
}

// logFinmindRecord prints the channel_health record the acceptance criteria ask
// for (status / consecutive_failures / last_error) and returns it. Visible with
// `go test ./internal/apigateway/ -run TestFinMindChannel -v`.
func logFinmindRecord(t *testing.T, g *Gateway, label string) *ChannelHealthRecord {
	t.Helper()
	rec := g.Health().Get("finmind")
	if rec == nil {
		t.Fatalf("%s: expected a finmind health record", label)
	}
	t.Logf("%s: channel_health = status=%q consecutive_failures=%d last_error=%q last_success_at=%q",
		label, rec.Status, rec.ConsecutiveFailures, rec.LastError, rec.LastSuccessAt)
	return rec
}

func mustParseTaipei(t *testing.T, ts string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Fatalf("bad fixture %q: %v", ts, err)
	}
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Taipei): %v", err)
	}
	return parsed.In(loc)
}

// requireFixtureCalendar asserts the calendar facts the fixtures rely on, so a
// drift in internal/taiwanholidays fails loudly instead of silently weakening
// the suite.
func requireFixtureCalendar(t *testing.T) {
	t.Helper()
	nonTrading := []string{"2026-09-25", "2026-09-26", "2026-09-27"}
	for _, day := range nonTrading {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			t.Fatalf("bad fixture %q: %v", day, err)
		}
		if taiwanholidays.IsTradingDay(d) {
			t.Fatalf("fixture requires %s to be a non-trading day (#1999 production evidence)", day)
		}
	}
	d, _ := time.Parse("2006-01-02", "2026-09-28")
	if !taiwanholidays.IsTradingDay(d) {
		t.Fatal("fixture requires 2026-09-28 to be a trading day (pre-market case)")
	}
}

// TestFinMindChannel_HolidayProbe_NotError is the #1999 positive control: a
// probe issued on a 休市日 (the production 2026-09-26T01:01Z sample) asks for
// the most recent TRADING day, gets a row, and the channel stays "ok".
func TestFinMindChannel_HolidayProbe_NotError(t *testing.T) {
	requireFixtureCalendar(t)
	// 2026-09-26 = Saturday; the production probe ran at 01:01Z = 09:01 Taipei.
	now := mustParseTaipei(t, "2026-09-26T09:01:00+08:00")
	u := newFinmindFakeUpstream(t, respondFinMindRow)
	g := newFinmindChannelGateway(t, now, u)

	res, err := g.Fetch(context.Background(), "finmind")
	if err != nil {
		t.Fatalf("休市日 probe must not fail, got %v", err)
	}
	if res == nil || len(res.Data) == 0 {
		t.Fatal("expected a quote payload for the most recent trading day")
	}

	dates := u.requestedDates()
	if len(dates) != 1 {
		t.Fatalf("upstream requests = %d, want exactly 1 (%v)", len(dates), dates)
	}
	if dates[0] != "2026-09-24" {
		t.Errorf("probe date = %s, want 2026-09-24 (the most recent trading day)", dates[0])
	}
	if dates[0] == "2026-09-25" {
		t.Error("probe asked for 2026-09-25 — the 休市日 that produced the #1999 false error")
	}
	if symbols := u.requestedSymbols(); len(symbols) != 1 || symbols[0] != "2330" {
		t.Errorf("probe symbols = %v, want [2330]", symbols)
	}

	rec := logFinmindRecord(t, g, "休市日 probe (2026-09-26 Sat 09:01 Taipei)")
	if rec.Status != StatusOK {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
	if rec.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", rec.ConsecutiveFailures)
	}
	if rec.LastError != "" {
		t.Errorf("LastError = %q, want empty", rec.LastError)
	}
	if rec.LastSuccessAt == "" {
		t.Error("LastSuccessAt should advance: real data for a real trading day did land")
	}
}

// TestFinMindChannel_TradingDayPreMarketProbe_NotError covers the second
// acceptance scenario: the probe runs 盤前 on a TRADING day, so the current
// session's data does not exist yet. The probe must still land on the previous
// trading day and stay "ok".
func TestFinMindChannel_TradingDayPreMarketProbe_NotError(t *testing.T) {
	requireFixtureCalendar(t)
	// 2026-09-28 is a Monday trading day; 09:01 Taipei is before the open.
	now := mustParseTaipei(t, "2026-09-28T09:01:00+08:00")
	if !taiwanholidays.IsTradingDay(now) {
		t.Fatal("fixture requires 2026-09-28 09:01 Taipei to be a trading day")
	}
	u := newFinmindFakeUpstream(t, respondFinMindRow)
	g := newFinmindChannelGateway(t, now, u)

	if _, err := g.Fetch(context.Background(), "finmind"); err != nil {
		t.Fatalf("盤前 probe must not fail, got %v", err)
	}

	dates := u.requestedDates()
	if len(dates) != 1 || dates[0] != "2026-09-24" {
		t.Fatalf("probe dates = %v, want [2026-09-24] (previous trading day; 09-25 is a holiday)", dates)
	}
	rec := logFinmindRecord(t, g, "交易日盤前 probe (2026-09-28 Mon 09:01 Taipei)")
	if rec.Status != StatusOK {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
	if rec.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", rec.ConsecutiveFailures)
	}
}

// TestFinMindChannel_NoRowForTargetDay_RecordsWarn covers the residual
// "empty dataset for a TRADING day" case — FinMind answers HTTP 200 with a
// valid envelope and no row. The adapter re-classifies ErrNoDataForSymbol as
// marketdata.ErrEmptyQuote (NOT ErrNoData): the probe date is a trading day by
// construction, so this is never calendar-explained, and a daily channel must
// not read "ok" while it is dark. Gateway.Fetch then records WARN — reason
// visible, no page, consecutive_failures untouched, LastSuccessAt frozen.
//
// The error is still returned to the caller on purpose: the 1h
// channel_health_finmind task is the only signal that survives a long outage
// for this channel (there is no freshness backstop — see the contract comment).
func TestFinMindChannel_NoRowForTargetDay_RecordsWarn(t *testing.T) {
	requireFixtureCalendar(t)
	now := mustParseTaipei(t, "2026-09-26T09:01:00+08:00")
	u := newFinmindFakeUpstream(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(finmindEmptyJSON))
	})
	g := newFinmindChannelGateway(t, now, u)

	// Two consecutive probes: the production failure was exactly "2 consecutive
	// failures escalated to error".
	for i := range 2 {
		_, err := g.Fetch(context.Background(), "finmind")
		if err == nil {
			t.Fatalf("probe %d: Fetch must still surface the empty-dataset error to callers", i+1)
		}
		if !errors.Is(err, marketdata.ErrEmptyQuote) {
			t.Fatalf("probe %d: err = %v, want wrapped marketdata.ErrEmptyQuote", i+1, err)
		}
		if errors.Is(err, marketdata.ErrNoData) {
			t.Errorf("probe %d: a trading day with no row must NOT classify as ErrNoData (that would record ok)", i+1)
		}
	}

	rec := logFinmindRecord(t, g, "交易日的目標日無資料列 (2 probes)")
	if rec.Status != StatusWarn {
		t.Errorf("Status = %q, want warn (empty for a trading day must stay visible, but must not page)", rec.Status)
	}
	if rec.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after 2 empty probes", rec.ConsecutiveFailures)
	}
	if rec.LastError == "" {
		t.Error("LastError should keep the reason on the channel page")
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("LastSuccessAt = %q, want empty (no data landed → freshness anchor must not move)", rec.LastSuccessAt)
	}
}

// TestFinMindChannel_RealFailure_StillError is the negative control: the #1999
// fix must not blanket-suppress errors on holidays. An upstream outage (HTTP
// 500 here; 401/403/schema drift take the same path) still escalates to
// "error" after the contract's GraceFailures (default 2).
func TestFinMindChannel_RealFailure_StillError(t *testing.T) {
	requireFixtureCalendar(t)
	now := mustParseTaipei(t, "2026-09-26T09:01:00+08:00")
	u := newFinmindFakeUpstream(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	g := newFinmindChannelGateway(t, now, u)

	for i := range 2 {
		_, err := g.Fetch(context.Background(), "finmind")
		if err == nil {
			t.Fatalf("probe %d: expected an error from a 500 upstream", i+1)
		}
		if errors.Is(err, marketdata.ErrNoData) {
			t.Fatalf("probe %d: a 500 must NOT classify as ErrNoData, got %v", i+1, err)
		}
		if !strings.Contains(err.Error(), "finmind fetch:") {
			t.Errorf("probe %d: err = %v, want the ordinary 'finmind fetch:' failure path", i+1, err)
		}
	}

	rec := logFinmindRecord(t, g, "真故障 (HTTP 500, 2 probes)")
	if rec.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", rec.ConsecutiveFailures)
	}
	if rec.Status != StatusError {
		t.Errorf("Status = %q, want error (a real outage must still page)", rec.Status)
	}
	if rec.LastError == "" {
		t.Error("LastError should carry the upstream failure reason")
	}
}

// TestFinMindChannel_HealthCheckNoRowIsWarn pins the same semantics on the
// second entry point (UnifiedHealthStore.CheckHealth — currently without a
// production caller, kept correct anyway). That function overwrites any
// non-nil error with Status{Status:"error"}, so returning (warn, nil) — not
// (warn, err) — is load-bearing here.
func TestFinMindChannel_HealthCheckNoRowIsWarn(t *testing.T) {
	requireFixtureCalendar(t)
	now := mustParseTaipei(t, "2026-09-26T09:01:00+08:00")
	u := newFinmindFakeUpstream(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(finmindEmptyJSON))
	})
	adapter := newFinmindAdapter(t, now, u)

	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck must not surface 'no row' as an error, got %v", err)
	}
	if status.Status != StatusWarn {
		t.Errorf("Status = %q, want warn", status.Status)
	}
	if status.LastError == "" {
		t.Error("LastError should carry the reason")
	}
	if dates := u.requestedDates(); len(dates) != 1 || dates[0] != "2026-09-24" {
		t.Errorf("probe dates = %v, want [2026-09-24]", dates)
	}
}

// TestFinMindChannel_HealthCheckRealFailureStillError is the negative control
// for HealthCheck.
func TestFinMindChannel_HealthCheckRealFailureStillError(t *testing.T) {
	now := mustParseTaipei(t, "2026-09-26T09:01:00+08:00")
	u := newFinmindFakeUpstream(t, func(w http.ResponseWriter, _ string) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	adapter := newFinmindAdapter(t, now, u)

	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("a 500 upstream must surface an error")
	}
	if status.Status != StatusError {
		t.Errorf("Status = %q, want error", status.Status)
	}
}

// TestFinmindProbeErr_Classification pins the failure classifier directly: a
// future refactor must not widen the "empty dataset" verdict into real faults,
// narrow it back into the #1999 false positive (error), or collapse it into
// ErrNoData (which would report a daily channel as "ok" while it is dark).
func TestFinmindProbeErr_Classification(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantEmpty   bool
		wantPrefix  string
		wantWrapped error
	}{
		{
			name:        "per-symbol no row → ErrEmptyQuote (warn, never error, never ok)",
			err:         fmt.Errorf("finmind: no price data for 2330 on 2026-09-24: %w", marketdata.ErrNoDataForSymbol),
			wantEmpty:   true,
			wantPrefix:  "finmind: no price row for 2330 on 2026-09-24",
			wantWrapped: marketdata.ErrEmptyQuote,
		},
		{
			name:        "a bare ErrNoData is not passed through as ErrNoData either",
			err:         fmt.Errorf("finmind: %w", marketdata.ErrNoData),
			wantEmpty:   true,
			wantPrefix:  "finmind: no price row for 2330 on 2026-09-24",
			wantWrapped: marketdata.ErrEmptyQuote,
		},
		{
			name:       "HTTP 5xx keeps the ordinary failure path",
			err:        errors.New("finmind: http request: http status 500 after 3 attempts"),
			wantPrefix: "finmind fetch: ",
		},
		{
			name:        "quota exhaustion is NOT re-classified as empty",
			err:         fmt.Errorf("finmind: %w (used=14400, remaining=0)", marketdata.ErrQuotaExhausted),
			wantPrefix:  "finmind fetch: ",
			wantWrapped: marketdata.ErrQuotaExhausted,
		},
		{
			name:        "IP ban is NOT re-classified as empty",
			err:         fmt.Errorf("finmind: %w (unblocks in 971s)", marketdata.ErrIPBanned),
			wantPrefix:  "finmind fetch: ",
			wantWrapped: marketdata.ErrIPBanned,
		},
		{
			name:        "open client breaker keeps the ordinary failure path",
			err:         fmt.Errorf("finmind: %w", marketdata.ErrFinMindBreakerOpen),
			wantPrefix:  "finmind fetch: ",
			wantWrapped: marketdata.ErrFinMindBreakerOpen,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := finmindProbeErr(tc.err, "2330", "2026-09-24")
			if errors.Is(got, marketdata.ErrEmptyQuote) != tc.wantEmpty {
				t.Errorf("errors.Is(err, ErrEmptyQuote) = %v, want %v (err = %v)",
					errors.Is(got, marketdata.ErrEmptyQuote), tc.wantEmpty, got)
			}
			if errors.Is(got, marketdata.ErrNoData) {
				t.Errorf("err = %v: must never classify as ErrNoData (that records the channel as ok)", got)
			}
			if !strings.HasPrefix(got.Error(), tc.wantPrefix) {
				t.Errorf("err = %q, want prefix %q", got.Error(), tc.wantPrefix)
			}
			if tc.wantWrapped != nil && !errors.Is(got, tc.wantWrapped) {
				t.Errorf("err = %v, want it to still wrap %v", got, tc.wantWrapped)
			}
		})
	}
}
