package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMiniMax_Chat_Success verifies Chat() correctly parses a valid
// OpenAI-compatible response from a mock MiniMax server.
func TestMiniMax_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := miniMaxResponseBody{
			Model: "MiniMax-M3",
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
						Content: "Hello from MiniMax!",
					},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			}{
				PromptTokens:     20,
				CompletionTokens: 8,
				TotalTokens:      28,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewMiniMaxClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL

	msgs := []Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "What is 2+2?"},
	}
	resp, err := client.Chat(context.Background(), "", msgs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from MiniMax!" {
		t.Errorf("expected content %q, got %q", "Hello from MiniMax!", resp.Content)
	}
	if resp.Model != "MiniMax-M3" {
		t.Errorf("expected model %q, got %q", "MiniMax-M3", resp.Model)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason %q, got %q", "stop", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 28 {
		t.Errorf("expected total_tokens 28, got %d", resp.Usage.TotalTokens)
	}
}

// TestMiniMax_Chat_AuthHeader verifies the Authorization header is sent.
func TestMiniMax_Chat_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer test-key-minimax"
		if auth != expected {
			t.Errorf("expected Authorization %q, got %q", expected, auth)
		}

		resp := miniMaxResponseBody{
			Model: "MiniMax-M3",
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

	client := NewMiniMaxClient("test-key-minimax", newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), DefaultModelMiniMaxM3, []Message{
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMiniMax_Chat_AnthropicEndpoint verifies the Anthropic-compatible
// endpoint is used when UseAnthropicFormat is true.
func TestMiniMax_Chat_AnthropicEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/anthropic/chat/completions") {
			t.Errorf("expected Anthropic endpoint, got path: %s", r.URL.Path)
		}

		resp := miniMaxResponseBody{
			Model: "MiniMax-M3",
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
						Content: "anthropic mode",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewMiniMaxClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL
	client.UseAnthropicFormat = true

	resp, err := client.Chat(context.Background(), "", []Message{
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "anthropic mode" {
		t.Errorf("expected content %q, got %q", "anthropic mode", resp.Content)
	}
}

// TestMiniMax_NilBaseClientUsesDefault verifies that NewMiniMaxClient accepts a
// nil BaseClient and falls back to a working default instead of panicking.
func TestMiniMax_NilBaseClientUsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(miniMaxResponseBody{
			Model: "MiniMax-M3",
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

	client := NewMiniMaxClient("test-key", nil)
	client.BaseURL = srv.URL

	resp, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content %q, got %q", "ok", resp.Content)
	}
}
