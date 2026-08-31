package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// TWSESectorIndexChannelAdapter wraps TWSESectorIndexProvider as a Gateway channel.
// It fetches the Taiwan Semiconductor Index (TAISEMI, 半導體類指數) from TWSE
// OpenAPI v1 and returns it as a MacroDataPoint suitable for MacroDataSnapshot.
type TWSESectorIndexChannelAdapter struct {
	provider *marketdata.TWSESectorIndexProvider
	limiter  *rate.Limiter
}

// NewTWSESectorIndexChannelAdapter creates a channel adapter for TWSE sector indices.
func NewTWSESectorIndexChannelAdapter(provider *marketdata.TWSESectorIndexProvider) *TWSESectorIndexChannelAdapter {
	return &TWSESectorIndexChannelAdapter{
		provider: provider,
		// P1-13: rate limiting is enforced by the provider-side shared TWSE
		// bucket; this adapter's 5s bucket used to double-wait (k3 audit Low
		// 2026-08-24). rate.Inf keeps the Wait as a ctx-cancellation check
		// point without throttling; Tokens() stays full as RateLimit metadata.
		limiter: rate.NewLimiter(rate.Inf, 0),
	}
}

// Fetch retrieves the latest Taiwan Semiconductor Index reading.
func (a *TWSESectorIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()

	// P1-13: rate limiting is enforced by the provider-side shared TWSE
	// bucket (getTWSESharedLimiter); this adapter's own 5s bucket used to
	// double-wait (k3 audit Low 2026-08-24). The limiter is now rate.Inf
	// (pure RateLimit metadata, Tokens() full) and the Wait was replaced by
	// an explicit ctx check that keeps the cancelled-context contract.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("twse_sector_index: %w", err)
	}

	today := time.Now()
	data, err := a.provider.FetchSectorIndices(ctx, today, today)
	if err != nil && errors.Is(err, marketdata.ErrLatestOnly) {
		// MI_INDEX openapi is latest-only (G2): whenever the upstream's
		// latest session is not TODAY (weekends, TW holidays, pre-open
		// mornings), the response date is the previous session while the
		// request said today, so the MUST-2 guard rejects it and the error
		// tripped the circuit breaker for entire weekends (#1767). Retry
		// with the trading day strictly BEFORE today — on Sunday, and on
		// Monday pre-open, that is Friday, whose data upstream holds.
		prev := taiwanholidays.PreviousTradingDay(today.AddDate(0, 0, -1), 0)
		data, err = a.provider.FetchSectorIndices(ctx, prev, prev)
	}
	if err != nil {
		return nil, fmt.Errorf("twse_sector_index: fetch: %w", err)
	}

	semiData, ok := data["semiconductor"]
	if !ok || len(semiData) == 0 {
		return nil, fmt.Errorf("twse_sector_index: semiconductor index not found in TWSE response")
	}

	latest := semiData[len(semiData)-1]

	point := marketdata.MacroDataPoint{
		Symbol:    "TAISEMI",
		Value:     latest.Index,
		ChangePct: latest.ReturnPct,
		Timestamp: time.Now().Unix(),
	}

	jsonData, err := json.Marshal(point)
	if err != nil {
		return nil, fmt.Errorf("twse_sector_index: marshal: %w", err)
	}

	return &FetchResult{
		Data: jsonData,
		Meta: FetchMetadata{
			ChannelID: "twse_sector_index",
			LatencyMs: time.Since(start).Milliseconds(),
			Timestamp: time.Now(),
		},
	}, nil
}

// HealthCheck verifies TWSE OpenAPI connectivity by fetching a single-day range.
func (a *TWSESectorIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	today := time.Now()
	_, err := a.provider.FetchSectorIndices(ctx, today, today)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

// RateLimit returns the rate limiter.
func (a *TWSESectorIndexChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns channel metadata.
func (a *TWSESectorIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_sector_index",
		Country:    "台灣",
		Platform:   "TWSE OpenAPI v1",
		APIFormat:  "REST JSON",
		Path:       "openapi.twse.com.tw",
		HasLimiter: true,
	}
}
