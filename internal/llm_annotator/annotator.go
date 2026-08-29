package llm_annotator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// MockAnnotator is a test-friendly Annotator that returns a configurable
// response or error. It is safe for concurrent use; the zero value is
// usable and always returns ("", nil).
type MockAnnotator struct {
	mu       sync.Mutex
	Response string
	Err      error
	Calls    int
}

// NewMock returns a MockAnnotator that always returns the given string with
// no error. Useful for the common "happy path" test case.
func NewMock(response string) *MockAnnotator {
	return &MockAnnotator{Response: response}
}

// Name implements Annotator.
func (m *MockAnnotator) Name() string { return "mock" }

// Annotate implements Annotator. It records the call count for assertions
// and returns the configured response or error.
func (m *MockAnnotator) Annotate(_ context.Context, _ FailureContext) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls++
	return m.Response, m.Err
}

// KimiClient is an Annotator that calls the Moonshot chat-completions API.
// It is OpenAI-API compatible, so the same struct would work for any
// OpenAI-compatible endpoint with a different BaseURL.
type KimiClient struct {
	cfg               Config
	hc                *http.Client
	limiter           *rate.Limiter
	maxAttempts       int
	backoff           func(attempt int) time.Duration
	usageMu           sync.RWMutex
	usage             Usage
	usageByLabel      map[string]Usage
	budgetMu          sync.Mutex
	budgetFired       bool
	cache             *responseCache
	cacheTTL          time.Duration
	metrics           MetricsRecorder
	breaker           *CircuitBreaker
	lastDurationNanos atomic.Int64
	annotationMu      sync.Mutex
	recentAnnotations []AnnotationRecord
	annotationCounter uint64
	annotationStore   AnnotationStore
}

// responseCache is a TTL-keyed string cache for LLM responses. A nil
// *responseCache is a valid no-op receiver so callers can disable caching
// without conditionals at the call site.
type responseCache struct {
	mu      sync.RWMutex
	entries map[string]responseCacheEntry
}

type responseCacheEntry struct {
	content   string
	expiresAt time.Time
}

func (c *responseCache) get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.content, true
}

func (c *responseCache) put(key, content string, ttl time.Duration) {
	if c == nil || ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]responseCacheEntry)
	}
	c.entries[key] = responseCacheEntry{content: content, expiresAt: time.Now().Add(ttl)}
}

// cacheKey hashes the deterministic portion of FailureContext. OccurredAt
// is excluded so repeated annotations of the same failure share a cache
// entry even when their timestamps differ.
func cacheKey(fc FailureContext) string {
	fc.OccurredAt = time.Time{}
	data, _ := json.Marshal(fc)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Usage is the cumulative token/accounting snapshot for a KimiClient.
// It is safe to read from any goroutine via Usage().
type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	Requests         int64 `json:"requests"`
}

// NewKimiClient returns a KimiClient wired to the given config. It validates
// the config and returns an error if the API key is missing.
func NewKimiClient(cfg Config) (*KimiClient, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	metrics := cfg.Metrics
	if metrics == nil {
		metrics = noopMetrics{}
	}
	budgetCb := cfg.BudgetCallback
	if cfg.BudgetThreshold > 0 && budgetCb != nil && cfg.Breaker != nil {
		br := cfg.Breaker
		originalCb := budgetCb
		budgetCb = func(u Usage) {
			// apigateway.ForceOpen does not arm manual override, so a
			// follow-up success within the recovery window would still
			// auto-close the breaker and re-enable LLM calls. Pairing it
			// with SetManualOverride(true) preserves the budget semantic
			// of "stay open until an operator resets".
			br.ForceOpen()
			br.SetManualOverride(true)
			originalCb(u)
		}
	}
	return &KimiClient{
		cfg: Config{
			APIKey:          cfg.APIKey,
			BaseURL:         cfg.BaseURL,
			Model:           cfg.Model,
			Timeout:         cfg.Timeout,
			MaxTokens:       cfg.MaxTokens,
			BudgetThreshold: cfg.BudgetThreshold,
			BudgetCallback:  budgetCb,
			Metrics:         cfg.Metrics,
		},
		hc:           &http.Client{Timeout: cfg.Timeout},
		limiter:      rate.NewLimiter(rate.Every(time.Second), 4),
		maxAttempts:  3,
		backoff:      defaultBackoff,
		usageByLabel: make(map[string]Usage),
		cache:        &responseCache{},
		cacheTTL:     time.Hour,
		metrics:      metrics,
		breaker:      cfg.Breaker,
	}, nil
}

