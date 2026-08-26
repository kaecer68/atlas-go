package marketdata

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// ErrTAIFEXSchema is returned when a TAIFEX response cannot be parsed —
// a required field is missing or non-numeric (upstream schema change /
// renamed column). Mirrors the twse_etf / fubon_etf typed-error convention
// (P0-3): previously parseInt64/parseFloat64 silently returned 0, so a
// column rename produced "0 data, nil error" and the channel looked healthy.
var ErrTAIFEXSchema = fmt.Errorf("taifex: schema mismatch: %w", ErrSchema)

// PCRStats holds the put/call ratio data from TAIFEX.
type PCRStats struct {
	Date               string  `json:"date"`
	PutVolume          int64   `json:"put_volume"`
	CallVolume         int64   `json:"call_volume"`
	PutCallVolumeRatio float64 `json:"put_call_volume_ratio"`
	PutOI              int64   `json:"put_oi"`
	CallOI             int64   `json:"call_oi"`
	PutCallOIRatio     float64 `json:"put_call_oi_ratio"`
}

// RetailFuturesOI holds the retail trader open interest breakdown for TX futures.
// Retail OI is derived from total market OI minus Top10 large-trader positions.
type RetailFuturesOI struct {
	Date           string  `json:"date"`
	Top5LongOI     int64   `json:"top5_long_oi"`
	Top5ShortOI    int64   `json:"top5_short_oi"`
	Top10LongOI    int64   `json:"top10_long_oi"`
	Top10ShortOI   int64   `json:"top10_short_oi"`
	TotalMarketOI  int64   `json:"total_market_oi"`
	RetailLongOI   int64   `json:"retail_long_oi"`
	RetailShortOI  int64   `json:"retail_short_oi"`
	RetailLongPct  float64 `json:"retail_long_pct"`
	RetailShortPct float64 `json:"retail_short_pct"`
}

// TAIFEXProvider fetches derivatives market data from the TAIFEX OpenAPI.
// Data is free and requires no API key.
type TAIFEXProvider struct {
	client      *http.Client
	baseURL     string
	rateLimiter *rate.Limiter
	// retryCfg is the shared fetchWithRetry policy (P0-5). TAIFEX had no
	// retry and no breaker — a 5xx during the 30s upstream window failed
	// the whole cycle.
	retryCfg retryConfig
	// breaker is the provider-level circuit breaker (P1-7). Empty-list
	// (no-data) responses do NOT trip it; transport/HTTP/schema failures do.
	breaker *providerBreaker
}

