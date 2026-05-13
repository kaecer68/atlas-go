package live

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// TWSEBrokerAdapterConfig holds configuration for the TWSE broker adapter.
type TWSEBrokerAdapterConfig struct {
	BaseURL   string
	APIKey    string
	APISecret string
	AccountID string
	Timeout   time.Duration
	// MaxRequestsPerSecond controls rate limiting (0 = disabled).
	MaxRequestsPerSecond float64
}

// TWSEBrokerAdapter implements LiveExecutionAdapter for the TWSE REST API.
// It wraps an internal HTTP client with HMAC-SHA256 signing, nonce-based
// replay protection, and per-second rate limiting.
type TWSEBrokerAdapter struct {
	baseURL           string
	apiKey            string
	apiSecret         string
	accountID         string
	timeout           time.Duration
	maxRequestsPerSec float64
	client            *http.Client
	circuitBreaker    *CircuitBreaker
	nowFn             func() time.Time
	mu                sync.Mutex
	lastRequestTime   time.Time
	tokens            float64
}

// NewTWSEBrokerAdapter creates a TWSE broker adapter with the given config
// and circuit breaker. The circuit breaker is checked before every order.
func NewTWSEBrokerAdapter(cfg TWSEBrokerAdapterConfig, cb *CircuitBreaker) *TWSEBrokerAdapter {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	maxRPS := cfg.MaxRequestsPerSecond
	if maxRPS <= 0 {
		maxRPS = 2.0
	}

	return &TWSEBrokerAdapter{
		baseURL:           strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:            strings.TrimSpace(cfg.APIKey),
		apiSecret:         strings.TrimSpace(cfg.APISecret),
		accountID:         strings.TrimSpace(cfg.AccountID),
		timeout:           timeout,
		maxRequestsPerSec: maxRPS,
		client:            httpclient.NewFactory().NewClient(timeout),
		circuitBreaker:    cb,
		nowFn:             time.Now,
		tokens:            maxRPS,
	}
}

func (a *TWSEBrokerAdapter) waitRateLimit() {
	if a.maxRequestsPerSec <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.nowFn()
	elapsed := now.Sub(a.lastRequestTime).Seconds()
	a.tokens += elapsed * a.maxRequestsPerSec
	if a.tokens > a.maxRequestsPerSec {
		a.tokens = a.maxRequestsPerSec
	}
	if a.tokens < 1 {
		waitTime := time.Duration((1-a.tokens)/a.maxRequestsPerSec*float64(time.Second)) + time.Millisecond
		a.mu.Unlock()
		time.Sleep(waitTime)
		a.mu.Lock()
	}
	a.tokens -= 1
	a.lastRequestTime = a.nowFn()
}

func (a *TWSEBrokerAdapter) newNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fallback := sha256.Sum256([]byte(a.nowFn().UTC().Format(time.RFC3339Nano)))
		return hex.EncodeToString(fallback[:16])
	}
	return hex.EncodeToString(b)
}

