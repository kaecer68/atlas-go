package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// FinMind API client for Taiwan stock data.
// NOTE: FinMind requires API key rotation every 7 days.
// This client is intended as a manual backup / fallback only.
// For production, prefer Fugle (real-time) or TWSE OpenAPI (free, no key).
// To rotate key: update FINMIND_API_KEY in .env and restart the service.
const (
	finmindBaseURL          = constants.FinMindBaseURL
	finmindRateLimitFree    = 600  // 600 requests per hour for the free tier
	finmindRateLimitSponsor = 6000 // Sponsor tier: 6000/hr (2026-08-30 upgrade)
	finmindBurst            = 60
)

// finmindRateLimitPerHour returns the LOCAL FinMind request budget per hour.
// The free tier allows 600/hr; the Sponsor tier (2026-08-30 upgrade,
// issue #1742) allows 6000/hr. The local limiter must match the ACTIVE tier —
// after upgrading, self-throttling at free-tier speed left ~90% of the paid
// quota unusable and made the auto_cycle_update startup stampede time out
// ("rate limited" → "no valid data for industry"). Override with the
// FINMIND_RATE_LIMIT_PER_HOUR env var; unset falls back to the free tier.
func finmindRateLimitPerHour() int {
	if v := strings.TrimSpace(os.Getenv("FINMIND_RATE_LIMIT_PER_HOUR")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return finmindRateLimitFree
}

// newFinMindRateLimiter builds the shared rate limiter for the configured
// hourly budget, with a burst of rate/10 bounded to [60, 300] — enough to
// absorb startup stampedes (auto_cycle_update aggregates many symbols at
// once) without risking a 402 from the upstream quota.
func newFinMindRateLimiter() *rate.Limiter {
	perHour := finmindRateLimitPerHour()
	burst := perHour / 10
	if burst < finmindBurst {
		burst = finmindBurst
	}
	if burst > 300 {
		burst = 300
	}
	return rate.NewLimiter(rate.Limit(float64(perHour)/3600.0), burst)
}

// finmindDailyLimit is the observed upstream daily quota cap (used=14400,
// remaining=0 exhaustion on 2026-09-02). NOTE: this account is a paid
// sponsorship (active until ~2026-10-03), hourly budget 6000/hr via
// FINMIND_RATE_LIMIT_PER_HOUR (see finmindRateLimitPerHour) — the hourly
// limiter paces throughput while this daily tracker bounds total spend.
// When the sponsorship lapses, upstream reverts to free-tier limits and
// both values must be revisited. We track this with DailyQuotaTracker so concurrent
// callers (auto_cycle_update, auto_quote_backfill, channel_health_finmind,
// tsmc_revenue, ad-hoc lookups) don't collectively exceed it. Without the
// tracker, cold-start backfill of N symbols × 90 days can blow the daily
// quota in a single scheduled run, leaving the channel dead for the rest of
// the day (regression: commit 35642c13 switched auto_quote_backfill from Fugle
// to FinMind, multiplying the call volume against this single channel).
const finmindDailyLimit = 14400

// ErrQuotaExhausted is returned by fetchDataset when the daily quota is gone.
// Callers should treat this as a transient, scheduled-skippable condition —
// distinct from API auth/quota errors that need human intervention. The
// channel adapter maps this to a "warn" status (not "error") so on-call
// doesn't get paged just because the daily budget ran out.
var ErrQuotaExhausted = fmt.Errorf("finmind: daily quota exhausted")

type FinMindClient struct {
	apiKey       string
	httpClient   *http.Client
	baseURL      string // overridable for tests; defaults to finmindBaseURL
	rateLimiter  *rate.Limiter
	quotaTracker *DailyQuotaTracker
	// retryCfg is the shared fetchWithRetry policy (P0-5). Before P0-5 the
	// main fetchDataset path had NO retry at all — every 5xx/429 failed
	// immediately and the next scheduled cycle repeated the failure.
	retryCfg retryConfig
	// breaker is the client-level circuit breaker (P1-7). All call sites
	// funnel through fetchDataset, so one breaker covers every FinMind
	// consumer (gateway channel, auto_quote_backfill, TSMC revenue,
	// hybrid fallback). Quota-exhaustion and no-data conditions do NOT
	// trip it — they are budget/holiday conditions, not outages.
	breaker *providerBreaker
	// ipBanUntilSec is the unix timestamp until which FinMind has banned
	// this client's outbound IP (observed 2026-09-06: HTTP 403
	// {"msg":"ip banned","retry_after":971} after multi-process cron
	// containers collectively exceeded the per-IP rate on the sponsor
	// token). While set, fetchDataset short-circuits without an HTTP call
	// so the ban window is respected instead of hammering a closed door
	// and tripping the breaker. 0 = no ban. Atomic: fetchDataset is called
	// concurrently by every FinMind consumer.
	ipBanUntilSec atomic.Int64
}

// ErrIPBanned is returned when FinMind has rate-banned this client's
// outbound IP (HTTP 403 body {"msg":"ip banned","retry_after":N}). Like
// ErrQuotaExhausted this is a transient upstream throttling condition that
// self-heals after retry_after — NOT an outage — so callers (gateway
// channel adapter, HealthCheck) map it to warn/waiting rather than error.
var ErrIPBanned = fmt.Errorf("finmind: ip banned by upstream (rate limit)")

// finmindIPBanDefaultRetryAfterSec is the fallback ban window when the 403
// body carries no parseable retry_after. Slightly above the observed 971s
// so a mis-parse never unblocks early.
const finmindIPBanDefaultRetryAfterSec = 1020

type FinMindResponse struct {
	Msg    string           `json:"msg"`
	Status int              `json:"status"`
	Data   []map[string]any `json:"data"`
}

type StockInfo struct {
	StockID          string `json:"stock_id"`
	StockName        string `json:"stock_name"`
	IndustryCategory string `json:"industry_category"`
	Type             string `json:"type"`
}

var (
	sharedFinMindClient     *FinMindClient
	sharedFinMindClientOnce sync.Once
	sharedFinMindClientMu   sync.RWMutex
)

// GetSharedFinMindClient returns a singleton FinMindClient that all components
// share. Using a single client ensures one token bucket enforces the 600 req/hr
// limit across all call sites (gateway channels, TSMC revenue, cycle aggregator).
// The apiKey is used only on first call; subsequent calls ignore it. The
// stateDir is also captured once for the shared DailyQuotaTracker; callers
// that need a different state directory should use NewFinMindClient directly.
func GetSharedFinMindClient(apiKey string, stateDir ...string) *FinMindClient {
	dir := "data/state"
	if len(stateDir) > 0 && stateDir[0] != "" {
		dir = stateDir[0]
	}
	sharedFinMindClientOnce.Do(func() {
		sharedFinMindClient = newFinMindClientInternal(apiKey, dir)
	})
	return sharedFinMindClient
}

// UpdateSharedFinMindAPIKey replaces the API key on the shared client without
// recreating the rate limiter. Use after rotating the FinMind token at runtime.
func UpdateSharedFinMindAPIKey(apiKey string) {
	sharedFinMindClientMu.Lock()
	defer sharedFinMindClientMu.Unlock()
	if sharedFinMindClient != nil {
		sharedFinMindClient.apiKey = apiKey
	}
}

// ResetSharedFinMindClient clears the singleton (for tests).
func ResetSharedFinMindClient() {
	sharedFinMindClientMu.Lock()
	defer sharedFinMindClientMu.Unlock()
	sharedFinMindClient = nil
	sharedFinMindClientOnce = sync.Once{}
}

// NewFinMindClient creates a standalone FinMindClient with its own rate limiter.
// Prefer GetSharedFinMindClient in production to avoid multiple independent
// token buckets that can collectively exceed the free-tier limit.
func NewFinMindClient(apiKey string) *FinMindClient {
	return newFinMindClientInternal(apiKey, "data/state")
}

// SetBaseURL overrides the FinMind API base URL (testing only).
func (c *FinMindClient) SetBaseURL(u string) { c.baseURL = u }

// NewFinMindClientWithStateDir creates a standalone FinMindClient whose
// DailyQuotaTracker persists under the given stateDir instead of the
// default "data/state". Test-only convenience — production callers
// should use GetSharedFinMindClient (which routes through
// newFinMindClientInternal with the configured WorkDir) so the quota
// state file lives next to the other runtime state.
func NewFinMindClientWithStateDir(apiKey, stateDir string) *FinMindClient {
	return newFinMindClientInternal(apiKey, stateDir)
}

// newFinMindClientInternal is the shared constructor used by both the
// singleton accessor and the standalone constructor. It wires the shared
// DailyQuotaTracker so all call sites share one daily counter.
func newFinMindClientInternal(apiKey, stateDir string) *FinMindClient {
	tracker := NewDailyQuotaTracker("finmind", stateDir, finmindDailyLimit)
	// Register the tracker with the global QuotaRegistry so the dashboard's
	// channel-health page and the future /api/dashboard/quota endpoint see
	// FinMind alongside Fugle in one Snapshot() — addressing kaecer's
	// 2026-08-04 feedback to manage FinMind + Fugle together.
	GlobalQuotaRegistry().Register("finmind", tracker)
	return &FinMindClient{
		apiKey:       apiKey,
		baseURL:      finmindBaseURL,
		httpClient:   httpclient.NewFactory().NewClient(30 * time.Second),
		rateLimiter:  newFinMindRateLimiter(),
		quotaTracker: tracker,
		retryCfg:     defaultRetryConfig(),
		breaker:      newProviderBreaker("finmind", defaultCircuitBreakerConfig()),
	}
}
func (c *FinMindClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// SetRateLimiter overrides the rate limiter (tests only; use rate.Inf to disable pacing).
func (c *FinMindClient) SetRateLimiter(limiter *rate.Limiter) {
	c.rateLimiter = limiter
}

// RateLimiter returns the rate limiter for Gateway adapter registration.
func (c *FinMindClient) RateLimiter() *rate.Limiter {
	return c.rateLimiter
}

// QuotaUsed returns the number of FinMind API calls made today (across all
// callers sharing the DailyQuotaTracker). Surfaced by the channel adapter
// health record so the dashboard can warn before the budget runs out.
func (c *FinMindClient) QuotaUsed() int {
	if c.quotaTracker == nil {
		return 0
	}
	return c.quotaTracker.CallsToday()
}

// QuotaRemaining returns the unused portion of today's FinMind budget.
// Returns the full daily limit when no tracker is configured.
func (c *FinMindClient) QuotaRemaining() int {
	if c.quotaTracker == nil {
		return finmindDailyLimit
	}
	return c.quotaTracker.Remaining()
}

// SetQuotaLimit overrides the daily ceiling (e.g., when the FinMind tier
// changes). Delegates to the shared DailyQuotaTracker so the QuotaRegistry
// view updates consistently.
func (c *FinMindClient) SetQuotaLimit(limit int) {
	if c.quotaTracker != nil {
		c.quotaTracker.SetLimit(limit)
	}
}

// FetchDatasetRaw exposes a generic FinMind dataset fetch for providers that
// need datasets without a dedicated typed method (G01 equity dispersion /
// G02 SBL balances). Full-market queries pass an empty dataId — FinMind
// returns every listed symbol for the window. Rate limiting, quota gating,
// retries and the client-level breaker are all inherited from fetchDataset.
func (c *FinMindClient) FetchDatasetRaw(ctx context.Context, dataset string, dataId string, startDate string, endDate string) ([]map[string]any, error) {
	return c.fetchDataset(ctx, dataset, dataId, startDate, endDate)
}

func (c *FinMindClient) fetchDataset(ctx context.Context, dataset string, dataId string, startDate string, endDate string) ([]map[string]any, error) {
	// P1-7: client-level breaker — open 時不發 HTTP，所有 FinMind 消費層
	// 共享同一個 breaker（shared client）。quota exhausted 與 no-data 是
	// 預算/假日條件，不是 outage，不計 failure（見下方 record 點位）。
	if c.breaker != nil && !c.breaker.shouldTry() {
		return nil, fmt.Errorf("finmind: %w", ErrFinMindBreakerOpen)
	}
	// IP-ban gate: while FinMind has banned this client's outbound IP
	// (403 "ip banned" — observed 2026-09-06, retry_after 971s), do NOT
	// spend the rate-limiter budget or the daily quota on requests that
	// are guaranteed to bounce. Short-circuit as a typed throttling
	// condition so every consumer (gateway channel, cron backfills,
	// tsmc_revenue) waits out the ban instead of hammering the closed
	// door and tripping the breaker. Like quota exhaustion this is a
	// throttling condition — record breaker success, not failure.
	if until := c.ipBanUntilSec.Load(); until > time.Now().Unix() {
		c.breakerRecordSuccess()
		return nil, fmt.Errorf("finmind: %w (unblocks in %ds)", ErrIPBanned, until-time.Now().Unix())
	}
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("finmind: rate limit wait: %w", ErrRateLimited)
	}
	// Daily-quota gate: every call site funnels through fetchDataset, so a
	// single AllowCall check protects the whole channel from cold-start
	// bursts (commit 35642c13 switched auto_quote_backfill to FinMind, which
	// can hit 1000s of calls per cycle without this gate). When the daily
	// budget is gone we return ErrQuotaExhausted rather than letting the
	// HTTP request fail with a misleading 400 status.
	// P1-7: quota exhaustion is a budget condition (auto-resets at 00:00 TW)
	// — it must NOT trip the breaker, so we reset instead of recording a
	// failure.
	if c.quotaTracker != nil && !c.quotaTracker.AllowCall() {
		c.breakerRecordSuccess()
		return nil, fmt.Errorf("finmind: %w (used=%d, remaining=%d)", ErrQuotaExhausted, c.quotaTracker.CallsToday(), c.quotaTracker.Remaining())
	}

	endpoint := fmt.Sprintf("%s/data", c.baseURL)
	params := url.Values{}
	params.Set("dataset", dataset)
	params.Set("data_id", normalizeFinMindStockID(dataId))
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("finmind: create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	// P0-5: shared fetchWithRetry — 429/5xx retried with Retry-After /
	// exponential backoff (previously no retry on the main data path).
	resp, err := fetchWithRetry(ctx, c.httpClient, req, c.retryCfg)
	if err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Capture response body on non-200 so channel_health surfaces the real
	// reason. Without this, the FinMind API's "Token is illegal" / "no
	// data" / "rate limit exceeded" messages get dropped, and operators
	// only see "finmind: status 400" — which forced hermes + this agent to
	// debug in circles before realising the real issue (FINMIND_API_KEY
	// env mismatch, NOT quota exhaustion, on 2026-08-04).
	// Limit read to 512 bytes: enough for FinMind's JSON error envelope,
	// bounded so a malicious / oversized body can't blow memory.
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		if bodyStr == "" {
			bodyStr = "(empty body)"
		}
		logging.Warn("finmind", "fetch_non_2xx",
			"status", resp.StatusCode,
			"body", bodyStr,
			"dataset", dataset,
			"data_id", dataId,
		)
		// P0-1 (provider-resilience): a server-side 402 IS the FinMind quota
		// signal ("Requests reach the upper limit" — free-tier daily cap).
		// Wrap ErrQuotaExhausted so errors.Is at the adapter/industry layer
		// maps it to warn/quotas instead of a plain "status 402" string that
		// paged on-call. Previously only the LOCAL daily-quota gate wrapped
		// the sentinel; the server-side 402 fell through to the generic
		// status error below, so channel-health reported "error" for a
		// budget condition that auto-resets at 00:00 TW.
		// P1-7: 402 is the server-side quota signal — a budget condition, not
		// an outage; do NOT trip the breaker (same rule as the local gate).
		if resp.StatusCode == http.StatusPaymentRequired {
			c.breakerRecordSuccess()
			return nil, fmt.Errorf("finmind: %w: %s", ErrQuotaExhausted, bodyStr)
		}
		// 403 "ip banned" is FinMind's per-IP rate-limit signal — a
		// throttling condition that self-heals after retry_after, NOT an
		// outage. Record the ban window (fetchDataset short-circuits for
		// its duration) and map to ErrIPBanned so the gateway / adapter
		// layers treat it like quota exhaustion (warn), not a hard
		// failure (error alert + breaker trip). Observed in production
		// 2026-09-06 04:10Z after multi-process cron containers
		// collectively exceeded the per-IP rate: the untyped 403 marked
		// the finmind channel error for ~30m and fired
		// ChannelHealthStatusError even though the ban self-healed.
		if resp.StatusCode == http.StatusForbidden && strings.Contains(bodyStr, "ip banned") {
			retryAfter := finmindIPBanDefaultRetryAfterSec
			var banBody struct {
				RetryAfter int `json:"retry_after"`
			}
			if err := json.Unmarshal([]byte(bodyStr), &banBody); err == nil && banBody.RetryAfter > 0 {
				retryAfter = banBody.RetryAfter
			}
			c.ipBanUntilSec.Store(time.Now().Add(time.Duration(retryAfter) * time.Second).Unix())
			c.breakerRecordSuccess()
			logging.Warn("finmind", "ip_banned_short_circuit",
				"retry_after_sec", retryAfter,
				"dataset", dataset,
			)
			return nil, fmt.Errorf("finmind: %w (retry_after=%ds): %s", ErrIPBanned, retryAfter, bodyStr)
		}
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: status %d, body: %s", resp.StatusCode, bodyStr)
	}

	var finmindResp FinMindResponse
	if err := json.NewDecoder(resp.Body).Decode(&finmindResp); err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: decode response: %w", err)
	}

	if finmindResp.Status != 200 {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: API error: %s", finmindResp.Msg)
	}

	// Free-tier throttling masquerades as success: FinMind answers HTTP 200
	// with a non-"success" msg (e.g. "Your level is free. Please update
	// your user level.") and an EMPTY data array when the request budget is
	// exhausted. Callers that walk dates backward (tdcc/sbl history probes)
	// would otherwise burn their remaining budget re-probing empty days
	// (observed 2026-09-02: tdcc channel "no dispersion data" after the
	// backfill cursor + scheduled tasks shared the free quota). Classify it
	// as a quota condition — a budget state that resets, not an outage.
	if finmindResp.Msg != "" && finmindResp.Msg != "success" && len(finmindResp.Data) == 0 {
		c.breakerRecordSuccess()
		return nil, fmt.Errorf("finmind: %w: %s", ErrQuotaExhausted, finmindResp.Msg)
	}

	// P2-15: response schema fingerprint — warn the moment the upstream
	// renames/drops a field this client depends on, instead of surfacing
	// later as an obscure type-assertion error in the dataset callers.
	// Envelope shape + first data row against the dataset's required fields.
	warnFingerprint(finmindEnvelopeFingerprint, map[string]any{
		"msg":    finmindResp.Msg,
		"status": finmindResp.Status,
		"data":   finmindResp.Data,
	})
	warnFinMindDatasetFingerprint(dataset, finmindResp.Data)

	c.breakerRecordSuccess()
	return finmindResp.Data, nil
}

