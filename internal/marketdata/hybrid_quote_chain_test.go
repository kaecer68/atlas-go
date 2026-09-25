package marketdata

// hybrid_quote_chain_test.go — regression coverage for issue #1986.
//
// The production failure this file pins down (Mac Mini, 2026-09-25 07:10Z run,
// fubon-proxy access log): the fubon arm answered every chunk with HTTP 200, but
// a single symbol without a usable quote made the OLD chain discard all 50 and
// re-ask the next arms, which issue one HTTP request per symbol and are rate
// limited to 0.17-0.5 req/s. The measured chunk cadence in the proxy log is
// 4-6 s for a healthy chunk and 59-61 s for one that fell through, i.e. the
// fallback path consumed the whole 60 s chunk budget; FinMind's daily quota was
// exhausted (14,361 / 14,400) and two chunks failed outright.
//
// Every test here drives the REAL HybridProvider against httptest upstreams
// shaped like the real ones, so the assertions are about the shipped code path.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// chainSymbols returns n synthetic Taiwan-style codes.
func chainSymbols(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, fmt.Sprintf("%04d", 1101+i))
	}
	return out
}

// recordingUpstream is an httptest server that records every request.
type recordingUpstream struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string // request URI (path?query)
}

func newRecordingUpstream(t *testing.T, handler http.HandlerFunc) *recordingUpstream {
	t.Helper()
	u := &recordingUpstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.requests = append(u.requests, r.URL.RequestURI())
		u.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *recordingUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func (u *recordingUpstream) snapshot() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.requests...)
}

