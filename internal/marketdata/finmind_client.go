package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

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

// finmindObservedUpstreamRefusalLimit is the call count at which FinMind
// ACTUALLY refused this account: on 2026-09-26 02:10Z the shared tracker
// stood at 12,500 calls for the quota day when the upstream answered
// 402 `finmind: daily quota exhausted: {"msg":"Requests reach the upper
// limit. ..."}` on the auto_quote_backfill run (824 symbols).
//
// This is an OBSERVATION, not a budget. The local ceiling must stay strictly
// below it (see finmindDailyLimit) so the platform stops itself instead of
// discovering the wall with a 402, and so every "reserve" derived from the
// ceiling keeps the headroom it claims to keep.
const finmindObservedUpstreamRefusalLimit = 12500

// finmindDailyLimit is the LOCAL daily call ceiling enforced by the shared
// DailyQuotaTracker. We track this so concurrent callers (auto_cycle_update,
// auto_quote_backfill, channel_health_finmind, tsmc_revenue, ad-hoc lookups)
// don't collectively exceed it: without the tracker, a cold-start backfill of
// N symbols × 90 days blows the daily quota in a single scheduled run and
// leaves the channel dead for the rest of the day (regression: commit
// 35642c13 switched auto_quote_backfill from Fugle to FinMind, multiplying
// the call volume against this single channel).
//
// 14400 → 12000 (fix/finmind-quota-honor-402-r, 2026-09-26 production
// evidence): 14400 was inferred from a 2026-09-02 exhaustion (used=14400) and
// was stale — the upstream now refuses at ~12,500. Every stop-loss built on
// 14400 was therefore past the real wall: the auto_quote_backfill floor of
// 1,500 calls only stopped at 12,900 calls, i.e. ~400 calls AFTER FinMind had
// already started answering 402, so the backfill spent its day burning
// guaranteed failures and the shared budget. The ceiling is now 12,000 — 500
// calls (4%) below the observed refusal point — so the local gate trips
// first, and downstream reserves (1,500 for the backfill, 500 for the
// sbl/tdcc history backfill) are measured against a number that is actually
// usable. Anything derived from this constant must be re-derived when the
// upstream tier changes: override the ceiling with FINMIND_DAILY_LIMIT
// instead of editing the constant blindly, and never raise it above
// finmindObservedUpstreamRefusalLimit to make a test or an incident "pass".
const finmindDailyLimit = 12000

// finmindDailyLimitResolved returns the effective local daily ceiling: the
// FINMIND_DAILY_LIMIT env var when it holds a positive integer, else
// finmindDailyLimit. Mirrors finmindRateLimitPerHour so a tier change is a
// config change, not a rebuild.
func finmindDailyLimitResolved() int {
	v := strings.TrimSpace(os.Getenv("FINMIND_DAILY_LIMIT"))
	if v == "" {
		return finmindDailyLimit
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return finmindDailyLimit
	}
	if n > finmindObservedUpstreamRefusalLimit {
		// Loud, not fatal: raising the ceiling past the point where the
		// upstream demonstrably refuses re-creates the 2026-09-26 incident.
		logging.Warn("marketdata", "finmind_daily_limit_override_above_observed_refusal",
			"override", n,
			"observed_refusal", finmindObservedUpstreamRefusalLimit,
			"default", finmindDailyLimit,
		)
	}
	return n
}

// FinMindDailyLimit exposes the effective local daily ceiling so other
// packages (monitoring backfill floors, cmd/atlas reserve gates, their tests)
// can be written against the single source of truth instead of copying the
// number — the 2026-09-26 incident was in part three copies of "14400".
func FinMindDailyLimit() int { return finmindDailyLimitResolved() }

// ErrQuotaExhausted is returned by fetchDataset when the daily quota is gone.
// Callers should treat this as a transient, scheduled-skippable condition —
// distinct from API auth/quota errors that need human intervention. The
// channel adapter maps this to a "warn" status (not "error") so on-call
// doesn't get paged just because the daily budget ran out.
var ErrQuotaExhausted = fmt.Errorf("finmind: daily quota exhausted")

