// Package fubonproxy manages the lifecycle of the Python fubon-proxy service.
//
// fubonproxy 啟動、監控、重啟 Python FastAPI 微服務，作為 atlas 啟動生命週期的一部分。
// 當 atlas 以 API/server 模式啟動時，ProcessManager 會自動偵測 fubon-proxy 是否已在運行，
// 若未運行則啟動並非同步等待健康檢查通過（circuit breaker pattern：Start() 立即返回，
// 健康檢查在背景 goroutine 中進行）。supervise goroutine 持續監控程序狀態並在崩潰時自動重啟。
//
// 使用方式：
//
//	mgr := fubonproxy.NewManager(cfg.WorkDir)
//	if err := mgr.Start(ctx); err != nil {
//	    // 非致命：記錄警告後繼續
//	    logging.Warn("fubonproxy", "start_failed", logging.Err(err))
//	}
//	defer mgr.Stop()
package fubonproxy

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// healthEndpoint 是 fubon-proxy 健康檢查的 HTTP 端點。
	healthEndpoint = "http://localhost:8081/health"

	// healthCheckTimeout 是單次健康檢查 HTTP 請求的超時時間。
	healthCheckTimeout = 3 * time.Second

	// healthCheckInterval 是啟動時輪詢健康檢查的間隔。
	healthCheckInterval = 500 * time.Millisecond

	// startupTimeout 是等待健康檢查通過的最長時間。
	startupTimeout = 30 * time.Second

	// gracefulShutdownTimeout 是發送 SIGINT 後等待程序退出的時間。
	gracefulShutdownTimeout = 5 * time.Second

	// restartInitialDelay 是崩潰後首次重啟前的等待時間。
	restartInitialDelay = 3 * time.Second

	// restartBackoffDelay 是連續崩潰後的重啟等待時間。
	restartBackoffDelay = 10 * time.Second
)