// fubonQuotesHandler answers the fubon-proxy /quotes contract.
//
// drop is the set of symbols the proxy does not publish (a suspended stock, a
// symbol the upstream SDK refused); unusable is the set it publishes with no
// usable price (Last/OHLC all zero).
func fubonQuotesHandler(drop, unusable map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query()["symbols"]
		symbols := make([]string, 0, len(raw))
		for _, group := range raw {
			for _, s := range strings.Split(group, ",") {
				if s = strings.TrimSpace(s); s != "" {
					symbols = append(symbols, s)
				}
			}
		}
		out := make([]FubonQuoteResponse, 0, len(symbols))
		for _, s := range symbols {
			if drop[s] {
				continue
			}
			resp := FubonQuoteResponse{
				Symbol: s, Last: 100, Open: 99, High: 101, Low: 98,
				Volume: 1_000_000, IsOpen: true, Source: "fubon",
			}
			if unusable[s] {
				resp.Last, resp.Open, resp.High, resp.Low, resp.Volume = 0, 0, 0, 0, 0
			}
			out = append(out, resp)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// finmindQuotesHandler answers api.finmindtrade.com/data for dataset
// TaiwanStockPrice, one row per data_id unless the symbol is in noData.
func finmindQuotesHandler(noData map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("data_id")
		data := []map[string]any{}
		if !noData[symbol] {
			data = append(data, map[string]any{
				"date": "2026-09-24", "stock_id": symbol,
				"open": 99.0, "max": 101.0, "min": 98.0, "close": 100.0,
				"Trading_Volume": 1_000_000.0,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": 200, "msg": "success", "data": data,
		})
	}
}

// twseDailyQuotesHandler answers the TWSE STOCK_DAY_ALL table.
func twseDailyQuotesHandler(published map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows := make([][]string, 0, len(published))
		for sym := range published {
			rows = append(rows, []string{
				sym, "TEST", "1000", "100000", "99.00", "101.00", "98.00", "100.00", "+1.00", "500",
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TWSEDailyResponse{Stat: "OK", Data: rows})
	}
}

// newFubonTestProvider builds a FubonProvider pointed at srv.
func newFubonTestProvider(t *testing.T, srv *httptest.Server) *FubonProvider {
	t.Helper()
	client := &FubonClient{
		proxyURL:        srv.URL,
		httpClient:      srv.Client(),
		intradayLimiter: rate.NewLimiter(rate.Inf, 1),
		breaker:         newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
	}
	client.healthy.Store(true)
	return NewFubonProviderWithClient(client)
}

// newFinMindTestProvider builds a FinMindProvider pointed at srv.
func newFinMindTestProvider(t *testing.T, srv *httptest.Server) *FinMindProvider {
	t.Helper()
	client := NewFinMindClient("test-key")
	client.SetBaseURL(srv.URL)
	client.SetHTTPClient(srv.Client())
	client.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	client.SetQuotaLimit(1_000_000)
	return NewFinMindProviderWithClient(client)
}

// newTWSETestProvider builds a TWSEClient pointed at srv.
func newTWSETestProvider(t *testing.T, srv *httptest.Server) *TWSEClient {
	t.Helper()
	client := &TWSEClient{
		baseURL:     srv.URL,
		httpClient:  srv.Client(),
		rateLimiter: rate.NewLimiter(rate.Inf, 1),
		breaker:     newProviderBreaker("twse", defaultCircuitBreakerConfig()),
		retryCfg:    retryConfig{maxAttempts: 1},
	}
	return client
}

// tradingAsOf is a date the FinMind arm accepts (isTaiwanTradingDay).
func tradingAsOf(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", "2026-09-24")
	if err != nil {
		t.Fatalf("parse asOf: %v", err)
	}
	if !isTaiwanTradingDay(d) {
		t.Fatalf("test fixture assumes 2026-09-24 is a trading day")
	}
	return d
}

// ── Requirement 1: only the missing symbols are re-fetched ──────────────────

// TestHybridQuoteChain_PartialArmRetriesOnlyMissingSymbols is the core
// regression test: one symbol the fubon arm does not publish must cost exactly
// ONE per-symbol fallback request, not fifty.
func TestHybridQuoteChain_PartialArmRetriesOnlyMissingSymbols(t *testing.T) {
	symbols := chainSymbols(50)
	missing := symbols[7]

	fubon := newRecordingUpstream(t, fubonQuotesHandler(map[string]bool{missing: true}, nil))
	finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
	twse := newRecordingUpstream(t, twseDailyQuotesHandler(nil))

	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}

	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if len(batch.Quotes) != len(symbols) {
		t.Fatalf("resolved %d quotes, want %d (the one missing symbol must be filled from the next arm)",
			len(batch.Quotes), len(symbols))
	}
	if got := batch.Outcomes[missing]; got != QuoteOutcomeOK {
		t.Errorf("outcome for %s = %q, want ok", missing, got)
	}

	if n := fubon.count(); n != 1 {
		t.Errorf("fubon calls = %d, want 1", n)
	}
	finmindReqs := finmind.snapshot()
	if len(finmindReqs) != 1 {
		t.Fatalf("FinMind requests = %d (%v), want 1: only the missing symbol may be re-fetched",
			len(finmindReqs), finmindReqs)
	}
	if !strings.Contains(finmindReqs[0], "data_id="+missing) {
		t.Errorf("FinMind was asked for %q, want only %s", finmindReqs[0], missing)
	}
	if n := twse.count(); n != 0 {
		t.Errorf("TWSE calls = %d, want 0: the batch was already complete", n)
	}
}

// TestHybridQuoteChain_UnusableQuoteCostsOneFallback pins the same property for
// the other shape of "one bad symbol": the arm publishes it with all-zero
// prices, which the old chain also treated as a whole-batch failure.
func TestHybridQuoteChain_UnusableQuoteCostsOneFallback(t *testing.T) {
	symbols := chainSymbols(20)
	bad := symbols[3]

	fubon := newRecordingUpstream(t, fubonQuotesHandler(nil, map[string]bool{bad: true}))
	finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
	twse := newRecordingUpstream(t, twseDailyQuotesHandler(nil))

	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}

	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if len(batch.Quotes) != len(symbols) {
		t.Fatalf("resolved %d quotes, want %d", len(batch.Quotes), len(symbols))
	}
	if got := len(finmind.snapshot()); got != 1 {
		t.Fatalf("FinMind requests = %d, want 1 (only the unusable symbol)", got)
	}
}

// TestHybridQuoteChain_PartialAnswerDoesNotTripThePrimaryBreaker pins the second
// half of the production damage: the old chain called recordFailure whenever the
// batch was not 100 % usable, so three chunks with one bad symbol each opened the
// fubon breaker for five minutes and the remaining chunks never asked fubon at
// all. The proxy log shows exactly that (28 chunk calls for 32 chunks).
func TestHybridQuoteChain_PartialAnswerDoesNotTripThePrimaryBreaker(t *testing.T) {
	symbols := chainSymbols(20)
	bad := symbols[5]

	fubon := newRecordingUpstream(t, fubonQuotesHandler(nil, map[string]bool{bad: true}))
	finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
	twse := newRecordingUpstream(t, twseDailyQuotesHandler(nil))

	fubonBreaker := newProviderBreaker("fubon", defaultCircuitBreakerConfig())
	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": fubonBreaker,
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}

	for round := range 6 {
		if _, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	if got := fubon.count(); got != 6 {
		t.Errorf("fubon calls = %d, want 6: a partial answer must not disable the primary arm", got)
	}
	if state := fubonBreaker.stateSnapshot().State; state != ProviderCircuitClosed {
		t.Errorf("fubon breaker = %s, want closed (threshold %d)", state, defaultCircuitBreakerConfig().failureThreshold)
	}
}

// ── Requirement 2: bounded worst case ───────────────────────────────────────

// TestHybridQuoteChain_SkipsPerSymbolArmOnLargeResidual pins the budget guard:
// when the primary arm yields nothing, the residual is the whole chunk and the
// per-symbol arm must NOT be asked for it (that is what ate the 60 s chunk
// budget and the FinMind daily quota in production).
func TestHybridQuoteChain_SkipsPerSymbolArmOnLargeResidual(t *testing.T) {
	symbols := chainSymbols(50)
	published := make(map[string]bool)
	for _, s := range symbols[:30] {
		published[s] = true
	}

	fubon := newRecordingUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "proxy down", http.StatusBadGateway)
	})
	finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
	twse := newRecordingUpstream(t, twseDailyQuotesHandler(published))

	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}

	start := time.Now()
	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if n := finmind.count(); n != 0 {
		t.Fatalf("FinMind requests = %d, want 0: a 50-symbol residual must not be fanned out per symbol", n)
	}
	if len(batch.Quotes) != 30 {
		t.Fatalf("resolved %d quotes, want the 30 the whole-market arm publishes", len(batch.Quotes))
	}
	for _, s := range symbols[30:] {
		if got := batch.Outcomes[s]; got != QuoteOutcomeNotCovered {
			t.Errorf("outcome for %s = %q, want not_covered (absent from a successful whole-market table)", s, got)
		}
	}
	if elapsed > 20*time.Second {
		t.Errorf("chunk took %s; the bounded path must stay far inside the 60 s chunk timeout", elapsed)
	}
}

