package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/mcp/anomaly"
)

func newAnomalyTestHarness(t *testing.T) (*server, *anomaly.Detector, *reqRecorder, func()) {
	t.Helper()

	rec := &reqRecorder{responseBody: []byte(`{"acknowledged":true}`)}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.path = r.URL.Path
		rec.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(rec.responseBody)
	}))

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")
	audit, err := NewAuditWriter(auditPath)
	if err != nil {
		ts.Close()
		t.Fatalf("audit writer: %v", err)
	}

	cfg := Config{
		AtlasBaseURL: ts.URL,
		AuditLogPath: auditPath,
		HTTPTimeout:  0,
	}
	metrics := NewMetrics()
	detector := anomaly.NewDetector(anomaly.Config{}, metrics, nil)
	s := &server{
		cfg:      cfg,
		audit:    audit,
		cli:      NewHTTPClient(cfg),
		metrics:  metrics,
		detector: detector,
	}

	cleanup := func() {
		_ = audit.Close()
		ts.Close()
	}
	return s, detector, rec, cleanup
}

func TestHandleAnomalyGetRecent_DefaultLimit(t *testing.T) {
	s, detector, _, done := newAnomalyTestHarness(t)
	defer done()

	for i := range 15 {
		detector.Store().Add(anomaly.AnomalyEvent{TenantID: "t1", AnomalyType: "burst", Score: float64(i)})
	}

	_, out, err := s.handleAnomalyGetRecent(context.Background(), nil, AnomalyGetRecentInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Events) != 10 {
		t.Fatalf("expected default limit 10, got %d", len(out.Events))
	}
}

func TestHandleAnomalyGetRecent_RespectsLimit(t *testing.T) {
	s, detector, _, done := newAnomalyTestHarness(t)
	defer done()

	for i := range 5 {
		detector.Store().Add(anomaly.AnomalyEvent{TenantID: "t1", AnomalyType: "burst", Score: float64(i)})
	}

	_, out, err := s.handleAnomalyGetRecent(context.Background(), nil, AnomalyGetRecentInput{Limit: new(3)})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Events) != 3 {
		t.Fatalf("expected limit 3, got %d", len(out.Events))
	}
}

func TestHandleAnomalyGetRecent_NewestFirst(t *testing.T) {
	s, detector, _, done := newAnomalyTestHarness(t)
	defer done()

	for i := range 3 {
		detector.Store().Add(anomaly.AnomalyEvent{TenantID: "t1", AnomalyType: "burst", Score: float64(i)})
	}

	_, out, err := s.handleAnomalyGetRecent(context.Background(), nil, AnomalyGetRecentInput{Limit: new(3)})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(out.Events))
	}
	// Events are stored in append order; Recent returns newest-last first.
	if out.Events[0].Score != 2 {
		t.Fatalf("expected newest event first, got score %v", out.Events[0].Score)
	}
}

func TestHandleAnomalyAck_OK(t *testing.T) {
	s, _, rec, done := newAnomalyTestHarness(t)
	defer done()

	_, out, err := s.handleAnomalyAck(context.Background(), nil, AnomalyAckInput{AlertID: "alert-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Acknowledged {
		t.Fatal("expected acknowledged=true")
	}
	if rec.path != "/api/alerts/acknowledge" {
		t.Fatalf("path=%s", rec.path)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.body, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["alert_id"] != "alert-42" {
		t.Fatalf("expected alert_id=alert-42, got %v", got)
	}
}

func TestHandleAnomalyAck_MissingID(t *testing.T) {
	s, _, _, done := newAnomalyTestHarness(t)
	defer done()

	_, _, err := s.handleAnomalyAck(context.Background(), nil, AnomalyAckInput{AlertID: ""})
	if err == nil {
		t.Fatal("expected error when alert_id is empty")
	}
}

func TestHandleAnomalyGetRecent_WritesAudit(t *testing.T) {
	s, _, _, done := newAnomalyTestHarness(t)
	defer done()

	if _, _, err := s.handleAnomalyGetRecent(context.Background(), nil, AnomalyGetRecentInput{}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	raw, err := os.ReadFile(s.cfg.AuditLogPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(raw), `"tool":"mcp_anomaly_get_recent"`) {
		t.Fatalf("expected anomaly tool audit entry, got %s", string(raw))
	}
}

func TestHandleAnomalyAck_WritesAudit(t *testing.T) {
	s, _, _, done := newAnomalyTestHarness(t)
	defer done()

	if _, _, err := s.handleAnomalyAck(context.Background(), nil, AnomalyAckInput{AlertID: "alert-1"}); err != nil {
		t.Fatalf("handler: %v", err)
	}

	raw, err := os.ReadFile(s.cfg.AuditLogPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !strings.Contains(string(raw), `"tool":"mcp_anomaly_ack"`) {
		t.Fatalf("expected mcp_anomaly_ack audit entry, got %s", string(raw))
	}
}
