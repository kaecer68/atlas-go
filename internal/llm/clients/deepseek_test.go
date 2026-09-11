package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// TestDeepSeek_Chat_Success verifies that Chat() correctly parses a valid
// OpenAI-compatible response from a mock DeepSeek server.
func TestDeepSeek_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := deepSeekResponseBody{
			Model: DefaultModelV4_1Flash,
			Choices: []struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{
						Role:    "assistant",
						Content: "Hello from DeepSeek!",
					},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			}{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewDeepSeekClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL

	msgs := []Message{
		{Role: "user", Content: "Hello"},
	}
	resp, err := client.Chat(context.Background(), "", msgs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from DeepSeek!" {
		t.Errorf("expected content %q, got %q", "Hello from DeepSeek!", resp.Content)
	}
	if resp.Model != DefaultModelV4_1Flash {
		t.Errorf("expected model %q, got %q", DefaultModelV4_1Flash, resp.Model)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason %q, got %q", "stop", resp.FinishReason)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 || resp.Usage.TotalTokens != 15 {
		t.Errorf("unexpected usage: input=%d output=%d total=%d",
			resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.TotalTokens)
	}
}

// TestDeepSeek_Chat_AuthHeader verifies that the Authorization header is
// sent correctly with Bearer auth.
func TestDeepSeek_Chat_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer test-key-deepseek"
		if auth != expected {
			t.Errorf("expected Authorization %q, got %q", expected, auth)
		}

		resp := deepSeekResponseBody{
			Model: DefaultModelV4_1Flash,
			Choices: []struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{
						Role:    "assistant",
						Content: "ok",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewDeepSeekClient("test-key-deepseek", newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), DefaultModelV4Flash, []Message{
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDeepSeek_Chat_ModelDefault verifies the default model (the canonical
// "deepseek-flash") is used when model="" is passed. DeepSeek-V4-Pro is
// retired, so the client must never default to it.
func TestDeepSeek_Chat_ModelDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody deepSeekRequestBody
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if reqBody.Model != DefaultModelV4_1Flash {
			t.Errorf("expected model %q, got %q", DefaultModelV4_1Flash, reqBody.Model)
		}

		resp := deepSeekResponseBody{
			Model: DefaultModelV4_1Flash,
			Choices: []struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{
						Role:    "assistant",
						Content: "ok",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewDeepSeekClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), "", []Message{
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// newTestBaseClient returns a BaseClient suitable for use with httptest servers.
// It uses the server's client (via the caller setting BaseURL) with generous
// rate limits and single attempts to keep tests deterministic.
func newTestBaseClient() *BaseClient {
	bc := NewBaseClient(llm.ProviderMock, BaseClientConfig{
		RatePerSecond: 1000,
		Burst:         100,
		MaxAttempts:   1,
	})
	// Disable the circuit breaker for tests — we control errors via the mock server.
	bc.Breaker = nil
	return bc
}

// TestDeepSeek_NilBaseClientUsesDefault verifies that NewDeepSeekClient accepts
// a nil BaseClient and falls back to a working default instead of panicking.
// Regression test for the startup panic where cmd/atlas passed nil and the
// first Chat() call dereferenced a nil pointer in BaseClient.DoRequest.
func TestDeepSeek_NilBaseClientUsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deepSeekResponseBody{
			Model: DefaultModelV4_1Flash,
			Choices: []struct {
				Message struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			}{
				{
					Message: struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					}{Role: "assistant", Content: "ok"},
					FinishReason: "stop",
				},
			},
		})
	}))
	defer srv.Close()

	client := NewDeepSeekClient("test-key", nil)
	client.BaseURL = srv.URL

	resp, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content %q, got %q", "ok", resp.Content)
	}
}

// TestResolveDeepSeekModel covers LLM_DEEPSEEK_MODEL resolution (ADR-012):
// unset/empty yields the canonical "deepseek-flash"; any other value wins.
func TestResolveDeepSeekModel(t *testing.T) {
	if got := ResolveDeepSeekModel(""); got != DefaultModelV4_1Flash {
		t.Errorf("ResolveDeepSeekModel(\"\") = %q, want %q", got, DefaultModelV4_1Flash)
	}
	if got := ResolveDeepSeekModel("deepseek-chat"); got != "deepseek-chat" {
		t.Errorf("ResolveDeepSeekModel override = %q, want %q", got, "deepseek-chat")
	}
	if DeepSeekModelEnvVar != "LLM_DEEPSEEK_MODEL" {
		t.Errorf("DeepSeekModelEnvVar = %q, want LLM_DEEPSEEK_MODEL", DeepSeekModelEnvVar)
	}
}

// TestNewDeepSeekClient_DefaultModelIsCanonical verifies the retired V4-Pro
// name is not the constructor default any more.
func TestNewDeepSeekClient_DefaultModelIsCanonical(t *testing.T) {
	c := NewDeepSeekClient("test-key", newTestBaseClient())
	if c.DefaultModel != DefaultModelV4_1Flash {
		t.Errorf("DefaultModel = %q, want %q", c.DefaultModel, DefaultModelV4_1Flash)
	}
	if c.DefaultModel == DefaultModelV4Pro {
		t.Error("DefaultModel must not be the retired V4-Pro constant")
	}
}
