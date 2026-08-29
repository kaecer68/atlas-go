package live

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

type HTTPBrokerAdapterConfig struct {
	BaseURL              string
	APIKey               string
	APISecret            string
	KeyID                string
	Timeout              time.Duration
	MaxAttempts          int
	RetryableStatusCodes []int
	MaxClockSkew         time.Duration
	NonceTTL             time.Duration
	NonceStore           livestore.NonceReplayStore
	Client               *http.Client
	Signer               string
	Now                  func() time.Time
	CurrentTime          func() time.Time
	Nonce                func() string
}

type HTTPBrokerAdapter struct {
	baseURL              string
	apiKey               string
	apiSecret            string
	keyID                string
	timeout              time.Duration
	maxAttempts          int
	retryableStatusCodes map[int]bool
	client               *http.Client
	signerName           string
	signerVer            string
	signer               requestSigner
	nowFn                func() time.Time
	currentTimeFn        func() time.Time
	nonceFn              func() string
	maxClockSkew         time.Duration
	nonceTTL             time.Duration
	nonceStore           livestore.NonceReplayStore
}

type requestSigner interface {
	Name() string
	Version() string
	Sign(payload []byte, secret string, timestamp string, nonce string) string
}

type placeholderSigner struct{}

func (s placeholderSigner) Name() string { return "placeholder" }

func (s placeholderSigner) Version() string { return "v1" }

func (s placeholderSigner) Sign(payload []byte, secret string, timestamp string, nonce string) string {
	h := sha256.Sum256(canonicalSignInput(payload, secret, timestamp, nonce))
	return "placeholder-" + hex.EncodeToString(h[:])
}

// deadSigner is a fail-closed signer. It produces an empty signature that will
// be rejected by any broker API with a 401/403 before any order is submitted.
// It is used when selectSigner receives an unrecognized signer name (typo or
// misconfiguration), preventing silent fallback to a working placeholder.
// (CRIT-2 fix, audit 2026-07-06)
type deadSigner struct{}

func (s deadSigner) Name() string    { return "dead" }
func (s deadSigner) Version() string { return "v0" }

func (s deadSigner) Sign(payload []byte, secret string, timestamp string, nonce string) string {
	return ""
}

type hmacSHA256Signer struct{}

func (s hmacSHA256Signer) Name() string { return "hmac-sha256" }

func (s hmacSHA256Signer) Version() string { return "v1" }

func (s hmacSHA256Signer) Sign(payload []byte, secret string, timestamp string, nonce string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write(canonicalSignInput(payload, secret, timestamp, nonce))
	return hex.EncodeToString(h.Sum(nil))
}

