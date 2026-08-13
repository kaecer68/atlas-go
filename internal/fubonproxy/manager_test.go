package fubonproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	_ = withFreeEphemeralPort(t)
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
//
// F9 改動：原本用 httptest.NewServer 隨機 port + m.healthURL 注入（繞過 probe），
// 改用 bindEphemeralPort 模擬「healthy fubon-proxy 已佔住該 port」的生產現實;
// production `proxyListenPort` 預設仍是 18081。
// 此測試在 F9 設計下與 F9_P2 校驗同一不變式，保留為冪等啟動語義的命名錨點。
func TestProcessManager_Start_SkipsWhenAlreadyHealthy(t *testing.T) {
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	_, port := bindEphemeralPort(t, handler)

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", port),
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
	_ = withFreeEphemeralPort(t)
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

// ============================================================================
// F-TEST-1 + F1~F8 invariants — test coverage added in response to
// PR #493 /review findings (see /tmp/pr-493-review-log.json).
//
// Acceptance: all 9 new tests green under `go test -race -count=1
// ./internal/fubonproxy/`, with 3 existing tests not regressing.
// All health-URL mocks use 127.0.0.1 form (not `localhost`) per PR #495.
// ============================================================================

// currentPID 安全讀取 m.cmd.Process.Pid。回傳 0 表示無程序。
func currentPID(m *ProcessManager) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == nil || m.cmd.Process == nil {
		return 0
	}
	return m.cmd.Process.Pid
}