type FinMindClient struct {
	apiKey string
	// keyMu guards apiKey reads/writes: UpdateSharedFinMindAPIKey (issue
	// #1776 hot reload) swaps the key while in-flight requests read it.
	// (Pre-#1776 the update wrote under the singleton mutex but reads were
	// unsynchronized — a latent data race this field fixes.)
	keyMu        sync.RWMutex
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

// finmindRedacted replaces secret material in anything derived from an
// upstream response before it reaches a log line, an error string, or the
// on-disk quota state.
const finmindRedacted = "[REDACTED]"

// finmindSecretJSONKeys lists the upstream JSON fields that carry secret
// material. FinMind echoes a fragment of the caller's API key back on auth
// and quota errors — `{"msg":"Token is illegal.","status":400,
// "token_tail":"...abc"}` and, observed in production 2026-09-26, on the 402
// quota response as well. Any code path that surfaces raw upstream text must
// run it through sanitizeFinMindBody first, otherwise a token fragment lands
// in channel-health records, error strings and logs.
var finmindSecretJSONKeys = []string{
	"token_tail",
	"token",
	"api_key",
	"apikey",
	"authorization",
	"access_token",
	"password",
}

// finmindSecretKeyRe matches the `"key": "value"` shape of the fields above in
// bodies that do not parse as JSON (truncated bodies — the 512-byte cap can
// cut an envelope in half — HTML error pages, plain text). The whole pair,
// key included, is replaced: the field NAME is dropped too, so a log line or
// an error string can never be mistaken for a place to look for the value.
var finmindSecretKeyRe = regexp.MustCompile(`(?i)"?(?:token_tail|token|api_key|apikey|authorization|access_token|password)"?[\s]*[:=][\s]*("[^"]*"|[^",}\s]+)`)

// sanitizeFinMindBody removes secret material from a raw upstream body.
//
// It redacts (a) JSON object members whose key is listed in
// finmindSecretJSONKeys — recursively, so nested envelopes are covered — and
// (b) any literal occurrence of the API keys passed in secrets. The literal
// rule only applies to values of at least 8 bytes: shorter strings would
// mangle unrelated text (a 1-byte key would gut every message), and real
// FinMind tokens are JWT-sized. Bodies shorter than that bound are still
// covered by the field redaction above.
//
// The result keeps everything an operator needs ("Token is illegal.",
// "Requests reach the upper limit") while dropping the credential fragment.
func sanitizeFinMindBody(body string, secrets ...string) string {
	out := scrubFinMindJSONSecrets(body)
	for _, s := range secrets {
		if len(s) >= 8 {
			out = strings.ReplaceAll(out, s, finmindRedacted)
		}
	}
	return out
}

// scrubFinMindJSONSecrets redacts the known secret fields in body, preferring
// a structural (JSON) rewrite and falling back to a textual scrub.
func scrubFinMindJSONSecrets(body string) string {
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err == nil {
		if !redactFinMindSecretsInValue(decoded) {
			return body
		}
		if reencoded, err := json.Marshal(decoded); err == nil {
			return string(reencoded)
		}
		return body
	}
	return finmindSecretKeyRe.ReplaceAllString(body, finmindRedacted)
}

// redactFinMindSecretsInValue walks a decoded JSON value and DROPS every
// object member whose key is secret-looking (the member is removed, not merely
// masked, so the field name cannot hint at where to look). Reports whether
// anything changed.
func redactFinMindSecretsInValue(v any) bool {
	changed := false
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if isFinMindSecretKey(k) {
				delete(node, k)
				changed = true
				continue
			}
			if redactFinMindSecretsInValue(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range node {
			if redactFinMindSecretsInValue(child) {
				changed = true
			}
		}
	}
	return changed
}

// isFinMindSecretKey reports whether a JSON member key carries secret material.
func isFinMindSecretKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, secret := range finmindSecretJSONKeys {
		if normalized == secret {
			return true
		}
	}
	return false
}