// ErrFinMindBreakerOpen is returned by fetchDataset when the client-level
// circuit breaker is open. All FinMind consumers share the singleton client,
// so an open breaker short-circuits the whole channel until the recovery
// timeout elapses.
var ErrFinMindBreakerOpen = fmt.Errorf("finmind: circuit breaker open")

// breakerRecordSuccess / breakerRecordFailure are nil-safe breaker wrappers
// (hand-constructed FinMindClient values in tests may have a nil breaker).
// P1-7 semantics: quota exhaustion and no-data DO NOT count as failures.
func (c *FinMindClient) breakerRecordSuccess() {
	if c.breaker != nil {
		c.breaker.recordSuccess()
	}
}

func (c *FinMindClient) breakerRecordFailure() {
	if c.breaker != nil {
		c.breaker.recordFailure()
	}
}

// BreakerInfo exposes the breaker state for tests and observability.
func (c *FinMindClient) BreakerInfo() ProviderBreakerInfo {
	if c.breaker == nil {
		return ProviderBreakerInfo{Name: "finmind", State: ProviderCircuitClosed}
	}
	return c.breaker.stateSnapshot()
}

// normalizeFinMindStockID 將 FinMind Taiwan stock dataset 的 data_id 正規化為
// 裸股票代碼（剝離 .TW / .TWO suffix，大小寫不敏感）。
//
// A01 修復（2026-08-10 audit）：ClassificationTree 的 RepresentativeStocks
// 使用 "1513.TW" 形式，而 FinMind API 只接受裸碼 "1513"。在 fetchDataset
// 統一正規化，讓所有 caller（auto_cycle_update、ODM provider、TSMC revenue、
// quote backfill、dividend）共用同一契約，避免每個呼叫端各自 trim 造成分叉。
// 對非股票 data_id（例如 "TSE_DAYTRADE"、ETF 代碼 "0050"）是 no-op。
func normalizeFinMindStockID(dataID string) string {
	id := strings.TrimSpace(dataID)
	if len(id) > 3 && strings.EqualFold(id[len(id)-3:], ".TW") {
		id = id[:len(id)-3]
	} else if len(id) > 4 && strings.EqualFold(id[len(id)-4:], ".TWO") {
		id = id[:len(id)-4]
	}
	return id
}

