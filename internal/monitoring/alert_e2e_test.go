package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestE2E_AlertTriggered exercises the production alert pipeline end-to-end:
// real AlertStore (file-backed in t.TempDir) + AlertAPI.RegisterRoutes on a
// live mux + httptest server. The "channel went stale" trigger condition is
// represented by AlertStore.Save() with Status=AlertStatusTriggered, which is
// what monitoring/stage3_rules.go:evaluateDataStaleness would do in
// production when a data channel's last_fetch_at exceeds the 5-minute
// staleness window. The test then asserts the alert is queryable via the
// public HTTP API and round-trips with the correct Severity/Status/DedupKey.
func TestE2E_AlertTriggered(t *testing.T) {
	store, err := NewAlertStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewAlertStore: %v", err)
	}

	now := time.Now()
	triggered := domain.AlertRecord{
		ID:        "e2e-alert-1",
		Timestamp: now,
		Rule:      "channel_health",
		Severity:  "ERROR",
		Message:   "channel us_yahoo fresh=false (last fetch > 5m ago)",
		Status:    domain.AlertStatusTriggered,
		DedupKey:  "channel_health:us_yahoo",
		Value:     9.5,
		Threshold: 5.0,
	}
	if err := store.Save(triggered); err != nil {
		t.Fatalf("Save triggered alert: %v", err)
	}

	api := NewAlertAPI(store)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Query the production endpoint via real HTTP.
	resp, err := http.Get(srv.URL + "/api/alerts?status=triggered")
	if err != nil {
		t.Fatalf("GET /api/alerts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Alerts []domain.AlertRecord `json:"alerts"`
		Total  int                  `json:"total"`
		Page   int                  `json:"page"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if payload.Total != 1 {
		t.Fatalf("expected total=1 triggered alert, got %d", payload.Total)
	}
	got := payload.Alerts[0]

	// Lock in the round-trip invariants: pipeline emitted → store persisted
	// → HTTP API returned with these exact field values. If a downstream
	// change drops a field or alters the JSON tag, this test will catch it.
	if got.ID != triggered.ID {
		t.Errorf("id mismatch: want %q, got %q", triggered.ID, got.ID)
	}
	if got.Severity != "ERROR" {
		t.Errorf("severity: want ERROR, got %q", got.Severity)
	}
	if got.Status != domain.AlertStatusTriggered {
		t.Errorf("status: want %q, got %q", domain.AlertStatusTriggered, got.Status)
	}
	if got.DedupKey != "channel_health:us_yahoo" {
		t.Errorf("dedup_key round-trip: want %q, got %q", "channel_health:us_yahoo", got.DedupKey)
	}
}
