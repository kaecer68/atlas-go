package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

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

	latest := closes[len(closes)-1]
	if math.IsNaN(latest) || math.IsInf(latest, 0) || latest == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("%s: invalid latest price: %v", p.channelID, latest)
	}

	// Daily change: compare latest close to the previous trading day's close.
	prev := latest
	if len(closes) > 1 {
		candidate := closes[len(closes)-2]
		if !math.IsNaN(candidate) && !math.IsInf(candidate, 0) && candidate != 0 {
			prev = candidate
		}
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
	mu        sync.RWMutex
	client    *http.Client
	cookie    string
	crumb     string
	lastFetch time.Time
	ttl       time.Duration
	hosts     []string
}

// newYahooSession creates a session manager for Yahoo Finance API access.
func newYahooSession() *yahooSession {
	return &yahooSession{
		client: httpclient.NewFactory().NewClient(15 * time.Second),
		hosts:  yahooHosts,
		ttl:    15 * time.Minute, // re-fetch crumb every 15 minutes
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
	// Step 1: Get session cookie from fc.yahoo.com
	cookieURL := "https://fc.yahoo.com/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cookieURL, nil)
	if err != nil {
		return fmt.Errorf("create cookie request: %w", err)
	}
	setUA(req)

	resp, err := s.client.Do(req)
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
	for _, host := range s.hosts {
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
func (s *yahooSession) ensureCrumb(ctx context.Context) error {
	s.mu.RLock()
	hasCrumb := s.crumb != "" && time.Since(s.lastFetch) < s.ttl
	s.mu.RUnlock()
	if hasCrumb {
		return nil
	}
	return s.fetchCrumb(ctx)
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

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 && body[0] == '<' {
			return nil, fmt.Errorf("http %d: HTML response from %s (rate limited or blocked)", resp.StatusCode, host)
		}
		return nil, fmt.Errorf("http %d from %s: %s", resp.StatusCode, host, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if len(body) > 0 && body[0] == '<' {
		return nil, fmt.Errorf("HTML response from %s (likely rate limited)", host)
	}

	return body, nil
}

// fetchWithFallback tries each host in order and returns on first success.
func (s *yahooSession) fetchWithFallback(ctx context.Context, symbol string, params map[string]string) ([]byte, error) {
	var lastErr error
	// Rotate starting host for load distribution
	startIdx := int(time.Now().UnixNano() / 1e9 % int64(len(s.hosts)))
	for i := 0; i < len(s.hosts); i++ {
		host := s.hosts[(startIdx+i)%len(s.hosts)]
		body, err := s.fetchFromHost(ctx, host, symbol, params)
		if err == nil {
			return body, nil
		}
		lastErr = err
		logging.Warn("yahoo_session", "host_failed", "host", host, "error", err)
	}
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

	// Reset shared caches so each mock-server test starts from a clean
	// state and does not observe data from a prior test/subtest.
	usCache.reset()
	twiiCache.reset()

	// Replace the shared rate limiter with an unlimited one so mock-based
	// tests do not accumulate artificial wait time.
	yahooSharedLimiter = rate.NewLimiter(rate.Inf, 0)
}