// TestHybridQuoteChain_AsksPerSymbolArmUpToTheResidualBudget is the complement:
// a residual at the budget is still served by the cheaper per-symbol arm.
func TestHybridQuoteChain_AsksPerSymbolArmUpToTheResidualBudget(t *testing.T) {
	for _, tc := range []struct {
		name       string
		residual   int
		wantCalled bool
	}{
		{name: "at budget", residual: hybridNarrowResidualMax, wantCalled: true},
		{name: "above budget", residual: hybridNarrowResidualMax + 1, wantCalled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			symbols := chainSymbols(tc.residual)
			drop := make(map[string]bool, tc.residual)
			for _, s := range symbols {
				drop[s] = true
			}
			fubon := newRecordingUpstream(t, fubonQuotesHandler(drop, nil))
			finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
			// The whole-market arm publishes only the first symbol, so any
			// residual that survives it is what the per-symbol arm must decide.
			twse := newRecordingUpstream(t, twseDailyQuotesHandler(map[string]bool{symbols[0]: true}))

			p := &HybridProvider{
				fubonProvider:   newFubonTestProvider(t, fubon.Server),
				finmindProvider: newFinMindTestProvider(t, finmind.Server),
				twseClient:      newTWSETestProvider(t, twse.Server),
				breakers: map[string]*providerBreaker{
					"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
					"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
				},
			}

			batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
			if err != nil {
				t.Fatalf("GetQuotesBatch: %v", err)
			}
			called := finmind.count() > 0
			if called != tc.wantCalled {
				t.Fatalf("FinMind called = %v, want %v (residual %d, budget %d)",
					called, tc.wantCalled, tc.residual, hybridNarrowResidualMax)
			}
			if tc.wantCalled && len(batch.Quotes) != tc.residual {
				t.Errorf("resolved %d quotes, want %d", len(batch.Quotes), tc.residual)
			}
		})
	}
}

