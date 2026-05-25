package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
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

// Fetch retrieves a quote for 1476 (聚亨, Fugle test symbol) as a health check sample.
// Uses the same symbol as HealthCheck() for API key compatibility.
func (a *FugleChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quote, err := a.client.GetQuote(ctx, "1476")
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
// TWSECapitalFlowChannelAdapter — wraps *marketdata.TWSECapitalFlowProvider
// ---------------------------------------------------------------------------

// TWSECapitalFlowChannelAdapter adapts a TWSECapitalFlowProvider to the DataProvider interface.
type TWSECapitalFlowChannelAdapter struct {
	provider *marketdata.TWSECapitalFlowProvider
	limiter  *rate.Limiter
}

// NewTWSECapitalFlowChannelAdapter creates a new adapter for the TWSE capital flow channel.
func NewTWSECapitalFlowChannelAdapter(provider *marketdata.TWSECapitalFlowProvider) *TWSECapitalFlowChannelAdapter {
	return &TWSECapitalFlowChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(TWSEOpenAPIRate, TWSEOpenAPIBurst),
	}
}

// Fetch retrieves the latest capital flow snapshot.
func (a *TWSECapitalFlowChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("capital_flow fetch: %w", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("capital_flow marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_capital_flow",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TWSECapitalFlowChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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

// RateLimit returns the TWSE capital flow rate limiter.
func (a *TWSECapitalFlowChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE capital flow.
func (a *TWSECapitalFlowChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_capital_flow",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw/rwd/zh/fund/T86",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// TWSEMarginChannelAdapter — wraps *marketdata.TWSEMarginBalanceProvider
// ---------------------------------------------------------------------------

// TWSEMarginChannelAdapter adapts a TWSEMarginBalanceProvider to the DataProvider interface.
type TWSEMarginChannelAdapter struct {
	provider *marketdata.TWSEMarginBalanceProvider
	limiter  *rate.Limiter
}

// NewTWSEMarginChannelAdapter creates a new adapter for the TWSE margin channel.
func NewTWSEMarginChannelAdapter(provider *marketdata.TWSEMarginBalanceProvider) *TWSEMarginChannelAdapter {
	return &TWSEMarginChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(TWSEOpenAPIRate, TWSEOpenAPIBurst),
	}
}

// Fetch retrieves the latest margin balance snapshot.
func (a *TWSEMarginChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("margin fetch: %w", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("margin marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_margin",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TWSEMarginChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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

// RateLimit returns the TWSE margin rate limiter.
func (a *TWSEMarginChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE margin balance.
func (a *TWSEMarginChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_margin",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw/rwd/zh/marginTrading",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// ExportStatisticsChannelAdapter — wraps *marketdata.ExportStatisticsProvider
// ---------------------------------------------------------------------------

// ExportStatisticsChannelAdapter adapts an ExportStatisticsProvider to the DataProvider interface.
type ExportStatisticsChannelAdapter struct {
	provider *marketdata.ExportStatisticsProvider
	limiter  *rate.Limiter
}

// NewExportStatisticsChannelAdapter creates a new adapter for the export statistics channel.
func NewExportStatisticsChannelAdapter(provider *marketdata.ExportStatisticsProvider) *ExportStatisticsChannelAdapter {
	return &ExportStatisticsChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves the latest export statistics snapshot.
func (a *ExportStatisticsChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("export fetch: %w", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("export marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "export_statistics",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *ExportStatisticsChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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

// RateLimit returns the export statistics rate limiter.
func (a *ExportStatisticsChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for export statistics.
func (a *ExportStatisticsChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "export_statistics",
		Country:    "台灣",
		Platform:   "關務署",
		APIFormat:  "csv",
		Path:       "opendata.customs.gov.tw",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// FubonChannelAdapter — wraps *marketdata.FubonClient
// ---------------------------------------------------------------------------

// FubonChannelAdapter adapts a FubonClient to the DataProvider interface.
type FubonChannelAdapter struct {
	client  *marketdata.FubonClient
	limiter *rate.Limiter
}

// NewFubonChannelAdapter creates a new adapter for the Fubon channel.
func NewFubonChannelAdapter(client *marketdata.FubonClient) *FubonChannelAdapter {
	return &FubonChannelAdapter{
		client:  client,
		limiter: rate.NewLimiter(TWSEOpenAPIRate, TWSEOpenAPIBurst),
	}
}

// Fetch retrieves quotes for 2330 (台積電) and 0050 as representative samples.
func (a *FubonChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quotes, err := a.client.GetQuotes(ctx, []string{"2330", "0050"})
	if err != nil {
		return nil, fmt.Errorf("fubon fetch: %w", err)
	}
	data, err := json.Marshal(quotes)
	if err != nil {
		return nil, fmt.Errorf("fubon marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "fubon",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity to the Fubon proxy.
func (a *FubonChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if err := a.client.HealthCheck(ctx); err != nil {
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

// RateLimit returns the Fubon rate limiter.
func (a *FubonChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Fubon.
func (a *FubonChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "fubon",
		Country:    "台灣",
		Platform:   "富邦證券",
		APIFormat:  "REST JSON",
		Path:       "api.fubon.com.tw (via Python proxy)",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// TEJChannelAdapter — wraps *marketdata.TEJClient
// ---------------------------------------------------------------------------

// TEJChannelAdapter adapts a TEJClient to the DataProvider interface.
type TEJChannelAdapter struct {
	client  *marketdata.TEJClient
	limiter *rate.Limiter
}

// NewTEJChannelAdapter creates a new adapter for the TEJ channel.
func NewTEJChannelAdapter(client *marketdata.TEJClient) *TEJChannelAdapter {
	return &TEJChannelAdapter{
		client:  client,
		limiter: rate.NewLimiter(TWSEOpenAPIRate, TWSEOpenAPIBurst),
	}
}

// Fetch pings the TEJ API and fetches 2330 daily price as a representative sample.
func (a *TEJChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	now := time.Now()
	startDate := now.AddDate(0, 0, -5).Format("2006-01-02")
	endDate := now.Format("2006-01-02")
	rows, err := a.client.GetStockPriceDaily(ctx, "2330", startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("tej fetch: %w", err)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return nil, fmt.Errorf("tej marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "tej",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity to the TEJ API.
func (a *TEJChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if err := a.client.Ping(ctx); err != nil {
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

// RateLimit returns the TEJ rate limiter.
func (a *TEJChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TEJ.
func (a *TEJChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tej",
		Country:    "台灣",
		Platform:   "TEJ 台灣經濟新報",
		APIFormat:  "REST JSON",
		Path:       "api.tej.com.tw",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// GeopoliticalChannelAdapter — wraps narrative geopolitical providers
// ---------------------------------------------------------------------------

// GeopoliticalChannelAdapter adapts narrative geopolitical providers to the DataProvider interface.
type GeopoliticalChannelAdapter struct {
	workDir        string
	limiter        *rate.Limiter
	globalProvider *narrative.CompositeGeopoliticalProvider
	taiwanProvider *narrative.CompositeTaiwanGeopoliticalProvider
}

// NewGeopoliticalChannelAdapter creates a new adapter for the geopolitical channel.
func NewGeopoliticalChannelAdapter(workDir string) *GeopoliticalChannelAdapter {
	return &GeopoliticalChannelAdapter{
		workDir:        workDir,
		limiter:        rate.NewLimiter(rate.Every(time.Minute), 1),
		globalProvider: narrative.NewCompositeGeopoliticalProvider(narrative.NewRSSGeopoliticalProvider(), narrative.NewGDELTGeopoliticalProvider()),
		taiwanProvider: narrative.NewCompositeTaiwanGeopoliticalProvider(narrative.NewTaiwanRSSGeopoliticalProvider()),
	}
}

// Fetch retrieves global and Taiwan geopolitical risk scores.
func (a *GeopoliticalChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	globalStore := narrative.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical"))
	taiwanStore := narrative.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical/taiwan"))

	type geopoliticalResult struct {
		Global *narrative.GeopoliticalRiskScore `json:"global,omitempty"`
		Taiwan *narrative.GeopoliticalRiskScore `json:"taiwan,omitempty"`
	}

	result := &geopoliticalResult{}

	bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	score, err := a.globalProvider.FetchScore(bgCtx)
	cancel()
	if err != nil {
		logging.Error("apigateway", "geopolitical_fetch_failed", "err", err)
	} else {
		score.Timestamp = time.Now()
		result.Global = &score
		if err := globalStore.Save(score); err != nil {
			logging.Error("apigateway", "geopolitical_save_failed", "err", err)
		}
	}

	bgCtx2, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	twScore, err := a.taiwanProvider.FetchScore(bgCtx2)
	cancel2()
	if err != nil {
		logging.Error("apigateway", "taiwan_geopolitical_fetch_failed", "err", err)
	} else {
		twScore.Timestamp = time.Now()
		result.Taiwan = &twScore
		if err := taiwanStore.Save(twScore); err != nil {
			logging.Error("apigateway", "taiwan_geopolitical_save_failed", "err", err)
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("geopolitical marshal: %w", err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "geopolitical",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching scores.
func (a *GeopoliticalChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.Fetch(ctx)
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

// RateLimit returns the geopolitical rate limiter.
func (a *GeopoliticalChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for geopolitical.
func (a *GeopoliticalChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "geopolitical",
		Country:    "全球",
		Platform:   "RSS + GDELT",
		APIFormat:  "Composite",
		Path:       "geopolitical",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// JPYYahooChannelAdapter — wraps FrankfurterFXProvider for JPY rate data
// ---------------------------------------------------------------------------

// JPYYahooChannelAdapter adapts FrankfurterFXProvider to the DataProvider.
type JPYYahooChannelAdapter struct {
	provider *marketdata.FrankfurterFXProvider
	limiter  *rate.Limiter
}

// NewJPYYahooChannelAdapter creates a new adapter for the JPY Yahoo channel.
func NewJPYYahooChannelAdapter(provider *marketdata.FrankfurterFXProvider) *JPYYahooChannelAdapter {
	return &JPYYahooChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(10*time.Second), 1),
	}
}

// Fetch retrieves the latest USD/JPY exchange rate.
func (a *JPYYahooChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("jpy_yahoo fetch: %w", err)
	}
	data, err := json.Marshal(snap.JPY)
	if err != nil {
		return nil, fmt.Errorf("jpy_yahoo marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "jpy_yahoo",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies the Frankfurter API is reachable.
func (a *JPYYahooChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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

// RateLimit returns the JPY Yahoo rate limiter.
func (a *JPYYahooChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for JPY Yahoo.
func (a *JPYYahooChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "jpy_yahoo",
		Country:    "日本",
		Platform:   "Yahoo Finance (JPY) / Frankfurter",
		APIFormat:  "REST JSON",
		Path:       "api.frankfurter.app/latest?from=USD&to=JPY",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// TSMCRevenueChannelAdapter — wraps TSMCRevenueProvider
// ---------------------------------------------------------------------------

// TSMCRevenueChannelAdapter adapts a TSMCRevenueProvider to the DataProvider interface.
type TSMCRevenueChannelAdapter struct {
	provider *marketdata.TSMCRevenueProvider
	limiter  *rate.Limiter
}

// NewTSMCRevenueChannelAdapter creates a new adapter for the TSMC revenue channel.
func NewTSMCRevenueChannelAdapter(provider *marketdata.TSMCRevenueProvider) *TSMCRevenueChannelAdapter {
	return &TSMCRevenueChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(2*time.Minute), 1),
	}
}

// Fetch retrieves the latest TSMC monthly revenue.
func (a *TSMCRevenueChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("tsmc_revenue fetch: %w", err)
	}
	data, err := json.Marshal(snap.TSMCRevenue)
	if err != nil {
		return nil, fmt.Errorf("tsmc_revenue marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "tsmc_revenue",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TSMCRevenueChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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

// RateLimit returns the TSMC revenue rate limiter.
func (a *TSMCRevenueChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TSMC revenue.
func (a *TSMCRevenueChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tsmc_revenue",
		Country:    "台灣",
		Platform:   "TWSE 台積電月營收",
		APIFormat:  "REST JSON / FinMind TWT49U",
		Path:       "api.finmindtrade.com / www.twse.com.tw",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// TaiwanGeopoliticalChannelAdapter — wraps TaiwanRSSGeopoliticalProvider
// ---------------------------------------------------------------------------

// TaiwanGeopoliticalChannelAdapter adapts Taiwan RSS geopolitical provider.
type TaiwanGeopoliticalChannelAdapter struct {
	provider *narrative.TaiwanRSSGeopoliticalProvider
	workDir  string
	limiter  *rate.Limiter
}

// NewTaiwanGeopoliticalChannelAdapter creates a new adapter.
func NewTaiwanGeopoliticalChannelAdapter(workDir string) *TaiwanGeopoliticalChannelAdapter {
	return &TaiwanGeopoliticalChannelAdapter{
		provider: narrative.NewTaiwanRSSGeopoliticalProvider(),
		workDir:  workDir,
		limiter:  rate.NewLimiter(rate.Every(time.Minute), 1),
	}
}

// Fetch retrieves the Taiwan-specific geopolitical score.
func (a *TaiwanGeopoliticalChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	score, err := a.provider.FetchScore(bgCtx)
	if err != nil {
		return nil, fmt.Errorf("taiwan_geopolitical fetch: %w", err)
	}
	score.Timestamp = time.Now()
	store := narrative.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical/taiwan"))
	if err := store.Save(score); err != nil {
		logging.Error("apigateway", "taiwan_geopolitical_save_failed", "err", err)
	}
	data, err := json.Marshal(score)
	if err != nil {
		return nil, fmt.Errorf("taiwan_geopolitical marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "geopolitical_taiwan",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching scores.
func (a *TaiwanGeopoliticalChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.Fetch(ctx)
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

// RateLimit returns the Taiwan geopolitical rate limiter.
func (a *TaiwanGeopoliticalChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Taiwan geopolitical.
func (a *TaiwanGeopoliticalChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "geopolitical_taiwan",
		Country:    "台灣",
		Platform:   "CNA / 自由時報 / TVBS RSS",
		APIFormat:  "RSS XML",
		Path:       "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw",
		HasLimiter: true,
	}
}

// ---------------------------------------------------------------------------
// JANUSRegimeChannelAdapter — wraps janus.Engine for computed regime status
// ---------------------------------------------------------------------------

// JANUSRegimeChannelAdapter adapts the JANUS engine to the DataProvider interface.
type JANUSRegimeChannelAdapter struct {
	engine  *janus.Engine
	limiter *rate.Limiter
}

// NewJANUSRegimeChannelAdapter creates a new adapter for the JANUS regime channel.
func NewJANUSRegimeChannelAdapter(engine *janus.Engine) *JANUSRegimeChannelAdapter {
	return &JANUSRegimeChannelAdapter{
		engine:  engine,
		limiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves the current JANUS regime status (computed, not fetched).
func (a *JANUSRegimeChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	if a.engine == nil {
		return nil, fmt.Errorf("janus_regime: engine not initialized")
	}
	status := a.engine.GetStatus()
	data, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("janus_regime marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "janus_regime",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies the JANUS engine is running.
func (a *JANUSRegimeChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if a.engine == nil {
		return HealthStatus{
			Status:    "inactive",
			LastError: "JANUS engine not initialized",
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "computed",
		}, nil
	}
	status := a.engine.GetStatus()
	if status.LastUpdated.IsZero() {
		return HealthStatus{
			Status:    "warn",
			LastError: "JANUS loaded but not yet updated",
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "computed",
		}, nil
	}
	age := time.Since(status.LastUpdated)
	if age > 7*24*time.Hour {
		return HealthStatus{
			Status:    "warn",
			LastError: fmt.Sprintf("JANUS last updated %d days ago", int(age.Hours()/24)),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "computed",
		}, nil
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "computed",
	}, nil
}

// RateLimit returns the JANUS regime rate limiter.
func (a *JANUSRegimeChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for JANUS regime.
func (a *JANUSRegimeChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "janus_regime",
		Country:    "全域",
		Platform:   "JANUS Engine",
		APIFormat:  "Internal (computed)",
		Path:       "internal/janus",
		HasLimiter: false,
	}
}

// ---------------------------------------------------------------------------
// ExchangeRateChannelAdapter
// ---------------------------------------------------------------------------

type ExchangeRateChannelAdapter struct {
	provider *marketdata.ExchangeRateProvider
	limiter  *rate.Limiter
}

func NewExchangeRateChannelAdapter(p *marketdata.ExchangeRateProvider) *ExchangeRateChannelAdapter {
	return &ExchangeRateChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *ExchangeRateChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("exchange rate marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "exchange_rate", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *ExchangeRateChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "liveness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *ExchangeRateChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *ExchangeRateChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "exchange_rate", Country: "全球", Platform: "Frankfurter/ECB", APIFormat: "REST JSON", Path: "api.frankfurter.dev", HasLimiter: true}
}

// ---------------------------------------------------------------------------
// SOXIndexChannelAdapter
// ---------------------------------------------------------------------------

type SOXIndexChannelAdapter struct {
	provider *marketdata.SOXIndexProvider
	limiter  *rate.Limiter
}

func NewSOXIndexChannelAdapter(p *marketdata.SOXIndexProvider) *SOXIndexChannelAdapter {
	return &SOXIndexChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *SOXIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("sox index marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "sox_index", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *SOXIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "liveness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *SOXIndexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *SOXIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "sox_index", Country: "美國", Platform: "Yahoo Finance", APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true}
}

// ---------------------------------------------------------------------------
// SectorDataChannelAdapter
// ---------------------------------------------------------------------------

type SectorDataChannelAdapter struct {
	provider *marketdata.SectorDataProvider
	limiter  *rate.Limiter
}

func NewSectorDataChannelAdapter(p *marketdata.SectorDataProvider) *SectorDataChannelAdapter {
	return &SectorDataChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Inf, 0),
	}
}

func (a *SectorDataChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("sector data marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "sector_data", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *SectorDataChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "readiness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *SectorDataChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *SectorDataChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "sector_data", Country: "台灣", Platform: "TWSE", APIFormat: "CSV/JSON", Path: "data/state/sector_data", HasLimiter: false}
}

// ---------------------------------------------------------------------------
// DayTradingChannelAdapter — wraps *marketdata.DayTradingProvider
// ---------------------------------------------------------------------------

// DayTradingChannelAdapter adapts the TWSE day trading provider.
type DayTradingChannelAdapter struct {
	provider *marketdata.DayTradingProvider
	limiter  *rate.Limiter
}

// NewDayTradingChannelAdapter creates a new adapter.
func NewDayTradingChannelAdapter() *DayTradingChannelAdapter {
	return &DayTradingChannelAdapter{
		provider: marketdata.NewDayTradingProvider(),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

func (a *DayTradingChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	stats, err := a.provider.FetchLatest(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("day trading marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "day_trading", LatencyMs: time.Since(start).Milliseconds(),
		Timestamp: time.Now(),
	}}, nil
}

func (a *DayTradingChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return HealthStatus{Status: "ok", CheckType: "liveness", UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (a *DayTradingChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *DayTradingChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "day_trading", Country: "台灣", Platform: "TWSE", APIFormat: "REST JSON", Path: "www.twse.com.tw/exchangeReport/TWTB4U", HasLimiter: true}
}

// ---------------------------------------------------------------------------
// RegisterChannelAdapters — wires concrete clients into the Gateway registry
// ---------------------------------------------------------------------------

// RegisterChannelAdapters creates concrete market-data clients from cfg,
// wraps each in a channel adapter, and registers them in the Gateway's
// ChannelRegistry. Clients that require API keys are silently skipped
// when the key is not configured.
func RegisterChannelAdapters(g *Gateway, workDir string, cfg config.Config, janusEngine *janus.Engine) error {
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

	// --- Fubon ---
	fubonKey := cfg.FubonAPIKey
	if fubonKey == "" {
		fubonKey = config.GetSecret("ATLAS_FUBON_API_KEY")
	}
	if fubonKey != "" {
		fubonClient := marketdata.NewFubonClient(fubonKey)
		fubonAdapter := NewFubonChannelAdapter(fubonClient)
		g.registry.Register("fubon", fubonAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "fubon")
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

	// --- TWSE Capital Flow (no API key required) ---
	capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow"))
	capFlowAdapter := NewTWSECapitalFlowChannelAdapter(capFlowProvider)
	g.registry.Register("twse_capital_flow", capFlowAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_capital_flow")

	// --- TWSE Margin Balance (no API key required) ---
	marginProvider := marketdata.NewTWSEMarginBalanceProvider(filepath.Join(workDir, "data/state/margin"))
	marginAdapter := NewTWSEMarginChannelAdapter(marginProvider)
	g.registry.Register("twse_margin", marginAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "twse_margin")

	// --- Export Statistics (no API key required) ---
	exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export"))
	exportAdapter := NewExportStatisticsChannelAdapter(exportProvider)
	g.registry.Register("export_statistics", exportAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "export_statistics")

	// --- TEJ ---
	if tejKey := config.GetSecret("TEJ_API_KEY"); tejKey != "" {
		tejClient := marketdata.NewTEJClient(tejKey)
		tejAdapter := NewTEJChannelAdapter(tejClient)
		g.registry.Register("tej", tejAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "tej")
	}

	// --- Geopolitical (RSS + GDELT) ---
	geoAdapter := NewGeopoliticalChannelAdapter(workDir)
	g.registry.Register("geopolitical", geoAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "geopolitical")

	// --- JPY Yahoo (FrankfurterFXProvider for USD/JPY rate) ---
	jpyProvider := marketdata.NewFrankfurterFXProvider()
	jpyAdapter := NewJPYYahooChannelAdapter(jpyProvider)
	g.registry.Register("jpy_yahoo", jpyAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "jpy_yahoo")

	// --- TSMC Revenue (FinMind, requires API key) ---
	tsmcProvider := marketdata.NewTSMCRevenueProvider(cfg.FinMindAPIKey)
	tsmcAdapter := NewTSMCRevenueChannelAdapter(tsmcProvider)
	g.registry.Register("tsmc_revenue", tsmcAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "tsmc_revenue")

	// --- Taiwan Geopolitical (CNA / 自由時報 / TVBS RSS) ---
	taiwanGeoAdapter := NewTaiwanGeopoliticalChannelAdapter(workDir)
	g.registry.Register("geopolitical_taiwan", taiwanGeoAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "geopolitical_taiwan")

	// --- Exchange Rate (Frankfurter API) ---
	exchangeProvider := marketdata.NewExchangeRateProvider()
	exchangeAdapter := NewExchangeRateChannelAdapter(exchangeProvider)
	g.registry.Register("exchange_rate", exchangeAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "exchange_rate")

	// --- SOX Index (Philadelphia Semiconductor Index) ---
	soxProvider := marketdata.NewSOXIndexProvider()
	soxAdapter := NewSOXIndexChannelAdapter(soxProvider)
	g.registry.Register("sox_index", soxAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "sox_index")

	// --- Sector Data (TWSE sector classification) ---
	sectorProvider := marketdata.NewSectorDataProvider(filepath.Join(workDir, "data/state/sector_data"))
	sectorAdapter := NewSectorDataChannelAdapter(sectorProvider)
	g.registry.Register("sector_data", sectorAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "sector_data")

	// --- TWSE Day Trading (no API key required) ---
	dayTradingAdapter := NewDayTradingChannelAdapter()
	g.registry.Register("day_trading", dayTradingAdapter)
	logging.Info("apigateway", "adapter_registered", "channel", "day_trading")

	// --- JANUS Regime (internal computed engine, optional) ---
	if janusEngine != nil {
		janusAdapter := NewJANUSRegimeChannelAdapter(janusEngine)
		g.registry.Register("janus_regime", janusAdapter)
		logging.Info("apigateway", "adapter_registered", "channel", "janus_regime")
	}

	return nil
}
