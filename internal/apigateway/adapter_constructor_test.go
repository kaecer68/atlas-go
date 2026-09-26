package apigateway

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestNewFugleChannelAdapter(t *testing.T) {
	client := &marketdata.FugleClient{}
	workDir := t.TempDir()
	a := NewFugleChannelAdapter(client, workDir)
	if a == nil {
		t.Fatal("NewFugleChannelAdapter returned nil")
	}
	if a.client != client {
		t.Error("client not set correctly")
	}
	if a.snapshotBase != workDir {
		t.Errorf("snapshotBase = %q, want %q (the snapshot base must be injected, not CWD-relative)", a.snapshotBase, workDir)
	}
}

func TestNewFinMindChannelAdapter(t *testing.T) {
	client := &marketdata.FinMindClient{}
	workDir := t.TempDir()
	a := NewFinMindChannelAdapter(client, workDir)
	if a == nil {
		t.Fatal("NewFinMindChannelAdapter returned nil")
	}
	if a.client != client {
		t.Error("client not set correctly")
	}
	if a.snapshotBase != workDir {
		t.Errorf("snapshotBase = %q, want %q (the snapshot base must be injected, not CWD-relative)", a.snapshotBase, workDir)
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