// finmindUpstreamQuotaReason renders the reason stored with the upstream quota
// latch: status + sanitized body, so the persisted reason (and every error that
// replays it) says WHICH signal fired.
func finmindUpstreamQuotaReason(status int, body string) string {
	return fmt.Sprintf("upstream HTTP %d: %s", status, body)
}

// finmindQuotaBody reports whether upstream text is the DAILY-quota verdict
// ("Requests reach the upper limit", the string FinMind sends with 402) as
// opposed to a per-request throttle or a free-tier notice. Only this text may
// latch the whole quota day as exhausted.
func finmindQuotaBody(body string) bool {
	return strings.Contains(strings.ToLower(body), "upper limit")
}

// clampForError bounds an upstream-derived string before it is embedded in an
// error or persisted to the quota state file, so one verbose upstream body
// cannot produce an unbounded error (or a state file) later on.
func clampForError(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	// Cut on a rune boundary so the result stays valid UTF-8.
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

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
	sharedFinMindClientMu.RLock()
	defer sharedFinMindClientMu.RUnlock()
	if sharedFinMindClient != nil {
		sharedFinMindClient.SetAPIKey(apiKey)
	}
}

// SetAPIKey swaps the client API key (thread-safe).
func (c *FinMindClient) SetAPIKey(key string) {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	c.apiKey = key
}

