package apigateway

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// UnifiedHealthStore wraps monitoring.ChannelHealthStore with gateway-specific features.
type UnifiedHealthStore struct {
	store *monitoring.ChannelHealthStore
}

// NewUnifiedHealthStore creates a health store.
func NewUnifiedHealthStore(dir string, pool *pgxpool.Pool) *UnifiedHealthStore {
	return &UnifiedHealthStore{
		store: monitoring.NewChannelHealthStoreWithPool(dir, pool),
	}
}

// Record updates the health record for a channel.
func (u *UnifiedHealthStore) Record(channelID, status, errMsg string, opts ...monitoring.RecordOption) error {
	return u.store.Record(channelID, status, errMsg, opts...)
}

// Get retrieves the health record for a channel.
func (u *UnifiedHealthStore) Get(channelID string) *monitoring.ChannelHealthRecord {
	return u.store.Get(channelID)
}

// Alerts returns all channels with non-ok status.
func (u *UnifiedHealthStore) Alerts() []Alert {
	records := u.store.Alerts()
	alerts := make([]Alert, len(records))
	for i, r := range records {
		alerts[i] = Alert{
			ChannelID: r.ChannelID,
			Status:    r.Status,
			Error:     r.Error,
			FetchAt:   r.FetchAt,
		}
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

// RecordChannelFetch is a convenience function for background tasks.
func RecordChannelFetch(store *UnifiedHealthStore, channelID string, result *FetchResult, err error) {
	if err != nil {
		_ = store.Record(channelID, "error", err.Error())
		return
	}

	opts := []monitoring.RecordOption{
		monitoring.WithLatencyMs(result.Meta.LatencyMs),
	}
	if result.Meta.RateLimitRemaining > 0 {
		opts = append(opts, monitoring.WithRateLimitRemaining(result.Meta.RateLimitRemaining))
	}

	_ = store.Record(channelID, "ok", "", opts...)
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