// BreakerState returns the circuit breaker's state as a string. Returns
// "disabled" when no breaker was configured.
func (k *KimiClient) BreakerState() string {
	if k.breaker == nil {
		return "disabled"
	}
	return k.breaker.Snapshot()
}

// Usage returns the cumulative token/request snapshot for this client.
// Safe to call from any goroutine.
func (k *KimiClient) Usage() Usage {
	k.usageMu.RLock()
	defer k.usageMu.RUnlock()
	return k.usage
}

// UsageByLabel returns the cumulative Usage recorded against the given
// per-feature label. A zero Usage is returned for unknown labels. Safe
// to call from any goroutine.
func (k *KimiClient) UsageByLabel(label string) Usage {
	k.usageMu.RLock()
	defer k.usageMu.RUnlock()
	return k.usageByLabel[label]
}

// UsageAll returns a copy of the per-label usage map. The returned map is
// safe to mutate; it does not affect the client's internal state.
func (k *KimiClient) UsageAll() map[string]Usage {
	k.usageMu.RLock()
	defer k.usageMu.RUnlock()
	out := make(map[string]Usage, len(k.usageByLabel))
	maps.Copy(out, k.usageByLabel)
	return out
}

// Latency returns the wall-clock duration of the most recent Annotate call,
// including any retries. Returns 0 if Annotate has not yet been called.
// Safe to call from any goroutine.
func (k *KimiClient) Latency() time.Duration {
	return time.Duration(k.lastDurationNanos.Load())
}

// Snapshot is a point-in-time view of a KimiClient's operational state.
// It is safe to read from any goroutine.
type Snapshot struct {
	Latency      time.Duration    `json:"latency"`
	Usage        Usage            `json:"usage"`
	UsageByLabel map[string]Usage `json:"usage_by_label"`
	Provider     string           `json:"provider"`
}

// Snapshot returns a Snapshot of the client's current operational state.
// Safe to call from any goroutine.
func (k *KimiClient) Snapshot() Snapshot {
	k.usageMu.RLock()
	byLabel := make(map[string]Usage, len(k.usageByLabel))
	maps.Copy(byLabel, k.usageByLabel)
	usage := k.usage
	k.usageMu.RUnlock()
	return Snapshot{
		Latency:      k.Latency(),
		Usage:        usage,
		UsageByLabel: byLabel,
		Provider:     k.Name(),
	}
}

func defaultBackoff(attempt int) time.Duration {
	return time.Duration(100*(1<<attempt)) * time.Millisecond
}

// Name implements Annotator.
func (k *KimiClient) Name() string { return "kimi" }

