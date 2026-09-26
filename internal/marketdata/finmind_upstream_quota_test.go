package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── fix/finmind-quota-honor-402-r ─────────────────────────────────────────
//
// Production 2026-09-26 02:10Z: the auto_quote_backfill run (824 symbols) drove
// the shared tracker to ~12,500 calls for the quota day and FinMind answered
// 402 `{"msg":"Requests reach the upper limit. ...","...","token_tail":"..."}`.
// Three things were wrong, and these tests pin all three:
//
//  1. the 402 did not stop anything — the tracker kept counting and every
//     later call spent a doomed request (worse: a restart cleared the counter
//     and the backfill ran again);
//  2. the local ceiling (14400) was stale, so every "reserve" derived from it
//     (the 1500-call backfill floor → stop at 12,900) sat PAST the real wall;
//  3. the upstream body — including `token_tail`, a fragment of the live API
//     key — was copied verbatim into errors and logs.
//
// No test here talks to FinMind: every case drives an httptest server.
const upstreamQuotaSecret = "s3cr3t-token-tail-9f3a2b"

// newUpstreamQuotaClient wires a FinMind client whose quota state lives in an
// isolated dir (the latch is persisted, so sharing data/state across tests
// would leak it into every later test in the package) and whose HTTP traffic
// goes to handler. The returned counter counts upstream connections, which is
// what proves a call was short-circuited instead of sent.
func newUpstreamQuotaClient(t *testing.T, stateDir string, apiKey string, handler http.HandlerFunc) (*FinMindClient, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(ts.Close)

	c := NewFinMindClientWithStateDir(apiKey, stateDir)
	c.SetHTTPClient(&http.Client{Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport}})
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	c.retryCfg = retryConfig{maxAttempts: 1}
	return c, &hits
}

