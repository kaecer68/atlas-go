package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
)

// twseBrowserUserAgent is a full browser User-Agent required to pass the
// TWSE WAF (short UAs such as "Mozilla/5.0" get 403 Forbidden).
const twseBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0 Safari/537.36"

// MarketVolumeResult holds the parsed 集中市場成交金額 from TWSE MI_INDEX.
// MarketVolume is in 億元 (hundred million NTD), matching the applyMarketVolume contract.
type MarketVolumeResult struct {
	MarketVolume float64 `json:"market_volume"` // 億元
	Date         string  `json:"date"`          // YYYYMMDD
}

// MarketVolumeProvider fetches 集中市場大盤統計資訊 (成交金額) from TWSE.
//
// Data source: TWSE exchangeReport/MI_INDEX?type=MS.
// Returns the 一般股票 成交金額 converted from 元 to 億元.
type MarketVolumeProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
}

// NewMarketVolumeProvider creates a new TWSE market volume provider.
func NewMarketVolumeProvider() *MarketVolumeProvider {
	return &MarketVolumeProvider{
		client:      httpclient.NewFactory().NewClient(20 * time.Second),
		baseURL:     constants.TWSEBaseURL,
		rateLimiter: getTWSESharedLimiter(), // P1-13: shared TWSE bucket
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (p *MarketVolumeProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.client = client
	}
}

// SetRateLimiter overrides the rate limiter (for testing).
func (p *MarketVolumeProvider) SetRateLimiter(lim *rate.Limiter) {
	if lim != nil {
		p.rateLimiter = lim
	}
}

// Name returns the provider name.
func (p *MarketVolumeProvider) Name() string {
	return "market_volume"
}

// FetchLatest retrieves the most recent 集中市場成交金額.
// Calendar-aware scan (#1767): walk back over EXPECTED Taiwan trading days
// only (weekends/holidays skipped via taiwanholidays) instead of blind 7
// calendar days — on a Sunday the blind scan burned 2 rate-limiter tokens on
// empty Saturday/Sunday queries before reaching Friday, which under the
// shared TWSE bucket queued past the caller's context deadline and tripped
// the circuit breaker for a channel whose data was actually fine.
func (p *MarketVolumeProvider) FetchLatest(ctx context.Context) (*MarketVolumeResult, error) {
	const maxAttempts = 3
	var lastErr error
	for _, day := range RecentTradingDays(time.Now().UTC(), maxAttempts) {
		result, err := p.fetchDate(ctx, day.Format("20060102"))
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("no TWSE market volume data available in the last %d expected trading days: last error: %w", maxAttempts, lastErr)
}

// FetchDate retrieves 集中市場成交金額 for a specific trading date (YYYYMMDD).
// Non-trading days (weekends/holidays) return an error so callers can skip them.
// Exported for historical backfill (cmd/backfill-market-volume); FetchLatest
// keeps scanning the recent window via the private fetchDate.
func (p *MarketVolumeProvider) FetchDate(ctx context.Context, dateStr string) (*MarketVolumeResult, error) {
	return p.fetchDate(ctx, dateStr)
}

func (p *MarketVolumeProvider) fetchDate(ctx context.Context, dateStr string) (*MarketVolumeResult, error) {
	if err := p.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := fmt.Sprintf("%s/exchangeReport/MI_INDEX?response=json&date=%s&type=MS", p.baseURL, dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	// Full browser UA: TWSE's WAF returns 403 for short UAs (observed
	// 2026-08-21 during R4 backfill). Matches government_broker_aggregator.
	req.Header.Set("User-Agent", twseBrowserUserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseMIIndexResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// A closed market (weekend, holiday) answers with prose in `stat`, not with
	// an HTTP error — measured 2026-09-27: date=20260925 (中秋節) →
	// {"stat":"很抱歉，沒有符合條件的資料!","date":"20260925"} with 10 empty tables,
	// so neither the stat nor the table count may be read as "OK, one day".
	if !strings.EqualFold(apiResp.Stat, "OK") {
		return nil, fmt.Errorf("TWSE MI_INDEX returned no data: stat=%s tables=%d", apiResp.Stat, len(apiResp.Tables))
	}

	// 大盤統計資訊, selected by field names so the title read below provably
	// belongs to the market-stats section (E29-2).
	marketTable, ok := apiResp.MarketStatsTable()
	if !ok {
		return nil, fmt.Errorf("TWSE MI_INDEX: 大盤統計資訊 table not found (schema change?): tables=%d", len(apiResp.Tables))
	}
	if len(marketTable.Data) == 0 {
		return nil, fmt.Errorf("TWSE MI_INDEX returned empty market stats")
	}

	// E29-2 (2026-09-27): the day these numbers describe comes from the PAYLOAD,
	// never from the request. The 大盤統計資訊 title states it ("115年09月24日
	// 大盤統計資訊") while the envelope's `date` is not evidence at all — TWSE
	// echoes the requested date there even when it cannot answer the request
	// (same measurement as above). Stamping `dateStr` onto the result, which is
	// what this provider used to do, is how a mis-dated payload (a cached or
	// "latest-only" upstream answer) entered the snapshot labelled as the
	// requested trading day. A payload for another day is refused, so the caller
	// can walk back to a day the exchange actually published.
	dataDate, ok := marketTable.TitleDate()
	if !ok {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: title carries no data date (schema change?): title=%q", marketTable.Title)
	}
	wantDate, err := time.Parse("20060102", dateStr)
	if err != nil {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: invalid requested date %q: %w", dateStr, err)
	}
	if want := wantDate.Format("2006-01-02"); dataDate != want {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: payload describes %s, refusing to label it %s (mis-dated payload)", dataDate, want)
	}

	// Row 0 = "1.一般股票", column 1 = 成交金額(元)
	row := marketTable.Data[0]
	if len(row) < 2 {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: insufficient columns")
	}

	amountYuan := parseTWSEFloat(row[1])
	if amountYuan <= 0 {
		return nil, fmt.Errorf("TWSE MI_INDEX market stats: non-positive amount: %.0f", amountYuan)
	}

	// Convert 元 → 億元
	amountYi := amountYuan / 100_000_000

	return &MarketVolumeResult{
		MarketVolume: amountYi,
		// YYYYMMDD of the payload's OWN date (== the requested date, verified
		// above). numeric value is untouched by E29-2: only the provenance of
		// the date changed, verified bit-for-bit for 20260924.
		Date: strings.ReplaceAll(dataDate, "-", ""),
	}, nil
}
