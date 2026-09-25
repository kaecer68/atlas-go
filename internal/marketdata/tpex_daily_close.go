package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata/twse"
)

// TPEx (證券櫃檯買賣中心, 上櫃) whole-market daily close quotes, one request for
// the entire 上櫃 market (issue #1986).
//
// # Why this source exists
//
// The SmartUniverse population is the whole listed market (上市 + 上櫃, 1,599
// symbols on 2026-09-25) because the per-stock industry substrate (#1943/#1977)
// replaced the classification tree's ~27 representative stocks. TWSE's
// STOCK_DAY_ALL covers 上市 only: measured on 2026-09-24 it returned 1,380 rows,
// of which 904 were in the universe population, so 695 (43 %) of the population
// had NO quote from that arm. Every per-symbol fallback arm (FinMind, Fugle)
// issues one HTTP request per symbol and is rate limited to 0.17-0.5 req/s, so
// it cannot close a 695-symbol gap inside any chunk timeout — the gap has to be
// closed by a *whole-market* source, which is what this client is.
//
// Measured coverage of the same 1,599-symbol population on 2026-09-24:
// TWSE 904 + TPEx 689 = 1,593 (99.6 %); the remaining 6 symbols are published
// by neither venue's daily table.
//
// Governance: like TWSEClient, this client is called directly by the coverage
// layer of the quote chain rather than through apigateway.Fetch(channelID) —
// the 「hybrid provider (live/simulation) | 有自己的 providerBreaker + fallback
// 鏈」exception in internal/apigateway/CONSTITUTION.md § 1.4. It reuses the
// shared TWSE/TPEx token bucket (one operator, one documented OpenAPI policy)
// and its own circuit breaker, and it is only ever asked for symbols the
// primary arms could not resolve.
const (
	tpexAPIBaseURL = "https://www.tpex.org.tw/openapi/v1"

	// tpexDailyClosePath is the first-party "上櫃每日收盤行情" OpenAPI table.
	tpexDailyClosePath = "/tpex_mainboard_daily_close_quotes"
)

// tpexDailyCloseRow is one row of tpex_mainboard_daily_close_quotes.
//
// Every numeric field is a JSON *string* upstream, and the placeholders differ
// per field ("---", "--", "-", ""), so they are all parsed with twse.ParseFloat
// / twse.ParseInt64 (the same helpers the TWSE OpenAPI parsers use).
//
// Deliberately NO `json:` tags: the exported field names are already the
// upstream keys (encoding/json matches field names case-insensitively), and the
// field-type generator (cmd/gentags) turns every JSON tag in the repo into a
// frontend field name. Tagging these market-table columns would leak
// "TradingShares" / "TransactionAmount" into the web field registry for no
// reason.
type tpexDailyCloseRow struct {
	Date                  string
	SecuritiesCompanyCode string
	CompanyName           string
	Close                 string
	Change                string
	Open                  string
	High                  string
	Low                   string
	Average               string
	TradingShares         string
	TransactionAmount     string
}

// TPExDailyCloseClient fetches the whole-market 上櫃 daily close table.
type TPExDailyCloseClient struct {
	httpClient *http.Client
	baseURL    string
	breaker    *providerBreaker
	retryCfg   retryConfig
}

// GetSharedTPExDailyCloseClient returns a process-wide client so the shared
// TWSE/TPEx rate limiter and the circuit breaker are actually shared.
func GetSharedTPExDailyCloseClient() *TPExDailyCloseClient {
	sharedTPExOnce.Do(func() {
		timeout := time.Duration(config.GetParametersConfig().Marketdata.TWSEAPITimeoutSec.Value) * time.Second
		sharedTPExClient = &TPExDailyCloseClient{
			httpClient: httpclient.NewFactory().NewClient(timeout),
			baseURL:    tpexAPIBaseURL,
			breaker:    newProviderBreaker("tpex_daily_close", defaultCircuitBreakerConfig()),
			retryCfg:   twseRetryConfig(),
		}
	})
	return sharedTPExClient
}