// TestHybridQuoteChain_PerSymbolArmDeadlineBoundsTheArm pins the second guard:
// even for a small residual, a per-symbol arm that stalls cannot consume the
// caller's whole chunk budget.
func TestHybridQuoteChain_PerSymbolArmDeadlineBoundsTheArm(t *testing.T) {
	symbols := chainSymbols(3)

	fubon := newRecordingUpstream(t, fubonQuotesHandler(
		map[string]bool{symbols[0]: true, symbols[1]: true, symbols[2]: true}, nil))
	// A FinMind upstream that never answers.
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })
	finmind := newRecordingUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-blocked:
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
		http.Error(w, "too slow", http.StatusGatewayTimeout)
	})
	published := map[string]bool{symbols[2]: true}
	twse := newRecordingUpstream(t, twseDailyQuotesHandler(published))

	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}
	p.SetNarrowFallbackPolicy(hybridNarrowResidualMax, 200*time.Millisecond)

	start := time.Now()
	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("chunk took %s: the per-symbol arm deadline did not bound the arm", elapsed)
	}
	// The wide arm still ran and resolved what it could.
	if len(batch.Quotes) != 1 {
		t.Fatalf("resolved %d quotes, want 1 (the symbol the TWSE table publishes)", len(batch.Quotes))
	}
	for _, s := range symbols[:2] {
		if got := batch.Outcomes[s]; got != QuoteOutcomeNotCovered {
			t.Errorf("outcome for %s = %q, want not_covered (the TWSE table does not publish it and the stalled arm was cut off)", s, got)
		}
	}
}

// ── Requirement 3: the three kinds of "missing" are distinguishable ─────────

// TestHybridQuoteChain_NoDataVersusNotCovered separates a symbol the
// whole-market table publishes without a usable price (suspended / no trades)
// from a symbol the table does not publish at all (out of venue scope).
func TestHybridQuoteChain_NoDataVersusNotCovered(t *testing.T) {
	symbols := []string{"1101", "1102", "1103", "1104"}

	// fubon answers nothing at all, so every symbol reaches the wide arm.
	fubon := newRecordingUpstream(t, fubonQuotesHandler(
		map[string]bool{"1101": true, "1102": true, "1103": true, "1104": true}, nil))
	finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
	// The TWSE table publishes 1101 (good), 1102 (present but all-zero) and a
	// truncated row for 1103, and nothing at all for 1104.
	twse := newRecordingUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		rows := [][]string{
			{"1101", "TEST", "1000", "100000", "99.00", "101.00", "98.00", "100.00", "+1.00", "500"},
			{"1102", "SUSP", "0", "0", "0.00", "0.00", "0.00", "0.00", "0.00", "0"},
			{"1103", "SHORT", "x"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TWSEDailyResponse{Stat: "OK", Data: rows})
	})

	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}
	// Keep the per-symbol arm out of the way (residual 4 > budget 1) so the wide
	// arm's verdicts are what the test observes.
	p.SetNarrowFallbackPolicy(1, 50*time.Millisecond)

	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	want := map[string]QuoteOutcome{
		"1101": QuoteOutcomeOK,
		"1102": QuoteOutcomeNoData,
		// 1103's row is too short to be a security row, so TWSEClient filters it
		// before the chain sees it and the symbol looks unpublished. Reported as
		// not_covered rather than no_data: the chain can only classify what the
		// client surfaces. A truncated-row counter on the client is a separate
		// concern from #1986 and is deliberately not asserted here.
		"1103": QuoteOutcomeNotCovered,
		"1104": QuoteOutcomeNotCovered,
	}
	for sym, wantOutcome := range want {
		if got := batch.Outcomes[sym]; got != wantOutcome {
			t.Errorf("outcome for %s = %q, want %q", sym, got, wantOutcome)
		}
	}
}

// TestHybridQuoteChain_PerSymbolNoDataIsReported pins that the FinMind arm can
// report an authoritative "no row for this symbol" instead of an error, which is
// what makes the benign category reachable in production (a suspended 上櫃 name
// the TWSE table does not carry either).
func TestHybridQuoteChain_PerSymbolNoDataIsReported(t *testing.T) {
	symbols := []string{"1101", "1102"}

	fubon := newRecordingUpstream(t, fubonQuotesHandler(
		map[string]bool{"1101": true, "1102": true}, nil))
	finmind := newRecordingUpstream(t, finmindQuotesHandler(map[string]bool{"1102": true}))
	twse := newRecordingUpstream(t, twseDailyQuotesHandler(map[string]bool{"1101": true}))

	p := &HybridProvider{
		fubonProvider:   newFubonTestProvider(t, fubon.Server),
		finmindProvider: newFinMindTestProvider(t, finmind.Server),
		twseClient:      newTWSETestProvider(t, twse.Server),
		breakers: map[string]*providerBreaker{
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
		},
	}

	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["1102"]; got != QuoteOutcomeNoData {
		t.Fatalf("outcome for the symbol FinMind has no row for = %q, want no_data", got)
	}
	if got := len(batch.Quotes); got != 1 {
		t.Fatalf("resolved %d quotes, want 1", got)
	}
}

