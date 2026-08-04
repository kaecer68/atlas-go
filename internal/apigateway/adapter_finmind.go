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

// FinMindChannelAdapter adapts a FinMindClient to the DataProvider interface.
type FinMindChannelAdapter struct {
	client *marketdata.FinMindClient
}

// NewFinMindChannelAdapter creates a new adapter for the FinMind channel.
func NewFinMindChannelAdapter(client *marketdata.FinMindClient) *FinMindChannelAdapter {
	return &FinMindChannelAdapter{client: client}
}

// yesterday returns a date string for yesterday in YYYY-MM-DD format.
func yesterday() string {
	t := time.Now().AddDate(0, 0, -1)
	switch t.Weekday() {
	case time.Saturday:
		t = t.AddDate(0, 0, -1) // Friday
	case time.Sunday:
		t = t.AddDate(0, 0, -2) // Friday
	case time.Monday:
		t = t.AddDate(0, 0, -3) // Friday
	}
	return t.Format("2006-01-02")
}

func (a *FinMindChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	quote, err := a.client.GetStockPrice(ctx, "2330", yesterday())
	if err != nil {
		return nil, fmt.Errorf("finmind fetch: %w", err)
	}
	data, err := json.Marshal(quote)
	if err != nil {
		return nil, fmt.Errorf("finmind marshal: %w", err)
	}
	limiter := a.RateLimit()
	saveSnapshot("finmind", data)
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "finmind",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching 2330 from yesterday.
//
// Daily-quota exhaustion is surfaced as "warn" (not "error") because the
// underlying channel is healthy — the budget just ran out for the day.
// On-call should not be paged for this; the dashboard surfaces it via
// the channel-health page and the budget auto-resets at 00:00 TW.
func (a *FinMindChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.client.GetStockPrice(ctx, "2330", yesterday())
	if err != nil {
		status := "error"
		if errors.Is(err, marketdata.ErrQuotaExhausted) {
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

// RateLimit returns the underlying FinMind client rate limiter.
func (a *FinMindChannelAdapter) RateLimit() *rate.Limiter {
	return a.client.RateLimiter()
}

// Metadata returns static channel metadata for FinMind.
func (a *FinMindChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "finmind",
		Country:    "台灣",
		Platform:   "FinMind",
		APIFormat:  "json",
		Path:       "api.finmindtrade.com",
		HasLimiter: true,
	}
}
