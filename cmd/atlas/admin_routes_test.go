package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrapAdminAuth_HashComparison(t *testing.T) {
	called := false
	handler := wrapAdminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("correct key passes", func(t *testing.T) {
		called = false
		t.Setenv("ATLAS_API_KEY", "test-key")
		req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
		req.Header.Set("X-API-Key", "test-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if !called {
			t.Error("handler should have been called")
		}
	})

	t.Run("wrong key returns 401", func(t *testing.T) {
		called = false
		t.Setenv("ATLAS_API_KEY", "test-key")
		req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
		req.Header.Set("X-API-Key", "wrong-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		if called {
			t.Error("handler should NOT have been called")
		}
	})

	t.Run("production without key returns 503", func(t *testing.T) {
		called = false
		t.Setenv("ATLAS_ENV", "production")
		t.Setenv("ATLAS_API_KEY", "")
		req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", rec.Code)
		}
		if called {
			t.Error("handler should NOT have been called")
		}
	})

	t.Run("bearer token passes", func(t *testing.T) {
		called = false
		t.Setenv("ATLAS_API_KEY", "test-key")
		req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
		req.Header.Set("Authorization", "Bearer test-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if !called {
			t.Error("handler should have been called")
		}
	})

	t.Run("no key set passes through", func(t *testing.T) {
		called = false
		t.Setenv("ATLAS_API_KEY", "")
		req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 (dev mode pass-through), got %d", rec.Code)
		}
		if !called {
			t.Error("handler should have been called (no auth in dev mode)")
		}
	})

	t.Run("production with key + correct bearer passes", func(t *testing.T) {
		called = false
		t.Setenv("ATLAS_ENV", "production")
		t.Setenv("ATLAS_API_KEY", "prod-key")
		req := httptest.NewRequest(http.MethodGet, "/admin/reload-config", nil)
		req.Header.Set("Authorization", "Bearer prod-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
		if !called {
			t.Error("handler should have been called")
		}
	})
}
