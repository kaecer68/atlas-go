package marketdata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu        sync.Mutex
	remaining int
	limit     int
	requests  []time.Time
	window    time.Duration
}

func NewRateLimiter(limit int) *RateLimiter {
	return &RateLimiter{
		limit:     limit,
		remaining: limit,
		window:    time.Hour,
		requests:  make([]time.Time, 0),
	}
}

func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	now := time.Now()
	cutoff := now.Add(-r.window)
	valid := r.requests[:0]
	for _, t := range r.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	r.requests = valid

	if len(r.requests) >= r.limit {
		sleepDuration := r.window - now.Sub(r.requests[0])
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDuration):
			r.mu.Lock()
			r.remaining--
			r.requests = append(r.requests, time.Now())
			r.mu.Unlock()
			return nil
		}
	}

	r.remaining--
	r.requests = append(r.requests, now)
	r.mu.Unlock()
	return nil
}

func (r *RateLimiter) RecordUse() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, time.Now())
	if r.remaining > 0 {
		r.remaining--
	}
}

func (r *RateLimiter) Remaining() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.remaining
}

func (r *RateLimiter) WaitForReset(resetAt time.Time) time.Duration {
	return time.Until(resetAt) + time.Second
}

type FinMindAPIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e *FinMindAPIError) Error() string {
	return fmt.Sprintf("finmind: status %d: %s", e.StatusCode, e.Message)
}

func (e *FinMindAPIError) IsRateLimit() bool {
	return e.StatusCode == 429
}

func (e *FinMindAPIError) IsServerError() bool {
	return e.StatusCode >= 500
}

func FetchWithRetry(ctx context.Context, client *http.Client, url string, apiKey string, limiter *RateLimiter, maxRetries int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("finmind: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("finmind: rate limit: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			return nil, fmt.Errorf("finmind: http request: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close() // ignore close error

		if resp.StatusCode == 429 {
			retryAfter := resp.Header.Get("Retry-After")
			var waitTime time.Duration
			if retryAfter != "" {
				if secs, parseErr := time.ParseDuration(retryAfter + "s"); parseErr == nil {
					waitTime = secs
				}
			}
			if waitTime == 0 {
				waitTime = time.Minute
			}
			time.Sleep(waitTime)
			continue
		}

		if resp.StatusCode >= 500 && attempt < maxRetries {
			time.Sleep(time.Duration(attempt+1) * 5 * time.Second)
			continue
		}

		if resp.StatusCode >= 400 {
			return nil, &FinMindAPIError{
				StatusCode: resp.StatusCode,
				Message:    string(body),
			}
		}

		return body, readErr
	}
	return nil, &FinMindAPIError{StatusCode: 0, Message: "max retries exceeded"}
}
