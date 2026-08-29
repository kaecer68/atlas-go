package llm_annotator

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKimiClient_CostReport_EmptyUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.cacheTTL = 0

	report := c.CostReport(0.001) // $0.001 per 1k tokens

	if report.Provider != "kimi" {
		t.Errorf("Provider = %q, want kimi", report.Provider)
	}
	if report.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 (no calls made)", report.TotalTokens)
	}
	if report.TotalCost != 0 {
		t.Errorf("TotalCost = %v, want 0", report.TotalCost)
	}
	if len(report.ByFeature) != 0 {
		t.Errorf("ByFeature = %v, want empty map", report.ByFeature)
	}
}

func TestKimiClient_CostReport_AccumulatesPerFeature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.cacheTTL = 0

	// 2 calls labeled "alert" + 1 call labeled "summary" = 3 calls * 19 tokens = 57 total
	for range 2 {
		if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x", Label: "alert"}); err != nil {
			t.Fatalf("alert Annotate: %v", err)
		}
	}
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "y", Label: "summary"}); err != nil {
		t.Fatalf("summary Annotate: %v", err)
	}

	// costPer1kTokens = 0.01 → 57 tokens = $0.00057
	report := c.CostReport(0.01)

	if got, want := report.TotalTokens, int64(57); got != want {
		t.Errorf("TotalTokens = %d, want %d", got, want)
	}
	wantCost := 57.0 * 0.01 / 1000.0
	if math.Abs(report.TotalCost-wantCost) > 1e-9 {
		t.Errorf("TotalCost = %v, want %v", report.TotalCost, wantCost)
	}
	if got, want := report.ByFeature["alert"].Tokens, int64(38); got != want {
		t.Errorf("alert tokens = %d, want %d (2 calls × 19)", got, want)
	}
	if report.ByFeature["alert"].Requests != 2 {
		t.Errorf("alert requests = %d, want 2", report.ByFeature["alert"].Requests)
	}
	if got, want := report.ByFeature["summary"].Tokens, int64(19); got != want {
		t.Errorf("summary tokens = %d, want %d", got, want)
	}
	if report.ByFeature["summary"].Requests != 1 {
		t.Errorf("summary requests = %d, want 1", report.ByFeature["summary"].Requests)
	}
}

func TestKimiClient_CostReport_LatencyPopulated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.cacheTTL = 0

	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	report := c.CostReport(0.001)
	if report.LatencyMillis < 0 {
		t.Errorf("LatencyMillis = %d, want >= 0", report.LatencyMillis)
	}
	if report.GeneratedAt.IsZero() {
		t.Error("GeneratedAt is zero, want populated")
	}
	if time.Since(report.GeneratedAt) > time.Second {
		t.Errorf("GeneratedAt = %v, want recent", report.GeneratedAt)
	}
}
