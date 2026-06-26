package monitoring

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CleanupStaleHeartbeats is the one-time migration (Decision 1,
// alert-redesign-v2.md Part 3.1) that deletes the historical
// channel_health_summary alerts that were the original 16,806-noise
// problem. After the migration, the rule=channel_health_summary is
// suppressed (see parameters.json:alert.suppress_categories="gateway")
// and the health summary is logged to the health widget (see
// internal/monitoring/health.go:checkGateway) — no longer the alert
// stream.
//
// ttl defines the staleness window: alerts older than (now - ttl)
// are considered "stale heartbeats" and deleted. The function is
// idempotent and safe to run multiple times.
//
// Returns the number of alerts deleted.
func CleanupStaleHeartbeats(store *AlertStore, ttl time.Duration) (int, error) {
	cutoff := time.Now().Add(-ttl)
	return store.DeleteWhere(func(rec *domain.AlertRecord) bool {
		return rec.Rule == "channel_health_summary" && rec.Timestamp.Before(cutoff)
	})
}
