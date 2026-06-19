package alerting

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// AlertmanagerPayload is the JSON payload sent by Alertmanager's webhook
// receiver. The schema is defined in
// https://github.com/prometheus/alertmanager/blob/main/template/default.tmpl.
type AlertmanagerPayload struct {
	Version  string              `json:"version"`
	GroupKey string              `json:"groupKey"`
	Status   string              `json:"status"`
	Receiver string              `json:"receiver"`
	Alerts   []AlertmanagerAlert `json:"alerts"`
}

// AlertmanagerAlert is a single alert entry inside an AlertmanagerPayload.
type AlertmanagerAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

// AlertWebhookHandler receives Alertmanager webhooks at /api/v1/alerts and
// retains them in memory for inspection by other components (e.g. an SSE
// stream or a recent-alerts endpoint).
type AlertWebhookHandler struct {
	mu    sync.RWMutex
	store []AlertmanagerAlert
	cap   int
}

// NewAlertWebhookHandler creates a handler with the given in-memory capacity.
// Non-positive cap defaults to 1000.
func NewAlertWebhookHandler(cap int) *AlertWebhookHandler {
	if cap <= 0 {
		cap = 1000
	}
	return &AlertWebhookHandler{cap: cap}
}

// ServeHTTP dispatches GET → recent alerts JSON, POST → ingest payload.
// Other methods are rejected with 405.
func (h *AlertWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.serveRecent(w, r)
	case http.MethodPost:
		h.serveIngest(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AlertWebhookHandler) serveRecent(w http.ResponseWriter, r *http.Request) {
	n := 100
	if v := r.URL.Query().Get("n"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			n = parsed
		}
	}
	alerts := h.Recent(n)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Alerts []AlertmanagerAlert `json:"alerts"`
		Count  int                 `json:"count"`
	}{Alerts: alerts, Count: len(alerts)})
}

func (h *AlertWebhookHandler) serveIngest(w http.ResponseWriter, r *http.Request) {
	var p AlertmanagerPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.store = append(h.store, p.Alerts...)
	if len(h.store) > h.cap {
		drop := len(h.store) - h.cap
		h.store = h.store[drop:]
	}
	w.WriteHeader(http.StatusOK)
}

// Recent returns a snapshot of the last n alerts (most recent first).
func (h *AlertWebhookHandler) Recent(n int) []AlertmanagerAlert {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n <= 0 || n > len(h.store) {
		n = len(h.store)
	}
	out := make([]AlertmanagerAlert, n)
	copy(out, h.store[len(h.store)-n:])
	return out
}

// Len returns the number of alerts currently retained.
func (h *AlertWebhookHandler) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.store)
}
