package shared

import (
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
