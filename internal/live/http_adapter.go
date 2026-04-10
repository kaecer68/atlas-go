package live

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type HTTPBrokerAdapterConfig struct {
	BaseURL     string
	APIKey      string
	APISecret   string
	Timeout     time.Duration
	MaxAttempts int
	Client      *http.Client
}

type HTTPBrokerAdapter struct {
	baseURL     string
	apiKey      string
	apiSecret   string
	timeout     time.Duration
	maxAttempts int
	client      *http.Client
}

func NewHTTPBrokerAdapter(cfg HTTPBrokerAdapterConfig) *HTTPBrokerAdapter {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{}
	}

	return &HTTPBrokerAdapter{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:      strings.TrimSpace(cfg.APIKey),
		apiSecret:   strings.TrimSpace(cfg.APISecret),
		timeout:     timeout,
		maxAttempts: maxAttempts,
		client:      client,
	}
}

func (a *HTTPBrokerAdapter) SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error) {
	if err := validateOrder(order); err != nil {
		return BrokerResult{}, err
	}
	if a.baseURL == "" {
		return BrokerResult{}, fmt.Errorf("http broker adapter: base url is required")
	}
	if a.apiKey == "" {
		return BrokerResult{}, fmt.Errorf("http broker adapter: api key is required")
	}

	payload := map[string]interface{}{
		"symbol":   order.Symbol,
		"side":     order.Side,
		"quantity": order.Quantity,
		"price":    order.Price,
		"reason":   order.Reason,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("marshal order payload: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= a.maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, a.timeout)
		res, err := a.sendOrderRequest(attemptCtx, body)
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("attempt %d/%d: %w", attempt, a.maxAttempts, err)
			continue
		}
		return res, nil
	}

	return BrokerResult{}, fmt.Errorf("submit order failed after %d attempts: %w", a.maxAttempts, lastErr)
}

func (a *HTTPBrokerAdapter) sendOrderRequest(ctx context.Context, body []byte) (BrokerResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return BrokerResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("X-Signature", signaturePlaceholder(body, a.apiSecret))

	resp, err := a.client.Do(req)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return BrokerResult{}, fmt.Errorf("broker server error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if resp.StatusCode >= 400 {
		return BrokerResult{}, fmt.Errorf("broker rejected request: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var payload struct {
		OrderID   string  `json:"order_id"`
		Status    string  `json:"status"`
		FillPrice float64 `json:"fill_price"`
		Reason    string  `json:"reason"`
	}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &payload); err != nil {
			return BrokerResult{}, fmt.Errorf("decode broker response: %w", err)
		}
	}
	if strings.TrimSpace(payload.Status) == "" {
		payload.Status = "placed"
	}

	return BrokerResult{
		OrderID:   payload.OrderID,
		Status:    payload.Status,
		FillPrice: payload.FillPrice,
		Reason:    payload.Reason,
	}, nil
}

func signaturePlaceholder(payload []byte, secret string) string {
	h := sha256.Sum256([]byte(secret + ":" + string(payload)))
	return "placeholder-" + hex.EncodeToString(h[:])
}