// quarterOfDate 回傳 dateStr（YYYY-MM-DD）對應的季度 (1-4)。解析失敗回傳 0。
func quarterOfDate(dateStr string) int {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	return (int(t.Month())-1)/3 + 1
}

// lastDayOfMonth returns the last calendar day of (year, month) — 28/29/30/31
// depending on the month and leap year. Uses Go's time.Date normalisation:
// time.Date(y, m+1, 0, ...) is the idiomatic way to get the last day of
// month m in year y.
//
// PR-E (kaecer 2026-08-05 dispatch). This replaces the previous hardcoded
// "31" in endDate construction, which caused FinMind to return
// "parameter YYYY-MM-31 is illegal" errors for any month with 30 or
// fewer days (Feb/Apr/Jun/Sep/Nov). The 80+ day auto_cycle_update stale
// issue documented in v3.0 §A 問題 5 is rooted in this bug, not in
// upstream TWSE as the v3.0 report assumed. See post-restart-e2e-
// verification-2026-08-05.md §3 for the full log evidence.
func lastDayOfMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (c *FinMindClient) GetMonthRevenue(ctx context.Context, symbol string, year int, month int) (float64, error) {
	startDate := fmt.Sprintf("%d-%02d-01", year, month)
	endDate := fmt.Sprintf("%d-%02d-%02d", year, month, lastDayOfMonth(year, time.Month(month)))

	data, err := c.fetchDataset(ctx, "TaiwanStockMonthRevenue", symbol, startDate, endDate)
	if err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, fmt.Errorf("finmind: no month revenue data for %s %d-%02d", symbol, year, month)
	}

	revenue, ok := data[0]["revenue"].(float64)
	if !ok {
		return 0, fmt.Errorf("finmind: cannot parse revenue from response")
	}

	return revenue, nil
}

