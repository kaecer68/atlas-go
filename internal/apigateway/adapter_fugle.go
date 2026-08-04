package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FugleChannelAdapter adapts a FugleClient to the DataProvider interface.
type FugleChannelAdapter struct {
	client *marketdata.FugleClient
}

// NewFugleChannelAdapter creates a new adapter for the Fugle channel.
func NewFugleChannelAdapter(client *marketdata.FugleClient) *FugleChannelAdapter {
	return &FugleChannelAdapter{client: client}
}

// Fetch retrieves a quote for 1476 (聚亨, Fugle test symbol) as a health check sample.
// Uses the same symbol as HealthCheck() for API key compatibility.
func (a *FugleChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	quote, err := a.client.GetQuote(ctx, "1476")
	if err != nil {
		return nil, fmt.Errorf("fugle fetch: %w", err)
	}
	data, err := json.Marshal(quote)
	if err != nil {
		return nil, fmt.Errorf("fugle marshal: %w", err)
	}
	limiter := a.RateLimit()
	saveSnapshot("fugle", data)
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "fugle",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching 1476 (Fugle test symbol).
//
// Daily-quota exhaustion is surfaced as "warn" (not "error") because the
// channel itself is healthy — the budget just ran out for the day. Mirrors
// the FinMind adapter's warn mapping so the channel-health page presents
// a single, consistent view across both providers (see kaecer feedback
// 2026-08-04: don't fix one provider's quota gate while breaking another's).
func (a *FugleChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.client.GetQuote(ctx, "1476")
	if err != nil {
		status := "error"
		if errors.Is(err, marketdata.ErrFugleQuotaExhausted) {
			status = "warn"
		}
		return HealthStatus{
			Status:    status,
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "liveness",
	}, nil
}

// RateLimit returns the underlying Fugle client rate limiter.
func (a *FugleChannelAdapter) RateLimit() *rate.Limiter {
	return a.client.RateLimiter()
}

// Metadata returns static channel metadata for Fugle.
func (a *FugleChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "fugle",
		Country:    "台灣",
		Platform:   "Fugle 富果",
		APIFormat:  "json",
		Path:       "api.fugle.tw",
		HasLimiter: true,
	}
}
