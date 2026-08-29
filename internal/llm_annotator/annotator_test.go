package llm_annotator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMockAnnotator_HappyPath(t *testing.T) {
	m := NewMock("融資餘額未達 3500 億，散戶槓桿未過熱")
	got, err := m.Annotate(context.Background(), FailureContext{FrameID: "margin-balance-extreme"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "3500 億") {
		t.Errorf("response missing expected token: %q", got)
	}
	if m.Calls != 1 {
		t.Errorf("Calls = %d, want 1", m.Calls)
	}
}

func TestMockAnnotator_ReturnsError(t *testing.T) {
	m := &MockAnnotator{Err: ErrUnavailable}
	_, err := m.Annotate(context.Background(), FailureContext{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("got %v, want ErrUnavailable", err)
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	c := Config{APIKey: "k"}.WithDefaults()
	if c.BaseURL != "https://api.kimi.com/coding/v1" {
		t.Errorf("BaseURL = %q, want default", c.BaseURL)
	}
	if c.Model != "moonshot-v1-8k" {
		t.Errorf("Model = %q, want default", c.Model)
	}
	if c.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", c.Timeout)
	}
	if c.MaxTokens != 512 {
		t.Errorf("MaxTokens = %d, want 512", c.MaxTokens)
	}
}

func TestConfig_Validate(t *testing.T) {
	if err := (Config{}).Validate(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("empty cfg: got %v, want ErrUnavailable", err)
	}
	if err := (Config{APIKey: "k"}).Validate(); err != nil {
		t.Errorf("non-empty cfg: unexpected error: %v", err)
	}
}

func TestNewKimiClient_MissingKey(t *testing.T) {
	if _, err := NewKimiClient(Config{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("empty cfg: got %v, want ErrUnavailable", err)
	}
}

func TestKimiClient_Annotate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"choices": [
				{"message": {"role": "assistant", "content": "融資餘額 3200 億，未達 3500 億門檻，散戶槓桿未過熱"}}
			]
		}`))
	}))
	defer srv.Close()

	c, err := NewKimiClient(Config{APIKey: "test-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	fc := FailureContext{
		FrameID:    "margin-balance-extreme",
		FrameName:  "融資餘額極端反轉",
		Layer:      "L4",
		OccurredAt: time.Now(),
		Snap: MacroSnapshot{
			RetailMarginBalance: 3200,
		},
		Conditions: []ConditionSnapshot{
			{Field: "RetailMarginBalance", Operator: "gt", Threshold: 3500, ActualValue: 3200, Timeframe: "3D"},
		},
	}
	got, err := c.Annotate(context.Background(), fc)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if !strings.Contains(got, "3200") || !strings.Contains(got, "3500") {
		t.Errorf("response missing expected tokens: %q", got)
	}
}

func TestKimiClient_Annotate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	_, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

func TestKimiClient_Annotate_NetworkError(t *testing.T) {
	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: "http://127.0.0.1:1"})
	_, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

func TestKimiClient_Annotate_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	_, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

func TestKimiClient_Annotate_RetriesOn5xx(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"recovered"}}]}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	got, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if got != "recovered" {
		t.Errorf("got %q, want %q", got, "recovered")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestKimiClient_Annotate_RetriesOn429(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	got, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestKimiClient_Annotate_NoRetryOn4xx(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	_, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 4xx), got %d", attempts)
	}
}

func TestKimiClient_Annotate_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	_, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

func TestFailureContextToPrompt(t *testing.T) {
	fc := FailureContext{
		FrameID:    "x",
		FrameName:  "test",
		Layer:      "L4",
		OccurredAt: time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
		Conditions: []ConditionSnapshot{
			{Field: "f", Operator: "gt", Threshold: 1, ActualValue: 2, Timeframe: "1D"},
		},
	}
	prompt := failureContextToPrompt(fc)
	for _, want := range []string{
		"frame_id=x",
		"frame_name=test",
		"layer=L4",
		"2026-06-11T00:00:00Z",
		"macro.foreign_capital_net_twd=",
		"conditions:",
		"[0] field=f op=gt threshold=1.0000 actual=2.0000",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\nfull prompt:\n%s", want, prompt)
		}
	}
}

func TestKimiClient_Annotate_TracksTokenUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	c.cacheTTL = 0
	for i := range 3 {
		if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	u := c.Usage()
	if u.PromptTokens != 36 {
		t.Errorf("prompt_tokens: got %d, want 36", u.PromptTokens)
	}
	if u.CompletionTokens != 21 {
		t.Errorf("completion_tokens: got %d, want 21", u.CompletionTokens)
	}
	if u.TotalTokens != 57 {
		t.Errorf("total_tokens: got %d, want 57", u.TotalTokens)
	}
	if u.Requests != 3 {
		t.Errorf("requests: got %d, want 3", u.Requests)
	}
}

func TestKimiClient_Annotate_NoUsageOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	u := c.Usage()
	if u.Requests != 0 {
		t.Errorf("requests should be 0 on failure, got %d", u.Requests)
	}
}

// newLabeledKimiClient returns a KimiClient that records 100 tokens (50 prompt
// + 50 completion) per call, with no budget alert, for per-feature tests.
func newLabeledKimiClient(t *testing.T) *KimiClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":50,"completion_tokens":50,"total_tokens":100}}`))
	}))
	t.Cleanup(srv.Close)
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL, MaxTokens: 64, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.backoff = func(int) time.Duration { return 0 }
	c.cacheTTL = 0
	return c
}