func (c *FinMindClient) GetFinancialStatements(ctx context.Context, symbol string, year int, quarter int) (map[string]float64, error) {
	startDate := fmt.Sprintf("%d-01-01", year)
	endDate := fmt.Sprintf("%d-12-%02d", year, lastDayOfMonth(year, time.December))

	data, err := c.fetchDataset(ctx, "TaiwanStockFinancialStatements", symbol, startDate, endDate)
	if err != nil {
		return nil, err
	}

	result := make(map[string]float64)
	for _, item := range data {
		dateStr, ok := item["date"].(string)
		if !ok {
			continue
		}
		// A02 修復：quarter 由完整日期計算（3月→Q1、6月→Q2、9月→Q3、
		// 12月→Q4），取代舊的 dateStr[5]（月份十位數）錯誤 heuristic —
		// 舊邏輯把 2026-12-31 判成 Q1、2026-03-31 判成 Q0。
		if q := quarterOfDate(dateStr); q == quarter {
			if val, ok := item["value"].(float64); ok {
				originName, _ := item["origin_name"].(string)
				result[originName] = val
			}
		}
	}

	return result, nil
}

func (c *FinMindClient) GetInstitutionalInvestors(ctx context.Context, symbol string, date string) (foreign, domestic, dealer float64, err error) {
	data, err := c.fetchDataset(ctx, "TaiwanStockInstitutionalInvestorsBuySell", symbol, date, date)
	if err != nil {
		return 0, 0, 0, err
	}

	for _, item := range data {
		name, ok := item["name"].(string)
		if !ok {
			continue
		}
		buy, _ := item["buy"].(float64)
		sell, _ := item["sell"].(float64)
		net := buy - sell

		switch name {
		case "ForeignInvestors", "ForeignDealer":
			foreign += net
		case "InvestmentTrust", "DomesticInstitution":
			domestic += net
		case "Dealer":
			dealer += net
		}
	}

	return foreign, domestic, dealer, nil
}

