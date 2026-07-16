package apigateway

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// USMarketChannels returns the 8 US market channel IDs that hit Yahoo Finance
// v8 chart API. These channels were registered by PR #416 as on-demand only;
// this task provides periodic refresh so API consumers hit the cache instead
// of making live calls. Channels are split across yahooIndexLimiter (3 major
// indexes), ExportStatisticsRate (sox_index), and yahooTechLimiter (4 tech
// stocks + TSM ADR) for parallelized fetching.
func USMarketChannels() []string {
	return []string{
		"us_spx",
		"us_ndx",
		"us_dji",
		"sox_index",
		"us_nvda",
		"us_aapl",
		"us_msft",
		"tsm_adr",
	}
}

// NewUSMarketRefreshTask returns a BackgroundTaskFunc that batch-fetches all
// 8 US market channels concurrently. Channels are split across three limiter
// groups — yahooIndexLimiter (3 major indexes), ExportStatisticsRate (sox_index),
// and yahooTechLimiter (4 tech stocks + TSM ADR) — so the batch parallelizes
// instead of serializing at 1 req/s. A 100 ms stagger between launches prevents
// a thundering herd against the Yahoo Finance edge. Per-channel errors are
// logged as warnings but do not fail the whole batch; a single channel's
// transient failure should not block the other channels from being refreshed.
//
// Circuit breaker and rate limiting are handled internally by Gateway.Fetch;
// this function does not need to check them explicitly.
func NewUSMarketRefreshTask(g *Gateway) BackgroundTaskFunc {
	channels := USMarketChannels()
	return func(ctx context.Context) error {
		var wg sync.WaitGroup
		for i, ch := range channels {
			wg.Add(1)
			go func(ch string, idx int) {
				defer wg.Done()
				// Stagger launches to avoid a thundering herd at the Yahoo edge.
				if idx > 0 {
					select {
					case <-time.After(100 * time.Millisecond):
					case <-ctx.Done():
						return
					}
				}
				// Give each channel its own timeout so one slow fetch does not
				// starve the rest of the batch.
				fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()
				_, err := g.Fetch(fetchCtx, ch)
				if err != nil {
					logging.Warn(
						"apigateway", "us_market_refresh_failed",
						"channel", ch,
						"err", err,
					)
				} else {
					logging.Info(
						"apigateway", "us_market_refresh_ok",
						"channel", ch,
					)
				}
			}(ch, i)
		}
		wg.Wait()
		return nil
	}
}
