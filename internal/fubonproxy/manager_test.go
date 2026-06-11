package fubonproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// waitForProcessExit polls the process via signal 0 until it exits or the
// timeout elapses. It does NOT call cmd.Wait() — supervise() owns that call
// (Go's exec.Cmd only allows a single Wait call per process).
func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// signal 0 does not deliver a signal — it only checks if the process exists
		// and is owned by us. Safe to call repeatedly.
		if p, err := os.FindProcess(pid); err == nil {
			if err := p.Signal(syscall.Signal(0)); err != nil {
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func TestProcessManager_Start_NonBlocking_WhenProxyUnhealthy(t *testing.T) {
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	// long sleep so the process is still alive when we assert
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health", // 永遠不通 — 確保 IsHealthy()=false，程序會真的啟動
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Start() 必須不阻塞（circuit breaker 核心保證）
	start := time.Now()
	err := m.Start(ctx)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Start() blocked for %v; expected < 2s", elapsed)
	}
	if err != nil {
		t.Errorf("Start() returned error: %v", err)
	}

	// 2. 程序必須真的在執行（F3: 不只驗計時，要驗證 m.running / m.cmd）
	m.mu.Lock()
	if !m.running {
		t.Error("expected m.running=true after Start()")
	}
	if m.cmd == nil {
		t.Error("expected m.cmd to be set after Start()")
	} else if m.cmd.Process == nil {
		t.Error("expected m.cmd.Process to be non-nil")
	} else {
		if err := m.cmd.Process.Signal(syscall.Signal(0)); err != nil {
			t.Errorf("process is not alive: %v", err)
		} else {
			t.Logf("process alive with pid %d", m.cmd.Process.Pid)
		}
	}
	pid := 0
	if m.cmd != nil && m.cmd.Process != nil {
		pid = m.cmd.Process.Pid
	}
	stoppingBefore := m.stopping
	m.mu.Unlock()

	if stoppingBefore {
		t.Error("m.stopping should be false before Stop()")
	}
	if pid == 0 {
		t.Fatal("could not capture pid for exit verification")
	}

	// 3. Stop() 必須真的終止程序（F3: 驗證 Stop() 確實 kill 程序）
	m.Stop()

	if !waitForProcessExit(pid, 6*time.Second) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("process (pid %d) still running 6s after Stop()", pid)
	} else {
		t.Logf("process (pid %d) exited after Stop()", pid)
	}

	// 4. Stop() 後狀態必須被清理
	m.mu.Lock()
	if m.running {
		t.Error("expected m.running=false after Stop()")
	}
	if m.cmd != nil {
		t.Error("expected m.cmd=nil after Stop()")
	}
	if m.stopping {
		t.Error("expected m.stopping=false after Stop() (reset)")
	}
	m.mu.Unlock()
}

// TestProcessManager_Start_SkipsWhenAlreadyHealthy 驗證：
// 當 health 端點已可達（IsHealthy()=true）時，Start() 應立即返回且不啟動新程序。
// 這是「冪等啟動」的關鍵不變式 — 重複呼叫 Start() 不應重複啟動程序。
func TestProcessManager_Start_SkipsWhenAlreadyHealthy(t *testing.T) {
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	// 模擬「已有 proxy 在運行」：health 端點回 200
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

	// Start() 必須不阻塞且不啟動程序（IsHealthy()=true → 早退）
	start := time.Now()
	err := m.Start(ctx)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("Start() blocked for %v; expected < 1s (should skip due to IsHealthy)", elapsed)
	}
	if err != nil {
		t.Errorf("Start() returned error: %v (expected nil for already-healthy case)", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		t.Error("expected m.running=false (process should not be started when already healthy)")
	}
	if m.cmd != nil {
		t.Error("expected m.cmd=nil (process should not be started when already healthy)")
	}
}

// TestProcessManager_Stop_DoesNotOrphanDuringRestart 驗證 F1：
// 當程序崩潰後 supervise 進入重啟路徑時呼叫 Stop()，不應留下孤兒行程。
func TestProcessManager_Stop_DoesNotOrphanDuringRestart(t *testing.T) {
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health", // 永遠不通 — 確保程序真的啟動
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// 取得第一次啟動的 pid
	m.mu.Lock()
	firstPID := 0
	if m.cmd != nil && m.cmd.Process != nil {
		firstPID = m.cmd.Process.Pid
	}
	m.mu.Unlock()

	if firstPID == 0 {
		t.Fatal("could not capture first pid")
	}

	// 等待短暫時間，讓 supervise 進入 waitForHealthy（阻塞中）
	time.Sleep(200 * time.Millisecond)

	// 在 supervise 阻塞時呼叫 Stop() — 觸發 F1 race 視窗
	m.Stop()

	// 等所有 supervise 狀態穩定
	time.Sleep(200 * time.Millisecond)

	// 驗證：第一個程序已死、沒有新程序被啟動
	if !waitForProcessExit(firstPID, 3*time.Second) {
		_ = syscall.Kill(firstPID, syscall.SIGKILL)
		t.Errorf("first process (pid %d) still running after Stop()", firstPID)
	} else {
		t.Logf("first process (pid %d) exited after Stop() during restart window", firstPID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		t.Error("expected m.running=false after Stop() during restart window")
	}
	if m.cmd != nil {
		t.Errorf("expected m.cmd=nil after Stop(), got %+v", m.cmd)
	}
}
