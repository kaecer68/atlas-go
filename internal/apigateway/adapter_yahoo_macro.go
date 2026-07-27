package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// YahooMacroChannelAdapter adapts a MacroDataProvider to the DataProvider interface.
type YahooMacroChannelAdapter struct {
	provider marketdata.MacroDataProvider
	limiter  *rate.Limiter
}

// NewYahooMacroChannelAdapter creates a new adapter for the Yahoo Finance macro channel.
func NewYahooMacroChannelAdapter(provider marketdata.MacroDataProvider) *YahooMacroChannelAdapter {
	return &YahooMacroChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(YahooFinanceRate, YahooFinanceBurst),
	}
}

// Fetch retrieves a full macro data snapshot from Yahoo Finance.
func (a *YahooMacroChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// Partial failure is acceptable — at least one macro indicator has
		// valid data (non-empty Symbol). Only treat a total failure (no
		// indicator produced data) as a channel error so the circuit breaker
		// does not open for transient off-hours gaps.
		if snapshotHasAnySymbol(snap) {
			logging.Warn(
				"apigateway", "yahoo_macro_partial_fetch",
				"error", err.Error(),
			)
		} else {
			return nil, fmt.Errorf("yahoo macro fetch: %w", err)
		}
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("yahoo macro marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "us_yahoo",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// snapshotHasAnySymbol returns true if at least one macro indicator in the
// snapshot has a non-empty Symbol (meaning real data was fetched). This
// distinguishes partial success from total failure more accurately than
// checking RecordedAt (which is set before any fetches start).
func snapshotHasAnySymbol(snap marketdata.MacroDataSnapshot) bool {
	return snap.US10Y.Symbol != "" || snap.DXY.Symbol != "" ||
		snap.VIX.Symbol != "" || snap.Oil.Symbol != "" ||
		snap.Gold.Symbol != "" || snap.USD_TWD.Symbol != "" ||
		snap.Silver.Symbol != "" || snap.Copper.Symbol != ""
}

// HealthCheck verifies connectivity by attempting a snapshot fetch.
// A partial success (some indicators fail) is still treated as ok
// since at least one indicator responded.
func (a *YahooMacroChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// snapshotHasAnySymbol distinguishes partial failure (some indicators
		// loaded) from total failure, unlike RecordedAt which is always >0.
		if snapshotHasAnySymbol(snap) {
			return HealthStatus{
				Status:    "warn",
				LastError: err.Error(),
				UpdatedAt: time.Now().Format(time.RFC3339),
				CheckType: "liveness",
			}, nil
		}
		return HealthStatus{
			Status:    "error",
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

// RateLimit returns the Yahoo Finance rate limiter from limits.go.
func (a *YahooMacroChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Yahoo Finance.
func (a *YahooMacroChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "us_yahoo",
		Country:    "美國",
		Platform:   "Yahoo Finance",
		APIFormat:  "json",
		Path:       "query1.finance.yahoo.com",
		HasLimiter: true,
	}
}