func TestPerFeatureUsage_TracksPerLabel(t *testing.T) {
	c := newLabeledKimiClient(t)
	ctx := context.Background()
	for i := range 3 {
		if _, err := c.Annotate(ctx, FailureContext{FrameID: "x", Label: "alert"}); err != nil {
			t.Fatalf("alert call %d: %v", i, err)
		}
	}
	for i := range 2 {
		if _, err := c.Annotate(ctx, FailureContext{FrameID: "x", Label: "summary"}); err != nil {
			t.Fatalf("summary call %d: %v", i, err)
		}
	}
	alertU := c.UsageByLabel("alert")
	summaryU := c.UsageByLabel("summary")
	if alertU.Requests != 3 || alertU.TotalTokens != 300 {
		t.Errorf("alert usage = %+v, want Requests=3 TotalTokens=300", alertU)
	}
	if summaryU.Requests != 2 || summaryU.TotalTokens != 200 {
		t.Errorf("summary usage = %+v, want Requests=2 TotalTokens=200", summaryU)
	}
}

func TestPerFeatureUsage_TotalIncludesAllLabels(t *testing.T) {
	c := newLabeledKimiClient(t)
	ctx := context.Background()
	labels := []string{"alpha", "beta", "gamma", "alpha", "beta"}
	for _, l := range labels {
		if _, err := c.Annotate(ctx, FailureContext{FrameID: "x", Label: l}); err != nil {
			t.Fatalf("label %q: %v", l, err)
		}
	}
	total := c.Usage()
	if total.Requests != 5 || total.TotalTokens != 500 {
		t.Errorf("total = %+v, want Requests=5 TotalTokens=500", total)
	}
	var sumRequests, sumTokens int64
	for _, u := range c.UsageAll() {
		sumRequests += u.Requests
		sumTokens += u.TotalTokens
	}
	if sumRequests != total.Requests || sumTokens != total.TotalTokens {
		t.Errorf("per-label sum (req=%d, tok=%d) != total (req=%d, tok=%d)",
			sumRequests, sumTokens, total.Requests, total.TotalTokens)
	}
}

func TestPerFeatureUsage_EmptyLabelNotTracked(t *testing.T) {
	c := newLabeledKimiClient(t)
	ctx := context.Background()
	if _, err := c.Annotate(ctx, FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("empty-label call: %v", err)
	}
	if _, err := c.Annotate(ctx, FailureContext{FrameID: "x", Label: "named"}); err != nil {
		t.Fatalf("named call: %v", err)
	}
	all := c.UsageAll()
	if len(all) != 1 {
		t.Errorf("UsageAll() should have 1 entry (named only), got %d: %+v", len(all), all)
	}
	if _, ok := all[""]; ok {
		t.Errorf("empty label must not appear in UsageAll(), got %+v", all)
	}
	if _, ok := all["named"]; !ok {
		t.Errorf("named label missing from UsageAll(), got %+v", all)
	}
	if c.Usage().Requests != 2 {
		t.Errorf("total requests = %d, want 2 (both calls recorded)", c.Usage().Requests)
	}
}

func TestPerFeatureUsage_UsageAllReturnsCopy(t *testing.T) {
	c := newLabeledKimiClient(t)
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x", Label: "feat"}); err != nil {
		t.Fatalf("call: %v", err)
	}
	all := c.UsageAll()
	all["feat"] = Usage{Requests: 9999, TotalTokens: 9999}
	all["rogue"] = Usage{Requests: 1}
	fresh := c.UsageByLabel("feat")
	if fresh.Requests != 1 || fresh.TotalTokens != 100 {
		t.Errorf("UsageByLabel saw mutated value: %+v", fresh)
	}
	if _, ok := c.UsageAll()["rogue"]; ok {
		t.Errorf("rogue key leaked into client state")
	}
}

