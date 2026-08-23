package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// Yahoo stock/index provider conventions:
//   - Range: "5d" (previous trading day comparison) — NEVER use "1y" which
//     produces year-over-year change instead of daily change.
//   - ChangePct bounds: ±30% hard cap — implausible daily changes are
//     rejected as a safety gate against data corruption.
//
// These values are package-level constants so all Yahoo providers stay
// synchronized; a single-source-of-truth prevents drift across 7 providers.
const (
	yahooStockRange   = "5d" // Yahoo chart range for daily change computation
	maxDailyChangePct = 30.0 // abs(changePct) > 30% is rejected as implausible
)

// YahooStockProvider is a parametrized Yahoo Finance provider for stocks and
// indices. It eliminates ~250 lines of duplicate boilerplate across 7 separate
// provider files by centralizing the fetch→parse→bounds→emit pipeline.
type YahooStockProvider struct {
	ticker    string
	channelID string
	fieldFn   func(*MacroDataSnapshot) *MacroDataPoint
}

// newYahooStockProvider creates a Yahoo stock/index provider.
func newYahooStockProvider(ticker, channelID string, fieldFn func(*MacroDataSnapshot) *MacroDataPoint) *YahooStockProvider {
	return &YahooStockProvider{
		ticker:    ticker,
		channelID: channelID,
		fieldFn:   fieldFn,
	}
}

// Name returns the data channel identifier.
func (p *YahooStockProvider) Name() string { return p.channelID }

// FetchSnapshot fetches the latest price and daily change from Yahoo Finance.
func (p *YahooStockProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	s := getYahooSession()
	if err := yahooSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s rate limit: %w", p.channelID, err)
	}

	params := map[string]string{
		"interval": "1d",
		"range":    yahooStockRange,
	}

	// Check shared US market cache (P1 B01+B02: deduplicate across 7 channels)
	var body []byte
	if cached := usCache.get(p.ticker, params["interval"], params["range"]); cached != nil {
		body = cached
	} else {
		var err error
		body, err = s.fetchWithFallback(ctx, p.ticker, params)
		if err != nil {
			return MacroDataSnapshot{}, fmt.Errorf("%s: %w", p.channelID, err)
		}
		usCache.set(p.ticker, params["interval"], params["range"], body)
	}

	chartResp, err := UnmarshalYahooChart(body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("%s: %w", p.channelID, err)
	}

	result := chartResp.Chart.Result
	if len(result) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: no chart result", p.channelID)
	}

	closes := result[0].Indicators.Quote[0].Close
	if len(closes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: no close prices", p.channelID)
	}

	// P0-6: unified prev-close algorithm — findLastValidClose is the SAME
	// helper the macro provider uses (yahoo_macro_provider.go, #1601 DXY
	// fix): walk backwards skipping zero/NaN/Inf so both the latest valid
	// close and the previous valid close are found even when Yahoo pads the
	// tail with 0/NaN (off-hours). Previously this provider only looked at
	// closes[len-2], so a trailing zero made prev=latest → change_pct was
	// stuck at 0 — the exact regression that hit US channels (change_pct
	// 恆 0) before the macro side was fixed.
	latest, prev := findLastValidClose(closes)
	if latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: no valid close prices", p.channelID)
	}

	changePct := 0.0
	if prev != 0 {
		changePct = (latest - prev) / prev * 100
	}

	if math.IsNaN(changePct) || math.IsInf(changePct, 0) {
		return MacroDataSnapshot{}, fmt.Errorf("%s: invalid change percentage: %v", p.channelID, changePct)
	}

	if math.Abs(changePct) > maxDailyChangePct {
		return MacroDataSnapshot{}, fmt.Errorf("%s: implausible daily change %.2f%% (>|%.1f%%|)",
			p.channelID, changePct, maxDailyChangePct)
	}

	snap := MacroDataSnapshot{
		RecordedAt: time.Now().Unix(),
	}
	if target := p.fieldFn(&snap); target != nil {
		target.Symbol = p.ticker
		target.Value = latest
		target.ChangePct = math.Round(changePct*100) / 100
		target.Timestamp = result[0].Meta.RegularMarketTime
	}
	return snap, nil
}

