package llm_annotator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
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
	cfg         Config
	hc          *http.Client
	limiter     *rate.Limiter
	maxAttempts int
	backoff     func(attempt int) time.Duration
	usageMu     sync.Mutex
	usage       Usage
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
	return &KimiClient{
		cfg:         cfg,
		hc:          &http.Client{Timeout: cfg.Timeout},
		limiter:     rate.NewLimiter(rate.Every(time.Second), 4),
		maxAttempts: 3,
		backoff:     defaultBackoff,
	}, nil
}

// Usage returns the cumulative token/request snapshot for this client.
// Safe to call from any goroutine.
func (k *KimiClient) Usage() Usage {
	return k.usage
}

func defaultBackoff(attempt int) time.Duration {
	return time.Duration(100*(1<<attempt)) * time.Millisecond
}

// Name implements Annotator.
func (k *KimiClient) Name() string { return "kimi" }

// Annotate implements Annotator. It blocks on the rate limiter, then sends
// the chat-completion request with automatic retry on transient errors
// (5xx, 429). Client errors (4xx) and transport errors fail fast.
func (k *KimiClient) Annotate(ctx context.Context, fc FailureContext) (string, error) {
	if err := k.limiter.Wait(ctx); err != nil {
		return "", fmt.Errorf("%w: rate limit wait: %v", ErrUnavailable, err)
	}

	body := buildRequest(k.cfg, fc)
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("%w: marshal request: %v", ErrUnavailable, err)
	}

	var lastErr error
	for attempt := 0; attempt < k.maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("%w: context done during backoff: %v", ErrUnavailable, ctx.Err())
			case <-time.After(k.backoff(attempt - 1)):
			}
		}

		content, used, retryable, err := k.doRequest(ctx, raw)
		if err == nil {
			k.recordUsage(used)
			return content, nil
		}
		lastErr = err
		if !retryable {
			return "", err
		}
	}
	return "", lastErr
}

func (k *KimiClient) recordUsage(u Usage) {
	k.usageMu.Lock()
	defer k.usageMu.Unlock()
	k.usage.PromptTokens += u.PromptTokens
	k.usage.CompletionTokens += u.CompletionTokens
	k.usage.TotalTokens += u.TotalTokens
	k.usage.Requests++
}

func (k *KimiClient) doRequest(ctx context.Context, raw []byte) (string, Usage, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		k.cfg.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", Usage{}, false, fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+k.cfg.APIKey)

	resp, err := k.hc.Do(req)
	if err != nil {
		return "", Usage{}, true, fmt.Errorf("%w: http: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", Usage{}, true, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if resp.StatusCode/100 != 2 {
		retryable := resp.StatusCode == 429 || resp.StatusCode/100 == 5
		return "", Usage{}, retryable, fmt.Errorf("%w: status %d: %s",
			ErrUnavailable, resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", Usage{}, false, fmt.Errorf("%w: unmarshal: %v", ErrUnavailable, err)
	}
	if len(parsed.Choices) == 0 {
		return "", Usage{}, false, fmt.Errorf("%w: empty choices", ErrUnavailable)
	}
	return parsed.Choices[0].Message.Content, Usage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}, false, nil
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
