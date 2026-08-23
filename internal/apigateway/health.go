package apigateway

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StaleDataThreshold is the maximum age of a "ok" ChannelHealthRecord.LastFetchAt
// before StatusSummary() downgrades the channel's status to "stale".
//
// Background: Issue #1086 — TEJ and janus_regime channels were last fetched
// 66+ days ago but HealthCheck (live API ping) still returned "ok" because
// the API itself was reachable. To catch the silent-failure pattern without
// breaking channels that legitimately have low-frequency schedules, we
// downgrade *ok* status to *stale* when no successful fetch has happened
// within the threshold.
//
// 48h was chosen because:
//   - The slowest legitimate refresh interval is 24h (etf_nav_refresh,
//     auto_daily_simulation). 48h gives a full interval of slack.
//   - Channels that fail to refresh for 2x their normal interval almost
//     certainly have a real problem (scheduler drift, upstream outage, etc).
//
// Per-channel override: a channel's ChannelContract.FreshnessWindow replaces
// this global for that channel (see channel_contract.go and
// deriveStatusWithContract). Zero/unspecified windows inherit this constant.
const StaleDataThreshold = 48 * time.Hour

// UnifiedHealthStore wraps the local ChannelHealthStore (relocated from
// internal/monitoring in Wave 12 Phase 2, Issue #731) with gateway-specific
// features. The backing store lives in apigateway now so the gateway does
// not need to import internal/monitoring — see channel_health.go for the
// cycle-breaking rationale.
type UnifiedHealthStore struct {
	store *ChannelHealthStore
}

// NewUnifiedHealthStore creates a health store.
func NewUnifiedHealthStore(dir string, pool *pgxpool.Pool) *UnifiedHealthStore {
	return &UnifiedHealthStore{
		store: NewChannelHealthStoreWithPool(dir, pool),
	}
}

// Record updates the health record for a channel.
func (u *UnifiedHealthStore) Record(channelID, status, errMsg string, opts ...RecordOption) error {
	return u.store.Record(channelID, status, errMsg, opts...)
}

// Get retrieves the health record for a channel.
func (u *UnifiedHealthStore) Get(channelID string) *ChannelHealthRecord {
	return u.store.Get(channelID)
}

// Alerts returns all channels with non-ok status.
func (u *UnifiedHealthStore) Alerts() []Alert {
	records := u.store.Alerts()
	alerts := make([]Alert, len(records))
	for i, r := range records {
		alerts[i] = Alert(r)
	}
	return alerts
}

// Alert represents a channel health alert.
type Alert struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	FetchAt   string `json:"fetch_at,omitempty"`
}

// RecordChannelHealthFromResult is a convenience function for background
// tasks that have a FetchResult in hand. Renamed from RecordChannelFetch
// during the Wave 12 Phase 2 (Issue #731) relocation to avoid clashing
// with the file-backed CLI helper of the same name in channel_health.go.
func RecordChannelHealthFromResult(store *UnifiedHealthStore, channelID string, result *FetchResult, err error) {
	if err != nil {
		_ = store.Record(channelID, "error", err.Error())
		return
	}

	opts := []RecordOption{
		WithLatencyMs(result.Meta.LatencyMs),
	}
	if result.Meta.RateLimitRemaining > 0 {
		opts = append(opts, WithRateLimitRemaining(result.Meta.RateLimitRemaining))
	}

	_ = store.Record(channelID, "ok", "", opts...)
}

// ChannelIDs returns all registered channel IDs.
func (u *UnifiedHealthStore) ChannelIDs() []string {
	return channelIDs()
}

// ChannelLatencyMs returns the last recorded latency for a channel in
// milliseconds. It returns 0 when the channel has no record or no latency.
func (u *UnifiedHealthStore) ChannelLatencyMs(channelID string) int64 {
	rec := u.Get(channelID)
	if rec == nil {
		return 0
	}
	return rec.LatencyMs
}

