package subscription

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newWaitlistHandler(t *testing.T) (*Handler, *WaitlistStore) {
	t.Helper()
	s := newTestStore(t)
	jwt := NewJWTManager("test-secret", "")
	path := filepath.Join(t.TempDir(), "waitlist.jsonl")
	h := NewHandler(s, jwt).WithWaitlist(path)
	return h, h.waitlist
}

func postWaitlist(t *testing.T, h *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/waitlist", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.handleWaitlist(rec, req)
	return rec
}

func TestWaitlistAddAndDedup(t *testing.T) {
	h, store := newWaitlistHandler(t)

	rec := postWaitlist(t, h, `{"email":"lead@example.com","source":"premium"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["already_registered"] != false {
		t.Errorf("first add should report already_registered=false, got %v", resp["already_registered"])
	}

	// Duplicate (different case) is accepted but flagged, and not appended twice.
	rec = postWaitlist(t, h, `{"email":"LEAD@example.com"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("dup: expected 202, got %d", rec.Code)
	}
	resp = nil
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["already_registered"] != true {
		t.Errorf("dup should report already_registered=true, got %v", resp["already_registered"])
	}
	if got := store.Count(); got != 1 {
		t.Errorf("store should hold 1 entry after dup, got %d", got)
	}
}

func TestWaitlistRejectsInvalidInput(t *testing.T) {
	h, store := newWaitlistHandler(t)

	for _, body := range []string{
		`{"email":"not-an-email"}`,
		`{"email":""}`,
	} {
		rec := postWaitlist(t, h, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, rec.Code)
		}
	}

	rec := postWaitlist(t, h, `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed json: expected 400, got %d", rec.Code)
	}
	if got := store.Count(); got != 0 {
		t.Errorf("invalid submissions must not persist, got %d entries", got)
	}
}
