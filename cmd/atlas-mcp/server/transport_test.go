package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"Bearer ", ""},
		{"Bearer abc", "abc"},
		{"Bearer abc def", "abc def"},
		{"bearer abc", ""}, // case-sensitive per RFC 6750 §2.1
		{"Basic abc", ""},
		{"Bearer", ""}, // missing space
	}
	for _, c := range cases {
		if got := extractBearer(c.in); got != c.want {
			t.Errorf("extractBearer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBearerAuth_MissingHeader verifies the middleware rejects requests
// with no Authorization header when a token is configured.
func TestBearerAuth_MissingHeader(t *testing.T) {
	auth := NewTokenAuth("secret-token")
	handler := BearerAuth(auth)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unauthorized") {
		t.Errorf("expected body to contain \"unauthorized\", got %q", rec.Body.String())
	}
}

// TestBearerAuth_InvalidToken verifies the middleware rejects a wrong token.
func TestBearerAuth_InvalidToken(t *testing.T) {
	auth := NewTokenAuth("secret-token")
	called := false
	handler := BearerAuth(auth)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("downstream handler was invoked despite invalid token")
	}
}

// TestBearerAuth_ValidToken verifies the middleware passes through a
// matching token and the downstream handler is invoked.
func TestBearerAuth_ValidToken(t *testing.T) {
	auth := NewTokenAuth("secret-token")
	called := false
	handler := BearerAuth(auth)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Fatal("downstream handler not invoked with valid token")
	}
}

// TestBearerAuth_DevMode verifies that an empty TokenAuth (dev mode)
// allows all requests through without auth.
func TestBearerAuth_DevMode(t *testing.T) {
	auth := NewTokenAuth("") // dev mode
	called := false
	handler := BearerAuth(auth)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Fatal("downstream handler not invoked in dev mode")
	}
}

// TestServeStdio_NilServer ensures guard clauses reject nil server.
func TestServeStdio_NilServer(t *testing.T) {
	if err := ServeStdio(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil server, got nil")
	}
}

// TestServeSSE_NilServerAndAddr covers both required-field guards.
func TestServeSSE_NilServerAndAddr(t *testing.T) {
	impl := &mcp.Implementation{Name: "atlas-mcp-test", Version: "v0.0.0"}
	mcpSrv := mcp.NewServer(impl, nil)

	if err := ServeSSE(context.Background(), nil, "127.0.0.1:9090", NewTokenAuth("")); err == nil {
		t.Error("expected error for nil server, got nil")
	}
	if err := ServeSSE(context.Background(), mcpSrv, "", NewTokenAuth("")); err == nil {
		t.Error("expected error for empty addr, got nil")
	}
}

// TestServeStreamableHTTP_NilServerAndAddr covers both required-field guards.
func TestServeStreamableHTTP_NilServerAndAddr(t *testing.T) {
	impl := &mcp.Implementation{Name: "atlas-mcp-test", Version: "v0.0.0"}
	mcpSrv := mcp.NewServer(impl, nil)

	if err := ServeStreamableHTTP(context.Background(), nil, "127.0.0.1:9090", NewTokenAuth("")); err == nil {
		t.Error("expected error for nil server, got nil")
	}
	if err := ServeStreamableHTTP(context.Background(), mcpSrv, "", NewTokenAuth("")); err == nil {
		t.Error("expected error for empty addr, got nil")
	}
}
