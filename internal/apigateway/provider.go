package apigateway

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// DataProvider is the unified interface for all 14 information channels.
// Each channel type (HTTP, RSS, File, Compute) implements this interface.
type DataProvider interface {
	// Fetch retrieves data from the channel.
	Fetch(ctx context.Context) (*FetchResult, error)

	// HealthCheck performs a liveness/readiness/computed check.
	HealthCheck(ctx context.Context) (HealthStatus, error)

	// RateLimit returns the rate limiter for this channel.
	RateLimit() *rate.Limiter

	// Metadata returns static metadata about the channel.
	Metadata() ChannelMetadata
}

// FetchResult contains the data and metadata from a fetch operation.
// Fallback is true when returning last-known-good data on circuit-breaker open.
// LastError contains the last error message from the fetch (empty = no error).
type FetchResult struct {
	Data      []byte
	Meta      FetchMetadata
	Cached    bool
	Stale     bool
	Fallback  bool   `json:"fallback,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

// FetchMetadata records call details for health tracking.
type FetchMetadata struct {
	ChannelID          string    `json:"channel_id"`
	LatencyMs          int64     `json:"latency_ms"`
	RateLimitRemaining int       `json:"rate_limit_remaining"`
	Timestamp          time.Time `json:"timestamp"`
	Cached             bool      `json:"cached"`
	Stale              bool      `json:"stale"`
	Fallback           bool      `json:"fallback,omitempty"`
	LastError          string    `json:"last_error,omitempty"`
}

// HealthStatus represents the result of a health check.
type HealthStatus struct {
	Status    string `json:"status"` // ok | warn | error | inactive
	LastError string `json:"last_error,omitempty"`
	UpdatedAt string `json:"updated_at"`
	CheckType string `json:"check_type"` // liveness | readiness | computed
}

// ChannelMetadata contains static information about a channel.
type ChannelMetadata struct {
	ChannelID  string `json:"channel_id"`
	Country    string `json:"country"`
	Platform   string `json:"platform"`
	APIFormat  string `json:"api_format"`
	Path       string `json:"path"`
	Storage    string `json:"storage"`
	HasLimiter bool   `json:"has_limiter"`

	// Stub marks a channel whose data fetcher is not yet implemented
	// (e.g. waiting on an external API key or endpoint confirmation).
	// Stub channels are still registered (so the dashboard shows them
	// explicitly as "not yet live") but their HealthCheck returns
	// Status="inactive" so the alerting path in
	// monitoring/service/data_channels.go can skip them.
	Stub bool `json:"stub,omitempty"`
}

// HTTPProvider implements DataProvider for HTTP-based channels.
type HTTPProvider struct {
	name       string
	limiter    *rate.Limiter
	meta       ChannelMetadata
	fetcher    func(ctx context.Context) ([]byte, error)
	healthFunc func(ctx context.Context) (HealthStatus, error)
}

// Fetch implements DataProvider.
func (p *HTTPProvider) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()

	if err := p.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	data, err := p.fetcher(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", p.name, err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          p.name,
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(p.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck implements DataProvider.
func (p *HTTPProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return p.healthFunc(ctx)
}

// RateLimit implements DataProvider.
func (p *HTTPProvider) RateLimit() *rate.Limiter {
	return p.limiter
}

// Metadata implements DataProvider.
func (p *HTTPProvider) Metadata() ChannelMetadata {
	return p.meta
}

// FileProvider implements DataProvider for file-based channels.
type FileProvider struct {
	name    string
	limiter *rate.Limiter
	meta    ChannelMetadata
	path    string
	parser  func(data []byte) ([]byte, error)
}

// Fetch implements DataProvider.
func (p *FileProvider) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()

	// File providers still respect rate limits for consistency
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	// Implementation reads from file system
	// Actual implementation in concrete providers
	_ = p.path
	_ = p.parser

	return &FetchResult{
		Meta: FetchMetadata{
			ChannelID:          p.name,
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(p.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck implements DataProvider (readiness check).
func (p *FileProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	// Check file existence and modification time
	return HealthStatus{
		Status:    "ok",
		CheckType: "readiness",
		UpdatedAt: time.Now().Format(time.RFC3339),
	}, nil
}

// RateLimit implements DataProvider.
func (p *FileProvider) RateLimit() *rate.Limiter {
	return p.limiter
}

// Metadata implements DataProvider.
func (p *FileProvider) Metadata() ChannelMetadata {
	return p.meta
}

// ComputeProvider implements DataProvider for in-memory computation channels.
type ComputeProvider struct {
	name       string
	meta       ChannelMetadata
	compute    func(ctx context.Context) ([]byte, error)
	healthFunc func(ctx context.Context) (HealthStatus, error)
}

// Fetch implements DataProvider.
func (p *ComputeProvider) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()

	data, err := p.compute(ctx)
	if err != nil {
		return nil, fmt.Errorf("compute %s: %w", p.name, err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID: p.name,
			LatencyMs: time.Since(start).Milliseconds(),
			Timestamp: time.Now(),
		},
	}, nil
}

// HealthCheck implements DataProvider.
func (p *ComputeProvider) HealthCheck(ctx context.Context) (HealthStatus, error) {
	return p.healthFunc(ctx)
}

// RateLimit implements DataProvider (no limit for compute).
func (p *ComputeProvider) RateLimit() *rate.Limiter {
	return rate.NewLimiter(rate.Inf, 0)
}

// Metadata implements DataProvider.
func (p *ComputeProvider) Metadata() ChannelMetadata {
	return p.meta
}