// GetMarginMaintenanceLatest fetches the most recent whole-market margin
// maintenance ratio (%) from the FinMind TaiwanTotalExchangeMarginMaintenance
// dataset at or before endDate. The dataset is a market-wide daily series
// with no data_id, so one API call returns the whole window — this keeps the
// live-fill cost at ~1 FinMind call per ingest regardless of weekends or
// holidays. endDate is "YYYY-MM-DD"; the lookback window is 14 days so the
// latest published trading day is always covered.
// Returns (rowDate, ratio, nil) for the latest row, or ErrNoData when the
// window contains nothing (FinMind releases the ratio after TWSE's evening
// processing, so same-day morning queries legitimately come back empty).
func (c *FinMindClient) GetMarginMaintenanceLatest(ctx context.Context, endDate string) (string, float64, error) {
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return "", 0, fmt.Errorf("finmind: parse endDate %q: %w", endDate, err)
	}
	start := end.AddDate(0, 0, -14).Format("2006-01-02")

	data, err := c.fetchDataset(ctx, "TaiwanTotalExchangeMarginMaintenance", "", start, endDate)
	if err != nil {
		return "", 0, err
	}
	if len(data) == 0 {
		return "", 0, fmt.Errorf("finmind: %w: no margin maintenance ratio up to %s", ErrNoData, endDate)
	}

	// Rows are date-ascending from FinMind; take the latest parseable row.
	lastDate := ""
	ratio := 0.0
	for _, item := range data {
		d, _ := item["date"].(string)
		v, ok := item["TotalExchangeMarginMaintenance"].(float64)
		if d == "" || !ok {
			continue
		}
		lastDate, ratio = d, v
	}
	if lastDate == "" {
		return "", 0, fmt.Errorf("finmind: margin maintenance ratio field missing up to %s", endDate)
	}
	return lastDate, ratio, nil
}

