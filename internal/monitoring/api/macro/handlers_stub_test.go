package macro

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
