package monitoring

// #1787 alert lifecycle: TTL auto-archival.
//
// Alerts are condition instances, not events: while a condition stays open
// (status=triggered) recurrences update the same record. Once the condition
// stops recurring, the record must not pollute the "需要決策" queue forever.
// This archiver resolves open alerts whose condition has not been seen for
// a severity-dependent grace period:
//
//	WARNING  -> 7 days without recurrence
//	ERROR    -> 30 days without recurrence
//	CRITICAL -> 30 days without recurrence
//
// Resolved records carry user "ttl-expiry" so admins can distinguish
// automatic archival from human acknowledgement in the audit trail.

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// StartAlertTTLLifecycle launches a background goroutine that periodically
// resolves stale open alerts. Cancel ctx to stop. Runs once immediately so
// a restart with an old backlog is cleaned on boot, then every interval.
func StartAlertTTLLifecycle(store *AlertStore, ctx context.Context, interval time.Duration) {
	if store == nil {
		return
	}
	go func() {
		runAlertTTLExpiry(store)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runAlertTTLExpiry(store)
			}
		}
	}()
}

func runAlertTTLExpiry(store *AlertStore) {
	now := time.Now()
	resolved, err := store.ResolveWhere(func(r *domain.AlertRecord) bool {
		if r.Status != domain.AlertStatusTriggered {
			return false
		}
		last := r.Timestamp
		if r.LastSeen != nil && r.LastSeen.After(last) {
			last = *r.LastSeen
		}
		switch r.Severity {
		case "WARNING":
			return now.Sub(last) > 7*24*time.Hour
		case "ERROR", "CRITICAL":
			return now.Sub(last) > 30*24*time.Hour
		default:
			return false
		}
	}, "ttl-expiry")
	if err != nil {
		logging.Warn("alerts", "ttl_expiry_failed", logging.Err(err))
		return
	}
	if resolved > 0 {
		logging.Info("alerts", "ttl_expired_resolved", "count", resolved)
	}
}
