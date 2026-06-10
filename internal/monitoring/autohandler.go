package monitoring

import (
	"sync"
	"time"
)

// suppressRule represents a suppression rule matched by alert category.
type suppressRule struct {
	Category string // If empty, matches all
	Duration time.Duration
}

// AutoHandler is an AlertHandler that applies severity-based routing:
// auto-acknowledges INFO alerts, applies suppression rules, and
// handles recovery signals.
type AutoHandler struct {
	mu            sync.Mutex
	suppressUntil map[string]time.Time // category → expiry
	rules         []suppressRule
	alertStore    *AlertStore
}

// NewAutoHandler creates an AutoHandler with the given suppression rules.
func NewAutoHandler(store *AlertStore, rules []suppressRule) *AutoHandler {
	if rules == nil {
		rules = make([]suppressRule, 0)
	}
	return &AutoHandler{
		suppressUntil: make(map[string]time.Time),
		rules:         rules,
		alertStore:    store,
	}
}

// Handle processes an alert through the auto-handler pipeline.
// It implements the AlertHandler function signature.
func (h *AutoHandler) Handle(alert Alert) {
	// Step 1: Check suppression rules
	if h.isSuppressed(alert) {
		return
	}

	// Step 2: Auto-acknowledge INFO alerts
	if alert.Level == AlertLevelInfo {
		h.autoAcknowledge(alert)
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
