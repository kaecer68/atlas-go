package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// l24Key is a fake credential shaped like the one an upstream echoes in its
// rejection body.
const l24Key = "FAKE-l24-preflight-key-5d6e7f8a9b0c1d2e"

// TestLLMHealthCheck_Non200BodyRedacted is the FU-20261016-16 probe for this
// preflight tool: the check message goes to the operator's terminal.
func TestLLMHealthCheck_Non200BodyRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"msg":"router upstream refused","status":502,"api_key":"` + l24Key + `"}`))
	}))
	t.Cleanup(srv.Close)

	got := checkLLMHealth(srv.URL)
	t.Logf("llm health -> %s", got.Message)
	if strings.Contains(got.Message, l24Key) {
		t.Errorf("llm health message leaks the upstream credential: %s", got.Message)
	}
	if !strings.Contains(got.Message, "status=502") || !strings.Contains(got.Message, "router upstream refused") {
		t.Errorf("llm health message dropped the diagnosis: %s", got.Message)
	}
}
