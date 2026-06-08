package apigateway

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// USMarketChannels returns the 7 US market channel IDs that share the Yahoo
// Finance v8 chart API endpoint. These channels were registered by PR #416
// as on-demand only; this task provides periodic refresh so API consumers
// hit the cache instead of making live calls.
func USMarketChannels() []string {
	return []string{
		"us_spx",
		"us_ndx",
		"us_dji",
		"us_nvda",
		"us_aapl",
		"us_msft",
		"tsm_adr",
	}
}

// NewUSMarketRefreshTask returns a BackgroundTaskFunc that batch-fetches all
// 7 US market channels via the shared yahooSharedLimiter. Per-channel errors
// are logged as warnings but do not fail the whole batch — a single channel's
// transient failure should not block the other channels from being refreshed.
//
// Circuit breaker and rate limiting are handled internally by Gateway.Fetch;
// this function does not need to check them explicitly.
func NewUSMarketRefreshTask(g *Gateway) BackgroundTaskFunc {
	channels := USMarketChannels()
	return func(ctx context.Context) error {
		for _, ch := range channels {
			_, err := g.Fetch(ctx, ch)
			if err != nil {
				logging.Warn("apigateway", "us_market_refresh_failed",
					"channel", ch,
					"err", err,
				)
			} else {
				logging.Info("apigateway", "us_market_refresh_ok",
					"channel", ch,
				)
			}
		}
		return nil
	}
}
