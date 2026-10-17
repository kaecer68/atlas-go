package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCollectorKey is a fake credential, shaped like the real thing so a
// failure message reads like the incident it guards.
const fakeCollectorKey = "FAKE-c07collector-key-5d6e7f8a9b0c1d2e"

// TestFetchPredictionReportHTTPErrorRedactsUpstreamBody covers the non-200
// branch of fetchPredictionReport (FU-20261016-16). The row this collector
// writes ends up in an operator-facing observation log, and its errors are
// printed by main, so the body's credential must not survive while the status
// and the upstream message skeleton must.
func TestFetchPredictionReportHTTPErrorRedactsUpstreamBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"msg":"Token is illegal.","status":401,"token_tail":"` + fakeCollectorKey + `"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := fetchPredictionReport(srv.URL)
	if err == nil {
		t.Fatal("want an error for HTTP 401, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, fakeCollectorKey) {
		t.Errorf("error leaks the upstream credential: %s", msg)
	}
	if !strings.Contains(msg, "status 401") || !strings.Contains(msg, "Token is illegal.") {
		t.Errorf("error dropped the diagnosis: %s", msg)
	}
}
