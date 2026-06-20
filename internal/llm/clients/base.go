package clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/llm"
)

// BaseClient provides shared HTTP infrastructure for LLM provider clients.
// It encapsulates rate limiting, retry with exponential backoff, circuit
// breaking, and metrics emission. Concrete provider implementations
// (DeepSeek, MiniMax, Kimi standalone) compose a BaseClient rather than
// reimplementing these cross-cutting concerns.
type BaseClient struct {
	HTTPClient   *http.Client
	Limiter      *rate.Limiter
	MaxAttempts  int
	Backoff      func(attempt int) time.Duration
	Metrics      MetricsRecorder
	Breaker      *CircuitBreaker
	ProviderName llm.Provider
}

// BaseClientConfig holds the tunable parameters for a BaseClient.
// Zero-valued fields receive sensible defaults at construction time.
type BaseClientConfig struct {
	Timeout       time.Duration
	RatePerSecond float64
	Burst         int
	MaxAttempts   int
}

// NewBaseClient creates a BaseClient wired with the given config. Fields
// that are zero receive defaults: Timeout→30s, RatePerSecond→1.0,
// Burst→4, MaxAttempts→3. The caller should set BaseClient.Metrics
// after construction if metrics collection is needed.
func NewBaseClient(name llm.Provider, cfg BaseClientConfig) *BaseClient {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.RatePerSecond <= 0 {
		cfg.RatePerSecond = 1.0
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 4
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}

	return &BaseClient{
		HTTPClient:   &http.Client{Timeout: cfg.Timeout},
		Limiter:      rate.NewLimiter(rate.Limit(cfg.RatePerSecond), cfg.Burst),
		MaxAttempts:  cfg.MaxAttempts,
		Backoff:      defaultBackoff,
		Metrics:      NoOpMetrics{},
		Breaker:      NewCircuitBreaker(),
		ProviderName: name,
	}
}

// defaultBackoff implements exponential backoff: 100ms, 200ms, 400ms, ...
func defaultBackoff(attempt int) time.Duration {
	return time.Duration(100*(1<<attempt)) * time.Millisecond
}

// DoRequest executes an HTTP request with rate limiting, circuit breaking,
// and automatic retry on transient errors (5xx, 429). Non-retryable 4xx
// errors fail immediately. Returns the raw response, the consumed body
// bytes, and any error encountered.
func (b *BaseClient) DoRequest(ctx context.Context, method, url string, headers map[string]string, body []byte) (*http.Response, []byte, error) {
	start := time.Now()

	if err := b.Limiter.Wait(ctx); err != nil {
		b.recordOutcome("rate_limited", 0)
		return nil, nil, fmt.Errorf("rate limit wait: %w", err)
	}

	var lastErr error
	var lastStatus int
	var resultResp *http.Response
	var resultBody []byte

	op := func() error {
		for attempt := 0; attempt < b.MaxAttempts; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					b.recordOutcome("canceled", 0)
					return fmt.Errorf("context done during backoff: %w", ctx.Err())
				case <-time.After(b.Backoff(attempt - 1)):
				}
			}

			resp, respBody, status, retryable, err := b.doHTTP(ctx, method, url, headers, body)
			if err == nil {
				resultResp = resp
				resultBody = respBody
				b.recordOutcome("success", status)
				return nil
			}

			lastErr = err
			lastStatus = status

			if !retryable {
				b.recordOutcome("client_error", status)
				return err
			}
			b.recordOutcome("retry", 0)
		}
		b.recordOutcome("retry_exhausted", lastStatus)
		return lastErr
	}

	if b.Breaker != nil && b.Breaker.State() == "open" {
		b.recordOutcome("circuit_open", 0)
		return nil, nil, fmt.Errorf("%w: circuit breaker is open", ErrCircuitOpen)
	}

	if b.Breaker != nil {
		if cbErr := b.Breaker.Call(op); cbErr != nil {
			return nil, nil, cbErr
		}
	} else {
		if err := op(); err != nil {
			return nil, nil, err
		}
	}

	if b.Metrics != nil {
		b.Metrics.RecordGauge("llm_client_latency_seconds", time.Since(start).Seconds(),
			map[string]string{"provider": string(b.ProviderName)})
	}

	return resultResp, resultBody, nil
}

func (b *BaseClient) doHTTP(ctx context.Context, method, url string, headers map[string]string, body []byte) (*http.Response, []byte, int, bool, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("build request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := b.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, 0, true, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, resp.StatusCode, true, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode/100 == 5
		return nil, nil, resp.StatusCode, retryable,
			fmt.Errorf("http status %d: %s", resp.StatusCode, truncateForError(respBody))
	}

	return resp, respBody, resp.StatusCode, false, nil
}

func (b *BaseClient) recordOutcome(outcome string, status int) {
	if b.Metrics == nil {
		return
	}
	labels := map[string]string{
		"provider": string(b.ProviderName),
		"outcome":  outcome,
	}
	if status > 0 {
		labels["status"] = strconv.Itoa(status)
	}
	b.Metrics.RecordCounter("llm_client_requests_total", 1, labels)
}

func truncateForError(b []byte) string {
	if len(b) <= 200 {
		return string(b)
	}
	return string(b[:200]) + "..."
}

// Compile-time guard: ensure NoOpMetrics satisfies MetricsRecorder.
var _ MetricsRecorder = NoOpMetrics{}
