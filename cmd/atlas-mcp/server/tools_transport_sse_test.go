package server

// Integration tests verifying that:
//  1. The SSE transport handler from go-sdk can be mounted behind the
//     BearerAuth middleware and accepts requests when the token is valid.
//  2. Bearer auth rejection paths (missing header, wrong token, malformed
//     scheme) return 401 across BOTH the SSE and streamable-HTTP transports
//     (parametrised sub-test).
//  3. Running the full registerTools + registerAuditTools pipeline through a
//     server equipped for HTTP transport still lands RegisteredToolCount
//     inside the 81–83 assertion range used by server.Run().
//
// These tests complement transport_test.go's unit-level coverage of the
// middleware by exercising the real go-sdk handlers over net/http.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestSSE_HandlerMountsOverHTTPServer wires the SSE handler from go-sdk
// behind BearerAuth middleware, exposes it via httptest.NewServer, and
// asserts:
//
//	(a) no-token GET is rejected with 401 — proving BearerAuth is in the
//	    request path (not silently bypassed), and
//	(b) a correctly-tokened GET reaches the SSE handler — non-401, non-5xx
//	    response headers — proving the wiring reaches the go-sdk handler.
//	    The event-stream body never closes by design, so we use a
//	    context-bound timeout to abort the body read after headers land.
func TestSSE_HandlerMountsOverHTTPServer(t *testing.T) {
	auth := NewTokenAuth("integration-test-token")
	impl := &mcp.Implementation{Name: "atlas-mcp-it-sse", Version: "v0.0.1"}
	mcpSrv := mcp.NewServer(impl, nil)
	sse := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return mcpSrv }, nil)
	handler := BearerAuth(auth)(sse)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("missing_token_is_rejected", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("no-token request: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("missing-token SSE: got %d, want 401", resp.StatusCode)
		}
	})

	t.Run("valid_token_reaches_sse_handler", func(t *testing.T) {
		// SSE event streams never close; we cap the body read at 500ms so
		// the test terminates. http.Client.Do returns once headers are
		// received, so the StatusCode check below is what tells us whether
		// the SSE handler (vs the 401 middleware) served the response.
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/", nil)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer integration-test-token")
		resp, err := http.DefaultClient.Do(req)
		// Context-timeout mid-body-read is expected for SSE; treat as "ok".
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("valid-token request: %v", err)
		}
		if resp == nil {
			t.Fatalf("valid-token request: nil response (handler never replied)")
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("valid token rejected as 401 — BearerAuth is gating the SSE route")
		}
		if resp.StatusCode >= 500 {
			t.Fatalf("SSE mount failed with %d (handler reached but errored)", resp.StatusCode)
		}
	})
}

// TestBearerAuth_RejectionPathsAcrossTransports parametrises the three
// bearer-token rejection cases (missing header, wrong token, wrong scheme)
// against both HTTP transports (SSE + streamable-HTTP). Each sub-test
// expects exactly 401. We also confirm the response body contains
// "unauthorized" so operators can debug middleware misconfiguration
// from log scrapes.
func TestBearerAuth_RejectionPathsAcrossTransports(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string // empty = no header set
	}{
		{name: "missing_header", authHeader: ""},
		{name: "wrong_token", authHeader: "Bearer not-the-right-token"},
		{name: "wrong_scheme", authHeader: "Basic dXNlcjpwYXNz"},
		{name: "lowercase_bearer", authHeader: "bearer integration-test-token"}, // RFC 6750 §2.1 — case-sensitive
	}

	transports := []struct {
		name  string
		mount func(srv *mcp.Server) http.Handler
	}{
		{
			name: "sse",
			mount: func(srv *mcp.Server) http.Handler {
				return mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, nil)
			},
		},
		{
			name: "streamable-http",
			mount: func(srv *mcp.Server) http.Handler {
				return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
			},
		},
	}

	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			auth := NewTokenAuth("integration-test-token")
			impl := &mcp.Implementation{Name: "atlas-mcp-it-" + tr.name, Version: "v0.0.1"}
			mcpSrv := mcp.NewServer(impl, nil)
			handler := BearerAuth(auth)(tr.mount(mcpSrv))

			srv := httptest.NewServer(handler)
			defer srv.Close()

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/", nil)
					if err != nil {
						t.Fatalf("build request: %v", err)
					}
					if tc.authHeader != "" {
						req.Header.Set("Authorization", tc.authHeader)
					}
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatalf("do: %v", err)
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusUnauthorized {
						t.Fatalf("transport=%s case=%s: got %d body=%q, want 401",
							tr.name, tc.name, resp.StatusCode, string(body))
					}
					// http.Error() writes "unauthorized\n"; substring match is enough for log greps.
					if got := string(body); !contains(got, "unauthorized") {
						t.Fatalf("transport=%s case=%s: body %q missing 'unauthorized'", tr.name, tc.name, got)
					}
				})
			}
		})
	}
}

// TestRegisteredToolCount_RemainsInRangeAfterHTTPTransportRegistration
// asserts that running registerTools + registerAuditTools through a
// server whose Config.Transport is set to an HTTP transport (the path
// used in production by SSE and streamable-HTTP deployments) still
// produces 103–105 business tools (97 base + roots (2) + template_detector (2) +
// sector (2) + 0-2 sampling/elicitation + 1 stock_get_monthly_revenue added 2026-08-07)
// + 4 audit tools = 114–117 total,
// matching the assertion in server.Run(). The delta check keeps the test robust
// against other tests in the package that may have already advanced the
// package-level counter.
func TestRegisteredToolCount_RemainsInRangeAfterHTTPTransportRegistration(t *testing.T) {
	audit := newTestAuditWriter(t)
	srv := &server{
		cfg: Config{
			AtlasBaseURL: "http://127.0.0.1:0",
			AuditLogPath: audit.path,
			// Production HTTP-transport deployments set Transport up front
			// so the transport dispatch in server.Run() picks the HTTP path.
			Transport: TransportSSE,
			Addr:      "127.0.0.1:0",
		},
		audit: audit,
		cli:   NewHTTPClient(Config{AtlasBaseURL: "http://127.0.0.1:0"}),
	}

	impl := &mcp.Implementation{Name: "atlas-mcp-it-registration", Version: "v0.0.1"}
	mcpSrv := mcp.NewServer(impl, nil)

	before := RegisteredToolCount
	registerTools(mcpSrv, srv)
	registerAuditTools(mcpSrv, srv)
	delta := RegisteredToolCount - before

	if delta < 114 || delta > 117 {
		t.Fatalf("tool count drift in HTTP-transport registration: delta=%d (total=%d), expected 114-117 (stock_get_monthly_revenue added 2026-08-07)",
			delta, RegisteredToolCount)
	}
}

// contains is a tiny strings.Contains replacement to keep this file's
// import block minimal (transport_test.go already imports strings, but we
// avoid that dep here since the test only needs one substring check).
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
