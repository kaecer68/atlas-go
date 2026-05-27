package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

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

func TestNew(t *testing.T) {
	cfg := Config{
		Timeout:         5 * time.Second,
		MaxIdleConns:    20,
		IdleConnTimeout: 60 * time.Second,
	}

	client := New(cfg)

	if client.Timeout != cfg.Timeout {
		t.Errorf("client.Timeout = %v, want %v", client.Timeout, cfg.Timeout)
	}
	if client.Transport == nil {
		t.Fatal("client.Transport is nil")
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", client.Transport)
	}
	if transport.MaxIdleConns != cfg.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, cfg.MaxIdleConns)
	}
	if transport.IdleConnTimeout != cfg.IdleConnTimeout {
		t.Errorf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, cfg.IdleConnTimeout)
	}
}

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	if f == nil {
		t.Fatal("NewFactory returned nil")
	}
	if f.baseConfig.Timeout != 30*time.Second {
		t.Errorf("baseConfig.Timeout = %v, want 30s", f.baseConfig.Timeout)
	}
}

func TestFactory_NewClient(t *testing.T) {
	f := NewFactory()
	customTimeout := 10 * time.Second

	client := f.NewClient(customTimeout)

	if client.Timeout != customTimeout {
		t.Errorf("Timeout = %v, want %v", client.Timeout, customTimeout)
	}
	if transport := client.Transport.(*http.Transport); transport.MaxIdleConns != f.baseConfig.MaxIdleConns {
		t.Errorf("MaxIdleConns = %d, want %d (inherited from base)", transport.MaxIdleConns, f.baseConfig.MaxIdleConns)
	}
}

func TestFactory_NewClient_DoesNotMutateBaseConfig(t *testing.T) {
	f := NewFactory()
	baseTimeout := f.baseConfig.Timeout

	f.NewClient(5 * time.Second)

	if f.baseConfig.Timeout != baseTimeout {
		t.Errorf("baseConfig.Timeout changed from %v to %v", baseTimeout, f.baseConfig.Timeout)
	}
}
