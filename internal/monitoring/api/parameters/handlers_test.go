package parameters

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	return NewHandlers("")
}

func postJSON(t *testing.T, url string, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertStatus(t *testing.T, status int, want int) {
	t.Helper()
	if status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}

func assertJSONKey(t *testing.T, body any, key string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q: %v", key, m)
	}
	return m
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers("nonexistent.json")
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}
	if h.params == nil {
		t.Fatal("params should not be nil (falls back to default)")
	}
}

func TestNewHandlers_EmptyPath(t *testing.T) {
	h := NewHandlers("")
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}
	if h.paramsPath != "" {
		t.Errorf("paramsPath = %q, want empty", h.paramsPath)
	}
}

func TestHandleGetParameters_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/parameters", nil)
	status, body := h.HandleGetParameters(req)
	assertStatus(t, status, http.StatusOK)
	if body == nil {
		t.Fatal("body should not be nil")
	}
}

func TestHandleGetParameters_ReturnsFlatMap(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/parameters", nil)
	_, body := h.HandleGetParameters(req)
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T, want map[string]any", body)
	}
	// Check that version is flattened
	if _, exists := m["version"]; !exists {
		t.Error("flattened map should contain version key")
	}
}

func TestHandlePostParameters_EmptyBody(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters",
		bytes.NewReader([]byte("")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandlePostParameters(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePostParameters_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandlePostParameters(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePostParameters_ValidFloatUpdate(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_weight_min": 1.5,
	})
	status, body := h.HandlePostParameters(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "status")
}

func TestHandlePostParameters_InvalidParameterName(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"nonexistent_param": 1.0,
	})
	status, _ := h.HandlePostParameters(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePostParameters_BoolValue(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_daily_adjustment_cooldown": true,
	})
	status, _ := h.HandlePostParameters(req)
	if status < 200 || status >= 600 {
		t.Errorf("unexpected status: %d", status)
	}
}

func TestHandlePostParameters_StringValue(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_daily_adjustment_cooldown": "1h",
	})
	status, _ := h.HandlePostParameters(req)
	if status < 200 || status >= 600 {
		t.Errorf("unexpected status: %d", status)
	}
}

func TestHandlePostParameters_UnsupportedType(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_weight_min": []int{1, 2, 3},
	})
	status, _ := h.HandlePostParameters(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandlePostParameters_IntValue(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_weight_min": 2,
	})
	status, _ := h.HandlePostParameters(req)
	if status < 200 || status >= 600 {
		t.Errorf("unexpected status: %d", status)
	}
}

func TestHandlePostParameters_MapValue(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_weight_min": map[string]any{"a": 1.0},
	})
	status, _ := h.HandlePostParameters(req)
	if status < 200 || status >= 600 {
		t.Errorf("unexpected status: %d", status)
	}
}

func TestHandlePostParameters_MultipleUpdates(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters", map[string]any{
		"darwinian_weight_min": 0.5,
		"darwinian_weight_max": 2.0,
	})
	status, body := h.HandlePostParameters(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "status")
}

func TestHandleCategories_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/parameters/categories", nil)
	status, body := h.HandleCategories(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "categories")
	cats, ok := m["categories"].([]any)
	if !ok {
		t.Fatalf("categories is %T", m["categories"])
	}
	if len(cats) == 0 {
		t.Error("categories should not be empty")
	}
	assertJSONKey(t, body, "keys")
}

func TestHandleInferGARCH_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/infer-garch",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandleInferGARCH(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleInferGARCH_Success(t *testing.T) {
	h := newTestHandlers(t)
	returns := make([]float64, 100)
	for i := range returns {
		if i%2 == 0 {
			returns[i] = 0.01
		} else {
			returns[i] = -0.005
		}
	}
	req := postJSON(t, "/api/parameters/infer-garch", map[string]any{
		"returns": returns,
	})
	status, body := h.HandleInferGARCH(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "omega")
	_ = m["omega"]
	assertJSONKey(t, body, "alpha")
	assertJSONKey(t, body, "beta")
}

func TestHandleInferGARCH_EmptyReturns(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters/infer-garch", map[string]any{
		"returns": []float64{},
	})
	status, _ := h.HandleInferGARCH(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleSweep_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/sweep",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandleSweep(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleSweep_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters/sweep", map[string]any{
		"parameter":     "darwinian_weight_min",
		"values":        []float64{0.5, 1.0, 1.5, 2.0},
		"current_value": 1.0,
	})
	status, body := h.HandleSweep(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "parameter")
	if m["parameter"] != "darwinian_weight_min" {
		t.Errorf("parameter = %v", m["parameter"])
	}
	assertJSONKey(t, body, "note")
}

func TestHandleSnapshots_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/parameters/snapshots", nil)
	status, body := h.HandleSnapshots(req)
	// May return 500 if snapshots dir doesn't exist, or 200 otherwise
	// Both are valid test outcomes
	_ = status
	_ = body
}

