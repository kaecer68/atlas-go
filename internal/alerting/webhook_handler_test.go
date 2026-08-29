package alerting

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAlertWebhookHandler_AcceptsPayload(t *testing.T) {
	h := NewAlertWebhookHandler(100)
	body := AlertmanagerPayload{
		Version:  "4",
		Status:   "firing",
		Receiver: "atlas",
		Alerts: []AlertmanagerAlert{
			{
				Status: "firing",
				Labels: map[string]string{"alertname": "HighErrorRate", "severity": "critical"},
				Annotations: map[string]string{
					"summary":     "elevated error rate",
					"description": "5xx errors > 5% for 10m",
				},
			},
		},
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", bytes.NewReader(buf))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := h.Len(); got != 1 {
		t.Fatalf("expected 1 alert retained, got %d", got)
	}
	recent := h.Recent(1)
	if len(recent) != 1 || recent[0].Labels["alertname"] != "HighErrorRate" {
		t.Fatalf("unexpected recent: %+v", recent)
	}
}

func TestAlertWebhookHandler_RejectsNonPost(t *testing.T) {
	h := NewAlertWebhookHandler(100)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/alerts", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestAlertWebhookHandler_RejectsBadJSON(t *testing.T) {
	h := NewAlertWebhookHandler(100)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", bytes.NewReader([]byte("not-json")))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if got := h.Len(); got != 0 {
		t.Fatalf("expected 0 retained, got %d", got)
	}
}

func TestAlertWebhookHandler_CapacityTrims(t *testing.T) {
	h := NewAlertWebhookHandler(3)
	for i := range 5 {
		body, _ := json.Marshal(AlertmanagerPayload{Alerts: []AlertmanagerAlert{{Labels: map[string]string{"i": itoa(i)}}}})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
	}
	if got := h.Len(); got != 3 {
		t.Fatalf("expected cap=3 retained, got %d", got)
	}
	recent := h.Recent(10)
	if recent[0].Labels["i"] != "2" || recent[2].Labels["i"] != "4" {
		t.Fatalf("expected oldest retained [2..4], got [0]=%s [2]=%s", recent[0].Labels["i"], recent[2].Labels["i"])
	}
}

func TestAlertWebhookHandler_ConcurrentSafe(t *testing.T) {
	h := NewAlertWebhookHandler(1000)
	body, _ := json.Marshal(AlertmanagerPayload{Alerts: []AlertmanagerAlert{{Labels: map[string]string{"a": "b"}}}})
	done := make(chan struct{})
	for range 20 {
		go func() {
			for range 5 {
				req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", bytes.NewReader(body))
				rr := httptest.NewRecorder()
				h.ServeHTTP(rr, req)
			}
			done <- struct{}{}
		}()
	}
	for range 20 {
		<-done
	}
	if got := h.Len(); got != 100 {
		t.Fatalf("expected 100 alerts retained, got %d", got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	const digits = "0123456789"
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return string(b)
}

func TestAlertWebhookHandler_GetReturnsRecent(t *testing.T) {
	h := NewAlertWebhookHandler(100)

	body := AlertmanagerPayload{
		Version:  "4",
		Status:   "firing",
		Receiver: "atlas",
		Alerts: []AlertmanagerAlert{
			{
				Status:      "firing",
				Labels:      map[string]string{"alertname": "TestAlert", "severity": "critical"},
				Annotations: map[string]string{"summary": "test alert"},
			},
		},
	}
	buf, _ := json.Marshal(body)
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", bytes.NewReader(buf))
	h.ServeHTTP(httptest.NewRecorder(), postReq)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/alerts?n=10", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, getReq)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp struct {
		Alerts []AlertmanagerAlert `json:"alerts"`
		Count  int                 `json:"count"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Count)
	}
	if len(resp.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(resp.Alerts))
	}
	if resp.Alerts[0].Labels["alertname"] != "TestAlert" {
		t.Errorf("unexpected alert: %+v", resp.Alerts[0])
	}
}