// NewTAIFEXProvider creates a new TAIFEX data provider.
func NewTAIFEXProvider() *TAIFEXProvider {
	return &TAIFEXProvider{
		client:      httpclient.NewFactory().NewClient(30 * time.Second), // P1 B: upstream can exceed 20s under load
		baseURL:     "https://openapi.taifex.com.tw/v1",
		rateLimiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
		retryCfg:    defaultRetryConfig(),
		breaker:     newProviderBreaker("taifex", defaultCircuitBreakerConfig()),
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (t *TAIFEXProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		t.client = client
	}
}

// Name returns the provider name.
func (t *TAIFEXProvider) Name() string {
	return "taifex"
}

// breakerRecordSuccess / breakerRecordFailure are nil-safe breaker wrappers
// (hand-constructed TAIFEXProvider values in tests may have a nil breaker).
func (t *TAIFEXProvider) breakerRecordSuccess() {
	if t.breaker != nil {
		t.breaker.recordSuccess()
	}
}

func (t *TAIFEXProvider) breakerRecordFailure() {
	if t.breaker != nil {
		t.breaker.recordFailure()
	}
}

// BreakerInfo exposes the breaker state for tests and observability.
func (t *TAIFEXProvider) BreakerInfo() ProviderBreakerInfo {
	if t.breaker == nil {
		return ProviderBreakerInfo{Name: "taifex", State: ProviderCircuitClosed}
	}
	return t.breaker.stateSnapshot()
}

// FetchPCR retrieves the most recent available put/call ratio data.
func (t *TAIFEXProvider) FetchPCR(ctx context.Context) (*PCRStats, error) {
	// P1-7: provider-level breaker.
	if t.breaker != nil && !t.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: taifex circuit breaker open", ErrUpstream)
	}
	if err := WaitForLimiter(ctx, t.rateLimiter); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := t.baseURL + "/PutCallRatio"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	// P0-5: shared fetchWithRetry — 429/5xx retried (TAIFEX previously
	// failed the whole cycle on the first transient 5xx).
	resp, err := fetchWithRetry(ctx, t.client, req, t.retryCfg)
	if err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("taifex pcr http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readTAIFEXBody(resp)
	if err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("read pcr body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("taifex pcr api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	// P2-15: response schema fingerprint — warn with the exact missing key
	// when the raw PutCallRatio rows no longer match the expected shape. The
	// struct-level parseOK checks below remain the hard gate (ErrTAIFEXSchema
	// + breaker trip); the fingerprint is the early warn-only canary.
	warnTAIFEXPCRFingerprint(body, resp.Header.Get("Content-Type"))

	var rawList []taifexPCRRaw
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &rawList); err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("decode pcr response: %w", err)
	}

	if len(rawList) == 0 {
		// P1-7: no-data (holiday / not yet published) — not a failure.
		t.breakerRecordSuccess()
		return nil, fmt.Errorf("taifex pcr api returned empty list")
	}

	// P2-16: pick the LATEST row by Date instead of rawList[0]. The upstream
	// order is not a documented contract — a sorting change would silently
	// serve stale PCR data. Same latestDate pattern as FetchRetailFuturesOI
	// (and FetchFutures).
	var latest *taifexPCRRaw
	var latestDate string
	for i := range rawList {
		r := &rawList[i]
		if r.Date > latestDate {
			latestDate = r.Date
			latest = r
		}
	}
	if latest == nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: every PutCallRatio row has an empty Date", ErrTAIFEXSchema)
	}
	raw := latest
	putVolume, ok := parseInt64OK(raw.PutVolume)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: PutVolume=%q not parseable", ErrTAIFEXSchema, raw.PutVolume)
	}
	callVolume, ok := parseInt64OK(raw.CallVolume)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: CallVolume=%q not parseable", ErrTAIFEXSchema, raw.CallVolume)
	}
	putCallVolumeRatioPct, ok := parseFloat64OK(raw.PutCallVolumeRatioPct)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: PutCallVolumeRatio%%=%q not parseable", ErrTAIFEXSchema, raw.PutCallVolumeRatioPct)
	}
	putOI, ok := parseInt64OK(raw.PutOI)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: PutOI=%q not parseable", ErrTAIFEXSchema, raw.PutOI)
	}
	callOI, ok := parseInt64OK(raw.CallOI)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: CallOI=%q not parseable", ErrTAIFEXSchema, raw.CallOI)
	}
	putCallOIRatioPct, ok := parseFloat64OK(raw.PutCallOIRatioPct)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: PutCallOIRatio%%=%q not parseable", ErrTAIFEXSchema, raw.PutCallOIRatioPct)
	}

	stats := &PCRStats{
		Date:               raw.Date,
		PutVolume:          putVolume,
		CallVolume:         callVolume,
		PutCallVolumeRatio: putCallVolumeRatioPct / 100.0,
		PutOI:              putOI,
		CallOI:             callOI,
		PutCallOIRatio:     putCallOIRatioPct / 100.0,
	}

	t.breakerRecordSuccess()
	return stats, nil
}

