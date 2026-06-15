package shared

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireAdmin_NoKey_PassesThrough(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "")
	h := RequireAdmin(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"admin": "ok"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	status, data := h(req)
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
	_ = w
	_ = data
}

func TestRequireAdmin_MissingHeader(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "secret")
	h := RequireAdmin(func(r *http.Request) (int, any) {
		return http.StatusOK, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	status, data := h(req)
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", status, http.StatusUnauthorized)
	}
	m := data.(map[string]string)
	if m["error"] != "admin access required" {
		t.Errorf("error = %q, want %q", m["error"], "admin access required")
	}
}

func TestRequireAdmin_ValidHeader(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "secret")
	h := RequireAdmin(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"result": "done"}
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("X-Admin-Key", "secret")
	status, data := h(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	m := data.(map[string]string)
	if m["result"] != "done" {
		t.Errorf("result = %q, want %q", m["result"], "done")
	}
}

func TestRequireAdmin_AuthorizationHeader(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "secret")
	h := RequireAdmin(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"result": "done"}
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Authorization", "Admin secret")
	status, _ := h(req)
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

func TestAdminPost_MethodEnforced(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "")
	handler := AdminPost(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "post"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestWriteJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSONError(rr, http.StatusBadRequest, "bad request")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"error":"bad request"`) {
		t.Errorf("body = %q, want error payload", body)
	}
}
