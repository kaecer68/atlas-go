package live

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	KeyID       string
	Timeout     time.Duration
	MaxAttempts int
	Client      *http.Client
	Signer      string
}

type HTTPBrokerAdapter struct {
	baseURL     string
	apiKey      string
	apiSecret   string
	keyID       string
	timeout     time.Duration
	maxAttempts int
	client      *http.Client
	signerName  string
	signerVer   string
	signer      requestSigner
}

type requestSigner interface {
	Name() string
	Version() string
	Sign(payload []byte, secret string) string
}

type placeholderSigner struct{}

func (s placeholderSigner) Name() string { return "placeholder" }

func (s placeholderSigner) Version() string { return "v1" }

func (s placeholderSigner) Sign(payload []byte, secret string) string {
	h := sha256.Sum256([]byte(secret + ":" + string(payload)))
	return "placeholder-" + hex.EncodeToString(h[:])
}

type hmacSHA256Signer struct{}

func (s hmacSHA256Signer) Name() string { return "hmac-sha256" }

func (s hmacSHA256Signer) Version() string { return "v1" }

func (s hmacSHA256Signer) Sign(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

type brokerHTTPError struct {
	message   string
	retryable bool
}

func (e *brokerHTTPError) Error() string {
	return e.message
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
	signerName, signer := selectSigner(cfg.Signer)
	keyID := strings.TrimSpace(cfg.KeyID)
	if keyID == "" {
		keyID = "default"
	}

	return &HTTPBrokerAdapter{
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:      strings.TrimSpace(cfg.APIKey),
		apiSecret:   strings.TrimSpace(cfg.APISecret),
		keyID:       keyID,
		timeout:     timeout,
		maxAttempts: maxAttempts,
		client:      client,
		signerName:  signerName,
		signerVer:   signer.Version(),
		signer:      signer,
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
	if a.signer == nil {
		a.signerName, a.signer = selectSigner("")
		a.signerVer = a.signer.Version()
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
	idempotencyKey := orderIdempotencyKey(order)

	var lastErr error
	for attempt := 1; attempt <= a.maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, a.timeout)
		res, err := a.sendOrderRequest(attemptCtx, body, idempotencyKey)
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("attempt %d/%d: %w", attempt, a.maxAttempts, err)
			if !isRetryableBrokerError(err) {
				break
			}
			continue
		}
		return res, nil
	}

	return BrokerResult{}, fmt.Errorf("submit order failed after %d attempts: %w", a.maxAttempts, lastErr)
}

func (a *HTTPBrokerAdapter) sendOrderRequest(ctx context.Context, body []byte, idempotencyKey string) (BrokerResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("build request: %v", err), retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", a.apiKey)
	req.Header.Set("X-Signature", a.signer.Sign(body, a.apiSecret))
	req.Header.Set("X-Signature-Method", a.signerName)
	req.Header.Set("X-Signature-Version", a.signerVer)
	req.Header.Set("X-Key-Id", a.keyID)
	req.Header.Set("X-Idempotency-Key", idempotencyKey)

	resp, err := a.client.Do(req)
	if err != nil {
		retryable := true
		if errors.Is(err, context.Canceled) {
			retryable = false
		}
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("http request failed: %v", err), retryable: retryable}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 {
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("broker server error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody))), retryable: true}
	}
	if resp.StatusCode >= 400 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("broker rejected request: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody))), retryable: retryable}
	}

	var payload struct {
		OrderID   string  `json:"order_id"`
		Status    string  `json:"status"`
		FillPrice float64 `json:"fill_price"`
		Reason    string  `json:"reason"`
	}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &payload); err != nil {
			return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("decode broker response: %v", err), retryable: false}
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

func orderIdempotencyKey(order domain.Order) string {
	base := fmt.Sprintf("%s|%s|%d|%.6f|%s", order.Symbol, order.Side, order.Quantity, order.Price, order.Reason)
	h := sha256.Sum256([]byte(base))
	return "atlas-" + hex.EncodeToString(h[:16])
}

func isRetryableBrokerError(err error) bool {
	var be *brokerHTTPError
	if errors.As(err, &be) {
		return be.retryable
	}
	return false
}

func selectSigner(signer string) (string, requestSigner) {
	name := strings.TrimSpace(strings.ToLower(signer))
	switch name {
	case "hmac-sha256":
		return "hmac-sha256", hmacSHA256Signer{}
	case "placeholder", "":
		return "placeholder", placeholderSigner{}
	default:
		return "placeholder", placeholderSigner{}
	}
}
