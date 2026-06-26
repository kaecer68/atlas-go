package clients

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// TestKimi_Chat_Success verifies Chat() correctly parses a valid
// response from a mock Kimi K2.7 server.
func TestKimi_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/coding/v1/chat/completions") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := kimiResponseBody{
			Model: "kimi-for-coding",
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
						Content: "Hello from Kimi K2.7!",
					},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			}{
				PromptTokens:     15,
				CompletionTokens: 7,
				TotalTokens:      22,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewKimiClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL

	msgs := []Message{
		{Role: "user", Content: "Hello"},
	}
	resp, err := client.Chat(context.Background(), msgs, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Kimi K2.7!" {
		t.Errorf("expected content %q, got %q", "Hello from Kimi K2.7!", resp.Content)
	}
	if resp.Model != "kimi-for-coding" {
		t.Errorf("expected model %q, got %q", "kimi-for-coding", resp.Model)
	}
	if resp.Usage.TotalTokens != 22 {
		t.Errorf("expected total_tokens 22, got %d", resp.Usage.TotalTokens)
	}
}

// TestKimi_Chat_AuthHeader verifies the Authorization header is sent.
func TestKimi_Chat_AuthHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		expected := "Bearer test-key-kimi"
		if auth != expected {
			t.Errorf("expected Authorization %q, got %q", expected, auth)
		}

		resp := kimiResponseBody{
			Model: "kimi-for-coding",
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

	client := NewKimiClient("test-key-kimi", newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestKimi_RejectsRegulatedDataClass verifies that DataClassRegulated
// causes Chat() to return ErrIncompatibleDataClass.
func TestKimi_RejectsRegulatedDataClass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called for rejected data class")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewKimiClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "secret stuff"},
	}, &ChatOptions{DataClass: llm.DataClassRegulated})
	if err == nil {
		t.Fatal("expected error for DataClassRegulated")
	}
	if !errors.Is(err, ErrIncompatibleDataClass) {
		t.Errorf("expected ErrIncompatibleDataClass, got %v", err)
	}
}

// TestKimi_RejectsSecretDataClass verifies that DataClassSecret also
// causes Chat() to return ErrIncompatibleDataClass.
func TestKimi_RejectsSecretDataClass(t *testing.T) {
	client := NewKimiClient("test-key", newTestBaseClient())
	client.BaseURL = "http://localhost:1" // won't be reached

	_, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "top secret"},
	}, &ChatOptions{DataClass: llm.DataClassSecret})
	if err == nil {
		t.Fatal("expected error for DataClassSecret")
	}
	if !errors.Is(err, ErrIncompatibleDataClass) {
		t.Errorf("expected ErrIncompatibleDataClass, got %v", err)
	}
}

// TestKimi_ForcesThinkingAndTemp verifies that the request body includes
// thinking: {type: "enabled"} and temperature: 1.0 even when ChatOptions
// specifies a different temperature.
func TestKimi_ForcesThinkingAndTemp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// Verify temperature is locked to 1.0.
		temp, ok := body["temperature"].(float64)
		if !ok {
			t.Error("temperature field missing or not a number")
		}
		if temp != 1.0 {
			t.Errorf("expected temperature 1.0, got %v", temp)
		}

		// Verify thinking is forced on.
		thinking, ok := body["thinking"].(map[string]any)
		if !ok {
			t.Error("thinking field missing or not an object")
		}
		thinkingType, ok := thinking["type"].(string)
		if !ok || thinkingType != "enabled" {
			t.Errorf("expected thinking.type=enabled, got %v", thinkingType)
		}

		// Verify model is always kimi-for-coding.
		model, _ := body["model"].(string)
		if model != "kimi-for-coding" {
			t.Errorf("expected model kimi-for-coding, got %q", model)
		}

		resp := kimiResponseBody{
			Model: "kimi-for-coding",
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
						Content: "thinking",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewKimiClient("test-key", newTestBaseClient())
	client.BaseURL = srv.URL

	// Pass a different temperature — it should be overridden.
	customTemp := 0.3
	_, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "test"},
	}, &ChatOptions{Temperature: &customTemp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestKimi_NilBaseClientUsesDefault verifies that NewKimiClient accepts a nil
// BaseClient and falls back to a working default instead of panicking.
func TestKimi_NilBaseClientUsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(kimiResponseBody{
			Model: "kimi-for-coding",
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

	client := NewKimiClient("test-key", nil)
	client.BaseURL = srv.URL

	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content %q, got %q", "ok", resp.Content)
	}
}
