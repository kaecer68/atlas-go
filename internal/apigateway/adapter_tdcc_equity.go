package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TDCClientChannelAdapter wraps TDCClient as a DataProvider (G01).
type TDCClientChannelAdapter struct {
	provider *marketdata.TDCClient
	limiter  *rate.Limiter
}

// NewTDCClientChannelAdapter creates a TDCC equity dispersion channel adapter.
func NewTDCClientChannelAdapter() *TDCClientChannelAdapter {
	p := marketdata.NewTDCClient()
	return &TDCClientChannelAdapter{provider: p, limiter: p.RateLimiter()}
}

// SetFinMindClient injects the shared FinMind client (G01 live wiring).
func (a *TDCClientChannelAdapter) SetFinMindClient(f *marketdata.FinMindClient) {
	a.provider.SetFinMindClient(f)
}

// SetStorageDir enables weekly snapshot + latest.json persistence.
func (a *TDCClientChannelAdapter) SetStorageDir(dir string) {
	a.provider.SetStorageDir(dir)
}

// StorageDir exposes the configured directory (backfill task wiring).
func (a *TDCClientChannelAdapter) StorageDir() string { return a.provider.StorageDir() }

// Provider exposes the underlying provider (backfill task wiring for the
// G01 monthly history walk).
func (a *TDCClientChannelAdapter) Provider() *marketdata.TDCClient { return a.provider }

func (a *TDCClientChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("tdcc rate limit: %w", err)
	}
	stats, err := a.provider.FetchDispersion(ctx, time.Now().Format("20060102"))
	if err != nil {
		return nil, fmt.Errorf("tdcc_equity_dispersion: %w", err)
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("tdcc marshal: %w", err)
	}
	return &FetchResult{
		Data: payload,
		Meta: FetchMetadata{
			ChannelID:          "tdcc_equity_dispersion",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

func (a *TDCClientChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	// G01 live: report the provider's last fetch outcome instead of
	// re-fetching the ~68k-row weekly table on every health probe.
	lastSuccessAt, lastErr := a.provider.LastFetchState()
	st := struct {
		LastError     string
		LastSuccessAt time.Time
	}{lastErr, lastSuccessAt}
	switch {
	case st.LastError != "":
		return HealthStatus{
			Status:    "error",
			CheckType: "readiness",
			LastError: st.LastError,
			UpdatedAt: time.Now().Format(time.RFC3339),
		}, nil
	case st.LastSuccessAt.IsZero():
		return HealthStatus{
			Status:    "unknown",
			CheckType: "liveness",
			LastError: "no fetch completed yet",
			UpdatedAt: time.Now().Format(time.RFC3339),
		}, nil
	default:
		return HealthStatus{
			Status:    "ok",
			CheckType: "liveness",
			UpdatedAt: time.Now().Format(time.RFC3339),
		}, nil
	}
}

func (a *TDCClientChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TDCClientChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tdcc_equity_dispersion",
		Country:    "TW",
		Platform:   "TDCC",
		APIFormat:  "JSON",
		Path:       "FinMind:TaiwanStockHoldingSharesPer",
		HasLimiter: true,
	}
}