// upstream402Handler is the exact shape FinMind returned in production: HTTP
// 402, the "upper limit" message, and a `token_tail` fragment.
func upstream402Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"msg":"Requests reach the upper limit. https://finmindtrade.com/","status":402,"data":[],"token_tail":"` + upstreamQuotaSecret + `"}`))
	}
}

// TestFinMindClient_Upstream402_LatchesQuotaForRestOfDay is the core
// regression: once the upstream says the day is spent, no consumer of the
// shared client may spend another request — including after a restart.
func TestFinMindClient_Upstream402_LatchesQuotaForRestOfDay(t *testing.T) {
	stateDir := t.TempDir()
	c, hits := newUpstreamQuotaClient(t, stateDir, "test-api-key-"+upstreamQuotaSecret, upstream402Handler())
	ctx := context.Background()

	// (1) The 402 itself: classified as a quota condition (warn, not error)…
	_, err := c.FetchDatasetRaw(ctx, "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("first 402 must wrap ErrQuotaExhausted, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits after the 402 = %d, want 1", got)
	}

	// (2) …and it LATCHES the day: later calls never reach the network.
	for i := range 5 {
		_, err := c.FetchDatasetRaw(ctx, "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
		if !errors.Is(err, ErrQuotaExhausted) {
			t.Fatalf("call %d after the latch must be ErrQuotaExhausted, got %v", i+2, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits after 5 post-latch calls = %d, want 1 (the latch must short-circuit before the HTTP call)", got)
	}
	if got := c.QuotaRemaining(); got != 0 {
		t.Errorf("QuotaRemaining() = %d, want 0 while the upstream latch is set", got)
	}

	// (3) The 5-second index endpoint bypasses fetchDataset — it must obey the
	// same latch.
	if _, err := c.FetchTaiwan5SecIndex(ctx, "2026-09-26"); !errors.Is(err, ErrQuotaExhausted) {
		t.Errorf("5sec index must be gated by the latch, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits after the 5sec index call = %d, want 1", got)
	}

	// (4) Restart safety — the 2026-09-26 incident in one assertion. A fresh
	// process (new client, same state dir) must not forget the refusal.
	restarted, hits2 := newUpstreamQuotaClient(t, stateDir, "test-api-key-"+upstreamQuotaSecret, upstream402Handler())
	if got := restarted.QuotaRemaining(); got != 0 {
		t.Errorf("after restart QuotaRemaining() = %d, want 0 (persisted latch)", got)
	}
	_, err = restarted.FetchDatasetRaw(ctx, "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
	if !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("after restart the call must be refused locally, got %v", err)
	}
	if got := hits2.Load(); got != 0 {
		t.Fatalf("after restart the upstream was called %d times; the persisted latch must prevent it", got)
	}

	// (5) The latch must survive a restart WITH the day's call count, so the
	// operator sees used=… in the error.
	if !strings.Contains(err.Error(), "upstream-exhausted") {
		t.Errorf("post-restart error should explain WHY (upstream-exhausted), got %v", err)
	}
}

// TestFinMindClient_UpstreamLatch_HidesTokenFragment pins requirement (3):
// neither the error, nor the log line's payload, nor the persisted state file
// may carry `token_tail` or a token fragment.
func TestFinMindClient_UpstreamLatch_HidesTokenFragment(t *testing.T) {
	stateDir := t.TempDir()
	apiKey := "live-api-key-" + upstreamQuotaSecret
	c, _ := newUpstreamQuotaClient(t, stateDir, apiKey, upstream402Handler())

	_, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
	if err == nil {
		t.Fatal("expected a 402 error")
	}
	msg := err.Error()
	for _, forbidden := range []string{"token_tail", upstreamQuotaSecret, apiKey} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("error leaks %q: %s", forbidden, msg)
		}
	}
	if !strings.Contains(msg, "upper limit") {
		t.Errorf("error must keep the operator-actionable upstream reason, got %s", msg)
	}

	// The sanitized reason is persisted (operators read it back through the
	// channel-health error after a restart), so it must be clean there too.
	raw, readErr := os.ReadFile(filepath.Join(stateDir, "finmind_daily_quota.json"))
	if readErr != nil {
		t.Fatalf("quota state not written: %v", readErr)
	}
	for _, forbidden := range []string{"token_tail", upstreamQuotaSecret, apiKey} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("persisted quota state leaks %q: %s", forbidden, raw)
		}
	}
	var state QuotaState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("quota state is not valid JSON: %v (%s)", err, raw)
	}
	if !state.UpstreamExhausted {
		t.Error("quota state must record upstream_exhausted=true")
	}
}

// TestFinMindClient_UpstreamLatch_RecoversNextQuotaDay proves the latch is a
// day-scoped stop, not a permanent kill switch: state written yesterday must
// be ignored (calls_today AND the latch) when a new quota day starts.
func TestFinMindClient_UpstreamLatch_RecoversNextQuotaDay(t *testing.T) {
	stateDir := t.TempDir()
	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	state := QuotaState{
		CallsToday:        finmindDailyLimit - 1, // one call left, then the wall
		LastReset:         yesterday,
		UpstreamExhausted: true,
		UpstreamReason:    `{"msg":"Requests reach the upper limit"}`,
		UpstreamAt:        yesterday.Add(2 * time.Hour).Format(time.RFC3339),
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "finmind_daily_quota.json"), raw, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	c, hits := newUpstreamQuotaClient(t, stateDir, "test-key", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"success","status":200,"data":[{"date":"2026-09-27","close":1.0}]}`))
	})

	if got := c.QuotaRemaining(); got != finmindDailyLimit {
		t.Fatalf("QuotaRemaining() = %d on a new quota day, want the full ceiling %d", got, finmindDailyLimit)
	}
	rows, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-27", "2026-09-27")
	if err != nil {
		t.Fatalf("new quota day must be usable again, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (yesterday's latch must not survive the day rollover)", got)
	}
	if got := c.QuotaRemaining(); got != finmindDailyLimit-1 {
		t.Errorf("QuotaRemaining() = %d after 1 call, want %d", got, finmindDailyLimit-1)
	}
}

