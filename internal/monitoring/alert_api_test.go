package monitoring

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func newTestAlertAPI(t *testing.T) (*AlertAPI, *AlertStore) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewAlertStore(dir)
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}
	return NewAlertAPI(store), store
}

func seedAlerts(t *testing.T, store *AlertStore) {
	t.Helper()
	a1 := domain.AlertRecord{
		ID:        "alert-1",
		Timestamp: time.Now(),
		Rule:      "rule_a",
		Severity:  "WARNING",
		Message:   "msg1",
		Status:    domain.AlertStatusTriggered,
	}
	a2 := domain.AlertRecord{
		ID:        "alert-2",
		Timestamp: time.Now(),
		Rule:      "rule_b",
		Severity:  "ERROR",
		Message:   "msg2",
		Status:    domain.AlertStatusTriggered,
	}
	a3 := domain.AlertRecord{
		ID:           "alert-3",
		Timestamp:    time.Now(),
		Rule:         "rule_c",
		Severity:     "CRITICAL",
		Message:      "msg3",
		Acknowledged: true,
		Status:       domain.AlertStatusAcknowledged,
	}
	for _, a := range []domain.AlertRecord{a1, a2, a3} {
		if err := store.Save(a); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
}

func TestAlertAPI_ListAlerts(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("total = %d, want 3", resp.Total)
	}
}

func TestAlertAPI_ListAlerts_Empty(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if resp.Alerts == nil {
		t.Error("alerts should be empty slice, not nil")
	}
}

