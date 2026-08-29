package service

import (
	"math"
	"sort"
)

// ChannelHealthStore is the minimal surface required from the apigateway health
// store to compute ingestion lag. It is satisfied by *apigateway.UnifiedHealthStore.
type ChannelHealthStore interface {
	ChannelIDs() []string
	ChannelLatencyMs(channelID string) int64
}

// ChannelHealthIngestionLagProvider implements IngestionLagProvider by computing
// the p99 latency across all registered apigateway channels using the provided
// health store.
type ChannelHealthIngestionLagProvider struct {
	store ChannelHealthStore
}

// NewChannelHealthIngestionLagProvider creates a production ingestion-lag provider.
func NewChannelHealthIngestionLagProvider(store ChannelHealthStore) *ChannelHealthIngestionLagProvider {
	return &ChannelHealthIngestionLagProvider{store: store}
}

// P99LatencySeconds returns the 99th percentile fetch latency across all known
// channels. Channels without a recorded latency are ignored. If no channel has
// latency data, the provider returns 0.
func (p *ChannelHealthIngestionLagProvider) P99LatencySeconds() float64 {
	if p.store == nil {
		return 0
	}

	ids := p.store.ChannelIDs()
	latencies := make([]float64, 0, len(ids))
	for _, channelID := range ids {
		ms := p.store.ChannelLatencyMs(channelID)
		if ms <= 0 {
			continue
		}
		latencies = append(latencies, float64(ms)/1000.0)
	}

	if len(latencies) == 0 {
		return 0
	}

	sort.Float64s(latencies)
	idx := max(int(math.Ceil(0.99*float64(len(latencies))))-1, 0)
	if idx >= len(latencies) {
		idx = len(latencies) - 1
	}
	return latencies[idx]
}