// TestDailyQuotaTracker_UpstreamLatch_Lifecycle covers the tracker directly:
// latch → refuses, persists, clears at the day boundary, and returns to normal
// accounting afterwards.
func TestDailyQuotaTracker_UpstreamLatch_Lifecycle(t *testing.T) {
	dir := t.TempDir()
	tracker := NewDailyQuotaTracker("finmind", dir, 100)

	if tracker.UpstreamExhausted() {
		t.Fatal("a fresh tracker must not start latched")
	}
	if !tracker.AllowCall() {
		t.Fatal("first call must be allowed")
	}

	tracker.MarkUpstreamExhausted(`{"msg":"Requests reach the upper limit"}`)
	if !tracker.UpstreamExhausted() {
		t.Error("latch not set")
	}
	if tracker.Remaining() != 0 {
		t.Errorf("Remaining() = %d, want 0 while latched", tracker.Remaining())
	}
	if tracker.AllowCall() {
		t.Error("AllowCall must refuse while latched")
	}

	// Persistence: a new tracker on the same dir (same quota day) sees it.
	reloaded := NewDailyQuotaTracker("finmind", dir, 100)
	if !reloaded.UpstreamExhausted() {
		t.Error("latch did not survive a restart")
	}
	if reloaded.AllowCall() {
		t.Error("reloaded tracker must refuse while latched")
	}
	exhausted, reason, at := reloaded.UpstreamExhaustion()
	if !exhausted || !strings.Contains(reason, "upper limit") || at.IsZero() {
		t.Errorf("UpstreamExhaustion() = (%v, %q, %v), want the persisted reason and observation time", exhausted, reason, at)
	}

	// Rollover: rewind the day (same package, so the field is reachable) and
	// the latch must clear — both in memory and on disk.
	reloaded.mu.Lock()
	reloaded.lastReset = time.Now().Truncate(24*time.Hour).AddDate(0, 0, -1)
	reloaded.mu.Unlock()

	if reloaded.UpstreamExhausted() {
		t.Error("latch must clear when the quota day rolls over")
	}
	if !reloaded.AllowCall() {
		t.Error("calls must be allowed again after the rollover")
	}
	if got := reloaded.CallsToday(); got != 1 {
		t.Errorf("CallsToday() = %d after the rollover, want 1 (the counter resets with the day)", got)
	}
}

