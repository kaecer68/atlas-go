package monitoring

import (
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// SuppressRule represents a suppression rule matched by alert category.
type SuppressRule struct {
	Category string // If empty, matches all
	Duration time.Duration
}

// AutoHandler is an AlertHandler that applies severity-based routing:
// auto-acknowledges INFO alerts, applies suppression rules, and
// handles recovery signals.
type AutoHandler struct {
	mu            sync.Mutex
	suppressUntil map[string]time.Time // category → expiry
	rules         []SuppressRule
	alertStore    *AlertStore
}

// NewAutoHandler creates an AutoHandler with the given suppression rules.
// Static rules are immediately applied to suppressUntil so they take effect
// without requiring a separate Suppress() call.
func NewAutoHandler(store *AlertStore, rules []SuppressRule) *AutoHandler {
	if rules == nil {
		rules = make([]SuppressRule, 0)
	}
	suppressUntil := make(map[string]time.Time)
	now := time.Now()
	for _, rule := range rules {
		suppressUntil[rule.Category] = now.Add(rule.Duration)
	}
	return &AutoHandler{
		suppressUntil: suppressUntil,
		rules:         rules,
		alertStore:    store,
	}
}

// Suppress dynamically suppresses alerts matching the given category for the
// specified duration. This overrides any existing suppression for the category.
func (h *AutoHandler) Suppress(category string, duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.suppressUntil[category] = time.Now().Add(duration)
}

// Recover resolves all triggered alerts matching the given category.
// Called when a system recovers from a failure state.
func (h *AutoHandler) Recover(category string) {
	if h.alertStore == nil {
		return
	}
	resolved, _ := h.alertStore.ResolveWhere(func(r *domain.AlertRecord) bool {
		return r.Rule == category && r.Status == domain.AlertStatusTriggered
	}, "auto-recovery")
	if resolved > 0 {
		logging.Info("autohandler", "auto_recovered",
			"category", category,
			"resolved", resolved)
	}
}

// Handle processes an alert through the auto-handler pipeline.
// It implements the AlertHandler function signature.
func (h *AutoHandler) Handle(alert Alert) {
	// Step 1: Auto-acknowledge INFO alerts (always — humans should never see them).
	// isSuppressed is invoked for its side-effect of cleaning up expired
	// suppression entries; the bool result is intentionally ignored for INFO.
	if alert.Level == AlertLevelInfo {
		h.isSuppressed(alert) //nolint:staticcheck // side-effect only
		h.autoAcknowledge(alert)
		return
	}

	// Step 2: Check suppression rules for non-INFO alerts.
	if h.isSuppressed(alert) {
		return
	}
}

// suppress temporarily suppresses alerts matching the given category.
// isSuppressed checks whether the alert's category is currently suppressed.
func (h *AutoHandler) isSuppressed(alert Alert) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	// Check dynamic suppress entries
	if expiry, ok := h.suppressUntil[alert.Category]; ok {
		if now.Before(expiry) {
			return true
		}
		delete(h.suppressUntil, alert.Category)
	}

	// Check static suppression rules
	for _, rule := range h.rules {
		if rule.Category == "" || rule.Category == alert.Category {
			if now.Before(h.suppressUntil[rule.Category]) {
				return true
			}
		}
	}
	return false
}

// autoAcknowledge marks an INFO alert as acknowledged in the store.
func (h *AutoHandler) autoAcknowledge(alert Alert) {
	if h.alertStore == nil {
		return
	}
	_ = h.alertStore.Acknowledge(alert.ID, "auto-handler")
}
