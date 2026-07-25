package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

func TestHandleCost_NilClientReturns503(t *testing.T) {
	handler := HandleCost(func() *llm_annotator.KimiClient { return nil }, 0.001)

	req := httptest.NewRequest(http.MethodGet, "/llm_annotator/cost", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["error"] == "" {
		t.Errorf("expected non-empty error field, got body: %s", rr.Body.String())
	}
}

func TestHandleCost_EmptyClientReturns200WithZeros(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	client, err := llm_annotator.NewKimiClient(llm_annotator.Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}

	handler := HandleCost(func() *llm_annotator.KimiClient { return client }, 0.5)

	req := httptest.NewRequest(http.MethodGet, "/llm_annotator/cost", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var report llm_annotator.CostReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Provider != "kimi" {
		t.Errorf("Provider = %q, want kimi", report.Provider)
	}
	if report.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 (no Annotate called)", report.TotalTokens)
	}
	if report.TotalCost != 0 {
		t.Errorf("TotalCost = %v, want 0", report.TotalCost)
	}
	if report.GeneratedAt.IsZero() {
		t.Errorf("GeneratedAt is zero, want populated")
	}
}

func TestHandleCost_PopulatedClientReturnsPerFeatureBreakdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`))
	}))
	defer srv.Close()

	client, err := llm_annotator.NewKimiClient(llm_annotator.Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Use unique FrameIDs so the response cache does not collapse the two
	// "alert" calls into a single recorded request.
	calls := []struct{ label, frameID string }{
		{"alert", "f1"}, {"alert", "f2"}, {"summary", "f3"},
	}
	for _, c := range calls {
		if _, err := client.Annotate(ctx, llm_annotator.FailureContext{FrameID: c.frameID, Label: c.label}); err != nil {
			t.Fatalf("Annotate %s/%s: %v", c.label, c.frameID, err)
		}
	}

	handler := HandleCost(func() *llm_annotator.KimiClient { return client }, 0.01) // $0.01 per 1k tokens

	req := httptest.NewRequest(http.MethodGet, "/llm_annotator/cost", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var report llm_annotator.CostReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 3 calls × 19 tokens = 57 tokens
	if report.TotalTokens != 57 {
		t.Errorf("TotalTokens = %d, want 57", report.TotalTokens)
	}
	if report.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", report.TotalRequests)
	}
	if got, want := report.ByFeature["alert"].Tokens, int64(38); got != want {
		t.Errorf("alert.Tokens = %d, want %d (2 calls × 19)", got, want)
	}
	if report.ByFeature["alert"].Requests != 2 {
		t.Errorf("alert.Requests = %d, want 2", report.ByFeature["alert"].Requests)
	}
	if got, want := report.ByFeature["summary"].Tokens, int64(19); got != want {
		t.Errorf("summary.Tokens = %d, want %d", got, want)
	}
}