// FetchRetailFuturesOI retrieves retail trader open interest for TX futures.
// Uses the large trader data to derive retail (small trader) share:
//
//	retail = total market OI - top10 large trader OI
func (t *TAIFEXProvider) FetchRetailFuturesOI(ctx context.Context) (*RetailFuturesOI, error) {
	// P1-7: provider-level breaker.
	if t.breaker != nil && !t.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: taifex circuit breaker open", ErrUpstream)
	}
	if err := WaitForLimiter(ctx, t.rateLimiter); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := t.baseURL + "/OpenInterestOfLargeTradersFutures"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	// P0-5: shared fetchWithRetry — 429/5xx retried (TAIFEX previously
	// failed the whole cycle on the first transient 5xx).
	resp, err := fetchWithRetry(ctx, t.client, req, t.retryCfg)
	if err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("taifex large trader http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readTAIFEXBody(resp)
	if err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("read large trader body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("taifex large trader api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var rawList []taifexLargeTraderRaw
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &rawList); err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("decode large trader response: %w", err)
	}

	// Find the latest TX futures all-months record for all traders (TypeOfTraders="0").
	// SettlementMonth "999912" = all months combined; "0" = all trader types.
	var latest *taifexLargeTraderRaw
	var latestDate string
	for i := range rawList {
		r := &rawList[i]
		if r.Contract != "TX" || r.SettlementMonth != "999912" || r.TypeOfTraders != "0" {
			continue
		}
		if r.Date > latestDate {
			latestDate = r.Date
			latest = r
		}
	}

	if latest == nil {
		// P1-7: no matching record (holiday / report not yet published) —
		// not a failure.
		t.breakerRecordSuccess()
		return nil, fmt.Errorf("taifex large trader api: no TX all-months record found")
	}

	if latest.Date == "" {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: missing Date in large-trader record", ErrTAIFEXSchema)
	}
	top5Long, ok := parseInt64OK(latest.Top5Buy)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Top5Buy=%q not parseable", ErrTAIFEXSchema, latest.Top5Buy)
	}
	top5Short, ok := parseInt64OK(latest.Top5Sell)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Top5Sell=%q not parseable", ErrTAIFEXSchema, latest.Top5Sell)
	}
	top10Long, ok := parseInt64OK(latest.Top10Buy)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Top10Buy=%q not parseable", ErrTAIFEXSchema, latest.Top10Buy)
	}
	top10Short, ok := parseInt64OK(latest.Top10Sell)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Top10Sell=%q not parseable", ErrTAIFEXSchema, latest.Top10Sell)
	}
	totalOI, ok := parseInt64OK(latest.OIOfMarket)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: OIOfMarket=%q not parseable", ErrTAIFEXSchema, latest.OIOfMarket)
	}

	retailLong := totalOI - top10Long
	retailShort := totalOI - top10Short
	if retailLong < 0 {
		retailLong = 0
	}
	if retailShort < 0 {
		retailShort = 0
	}

	retailLongPct := safePercent(retailLong, totalOI)
	retailShortPct := safePercent(retailShort, totalOI)

	t.breakerRecordSuccess()
	return &RetailFuturesOI{
		Date:           latest.Date,
		Top5LongOI:     top5Long,
		Top5ShortOI:    top5Short,
		Top10LongOI:    top10Long,
		Top10ShortOI:   top10Short,
		TotalMarketOI:  totalOI,
		RetailLongOI:   retailLong,
		RetailShortOI:  retailShort,
		RetailLongPct:  retailLongPct,
		RetailShortPct: retailShortPct,
	}, nil
}

// --- raw API response types ---

type taifexPCRRaw struct {
	Date                  string `json:"Date"`
	PutVolume             string `json:"PutVolume"`
	CallVolume            string `json:"CallVolume"`
	PutCallVolumeRatioPct string `json:"PutCallVolumeRatio%"`
	PutOI                 string `json:"PutOI"`
	CallOI                string `json:"CallOI"`
	PutCallOIRatioPct     string `json:"PutCallOIRatio%"`
}

type taifexLargeTraderRaw struct {
	Date            string `json:"Date"`
	Contract        string `json:"Contract"`
	ContractName    string `json:"ContractName"`
	SettlementMonth string `json:"SettlementMonth"`
	TypeOfTraders   string `json:"TypeOfTraders"`
	Top5Buy         string `json:"Top5Buy"`
	Top5Sell        string `json:"Top5Sell"`
	Top10Buy        string `json:"Top10Buy"`
	Top10Sell       string `json:"Top10Sell"`
	OIOfMarket      string `json:"OIOfMarket"`
}

// --- shared helpers ---