// StatusSummary returns a summary of all channel health statuses.
//
// Contract-aware: channels whose status is "ok" but whose LastFetchAt is older
// than the channel's contract FreshnessWindow (StaleDataThreshold 48h by
// default) are downgraded to "stale" by deriveStatusWithContract. This
// catches the silent-failure pattern reported in Issue #1086 where channels
// reported "ok" (live API ping worked) but had not actually been refreshed in
// 66+ days. Per-channel windows let fast channels (intraday quotes) stale out
// sooner and slow channels (twse_replay, 72h) stay ok longer.
func (u *UnifiedHealthStore) StatusSummary() map[string]HealthSummary {
	ids := channelIDs()
	summary := make(map[string]HealthSummary)
	contracts := ChannelContracts()

	for _, id := range ids {
		rec := u.store.Get(id)
		if rec == nil {
			summary[id] = HealthSummary{
				ChannelID: id,
				Status:    "unknown",
			}
			continue
		}

		summary[id] = HealthSummary{
			ChannelID: id,
			Status:    u.deriveStatusWithContract(rec, contracts.Contract(id)),
			LastFetch: rec.LastFetchAt,
			LastError: rec.LastError,
		}
	}

	return summary
}

// deriveStatusWithFreshness downgrades an "ok" record to "stale" when its
// LastFetchAt is older than StaleDataThreshold (the default contract window).
// Kept as a thin wrapper for callers that do not have a per-channel contract
// at hand; StatusSummary uses deriveStatusWithContract instead.
func (u *UnifiedHealthStore) deriveStatusWithFreshness(rec *ChannelHealthRecord) string {
	return u.deriveStatusWithContract(rec, DefaultChannelContract(""))
}

// deriveStatusWithContract downgrades an "ok" record to "stale" when its
// LastFetchAt is older than the channel contract's FreshnessWindow (default:
// StaleDataThreshold). Other statuses (error, warn, inactive, degraded) pass
// through unchanged — they're already alerting on real failures.
//
// Returns "stale" if the record is "ok" AND LastFetchAt parses as RFC3339 AND
// time.Since() exceeds the window. Unparseable timestamps or empty LastFetchAt
// keep the original status — the channel is broken, but not because of
// staleness, so mislabeling as "stale" would mislead on-call.
func (u *UnifiedHealthStore) deriveStatusWithContract(rec *ChannelHealthRecord, contract ChannelContract) string {
	if rec == nil {
		return "unknown"
	}
	if rec.Status != "ok" {
		return rec.Status
	}
	if rec.LastFetchAt == "" {
		return rec.Status
	}
	ts, err := time.Parse(time.RFC3339, rec.LastFetchAt)
	if err != nil {
		return rec.Status
	}
	window := contract.FreshnessWindow
	if window <= 0 {
		window = StaleDataThreshold
	}
	if time.Since(ts) > window {
		return "stale"
	}
	return rec.Status
}

type HealthSummary struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	LastFetch string `json:"last_fetch,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// CheckHealth performs a comprehensive health check for all channels.
func (u *UnifiedHealthStore) CheckHealth(ctx context.Context, registry *ChannelRegistry) map[string]HealthStatus {
	ids := channelIDs()
	results := make(map[string]HealthStatus)

	for _, id := range ids {
		provider, err := registry.Get(id)
		if err != nil {
			results[id] = HealthStatus{
				Status:    "error",
				LastError: fmt.Sprintf("provider not found: %v", err),
				CheckType: "liveness",
			}
			continue
		}

		status, err := provider.HealthCheck(ctx)
		if err != nil {
			status = HealthStatus{
				Status:    "error",
				LastError: err.Error(),
				CheckType: status.CheckType,
			}
		}

		// Apply the channel contract: an "ok" result is downgraded to
		// "degraded" when the contract requires data-level validation
		// (file_state / value_nonzero) and the persisted data state fails
		// the SuccessCriteria. This stops file-based channels from reporting
		// ok on file existence alone (government_broker ok 假象, 2026-08-22).
		status = EvaluateContractHealth(ctx, ChannelContracts().Contract(id), provider, status)

		results[id] = status
		_ = u.Record(id, status.Status, status.LastError)
	}

	return results
}