// newBudgetKimiClient returns a KimiClient wired to a stub server that always
// returns a chat-completion with the supplied per-call token counts. The
// shared server is closed via t.Cleanup.
func newBudgetKimiClient(t *testing.T, promptTokens, completionTokens int64, threshold int64, cb func(Usage)) *KimiClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
			promptTokens, completionTokens, promptTokens+completionTokens)
	}))
	t.Cleanup(srv.Close)
	c, err := NewKimiClient(Config{
		APIKey:          "k",
		BaseURL:         srv.URL,
		MaxTokens:       64,
		Timeout:         5 * time.Second,
		BudgetThreshold: threshold,
		BudgetCallback:  cb,
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.backoff = func(int) time.Duration { return 0 }
	c.cacheTTL = 0
	return c
}

func TestBudgetAlert_FiresWhenThresholdCrossed(t *testing.T) {
	var fired int
	var seen Usage
	var mu sync.Mutex
	callback := func(u Usage) {
		mu.Lock()
		defer mu.Unlock()
		fired++
		seen = u
	}
	// Each call returns 100 tokens total. After 2nd call we are at 200 >= 150.
	c := newBudgetKimiClient(t, 50, 50, 150, callback)
	for i := range 3 {
		if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if fired != 1 {
		t.Errorf("callback fired %d times, want 1 (threshold fires once)", fired)
	}
	if seen.TotalTokens < 150 {
		t.Errorf("callback saw TotalTokens=%d, want >=150", seen.TotalTokens)
	}
}

func TestBudgetAlert_DoesNotFireUnderThreshold(t *testing.T) {
	var fired int
	var mu sync.Mutex
	callback := func(u Usage) { mu.Lock(); fired++; mu.Unlock() }
	c := newBudgetKimiClient(t, 10, 10, 1000, callback)
	for i := range 3 {
		if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if fired != 0 {
		t.Errorf("callback fired %d times, want 0 (under threshold)", fired)
	}
}

func TestBudgetAlert_FiresExactlyOnceAcrossManyCalls(t *testing.T) {
	var fired int
	var mu sync.Mutex
	callback := func(u Usage) { mu.Lock(); fired++; mu.Unlock() }
	c := newBudgetKimiClient(t, 25, 25, 50, callback)
	for i := range 10 {
		if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if fired != 1 {
		t.Errorf("callback fired %d times, want exactly 1 across 10 calls", fired)
	}
}

func TestBudgetAlert_DisabledByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer srv.Close()
	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL, MaxTokens: 64, Timeout: 5 * time.Second})
	c.backoff = func(int) time.Duration { return 0 }
	c.cacheTTL = 0
	for i := range 3 {
		if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	u := c.Usage()
	if u.TotalTokens != 9 || u.Requests != 3 {
		t.Errorf("usage = %+v, want TotalTokens=9 Requests=3", u)
	}
}

func TestBudgetAlert_CallbackCanCallUsageWithoutDeadlock(t *testing.T) {
	// Regression guard: invoking Usage() inside the callback must not deadlock,
	// because the callback is dispatched outside the usage lock.
	var fired int
	var mu sync.Mutex
	callback := func(u Usage) {
		mu.Lock()
		defer mu.Unlock()
		fired++
		_ = u
	}
	c := newBudgetKimiClient(t, 50, 50, 100, callback)
	done := make(chan struct{})
	go func() {
		for range 2 {
			_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Annotate deadlocked: callback re-entry into client methods blocked")
	}
	mu.Lock()
	defer mu.Unlock()
	if fired == 0 {
		t.Errorf("callback never fired; threshold=100 should trigger on first call (100 tokens)")
	}
}

func TestKimiClient_Annotate_CacheHit(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"choices":[{"message":{"content":"cached"}}]}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	fc := FailureContext{FrameID: "f1", FrameName: "n1", Layer: "L1"}
	for i := range 3 {
		got, err := c.Annotate(context.Background(), fc)
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if got != "cached" {
			t.Errorf("attempt %d: got %q, want %q", i, got, "cached")
		}
	}
	if requests != 1 {
		t.Errorf("expected 1 HTTP request (2 cache hits), got %d", requests)
	}
	if u := c.Usage(); u.Requests != 1 {
		t.Errorf("Usage.Requests should count only HTTP calls, got %d", u.Requests)
	}
}

func TestKimiClient_Annotate_CacheTTL(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"choices":[{"message":{"content":"x"}}]}`))
	}))
	defer srv.Close()

	c, _ := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	c.backoff = func(int) time.Duration { return 0 }
	c.cacheTTL = 0
	fc := FailureContext{FrameID: "f1", FrameName: "n1", Layer: "L1"}
	for i := range 2 {
		if _, err := c.Annotate(context.Background(), fc); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if requests != 2 {
		t.Errorf("expected 2 HTTP requests (TTL=0 → no cache), got %d", requests)
	}
}
