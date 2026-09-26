package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FinMindChannelAdapter adapts a FinMindClient to the DataProvider interface.
type FinMindChannelAdapter struct {
	client *marketdata.FinMindClient
	// snapshotBase is the configured work dir (config.Config.WorkDir) the
	// L3 channel snapshot is written under. It is injected, never derived from
	// the process CWD: see saveSnapshot for why a relative path is forbidden.
	snapshotBase string
	// now is the clock used to pick the probe date. Nil means time.Now; tests
	// set it to replay a 休市日 / 交易日盤前 probe deterministically (#1999).
	now func() time.Time
}

// NewFinMindChannelAdapter creates a new adapter for the FinMind channel.
// workDir is the configured work dir; it MUST be non-empty (see saveSnapshot).
func NewFinMindChannelAdapter(client *marketdata.FinMindClient, workDir string) *FinMindChannelAdapter {
	return &FinMindChannelAdapter{client: client, snapshotBase: workDir, now: time.Now}
}

// clock returns the adapter's time source (nil-safe: a hand-constructed
// zero-value adapter falls back to time.Now).
func (a *FinMindChannelAdapter) clock() time.Time {
	if a.now == nil {
		return time.Now()
	}
	return a.now()
}

// finMindProbeSymbol is the symbol the liveness probe asks for. 2330 (台積電)
// is the most liquid TWSE listing, so its daily row is the first to exist and
// the most reliable liveness signal.
const finMindProbeSymbol = "2330"

// finmindProbeDate returns the date the channel probe must ask FinMind for:
// the most recent Taiwan trading day STRICTLY BEFORE now (YYYY-MM-DD, Taipei).
//
// Issue #1999: the previous helper (yesterday) only skipped weekends, so the
// 2026-09-26T01:01Z probe (= Saturday 09:01 Taipei) asked for 2026-09-25 —
// 中秋節, a 休市日 with no session at all. FinMind answered "no row", the
// adapter turned that into a fetch failure, and two consecutive hourly probes
// pinned channel_health finmind at status=error with consecutive_failures=2.
//
// The most recent *trading* day is by construction the newest session whose
// price row can exist, so the probe can no longer fail merely because the
// market was closed: a 休市日 probe and a 交易日盤前 probe both land on a
// session FinMind has already published (it releases TaiwanStockPrice after
// that session closes, well before the next session opens). Taipei time is the
// correct clock here — the date and the weekday must be the Taiwanese ones,
// not UTC's.
func finmindProbeDate(now time.Time) string {
	return marketdata.PreviousTradingDay(now.In(taipeiLoc), 1).Format("2006-01-02")
}

// finmindProbeErr classifies a probe failure.
//
// "The source has no row for the requested day" — ErrNoDataForSymbol: FinMind
// answers HTTP 200 with a structurally valid envelope and an empty dataset — is
// re-wrapped as marketdata.ErrEmptyQuote, deliberately NOT as ErrNoData.
//
// Why not ErrNoData (i.e. why not copy #1953's "non-trading day ⇒ ok" verdict
// verbatim): the probe date is a Taiwan TRADING day by construction
// (finmindProbeDate), so an empty answer can never be explained by the
// calendar. ErrNoData is documented as "holiday / weekend / not-yet-published"
// and maps to RecordWaiting ("ok"); using it here would make a DAILY channel
// whose upstream normally prints every business day read as healthy while it
// has been dark for days — the exact failure mode internal/marketdata/errors.go
// calls out when it explains why ErrEmptyQuote exists at all (BDI, 2026-09-20).
// #1953's verdict still applies to this channel for the case it was written
// for: a 休市日 / 週末 / 盤前 probe simply never reaches this branch, because it
// asks for a session that has already been published.
//
// ErrEmptyQuote carries the semantics we want in every existing consumer:
//   - Gateway.Fetch records WARN — the reason stays visible on the channel
//     page, and it can never page: the alert rules match atlas_channel_health_status
//     == 2 (error) only, and warn maps to 1.
//   - consecutive_failures is NOT incremented (recordInternal's warn branch
//     passes the streak through untouched).
//   - the circuit breaker treats it as a no-op (gateway isExpectedNonFailureErr).
//   - monitoring's classifyErrorSeverity maps it to warn.
//
// Sustained unexplained no-data still alarms — through this channel's own 1h
// probe task (cmd/atlas channel_health_finmind), which returns this error and
// therefore trips the background-task handler after 3 consecutive failures.
// There is deliberately no data-freshness backstop for finmind:
// DeriveChannelStatus anchors on LastFetchAt (channel_status.go), every attempt
// refreshes it, and finmind never sets last_data_at — so
// atlas_channel_staleness_overage_seconds stays 0 no matter how long the data
// has been missing. Do not describe the freshness window as this channel's
// safety net.
//
// Everything else — transport error, HTTP 4xx/5xx, schema change — keeps the
// ordinary failure path and still escalates to "error"; the pre-existing
// quota / IP-ban warn mappings are untouched because those sentinels are not
// ErrEmptyQuote.
func finmindProbeErr(err error, symbol, date string) error {
	if errors.Is(err, marketdata.ErrNoDataForSymbol) || errors.Is(err, marketdata.ErrNoData) {
		return fmt.Errorf("finmind: no price row for %s on %s (a Taiwan trading day): %w",
			symbol, date, marketdata.ErrEmptyQuote)
	}
	return fmt.Errorf("finmind fetch: %w", err)
}