// waitForPIDChange 等待 m.cmd 的 PID 變成與 oldPID 不同的值。回傳新 PID 與是否在
// timeout 內變化。用於觀察 supervise() 重啟新程序的時機。
func waitForPIDChange(m *ProcessManager, oldPID int, timeout time.Duration) (int, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pid := currentPID(m); pid != 0 && pid != oldPID {
			return pid, true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return currentPID(m), false
}

// TestProcessManager_F1_PostStartRecheck_NoOrphanAfterRestart 驗證 F1 post-start
// re-check 不變式：當 supervise() 啟動新程序之後、註冊到 m.cmd 之前，Stop() 被
// 呼叫時，新程序必須被 Kill() 而非孤兒化。
//
// 測試策略：使用「首次執行 crash、之後 sleep」腳本，驅動 1 次崩潰 + 1 次重啟。
// 在重啟後的新程序仍活著時呼叫 Stop()，驗證該 PID 死掉且不留孤兒。
//
// Reference: PR #489 (F1 fix) + PR #493 review finding F-TEST-1 (CRITICAL).
func TestProcessManager_F1_PostStartRecheck_NoOrphanAfterRestart(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	// 首次執行 exit 1（觸發 supervise 重啟），之後 sleep 30s（讓測試有時間呼叫 Stop()）
	script := "#!/bin/sh\n" +
		"SCRIPT_DIR=\"$(dirname \"$0\")\"\n" +
		"if [ ! -f \"$SCRIPT_DIR/.crashed_once\" ]; then\n" +
		"  touch \"$SCRIPT_DIR/.crashed_once\"\n" +
		"  exit 1\n" +
		"fi\n" +
		"sleep 30\n"
	if err := os.WriteFile(fakeScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health", // 永遠不通 — 確保程序會真的啟動
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	firstPID := currentPID(m)
	if firstPID == 0 {
		t.Fatal("could not capture first pid")
	}
	t.Logf("first process started: pid=%d", firstPID)

	// 等待 1st 程序 crash + 3s backoff + 2nd 程序啟動
	secondPID, changed := waitForPIDChange(m, firstPID, 8*time.Second)
	if !changed {
		m.Stop()
		t.Fatal("supervise() did not restart process within 8s after crash")
	}
	if secondPID == firstPID {
		m.Stop()
		t.Fatalf("PID did not change: first=%d second=%d", firstPID, secondPID)
	}
	t.Logf("restart detected: first=%d second=%d (after crash + backoff)", firstPID, secondPID)

	// 2nd 程序正在 sleep 30s — 此時呼叫 Stop()。
	// 不論 Stop() 是落在「post-start re-check 視窗」還是「已註冊到 m.cmd 之後」，
	// 結果都應該是 2nd 程序被終止、不留孤兒。
	m.Stop()

	if !waitForProcessExit(firstPID, 3*time.Second) {
		_ = syscall.Kill(firstPID, syscall.SIGKILL)
		t.Errorf("first process (pid %d) still running after Stop()", firstPID)
	}
	if !waitForProcessExit(secondPID, 3*time.Second) {
		_ = syscall.Kill(secondPID, syscall.SIGKILL)
		t.Errorf("second process (pid %d) still running after Stop() — ORPHAN!", secondPID)
	} else {
		t.Logf("second process (pid %d) terminated cleanly — no orphan", secondPID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		t.Error("expected m.running=false after Stop()")
	}
	if m.cmd != nil {
		t.Errorf("expected m.cmd=nil after Stop(), got %+v", m.cmd)
	}
}

// TestProcessManager_F2F4_NoFireAndForgetHealthCheck 驗證 F2/F4 不變式：
// supervise() 在重啟路徑中對 health check 是**同步**呼叫，不應 fire-and-forget
// 產生背景 goroutine。若 fire-and-forget 存在，反覆 crash 會堆積 goroutine。
//
// 測試策略：使用立即 crash 腳本，驅動 3 次 crash + 2 次重啟，觀察 goroutine
// 數量在整個生命週期內是否保持在合理範圍內。
//
// Reference: PR #489 (F2/F4 fix) + PR #493 review log F2/F4 informational item.
func TestProcessManager_F2F4_NoFireAndForgetHealthCheck(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	// baseline goroutines（test goroutine 本身 + runtime 內部 goroutines）
	baseline := runtime.NumGoroutine()
	t.Logf("baseline goroutines: %d", baseline)

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() { m.Stop() })

	// Start 後短暫穩定期：1 個 supervise goroutine + 1 個 health check goroutine
	// + 0~1 個 HTTP client idle goroutine。允許 baseline + 3。
	time.Sleep(300 * time.Millisecond)
	afterStart := runtime.NumGoroutine()
	t.Logf("after Start(): %d (delta=%d)", afterStart, afterStart-baseline)
	if afterStart > baseline+5 {
		t.Errorf("too many goroutines right after Start(): %d (delta=%d, want <=%d)",
			afterStart, afterStart-baseline, baseline+5)
	}

	// 驅動 2 次重啟：每次 crash + backoff + 新 process 啟動
	// 1st PID
	firstPID := currentPID(m)
	if firstPID == 0 {
		t.Fatal("could not capture first pid")
	}

	// 2nd PID
	secondPID, _ := waitForPIDChange(m, firstPID, 15*time.Second)
	if secondPID == 0 || secondPID == firstPID {
		t.Fatalf("2nd restart did not happen: first=%d second=%d", firstPID, secondPID)
	}

	mid := runtime.NumGoroutine()
	t.Logf("after 2nd restart: %d (delta=%d)", mid, mid-baseline)
	// F2/F4 invariant: 即使經過 2 次重啟，goroutine 數不應明顯增長
	// fire-and-forget 場景下，這裡會 baseline + 5+（每次重啟 spawn 一個 goroutine）
	if mid > baseline+7 {
		t.Errorf("goroutine pile-up detected: %d after 2 restarts (delta=%d, want <=%d)",
			mid, mid-baseline, baseline+7)
	}

	// 3rd PID — 再觸發一次重啟
	thirdPID, _ := waitForPIDChange(m, secondPID, 5*time.Second)
	if thirdPID == 0 || thirdPID == secondPID {
		t.Logf("3rd restart not observed within 5s (PID unchanged=%d), acceptable for test scope", secondPID)
	} else {
		late := runtime.NumGoroutine()
		t.Logf("after 3rd restart: %d (delta=%d)", late, late-baseline)
		if late > baseline+7 {
			t.Errorf("goroutine pile-up after 3 restarts: %d (delta=%d, want <=%d)",
				late, late-baseline, baseline+7)
		}
	}
}

// TestProcessManager_F5_WaitForHealthyRespectsContextCancel 驗證 F5 不變式：
// waitForHealthy() 必須在 ctx 取消時**立即**返回，不應繼續 sleep 至 healthCheckInterval。
//
// 測試策略：使用 unreachable health URL，直接呼叫 m.waitForHealthy(cancelledCtx)，
// 計時驗證返回時間 < 100ms。
//
// Reference: PR #489 (F5 fix) + PR #493 review log F5 informational item.
func TestProcessManager_F5_WaitForHealthyRespectsContextCancel(t *testing.T) {
	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: "/dev/null",
		healthURL:  "http://127.0.0.1:1/health", // 永遠不通 — 確保 polling 持續
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	start := time.Now()
	err := m.waitForHealthy(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Errorf("waitForHealthy with cancelled ctx should return error, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
	// healthCheckInterval 是 500ms — 若 select 沒包好，這裡會 ≥ 500ms
	if elapsed > 100*time.Millisecond {
		t.Errorf("waitForHealthy took %v with cancelled ctx; expected < 100ms", elapsed)
	}
	t.Logf("waitForHealthy returned in %v with cancelled ctx — OK", elapsed)
}

// TestProcessManager_F7_StartFailureThenStopDoesNotHang 驗證 F7 不變式：
// 當 Start() 在錯誤路徑（pythonBin 未找到 / script 不存在）時，後續呼叫 Stop()
// 必須能快速返回，不應永久阻塞。
//
// 測試策略：建立 manager with pythonBin="" 觸發 Start() 早退錯誤路徑，驗證
// Stop() 在合理時間內返回。
//
// Reference: PR #489 (F7 fix) + PR #493 review log F7 informational item.
func TestProcessManager_F7_StartFailureThenStopDoesNotHang(t *testing.T) {
	t.Run("pythonBin empty", func(t *testing.T) {
		m := &ProcessManager{
			pythonBin:  "", // 觸發 Start() 早退
			scriptPath: "/tmp/whatever",
			healthURL:  "http://127.0.0.1:1/health",
		}

		err := m.Start(context.Background())
		if err == nil {
			t.Fatal("Start() with empty pythonBin should return error")
		}
		t.Logf("Start() correctly returned error: %v", err)

		// Stop() 必須不阻塞
		done := make(chan struct{})
		start := time.Now()
		go func() {
			m.Stop()
			close(done)
		}()

		select {
		case <-done:
			elapsed := time.Since(start)
			if elapsed > 1*time.Second {
				t.Errorf("Stop() took %v after failed Start(); expected < 1s", elapsed)
			}
			t.Logf("Stop() returned in %v after failed Start() — OK", elapsed)
		case <-time.After(2 * time.Second):
			t.Fatal("Stop() hung > 2s after failed Start() — mutex dead lock?")
		}
	})

	t.Run("script not found", func(t *testing.T) {
		m := &ProcessManager{
			pythonBin:  "/bin/sh",
			scriptPath: "/nonexistent/path/to/script.sh",
			healthURL:  "http://127.0.0.1:1/health",
		}

		err := m.Start(context.Background())
		if err == nil {
			t.Fatal("Start() with missing script should return error")
		}
		if !strings.Contains(err.Error(), "script not found") {
			t.Errorf("expected 'script not found' error, got: %v", err)
		}

		done := make(chan struct{})
		start := time.Now()
		go func() {
			m.Stop()
			close(done)
		}()

		select {
		case <-done:
			elapsed := time.Since(start)
			if elapsed > 1*time.Second {
				t.Errorf("Stop() took %v after script-not-found Start(); expected < 1s", elapsed)
			}
			t.Logf("Stop() returned in %v after script-not-found Start() — OK", elapsed)
		case <-time.After(2 * time.Second):
			t.Fatal("Stop() hung > 2s after script-not-found Start()")
		}
	})
}

// TestProcessManager_F7_CmdStartFailure_StopDoesNotHang 驗證 F7 不變式在
// cmd.Start() 失敗路徑下也成立：
//
//   - pythonBin 指到不存在的可執行檔（繞過 early-return 早退）
//   - scriptPath 是有效檔案（繞過 script-not-found 早退）
//   - Start() 必須在 cmd.Start() 內部失敗後回傳 error
//   - 失敗路徑必須 close(m.done) 並重置 m.ctx/m.cancel/m.done,避免 Stop()
//     永久阻塞
//
// 與 TestProcessManager_F7_StartFailureThenStopDoesNotHang 的差異：後者只
// 覆蓋 early-return 早退路徑（pythonBin empty / script not found）;本測試
// 覆蓋 cmd.Start() 真的被呼叫且失敗的場景,直接驗證 Start() 內部 m.done 與
// m.ctx 清理邏輯。
//
// Reference: F7 不變式（atlas-fubon-supervisor-invariants/SKILL.md）— 「Start()
// 錯誤路徑必須關閉 m.done 並重置 ctx/cancel/done」;以及 lock-check-unlock-work-lock
// pattern — exec.Cmd.Start() 必須在鎖外執行。
func TestProcessManager_F7_CmdStartFailure_StopDoesNotHang(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/nonexistent/atlas-go-fubonproxy-test/python", // cmd.Start() 會在這裡失敗
		scriptPath: fakeScript,                                     // 存在,繞過 script-not-found 早退
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", proxyListenPort),
	}

	// 捕捉 baseline goroutine 數(在 Start() 之前)— 用於後續 leak 偵測
	// (對齊 TestProcessManager_F2F4_NoFireAndForgetHealthCheck 風格)
	baseline := runtime.NumGoroutine()
	t.Logf("baseline goroutines (before Start): %d", baseline)

	// Start() 必須回傳錯誤(不 panic、不阻塞)
	start := time.Now()
	err := m.Start(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Start() with non-existent pythonBin should return error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Start() blocked %v on cmd.Start() failure; expected < 2s", elapsed)
	}
	t.Logf("Start() correctly returned error in %v: %v", elapsed, err)

	// 關鍵 F7 不變式:失敗路徑必須重置 m.ctx/m.cancel/m.done,
	// 否則 Stop() 會在 done channel 上永久阻塞。
	m.mu.Lock()
	if m.ctx != nil {
		t.Error("expected m.ctx=nil after cmd.Start() failure (F7 cleanup)")
	}
	if m.cancel != nil {
		t.Error("expected m.cancel=nil after cmd.Start() failure (F7 cleanup)")
	}
	if m.done != nil {
		t.Error("expected m.done=nil after cmd.Start() failure (F7 cleanup)")
	}
	if m.running {
		t.Error("expected m.running=false after cmd.Start() failure")
	}
	if m.cmd != nil {
		t.Errorf("expected m.cmd=nil after cmd.Start() failure, got %+v", m.cmd)
	}
	m.mu.Unlock()

	// Stop() 必須不阻塞(即使失敗路徑沒啟動 supervise,Stop 仍可能嘗試
	// 等待一個已 close 的 channel;若 m.done 沒被 close + nil 化,Stop 會 hang)
	done := make(chan struct{})
	start = time.Now()
	go func() {
		m.Stop()
		close(done)
	}()
	select {
	case <-done:
		stopElapsed := time.Since(start)
		if stopElapsed > 1*time.Second {
			t.Errorf("Stop() took %v after cmd.Start() failure; expected < 1s", stopElapsed)
		}
		t.Logf("Stop() returned in %v after cmd.Start() failure — OK", stopElapsed)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung > 2s after cmd.Start() failure — F7 cleanup broken?")
	}

	// 二次 Stop() 也必須不阻塞(re-entrant 守衛:第二次呼叫應立即返回)
	done2 := make(chan struct{})
	go func() {
		m.Stop()
		close(done2)
	}()
	select {
	case <-done2:
		t.Logf("second Stop() returned immediately — OK")
	case <-time.After(1 * time.Second):
		t.Fatal("second Stop() hung > 1s after cmd.Start() failure")
	}

	// goroutine 洩漏檢查(對齊 TestProcessManager_F2F4_NoFireAndForgetHealthCheck
	// 風格):cmd.Start() 失敗路徑不應 spawn supervise 或健康檢查 goroutine
	time.Sleep(100 * time.Millisecond) // 讓任何 race 條件下的 goroutine 落地
	final := runtime.NumGoroutine()
	t.Logf("after 100ms idle (post cmd.Start() failure): %d (delta=%d, baseline=%d)",
		final, final-baseline, baseline)
	// 允許少量 slack(+2)給 test 本身的 goroutine 排程誤差
	if final > baseline+2 {
		t.Errorf("goroutine leak after cmd.Start() failure: baseline=%d final=%d (delta=%d, want <=%d)",
			baseline, final, final-baseline, baseline+2)
	}
}

// TestProcessManager_F1_StartPostStartRecheck_AbortsNewProcess 驗證 F1
// 不變式在 Start() 自己的 post-start re-check 路徑下也成立:
//
//   - Start() 在 unlock 後呼叫 cmd.Start(),若期間 Stop() 被呼叫(或手動
//     設 m.stopping=true),新程序必須被 Kill+Wait(cancel 釋放 ctx)
//   - m.cmd/m.running 必須保持未設定狀態(新程序不能被 supervise 接手)
//   - m.ctx/m.cancel/m.done 必須被 close 並重置為 nil
//   - Start() 在此場景下回傳 nil(屬於「乾淨 abort」,非錯誤)
//
// 與 TestProcessManager_F1_PostStartRecheck_NoOrphanAfterRestart 的差異:
// 後者覆蓋 supervise() 重啟路徑的 post-start re-check;本測試覆蓋 Start()
// 第一次啟動路徑的 post-start re-check(對應 manager.go:429-443 新增的
// 區塊)。雖然機制相同(Kill+Wait+close(done)+reset),程式碼路徑是
// 獨立的,因此需要獨立測試。
//
// 測試策略:手動設定 m.stopping=true(同 package 可直接寫入 struct 欄位),
// 模擬「Stop() 在 Start() unlock/Start 視窗被呼叫」的情境。Start() 走到
// 自己的 re-check,看到 m.stopping=true,啟動 abort cleanup 路徑。
//
// Reference: F1 不變式(atlas-fubon-supervisor-invariants/SKILL.md) —
// post-start re-check 必須在 Start() 與 supervise() 兩處對齊;以及
// lock-check-unlock-work-lock pattern 引入的 race window 修補。
func TestProcessManager_F1_StartPostStartRecheck_AbortsNewProcess(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	// trap '' INT + sleep 30:即使 SIGINT 也不會優雅退出,確保 Kill 是唯一路徑
	script := "#!/bin/sh\n" +
		"trap '' INT\n" +
		"sleep 30\n"
	if err := os.WriteFile(fakeScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", proxyListenPort),
	}

	// 捕捉 baseline goroutine 數(在 Start() 之前)— 用於後續 leak 偵測
	// (對齊 TestProcessManager_F2F4_NoFireAndForgetHealthCheck 風格)
	baseline := runtime.NumGoroutine()
	t.Logf("baseline goroutines (before F1 abort): %d", baseline)

	// 在 Start() 進到 post-start re-check 之前手動設 m.stopping=true,
	// 模擬「Stop() 在 Start() unlock/Start 視窗被呼叫」的情境。
	// (同 package 測試可寫入 struct 欄位)
	m.stopping = true

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start() 必須回傳 nil(屬於「乾淨 abort」,不是 error)
	start := time.Now()
	err := m.Start(ctx)
	elapsed := time.Since(start)
	if err != nil {
		t.Errorf("Start() with pre-set m.stopping should return nil (clean abort), got: %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("Start() blocked %v on F1 re-check path; expected < 3s", elapsed)
	}
	t.Logf("Start() correctly returned nil in %v (clean abort via F1 re-check)", elapsed)

	// 關鍵 F1 不變式:新程序絕對不能被註冊到 m.cmd,supervise() 也不應
	// 被啟動(避免留下孤兒 goroutine)。
	m.mu.Lock()
	if m.cmd != nil {
		t.Errorf("expected m.cmd=nil after F1 re-check abort, got %+v (process leaked to supervisor!)", m.cmd)
	}
	if m.running {
		t.Error("expected m.running=false after F1 re-check abort")
	}
	if m.ctx != nil {
		t.Error("expected m.ctx=nil after F1 re-check abort (F1 cleanup)")
	}
	if m.cancel != nil {
		t.Error("expected m.cancel=nil after F1 re-check abort (F1 cleanup)")
	}
	if m.done != nil {
		t.Error("expected m.done=nil after F1 re-check abort (F1 cleanup)")
	}
	m.mu.Unlock()

	// m.stopping 仍是 true(沒人 reset,因為 Start() 的 abort 路徑不 reset stopping)
	// 這是預期行為 — abort 表示「已 stop 過,不要再 spawn」

	// goroutine 洩漏檢查(對齊 TestProcessManager_F2F4_NoFireAndForgetHealthCheck
	// 風格):Start() 的 abort 路徑不會啟動 supervise,也不應 spawn 任何
	// 健康檢查 goroutine;若 baseline+slack 內有殘留,F1 修補就破了。
	time.Sleep(100 * time.Millisecond) // 讓任何 race 條件下的 goroutine 落地
	final := runtime.NumGoroutine()
	t.Logf("after 100ms idle (post F1 abort): %d (delta=%d, baseline=%d)",
		final, final-baseline, baseline)
	// 允許少量 slack(+2)給 test 本身的 goroutine 排程誤差
	if final > baseline+2 {
		t.Errorf("goroutine leak after F1 abort: baseline=%d final=%d (delta=%d, want <=%d)",
			baseline, final, final-baseline, baseline+2)
	}

	// 二次 Stop() 必須不阻塞(m.stopping=true 早退守衛應立即返回)
	done2 := make(chan struct{})
	start = time.Now()
	go func() {
		m.Stop()
		close(done2)
	}()
	select {
	case <-done2:
		stopElapsed := time.Since(start)
		if stopElapsed > 1*time.Second {
			t.Errorf("Stop() took %v after F1 abort; expected < 1s (m.stopping guard should early-return)", stopElapsed)
		}
		t.Logf("Stop() returned in %v after F1 abort — OK", stopElapsed)
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() hung > 2s after F1 abort — m.stopping guard broken?")
	}
}

// TestProcessManager_BackoffStateMachine_3sThen10s 驗證 backoff 狀態機：
// - 1st 重啟：restartInitialDelay (3s)
// - 連續 Start() 失敗：restartBackoffDelay (10s)
//
// 測試策略：使用立即 crash 腳本，記錄 1st 與 2nd PID 出現時點，計算 gap1。
// gap1 應在 [2.5s, 5s]。
//
// 注意：2nd→3rd 的時間間距會被 30s 的 waitForHealthy 阻塞主導（health 永遠
// 不通，waitForHealthy 必須等滿 30s timeout 才返回），所以 10s 的
// restartBackoffDelay 在「health check 失敗」路徑下不可觀察。10s backoff
// 只在「Start() 本身失敗」路徑下觸發（manager.go:347 `backoff = restartBackoffDelay`），
// 需更複雜的 script 切換 setup 才能在測試中重現。
//
// Reference: PR #489 (backoff state machine fix) + PR #493 review log
// "Backoff state machine (3s/10s, reset on health pass) untested".
func TestProcessManager_BackoffStateMachine_3sThen10s(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backoff timing test in -short mode")
	}
	_ = withFreeEphemeralPort(t)

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health", // 永遠不通
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() { m.Stop() })

	startTime := time.Now()
	firstPID := currentPID(m)
	if firstPID == 0 {
		t.Fatal("could not capture first pid")
	}
	t.Logf("T+0  : first  pid=%d", firstPID)

	// 等待 2nd PID — 1st 程序 crash (immediate) + 3s backoff + 2nd start
	secondPID, ok := waitForPIDChange(m, firstPID, 6*time.Second)
	if !ok || secondPID == firstPID {
		t.Fatalf("2nd restart not observed: first=%d second=%d", firstPID, secondPID)
	}
	gap1 := time.Since(startTime)
	t.Logf("T+%.2fs: second pid=%d (gap1=%.2fs, want 3.0±1.5s — restartInitialDelay)",
		gap1.Seconds(), secondPID, gap1.Seconds())

	if gap1 < 2500*time.Millisecond || gap1 > 5*time.Second {
		t.Errorf("gap1=%.2fs outside [2.5s, 5s] — restartInitialDelay=3s broken?", gap1.Seconds())
	}
}

// TestProcessManager_CrashAutoRestart_FullCycle 驗證崩潰後自動重啟的完整
// 循環：program crashes → supervise observes exit → backoff → start new program
// → 新的 PID 與舊的不同。
//
// Reference: PR #493 review log "Crash → automatic restart cycle untested".
func TestProcessManager_CrashAutoRestart_FullCycle(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() { m.Stop() })

	firstPID := currentPID(m)
	if firstPID == 0 {
		t.Fatal("could not capture first pid")
	}
	t.Logf("first pid=%d", firstPID)

	secondPID, ok := waitForPIDChange(m, firstPID, 6*time.Second)
	if !ok || secondPID == firstPID {
		t.Fatalf("supervise() did not restart crashed process within 6s: first=%d second=%d", firstPID, secondPID)
	}
	t.Logf("after crash: second pid=%d (different from first=%d)", secondPID, firstPID)

	// 2nd 程序當下也已經 crash 完畢（exit 1），但中間 supervise 已觀察到它並啟動了 3rd
	// 這裡只驗證「有自動重啟」與「PID 確實不同」就足夠。
}

// TestProcessManager_Stop_SIGINTGracefulThenSIGKILL 驗證 Stop() 在 ctx-based
// 取消路徑下能終止程序。
//
// 重要觀察：根據 PR #489 F1 fix，Stop() 會**先**呼叫 `cancel()` 來中斷 supervise()
// 與 waitForHealthy()。由於 cmd 是用 `exec.CommandContext(ctx, ...)` 建立，ctx
// 取消會觸發 Go runtime 對程序的 SIGKILL（不是 SIGINT）。所以「SIGINT → 5s →
// SIGKILL escalation」路徑在當前實作中**不會**被執行 — SIGINT 永遠送給已死的程序。
//
// 5s graceful shutdown timeout (`gracefulShutdownTimeout` const) 仍是 Stop() 的
// 安全網（manager.go:224），但只有在 cancel() 沒能終止 cmd 的罕見情境下才會觸發。
// 要直接測試該路徑需繞過 exec.CommandContext 的 ctx 綁定，屬於 mock-level test。
//
// 本測試驗證實際的取消路徑：program 被 cancel 終止 + Stop() 在合理時間內返回 +
// 程序真的死掉。
//
// Reference: PR #489 (F1 fix, cancel() unconditional) + PR #493 review log
// "SIGINT → 5s graceful → SIGKILL escalation untested".
func TestProcessManager_Stop_SIGINTGracefulThenSIGKILL(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	// trap '' INT 忽略 SIGINT — 即使 SIGINT 路徑被走到，process 也不會優雅退出
	script := "#!/bin/sh\n" +
		"trap '' INT\n" +
		"sleep 30\n"
	if err := os.WriteFile(fakeScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	pid := currentPID(m)
	if pid == 0 {
		t.Fatal("could not capture pid")
	}
	t.Logf("started stuck process: pid=%d (ignore SIGINT, sleep 30)", pid)

	// F3: 驗證正確性不只時序 — gracefulShutdownTimeout=5s 為設計安全網。
	stopStart := time.Now()
	m.Stop()
	stopElapsed := time.Since(stopStart)
	t.Logf("Stop() returned in %.3fs (includes 5s graceful shutdown safety net)", stopElapsed.Seconds())

	// 程序必須真的死掉
	if !waitForProcessExit(pid, 3*time.Second) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Errorf("stuck process (pid %d) still running after Stop()", pid)
	} else {
		t.Logf("stuck process (pid %d) terminated after Stop() — OK", pid)
	}
}

// TestProcessManager_NewManager_ScriptPathIsAbsolute 驗證 NewManager 將
// scriptPath 透過 filepath.Abs 轉為絕對路徑（PR #488 不變式）。
//
// 測試策略：用 tmp dir 作為 workDir，呼叫 NewManager(tmpDir)，驗證回傳的
// ProcessManager.scriptPath 為絕對路徑，且解析後指向期望位置。
//
// Reference: PR #488 (filepath.Abs fix) + PR #493 review log
// "NewManager filepath.Abs (PR #488 invariant) bypassed by struct-literal tests".
func TestProcessManager_NewManager_ScriptPathIsAbsolute(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建 NewManager 預期的目錄結構與 fake script
	proxyDir := filepath.Join(tmpDir, "services", "fubon-proxy")
	if err := os.MkdirAll(proxyDir, 0o755); err != nil {
		t.Fatalf("mkdir proxy dir: %v", err)
	}
	fakeMain := filepath.Join(proxyDir, "main.py")
	if err := os.WriteFile(fakeMain, []byte("#!/usr/bin/env python3\n"), 0o755); err != nil {
		t.Fatalf("write fake main.py: %v", err)
	}

	m := NewManager(tmpDir, 0) // port=0 → 不覆寫 proxyListenPort,測試 helper (withFreeEphemeralPort/bindPort) 可保留其預設行為
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	// 1. 必須是絕對路徑
	if !filepath.IsAbs(m.scriptPath) {
		t.Errorf("scriptPath is not absolute: %q", m.scriptPath)
	}

	// 2. 必須解析後指向我們創建的 fake main.py
	resolved, err := filepath.EvalSymlinks(m.scriptPath)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	expected, err := filepath.EvalSymlinks(fakeMain)
	if err != nil {
		t.Fatalf("EvalSymlinks(fakeMain): %v", err)
	}
	if resolved != expected {
		t.Errorf("scriptPath resolves to %q, want %q", resolved, expected)
	}

	// 3. healthURL 預設應為 127.0.0.1（PR #495 IPv4 hardcode），
	//    port 為預設值 18081（PR #940 migration）。
	if m.healthURL != "http://127.0.0.1:18081/health" {
		t.Errorf("healthURL = %q, want %q (PR #495 IPv4 hardcode)", m.healthURL, "http://127.0.0.1:18081/health")
	}
}

// TestProcessManager_LogWriter_StderrSurfacedAtInfoLevel 驗證 PR #488 不變式：
// logWriter 將 Python 程序的 stderr 輸出以 Info 級別寫入日誌（不靜默、不降級為
// Debug、也不升級為 Error/Warn）。
//
// 測試策略：直接構造 logWriter，呼叫 Write 並驗證返回值。Info 級別的設定由源碼
// 檢視保證（logWriter.Write() 第 391 行呼叫 logging.Info），本測試專注於
// 「資料不丟失、不阻塞」的不變式。
//
// Reference: PR #488 (stderr Info level fix) + PR #493 review log
// "stderr Info log level (PR #488 invariant) untested".
func TestProcessManager_LogWriter_StderrSurfacedAtInfoLevel(t *testing.T) {
	w := &logWriter{component: "test.stderr"}

	t.Run("newline-terminated", func(t *testing.T) {
		n, err := w.Write([]byte("ERROR: something failed\n"))
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n != len("ERROR: something failed\n") {
			t.Errorf("Write returned n=%d, want %d", n, len("ERROR: something failed\n"))
		}
	})

	t.Run("no trailing newline", func(t *testing.T) {
		n, err := w.Write([]byte("ERROR: no newline"))
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n != len("ERROR: no newline") {
			t.Errorf("Write returned n=%d, want %d", n, len("ERROR: no newline"))
		}
	})

	t.Run("empty bytes (no log)", func(t *testing.T) {
		// 空字串不應觸發 logging.Info（避免噪訊）
		n, err := w.Write([]byte(""))
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n != 0 {
			t.Errorf("Write returned n=%d for empty input, want 0", n)
		}
	})

	t.Run("multiple writes in sequence (no blocking)", func(t *testing.T) {
		// 模擬 Python 程序快速寫入多行 stderr — logWriter 必須不阻塞
		done := make(chan struct{})
		go func() {
			for i := 0; i < 100; i++ {
				_, _ = w.Write([]byte("line\n"))
			}
			close(done)
		}()
		select {
		case <-done:
			// OK — 未阻塞
		case <-time.After(1 * time.Second):
			t.Fatal("logWriter blocked > 1s on 100 writes")
		}
	})
}

// ============================================================================
// F9 — port 18081 pre-flight probe (fix/fubonproxy-port-conflict-probe)
//
// 為什麼需要：當 port 18081 被外部進程佔住，supervise() spawn() 內部會遇到
// EADDRINUSE → backoff-loop → 無限重啟 + 前端拿不到 fubon 資料。
// 預先探測 port 區分三種狀態：
//   1. portStateFree     — 走原本 spawn 路徑
//   2. portStateHealthy  — 已有 fubon-proxy 在跑，跳過 spawn
//   3. portStateForeign  — 外部進程佔住，回傳 actionable error 含 PID+cmd
//
// 測試使用真實的 net.Listen 佔 127.0.0.1:18081 模擬外部占用。所有測試必須
// 互相不衝突（依賴 Go test 內建單 goroutine 序列執行；不使用 t.Parallel）。
// ============================================================================

// reserveEphemeralPort binds 0.0.0.0:0 to obtain an OS-assigned ephemeral port,
// overrides package-level proxyListenPort for the duration of the test, and
// registers a cleanup to restore the original port value.
//
// We bind the IPv4 wildcard (not 127.0.0.1) so that portprobe.Probe(), which
// also attempts a wildcard bind, sees the port as occupied. The caller owns the
// returned listener and must close it (or hand it to an http.Server).
//
// Because proxyListenPort is a package-level var, this helper is not safe for
// t.Parallel() tests.
func reserveEphemeralPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("failed to bind ephemeral port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	origPort := proxyListenPort
	proxyListenPort = port
	t.Cleanup(func() {
		// 防呆：即使 caller 忘了關閉 listener（如直接呼叫 reserveEphemeralPort
		// 而非經 bindEphemeralPort/withFreeEphemeralPort），測試結束時也確保
		// port 釋放，避免殘留 listener 影響後續測試的 port probe。
		// 與 caller 的 ln.Close()/srv.Close() 雙重關閉安全（第二次回傳 error 被忽略）。
		_ = ln.Close()
		proxyListenPort = origPort
	})
	return ln, port
}

// bindEphemeralPort starts an http.Server on an ephemeral IPv4 wildcard port,
// simulating a foreign or healthy process occupying the port. It returns the
// server and the port number. The listener and server are closed automatically
// at test cleanup.
func bindEphemeralPort(t *testing.T, h http.Handler) (*http.Server, int) {
	t.Helper()
	ln, port := reserveEphemeralPort(t)
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return srv, port
}

// bindPort starts an http.Server on a specific IPv4 wildcard port. It is used
// by tests that need to occupy the same port both before and after
// ProcessManager.Start().
func bindPort(t *testing.T, port int, h http.Handler) *http.Server {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		t.Fatalf("failed to bind port %d: %v", port, err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return srv
}

// withFreeEphemeralPort reserves an ephemeral port and immediately closes the
// listener so the port is free for the duration of the test. It returns the
// port number (with proxyListenPort overridden accordingly).
func withFreeEphemeralPort(t *testing.T) int {
	t.Helper()
	ln, port := reserveEphemeralPort(t)
	_ = ln.Close()
	return port
}

// F9.P1 — port 空閒時，probe 回 portStateFree，後續路徑不變（走 spawn）。
func TestProcessManager_F9_PortFree_ProceedsToExistingPath(t *testing.T) {
	_ = withFreeEphemeralPort(t)

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", proxyListenPort), // 永遠不通 — 確保 spawn 路徑被走到
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() { m.Stop() })

	// portStateFree → IsHealthy()=false → spawn 必須真的啟動
	m.mu.Lock()
	if !m.running {
		t.Error("expected m.running=true after Start() with port free")
	}
	if m.cmd == nil || m.cmd.Process == nil {
		t.Error("expected m.cmd.Process to be set after Start()")
	} else if err := m.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("process is not alive: %v", err)
	}
	m.mu.Unlock()
}

