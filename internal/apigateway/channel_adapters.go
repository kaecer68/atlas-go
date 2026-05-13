package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// FugleChannelAdapter — wraps *marketdata.FugleClient
// ---------------------------------------------------------------------------

// FugleChannelAdapter adapts a FugleClient to the DataProvider interface.
type FugleChannelAdapter struct {
	client *marketdata.FugleClient
}

// NewFugleChannelAdapter creates a new adapter for the Fugle channel.
func NewFugleChannelAdapter(client *marketdata.FugleClient) *FugleChannelAdapter {
	return &FugleChannelAdapter{client: client}
}

// Fetch retrieves a quote for 0050 (元大台灣50) as a representative sample.
func (a *FugleChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quote, err := a.client.GetQuote(ctx, "0050")
	if err != nil {
		return nil, fmt.Errorf("fugle fetch: %w", err)
	}
	data, err := json.Marshal(quote)
	if err != nil {
		return nil, fmt.Errorf("fugle marshal: %w", err)
	}
	limiter := a.RateLimit()
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "fugle",
			LatencyMs:          0, // caller should measure
			RateLimitRemaining: int(limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching 1476 (Fugle test symbol).
func (a *FugleChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.client.GetQuote(ctx, "1476")
	if err != nil {
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

// ---------------------------------------------------------------------------
// FinMindChannelAdapter — wraps *marketdata.FinMindClient
// ---------------------------------------------------------------------------

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
	return time.Now().AddDate(0, 0, -1).Format("2006-01-02")
}

// Fetch retrieves a quote for 2330 (台積電) from yesterday as a sample.
func (a *FinMindChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quote, err := a.client.GetStockPrice(ctx, "2330", yesterday())
	if err != nil {
		return nil, fmt.Errorf("finmind fetch: %w", err)
	}
	data, err := json.Marshal(quote)
	if err != nil {
		return nil, fmt.Errorf("finmind marshal: %w", err)
	}
	limiter := a.RateLimit()
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "finmind",
			RateLimitRemaining: int(limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching 2330 from yesterday.
func (a *FinMindChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.client.GetStockPrice(ctx, "2330", yesterday())
	if err != nil {
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

// ---------------------------------------------------------------------------
// TWSEChannelAdapter — wraps *marketdata.TWSEClient
// ---------------------------------------------------------------------------

// TWSEChannelAdapter adapts a TWSEClient to the DataProvider interface.
type TWSEChannelAdapter struct {
	client  *marketdata.TWSEClient
	limiter *rate.Limiter
}

// NewTWSEChannelAdapter creates a new adapter for the TWSE channel.
func NewTWSEChannelAdapter(client *marketdata.TWSEClient) *TWSEChannelAdapter {
	return &TWSEChannelAdapter{
		client:  client,
		limiter: rate.NewLimiter(TWSEOpenAPIRate, TWSEOpenAPIBurst),
	}
}

// Fetch retrieves all quotes from TWSE OpenAPI as a sample dataset.
func (a *TWSEChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quotes, err := a.client.GetQuotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("twse fetch: %w", err)
	}
	data, err := json.Marshal(quotes)
	if err != nil {
		return nil, fmt.Errorf("twse marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_replay",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by attempting a bulk quote fetch.
func (a *TWSEChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.client.GetQuotes(ctx)
	if err != nil {
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

// RateLimit returns the TWSE rate limiter from limits.go.
func (a *TWSEChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE.
func (a *TWSEChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// YahooMacroChannelAdapter — wraps *marketdata.YahooFinanceMacroProvider
// ---------------------------------------------------------------------------

// YahooMacroChannelAdapter adapts a YahooFinanceMacroProvider to the DataProvider interface.
type YahooMacroChannelAdapter struct {
	provider *marketdata.YahooFinanceMacroProvider
	limiter  *rate.Limiter
}

// NewYahooMacroChannelAdapter creates a new adapter for the Yahoo Finance macro channel.
func NewYahooMacroChannelAdapter(provider *marketdata.YahooFinanceMacroProvider) *YahooMacroChannelAdapter {
	return &YahooMacroChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(YahooFinanceRate, YahooFinanceBurst),
	}
}

// Fetch retrieves a full macro data snapshot from Yahoo Finance.
func (a *YahooMacroChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("yahoo macro fetch: %w", err)
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

// HealthCheck verifies connectivity by attempting a snapshot fetch.
// A partial success (some indicators fail) is still treated as ok
// since at least one indicator responded.
func (a *YahooMacroChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// Partial failure is acceptable — at least some data came back.
		if snap.RecordedAt > 0 {
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

// ---------------------------------------------------------------------------
// RegisterChannelAdapters — wires concrete clients into the Gateway registry
// ---------------------------------------------------------------------------

// RegisterChannelAdapters creates concrete market-data clients from cfg,
// wraps each in a channel adapter, and registers them in the Gateway's
// ChannelRegistry. Clients that require API keys are silently skipped
// when the key is not configured.
func RegisterChannelAdapters(g *Gateway, workDir string, cfg config.Config) error {
	if g == nil {
		return fmt.Errorf("gateway is nil")
	}

	// --- Fugle ---
	if cfg.FugleAPIKey != "" {
		fugleClient := marketdata.NewFugleClient(cfg.FugleAPIKey)
		fugleAdapter := NewFugleChannelAdapter(fugleClient)
		g.registry.Register("fugle", fugleAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "fugle")
	}

	// --- FinMind ---
	if cfg.FinMindAPIKey != "" {
		finmindClient := marketdata.NewFinMindClient(cfg.FinMindAPIKey)
		finmindAdapter := NewFinMindChannelAdapter(finmindClient)
		g.registry.Register("finmind", finmindAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "finmind")
	}

	// --- TWSE (no API key required) ---
	twseClient := marketdata.NewTWSEClient()
	twseAdapter := NewTWSEChannelAdapter(twseClient)
	g.registry.Register("twse_replay", twseAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_replay")

	// --- Yahoo Finance Macro ---
	if cfg.YahooEnabled {
		yahooProvider := marketdata.NewYahooFinanceMacroProvider()
		yahooAdapter := NewYahooMacroChannelAdapter(yahooProvider)
		g.registry.Register("us_yahoo", yahooAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "us_yahoo")
	}

	return nil
}
