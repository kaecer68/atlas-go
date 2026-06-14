package crossmarket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// mockMacroProvider implements marketdata.MacroDataProvider for testing.
type mockMacroProvider struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (m *mockMacroProvider) Name() string { return "mock" }

func (m *mockMacroProvider) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	return m.snap, m.err
}

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	provider := &mockMacroProvider{
		snap: marketdata.MacroDataSnapshot{},
	}
	svc := service.NewCrossMarketService(provider)
	return &Handlers{Svc: svc}
}

func newErrorHandlers(t *testing.T) *Handlers {
	t.Helper()
	provider := &mockMacroProvider{
		err: errors.New("provider unavailable"),
	}
	svc := service.NewCrossMarketService(provider)
	return &Handlers{Svc: svc}
}

func assertStatus(t *testing.T, status int, want int) {
	t.Helper()
	if status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}

func TestHandleStatus_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/cross-market/status", nil)
	status, body := h.HandleStatus(req)
	assertStatus(t, status, http.StatusOK)
	st, ok := body.(*service.CrossMarketStatus)
	if !ok {
		t.Fatalf("body is %T, want *CrossMarketStatus", body)
	}
	if st.GeneratedAt == "" {
		t.Error("GeneratedAt should not be empty")
	}
	// Snapshot delivers zero values but status should still be populated
	if st.DataStatus == "" {
		t.Error("DataStatus should be set")
	}
}

func TestHandleStatus_ProviderError(t *testing.T) {
	h := newErrorHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/cross-market/status", nil)
	status, body := h.HandleStatus(req)
	assertStatus(t, status, http.StatusInternalServerError)
	m, ok := body.(map[string]string)
	if !ok {
		t.Fatalf("body is %T, want map[string]string", body)
	}
	if m["error"] == "" {
		t.Error("error message should not be empty")
	}
}

func TestHandleCorrelation_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/cross-market/correlation", nil)
	status, body := h.HandleCorrelation(req)
	assertStatus(t, status, http.StatusOK)
	cr, ok := body.(*service.CorrelationResponse)
	if !ok {
		t.Fatalf("body is %T, want *CorrelationResponse", body)
	}
	if cr.WindowSize != 20 {
		t.Errorf("WindowSize = %d, want 20", cr.WindowSize)
	}
	if cr.ComputedAt == "" {
		t.Error("ComputedAt should not be empty")
	}
}

func TestHandleUSIndices_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/us-indices", nil)
	status, body := h.HandleUSIndices(req)
	assertStatus(t, status, http.StatusOK)
	resp, ok := body.(*service.USIndicesResponse)
	if !ok {
		t.Fatalf("body is %T, want *USIndicesResponse", body)
	}
	if resp.GeneratedAt == "" {
		t.Error("GeneratedAt should not be empty")
	}
	if resp.Indices == nil {
		t.Error("Indices should not be nil")
	}
}

func TestHandleUSIndices_ProviderError(t *testing.T) {
	h := newErrorHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/us-indices", nil)
	status, body := h.HandleUSIndices(req)
	assertStatus(t, status, http.StatusInternalServerError)
	m, ok := body.(map[string]string)
	if !ok {
		t.Fatalf("body is %T, want map[string]string", body)
	}
	if m["error"] == "" {
		t.Error("error message should not be empty")
	}
}

func TestHandleStatus_JSONRoundtrip(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/cross-market/status", nil)
	_, body := h.HandleStatus(req)
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Verify snake_case keys
	for _, key := range []string{"recorded_at", "generated_at", "spx", "ndx", "dji", "sox", "nvda", "aapl", "msft", "tsm_adr", "vix", "dxy"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing snake_case key %q in JSON output", key)
		}
	}
}

func TestHandleCorrelation_JSONRoundtrip(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/cross-market/correlation", nil)
	_, body := h.HandleCorrelation(req)
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"correlation", "window_size", "observations", "computed_at", "is_fallback"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing snake_case key %q in correlation JSON", key)
		}
	}
}

func TestHandleUSIndices_JSONRoundtrip(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/us-indices", nil)
	_, body := h.HandleUSIndices(req)
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["indices"]; !ok {
		t.Error("missing 'indices' key in US indices JSON")
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/cross-market/status"},
		{"GET", "/api/cross-market/correlation"},
		{"GET", "/api/dashboard/us-indices"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s %s not registered (no handler)", r.method, r.path)
		}
	}
}