var (
	sharedTPExClient *TPExDailyCloseClient
	sharedTPExOnce   sync.Once
)

// ResetSharedTPExDailyCloseClient clears the singleton (tests).
func ResetSharedTPExDailyCloseClient() {
	sharedTPExOnce = sync.Once{}
	sharedTPExClient = nil
}

// Name implements MarketWideQuoteSource.
func (c *TPExDailyCloseClient) Name() string { return "tpex_daily_close" }

// Snapshot returns every quote the 上櫃 daily table currently publishes.
//
// It is the whole-market primitive: the caller filters by symbol, and caching is
// the caller's business — marketdata.CoverageCompletingProvider holds a
// short-lived copy so the chunked universe fetch does not download the same
// ~4 MB table 32 times. Keeping this client a pure fetcher means a caller that
// genuinely wants a fresh table always gets one.
func (c *TPExDailyCloseClient) Snapshot(ctx context.Context) ([]domain.Quote, error) {
	return c.fetch(ctx)
}

func (c *TPExDailyCloseClient) fetch(ctx context.Context) ([]domain.Quote, error) {
	if c.breaker != nil && !c.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: tpex daily close circuit breaker open", ErrUpstream)
	}

	waitCtx, cancel := context.WithTimeout(ctx, twseRateLimitWaitAllowance)
	defer cancel()
	// Shared bucket with the TWSE OpenAPI family: same operator, same
	// documented OpenAPI rate policy (marketdata.twse_api_rate_limit /
	// twse_api_rate_burst), so the two first-party venues cannot collectively
	// exceed it.
	if err := getTWSESharedLimiter().Wait(waitCtx); err != nil {
		return nil, fmt.Errorf("tpex daily close rate limit wait: %w", err)
	}

	endpoint := c.baseURL + tpexDailyClosePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("tpex daily close: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	// fetchWithRetry gives the same bounded 429/5xx/transport retry schedule as
	// the TWSE OpenAPI client (2026-09-23 evidence: the table's upstream had
	// multi-second slow windows, and a single attempt lost the whole fetch).
	resp, err := fetchWithRetry(ctx, c.httpClient, req, c.retryCfg)
	if err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("tpex daily close: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		c.breakerRecordFailure()
		return nil, fmt.Errorf("tpex daily close: status %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rows []tpexDailyCloseRow
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("tpex daily close: decode response: %w", err)
	}
	if len(rows) == 0 {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("%w: tpex daily close returned no rows", ErrTWSEEmptyData)
	}

	quotes := make([]domain.Quote, 0, len(rows))
	asOf := time.Now()
	for _, r := range rows {
		symbol := strings.TrimSpace(r.SecuritiesCompanyCode)
		if symbol == "" {
			continue
		}
		quotes = append(quotes, domain.Quote{
			Symbol:     symbol,
			Last:       twse.ParseFloat(r.Close),
			Open:       twse.ParseFloat(r.Open),
			High:       twse.ParseFloat(r.High),
			Low:        twse.ParseFloat(r.Low),
			Volume:     twse.ParseInt64(r.TradingShares),
			Market:     "TW",
			AsOf:       asOf,
			IsTradable: true,
			Source:     "tpex_daily_close",
		})
	}
	if len(quotes) == 0 {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("%w: tpex daily close decoded zero usable rows", ErrTWSEEmptyData)
	}

	c.breakerRecordSuccess()
	logging.Info("tpex_daily_close", "snapshot_ok",
		"rows", len(rows),
		"quotes", len(quotes))
	return quotes, nil
}

func (c *TPExDailyCloseClient) breakerRecordFailure() {
	if c.breaker != nil {
		c.breaker.recordFailure()
	}
}

func (c *TPExDailyCloseClient) breakerRecordSuccess() {
	if c.breaker != nil {
		c.breaker.recordSuccess()
	}
}
