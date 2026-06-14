package apigateway

import (
	"net/http"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

func TestDefaultHTTPClientConfig(t *testing.T) {
	cfg := DefaultHTTPClientConfig()
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", cfg.MaxIdleConns)
	}
	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", cfg.IdleConnTimeout)
	}
}

func TestNewHTTPClient(t *testing.T) {
	cfg := httpclient.Config{
		Timeout:         15 * time.Second,
		MaxIdleConns:    5,
		IdleConnTimeout: 60 * time.Second,
	}
	client := NewHTTPClient(cfg)
	if client == nil {
		t.Fatal("NewHTTPClient returned nil")
	}
	if client.Timeout != cfg.Timeout {
		t.Errorf("Timeout = %v, want %v", client.Timeout, cfg.Timeout)
	}
	// Verify the Transport is set up with the right config
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	if transport.MaxIdleConns != cfg.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, cfg.MaxIdleConns)
	}
	if transport.IdleConnTimeout != cfg.IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, cfg.IdleConnTimeout)
	}
}

func TestNewHTTPClientFactory(t *testing.T) {
	factory := NewHTTPClientFactory()
	if factory == nil {
		t.Fatal("NewHTTPClientFactory returned nil")
	}
	client := factory.NewClient(10 * time.Second)
	if client == nil {
		t.Fatal("Factory.NewClient returned nil")
	}
	if client.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", client.Timeout)
	}
}

func TestHTTPClientConfigTypeAlias(t *testing.T) {
	// Verify the alias types work correctly
	var _ HTTPClientConfig = httpclient.Config{}
	var _ *HTTPClientFactory = httpclient.NewFactory()
	// Ensure net/http client is usable
	client := NewHTTPClient(DefaultHTTPClientConfig())
	var _ *http.Client = client
}