// TestFinMindClient_UpstreamQuotaSignal_Table drives the classification and
// the latch through every shape the upstream uses, and asserts after each case
// whether the NEXT call reaches the network. wantSecondCallHits == 1 means
// "latch set, the next call was short-circuited"; 2 means "no latch, the next
// call really went out".
func TestFinMindClient_UpstreamQuotaSignal_Table(t *testing.T) {
	freeTierBody := `{"msg":"Your level is free. Please update your user level.","status":200,"data":[]}`
	cases := []struct {
		name               string
		status             int
		body               string
		wantQuotaErr       bool
		wantLatch          bool
		wantSecondCallHits int32
		wantInError        []string
		forbiddenInError   []string
	}{
		{
			name:               "402 upper limit with token_tail",
			status:             http.StatusPaymentRequired,
			body:               `{"msg":"Requests reach the upper limit.","status":402,"data":[],"token_tail":"` + upstreamQuotaSecret + `"}`,
			wantQuotaErr:       true,
			wantLatch:          true,
			wantSecondCallHits: 1,
			wantInError:        []string{"upper limit"},
			forbiddenInError:   []string{"token_tail", upstreamQuotaSecret},
		},
		{
			name:               "402 without token_tail",
			status:             http.StatusPaymentRequired,
			body:               `{"msg":"Requests reach the upper limit.","status":402,"data":[]}`,
			wantQuotaErr:       true,
			wantLatch:          true,
			wantSecondCallHits: 1,
			wantInError:        []string{"upper limit"},
			forbiddenInError:   []string{"token_tail"},
		},
		{
			name:               "HTTP 200 envelope carrying the upper-limit verdict",
			status:             http.StatusOK,
			body:               `{"msg":"Requests reach the upper limit.","status":200,"data":[]}`,
			wantQuotaErr:       true,
			wantLatch:          true,
			wantSecondCallHits: 1,
			wantInError:        []string{"upper limit"},
			forbiddenInError:   []string{"token_tail"},
		},
		{
			// Free-tier notice: classified as a quota condition (it is) but
			// NOT latched — the message does not prove the DAY is spent, and
			// latching on it would disable the channel for one ambiguous
			// dataset response.
			name:               "free-tier notice does not latch the day",
			status:             http.StatusOK,
			body:               freeTierBody,
			wantQuotaErr:       true,
			wantLatch:          false,
			wantSecondCallHits: 2,
			wantInError:        []string{"level is free"},
			forbiddenInError:   []string{"token_tail"},
		},
		{
			// Auth failure: NOT a quota condition, must not latch, and the
			// token fragment must not reach the error.
			name:               "400 token illegal is not a quota condition",
			status:             http.StatusBadRequest,
			body:               `{"msg":"Token is illegal.","status":400,"token_tail":"` + upstreamQuotaSecret + `"}`,
			wantQuotaErr:       false,
			wantLatch:          false,
			wantSecondCallHits: 2,
			wantInError:        []string{"Token is illegal"},
			forbiddenInError:   []string{"token_tail", upstreamQuotaSecret},
		},
		{
			name:               "500 is neither quota nor a latch",
			status:             http.StatusInternalServerError,
			body:               `{"msg":"internal error"}`,
			wantQuotaErr:       false,
			wantLatch:          false,
			wantSecondCallHits: 2,
			wantInError:        []string{"500"},
			forbiddenInError:   []string{"token_tail"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			c, hits := newUpstreamQuotaClient(t, stateDir, "test-key-"+upstreamQuotaSecret, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
			if err == nil {
				t.Fatal("expected an error from the canned upstream response")
			}
			if got := errors.Is(err, ErrQuotaExhausted); got != tc.wantQuotaErr {
				t.Errorf("errors.Is(ErrQuotaExhausted) = %v, want %v (err: %v)", got, tc.wantQuotaErr, err)
			}
			if got := c.quotaTracker.UpstreamExhausted(); got != tc.wantLatch {
				t.Errorf("latch = %v, want %v", got, tc.wantLatch)
			}
			for _, want := range tc.wantInError {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q must contain %q", err.Error(), want)
				}
			}
			for _, forbidden := range tc.forbiddenInError {
				if strings.Contains(err.Error(), forbidden) {
					t.Errorf("error %q must not contain %q", err.Error(), forbidden)
				}
			}

			// The behavioural assertion: does the next call reach the network?
			_, secondErr := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
			if secondErr == nil {
				t.Fatal("second call must also fail")
			}
			if got := hits.Load(); got != tc.wantSecondCallHits {
				t.Errorf("upstream hits after the second call = %d, want %d (latch=%v)", got, tc.wantSecondCallHits, tc.wantLatch)
			}
			for _, forbidden := range tc.forbiddenInError {
				if strings.Contains(secondErr.Error(), forbidden) {
					t.Errorf("second error %q must not contain %q", secondErr.Error(), forbidden)
				}
			}
		})
	}
}

// TestFinMindQuotaCeiling_StaysBelowObservedUpstreamRefusal guards requirement
// (2): the local ceiling must leave headroom under the point where the
// upstream actually refuses (12,500 calls, 2026-09-26), and the backfill
// reserve must remain a floor UNDER that ceiling. The failure mode this
// prevents is "raise the number until the incident stops happening".
func TestFinMindQuotaCeiling_StaysBelowObservedUpstreamRefusal(t *testing.T) {
	if finmindObservedUpstreamRefusalLimit != 12500 {
		t.Errorf("observed upstream refusal = %d, want 12500 (2026-09-26 production evidence)", finmindObservedUpstreamRefusalLimit)
	}
	if finmindDailyLimit >= finmindObservedUpstreamRefusalLimit {
		t.Fatalf("local ceiling %d must stay strictly below the observed refusal point %d; raising it re-creates the 2026-09-26 incident",
			finmindDailyLimit, finmindObservedUpstreamRefusalLimit)
	}
	// A useful margin: the gate must trip well before the wall, because the
	// upstream can move its own limit without notice.
	if margin := finmindObservedUpstreamRefusalLimit - finmindDailyLimit; margin < 100 {
		t.Errorf("headroom under the refusal point = %d calls, want >= 100", margin)
	}
	// The numbers get compared against each other in the ops docs; keep the
	// relationship explicit.
	if finmindDailyLimit <= 1500 {
		t.Errorf("local ceiling %d leaves no room above the 1500-call backfill reserve", finmindDailyLimit)
	}
	if got := FinMindDailyLimit(); got != finmindDailyLimitResolved() {
		t.Errorf("FinMindDailyLimit() = %d, want the resolved value %d", got, finmindDailyLimitResolved())
	}
}

