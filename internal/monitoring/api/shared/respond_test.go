package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON_NoContent_DoesNotWriteBody(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()

	WriteJSON(rr, http.StatusNoContent, nil)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("expected empty body for 204 response, got %q", rr.Body.String())
	}
}

func TestWriteJSON_OK_EncodesPayload(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()

	WriteJSON(rr, http.StatusOK, map[string]string{"status": "ok"})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("expected non-empty body for 200 response")
	}
}

func TestWriteJSONError_BackwardCompat_OnlyErrorField(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	WriteJSONError(rr, http.StatusInternalServerError, "something broke")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if body["error"] != "something broke" {
		t.Fatalf("expected error=%q, got %v", "something broke", body["error"])
	}
	if _, hasCode := body["code"]; hasCode {
		t.Fatalf("WriteJSONError should not emit code field, got body=%v", body)
	}
}

func TestWriteJSONErrorEx_WithCode_EmitsBothFields(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	WriteJSONErrorEx(rr, http.StatusBadRequest, "invalid_symbol", "symbol must be alphanumeric")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if body["error"] != "symbol must be alphanumeric" {
		t.Fatalf("expected error=%q, got %v", "symbol must be alphanumeric", body["error"])
	}
	if body["code"] != "invalid_symbol" {
		t.Fatalf("expected code=%q, got %v", "invalid_symbol", body["code"])
	}
}

func TestWriteJSONErrorEx_EmptyCode_OmitsField(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	WriteJSONErrorEx(rr, http.StatusServiceUnavailable, "", "no api key configured")

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if body["error"] != "no api key configured" {
		t.Fatalf("expected error=%q, got %v", "no api key configured", body["error"])
	}
	if _, hasCode := body["code"]; hasCode {
		t.Fatalf("empty code should be omitted, got body=%v", body)
	}
}