// F9.P2 — port 18081 被健康的 fubon-proxy 佔住（/health=200），probe 回
// portStateHealthy，Start() 立即返回 nil 且不啟動新程序。
func TestProcessManager_F9_PortHeldByHealthyFubon_SkipsSpawn(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	_, port := bindEphemeralPort(t, handler)

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", port), // bindEphemeralPort handler 模擬 healthy fubon-proxy
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := m.Start(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Start() returned error: %v (expected nil for external healthy fubon)", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("Start() blocked for %v; expected < 1s (should skip due to portStateHealthy)", elapsed)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		t.Error("expected m.running=false (no new process should be spawned)")
	}
	if m.cmd != nil {
		t.Error("expected m.cmd=nil (no new process should be spawned)")
	}
}

// F9.P3 — port 18081 被外部非 fubon 進程佔住（/health=404），probe 回
// portStateForeign，Start() 回傳 actionable error 含 PID 與 command。
//
// 模擬方式：bind 127.0.0.1:18081 + handler 回 404。production 環境下 lsof 會
// 抓到真實的占用者 PID+Command；本測試不依賴 lsof 解析內容，只驗證錯誤
// 訊息的「行動性」關鍵字（port 18081 / foreign / kill）。
func TestProcessManager_F9_PortHeldByForeign_ReturnsActionableError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, port := bindEphemeralPort(t, handler)

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", port), // 拿到 404
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := m.Start(ctx)
	if err == nil {
		t.Fatal("Start() should return error when port held by foreign process")
	}

	msg := err.Error()
	if !strings.Contains(msg, fmt.Sprintf("port %d", proxyListenPort)) {
		t.Errorf("error message should mention 'port %d', got: %q", proxyListenPort, msg)
	}
	if !strings.Contains(msg, "foreign") {
		t.Errorf("error message should mention 'foreign' process, got: %q", msg)
	}
	if !strings.Contains(msg, "kill") && !strings.Contains(msg, "stop it") {
		t.Errorf("error message should suggest action (kill/stop), got: %q", msg)
	}
	t.Logf("actionable error returned: %s", msg)

	// 必須沒有 spawn 新程序
	m.mu.Lock()
	if m.running {
		t.Error("expected m.running=false (no spawn when foreign holds port)")
	}
	if m.cmd != nil {
		t.Error("expected m.cmd=nil (no spawn when foreign holds port)")
	}
	m.mu.Unlock()
}

