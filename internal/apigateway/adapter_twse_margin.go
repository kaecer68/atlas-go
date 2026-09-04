package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEMarginChannelAdapter adapts a TWSEMarginBalanceProvider to the DataProvider interface.
type TWSEMarginChannelAdapter struct {
	provider *marketdata.TWSEMarginBalanceProvider
	limiter  *rate.Limiter

	// finmind is optional. When set, it fills snap.MarginMaintenanceRatio
	// whenever the TWSE provider comes back without one (TWSE MI_MARGN does
	// not expose the aggregate ratio, so before this field the live snapshot
	// never carried it — only backfills did).
	finmind *marketdata.FinMindClient

	// daily-dedup for the FinMind ratio fill (PR-2): the whole-market series
	// is 1 API call, but the gateway fan-out runs on every cache miss, so the
	// adapter remembers the last successful fill day and skips re-fetching
	// until the TW date rolls over.
	fillMu         sync.Mutex
	lastFillDayUTC string
}

// NewTWSEMarginChannelAdapter creates a new adapter for the TWSE margin channel.
func NewTWSEMarginChannelAdapter(provider *marketdata.TWSEMarginBalanceProvider) *TWSEMarginChannelAdapter {
	return &TWSEMarginChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst),
	}
}

// SetFinMindClient wires the (shared) FinMind client used to fill
// margin_maintenance_ratio when TWSE has none. Pass nil to disable the fill.
func (a *TWSEMarginChannelAdapter) SetFinMindClient(c *marketdata.FinMindClient) {
	a.finmind = c
}

// Fetch retrieves the latest margin balance snapshot.
func (a *TWSEMarginChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// Non-trading days or holidays: TWSE returns no data for the past 7 days,
		// which is expected behavior. Return a stale result instead of an error
		// to avoid triggering the circuit breaker.
		// P1-9: typed no-data classification (previously string matching).
		if errors.Is(err, marketdata.ErrNoData) {
			return &FetchResult{Stale: true, Meta: FetchMetadata{ChannelID: "twse_margin", Timestamp: time.Now()}}, nil
		}
		return nil, fmt.Errorf("margin fetch: %w", err)
	}
	// PR-2: fill margin_maintenance_ratio from FinMind when TWSE returns none,
	// so live snapshots carry the field instead of only backfills.
	a.fillMaintenanceRatioFromFinMind(ctx, &snap)

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

// fillMaintenanceRatioFromFinMind best-effort fills snap.MarginMaintenanceRatio
// from the FinMind TaiwanTotalExchangeMarginMaintenance dataset when the TWSE
// provider returned none. Cost control: one FinMind call per successful fill
// (whole-market series, no data_id) plus a daily dedup so repeated gateway
// fan-outs on the same day do not burn quota. Failures are logged and retried
// on the next fetch (e.g. the ratio is published after TWSE evening processing,
// so same-day morning fetches legitimately come back empty).
func (a *TWSEMarginChannelAdapter) fillMaintenanceRatioFromFinMind(ctx context.Context, snap *marketdata.MacroDataSnapshot) {
	if a.finmind == nil || snap.MarginMaintenanceRatio.Symbol != "" {
		return
	}
	day := time.Now().UTC().Format("2006-01-02")
	a.fillMu.Lock()
	if a.lastFillDayUTC == day {
		a.fillMu.Unlock()
		return // already filled today — keep the 1-call/day budget
	}
	a.fillMu.Unlock()

	rowDate, ratio, err := a.finmind.GetMarginMaintenanceLatest(ctx, day)
	if err != nil {
		logging.Warn("twse_margin", "margin_maintenance_ratio_finmind_failed",
			logging.Err(err), logging.FStr("end_date", day))
		return
	}

	a.fillMu.Lock()
	a.lastFillDayUTC = day
	a.fillMu.Unlock()

	snap.MarginMaintenanceRatio = marketdata.MacroDataPoint{
		Symbol:    "TSE_MARGIN_MAINT",
		Value:     ratio,
		Timestamp: time.Now().Unix(),
	}
	logging.Info("apigateway", "margin_maintenance_ratio_filled",
		logging.FStr("source", "finmind:TaiwanTotalExchangeMarginMaintenance"),
		logging.FStr("row_date", rowDate))
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TWSEMarginChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
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
