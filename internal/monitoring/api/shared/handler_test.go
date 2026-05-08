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
