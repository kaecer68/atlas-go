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
	}
	a2 := domain.AlertRecord{
		ID:        "alert-2",
		Timestamp: time.Now(),
		Rule:      "rule_b",
		Severity:  "ERROR",
		Message:   "msg2",
	}
	a3 := domain.AlertRecord{
		ID:           "alert-3",
		Timestamp:    time.Now(),
		Rule:         "rule_c",
		Severity:     "CRITICAL",
		Message:      "msg3",
		Acknowledged: true,
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
	rec := httptest.NewRecorder()
	api.handleAcknowledge(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
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
	rec := httptest.NewRecorder()
	api.handleAcknowledge(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAlertAPI_Acknowledge_MissingAlertID(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"user": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api.handleAcknowledge(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAlertAPI_Acknowledge_MissingUser(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	body, _ := json.Marshal(map[string]string{"alert_id": "alert-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api.handleAcknowledge(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAlertAPI_Acknowledge_InvalidJSON(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/alerts/acknowledge", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	api.handleAcknowledge(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAlertAPI_Acknowledge_MethodNotAllowed(t *testing.T) {
	api, _ := newTestAlertAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/alerts/acknowledge", nil)
	rec := httptest.NewRecorder()
	api.handleAcknowledge(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
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