// Annotate implements Annotator. It blocks on the rate limiter, checks the
// response cache, then sends the chat-completion request with automatic retry
// on transient errors (5xx, 429). Client errors (4xx) and transport errors
// fail fast. Cache hits return immediately without an HTTP round-trip and
// are not counted in Usage.
func (k *KimiClient) Annotate(ctx context.Context, fc FailureContext) (string, error) {
	start := time.Now()
	callID := k.nextAnnotationID()
	var finalOutcome string
	var finalTokens int64

	defer func() {
		dur := time.Since(start)
		k.lastDurationNanos.Store(int64(dur))
		k.metrics.RecordGauge("llm_annotator_last_call_seconds", dur.Seconds(),
			map[string]string{"provider": "kimi"})
		k.appendAnnotation(AnnotationRecord{
			ID:        callID,
			Timestamp: start,
			Label:     fc.Label,
			Tokens:    finalTokens,
			Outcome:   finalOutcome,
			LatencyMs: dur.Milliseconds(),
		})
	}()

	if err := k.limiter.Wait(ctx); err != nil {
		k.recordOutcome(&finalOutcome, "rate_limited", 0)
		return "", fmt.Errorf("%w: rate limit wait: %v", ErrUnavailable, err)
	}

	body := buildRequest(k.cfg, fc)
	raw, err := json.Marshal(body)
	if err != nil {
		k.recordOutcome(&finalOutcome, "protocol_error", 0)
		return "", fmt.Errorf("%w: marshal request: %v", ErrUnavailable, err)
	}

	var key string
	if k.cacheTTL > 0 {
		key = cacheKey(fc)
		if content, hit := k.cache.get(key); hit {
			k.recordOutcome(&finalOutcome, "cache_hit", 0)
			return content, nil
		}
	}

	var lastErr error
	var content string
	op := func() error {
		var opErr error
		var opStatus int
		for attempt := 0; attempt < k.maxAttempts; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					k.recordOutcome(&finalOutcome, "canceled", 0)
					return fmt.Errorf("%w: context done during backoff: %v", ErrUnavailable, ctx.Err())
				case <-time.After(k.backoff(attempt - 1)):
				}
			}

			c, used, retryable, status, err := k.doRequest(ctx, raw)
			if err == nil {
				content = c
				finalTokens = used.TotalTokens
				k.recordUsage(used, fc.Label)
				k.recordOutcome(&finalOutcome, "success", 0)
				if key != "" {
					k.cache.put(key, content, k.cacheTTL)
				}
				return nil
			}
			opErr = err
			opStatus = status
			if !retryable {
				lastErr = err
				if status == 0 {
					k.recordOutcome(&finalOutcome, "protocol_error", 0)
				} else {
					k.recordOutcome(&finalOutcome, "client_error", status)
				}
				return err
			}
			k.recordOutcome(&finalOutcome, "retry", 0)
		}
		lastErr = opErr
		if opStatus == 0 {
			k.recordOutcome(&finalOutcome, "transport_error", 0)
		} else {
			k.recordOutcome(&finalOutcome, "retry_exhausted", opStatus)
		}
		return opErr
	}

	if k.breaker != nil && k.breaker.IsOpen() {
		k.recordOutcome(&finalOutcome, "circuit_open", 0)
		return "", fmt.Errorf("%w: circuit breaker open", ErrUnavailable)
	}

	if k.breaker != nil {
		if cbErr := k.breaker.Call(op); cbErr != nil && cbErr != lastErr {
			k.recordOutcome(&finalOutcome, "circuit_open", 0)
			return "", fmt.Errorf("%w: %v", ErrUnavailable, cbErr)
		}
	} else {
		_ = op()
	}
	if lastErr != nil && content == "" {
		return "", lastErr
	}
	return content, nil
}

func (k *KimiClient) recordOutcome(outcomeSink *string, outcome string, status int) {
	if outcomeSink != nil {
		*outcomeSink = outcome
	}
	labels := map[string]string{"provider": "kimi", "outcome": outcome}
	if status > 0 {
		labels["status"] = strconv.Itoa(status)
	}
	k.metrics.RecordCounter("llm_annotator_requests_total", 1, labels)
}

func (k *KimiClient) recordUsage(u Usage, label string) {
	k.usageMu.Lock()
	k.usage.PromptTokens += u.PromptTokens
	k.usage.CompletionTokens += u.CompletionTokens
	k.usage.TotalTokens += u.TotalTokens
	k.usage.Requests++
	if label != "" {
		lu := k.usageByLabel[label]
		lu.PromptTokens += u.PromptTokens
		lu.CompletionTokens += u.CompletionTokens
		lu.TotalTokens += u.TotalTokens
		lu.Requests++
		k.usageByLabel[label] = lu
	}
	snapshot := k.usage
	k.usageMu.Unlock()

	threshold := k.cfg.BudgetThreshold
	callback := k.cfg.BudgetCallback
	if threshold <= 0 || callback == nil {
		return
	}
	k.budgetMu.Lock()
	if k.budgetFired {
		k.budgetMu.Unlock()
		return
	}
	if snapshot.TotalTokens < threshold {
		k.budgetMu.Unlock()
		return
	}
	k.budgetFired = true
	k.budgetMu.Unlock()
	callback(snapshot)
}