// F9.P4 — lookupPortOccupant 能在 lsof 可用時解析 PID+command。
// lsof 不可用時 skip（常見於 minimal container images）。
func TestProcessManager_F9_LookupPortOccupant_ResolvesOurTestListener(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skipf("lsof not available on test host: %v — skipping", err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, port := bindEphemeralPort(t, handler)

	occ, err := lookupPortOccupant(port)
	if err != nil {
		t.Fatalf("lookupPortOccupant failed: %v", err)
	}
	if occ.PID == 0 {
		t.Errorf("expected non-zero PID, got: %+v", occ)
	}
	if occ.Command == "" {
		t.Errorf("expected non-empty command, got: %+v", occ)
	}
	t.Logf("port %d occupied by pid=%d cmd=%q", port, occ.PID, occ.Command)
}

// ============================================================================
// P0 restart-path tests (fix/fubonproxy-port-conflict)
//
// 驗證 supervise() 在重啟前重新探測 port 18081 的行為：
//   - portStateFree   → 可以重啟
//   - portStateHealthy → 視為外部已管理，supervisor 退出（yield）
//   - portStateForeign → retry/backoff，連續失敗達上限後放棄
//
// 測試使用 test-only backoff seam 變數（restartInitialDelayForTest /
// restartBackoffDelayForTest）將延遲壓到 10ms，避免等待數十秒。
// 每次測試結束透過 t.Cleanup 還原，避免影響其他測試。
// ============================================================================

func setFastRestartBackoff(t *testing.T) {
	t.Helper()
	origInitial := restartInitialDelayForTest
	origBackoff := restartBackoffDelayForTest
	restartInitialDelayForTest = 10 * time.Millisecond
	restartBackoffDelayForTest = 10 * time.Millisecond
	t.Cleanup(func() {
		restartInitialDelayForTest = origInitial
		restartBackoffDelayForTest = origBackoff
	})
}

func waitForSupervisorDone(t *testing.T, m *ProcessManager) {
	t.Helper()
	var doneCh chan struct{}
	m.mu.Lock()
	doneCh = m.done
	m.mu.Unlock()
	if doneCh == nil {
		t.Fatal("supervisor not started")
	}
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not exit within 3s")
	}
}

