package apigateway

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
func (u *UnifiedHealthStore) StatusSummary() map[string]HealthSummary {
	ids := channelIDs()
	summary := make(map[string]HealthSummary)

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
			Status:    rec.Status,
			LastFetch: rec.LastFetchAt,
			LastError: rec.LastError,
		}
	}

	return summary
}

// HealthSummary represents a channel's health status.
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

		results[id] = status
		_ = u.Record(id, status.Status, status.LastError)
	}

	return results
}