// currentAPIKey returns the active API key (thread-safe).
func (c *FinMindClient) currentAPIKey() string {
	c.keyMu.RLock()
	defer c.keyMu.RUnlock()
	return c.apiKey
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
	tracker := NewDailyQuotaTracker("finmind", stateDir, finmindDailyLimitResolved())
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
// Returns the full daily limit when no tracker is configured, and 0 while
// the upstream latch is set (the provider said today's budget is gone).
func (c *FinMindClient) QuotaRemaining() int {
	if c.quotaTracker == nil {
		return finmindDailyLimitResolved()
	}
	return c.quotaTracker.Remaining()
}

// quotaGateError returns a typed ErrQuotaExhausted error when the shared
// daily budget is unavailable, or nil when the call may proceed. One
// implementation for every FinMind entry point (fetchDataset, the 5-second
// index endpoint) so the local ceiling and the upstream latch produce the
// same operator-facing classification and message shape.
//
// The pre-2026-09-26 message shape ("finmind: daily quota exhausted
// (used=%d, remaining=%d)") is preserved verbatim for the local-ceiling case;
// the latch case adds the upstream reason and the observation time, which is
// what tells an operator the difference between "we stopped ourselves" and
// "FinMind is refusing every call today".
func (c *FinMindClient) quotaGateError() error {
	if c.quotaTracker == nil {
		return nil
	}
	if c.quotaTracker.AllowCall() {
		return nil
	}
	// The shared counter itself is unusable (unreadable, unparsable, or a
	// platform without file locking): today's usage is UNKNOWN, so the call is
	// refused and the reason must not be mistakable for "we spent today's
	// budget" (#2014 requirement 3; same family as #2009, where an empty value
	// silently passed). It stays wrapped in ErrQuotaExhausted on purpose: every
	// existing consumer (channel warn mapping, cache fallback, the
	// atlas_data_aggregator_failures_total{kind="quota"} metric) keeps working,
	// while the message and the tracker's own Error log carry the distinct
	// signal.
	if stateErr := c.quotaTracker.StateErr(); stateErr != nil {
		return fmt.Errorf("finmind: %w (quota-state-unusable, remaining=0, error=%s)",
			ErrQuotaExhausted, clampForError(stateErr.Error(), 240))
	}
	used := c.quotaTracker.CallsToday()
	if exhausted, reason, at := c.quotaTracker.UpstreamExhaustion(); exhausted {
		if reason == "" {
			reason = "upstream refused a call (quota signal)"
		}
		return fmt.Errorf("finmind: %w (upstream-exhausted, used=%d, remaining=0, observed_at=%s, reason=%s)",
			ErrQuotaExhausted, used, at.UTC().Format(time.RFC3339), clampForError(reason, 200))
	}
	return fmt.Errorf("finmind: %w (used=%d, remaining=%d)", ErrQuotaExhausted, used, c.quotaTracker.Remaining())
}

// markDailyQuotaExhausted latches the upstream quota refusal on the shared
// tracker. The latch is persisted (see DailyQuotaTracker.MarkUpstreamExhausted)
// so every later call — and every process that starts after a restart —
// short-circuits at quotaGateError instead of spending a request on a 402.
// reason must already be sanitized.
func (c *FinMindClient) markDailyQuotaExhausted(reason string) {
	if c.quotaTracker == nil {
		return
	}
	c.quotaTracker.MarkUpstreamExhausted(reason)
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
	// single check protects the whole channel from cold-start bursts (commit
	// 35642c13 switched auto_quote_backfill to FinMind, which can hit 1000s
	// of calls per cycle without this gate). When the daily budget is gone we
	// return ErrQuotaExhausted rather than letting the HTTP request fail with
	// a misleading 400 status.
	//
	// The gate covers BOTH exhaustions (see quotaGateError): the local
	// ceiling, and the upstream latch set when FinMind itself answered 402.
	// The latch is what makes the gate durable across restarts — on
	// 2026-09-26 a restart cleared the in-memory counter, the backfill ran
	// again, and the platform re-discovered the wall one 402 at a time.
	//
	// P1-7: quota exhaustion is a budget condition (auto-resets at the
	// tracker's day boundary: 00:00 process-local, and production containers
	// run TZ-unset = UTC, i.e. 08:00 Taipei — 2026-09-24 evidence:
	// data/state/finmind_daily_quota.json last_reset=2026-09-24T00:00:00Z)
	// — it must NOT trip the breaker, so we reset instead of recording a
	// failure.
	if err := c.quotaGateError(); err != nil {
		c.breakerRecordSuccess()
		return nil, err
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

	if key := c.currentAPIKey(); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
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
		// Secret hygiene: the upstream body is logged and wrapped into errors
		// below, and FinMind echoes a fragment of the API key back in
		// `token_tail` (seen in production on the 2026-09-26 402 quota
		// response). Scrub before it reaches any sink.
		safeBody := sanitizeFinMindBody(bodyStr, c.currentAPIKey())
		logging.Warn("finmind", "fetch_non_2xx",
			"status", resp.StatusCode,
			"body", safeBody,
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
		// budget condition that auto-resets at the day boundary (00:00 UTC in
		// production = 08:00 Taipei).
		// P1-7: 402 is the server-side quota signal — a budget condition, not
		// an outage; do NOT trip the breaker (same rule as the local gate).
		//
		// fix/finmind-quota-honor-402-r (2026-09-26): a 402 also LATCHES the
		// day as exhausted (persisted in finmind_daily_quota.json). The local
		// ceiling is a guess about the upstream tier; the 402 is the upstream
		// telling us the answer. Before this, the tracker kept counting past
		// the refusal point, every subsequent call spent a doomed request,
		// and a restart re-opened the flood. The same latch fires for a
		// 2xx-with-"upper limit" envelope reached below.
		if resp.StatusCode == http.StatusPaymentRequired || finmindQuotaBody(safeBody) {
			// The latch reason carries the upstream STATUS as well as the body
			// (finmindUpstreamQuotaReason): it is persisted and replayed in
			// every later error, so an operator reading a channel-health
			// record after a restart must be able to tell "FinMind answered
			// 402" from "we hit our own local ceiling" without re-reading the
			// body — the two have different remedies.
			c.markDailyQuotaExhausted(finmindUpstreamQuotaReason(resp.StatusCode, safeBody))
			c.breakerRecordSuccess()
			return nil, fmt.Errorf("finmind: %w (upstream HTTP %d): %s", ErrQuotaExhausted, resp.StatusCode, safeBody)
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
			return nil, fmt.Errorf("finmind: %w (retry_after=%ds): %s", ErrIPBanned, retryAfter, safeBody)
		}
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: status %d, body: %s", resp.StatusCode, safeBody)
	}

	var finmindResp FinMindResponse
	if err := json.NewDecoder(resp.Body).Decode(&finmindResp); err != nil {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: decode response: %w", err)
	}

	if finmindResp.Status != 200 {
		c.breakerRecordFailure()
		return nil, fmt.Errorf("finmind: API error: %s", sanitizeFinMindBody(finmindResp.Msg, c.currentAPIKey()))
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
		safeMsg := sanitizeFinMindBody(finmindResp.Msg, c.currentAPIKey())
		// A 2xx envelope can carry the same daily-quota verdict as the 402
		// ("Requests reach the upper limit") when the upstream throttles
		// inside a 200. Latch it exactly like the 402 — but only that
		// message: other non-"success" envelopes with empty data (free-tier
		// notices, per-dataset "no data") are classified as quota without
		// claiming the whole day is spent, so a single empty dataset cannot
		// disable the channel until midnight.
		if finmindQuotaBody(safeMsg) {
			c.markDailyQuotaExhausted(finmindUpstreamQuotaReason(finmindResp.Status, safeMsg))
		}
		return nil, fmt.Errorf("finmind: %w: %s", ErrQuotaExhausted, safeMsg)
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

// ErrNoDataForSymbol marks an authoritative "the source has no row for this
// symbol" answer — FinMind returns HTTP 200 with an empty dataset for a symbol
// that did not trade on the requested date. It is deliberately distinct from a
// transport/quota error so the quote chain can classify the symbol as no_data
// instead of reporting an acquisition failure (issue #1986).
var ErrNoDataForSymbol = fmt.Errorf("no data for symbol")

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
		// Wrapped in ErrNoDataForSymbol so a caller can tell "the source has no
		// such row" (a market fact) from "the request failed" (issue #1986).
		return domain.Quote{}, fmt.Errorf("finmind: no price data for %s on %s: %w", symbol, date, ErrNoDataForSymbol)
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

// GetQuotesBatch implements PartialBatchProvider: it walks the symbols and
// reports, per symbol, whether FinMind answered with a row, answered "no row"
// (ErrNoDataForSymbol → QuoteOutcomeNoData) or failed (QuoteOutcomeError).
//
// The per-symbol walk is unavoidable here — FinMind's price dataset is
// per-symbol — which is exactly why the hybrid chain only ever sends this arm a
// small residual (see hybridNarrowResidualMax) instead of a whole chunk.
func (p *FinMindProvider) GetQuotesBatch(ctx context.Context, asOf time.Time, symbols []string) (QuoteBatch, error) {
	batch := NewQuoteBatch(symbols)
	if !isTaiwanTradingDay(asOf) {
		// A non-trading day is not "no data for this symbol": it says nothing
		// about the symbol itself, so every symbol is an acquisition gap.
		err := fmt.Errorf("finmind: asOf %s is not a Taiwan trading day (weekend or holiday)", asOf.Format("2006-01-02"))
		batch.Resolve(symbols, QuoteOutcomeError)
		return batch, err
	}
	date := asOf.Format("2006-01-02")

	var lastErr error
	for _, symbol := range symbols {
		quote, err := p.client.GetStockPrice(ctx, symbol, date)
		if err != nil {
			switch {
			case errors.Is(err, ErrNoDataForSymbol):
				batch.SetOutcome(symbol, QuoteOutcomeNoData)
			default:
				logging.Error("finmind", "fetch_failed", "symbol", symbol, logging.Err(err))
				batch.SetOutcome(symbol, QuoteOutcomeError)
				lastErr = err
			}
			continue
		}
		batch.RecordFor(symbol, quote)
	}

	if len(batch.Quotes) == 0 && lastErr != nil {
		return batch, fmt.Errorf("finmind: all symbols failed: %w", lastErr)
	}
	return batch, nil
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
