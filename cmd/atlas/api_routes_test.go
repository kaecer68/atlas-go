package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubProbe returns a probe function that maps addresses to pre-canned
// reports. Addresses not in the map default to free.
func stubProbe(reports map[string]portHealthReport) func(addr string) portHealthReport {
	return func(addr string) portHealthReport {
		if r, ok := reports[addr]; ok {
			return r
		}
		return portHealthReport{Addr: addr, State: "free"}
	}
}

func TestNewHealthHandler_StatusAndShape(t *testing.T) {
	probe := stubProbe(nil)
	h := newHealthHandler(healthConfig{Probe: probe})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json prefix", ct)
	}

	var body healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if body.Status != "ok" {
		t.Errorf("status field = %q, want ok (legacy contract)", body.Status)
	}
	got, ok := body.Ports["atlas_http"]
	if !ok {
		t.Fatal("missing ports.atlas_http")
	}
	if got.Addr != "127.0.0.1:18080" {
		t.Errorf("atlas_http.addr = %q, want 127.0.0.1:18080", got.Addr)
	}
	if got.State != "free" {
		t.Errorf("atlas_http.state = %q, want free (stubbed default)", got.State)
	}
	got, ok = body.Ports["fubon_proxy"]
	if !ok {
		t.Fatal("missing ports.fubon_proxy")
	}
	if got.Addr != "127.0.0.1:18081" {
		t.Errorf("fubon_proxy.addr = %q, want 127.0.0.1:18081", got.Addr)
	}
}

func TestNewHealthHandler_ForeignOccupant_EmitsPIDAndCommand(t *testing.T) {
	// Simulate the dev-machine case the PR #815 body documents:
	// Docker Desktop holds IPv6 :18081 with com.docker.backend (PID 5866).
	probe := stubProbe(map[string]portHealthReport{
		"127.0.0.1:18081": {
			Addr:    "127.0.0.1:18081",
			State:   "foreign",
			PID:     5866,
			Command: "com.docker.backend",
		},
	})
	h := newHealthHandler(healthConfig{Probe: probe})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	occ := body.Ports["fubon_proxy"]
	if occ.State != "foreign" {
		t.Errorf("fubon_proxy.state = %q, want foreign", occ.State)
	}
	if occ.PID != 5866 {
		t.Errorf("fubon_proxy.pid = %d, want 5866", occ.PID)
	}
	if occ.Command != "com.docker.backend" {
		t.Errorf("fubon_proxy.command = %q, want com.docker.backend", occ.Command)
	}
}

func TestNewHealthHandler_SelfAddrSkipProbe(t *testing.T) {
	probeCalled := 0
	probe := func(addr string) portHealthReport {
		probeCalled++
		return portHealthReport{Addr: addr, State: "free"}
	}
	h := newHealthHandler(healthConfig{Probe: probe, SelfAddr: ":18080"})

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Ports["atlas_http"].State != "healthy" {
		t.Errorf("atlas_http state = %q, want healthy (self-addr skip)", body.Ports["atlas_http"].State)
	}
	if probeCalled != 1 {
		t.Errorf("probe called %d times, want 1 (fubon_proxy only)", probeCalled)
	}
}

func TestAddrPortsMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{":18080", "127.0.0.1:18080", true},
		{":18080", "0.0.0.0:18080", true},
		{":18080", "127.0.0.1:18081", false},
		{"127.0.0.1:18080", "[::]:18080", true},
		{"bad", "127.0.0.1:18080", false},
	}
	for _, tc := range cases {
		if got := addrPortsMatch(tc.a, tc.b); got != tc.want {
			t.Errorf("addrPortsMatch(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReportPort_ErrorsSurfacedAsUnknown(t *testing.T) {
	// Malformed address should not panic; reportPort should return
	// State="unknown" with the parse error in the Error field.
	rep := reportPort("not-a-valid-address")
	if rep.State != "unknown" {
		t.Errorf("state = %q, want unknown", rep.State)
	}
	if rep.Error == "" {
		t.Error("Error field should be populated for parse failure")
	}
}

func TestPortHealthReport_OmitsZeroPIDAndEmptyError(t *testing.T) {
	// omitempty ensures JSON shape stays lean when State is "free" or "healthy".
	b, err := json.Marshal(portHealthReport{Addr: "127.0.0.1:18080", State: "free"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, `"pid"`) {
		t.Errorf(`"pid" must be omitted when PID == 0; got %s`, s)
	}
	if strings.Contains(s, `"error"`) {
		t.Errorf(`"error" must be omitted when Error == ""; got %s`, s)
	}
}
