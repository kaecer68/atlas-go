package macro

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// STUB-LOCK: HandleChannelsIngest currently always returns geo_ok: false.
// This test pins the broken contract so a future "fix" cannot silently
// change behavior. The real geo ingestion wiring is tracked separately.
func TestHandleChannelsIngest_StubLock_GeoAlwaysFalseOnMacroError(t *testing.T) {
	h := &Handlers{Service: newMacroServiceFailingIngest(t)}
	req := httptest.NewRequest(http.MethodPost, "/api/channels/ingest", nil)
	status, body := h.HandleChannelsIngest(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if geoOK, _ := resp["geo_ok"].(bool); geoOK {
		t.Errorf("STUB-LOCK VIOLATION: geo_ok must be false, got %v (full response: %v)", resp["geo_ok"], resp)
	}
	if geoErr, _ := resp["geo_error"].(string); geoErr == "" {
		t.Error("expected non-empty geo_error explaining why geo_ok is false")
	}
}

func newMacroServiceWithSnapshot(t *testing.T, snap marketdata.MacroDataSnapshot) *service.MacroService {
	t.Helper()
	snapDir := t.TempDir()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	for _, name := range []string{"latest.json", "2026-06-14.json"} {
		if err := os.WriteFile(filepath.Join(snapDir, name), b, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	ingestor := narrative.NewMacroIngestor(okProvider{}, snapDir)
	return service.NewMacroService(t.TempDir(), ingestor, narrative.NewTaiwanStressCalculator(nil, ""))
}

func testMacroSnapshot() marketdata.MacroDataSnapshot {
	now := time.Now().Unix()
	return marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Symbol: "DXY", Value: 104, ChangePct: 0.5, Timestamp: now},
		US10Y:              marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25, ChangePct: -0.2, Timestamp: now},
		VIX:                marketdata.MacroDataPoint{Symbol: "^VIX", Value: 18, ChangePct: 1.1, Timestamp: now},
		USD_TWD:            marketdata.MacroDataPoint{Symbol: "USDTWD", Value: 32.2, ChangePct: 0.1, Timestamp: now},
		Oil:                marketdata.MacroDataPoint{Symbol: "CL=F", Value: 75, ChangePct: -0.4, Timestamp: now},
		Gold:               marketdata.MacroDataPoint{Symbol: "GC=F", Value: 2300, ChangePct: 0.3, Timestamp: now},
		JPY:                marketdata.MacroDataPoint{Symbol: "JPY=X", Value: 155, ChangePct: -0.1, Timestamp: now},
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "FII", Value: 123.4, ChangePct: 0, Timestamp: now},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DFI", Value: -12.3, ChangePct: 0, Timestamp: now},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DLR", Value: 5.6, ChangePct: 0, Timestamp: now},
		RecordedAt:         now,
	}
}

func TestMacroHandlers_SnapshotAndFlowEndpoints(t *testing.T) {
	h := &Handlers{Service: newMacroServiceWithSnapshot(t, testMacroSnapshot())}
	endpoints := []struct {
		name string
		path string
		fn   func(*http.Request) (int, any)
	}{
		{"latest", "/api/macro/snapshot/latest", h.HandleMacroSnapshotLatest},
		{"history", "/api/macro/snapshot/history?date=2026-06-14", h.HandleMacroSnapshotHistory},
		{"timeline", "/api/macro/snapshot/timeline?days=30", h.HandleMacroSnapshotTimeline},
		{"capital_flow", "/api/macro/capital-flow/latest", h.HandleCapitalFlowLatest},
		{"stress", "/api/taiwan/stress-index", h.HandleTaiwanStressIndex},
		{"health", "/api/dashboard/macro-data-health", h.HandleMacroDataHealth},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint.path, nil)
			status, body := endpoint.fn(req)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body=%v)", status, body)
			}
			if _, err := json.Marshal(body); err != nil {
				t.Fatalf("response is not JSON encodable: %v", err)
			}
		})
	}
}

func TestHandleMacroSnapshotHistory_ValidationErrors(t *testing.T) {
	h := &Handlers{Service: newMacroServiceWithSnapshot(t, testMacroSnapshot())}
	cases := []struct {
		name string
		path string
		want int
	}{
		{"missing_date", "/api/macro/snapshot/history", http.StatusBadRequest},
		{"invalid_date", "/api/macro/snapshot/history?date=20260614", http.StatusBadRequest},
		{"not_found", "/api/macro/snapshot/history?date=2026-06-15", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			status, body := h.HandleMacroSnapshotHistory(req)
			if status != tc.want {
				t.Fatalf("status = %d, want %d (body=%v)", status, tc.want, body)
			}
		})
	}
}

