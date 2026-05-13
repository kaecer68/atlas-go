package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
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
	workDir string
	limiter *rate.Limiter
}

// NewGeopoliticalChannelAdapter creates a new adapter for the geopolitical channel.
func NewGeopoliticalChannelAdapter(workDir string) *GeopoliticalChannelAdapter {
	return &GeopoliticalChannelAdapter{
		workDir: workDir,
		limiter: rate.NewLimiter(rate.Every(time.Minute), 1),
	}
}

// Fetch retrieves global and Taiwan geopolitical risk scores.
func (a *GeopoliticalChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	globalProvider := narrative.NewCompositeGeopoliticalProvider(
		narrative.NewRSSGeopoliticalProvider(),
		narrative.NewGDELTGeopoliticalProvider(),
	)
	taiwanProvider := narrative.NewCompositeTaiwanGeopoliticalProvider(
		narrative.NewTaiwanRSSGeopoliticalProvider(),
	)

	globalStore := narrative.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical"))
	taiwanStore := narrative.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical/taiwan"))

	type geopoliticalResult struct {
		Global *narrative.GeopoliticalRiskScore `json:"global,omitempty"`
		Taiwan *narrative.GeopoliticalRiskScore `json:"taiwan,omitempty"`
	}

	result := &geopoliticalResult{}

	bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	score, err := globalProvider.FetchScore(bgCtx)
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
	twScore, err := taiwanProvider.FetchScore(bgCtx2)
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

	return nil
}