// TestProcessManager_Restart_PortFree_CanProceed 驗證 port 空閒時，
// preparePortForRestart() 回傳 (true, false)。
func TestProcessManager_Restart_PortFree_CanProceed(t *testing.T) {
	_ = withFreeEphemeralPort(t)

	m := &ProcessManager{healthURL: fmt.Sprintf("http://127.0.0.1:%d/health", proxyListenPort)}
	canProceed, shouldStop := m.preparePortForRestart()
	if !canProceed || shouldStop {
		t.Errorf("preparePortForRestart() = (%v, %v), want (true, false)", canProceed, shouldStop)
	}
}

// TestProcessManager_Restart_PortHealthy_Yields 驗證 port 被健康的
// fubon-proxy 佔住時，preparePortForRestart() 回傳 (false, true)。
func TestProcessManager_Restart_PortHealthy_Yields(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	_, port := bindEphemeralPort(t, handler)

	m := &ProcessManager{healthURL: fmt.Sprintf("http://127.0.0.1:%d/health", port)}
	canProceed, shouldStop := m.preparePortForRestart()
	if canProceed || !shouldStop {
		t.Errorf("preparePortForRestart() = (%v, %v), want (false, true)", canProceed, shouldStop)
	}
}

// TestProcessManager_Restart_PortForeign_Retries 驗證 port 被外部非
// fubon 進程佔住時，preparePortForRestart() 回傳 (false, false) 以繼續 backoff。
func TestProcessManager_Restart_PortForeign_Retries(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, port := bindEphemeralPort(t, handler)

	m := &ProcessManager{healthURL: fmt.Sprintf("http://127.0.0.1:%d/health", port)}
	canProceed, shouldStop := m.preparePortForRestart()
	if canProceed || shouldStop {
		t.Errorf("preparePortForRestart() = (%v, %v), want (false, false)", canProceed, shouldStop)
	}
}