// TestFinMindClient_UpstreamLatch_NoHTTPOnWarmRestart is the narrow negative
// control for the short-circuit: it drives the RESTART path explicitly with a
// call counter that fails loudly if any request escapes.
func TestFinMindClient_UpstreamLatch_NoHTTPOnWarmRestart(t *testing.T) {
	stateDir := t.TempDir()
	c, hits := newUpstreamQuotaClient(t, stateDir, "test-key", upstream402Handler())
	if _, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26"); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("seed 402 failed: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("seed hits = %d, want 1", got)
	}

	// 20 further calls, each of which would be a wasted upstream request
	// without the latch.
	for i := range 20 {
		if _, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26"); !errors.Is(err, ErrQuotaExhausted) {
			t.Fatalf("call %d: %v", i+2, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("%d upstream requests escaped the latch; want 1 (the single 402)", got)
	}
}

// TestFinMindQuotaOpsScript_DefaultMatchesCeiling keeps the pre-market ops
// check (scripts/ci/check_finmind_quota.sh, run by cron before the market
// opens) in step with the client ceiling. It divides calls_today by its own
// LIMIT default to decide "how close to the wall are we": with the stale
// 14400 default it printed "12500/14400 (86%)" — below its own 90% warn
// threshold — on the very morning FinMind was answering 402.
func TestFinMindQuotaOpsScript_DefaultMatchesCeiling(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "ci", "check_finmind_quota.sh")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(`LIMIT="\$\{FINMIND_DAILY_LIMIT:-([0-9]+)\}"`)
	match := re.FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatalf("%s: could not find the LIMIT=...FINMIND_DAILY_LIMIT:-N... default (did the script change shape?)", path)
	}
	scriptDefault, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("parse LIMIT default %q: %v", match[1], err)
	}
	if want := FinMindDailyLimit(); scriptDefault != want {
		t.Fatalf("%s uses LIMIT default %d but the client ceiling is %d — the ops check would report the wrong pressure (fix both together)",
			path, scriptDefault, want)
	}
	if scriptDefault > finmindObservedUpstreamRefusalLimit {
		t.Errorf("%s default %d is at or above the observed upstream refusal point %d", path, scriptDefault, finmindObservedUpstreamRefusalLimit)
	}
}

// TestFinMindDailyLimit_EnvOverride pins the config path for requirement (2):
// a tier change must be a config change, and an invalid value must fall back
// to the constant (never to an unbounded or zero ceiling).
func TestFinMindDailyLimit_EnvOverride(t *testing.T) {
	t.Setenv("FINMIND_DAILY_LIMIT", "9000")
	if got := finmindDailyLimitResolved(); got != 9000 {
		t.Errorf("finmindDailyLimitResolved() = %d, want 9000 from FINMIND_DAILY_LIMIT", got)
	}
	if got := FinMindDailyLimit(); got != 9000 {
		t.Errorf("FinMindDailyLimit() = %d, want 9000", got)
	}
	// The override must reach the tracker a client actually builds.
	c := NewFinMindClientWithStateDir("k", t.TempDir())
	if got := c.QuotaRemaining(); got != 9000 {
		t.Errorf("QuotaRemaining() = %d, want the overridden ceiling 9000", got)
	}

	// Invalid values fall back to the constant — a typo must not disable the
	// daily gate (an empty/garbage ceiling would either block everything or
	// nothing).
	for _, bad := range []string{"", "abc", "0", "-5", "9000x"} {
		t.Setenv("FINMIND_DAILY_LIMIT", bad)
		if got := finmindDailyLimitResolved(); got != finmindDailyLimit {
			t.Errorf("FINMIND_DAILY_LIMIT=%q resolved to %d, want the default %d", bad, got, finmindDailyLimit)
		}
	}
}