func (a *FinMindChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	date := finmindProbeDate(a.clock())
	quote, err := a.client.GetStockPrice(ctx, finMindProbeSymbol, date)
	if err != nil {
		return nil, finmindProbeErr(err, finMindProbeSymbol, date)
	}
	data, err := json.Marshal(quote)
	if err != nil {
		return nil, fmt.Errorf("finmind marshal: %w", err)
	}
	limiter := a.RateLimit()
	saveSnapshot(a.snapshotBase, "finmind", data)
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "finmind",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching 2330 from the most recent
// Taiwan trading day (see finmindProbeDate).
//
// Daily-quota exhaustion is surfaced as "warn" (not "error") because the
// underlying channel is healthy — the budget just ran out for the day.
// On-call should not be paged for this; the dashboard surfaces it via
// the channel-health page and the budget auto-resets at the quota day
// boundary (00:00 UTC in production = 08:00 Taipei).
//
// "No row for the requested day" is reported as "warn" — the same verdict
// Gateway.Fetch reaches for ErrEmptyQuote (see finmindProbeErr) — with a nil
// error. The nil error is load-bearing for this entry point:
// UnifiedHealthStore.CheckHealth overwrites any non-nil error with
// Status{Status: "error"} whatever the adapter said, so returning (warn, err)
// would still record error. (That function has no production caller today;
// finmind's runtime health is written by the 1h channel_health_finmind
// Gateway.Fetch task. Keeping the adapter honest for its declared contract
// costs nothing and removes a latent false positive.)
func (a *FinMindChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	date := finmindProbeDate(a.clock())
	_, err := a.client.GetStockPrice(ctx, finMindProbeSymbol, date)
	if err != nil {
		// 上游對「一個交易日」沒有資料列：非通道故障（不該 error/page），
		// 但也不是「日曆可解釋的無新資料」（探測日必為交易日）→ warn，
		// 讓通道頁保留原因。
		if errors.Is(err, marketdata.ErrNoDataForSymbol) || errors.Is(err, marketdata.ErrNoData) {
			return HealthStatus{
				Status:    "warn",
				LastError: finmindProbeErr(err, finMindProbeSymbol, date).Error(),
				UpdatedAt: time.Now().Format(time.RFC3339),
				CheckType: "liveness",
			}, nil
		}
		status := "error"
		// ErrIPBanned（403 "ip banned"）與 ErrQuotaExhausted 同族：上游
		// 限流/額度條件，retry_after 後自癒 — warn 不報 error（2026-09-06
		// 實證：未映射時 finmind 通道誤標 error 30 分鐘）。
		if errors.Is(err, marketdata.ErrQuotaExhausted) || errors.Is(err, marketdata.ErrIPBanned) {
			status = "warn"
		}
		return HealthStatus{
			Status:    status,
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "liveness",
	}, nil
}

// RateLimit returns the underlying FinMind client rate limiter.
func (a *FinMindChannelAdapter) RateLimit() *rate.Limiter {
	return a.client.RateLimiter()
}

// Metadata returns static channel metadata for FinMind.
func (a *FinMindChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "finmind",
		Country:    "台灣",
		Platform:   "FinMind",
		APIFormat:  "json",
		Path:       "api.finmindtrade.com",
		HasLimiter: true,
	}
}
