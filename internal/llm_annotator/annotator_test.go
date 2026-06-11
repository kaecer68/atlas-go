package llm_annotator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if c.BaseURL != "https://api.moonshot.cn/v1" {
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
