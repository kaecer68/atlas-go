package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_WaitAndRecord(t *testing.T) {
	limiter := NewRateLimiter(600) // 600 req/hr
	ctx := context.Background()

	// Should not block on first call
	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("first wait failed: %v", err)
	}

	remaining := limiter.Remaining()
	if remaining >= 600 {
		t.Fatalf("remaining should decrease after wait, got %d", remaining)
	}
}

func TestRateLimiter_DecrementsOnUse(t *testing.T) {
	limiter := NewRateLimiter(600)
	limiter.RecordUse()
	if limiter.Remaining() != 599 {
		t.Fatalf("expected 599 remaining, got %d", limiter.Remaining())
	}
}

func TestRateLimiter_429Handling(t *testing.T) {
	limiter := NewRateLimiter(600)
	// Simulate hitting 429 - should compute correct wait time
	resetAt := time.Now().Add(30 * time.Second)
	waitDuration := limiter.WaitForReset(resetAt)
	if waitDuration < 29*time.Second || waitDuration > 31*time.Second {
		t.Fatalf("expected ~30s wait, got %v", waitDuration)
	}
}

func TestFinMindAPIError(t *testing.T) {
	err := &FinMindAPIError{StatusCode: 429, Message: "rate limited"}
	if err.Error() != "finmind: status 429: rate limited" {
		t.Errorf("Error() = %q, want finmind: status 429: rate limited", err.Error())
	}
	if !err.IsRateLimit() {
		t.Error("expected IsRateLimit=true for 429")
	}
	if err.IsServerError() {
		t.Error("expected IsServerError=false for 429")
	}

	err500 := &FinMindAPIError{StatusCode: 500, Message: "server error"}
	if !err500.IsServerError() {
		t.Error("expected IsServerError=true for 500")
	}
}

func TestFetchWithRetry_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", auth)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	limiter := NewRateLimiter(600)
	client := srv.Client()
	body, err := FetchWithRetry(context.Background(), client, srv.URL, "test-key", limiter, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != `{"status":"ok"}` {
		t.Errorf("body = %q, want {\"status\":\"ok\"}", string(body))
	}
}

func TestFetchWithRetry_400Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`bad request`))
	}))
	defer srv.Close()

	limiter := NewRateLimiter(600)
	client := srv.Client()
	_, err := FetchWithRetry(context.Background(), client, srv.URL, "", limiter, 1)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if apiErr, ok := err.(*FinMindAPIError); !ok || apiErr.StatusCode != 400 {
		t.Errorf("expected FinMindAPIError with status 400, got %T %v", err, err)
	}
}

func TestFetchWithRetry_ContextCancelled(t *testing.T) {
	limiter := NewRateLimiter(600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := FetchWithRetry(ctx, http.DefaultClient, "http://example.com", "", limiter, 1)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}