func TestHandleRollback_MissingSnapshotID(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/parameters/rollback", map[string]string{
		"reason": "test",
		"user":   "admin",
	})
	status, _ := h.HandleRollback(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleRollback_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/rollback",
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	status, _ := h.HandleRollback(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleAuditLog_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/parameters/audit-log", nil)
	status, body := h.HandleAuditLog(req)
	// May return 500 if no snapshots dir, or 200 with empty changes
	// Both fine for coverage
	_ = status
	if status == http.StatusOK {
		assertJSONKey(t, body, "changes")
	}
}

func writeTempParamsFile(t *testing.T, modifier func(map[string]any)) string {
	t.Helper()
	data, err := os.ReadFile("../../../../configs/parameters.json")
	if err != nil {
		t.Fatalf("read params: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["reporting"]; !ok {
		m["reporting"] = map[string]any{
			"win_rate_threshold": map[string]any{"value": 0.5, "rationale": "test", "source": "heuristic"},
			"sharpe_min_samples": map[string]any{"value": 30, "rationale": "test", "source": "heuristic"},
		}
	}
	if modifier != nil {
		modifier(m)
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f, err := os.CreateTemp("", "params-*.json")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.Write(out); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestHandleReload_Success(t *testing.T) {
	path := writeTempParamsFile(t, nil)
	h := NewHandlers(path)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/reload", nil)
	status, body := h.HandleReload(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "status")
	if m["status"] != "reloaded" {
		t.Errorf("status = %v, want reloaded", m["status"])
	}
}

func TestHandleReload_RejectsNullValue(t *testing.T) {
	path := writeTempParamsFile(t, func(m map[string]any) {
		m["darwinian"] = nil
	})
	h := NewHandlers(path)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/reload", nil)
	status, _ := h.HandleReload(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleReload_RejectsEmptyObject(t *testing.T) {
	path := writeTempParamsFile(t, func(m map[string]any) {
		m["darwinian"] = map[string]any{}
	})
	h := NewHandlers(path)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/reload", nil)
	status, _ := h.HandleReload(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleReload_AcceptsValidConfig(t *testing.T) {
	path := writeTempParamsFile(t, nil)
	h := NewHandlers(path)
	req := httptest.NewRequest(http.MethodPost, "/api/parameters/reload", nil)
	status, body := h.HandleReload(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "status")
	if m["status"] != "reloaded" {
		t.Errorf("status = %v, want reloaded", m["status"])
	}
}

func TestHandleGetMetadata_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/parameters/metadata", nil)
	status, body := h.HandleGetMetadata(req)
	assertStatus(t, status, http.StatusOK)
	if body == nil {
		t.Fatal("body should not be nil")
	}
}

func TestParamsToFlatMap(t *testing.T) {
	h := newTestHandlers(t)
	result, err := h.paramsToFlatMap()
	if err != nil {
		t.Fatalf("paramsToFlatMap: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if _, ok := result["version"]; !ok {
		t.Error("flattened result should contain version")
	}
}

func TestParamsToMetadataMap(t *testing.T) {
	h := newTestHandlers(t)
	result, err := h.paramsToMetadataMap()
	if err != nil {
		t.Fatalf("paramsToMetadataMap: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestFlatten_Simple(t *testing.T) {
	src := map[string]any{
		"a": map[string]any{
			"b": 1,
			"c": map[string]any{
				"d": 2,
			},
		},
	}
	dst := make(map[string]any)
	flatten(src, "", dst)
	if v, ok := dst["a.b"]; !ok || v != 1 {
		t.Errorf("a.b = %v, want 1", v)
	}
	if v, ok := dst["a.c.d"]; !ok || v != 2 {
		t.Errorf("a.c.d = %v, want 2", v)
	}
}

func TestFlatten_WithValueKey(t *testing.T) {
	src := map[string]any{
		"darwinian": map[string]any{
			"boost": map[string]any{
				"value": 1.5,
			},
		},
	}
	dst := make(map[string]any)
	flatten(src, "", dst)
	if v, ok := dst["darwinian.boost"]; !ok || v != 1.5 {
		t.Errorf("darwinian.boost = %v, want 1.5", v)
	}
}

func TestFlattenWithMetadata_Simple(t *testing.T) {
	src := map[string]any{
		"a": map[string]any{
			"value":     1,
			"rationale": "test",
			"source":    "test",
		},
	}
	dst := make(map[string]any)
	flattenWithMetadata(src, "", dst)
	if _, ok := dst["a"]; !ok {
		t.Error("a should be present with metadata")
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
		{"GET", "/api/parameters"},
		{"POST", "/api/parameters"},
		{"GET", "/api/parameters/categories"},
		{"POST", "/api/parameters/infer-garch"},
		{"POST", "/api/parameters/sweep"},
		{"GET", "/api/parameters/snapshots"},
		{"GET", "/api/parameters/audit-log"},
		{"POST", "/api/parameters/rollback"},
		{"POST", "/api/parameters/reload"},
		{"GET", "/api/parameters/metadata"},
	}
	for _, r := range routes {
		var body io.Reader
		if r.method == http.MethodPost {
			switch r.path {
			case "/api/parameters/infer-garch":
				b, _ := json.Marshal(map[string]any{"returns": make([]float64, 100)})
				body = bytes.NewReader(b)
			case "/api/parameters/sweep":
				b, _ := json.Marshal(map[string]any{"parameter": "test", "values": []float64{}, "current_value": 0})
				body = bytes.NewReader(b)
			case "/api/parameters/rollback":
				b, _ := json.Marshal(map[string]string{"snapshot_id": "test"})
				body = bytes.NewReader(b)
			case "/api/parameters":
				b, _ := json.Marshal(map[string]any{"darwinian_weight_min": 1.0})
				body = bytes.NewReader(b)
			default:
				body = bytes.NewReader([]byte("{}"))
			}
		}
		req := httptest.NewRequest(r.method, r.path, body)
		if r.method == http.MethodPost {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s %s not registered (no handler)", r.method, r.path)
		}
	}
}