func (a *TWSEBrokerAdapter) sign(payload []byte, timestamp string, nonce string) string {
	mac := hmac.New(sha256.New, []byte(a.apiSecret))
	_, _ = mac.Write([]byte(a.apiSecret + "\n" + timestamp + "\n" + nonce + "\n" + string(payload)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *TWSEBrokerAdapter) validateConfig() error {
	if a.baseURL == "" {
		return fmt.Errorf("twse_adapter: base_url is required")
	}
	if a.apiKey == "" {
		return fmt.Errorf("twse_adapter: api_key is required")
	}
	if a.apiSecret == "" {
		return fmt.Errorf("twse_adapter: api_secret is required")
	}
	if a.accountID == "" {
		return fmt.Errorf("twse_adapter: account_id is required")
	}
	return nil
}

func mapSide(side domain.Side) string {
	switch side {
	case domain.SideBuy:
		return "B"
	case domain.SideSell:
		return "S"
	default:
		return "B"
	}
}

// SubmitOrder submits an order to the TWSE API. It implements LiveExecutionAdapter.
func (a *TWSEBrokerAdapter) SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error) {
	if err := validateOrder(order); err != nil {
		return BrokerResult{}, err
	}
	if err := a.validateConfig(); err != nil {
		return BrokerResult{}, err
	}
	if a.circuitBreaker != nil && !a.circuitBreaker.CanPlaceOrder(order.Side) {
		return BrokerResult{
			Status: "rejected",
			Reason: fmt.Sprintf("circuit breaker blocked %s order on %s", order.Side, order.Symbol),
		}, fmt.Errorf("twse_adapter: circuit breaker blocked %s order on %s", order.Side, order.Symbol)
	}

	orderType := "L"
	if order.Price <= 0 {
		orderType = "M"
	}

	reqPayload := TWSEOrderRequest{
		Symbol:      order.Symbol,
		Side:        mapSide(order.Side),
		Quantity:    order.Quantity,
		Price:       order.Price,
		OrderType:   orderType,
		TimeInForce: "ROD",
		AccountID:   a.accountID,
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("twse_adapter: marshal order: %w", err)
	}

	timestamp := a.nowFn().UTC().Format(time.RFC3339Nano)
	nonce := a.newNonce()
	signature := a.sign(body, timestamp, nonce)

	a.waitRateLimit()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return BrokerResult{}, fmt.Errorf("twse_adapter: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Signature-Method", "hmac-sha256")
	req.Header.Set("X-Signature-Version", "v1")
	req.Header.Set("X-Request-Timestamp", timestamp)
	req.Header.Set("X-Request-Nonce", nonce)

	resp, err := a.client.Do(req)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("twse_adapter: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("twse_adapter: read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return BrokerResult{}, fmt.Errorf("twse_adapter: API error status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var twseResp TWSEOrderResponse
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &twseResp); err != nil {
			return BrokerResult{}, fmt.Errorf("twse_adapter: decode response: %w", err)
		}
	}

	result := BrokerResult{
		OrderID:   twseResp.OrderID,
		Status:    twseResp.Status,
		FillPrice: twseResp.FillPrice,
		Reason:    twseResp.RejectReason,
	}
	if strings.TrimSpace(result.Status) == "" {
		result.Status = "placed"
	}
	if result.Reason == "" {
		result.Reason = twseResp.Message
	}
	return result, nil
}

// QueryOrderStatus retrieves the current status of a TWSE order.
func (a *TWSEBrokerAdapter) QueryOrderStatus(ctx context.Context, orderID string) (*TWSETradeResult, error) {
	if err := a.validateConfig(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("twse_adapter: order_id is required for status query")
	}

	timestamp := a.nowFn().UTC().Format(time.RFC3339Nano)
	nonce := a.newNonce()
	payload := []byte(orderID)
	signature := a.sign(payload, timestamp, nonce)

	a.waitRateLimit()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/orders/"+orderID, nil)
	if err != nil {
		return nil, fmt.Errorf("twse_adapter: build status request: %w", err)
	}
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Signature-Method", "hmac-sha256")
	req.Header.Set("X-Signature-Version", "v1")
	req.Header.Set("X-Request-Timestamp", timestamp)
	req.Header.Set("X-Request-Nonce", nonce)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twse_adapter: http status request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("twse_adapter: read status response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("twse_adapter: status API error status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result TWSETradeResult
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("twse_adapter: decode status response: %w", err)
		}
	}
	return &result, nil
}

// CancelOrder cancels a pending TWSE order.
func (a *TWSEBrokerAdapter) CancelOrder(ctx context.Context, orderID string) error {
	if err := a.validateConfig(); err != nil {
		return err
	}
	if strings.TrimSpace(orderID) == "" {
		return fmt.Errorf("twse_adapter: order_id is required for cancellation")
	}

	timestamp := a.nowFn().UTC().Format(time.RFC3339Nano)
	nonce := a.newNonce()
	payload := []byte(orderID)
	signature := a.sign(payload, timestamp, nonce)

	a.waitRateLimit()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.baseURL+"/orders/"+orderID, nil)
	if err != nil {
		return fmt.Errorf("twse_adapter: build cancel request: %w", err)
	}
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Signature-Method", "hmac-sha256")
	req.Header.Set("X-Signature-Version", "v1")
	req.Header.Set("X-Request-Timestamp", timestamp)
	req.Header.Set("X-Request-Nonce", nonce)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("twse_adapter: http cancel request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("twse_adapter: read cancel response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("twse_adapter: cancel API error status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
