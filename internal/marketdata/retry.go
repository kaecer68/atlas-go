package marketdata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// retryConfig captures the retry policy for fetchWithRetry.
type retryConfig struct {
	// maxAttempts is the TOTAL number of HTTP attempts including the first
	// (e.g. 3 = 1 initial + 2 retries). Values <= 0 degrade to a single
	// attempt.
	maxAttempts int
	// baseBackoff is the exponential backoff base: each retry waits
	// baseBackoff * 2^attempt (unless the response carries Retry-After).
	baseBackoff time.Duration
}

// defaultRetryConfig returns the production retry policy from the parameter
// system (marketdata.max_retry_attempts / retry_backoff_ms). P0-5: these two
// parameters existed since the parameter-system launch but were never
// consumed by production code — this helper is the first consumer.
func defaultRetryConfig() retryConfig {
	params := config.GetParametersConfig()
	return retryConfig{
		maxAttempts: params.Marketdata.MaxRetryAttempts.Value,
		baseBackoff: time.Duration(params.Marketdata.RetryBackoffMs.Value) * time.Millisecond,
	}
}

// fetchWithRetry sends req, retrying only transient upstream failures:
// HTTP 429 (rate limited) and 5xx (server error). It honors a Retry-After
// header when present (seconds or HTTP-date), otherwise waits
// baseBackoff * 2^attempt (exponential backoff). It stops retrying when
// maxAttempts is exhausted, the context is canceled, or a non-transient
// status arrives.
//
// Transport-level errors (DNS, timeout, connection refused) and 4xx (except
// 429) are NOT retried — they return immediately so the caller's breaker /
// error classification sees the real failure. On success (or a non-retryable
// status) the caller owns reading and closing the returned response body.
// The request must be reusable across attempts (GET with nil body — same
// contract as http.Client.Do with redirects).
func fetchWithRetry(ctx context.Context, client *http.Client, req *http.Request, cfg retryConfig) (*http.Response, error) {
	attempts := cfg.maxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			// Transport failure: do not retry (caller classifies it).
			return nil, err
		}
		code := resp.StatusCode
		if code != http.StatusTooManyRequests && code < 500 {
			// Non-transient (2xx/3xx/4xx-except-429): return as-is.
			return resp, nil
		}
		// 429/5xx: capture a bounded body snippet for diagnostics (so the
		// final error still tells operators WHAT the upstream said), then
		// drain + close so the connection is reusable.
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyStr := strings.TrimSpace(string(bodyBytes))
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if attempt == attempts-1 {
			if bodyStr == "" {
				bodyStr = "(empty body)"
			}
			return nil, fmt.Errorf("http status %d after %d attempts: %s", code, attempts, bodyStr)
		}
		wait := cfg.baseBackoff * time.Duration(1<<attempt)
		if wait <= 0 {
			wait = time.Second
		}
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
				wait = time.Duration(secs) * time.Second
			} else if t, parseErr := http.ParseTime(ra); parseErr == nil {
				if d := time.Until(t); d > 0 {
					wait = d
				}
			}
		}
		logging.Warn("marketdata", "fetch_retry",
			"status", code,
			"url", req.URL.String(),
			"attempt", attempt+1,
			"retry_in_s", int(wait.Seconds()))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("http request failed after %d attempts", attempts)
}
