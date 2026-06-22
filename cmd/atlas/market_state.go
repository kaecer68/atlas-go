package main

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// buildBaseState constructs the initial swarm.MarketState used by the
// simulation's first cycle. It queries provider.GetQuotes for the given
// symbols with a 5-second timeout; on failure (or per-symbol miss) it
// falls back to a deterministic placeholder (price=100, volume=5M) so the
// swarm can proceed even with partial data.
func buildBaseState(provider marketdata.Provider, symbols []string) swarm.MarketState {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	state := swarm.MarketState{
		Timestamp: time.Now(),
		Prices:    make(map[string]float64),
		Volumes:   make(map[string]float64),
	}

	quotes, err := provider.GetQuotes(ctx, time.Now(), symbols)
	if err != nil {
		logging.Warn("buildBaseState", "get_quotes_failed",
			logging.Err(err),
			"symbols", len(symbols))
		for _, sym := range symbols {
			state.Prices[sym] = 100.0
			state.Volumes[sym] = 5_000_000.0
		}
		return state
	}

	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	for _, sym := range symbols {
		if q, ok := quoteMap[sym]; ok {
			state.Prices[sym] = q.Last
			state.Volumes[sym] = float64(q.Volume)
		} else {
			logging.Warn("buildBaseState", "symbol_not_in_quotes",
				"symbol", sym)
			state.Prices[sym] = 100.0
			state.Volumes[sym] = 5_000_000.0
		}
	}
	return state
}