type brokerHTTPError struct {
	message   string
	code      string
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
	retryableStatusCodes := toRetryableStatusCodeSet(cfg.RetryableStatusCodes)
	client := cfg.Client
	if client == nil {
		client = httpclient.NewFactory().NewClient(timeout)
	}
	signerName, signer := selectSigner(cfg.Signer)
	keyID := strings.TrimSpace(cfg.KeyID)
	if keyID == "" {
		keyID = "default"
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	nonceFn := cfg.Nonce
	if nonceFn == nil {
		nonceFn = defaultRequestNonce
	}
	currentTimeFn := cfg.CurrentTime
	if currentTimeFn == nil {
		currentTimeFn = time.Now
	}
	maxClockSkew := max(cfg.MaxClockSkew, 0)
	nonceTTL := cfg.NonceTTL
	if nonceTTL <= 0 {
		nonceTTL = 5 * time.Minute
	}
	nonceStore := cfg.NonceStore
	if nonceStore == nil {
		nonceStore = livestore.NewInMemoryNonceReplayStore()
	}

	return &HTTPBrokerAdapter{
		baseURL:              strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:               strings.TrimSpace(cfg.APIKey),
		apiSecret:            strings.TrimSpace(cfg.APISecret),
		keyID:                keyID,
		timeout:              timeout,
		maxAttempts:          maxAttempts,
		retryableStatusCodes: retryableStatusCodes,
		client:               client,
		signerName:           signerName,
		signerVer:            signer.Version(),
		signer:               signer,
		nowFn:                nowFn,
		currentTimeFn:        currentTimeFn,
		nonceFn:              nonceFn,
		maxClockSkew:         maxClockSkew,
		nonceTTL:             nonceTTL,
		nonceStore:           nonceStore,
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
	if err := a.validateSignerConfig(); err != nil {
		return BrokerResult{}, err
	}

	payload := map[string]any{
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
	requestTime := a.nowFn().UTC()
	if err := a.validateClockSkew(requestTime); err != nil {
		return BrokerResult{}, err
	}
	timestamp := requestTime.Format(time.RFC3339Nano)
	nonce := a.nonceFn()
	if strings.TrimSpace(nonce) == "" {
		nonce = defaultRequestNonce()
	}
	if err := a.registerRequestNonce(nonce, requestTime); err != nil {
		return BrokerResult{}, err
	}

	var lastErr error
	for attempt := 1; attempt <= a.maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, a.timeout)
		res, err := a.sendOrderRequest(attemptCtx, body, idempotencyKey, timestamp, nonce)
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

func (a *HTTPBrokerAdapter) validateSignerConfig() error {
	if a.signerName == "hmac-sha256" && strings.TrimSpace(a.apiSecret) == "" {
		return &brokerHTTPError{message: "hmac signer requires non-empty api secret", code: "signer.misconfigured", retryable: false}
	}
	return nil
}

func (a *HTTPBrokerAdapter) validateClockSkew(requestTime time.Time) error {
	if a.maxClockSkew <= 0 {
		return nil
	}
	now := a.currentTimeFn().UTC()
	delta := now.Sub(requestTime.UTC())
	if delta < 0 {
		delta = -delta
	}
	if delta > a.maxClockSkew {
		return fmt.Errorf("request timestamp outside allowed skew: delta=%s allowed=%s", delta, a.maxClockSkew)
	}
	return nil
}

func (a *HTTPBrokerAdapter) registerRequestNonce(nonce string, requestTime time.Time) error {
	if a.nonceStore == nil {
		a.nonceStore = livestore.NewInMemoryNonceReplayStore()
	}
	err := a.nonceStore.Register(nonce, requestTime, a.nonceTTL)
	if err != nil {
		if errors.Is(err, livestore.ErrNonceReplayDetected) {
			return fmt.Errorf("request nonce replay detected: nonce=%s", nonce)
		}
		return fmt.Errorf("register nonce replay guard: %w", err)
	}
	return nil
}

func (a *HTTPBrokerAdapter) sendOrderRequest(ctx context.Context, body []byte, idempotencyKey string, timestamp string, nonce string) (BrokerResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/orders", bytes.NewReader(body))
	if err != nil {
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("build request: %v", err), code: "request.build_error", retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", a.apiKey)
	req.Header.Set("X-Signature", a.signer.Sign(body, a.apiSecret, timestamp, nonce))
	req.Header.Set("X-Signature-Method", a.signerName)
	req.Header.Set("X-Signature-Version", a.signerVer)
	req.Header.Set("X-Key-Id", a.keyID)
	req.Header.Set("X-Idempotency-Key", idempotencyKey)
	req.Header.Set("X-Request-Timestamp", timestamp)
	req.Header.Set("X-Request-Nonce", nonce)

	resp, err := a.client.Do(req)
	if err != nil {
		retryable := true
		if errors.Is(err, context.Canceled) {
			retryable = false
		}
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("http request failed: %v", err), code: "transport.request_failed", retryable: retryable}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		retryable := a.retryableStatusCodes[resp.StatusCode]
		prefix := "broker rejected request"
		if resp.StatusCode >= 500 {
			prefix = "broker server error"
		}
		code := classifyBrokerErrorCode(resp.StatusCode)
		return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("%s: code=%s status=%d body=%s", prefix, code, resp.StatusCode, strings.TrimSpace(string(respBody))), code: code, retryable: retryable}
	}

	var payload struct {
		OrderID   string  `json:"order_id"`
		Status    string  `json:"status"`
		FillPrice float64 `json:"fill_price"`
		Reason    string  `json:"reason"`
	}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &payload); err != nil {
			return BrokerResult{}, &brokerHTTPError{message: fmt.Sprintf("decode broker response: %v", err), code: "response.decode_error", retryable: false}
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

func classifyBrokerErrorCode(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "auth.signature_invalid"
	case http.StatusTooManyRequests:
		return "throttle.rate_limited"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "request.invalid"
	default:
		if statusCode >= 500 {
			return "server.unavailable"
		}
		return "request.rejected"
	}
}

func orderIdempotencyKey(order domain.Order) string {
	base := fmt.Sprintf("%s|%s|%d|%.6f|%s", order.Symbol, order.Side, order.Quantity, order.Price, order.Reason)
	h := sha256.Sum256([]byte(base))
	return "atlas-" + hex.EncodeToString(h[:16])
}

func isRetryableBrokerError(err error) bool {
	if be, ok := errors.AsType[*brokerHTTPError](err); ok {
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
		// Fail-closed: unrecognized signer name (typo or misconfiguration).
		// Returns deadSigner whose Sign() returns empty string, causing the
		// broker API to reject with 401 before any order is submitted.
		// (CRIT-2 fix, audit 2026-07-06)
		return "dead", deadSigner{}
	}
}

func defaultRetryableStatusCodes() []int {
	return []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooEarly,            // 425
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}
}

func toRetryableStatusCodeSet(codes []int) map[int]bool {
	selected := codes
	if len(selected) == 0 {
		selected = defaultRetryableStatusCodes()
	}
	set := make(map[int]bool, len(selected))
	for _, code := range selected {
		if code >= 400 && code <= 599 {
			set[code] = true
		}
	}
	return set
}

func canonicalSignInput(payload []byte, secret string, timestamp string, nonce string) []byte {
	return []byte(secret + "\n" + timestamp + "\n" + nonce + "\n" + string(payload))
}

func defaultRequestNonce() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		fallback := sha256.Sum256([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
		return hex.EncodeToString(fallback[:12])
	}
	return hex.EncodeToString(b)
}