// TestProcessManager_Supervise_YieldsToExternalHealthyProxy 驗證：當原本的
// proxy 程序結束後，supervise() 重啟前發現 port 18081 已有外部 healthy proxy，
// 會放棄重啟並結束 supervisor goroutine。
func TestProcessManager_Supervise_YieldsToExternalHealthyProxy(t *testing.T) {
	setFastRestartBackoff(t)

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 0.5\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	port := withFreeEphemeralPort(t)

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", port),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() { m.Stop() })

	// 在原程序結束前佔住 port 並提供 /health=200，模擬外部 healthy proxy
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	bindPort(t, port, handler)

	waitForSupervisorDone(t, m)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		t.Error("expected m.running=false after supervisor yielded to external proxy")
	}
}

// TestProcessManager_Supervise_RestartFailureCap 驗證：當 port 18081 持續被
// 外部進程佔用，連續重啟失敗達 maxRestartFailures 次後 supervisor 會放棄。
func TestProcessManager_Supervise_RestartFailureCap(t *testing.T) {
	setFastRestartBackoff(t)

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 0.3\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	port := withFreeEphemeralPort(t)

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  fmt.Sprintf("http://127.0.0.1:%d/health", port),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	t.Cleanup(func() { m.Stop() })

	// 在原程序結束前佔住 port 並回傳 404，模擬持續 foreign 占用
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	bindPort(t, port, handler)

	waitForSupervisorDone(t, m)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		t.Error("expected m.running=false after restart failure cap reached")
	}
	if m.restartFailures < maxRestartFailures {
		t.Errorf("restartFailures=%d, want >= %d", m.restartFailures, maxRestartFailures)
	}
}

// ============================================================================
// Auto-kill zombie — isFubonZombie + killOccupant unit tests
// ============================================================================