// ProcessManager 管理 fubon-proxy Python 服務的生命週期。
type ProcessManager struct {
	workDir    string
	pythonBin  string
	scriptPath string
	healthURL  string

	mu       sync.Mutex
	cmd      *exec.Cmd
	running  bool
	stopping bool
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewManager 建立新的 ProcessManager。
// workDir 為 atlas 專案根目錄，用於定位 services/fubon-proxy/main.py。
// 自動偵測 Python 虛擬環境路徑：優先使用 ~/.config/atlas-go/.fubon-env/bin/python，
// 若不存在則回退至系統 python3。
func NewManager(workDir string) *ProcessManager {
	pythonBin := resolvePythonBin()
	scriptPath := filepath.Join(workDir, "services", "fubon-proxy", "main.py")
	scriptPath, _ = filepath.Abs(scriptPath)

	return &ProcessManager{
		workDir:    workDir,
		pythonBin:  pythonBin,
		scriptPath: scriptPath,
		healthURL:  healthEndpoint,
	}
}

// resolvePythonBin 偵測 Python 可執行檔路徑。
// 優先檢查 ~/.config/atlas-go/.fubon-env/bin/python，回退至系統 python3。
func resolvePythonBin() string {
	homeDir, err := os.UserHomeDir()
	if err == nil {
		venvPython := filepath.Join(homeDir, ".config", "atlas-go", ".fubon-env", "bin", "python")
		if info, err := os.Stat(venvPython); err == nil && !info.IsDir() {
			logging.Info("fubonproxy", "venv_python_found", "path", venvPython)
			return venvPython
		}
	}

	if path, err := exec.LookPath("python3"); err == nil {
		logging.Info("fubonproxy", "system_python_found", "path", path)
		return path
	}

	logging.Warn("fubonproxy", "no_python_found", "message", "neither venv python nor system python3 found")
	return ""
}

// Start 啟動 fubon-proxy 服務（非同步 — circuit breaker pattern）。
// 若服務已在運行（健康檢查通過），則跳過啟動。
// 若 Python 路徑未找到或腳本不存在，記錄警告後回傳錯誤（非致命）。
//
// Circuit breaker: Start() 立即返回；健康檢查在背景 goroutine 中進行。
// 若健康檢查未在 startupTimeout 內通過，僅記錄警告，不視為致命錯誤。
func (m *ProcessManager) Start(ctx context.Context) error {
	if m.pythonBin == "" {
		return fmt.Errorf("fubonproxy: python binary not found")
	}

	if _, err := os.Stat(m.scriptPath); err != nil {
		return fmt.Errorf("fubonproxy: script not found at %s: %w", m.scriptPath, err)
	}

	// 檢查是否已有實體在運行
	if m.IsHealthy() {
		logging.Info("fubonproxy", "already_running", "message", "fubon-proxy is already running, skipping start")
		return nil
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	m.ctx = ctx
	m.cancel = cancel
	m.done = make(chan struct{})

	cmd := exec.CommandContext(ctx, m.pythonBin, m.scriptPath)
	cmd.Dir = filepath.Dir(m.scriptPath)
	cmd.Env = os.Environ()

	// 將 Python 程序的 stdout/stderr 導向 atlas 的日誌
	cmd.Stdout = &logWriter{component: "fubonproxy.stdout"}
	cmd.Stderr = &logWriter{component: "fubonproxy.stderr"}

	if err := cmd.Start(); err != nil {
		cancel()
		// 關閉 m.done 以避免 Stop() 永久阻塞 (no-supervise edge case)
		close(m.done)
		m.ctx = nil
		m.cancel = nil
		m.done = nil
		m.mu.Unlock()
		return fmt.Errorf("fubonproxy: failed to start process: %w", err)
	}

	m.cmd = cmd
	m.running = true
	m.mu.Unlock()

	logging.Info("fubonproxy", "process_started",
		"pid", cmd.Process.Pid,
		"python", m.pythonBin,
		"script", m.scriptPath,
	)

	// Circuit breaker: 健康檢查在背景 goroutine 中進行（不阻塞 Start）
	go func() {
		if err := m.waitForHealthy(ctx); err != nil {
			logging.Warn("fubonproxy", "health_check_timeout",
				logging.Err(err),
				"message", "proxy started but health check did not pass within timeout",
			)
		} else {
			logging.Info("fubonproxy", "health_check_passed")
		}
	}()

	go m.supervise()

	return nil
}

// Stop 停止 fubon-proxy 服務。
// 先發送 SIGINT，等待 5 秒後強制終止。
//
// 關鍵不變式：無論 m.running 為何，都會先設定 m.stopping 並取消 context。
// 這確保即使在 supervise() 的重啟路徑中被呼叫，也能中斷其重啟邏輯，
// 避免孤兒行程（orphan process）持續在背景執行（F1: Stop/restart race）。
func (m *ProcessManager) Stop() {
	m.mu.Lock()
	if m.stopping {
		m.mu.Unlock()
		return
	}
	// 必須先設定 m.stopping，supervise() 在重啟路徑看到此旗標才會退出
	m.stopping = true
	cancel := m.cancel
	cmd := m.cmd
	done := m.done
	m.mu.Unlock()

	logging.Info("fubonproxy", "stopping")

	// 無論是否有 cmd 都在第一時間取消 context，迫使 supervise() 的等待返回
	if cancel != nil {
		cancel()
	}

	if cmd != nil && cmd.Process != nil {
		// 發送 SIGINT 進行優雅關閉
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
			logging.Warn("fubonproxy", "sigint_failed", logging.Err(err))
		}

		// 等待 supervise() 結束（會在 cmd 退出後 close m.done）
		// 加上 graceful shutdown timeout 作為安全網
		select {
		case <-done:
			logging.Info("fubonproxy", "stopped_gracefully")
		case <-time.After(gracefulShutdownTimeout):
			logging.Warn("fubonproxy", "graceful_timeout_exceeded", "message", "force killing process")
			if err := cmd.Process.Kill(); err != nil {
				logging.Warn("fubonproxy", "kill_failed", logging.Err(err))
			}
		}
	} else if done != nil {
		// 沒有 cmd 但 supervise() 可能正在重啟路徑中等 ctx — 等待它退出
		<-done
	}

	m.mu.Lock()
	m.running = false
	m.stopping = false
	m.cmd = nil
	m.cancel = nil
	m.done = nil
	m.mu.Unlock()

	logging.Info("fubonproxy", "stopped")
}

// IsHealthy 檢查 fubon-proxy 是否健康運行。
// 向 m.healthURL 發送 GET 請求，期望 HTTP 200。
func (m *ProcessManager) IsHealthy() bool {
	client := &http.Client{Timeout: healthCheckTimeout}
	resp, err := client.Get(m.healthURL)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// waitForHealthy 輪詢健康檢查直到通過、超時、或 ctx 取消。
// 採用 ctx.Deadline() 與 startupTimeout 中較早者作為實際截止時間（F5）。
func (m *ProcessManager) waitForHealthy(ctx context.Context) error {
	deadline := time.Now().Add(startupTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if m.IsHealthy() {
			return nil
		}

		// 使用 select 包裝 sleep，讓 ctx 取消能立即被感知（不需等滿 healthCheckInterval）
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("fubon-proxy health check did not pass within %v", startupTimeout)
}

// supervise 在背景 goroutine 中監控程序健康狀態，崩潰時自動重啟。
// 採用 lock-check-unlock-work-lock 模式，避免 panic 導致 mutex 永久鎖定（F7）。
func (m *ProcessManager) supervise() {
	defer close(m.done)

	backoff := restartInitialDelay

	for {
		m.mu.Lock()
		cmd := m.cmd
		m.mu.Unlock()

		if cmd == nil {
			return
		}

		// 等待程序結束
		err := cmd.Wait()
		m.mu.Lock()
		wasStopping := m.stopping
		m.running = false
		m.mu.Unlock()

		if wasStopping {
			return
		}

		logging.Warn("fubonproxy", "process_exited",
			logging.Err(err),
			"message", "will attempt restart",
			"backoff", backoff.String(),
		)

		// 等待 backoff 期間或取消
		select {
		case <-time.After(backoff):
		case <-m.ctx.Done():
			logging.Info("fubonproxy", "supervisor_cancelled")
			return
		}

		// 嘗試重啟 — 先在鎖內檢查 stopping 並 snapshot ctx（F6：在鎖內讀 m.ctx）
		m.mu.Lock()
		if m.stopping {
			m.mu.Unlock()
			return
		}
		ctx := m.ctx
		m.mu.Unlock()

		// 在鎖外建構與啟動程序 — 若 exec.Cmd.Start() panic 不會卡死 mutex（F7）
		newCmd := exec.CommandContext(ctx, m.pythonBin, m.scriptPath)
		newCmd.Dir = filepath.Dir(m.scriptPath)
		newCmd.Env = os.Environ()
		newCmd.Stdout = &logWriter{component: "fubonproxy.stdout"}
		newCmd.Stderr = &logWriter{component: "fubonproxy.stderr"}

		if startErr := newCmd.Start(); startErr != nil {
			logging.Error("fubonproxy", "restart_failed", logging.Err(startErr))
			backoff = restartBackoffDelay
			continue
		}

		// 啟動成功 — 在鎖內更新狀態；若同時被要求停止，立即終止新程序（F1）
		m.mu.Lock()
		if m.stopping {
			m.mu.Unlock()
			_ = newCmd.Process.Kill()
			return
		}
		m.cmd = newCmd
		m.running = true
		m.mu.Unlock()

		logging.Info("fubonproxy", "process_restarted",
			"pid", newCmd.Process.Pid,
			"backoff", backoff.String(),
		)

		// 同步等待健康檢查 — 阻塞此 goroutine 安全（supervise 本身在背景 goroutine）
		// 同步等待確保 backoff 僅在健康通過時重置（F2）
		// 不再 fire-and-forget 產生 goroutine，避免快速重啟下的堆積（F4）
		if healthErr := m.waitForHealthy(ctx); healthErr != nil {
			logging.Warn("fubonproxy", "restart_health_check_failed", logging.Err(healthErr))
			// 失敗時保留較長 backoff，不重置
		} else {
			logging.Info("fubonproxy", "restart_health_check_passed")
			backoff = restartInitialDelay
		}
	}
}

// logWriter 將 Python 程序的輸出轉發至 atlas 日誌系統。
type logWriter struct {
	component string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	if msg != "" {
		logging.Info(w.component, "output", "message", msg)
	}
	return len(p), nil
}
