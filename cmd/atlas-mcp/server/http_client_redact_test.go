package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The credentials in these fixtures are fake.
const (
	fakeMCPBodyKey  = "FAKE-mcp-body-6f7a8b9c1d2e3f40"
	fakeMCPURLKey   = "FAKE-mcp-url-5a6b7c8d9e0f1a2b"
	fakeMCPAPIToken = "FAKE-mcp-apitoken-3c4d5e6f7a8b9c0d"
)

// newRedactionTestClient points a real HttpClient at srv.
func newRedactionTestClient(srv *httptest.Server) *HttpClient {
	return NewHTTPClient(Config{
		AtlasBaseURL: srv.URL,
		APIToken:     fakeMCPAPIToken,
		HTTPTimeout:  5 * time.Second,
	})
}

// TestHTTPClientStatusErrorRedactsBodyAndKeepsStatus covers the non-2xx branch
// of doRaw: the status code is the diagnosis and must survive; the body's
// credential must not (FU-20261016-16). The API token rides in a header and is
// never part of the message, which the third assertion pins.
func TestHTTPClientStatusErrorRedactsBodyAndKeepsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"msg":"Token is illegal.","status":401,"token_tail":"` + fakeMCPBodyKey + `"}`))
	}))
	t.Cleanup(srv.Close)

	err := newRedactionTestClient(srv).Get(context.Background(), "/api/macro/snapshot/latest", nil, nil)
	if err == nil {
		t.Fatal("want an error for HTTP 401, got nil")
	}
	msg := err.Error()
	for _, leak := range []string{fakeMCPBodyKey, fakeMCPAPIToken} {
		if strings.Contains(msg, leak) {
			t.Errorf("error leaks %q: %s", leak, msg)
		}
	}
	if !strings.Contains(msg, "-> 401") || !strings.Contains(msg, "Token is illegal.") {
		t.Errorf("error dropped the diagnosis: %s", msg)
	}
}

// TestHTTPClientStatusErrorMasksQueryCredential is the URL half of the same
// sink: fullURL is printed next to the status, and a credential the caller put
// in the query string must be masked while the parameter NAME stays readable.
func TestHTTPClientStatusErrorMasksQueryCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream exploded"}`))
	}))
	t.Cleanup(srv.Close)

	query := url.Values{"symbol": {"2330"}, "api_key": {fakeMCPURLKey}}
	err := newRedactionTestClient(srv).Get(context.Background(), "/api/stock/quote", query, nil)
	if err == nil {
		t.Fatal("want an error for HTTP 500, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, fakeMCPURLKey) {
		t.Errorf("error leaks the query-string credential: %s", msg)
	}
	if !strings.Contains(msg, "api_key=") || !strings.Contains(msg, "symbol=2330") {
		t.Errorf("error dropped the URL diagnosis: %s", msg)
	}
	if !strings.Contains(msg, "-> 500") {
		t.Errorf("error dropped the status: %s", msg)
	}
}

// TestHTTPClientTransportErrorMasksQueryCredential covers the transport branch:
// the *url.Error text Quotes() the whole URL, so the credential would ride out
// inside "dial tcp …" unless the wrapped error is redacted too.
func TestHTTPClientTransportErrorMasksQueryCredential(t *testing.T) {
	// Port 1 on the loopback refuses connections; no server is started.
	client := NewHTTPClient(Config{AtlasBaseURL: "http://127.0.0.1:1", HTTPTimeout: 2 * time.Second})
	query := url.Values{"api_key": {fakeMCPURLKey}}

	_, err := client.GetRaw(context.Background(), "/api/stock/quote", query)
	if err == nil {
		t.Fatal("want a transport error, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, fakeMCPURLKey) {
		t.Errorf("transport error leaks the query-string credential: %s", msg)
	}
	if !strings.Contains(msg, "api_key=") {
		t.Errorf("transport error dropped the parameter name: %s", msg)
	}
	if !strings.Contains(msg, "HttpClient: do:") {
		t.Errorf("transport error dropped its own context: %s", msg)
	}
}