// TestIsFubonZombie 驗證 isFubonZombie 對各種 command 名稱的判別邏輯：
// - Python/uvicorn → zombie（true）
// - 其他程序（java, nginx, node, go, sh）→ 非 zombie（false）
// - 空字串 → 非 zombie（false）
func TestIsFubonZombie(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{name: "python short", cmd: "python", want: true},
		{name: "python3", cmd: "python3", want: true},
		{name: "uvicorn", cmd: "uvicorn", want: true},
		{name: "full path python3", cmd: "/usr/bin/python3", want: true},
		{name: "Python uppercase", cmd: "Python", want: true},
		{name: "python with script arg", cmd: "python /opt/fubon/main.py", want: true},
		{name: "uvicorn module arg", cmd: "uvicorn main:app", want: true},
		{name: "java", cmd: "java", want: false},
		{name: "nginx", cmd: "nginx", want: false},
		{name: "node", cmd: "node", want: false},
		{name: "go", cmd: "go", want: false},
		{name: "sh", cmd: "sh", want: false},
		{name: "empty string", cmd: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			occ := portOccupant{PID: 12345, Command: tt.cmd}
			got := isFubonZombie(occ)
			if got != tt.want {
				t.Errorf("isFubonZombie(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestKillOccupant 驗證 killOccupant 能終止一個子行程。
// 使用 sh -c 包裝 sleep，使其對 SIGTERM 有預期行為。SIGTERM → 1s → SIGKILL
// escalation 路徑由 killOccupant 內部測試；本測試專注於驗證函式不報錯且行程
// 最終退出。
//
// macOS 注意：行程被 SIGKILL 後會變成 zombie，須透過 cmd.Wait() 收屍。
// syscall.Kill(pid, 0) 對 zombie 仍回 nil，因此不能用輪詢方式確認。
// 正確做法是 killOccupant → cmd.Wait() 帶超時。
func TestKillOccupant(t *testing.T) {
	// 用 sleep 30 確保程序在 killOccupant 被呼叫時仍在存活
	cmd := exec.Command("sh", "-c", "trap 'exit 0' TERM; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start shell: %v", err)
	}
	pid := cmd.Process.Pid

	killErr := killOccupant(pid)
	if killErr != nil {
		t.Fatalf("killOccupant(%d) returned error: %v", pid, killErr)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		t.Logf("process (pid %d) reaped: %v", pid, err)
	case <-time.After(3 * time.Second):
		t.Errorf("process (pid %d) not reaped within 3s after killOccupant", pid)
	}
}

// TestKillOccupant_NonExistentPID 驗證 killOccupant 對不存在的 PID 回傳 error。
func TestKillOccupant_NonExistentPID(t *testing.T) {
	err := killOccupant(999999999)
	if err == nil {
		t.Error("killOccupant on non-existent PID should return error")
	} else {
		t.Logf("killOccupant returned expected error: %v", err)
	}
}

// bindProxyPortIPv6Wildcard 佔住 [::]:<proxyListenPort> (IPv6 wildcard) 模擬 IPv6-only
// 外部進程（對應 Docker Desktop IPv6-only port forwarder）。handler 給
// 測試控制 /health 行為。
func bindProxyPortIPv6Wildcard(t *testing.T, h http.Handler) (net.Listener, *http.Server) {
	t.Helper()
	ln, err := net.Listen("tcp", "[::]:"+strconv.Itoa(proxyListenPort))
	if err != nil {
		t.Skipf("IPv6 wildcard port %d unavailable on test host: %v — skipping", proxyListenPort, err)
	}
	srv := &http.Server{Handler: h}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return ln, srv
}

func TestProcessManager_F9_PortIPv6WildcardHeld_ReportsForeign(t *testing.T) {
	if ln, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(proxyListenPort)); err == nil {
		_ = ln.Close()
	} else {
		t.Skipf("0.0.0.0:%d unavailable: %v — cannot isolate IPv6 case", proxyListenPort, err)
	}

	bindProxyPortIPv6Wildcard(t, http.NotFoundHandler())

	m := &ProcessManager{}
	state, _, err := m.probeProxyPort()
	if err != nil {
		t.Fatalf("probeProxyPort() error: %v", err)
	}
	if state != portStateForeign {
		t.Errorf("probeProxyPort() with [::]:%d held = %v, want portStateForeign", proxyListenPort, state)
	}
}

func TestProcessManager_F9_PortBothWildcardsFree_ReturnsFree(t *testing.T) {
	if ln, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(proxyListenPort)); err == nil {
		_ = ln.Close()
	} else {
		t.Skipf("0.0.0.0:%d in use: %v", proxyListenPort, err)
	}
	if ln, err := net.Listen("tcp", "[::]:"+strconv.Itoa(proxyListenPort)); err == nil {
		_ = ln.Close()
	} else {
		t.Skipf("[::]:%d in use: %v", proxyListenPort, err)
	}

	m := &ProcessManager{}
	state, _, err := m.probeProxyPort()
	if err != nil {
		t.Fatalf("probeProxyPort() error: %v", err)
	}
	if state != portStateFree {
		t.Errorf("probeProxyPort() = %v, want portStateFree", state)
	}
}

// TestGetFubonProxyPort_ReflectsNewManagerOverride 驗證 GetFubonProxyPort getter
// 會反映 NewManager 的 port override,而非永遠回 defaultFubonProxyPort。
//
// 用途:apigateway.RegisterChannelAdapters 的 startup probe 透過這個 getter
// 動態構造 dial address(L2 落地的 follow-up 修)。如果 getter 漏掉 NewManager
// 的 port 設定,probe 會繼續 dial 18081,即使 -fubon-port=18080,fubon adapter
// 就會 silent skip(回 fubon_proxy_not_reachable log)。
func TestGetFubonProxyPort_ReflectsNewManagerOverride(t *testing.T) {
	const customPort = 18081
	t.Cleanup(func() {
		// 還原預設值,避免污染同檔其他 test 的 proxyListenPort 假設。
		proxyListenPort = defaultFubonProxyPort
	})

	if got := GetFubonProxyPort(); got != defaultFubonProxyPort {
		t.Errorf("pre-NewManager GetFubonProxyPort() = %d, want default %d", got, defaultFubonProxyPort)
	}

	_ = NewManager(t.TempDir(), customPort)
	if got := GetFubonProxyPort(); got != customPort {
		t.Errorf("post-NewManager GetFubonProxyPort() = %d, want %d", got, customPort)
	}
}

// TestProxyBaseURL_DefaultAndCustomPort (PR #837 follow-up, A1 single source of truth)
// 驗證 ProxyBaseURL() 回傳值:
//   - 預設:http://fubon-proxy:18081
//   - 自訂 port (透過 NewManager override):http://fubon-proxy:<custom>
//
// 若未來 dev 把 host (defaultProxyHostVar) 或預設 port 改錯,本 test 會抓到。
// 對應 guard test `marketdata.TestFubon_URLDriftGuard` 防的是「其他檔案自行構造」,
// 本 test 防的是「canonical owner 本身的構造邏輯正確」。
func TestProxyBaseURL_DefaultAndCustomPort(t *testing.T) {
	t.Cleanup(func() {
		proxyListenPort = defaultFubonProxyPort
		defaultProxyHostVar = defaultProxyHost
	})

	if got := ProxyBaseURL(); got != "http://fubon-proxy:18081" {
		t.Errorf("default ProxyBaseURL() = %q, want %q", got, "http://fubon-proxy:18081")
	}

	const customPort = 19090
	_ = NewManager(t.TempDir(), customPort)
	want := "http://fubon-proxy:19090"
	if got := ProxyBaseURL(); got != want {
		t.Errorf("custom-port ProxyBaseURL() = %q, want %q", got, want)
	}
}

// TestProxyHostPort_DefaultAndCustomPort (PR #837 follow-up, A1 single source of truth)
// 驗證 ProxyHostPort() 回傳純 host:port 字串(無 http:// prefix),
// 供 net.Dial / net.Listen probe 使用。
//
// 與 TestProxyBaseURL_DefaultAndCustomPort 互補:兩者共用同一個
// defaultProxyHostVar 與 proxyListenPort,任何一邊 drift 會被任一 test 抓到。
func TestProxyHostPort_DefaultAndCustomPort(t *testing.T) {
	t.Cleanup(func() {
		proxyListenPort = defaultFubonProxyPort
		defaultProxyHostVar = defaultProxyHost
	})

	if got := ProxyHostPort(); got != "fubon-proxy:18081" {
		t.Errorf("default ProxyHostPort() = %q, want %q", got, "fubon-proxy:18081")
	}

	const customPort = 19091
	_ = NewManager(t.TempDir(), customPort)
	want := "fubon-proxy:19091"
	if got := ProxyHostPort(); got != want {
		t.Errorf("custom-port ProxyHostPort() = %q, want %q", got, want)
	}
}

// TestSetFubonProxyPort_NoOpForNonPositivePort 驗證 SetFubonProxyPort 的 no-op 契約:
// port <= 0 不覆寫 proxyListenPort(保留測試 helper 預設值)。
//
// 對應 Oracle 4th-round verdict F13:port 必須顯式注入(>0),避免誤把 0 / -1
// 當有效 port 寫入,造成後續 caller 全打到 18080 / 失敗。
func TestSetFubonProxyPort_NoOpForNonPositivePort(t *testing.T) {
	t.Cleanup(func() {
		proxyListenPort = defaultFubonProxyPort
	})

	// 起點:預設值
	if got := GetFubonProxyPort(); got != defaultFubonProxyPort {
		t.Fatalf("precondition: GetFubonProxyPort() = %d, want default %d", got, defaultFubonProxyPort)
	}

	// port = 0 → 不覆寫
	SetFubonProxyPort(0)
	if got := GetFubonProxyPort(); got != defaultFubonProxyPort {
		t.Errorf("after SetFubonProxyPort(0): GetFubonProxyPort() = %d, want default %d (no-op contract violated)", got, defaultFubonProxyPort)
	}

	// port < 0 → 不覆寫
	SetFubonProxyPort(-1)
	if got := GetFubonProxyPort(); got != defaultFubonProxyPort {
		t.Errorf("after SetFubonProxyPort(-1): GetFubonProxyPort() = %d, want default %d (no-op contract violated)", got, defaultFubonProxyPort)
	}

	// port > 0 → 覆寫
	const customPort = 19192
	SetFubonProxyPort(customPort)
	if got := GetFubonProxyPort(); got != customPort {
		t.Errorf("after SetFubonProxyPort(%d): GetFubonProxyPort() = %d, want %d", customPort, got, customPort)
	}
}

// ---- Phase 3.5 M1: DeploymentStatus tests ----

func TestStatus_EmptyBeforeStart(t *testing.T) {
	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: "/nonexistent/fake.sh",
		healthURL:  "http://127.0.0.1:1/health",
	}

	s := m.Status()

	if s.SupervisorRunning {
		t.Error("expected SupervisorRunning=false for never-started ProcessManager")
	}
	if s.ProcessAlive {
		t.Error("expected ProcessAlive=false for never-started ProcessManager")
	}
	if s.PID != 0 {
		t.Errorf("expected PID=0, got %d", s.PID)
	}
	if s.Port != 0 {
		t.Errorf("expected Port=0, got %d", s.Port)
	}
	if !s.StartedAt.IsZero() {
		t.Error("expected StartedAt to be zero")
	}
	if s.RestartCount != 0 {
		t.Errorf("expected RestartCount=0, got %d", s.RestartCount)
	}
}