func TestAlertAPI_ListAlerts_MethodNotAllowed(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestAlertAPI_Unacknowledged(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/unacknowledged", nil)
	rec := httptest.NewRecorder()
	api.handleUnacknowledged(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	for _, a := range resp.Alerts {
		if a.Acknowledged {
			t.Errorf("alert %s should not be acknowledged", a.ID)
		}
	}
}

func TestAlertAPI_Unacknowledged_MethodNotAllowed(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/unacknowledged", nil)
	rec := httptest.NewRecorder()
	api.handleUnacknowledged(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestAlertAPI_Acknowledge(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	body, _ := json.Marshal(map[string]string{"alert_id": "alert-1", "user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader(body))
	status, respBody := api.handleAcknowledge(req)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	resp, ok := respBody.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response type: %T", respBody)
	}
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}

	records, _ := store.LoadAll()
	for _, r := range records {
		if r.ID == "alert-1" {
			if !r.Acknowledged {
				t.Error("alert-1 should be acknowledged")
			}
			if r.AcknowledgedBy != "admin" {
				t.Errorf("AcknowledgedBy = %q, want admin", r.AcknowledgedBy)
			}
			return
		}
	}
	t.Fatal("alert-1 not found in store")
}

func TestAlertAPI_Acknowledge_NotFound(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"alert_id": "nonexistent", "user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader(body))
	status, _ := api.handleAcknowledge(req)

	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestAlertAPI_Acknowledge_MissingAlertID(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader(body))
	status, _ := api.handleAcknowledge(req)

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestAlertAPI_Acknowledge_MissingUser(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"alert_id": "alert-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader(body))
	status, _ := api.handleAcknowledge(req)

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestAlertAPI_Acknowledge_InvalidJSON(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader([]byte("not json")))
	status, _ := api.handleAcknowledge(req)

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestAlertAPI_Resolve(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	body, _ := json.Marshal(map[string]string{"alert_id": "alert-1", "user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/resolve", bytes.NewReader(body))
	status, respBody := api.handleResolve(req)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	resp, ok := respBody.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response type: %T", respBody)
	}
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}

	records, _ := store.LoadAll()
	for _, r := range records {
		if r.ID == "alert-1" {
			if r.Status != domain.AlertStatusResolved {
				t.Errorf("Status = %q, want resolved", r.Status)
			}
			if r.ResolvedBy != "admin" {
				t.Errorf("ResolvedBy = %q, want admin", r.ResolvedBy)
			}
			if r.ResolvedAt == nil {
				t.Error("ResolvedAt should not be nil")
			}
			return
		}
	}
	t.Fatal("alert-1 not found in store")
}

func TestAlertAPI_Resolve_NotFound(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"alert_id": "nonexistent", "user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/resolve", bytes.NewReader(body))
	status, _ := api.handleResolve(req)

	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestAlertAPI_Resolve_MissingAlertID(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/resolve", bytes.NewReader(body))
	status, _ := api.handleResolve(req)

	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestAlertAPI_RegisterRoutes(t *testing.T) {
	api, _ := newTestAlertAPI(t)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	tests := []struct {
		path   string
		method string
	}{
		{"/api/alerts", http.MethodGet},
		{"/api/alerts/unacknowledged", http.MethodGet},
		{"/api/alerts/acknowledge", http.MethodPost},
		{"/api/alerts/resolve", http.MethodPost},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Errorf("route %s %s not registered", tt.method, tt.path)
		}
	}
}

func TestAlertAPI_Stats(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/stats", nil)
	rec := httptest.NewRecorder()
	api.handleStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var stats map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats["total"] != 3 {
		t.Errorf("total = %d, want 3", stats["total"])
	}
	if stats["warning"] != 1 {
		t.Errorf("warning = %d, want 1", stats["warning"])
	}
	if stats["error"] != 1 {
		t.Errorf("error = %d, want 1", stats["error"])
	}
	if stats["critical"] != 1 {
		t.Errorf("critical = %d, want 1", stats["critical"])
	}
}

func TestAlertAPI_Stats_MethodNotAllowed(t *testing.T) {
	api, _ := newTestAlertAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/stats", nil)
	rec := httptest.NewRecorder()
	api.handleStats(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestAlertAPI_ListAlerts_FilterStatus(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?status=triggered", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
}

func TestAlertAPI_ListAlerts_FilterSeverity(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?severity=ERROR", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
	if len(resp.Alerts) != 1 || resp.Alerts[0].Severity != "ERROR" {
		t.Errorf("expected one ERROR alert, got %+v", resp.Alerts)
	}
}

func TestAlertAPI_ListAlerts_FilterRule(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?rule=rule_a", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestAlertAPI_ListAlerts_FilterTimeRange(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	from := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	url := "/api/alerts?from=" + from + "&to=" + to
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("total = %d, want 3", resp.Total)
	}
}

func TestAlertAPI_ListAlerts_Pagination(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?page=2&page_size=1", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
		Page   int                  `json:"page"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("total = %d, want 3", resp.Total)
	}
	if resp.Page != 2 {
		t.Errorf("page = %d, want 2", resp.Page)
	}
	if len(resp.Alerts) != 1 {
		t.Errorf("alerts len = %d, want 1", len(resp.Alerts))
	}
}

func TestAlertAPI_ListAlerts_PageSizeCapped(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?page_size=9999", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	var resp struct {
		PageSize int `json:"page_size"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PageSize != 50 {
		t.Errorf("page_size = %d, want 50 (capped)", resp.PageSize)
	}
}

func TestAlertAPI_ListAlerts_InvalidPageSize(t *testing.T) {
	api, store := newTestAlertAPI(t)
	seedAlerts(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts?page_size=abc", nil)
	rec := httptest.NewRecorder()
	api.handleListAlerts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp struct {
		PageSize int `json:"page_size"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PageSize != 50 {
		t.Errorf("page_size = %d, want 50 (default)", resp.PageSize)
	}
}

func TestAlertAPI_NilStore_GetHandlersReturnEmpty(t *testing.T) {
	api := NewAlertAPI(nil)

	cases := []struct {
		name     string
		path     string
		handler  func(http.ResponseWriter, *http.Request)
		wantKey  string
		wantCode int
	}{
		{"list", "/api/alerts", api.handleListAlerts, "alerts", http.StatusOK},
		{"unacknowledged", "/api/alerts/unacknowledged", api.handleUnacknowledged, "alerts", http.StatusOK},
		{"active", "/api/alerts/active", api.handleActiveSnapshot, "active", http.StatusOK},
		{"stats", "/api/alerts/stats", api.handleStats, "total", http.StatusOK},
		{"rules", "/api/alerts/rules", api.handleRules, "rules", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			tc.handler(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			var resp map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, ok := resp[tc.wantKey]; !ok {
				t.Errorf("missing key %q in response", tc.wantKey)
			}
		})
	}
}