func TestHandleMacroSnapshotTimeline_OK(t *testing.T) {
	h := &Handlers{Service: newMacroServiceWithSnapshot(t, testMacroSnapshot())}
	req := httptest.NewRequest(http.MethodGet, "/api/macro/snapshot/timeline?from=2026-06-01&to=2026-06-30", nil)
	status, body := h.HandleMacroSnapshotTimeline(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	for _, key := range []string{"snapshots", "range", "capacity_limit_hit", "missing_dates", "stats"} {
		if _, exists := resp[key]; !exists {
			t.Errorf("response missing key %q", key)
		}
	}
	snapshots, ok := resp["snapshots"].([]service.TimelineEntry)
	if !ok {
		t.Fatalf("expected []TimelineEntry, got %T", resp["snapshots"])
	}
	if len(snapshots) != 1 {
		t.Errorf("expected 1 snapshot from helper fixture, got %d", len(snapshots))
	}
}

func TestHandleMacroSnapshotTimeline_BadDateParams(t *testing.T) {
	h := &Handlers{Service: newMacroServiceWithSnapshot(t, testMacroSnapshot())}
	cases := []struct {
		name string
		path string
		want int
	}{
		{"days_zero", "/api/macro/snapshot/timeline?days=0", http.StatusBadRequest},
		{"days_negative", "/api/macro/snapshot/timeline?days=-5", http.StatusBadRequest},
		{"days_too_large", "/api/macro/snapshot/timeline?days=400", http.StatusBadRequest},
		{"days_non_int", "/api/macro/snapshot/timeline?days=abc", http.StatusBadRequest},
		{"from_days_mutual_excl", "/api/macro/snapshot/timeline?from=2026-04-21&days=10", http.StatusBadRequest},
		{"from_after_to", "/api/macro/snapshot/timeline?from=2026-07-20&to=2026-04-21", http.StatusBadRequest},
		{"from_invalid_format", "/api/macro/snapshot/timeline?from=20260421", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			status, body := h.HandleMacroSnapshotTimeline(req)
			if status != tc.want {
				t.Fatalf("status = %d, want %d (body=%v)", status, tc.want, body)
			}
		})
	}
}

func TestHandleMacroIngest_Error(t *testing.T) {
	h := &Handlers{Service: newMacroServiceFailingIngest(t)}
	req := httptest.NewRequest(http.MethodPost, "/api/macro/ingest", nil)
	status, body := h.HandleMacroIngest(req)
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body=%v)", status, body)
	}
}

func TestHandleChannelsIngest_StubLock_MacroErrorLeavesGeoFalse(t *testing.T) {
	h := &Handlers{Service: newMacroServiceFailingIngest(t)}
	req := httptest.NewRequest(http.MethodPost, "/api/channels/ingest", nil)
	_, body := h.HandleChannelsIngest(req)
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if macroOK, _ := resp["macro_ok"].(bool); macroOK {
		t.Errorf("expected macro_ok=false when service errors, got %v", resp["macro_ok"])
	}
	if macroErr, _ := resp["macro_error"].(string); macroErr == "" {
		t.Error("expected non-empty macro_error when service returns error")
	}
	if geoOK, _ := resp["geo_ok"].(bool); geoOK {
		t.Errorf("STUB-LOCK VIOLATION: geo_ok must be false even when macro errors, got %v", resp["geo_ok"])
	}
}

// newMacroServiceFailingIngest builds a MacroService whose Ingest calls a
// provider that returns an error. This drives HandleChannelsIngest's
// macro-error branch without needing a live data source.
func newMacroServiceFailingIngest(t *testing.T) *service.MacroService {
	t.Helper()
	ingestor := narrative.NewMacroIngestor(errProvider{}, t.TempDir())
	return &service.MacroService{
		WorkDir:       t.TempDir(),
		MacroIngestor: ingestor,
	}
}

// errProvider is a MacroDataProvider that always errors on fetch.
// Used to drive the macro-error branch in HandleChannelsIngest tests.
type errProvider struct{}

func (errProvider) Name() string { return "test-err-provider" }
func (errProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return marketdata.MacroDataSnapshot{}, errors.New("simulated provider failure")
}

func TestHandleChannelsIngest_StubLock_GeoStillFalseOnMacroSuccess(t *testing.T) {
	h := &Handlers{Service: newMacroServiceSucceedingIngest(t)}
	req := httptest.NewRequest(http.MethodPost, "/api/channels/ingest", nil)
	status, body := h.HandleChannelsIngest(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if macroOK, _ := resp["macro_ok"].(bool); !macroOK {
		t.Errorf("expected macro_ok=true on success path, got %v", resp["macro_ok"])
	}
	if macroErr, _ := resp["macro_error"].(string); macroErr != "" {
		t.Errorf("expected empty macro_error on success path, got %q", macroErr)
	}
	if geoOK, _ := resp["geo_ok"].(bool); geoOK {
		t.Errorf("STUB-LOCK VIOLATION: geo_ok must be false even on macro-success path, got %v", resp["geo_ok"])
	}
	if geoErr, _ := resp["geo_error"].(string); geoErr == "" {
		t.Error("expected non-empty geo_error on success path (geo is not wired)")
	}
}

func newMacroServiceSucceedingIngest(t *testing.T) *service.MacroService {
	t.Helper()
	snapDir := t.TempDir()
	ingestor := narrative.NewMacroIngestor(okProvider{}, snapDir)
	return &service.MacroService{
		WorkDir:       t.TempDir(),
		MacroIngestor: ingestor,
	}
}

type okProvider struct{}

func (okProvider) Name() string { return "test-ok-provider" }
func (okProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	now := time.Now().Unix()
	return marketdata.MacroDataSnapshot{
		US10Y:      marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.25, ChangePct: 0.5},
		RecordedAt: now,
	}, nil
}