// yahooSession manages Yahoo Finance crumb + cookie authentication.
// Yahoo gradually tightened access to its v8 chart endpoint, requiring a
// crumb token tied to a session cookie. This manager performs the handshake
// once and reuses the credentials for subsequent requests.
type yahooSession struct {
	mu          sync.RWMutex
	client      *http.Client
	cookie      string
	crumb       string
	lastFetch   time.Time
	ttl         time.Duration
	hosts       []string
	crumbFlight singleflight.Group
	// breaker is the session-level circuit breaker (P1-7). Every Yahoo
	// channel funnels through fetchWithFallback on this singleton session,
	// so one breaker short-circuits all 8+ channels when Yahoo is down.
	// "no valid close" / empty-chart parse results are NOT recorded here
	// (they are data-shape issues handled per-provider); transport/HTTP
	// failures are.
	breaker *providerBreaker
	// blockedUntil is the negative cache (P1-14): when Yahoo serves 429 or
	// an HTML block page, fetchFromHost records a session-level block
	// (Retry-After clamped to [5,10] minutes) and every Yahoo channel
	// short-circuits without touching the network until it elapses.
	blockedUntil time.Time
}

// newYahooSession creates a session manager for Yahoo Finance API access.
func newYahooSession() *yahooSession {
	return &yahooSession{
		client:  httpclient.NewFactory().NewClient(15 * time.Second),
		hosts:   yahooHosts,
		ttl:     15 * time.Minute, // re-fetch crumb every 15 minutes
		breaker: newProviderBreaker("yahoo", defaultCircuitBreakerConfig()),
	}
}

