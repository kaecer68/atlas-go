package apigateway

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestNewFugleChannelAdapter(t *testing.T) {
	client := &marketdata.FugleClient{}
	a := NewFugleChannelAdapter(client)
	if a == nil {
		t.Fatal("NewFugleChannelAdapter returned nil")
	}
	if a.client != client {
		t.Error("client not set correctly")
	}
}

func TestNewFinMindChannelAdapter(t *testing.T) {
	client := &marketdata.FinMindClient{}
	a := NewFinMindChannelAdapter(client)
	if a == nil {
		t.Fatal("NewFinMindChannelAdapter returned nil")
	}
	if a.client != client {
		t.Error("client not set correctly")
	}
}

func TestNewGeopoliticalChannelAdapter(t *testing.T) {
	tmpDir := t.TempDir()
	a := NewGeopoliticalChannelAdapter(tmpDir)
	if a == nil {
		t.Fatal("NewGeopoliticalChannelAdapter returned nil")
	}
	if a.workDir != tmpDir {
		t.Errorf("workDir = %q, want %q", a.workDir, tmpDir)
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil after constructor")
	}
}

func TestNewTaiwanGeopoliticalChannelAdapter(t *testing.T) {
	tmpDir := t.TempDir()
	a := NewTaiwanGeopoliticalChannelAdapter(tmpDir)
	if a == nil {
		t.Fatal("NewTaiwanGeopoliticalChannelAdapter returned nil")
	}
	if a.workDir != tmpDir {
		t.Errorf("workDir = %q, want %q", a.workDir, tmpDir)
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil after constructor")
	}
}
