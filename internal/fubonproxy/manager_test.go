package fubonproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessManager_Start_NonBlocking_WhenProxyUnhealthy(t *testing.T) {
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	err := m.Start(ctx)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Start() blocked for %v; expected < 2s", elapsed)
	}
	if err != nil {
		t.Logf("Start() returned error: %v (acceptable, non-fatal per doc.go)", err)
	}

	m.Stop()
}

func TestProcessManager_Start_NonBlocking_WhenHealthReachable(t *testing.T) {
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer healthServer.Close()

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  healthServer.URL + "/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	_ = m.Start(ctx)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Start() blocked for %v; expected < 2s", elapsed)
	}

	m.Stop()
}
