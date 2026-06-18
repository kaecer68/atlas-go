package llm_annotator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKimiClient_Snapshot_BeforeAnyCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	snap := c.Snapshot()
	if snap.Latency != 0 {
		t.Errorf("Latency = %v, want 0 before any call", snap.Latency)
	}
	if snap.Usage.Requests != 0 {
		t.Errorf("Usage.Requests = %d, want 0", snap.Usage.Requests)
	}
	if snap.Provider != "kimi" {
		t.Errorf("Provider = %q, want kimi", snap.Provider)
	}
}

func TestKimiClient_Snapshot_AfterSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	snap := c.Snapshot()
	if snap.Usage.Requests != 1 {
		t.Errorf("Usage.Requests = %d, want 1", snap.Usage.Requests)
	}
	if snap.Usage.TotalTokens != 19 {
		t.Errorf("Usage.TotalTokens = %d, want 19", snap.Usage.TotalTokens)
	}
	if snap.Latency <= 0 {
		t.Errorf("Latency = %v, want > 0", snap.Latency)
	}
}

func TestHandleHealth_ReturnsJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	h := HandleHealth(c)
	mux := http.NewServeMux()
	mux.HandleFunc("/health/llm-annotator", h)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Before any call: latency=0 but JSON shape must be present.
	resp, err := http.Get(ts.URL + "/health/llm-annotator")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}
	var snap Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Provider != "kimi" {
		t.Errorf("Provider = %q, want kimi", snap.Provider)
	}

	// After one call, Usage.Requests should be 1.
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	resp2, err := http.Get(ts.URL + "/health/llm-annotator")
	if err != nil {
		t.Fatalf("GET 2: %v", err)
	}
	defer resp2.Body.Close()
	var snap2 Snapshot
	if err := json.NewDecoder(resp2.Body).Decode(&snap2); err != nil {
		t.Fatalf("decode 2: %v", err)
	}
	if snap2.Usage.Requests != 1 {
		t.Errorf("Usage.Requests after call = %d, want 1", snap2.Usage.Requests)
	}
	if snap2.Latency <= 0 {
		t.Errorf("Latency after call = %v, want > 0", snap2.Latency)
	}
}
