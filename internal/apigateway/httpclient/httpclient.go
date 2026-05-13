// Package httpclient provides a centralized HTTP client factory for all data sources.
// This is a standalone sub-package of apigateway with zero internal dependencies
// to prevent import cycles (marketdata → apigateway → monitoring → industry → marketdata).
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// Config configures a shared HTTP client.
type Config struct {
	Timeout         time.Duration
	MaxIdleConns    int
	IdleConnTimeout time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:         30 * time.Second,
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	}
}

// New creates a configured HTTP client.
// All data source HTTP clients should use this factory instead of direct &http.Client{}.
func New(cfg Config) *http.Client {
	return &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:    cfg.MaxIdleConns,
			IdleConnTimeout: cfg.IdleConnTimeout,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
}

// Factory is the centralized factory for all HTTP clients.
// Deprecated: direct instantiation of &http.Client{} violates the Constitution.
// Use Factory.New() instead.
type Factory struct {
	baseConfig Config
}

// NewFactory creates a factory with default config.
func NewFactory() *Factory {
	return &Factory{
		baseConfig: DefaultConfig(),
	}
}

// NewClient creates a client with the factory's base configuration.
func (f *Factory) NewClient(timeout time.Duration) *http.Client {
	cfg := f.baseConfig
	cfg.Timeout = timeout
	return New(cfg)
}
