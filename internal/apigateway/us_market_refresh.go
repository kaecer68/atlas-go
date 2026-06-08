package apigateway

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// USMarketChannels returns the 7 US market channel IDs that hit Yahoo Finance
// v8 chart API. These channels were registered by PR #416 as on-demand only;
// this task provides periodic refresh so API consumers hit the cache instead
// of making live calls. Channels are split across yahooIndexLimiter (3
// indexes) and yahooTechLimiter (3 tech stocks + TSM ADR) so the 7-channel
// batch no longer serializes at 1 req/s.
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
// 7 US market channels. The 3 index channels share yahooIndexLimiter and the
// 4 tech/ADR channels share yahooTechLimiter, so the batch parallelizes across
// two limiter groups instead of serializing at 1 req/s. Per-channel errors
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
