package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdapt_Success(t *testing.T) {
	h := Adapt(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"status": "ok"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("body.status = %q, want \"ok\"", body["status"])
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want \"application/json\"", ct)
	}
}

func TestAdapt_Error(t *testing.T) {
	h := Adapt(func(r *http.Request) (int, any) {
		return http.StatusBadRequest, map[string]string{"error": "bad input"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "bad input" {
		t.Errorf("body.error = %q, want \"bad input\"", body["error"])
	}
}

func TestGet_AllowsGet(t *testing.T) {
	h := Get(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "get"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGet_RejectsPost(t *testing.T) {
	h := Get(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "get"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "method not allowed" {
		t.Errorf("body.error = %q, want \"method not allowed\"", body["error"])
	}
}

func TestPost_AllowsPost(t *testing.T) {
	h := Post(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "post"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPost_RejectsGet(t *testing.T) {
	h := Post(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"method": "post"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "method not allowed" {
		t.Errorf("body.error = %q, want \"method not allowed\"", body["error"])
	}
}

func TestAdaptRaw_AlreadyWrote(t *testing.T) {
	h := AdaptRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>hello</html>"))
		return 0, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want \"text/html\"", ct)
	}
	if body := w.Body.String(); body != "<html>hello</html>" {
		t.Errorf("body = %q, want \"<html>hello</html>\"", body)
	}
}

func TestAdaptRaw_FallbackJSON(t *testing.T) {
	h := AdaptRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"key": "value"}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["key"] != "value" {
		t.Errorf("body.key = %q, want \"value\"", body["key"])
	}
}

func TestGetRaw_AllowsGet(t *testing.T) {
	h := GetRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		w.Write([]byte("raw"))
		return 0, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "raw" {
		t.Errorf("body = %q, want \"raw\"", body)
	}
}

func TestGetRaw_RejectsPost(t *testing.T) {
	h := GetRaw(func(w http.ResponseWriter, r *http.Request) (int, any) {
		w.Write([]byte("raw"))
		return 0, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestAdapt_NoContent_EmptyBody(t *testing.T) {
	h := Adapt(func(r *http.Request) (int, any) {
		return http.StatusNoContent, nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "" {
		t.Errorf("body = %q, want empty for 204 No Content", body)
	}
}

func TestAuthMiddleware_ProductionRequiresAPIKey(t *testing.T) {
	t.Setenv("ATLAS_ENV", "production")
	t.Setenv("ATLAS_API_KEY", "")
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run when production key is missing")
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestAuthMiddleware_AcceptsBearerAndRejectsBadKey(t *testing.T) {
	t.Setenv("ATLAS_ENV", "")
	t.Setenv("ATLAS_API_KEY", "secret")
	h := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/test", nil))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad key status = %d, want %d", bad.Code, http.StatusUnauthorized)
	}

	good := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer secret")
	h.ServeHTTP(good, req)
	if good.Code != http.StatusTeapot {
		t.Fatalf("good key status = %d, want %d", good.Code, http.StatusTeapot)
	}
}

func TestRequireAdmin(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "admin-secret")
	h := RequireAdmin(func(r *http.Request) (int, any) {
		return http.StatusAccepted, map[string]string{"ok": "true"}
	})

	status, body := h(httptest.NewRequest(http.MethodPost, "/admin", nil))
	if status != http.StatusUnauthorized {
		t.Fatalf("missing admin status = %d, want 401 (body=%v)", status, body)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("Authorization", "Admin admin-secret")
	status, _ = h(req)
	if status != http.StatusAccepted {
		t.Fatalf("authorized status = %d, want 202", status)
	}
}

func TestAdminPost_RequiresPostAndAdmin(t *testing.T) {
	t.Setenv("ATLAS_ADMIN_KEY", "admin-secret")
	h := AdminPost(func(r *http.Request) (int, any) {
		return http.StatusOK, map[string]string{"status": "ok"}
	})

	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", get.Code)
	}

	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/admin", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("X-Admin-Key", "admin-secret")
	h.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want 200", authorized.Code)
	}
}