func (k *KimiClient) doRequest(ctx context.Context, raw []byte) (string, Usage, bool, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		k.cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", Usage{}, false, 0, fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.cfg.APIKey)

	resp, err := k.hc.Do(req)
	if err != nil {
		return "", Usage{}, true, 0, fmt.Errorf("%w: http: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", Usage{}, true, resp.StatusCode, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if resp.StatusCode/100 != 2 {
		retryable := resp.StatusCode == 429 || resp.StatusCode/100 == 5
		return "", Usage{}, retryable, resp.StatusCode, fmt.Errorf("%w: status %d: %s",
			ErrUnavailable, resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", Usage{}, false, resp.StatusCode, fmt.Errorf("%w: unmarshal: %v", ErrUnavailable, err)
	}
	if len(parsed.Choices) == 0 {
		return "", Usage{}, false, resp.StatusCode, fmt.Errorf("%w: empty choices", ErrUnavailable)
	}
	return parsed.Choices[0].Message.Content, Usage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}, false, resp.StatusCode, nil
}

// buildRequest assembles the OpenAI-compatible chat completions body.
func buildRequest(cfg Config, fc FailureContext) map[string]any {
	return map[string]any{
		"model":       cfg.Model,
		"max_tokens":  cfg.MaxTokens,
		"temperature": 0.2,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": "你是 Atlas-Go 台股投資系統的失效歸因助手。" +
					"請用繁體中文，1-2 句話，解釋為何這條心法在當前資料下未觸發。" +
					"不要給出投資建議，只描述失效原因。",
			},
			{
				"role":    "user",
				"content": failureContextToPrompt(fc),
			},
		},
	}
}

// failureContextToPrompt renders fc as a single user message. The format is
// deliberately key=value so the model can parse it without ambiguity and so
// that test fixtures can assert on the prompt text.
func failureContextToPrompt(fc FailureContext) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "frame_id=%s\n", fc.FrameID)
	fmt.Fprintf(&b, "frame_name=%s\n", fc.FrameName)
	fmt.Fprintf(&b, "layer=%s\n", fc.Layer)
	fmt.Fprintf(&b, "occurred_at=%s\n", fc.OccurredAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "macro.foreign_capital_net_twd=%.2f\n", fc.Snap.ForeignInvestorNet)
	fmt.Fprintf(&b, "macro.tsm_adr_pct=%.2f\n", fc.Snap.TSMADR)
	fmt.Fprintf(&b, "macro.nvda_pct=%.2f\n", fc.Snap.NVDA)
	fmt.Fprintf(&b, "macro.dxy_pct=%.2f\n", fc.Snap.DXY)
	fmt.Fprintf(&b, "macro.usd_twd=%.4f\n", fc.Snap.USD_TWD)
	fmt.Fprintf(&b, "macro.retail_margin_balance=%.2f\n", fc.Snap.RetailMarginBalance)
	fmt.Fprintf(&b, "macro.domestic_fund_net=%.2f\n", fc.Snap.DomesticFundNet)
	fmt.Fprintf(&b, "macro.dealer_net=%.2f\n", fc.Snap.DealerNet)
	fmt.Fprintf(&b, "macro.vix=%.2f\n", fc.Snap.VIX)
	fmt.Fprintf(&b, "macro.us10y=%.4f\n", fc.Snap.US10Y)
	b.WriteString("conditions:\n")
	for i, c := range fc.Conditions {
		fmt.Fprintf(&b, "  - [%d] field=%s op=%s threshold=%.4f actual=%.4f timeframe=%s\n",
			i, c.Field, c.Operator, c.Threshold, c.ActualValue, c.Timeframe)
	}
	return b.String()
}

// chatResponse is the OpenAI-compatible chat completions response. We
// only need the first choice's content and the usage block for cost tracking.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}