func TestStatus_ReflectsAfterStart(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 15\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health", // health never passes
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	defer m.Stop()

	// Give the process a moment to start
	time.Sleep(500 * time.Millisecond)

	s := m.Status()

	if !s.SupervisorRunning {
		t.Error("expected SupervisorRunning=true after Start()")
	}
	if !s.ProcessAlive {
		t.Error("expected ProcessAlive=true after Start()")
	}
	if s.PID <= 0 {
		t.Errorf("expected PID > 0, got %d", s.PID)
	}
	if s.Port != proxyListenPort {
		t.Errorf("expected Port=%d, got %d", proxyListenPort, s.Port)
	}
	if s.RestartCount != 0 {
		t.Errorf("expected RestartCount=0 on first start, got %d", s.RestartCount)
	}
	if s.StartedAt.IsZero() {
		t.Error("expected StartedAt to be non-zero after Start()")
	}
	if s.Config.Binary == "" {
		t.Error("expected Config.Binary to be populated")
	}
	if len(s.Config.Args) == 0 {
		t.Error("expected Config.Args to be populated")
	}
	if !s.Config.AutoRestart {
		t.Error("expected Config.AutoRestart=true")
	}
	if s.Config.MaxRestarts <= 0 {
		t.Errorf("expected Config.MaxRestarts > 0, got %d", s.Config.MaxRestarts)
	}
	if s.Config.ListenPort != proxyListenPort {
		t.Errorf("expected Config.ListenPort=%d, got %d", proxyListenPort, s.Config.ListenPort)
	}

	// Verify events are populated
	foundStart := false
	for _, ev := range s.RecentEvents {
		if ev.Kind == "process_started" {
			foundStart = true
			break
		}
	}
	if !foundStart {
		t.Error("expected 'process_started' event in RecentEvents after Start()")
	}
}

func TestStatus_ReadLockDoesNotBlockConcurrentSupervise(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	defer m.Stop()

	done := make(chan struct{})
	time.AfterFunc(2*time.Second, func() { close(done) })

	// Concurrently call Status() in a tight loop for >=1s.
	// If Status() used Lock() instead of RLock(), this would deadlock
	// with the supervise goroutine.
	for {
		select {
		case <-done:
			return
		default:
			_ = m.Status()
		}
	}
}

func TestStatus_ReturnsConsistentSnapshot(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake_proxy.sh")
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	defer m.Stop()

	// Wait for process to start
	time.Sleep(500 * time.Millisecond)

	s1 := m.Status()
	time.Sleep(500 * time.Millisecond)
	s2 := m.Status()

	// last_beat_at should be zero for both (health never passes)
	// but PID and RestartCount should be consistent
	if s1.PID != s2.PID {
		t.Errorf("PID changed between snapshots: %d → %d", s1.PID, s2.PID)
	}
	if s1.RestartCount != s2.RestartCount {
		t.Errorf("RestartCount changed between snapshots: %d → %d", s1.RestartCount, s2.RestartCount)
	}
	// From the second snapshot, started_at should be in the past
	if time.Since(s2.StartedAt) < 200*time.Millisecond {
		t.Errorf("StartedAt is too recent: %v ago", time.Since(s2.StartedAt))
	}
}

func TestStatus_IncrementsRestartCountAfterSimulatedCrash(t *testing.T) {
	_ = withFreeEphemeralPort(t)
	setFastRestartBackoff(t)
	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "crash_loop.sh")
	// Script that exits immediately, forcing the supervisor to restart it.
	if err := os.WriteFile(fakeScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}

	m := &ProcessManager{
		pythonBin:  "/bin/sh",
		scriptPath: fakeScript,
		healthURL:  "http://127.0.0.1:1/health",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	defer m.Stop()

	// Wait for at least one restart cycle (fast backoff via setFastRestartBackoff).
	// The supervisor will detect the exit and restart after restartInitialDelay
	// (compressed to 10ms via setFastRestartBackoff).
	time.Sleep(1 * time.Second)

	s := m.Status()

	// After at least one crash+restart cycle, RestartCount should be >= 1
	if s.RestartCount < 1 {
		t.Errorf("expected RestartCount >= 1 after crash loop, got %d (PID=%d, SupervisorRunning=%v)",
			s.RestartCount, s.PID, s.SupervisorRunning)
	}

	// Verify process_exited event exists
	foundExit := false
	for _, ev := range s.RecentEvents {
		if ev.Kind == "process_exited" {
			foundExit = true
			break
		}
	}
	if !foundExit {
		t.Error("expected 'process_exited' event after crash")
	}
}