func (c *FinMindClient) GetStockPrice(ctx context.Context, symbol string, date string) (domain.Quote, error) {
	data, err := c.fetchDataset(ctx, "TaiwanStockPrice", symbol, date, date)
	if err != nil {
		return domain.Quote{}, err
	}

	if len(data) == 0 {
		return domain.Quote{}, fmt.Errorf("finmind: no price data for %s on %s", symbol, date)
	}

	item := data[0]
	quote := domain.Quote{
		Symbol: symbol,
		Market: "TW",
		AsOf:   time.Now(),
		Source: "finmind",
	}

	if v, ok := item["close"].(float64); ok {
		quote.Last = v
		quote.High = v
		quote.Low = v
	}
	if v, ok := item["open"].(float64); ok {
		quote.Open = v
	}
	if v, ok := item["max"].(float64); ok {
		quote.High = v
	}
	if v, ok := item["min"].(float64); ok {
		quote.Low = v
	}
	if v, ok := item["Trading_Volume"].(float64); ok {
		quote.Volume = int64(v)
	}

	return quote, nil
}

func parseStockInfo(item map[string]any) (StockInfo, error) {
	var info StockInfo

	stockID, ok := item["stock_id"].(string)
	if !ok {
		return info, fmt.Errorf("finmind: missing stock_id in TaiwanStockInfo")
	}
	info.StockID = stockID

	stockName, _ := item["stock_name"].(string)
	info.StockName = stockName

	industryCategory, _ := item["industry_category"].(string)
	info.IndustryCategory = industryCategory

	stockType, _ := item["type"].(string)
	info.Type = stockType

	return info, nil
}