// TestHybridQuoteChain_NonTradingDayIsAnErrorNotNoData pins the conservative
// reading of a holiday: it says nothing about any individual symbol.
func TestHybridQuoteChain_NonTradingDayIsAnErrorNotNoData(t *testing.T) {
	symbols := []string{"1101"}
	finmind := newRecordingUpstream(t, finmindQuotesHandler(nil))
	p := NewFinMindProviderWithClient(newFinMindTestProvider(t, finmind.Server).GetClient())

	// 2026-09-25 is a Taiwan market holiday (verified with the repo calendar).
	holiday, _ := time.Parse("2006-01-02", "2026-09-25")
	if isTaiwanTradingDay(holiday) {
		t.Fatalf("fixture assumes 2026-09-25 is a non-trading day")
	}
	batch, err := p.GetQuotesBatch(context.Background(), holiday, symbols)
	if err == nil {
		t.Fatal("expected an error for a non-trading day")
	}
	if got := batch.Outcomes["1101"]; got != QuoteOutcomeError {
		t.Errorf("outcome = %q, want error", got)
	}
	if n := finmind.count(); n != 0 {
		t.Errorf("FinMind requests = %d, want 0: a non-trading day must not spend quota", n)
	}
}

// TestHybridQuoteChain_EmptyQuotesWithNoErrorStillReportsOutcomes pins that a
// provider answering with nothing does not leave the caller guessing.
func TestHybridQuoteChain_EmptyQuotesWithNoErrorStillReportsOutcomes(t *testing.T) {
	symbols := chainSymbols(3)
	// No arms configured at all: the batch must still carry a verdict per
	// symbol instead of an empty map.
	p := &HybridProvider{}
	batch, err := p.GetQuotesBatch(context.Background(), tradingAsOf(t), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if len(batch.Quotes) != 0 {
		t.Fatalf("quotes = %d, want 0", len(batch.Quotes))
	}
	for _, s := range symbols {
		if got := batch.Outcomes[s]; got == QuoteOutcomeOK {
			t.Errorf("outcome for %s = ok with no provider configured", s)
		}
	}
}

// TestHybridProvider_NilAwareQuoteBatchAdapter pins the adapter used for
// providers that cannot classify.
func TestHybridProvider_NilAwareQuoteBatchAdapter(t *testing.T) {
	symbols := []string{"1101", "1102"}
	batch, err := GetQuotesBatch(context.Background(), nil, time.Now(), symbols, QuoteOutcomeError)
	if err != nil {
		t.Fatalf("GetQuotesBatch(nil): %v", err)
	}
	for _, s := range symbols {
		if got := batch.Outcomes[s]; got != QuoteOutcomeError {
			t.Errorf("outcome for %s = %q, want error", s, got)
		}
	}
}

// TestQuoteBatch_MissingDoesNotReaskBenignSymbols documents the two notions of
// "missing" the chain depends on.
func TestQuoteBatch_MissingDoesNotReaskBenignSymbols(t *testing.T) {
	symbols := []string{"a", "b", "c", "d"}
	batch := NewQuoteBatch(symbols)
	batch.Record(domain.Quote{Symbol: "a", Last: 1, Open: 1, High: 1, Low: 1, Volume: 1})
	batch.SetOutcome("b", QuoteOutcomeNoData)
	batch.SetOutcome("c", QuoteOutcomeNotCovered)

	if got := batch.Missing(symbols); len(got) != 1 || got[0] != "d" {
		t.Errorf("Missing = %v, want [d] (only the unresolved symbol)", got)
	}
	if got := batch.MissingQuote(symbols); len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Errorf("MissingQuote = %v, want [c d] (not_covered is still worth another source)", got)
	}
	if got := batch.UnresolvedCount(symbols); got != 1 {
		t.Errorf("UnresolvedCount = %d, want 1", got)
	}
}