// parseInt64OK parses s as an int64, returning ok=false when the field is
// empty or malformed. P0-3: callers must check ok and surface a typed
// ErrTAIFEXSchema error instead of silently treating a schema change as 0.
func parseInt64OK(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseFloat64OK parses s as a float64, returning ok=false when the field
// is empty or malformed. P0-3: same contract as parseInt64OK.
func parseFloat64OK(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseInt64 / parseFloat64 are the legacy silent-zero helpers. They are
// kept for callers that genuinely tolerate a missing field (provider_names
// tests pin the 0-on-empty behavior); new TAIFEX parsing must use the OK
// variants and reject schema gaps with ErrTAIFEXSchema.
func parseInt64(s string) int64 {
	v, _ := parseInt64OK(s)
	return v
}

func parseFloat64(s string) float64 {
	v, _ := parseFloat64OK(s)
	return v
}

func safePercent(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// utf8BOM is the byte order mark prefix that some upstreams (including
// occasional TAIFEX responses) prepend to UTF-8 text. It must be removed
// before handing the body to the JSON decoder.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripBOM removes a leading UTF-8 BOM from b. It returns b unchanged if no
// BOM is present. A BOM in the byte stream becomes the invalid character
// 'ï' (U+00EF) when the decoder treats it as part of the JSON text.
func stripBOM(b []byte) []byte {
	if len(b) >= 3 && bytes.Equal(b[:3], utf8BOM) {
		return b[3:]
	}
	return b
}

// readTAIFEXBody reads a TAIFEX response body, transparently decompressing
// gzip content and stripping a leading UTF-8 BOM if present. We request
// Accept-Encoding: gzip explicitly because the openapi.taifex.com.tw
// endpoints return large JSON payloads; Go's Transport only auto-decompresses
// when it adds the header itself, so an explicit header requires explicit
// decompression here.
func readTAIFEXBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return stripBOM(body), nil
}

// TAIFEXFutures holds daily TX futures session data including night session.
type TAIFEXFutures struct {
	Date       string  `json:"date"`
	Open       float64 `json:"open"`
	High       float64 `json:"high"`
	Low        float64 `json:"low"`
	Close      float64 `json:"close"`
	Volume     int64   `json:"volume"`
	Settlement float64 `json:"settlement"`
	ChangePct  float64 `json:"change_pct"`
}

// FetchFutures retrieves the most recent TX futures daily data.
func (t *TAIFEXProvider) FetchFutures(ctx context.Context) (*TAIFEXFutures, error) {
	// P1-7: provider-level breaker.
	if t.breaker != nil && !t.breaker.shouldTry() {
		return nil, fmt.Errorf("%w: taifex circuit breaker open", ErrUpstream)
	}
	if err := WaitForLimiter(ctx, t.rateLimiter); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	url := t.baseURL + "/DailyMarketReportFutures"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")

	// P0-5: shared fetchWithRetry — 429/5xx retried (TAIFEX previously
	// failed the whole cycle on the first transient 5xx).
	resp, err := fetchWithRetry(ctx, t.client, req, t.retryCfg)
	if err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("taifex futures http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readTAIFEXBody(resp)
	if err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("read futures body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("taifex futures api error: status %d, body=%s", resp.StatusCode, string(body))
	}

	var rawList []taifexFuturesRaw
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &rawList); err != nil {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("decode futures response: %w", err)
	}

	var latest *taifexFuturesRaw
	var latestDate string
	for i := range rawList {
		r := &rawList[i]
		if r.Contract != "TX" {
			continue
		}
		if r.Date > latestDate {
			latestDate = r.Date
			latest = r
		}
	}

	if latest == nil {
		// P1-7: no-data — not a failure.
		t.breakerRecordSuccess()
		return nil, fmt.Errorf("taifex futures api: no TX contract found")
	}

	if latest.Date == "" {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: missing Date in futures record", ErrTAIFEXSchema)
	}
	openPrice, ok := parseFloat64OK(latest.Open)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Open=%q not parseable", ErrTAIFEXSchema, latest.Open)
	}
	highPrice, ok := parseFloat64OK(latest.High)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: High=%q not parseable", ErrTAIFEXSchema, latest.High)
	}
	lowPrice, ok := parseFloat64OK(latest.Low)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Low=%q not parseable", ErrTAIFEXSchema, latest.Low)
	}
	prevClose, ok := parseFloat64OK(latest.PreviousSettlementPrice)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: PreviousSettlementPrice=%q not parseable", ErrTAIFEXSchema, latest.PreviousSettlementPrice)
	}
	closePrice, ok := parseFloat64OK(latest.LastPrice)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: LastPrice=%q not parseable", ErrTAIFEXSchema, latest.LastPrice)
	}
	volume, ok := parseInt64OK(latest.Volume)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: Volume=%q not parseable", ErrTAIFEXSchema, latest.Volume)
	}
	settlementPrice, ok := parseFloat64OK(latest.SettlementPrice)
	if !ok {
		t.breakerRecordFailure()
		return nil, fmt.Errorf("%w: SettlementPrice=%q not parseable", ErrTAIFEXSchema, latest.SettlementPrice)
	}

	changePct := 0.0
	if prevClose > 0 {
		changePct = (closePrice - prevClose) / prevClose * 100
	}

	t.breakerRecordSuccess()
	return &TAIFEXFutures{
		Date:       latest.Date,
		Open:       openPrice,
		High:       highPrice,
		Low:        lowPrice,
		Close:      closePrice,
		Volume:     volume,
		Settlement: settlementPrice,
		ChangePct:  changePct,
	}, nil
}

type taifexFuturesRaw struct {
	Date                    string `json:"Date"`
	Contract                string `json:"Contract"`
	Open                    string `json:"Open"`
	High                    string `json:"High"`
	Low                     string `json:"Low"`
	LastPrice               string `json:"LastPrice"`
	Volume                  string `json:"Volume"`
	SettlementPrice         string `json:"SettlementPrice"`
	PreviousSettlementPrice string `json:"PreviousSettlementPrice"`
}