func (c *FinMindClient) GetStockInfo(ctx context.Context) ([]StockInfo, error) {
	data, err := c.fetchDataset(ctx, "TaiwanStockInfo", "", "", "")
	if err != nil {
		return nil, fmt.Errorf("finmind: TaiwanStockInfo: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("finmind: TaiwanStockInfo returned empty data")
	}

	infos := make([]StockInfo, 0, len(data))
	for _, item := range data {
		info, err := parseStockInfo(item)
		if err != nil {
			logging.Warn("finmind", "parse_stock_info_failed", logging.Err(err))
			continue
		}
		infos = append(infos, info)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("finmind: TaiwanStockInfo: all items failed to parse")
	}

	return infos, nil
}

type FinMindProvider struct {
	client *FinMindClient
}

func NewFinMindProviderWithClient(client *FinMindClient) *FinMindProvider {
	return &FinMindProvider{client: client}
}

func NewFinMindProvider(apiKey string) *FinMindProvider {
	return &FinMindProvider{client: GetSharedFinMindClient(apiKey)}
}

func (p *FinMindProvider) Name() string {
	return "finmind"
}

// isTaiwanTradingDay 已移至 calendar.go（B05：含國定假日判定）。
// 定義位置：internal/marketdata/calendar.go。

func (p *FinMindProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	if !isTaiwanTradingDay(asOf) {
		return nil, fmt.Errorf("finmind: asOf %s is not a Taiwan trading day (weekend or holiday)", asOf.Format("2006-01-02"))
	}
	date := asOf.Format("2006-01-02")
	quotes := make([]domain.Quote, 0, len(symbols))

	var lastErr error
	for _, symbol := range symbols {
		quote, err := p.client.GetStockPrice(ctx, symbol, date)
		if err != nil {
			logging.Error("finmind", "fetch_failed", "symbol", symbol, logging.Err(err))
			lastErr = err
			continue
		}
		quotes = append(quotes, quote)
	}

	if len(quotes) == 0 && lastErr != nil {
		return nil, fmt.Errorf("finmind: all symbols failed: %w", lastErr)
	}
	return quotes, nil
}

func (p *FinMindProvider) GetClient() *FinMindClient {
	return p.client
}

func (p *FinMindProvider) GetMonthRevenue(ctx context.Context, symbol string, year int, month int) (float64, error) {
	return p.client.GetMonthRevenue(ctx, symbol, year, month)
}

func (p *FinMindProvider) GetFinancialStatements(ctx context.Context, symbol string, year int, quarter int) (map[string]float64, error) {
	return p.client.GetFinancialStatements(ctx, symbol, year, quarter)
}

func (p *FinMindProvider) GetInstitutionalInvestors(ctx context.Context, symbol string, date string) (float64, float64, float64, error) {
	return p.client.GetInstitutionalInvestors(ctx, symbol, date)
}