// buildChartURL constructs a properly encoded URL for the v8 chart endpoint.
// NOTE: url.PathEscape is NOT used here because it would pre-encode the symbol,
// then url.URL.String() would re-encode it causing double encoding
// (e.g. ^VIX → %5EVIX → %255EVIX). Instead, url.URL.String() handles
// single encoding naturally from the raw Path field.
func (s *yahooSession) buildChartURL(host, symbol string, params map[string]string) string {
	u := &url.URL{
		Scheme: "https",
		Host:   host,
		Path:   fmt.Sprintf("/v8/finance/chart/%s", symbol),
	}

	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

// getCrumb performs the Yahoo crumb handshake.
// Steps:
//  1. Fetch a session cookie from fc.yahoo.com
//  2. Use that cookie to fetch a crumb token
func (s *yahooSession) fetchCrumb(ctx context.Context) error {
	// Snapshot client and hosts under the read lock, then perform all network
	// I/O without holding any lock. Only the final state write takes the lock.
	s.mu.RLock()
	client := s.client
	hosts := s.hosts
	s.mu.RUnlock()

	// Step 1: Get session cookie from fc.yahoo.com
	cookieURL := "https://fc.yahoo.com/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cookieURL, nil)
	if err != nil {
		return fmt.Errorf("create cookie request: %w", err)
	}
	setUA(req)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch cookie: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Consume and discard body
	_, _ = io.Copy(io.Discard, resp.Body)

	var sessionCookie string
	for _, c := range resp.Cookies() {
		if c.Name == "A3" || c.Name == "B3" {
			sessionCookie = c.Name + "=" + c.Value
			break
		}
	}
	if sessionCookie == "" {
		// Try any Yahoo session cookie
		for _, c := range resp.Cookies() {
			if c.Name != "" {
				sessionCookie = c.Name + "=" + c.Value
				break
			}
		}
	}
	if sessionCookie == "" {
		return fmt.Errorf("no session cookie received from fc.yahoo.com")
	}

	// Step 2: Get crumb using the session cookie
	var crumb string
	for _, host := range hosts {
		crumbURL := fmt.Sprintf("https://%s/v1/test/getcrumb", host)
		cReq, err := http.NewRequestWithContext(ctx, http.MethodGet, crumbURL, nil)
		if err != nil {
			continue
		}
		setUA(cReq)
		cReq.Header.Set("Cookie", sessionCookie)
		cReq.Header.Set("Referer", "https://finance.yahoo.com/")

		cResp, err := s.client.Do(cReq)
		if err != nil {
			logging.Warn("yahoo_session", "crumb_host_failed", "host", host, "error", err)
			continue
		}
		body, err := io.ReadAll(cResp.Body)
		_ = cResp.Body.Close()
		if err != nil || cResp.StatusCode != http.StatusOK {
			continue
		}
		crumb = string(body)
		if crumb != "" {
			break
		}
	}

	if crumb == "" {
		return fmt.Errorf("failed to obtain crumb from all Yahoo hosts")
	}

	s.mu.Lock()
	s.cookie = sessionCookie
	s.crumb = crumb
	s.lastFetch = time.Now()
	s.mu.Unlock()

	logging.Info("yahoo_session", "crumb_acquired", "hosts_tried", len(s.hosts))
	return nil
}

// ensureCrumb refreshes the crumb if it's stale.
//
// All network I/O happens outside of s.mu; the lock is only held to read the
// cached state and to write the result back. singleflight guarantees that
// concurrent callers share at most one in-flight crumb handshake, preventing
// a burst of requests from each performing the expensive cookie+crumb dance.
func (s *yahooSession) ensureCrumb(ctx context.Context) error {
	s.mu.RLock()
	hasCrumb := s.crumb != "" && time.Since(s.lastFetch) < s.ttl
	s.mu.RUnlock()
	if hasCrumb {
		return nil
	}

	_, err, _ := s.crumbFlight.Do("crumb", func() (interface{}, error) {
		// Double-check after winning the singleflight slot: another caller
		// may have refreshed the crumb while we were waiting.
		s.mu.RLock()
		hasCrumb := s.crumb != "" && time.Since(s.lastFetch) < s.ttl
		s.mu.RUnlock()
		if hasCrumb {
			return nil, nil
		}
		return nil, s.fetchCrumb(ctx)
	})
	return err
}

// fetchFromHost performs a Yahoo Finance API request with crumb auth.
// It tries each host in order and handles redirects/HTML/error responses.
func (s *yahooSession) fetchFromHost(ctx context.Context, host, symbol string, params map[string]string) ([]byte, error) {
	// Ensure we have a valid crumb
	if err := s.ensureCrumb(ctx); err != nil {
		logging.Warn("yahoo_session", "crumb_fetch_failed", "error", err)
		// Continue without crumb — might still work for some symbols
	}

	s.mu.RLock()
	hasCrumb := s.crumb != ""
	s.mu.RUnlock()

	// Build URL with optional crumb
	u := s.buildChartURL(host, symbol, params)

	// Add crumb if we have one
	if hasCrumb {
		s.mu.RLock()
		crumb := s.crumb
		s.mu.RUnlock()

		parsedURL, err := url.Parse(u)
		if err == nil {
			q := parsedURL.Query()
			q.Set("crumb", crumb)
			parsedURL.RawQuery = q.Encode()
			u = parsedURL.String()
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	setUA(req)
	req.Header.Set("Referer", "https://finance.yahoo.com/")
	if hasCrumb {
		s.mu.RLock()
		req.Header.Set("Cookie", s.cookie)
		s.mu.RUnlock()
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	block := func(reason string) error {
		// P1-14: negative cache — a 429 (or any HTML block page) means Yahoo
		// is rate-limiting our IP. Honor Retry-After when present, clamp to
		// [5,10] minutes, and mark the whole session blocked so every channel
		// short-circuits instead of re-hammering the same IP.
		wait := negativeCacheBlockMin
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
				wait = time.Duration(secs) * time.Second
			} else if t, parseErr := http.ParseTime(ra); parseErr == nil {
				if d := time.Until(t); d > 0 {
					wait = d
				}
			}
		}
		if wait < negativeCacheBlockMin {
			wait = negativeCacheBlockMin
		}
		if wait > negativeCacheBlockMax {
			wait = negativeCacheBlockMax
		}
		s.markBlocked(wait)
		logging.Warn("yahoo_session", "negative_cache_block",
			"host", host, "reason", reason, "block_minutes", int(wait.Minutes()))
		return fmt.Errorf("%s: %s", reason, host)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 && body[0] == '<' {
			return nil, block(fmt.Sprintf("http %d: HTML response (rate limited or blocked)", resp.StatusCode))
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, block("http 429: rate limited")
		}
		return nil, fmt.Errorf("http %d from %s: %s", resp.StatusCode, host, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if len(body) > 0 && body[0] == '<' {
		return nil, block("HTML response (likely rate limited)")
	}

	return body, nil
}

// breakerRecordSuccess / breakerRecordFailure are nil-safe breaker wrappers.
func (s *yahooSession) breakerRecordSuccess() {
	if s.breaker != nil {
		s.breaker.recordSuccess()
	}
}

func (s *yahooSession) breakerRecordFailure() {
	if s.breaker != nil {
		s.breaker.recordFailure()
	}
}

// BreakerInfo exposes the breaker state for tests and observability.
func (s *yahooSession) BreakerInfo() ProviderBreakerInfo {
	if s.breaker == nil {
		return ProviderBreakerInfo{Name: "yahoo", State: ProviderCircuitClosed}
	}
	return s.breaker.stateSnapshot()
}

// blockedUntilTime returns the negative-cache expiry (zero when no block is
// active). The value is read under the lock so callers never observe a torn
// timestamp (race-free under concurrent markBlocked).
func (s *yahooSession) blockedUntilTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockedUntil
}

// markBlocked sets the session-level negative cache for d (clamped by the
// caller to [5,10] minutes).
func (s *yahooSession) markBlocked(d time.Duration) {
	s.mu.Lock()
	s.blockedUntil = time.Now().Add(d)
	s.mu.Unlock()
}

// negativeCacheBlockMin/Max bound the negative-cache window: long enough to
// stop 8+ channels hammering a blocked IP, short enough to recover quickly
// once Yahoo lifts the block.
const (
	negativeCacheBlockMin = 5 * time.Minute
	negativeCacheBlockMax = 10 * time.Minute
)

// fetchWithFallback tries each host in order and returns on first success.
// P1-7: the session-level breaker gates the whole fallback chain — when it
// is open we return immediately without touching the network, so every
// Yahoo channel (US indices, tw_vol, TAIEX, DRAM, SOX, …) short-circuits
// together. All-hosts-failed records a failure; any-host-success resets.
// P1-14: the negative cache additionally short-circuits after a 429/HTML
// block, so a blocked IP stops being hammered by every channel each cycle.
func (s *yahooSession) fetchWithFallback(ctx context.Context, symbol string, params map[string]string) ([]byte, error) {
	if until := s.blockedUntilTime(); time.Now().Before(until) {
		return nil, fmt.Errorf("%w: yahoo negative-cache block active until %s", ErrUpstream, until.Format(time.RFC3339))
	}
	if s.breaker != nil && !s.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: yahoo circuit breaker open", ErrUpstream)
	}
	var lastErr error
	// Rotate starting host for load distribution
	startIdx := int(time.Now().UnixNano() / 1e9 % int64(len(s.hosts)))
	for i := 0; i < len(s.hosts); i++ {
		host := s.hosts[(startIdx+i)%len(s.hosts)]
		body, err := s.fetchFromHost(ctx, host, symbol, params)
		if err == nil {
			s.breakerRecordSuccess()
			return body, nil
		}
		lastErr = err
		logging.Warn("yahoo_session", "host_failed", "host", host, "error", err)
	}
	s.breakerRecordFailure()
	return nil, fmt.Errorf("all hosts failed for %s: %w", symbol, lastErr)
}

// yahooChartError represents the error object in Yahoo chart responses.
type yahooChartError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// yahooChartResult is the parsed top-level Yahoo Finance chart response.
type yahooChartResult struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketTime  int64   `json:"regularMarketTime"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *yahooChartError `json:"error,omitempty"`
	} `json:"chart"`
}

// UnmarshalYahooChart parses a Yahoo Finance chart response, handling error objects.
func UnmarshalYahooChart(body []byte) (*yahooChartResult, error) {
	var result yahooChartResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	if result.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo api error: [%s] %s",
			result.Chart.Error.Code, result.Chart.Error.Description)
	}
	return &result, nil
}

// globalYahooSession is the singleton session shared across all Yahoo providers.
var globalYahooSession struct {
	once sync.Once
	s    *yahooSession
}

func getYahooSession() *yahooSession {
	globalYahooSession.once.Do(func() {
		globalYahooSession.s = newYahooSession()
	})
	return globalYahooSession.s
}

// setUA sets a rotating User-Agent header on a request.
func setUA(req *http.Request) {
	ua := modernUserAgents[time.Now().UnixNano()%int64(len(modernUserAgents))]
	req.Header.Set("User-Agent", ua)
}

// SetYahooSessionClient overrides the HTTP client, hosts, and pre-populates
// a fake crumb on the singleton session. Only used in tests.
func SetYahooSessionClient(client *http.Client) {
	s := getYahooSession()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client = client
	s.hosts = yahooHosts
	// Pre-populate a test crumb so ensureCrumb does not hit real fc.yahoo.com.
	s.crumb = "test-crumb"
	s.cookie = "test-cookie=1"
	s.lastFetch = time.Now()
	// Reset the session-level breaker so every mock-server test starts from
	// a closed circuit (the singleton breaker otherwise accumulates failures
	// across unrelated failure-path tests and trips the whole package).
	if s.breaker != nil {
		s.breaker.reset()
	}
	// Reset the negative cache too (P1-14).
	s.blockedUntil = time.Time{}

	// Reset shared caches so each mock-server test starts from a clean
	// state and does not observe data from a prior test/subtest.
	usCache.reset()
	twiiCache.reset()

	// Replace the shared rate limiter with an unlimited one so mock-based
	// tests do not accumulate artificial wait time.
	yahooSharedLimiter = rate.NewLimiter(rate.Inf, 0)
}
