package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/monitoring/alertscanner"
)

// helper: build a store with N alerts for testing.
func buildAlertsForAPI(t *testing.T, api *AlertAPI) []domain.AlertRecord {
	t.Helper()
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	api.store = store
	now := time.Now()
	records := []domain.AlertRecord{
		{ID: "a-1", Timestamp: now, Rule: "channel_health", Severity: "warning", Status: domain.AlertStatusTriggered, Count: 1},
		{ID: "a-2", Timestamp: now.Add(-time.Minute), Rule: "channel_health", Severity: "error", Status: domain.AlertStatusTriggered, Count: 1},
		{ID: "a-3", Timestamp: now.Add(-2 * time.Minute), Rule: "drawdown", Severity: "critical", Status: domain.AlertStatusTriggered, Count: 1},
	}
	for i := range records {
		if err := store.Save(records[i]); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}
	return records
}

func TestAlertAPI_HandleActiveSnapshot(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/active", nil)
	rec := httptest.NewRecorder()
	api.handleActiveSnapshot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp alertscanner.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", rec.Body.String())
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}
	if !resp.Blocked {
		t.Errorf("expected blocked=true because critical/error alerts exist")
	}
	if resp.BySeverity["critical"] != 1 || resp.BySeverity["error"] != 1 || resp.BySeverity["warning"] != 1 {
		t.Errorf("unexpected severity counts: %+v", resp.BySeverity)
	}
	if len(resp.Alerts) != 3 {
		t.Errorf("expected 3 alerts, got %d", len(resp.Alerts))
	}
}

func TestAlertAPI_HandleActiveSnapshot_MethodNotAllowed(t *testing.T) {
	api := &AlertAPI{}
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/active", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	api.handleActiveSnapshot(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
func TestAlertAPI_HandleListAlerts_SortTimestampDesc(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?sort=timestamp_desc", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("expected total=3, got %d", resp.Total)
	}
	// timestamp_desc: a-1 (now) first, a-2 (-1min) second, a-3 (-2min) last
	if resp.Alerts[0].ID != "a-1" || resp.Alerts[1].ID != "a-2" || resp.Alerts[2].ID != "a-3" {
		t.Errorf("sort=timestamp_desc wrong order: %v", []string{resp.Alerts[0].ID, resp.Alerts[1].ID, resp.Alerts[2].ID})
	}
}

func TestAlertAPI_HandleListAlerts_SortTimestampAsc(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?sort=timestamp_asc", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// timestamp_asc: a-3 (-2min) first, a-2 (-1min) second, a-1 (now) last
	if resp.Alerts[0].ID != "a-3" || resp.Alerts[1].ID != "a-2" || resp.Alerts[2].ID != "a-1" {
		t.Errorf("sort=timestamp_asc wrong order: %v", []string{resp.Alerts[0].ID, resp.Alerts[1].ID, resp.Alerts[2].ID})
	}
}

func TestAlertAPI_HandleListAlerts_SortSeverityDesc(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?sort=severity_desc", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// severity_desc: critical > error > warning
	if resp.Alerts[0].Severity != "critical" {
		t.Errorf("expected first alert=critical, got %s", resp.Alerts[0].Severity)
	}
	if resp.Alerts[1].Severity != "error" {
		t.Errorf("expected second alert=error, got %s", resp.Alerts[1].Severity)
	}
	if resp.Alerts[2].Severity != "warning" {
		t.Errorf("expected third alert=warning, got %s", resp.Alerts[2].Severity)
	}
}

func TestAlertAPI_HandleListAlerts_SortDefault(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	// No sort param → default to timestamp_desc (newest first)
	req := httptest.NewRequest(http.MethodGet, "/api/alerts", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Alerts[0].ID != "a-1" {
		t.Errorf("default sort should be timestamp_desc, got first=%s", resp.Alerts[0].ID)
	}
}

func TestAlertAPI_HandleAcknowledgeBulk(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	body := `{"ids": ["a-1", "a-2", "nonexistent"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge-bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	sharedPost(api.handleAcknowledgeBulk).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Acknowledged int `json:"acknowledged"`
		Failed       int `json:"failed"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Acknowledged != 2 {
		t.Errorf("expected acknowledged=2, got %d", resp.Acknowledged)
	}
	if resp.Failed != 1 {
		t.Errorf("expected failed=1 (nonexistent), got %d", resp.Failed)
	}

	// Verify the two alerts are now acknowledged (AlertStore.Acknowledge sets
	// the Acknowledged bool field, not the Status enum).
	records, _ := api.store.LoadAll()
	ackCount := 0
	for _, r := range records {
		if r.Acknowledged {
			ackCount++
		}
	}
	if ackCount != 2 {
		t.Errorf("expected 2 acknowledged alerts in store, got %d", ackCount)
	}
}

func TestAlertAPI_HandleSilence(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	body := `{"rule": "channel_health", "duration_minutes": 60, "reason": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/silence", strings.NewReader(body))
	rec := httptest.NewRecorder()
	sharedPost(api.handleSilence).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		SilencedUntil string `json:"silenced_until"`
		Rule          string `json:"rule"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Rule != "channel_health" {
		t.Errorf("expected rule=channel_health, got %s", resp.Rule)
	}
	if resp.SilencedUntil == "" {
		t.Error("expected non-empty silenced_until")
	}
	// Verify it's a valid RFC3339 timestamp
	parsed, err := time.Parse(time.RFC3339, resp.SilencedUntil)
	if err != nil {
		t.Errorf("silenced_until not valid RFC3339: %v", err)
	}
	// Verify it's in the future (60 min from now ± 5 sec tolerance)
	if parsed.Before(time.Now().Add(-5 * time.Second)) {
		t.Errorf("silenced_until should be in the future, got %s", resp.SilencedUntil)
	}
}

func TestAlertAPI_HandleRules(t *testing.T) {
	api := &AlertAPI{}
	buildAlertsForAPI(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/rules", nil)
	rec := httptest.NewRecorder()
	api.handleRules(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Rules []struct {
			Rule        string `json:"rule"`
			ActiveCount int    `json:"active_count"`
			LastSeen    string `json:"last_seen"`
		} `json:"rules"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Should have 2 rules: channel_health (2 alerts) and drawdown (1 alert)
	ruleCounts := map[string]int{}
	for _, r := range resp.Rules {
		ruleCounts[r.Rule] = r.ActiveCount
	}
	if ruleCounts["channel_health"] != 2 {
		t.Errorf("expected channel_health active_count=2, got %d", ruleCounts["channel_health"])
	}
	if ruleCounts["drawdown"] != 1 {
		t.Errorf("expected drawdown active_count=1, got %d", ruleCounts["drawdown"])
	}
}

// sharedPost mimics internal/monitoring/api/shared.Post().
// Inlined here to avoid a circular import in the test file.
func sharedPost(handler func(r *http.Request) (int, any)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status, body := handler(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
}

// stub to keep imports (eventbus is referenced in comments only)
var (
	_ = eventbus.BusEvent{}
	_ = context.Background
)
