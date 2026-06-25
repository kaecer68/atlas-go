// Package monitoring — type aliases for channel-health types relocated to
// internal/apigateway/channel_health.go in Wave 12 Phase 2 (Issue #731).
//
// The relocation is the cycle-breaking move that lets llm_annotator import
// apigateway without dragging back through monitoring → llm/capabilities →
// llm_annotator. Existing callers that still reference
// monitoring.ChannelHealthStore etc. continue to compile because every
// symbol here is a Go type/function alias pointing at the canonical
// apigateway definition. New code should depend on apigateway directly.
package monitoring

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// ChannelHealthRecord — canonical definition in apigateway.
type ChannelHealthRecord = apigateway.ChannelHealthRecord

// ChannelHealthStore — canonical definition in apigateway.
type ChannelHealthStore = apigateway.ChannelHealthStore

// ChannelAlert — canonical definition in apigateway.
type ChannelAlert = apigateway.ChannelAlert

// ChannelFetchLogEntry — canonical definition in apigateway.
type ChannelFetchLogEntry = apigateway.ChannelFetchLogEntry

// RecordOption — canonical definition in apigateway.
type RecordOption = apigateway.RecordOption

// NewChannelHealthStore — wraps apigateway.NewChannelHealthStore so call
// sites continue to work after the relocation.
func NewChannelHealthStore(dir string) *ChannelHealthStore {
	return apigateway.NewChannelHealthStore(dir)
}

// NewChannelHealthStoreWithPool — wraps apigateway.NewChannelHealthStoreWithPool.
func NewChannelHealthStoreWithPool(dir string, pool *pgxpool.Pool) *ChannelHealthStore {
	return apigateway.NewChannelHealthStoreWithPool(dir, pool)
}

// WithLatencyMs — wraps apigateway.WithLatencyMs.
func WithLatencyMs(ms int64) RecordOption {
	return apigateway.WithLatencyMs(ms)
}

// WithRateLimitRemaining — wraps apigateway.WithRateLimitRemaining.
func WithRateLimitRemaining(remaining int) RecordOption {
	return apigateway.WithRateLimitRemaining(remaining)
}

// WithLastDataAt — wraps apigateway.WithLastDataAt.
func WithLastDataAt(t time.Time) RecordOption {
	return apigateway.WithLastDataAt(t)
}

// WithRecordsFetched — wraps apigateway.WithRecordsFetched.
func WithRecordsFetched(n int) RecordOption {
	return apigateway.WithRecordsFetched(n)
}

// WithSymbolsProcessed — wraps apigateway.WithSymbolsProcessed.
func WithSymbolsProcessed(n int) RecordOption {
	return apigateway.WithSymbolsProcessed(n)
}

// RecordChannelFetch — wraps apigateway.RecordChannelFetch for legacy callers.
func RecordChannelFetch(stateDir, channelID, status, errMsg string, rateRemaining int, latencyMs int64) {
	apigateway.RecordChannelFetch(stateDir, channelID, status, errMsg, rateRemaining, latencyMs)
}

// RecordChannelFetchWithPool — wraps apigateway.RecordChannelFetchWithPool.
func RecordChannelFetchWithPool(stateDir, channelID, status, errMsg string, pool *pgxpool.Pool, opts ...RecordOption) {
	apigateway.RecordChannelFetchWithPool(stateDir, channelID, status, errMsg, pool, opts...)
}
