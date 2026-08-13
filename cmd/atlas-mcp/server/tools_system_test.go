package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleSystemGetMetrics_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleSystemGetMetrics(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/metrics" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleSystemGetMetricsTrend_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleSystemGetMetricsTrend(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/metrics/trend" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleSystemGetThresholds_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleSystemGetThresholds(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/metrics/thresholds" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleSystemGetDataPipeline_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleSystemGetDataPipeline(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/data-pipeline" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleSystemGetCircuitBreaker_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleSystemGetCircuitBreaker(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/circuit-breaker" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleSystemGetMaturity_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleSystemGetMaturity(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/maturity" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleSystemGetMaturity_DegradesToEmbedded(t *testing.T) {
	// Backend that returns 500 → handler must fall back to embedded MATURITY.md.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: filepath.Join(t.TempDir(), "audit.log")}
	audit, err := NewAuditWriter(cfg.AuditLogPath)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	defer func() { _ = audit.Close() }()
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}

	_, out, err := s.handleSystemGetMaturity(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler should degrade, not error: %v", err)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil (degraded embedded snapshot)")
	}
	if (*out.Result)["degraded"] != true {
		t.Fatalf("expected degraded=true, got %v", (*out.Result)["degraded"])
	}
	content, _ := (*out.Result)["content"].(string)
	if len(content) == 0 {
		t.Fatal("expected embedded MATURITY.md content")
	}
	if !strings.Contains(content, "Maturity") && !strings.Contains(content, "maturity") {
		t.Fatalf("embedded content does not look like MATURITY.md: %.100s", content)
	}
}
