// Package fubonproxy manages the lifecycle of the Python fubon-proxy service.
//
// fubonproxy 啟動、監控、重啟 Python FastAPI 微服務，作為 atlas 啟動生命週期的一部分。
// 當 atlas 以 API/server 模式啟動時，ProcessManager 會自動偵測 fubon-proxy 是否已在運行，
// 若未運行則啟動並非同步等待健康檢查通過（circuit breaker pattern：Start() 立即返回，
// 健康檢查在背景 goroutine 中進行）。supervise goroutine 持續監控程序狀態並在崩潰時自動重啟。
//
// 使用方式：
//
//	mgr := fubonproxy.NewManager(cfg.WorkDir, 0)            // 用預設 :8081
//	mgr := fubonproxy.NewManager(cfg.WorkDir, 9999)         // 用 -fubon-port flag override
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
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// defaultFubonProxyPort 是 fubon-proxy 的預設 listen port(8081)。
// PR 2 Oracle 4th-round verdict F13:不指定 -fubon-port flag 時行為完全一致;
// marketdata 套件與本套件共用這常數以避免 port 漂移(F12:healthURL/proxyURL
// 必須同步來源)。
const defaultFubonProxyPort = 8081

const (
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

	// maxRestartFailures 是連續重啟失敗（含 port 衝突與 cmd.Start() 失敗）
	// 的上限；超過後 supervisor 放棄重啟，避免無限 EADDRINUSE loop。
	maxRestartFailures = 5
)

// restartInitialDelayForTest 與 restartBackoffDelayForTest 允許測試覆蓋
// supervise() 的 backoff 延遲，避免 cap 測試必須等待數十秒。生產環境保持預設常量。
var (
	restartInitialDelayForTest = restartInitialDelay
	restartBackoffDelayForTest = restartBackoffDelay

	// proxyListenPort 是 fubon-proxy 服務綁定的 TCP 埠。Start() 在 spawn
	// 之前會預先探測此埠的占用情況（F9 不變式），避免在 supervise() 內部
	// 才發現 EADDRINUSE 而陷入 backoff-loop。
	proxyListenPort = 8081
)

// portState 表達 port 8081 在 Start() 預先探測後的占用狀態。
// 設計目的：區分「可 spawn」、「外部 fubon-proxy 已管理」、「外部進程佔住」
// 三種情況，避免在 spawn() 內部才發現 EADDRINUSE 而延遲到 supervise() 才反應。
type portState = portprobe.State

const (
	portStateFree portState = iota
	portStateHealthy
	portStateForeign
)

// portOccupant 描述佔住 port 8081 的程序。用於構造 actionable error。
type portOccupant = portprobe.Occupant

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
	// restartFailures 由 supervise() goroutine 單一擁有，記錄連續重啟失敗次數
	//（含 port 衝突與 cmd.Start() 失敗），達到 maxRestartFailures 時放棄重啟。
	restartFailures int
	ctx             context.Context
	cancel          context.CancelFunc
	done            chan struct{}
}

// NewManager 建立新的 ProcessManager。
// workDir 為 atlas 專案根目錄，用於定位 services/fubon-proxy/main.py。
// 自動偵測 Python 虛擬環境路徑：優先使用 ~/.config/atlas-go/.fubon-env/bin/python，
// 若不存在則回退至系統 python3。
//
// port 為 fubon-proxy Python 服務綁定的 TCP 埠(也是 healthURL 的 port)。
// port > 0:override package-level proxyListenPort + log INFO(F11 custom port);
// port <= 0:不覆寫(保留測試 helper 透過 withFreeEphemeralPort 預設的值)。
// healthURL 動態從最終的 proxyListenPort 構造(PR 2 Oracle F12:與
// FubonClient.proxyURL 同步,皆由 cmd/atlas -fubon-port flag 統一注入)。
func NewManager(workDir string, port int) *ProcessManager {
	if port > 0 {
		if port != defaultFubonProxyPort {
			logging.Info("fubonproxy", "custom_listen_port",
				"port", port,
				"message", "fubon-proxy 將綁定非預設 port(預設 8081);由 cmd/atlas -fubon-port flag 注入。",
			)
		}
		proxyListenPort = port
	}

	pythonBin := resolvePythonBin()
	scriptPath := filepath.Join(workDir, "services", "fubon-proxy", "main.py")
	scriptPath, _ = filepath.Abs(scriptPath)

	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", proxyListenPort)

	return &ProcessManager{
		workDir:    workDir,
		pythonBin:  pythonBin,
		scriptPath: scriptPath,
		healthURL:  healthURL,
	}
}

// GetFubonProxyPort returns the TCP port fubon-proxy is configured to listen
// on, set by NewManager when port > 0 or by the cmd/atlas -fubon-port flag
// before NewManager is called. Returns defaultFubonProxyPort (8081) when
// NewManager has not yet been called with a non-zero port — callers that
// probe before fubonproxy.NewManager runs will see the default, which is
// the documented L1/L2 contract (Oracle F13).
func GetFubonProxyPort() int {
	return proxyListenPort
}

// resolvePythonBin 偵測 Python 可執行檔路徑。
// 優先檢查 FUBON_PROXY_PYTHON 環境變數（測試/開發覆蓋），
// 然後檢查 ~/.config/atlas-go/.fubon-env/bin/python，最後回退至系統 python3。
func resolvePythonBin() string {
	if envPath := os.Getenv("FUBON_PROXY_PYTHON"); envPath != "" {
		if info, err := os.Stat(envPath); err == nil && !info.IsDir() {
			logging.Info("fubonproxy", "env_python_found", "path", envPath)
			return envPath
		}
		logging.Warn("fubonproxy", "env_python_invalid",
			"path", envPath,
			"message", "FUBON_PROXY_PYTHON set but not a valid file; falling back",
		)
	}

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

	logging.Warn("fubonproxy", "no_python_found", "message", "neither env python, venv python nor system python3 found")
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

	// Pre-flight port 8081 探測（F9 不變式）。
	// 目的：避免 spawn() 在 supervise() 內部 EADDRINUSE 失敗而進入
	// backoff-loop（原始 bug：外部進程佔住 :8081 導致無限重啟 + 前端拿不到
	// fubon 資料）。三種狀態：
	//   - portStateFree     — 可 spawn，走原本路徑
	//   - portStateHealthy  — 外部 fubon-proxy 已管理 /health，跳過 spawn
	//   - portStateForeign  — 外部進程佔住，回傳 actionable error（含 PID+cmd）
	//
	// probe 失敗時退化為「直接 spawn」，保留舊行為；supervise() 仍會在
	// EADDRINUSE 時 retry，但我們已先把最常見的 lsof-not-found 情境 log 出來。
	state, occupant, probeErr := m.probePort8081()
	if probeErr != nil {
		logging.Warn("fubonproxy", "port_probe_failed",
			logging.Err(probeErr),
			"port", proxyListenPort,
			"fallback", "spawn_directly",
		)
	} else {
		switch state {
		case portStateHealthy:
			logging.Info("fubonproxy", "external_managed",
				"port", proxyListenPort,
				"occupant_pid", occupant.PID,
				"message", "port already serves /health; skipping spawn",
			)
			return nil
		case portStateForeign:
			if occupant.PID > 0 && isFubonZombie(occupant) {
				logging.Warn("fubonproxy", "zombie_detected",
					"pid", occupant.PID,
					"cmd", occupant.Command,
					"message", "auto-killing zombie fubon proxy on port "+strconv.Itoa(proxyListenPort),
				)
				if killErr := killOccupant(occupant.PID); killErr != nil {
					logging.Error("fubonproxy", "zombie_kill_failed",
						logging.Err(killErr),
						"message", "auto-kill failed; falling through to error",
					)
					return fmt.Errorf("fubonproxy: port %d held by zombie process "+
						"(pid=%d, cmd=%q); auto-kill failed: %v; "+
						"stop it manually with `kill %d`",
						proxyListenPort, occupant.PID, occupant.Command, killErr, occupant.PID)
				}
				// Kill successful — re-probe
				logging.Info("fubonproxy", "zombie_killed",
					"pid", occupant.PID,
					"message", "waiting for port release after zombie kill",
				)
				if !portprobe.WaitForPortFree(proxyListenPort, 5*time.Second) {
					logging.Warn("fubonproxy", "zombie_port_still_held",
						"port", proxyListenPort,
						"message", "port not free after zombie kill; proceeding with re-probe",
					)
				}
				newState, newOccupant, probeErr := m.probePort8081()
				if probeErr != nil {
					logging.Warn("fubonproxy", "reprobe_failed_after_zombie_kill",
						logging.Err(probeErr),
						"port", proxyListenPort,
						"fallback", "spawn_directly",
					)
					break // fall through to spawn
				}
				switch newState {
				case portStateFree:
					break // fall through to spawn
				case portStateHealthy:
					logging.Info("fubonproxy", "external_managed_after_kill",
						"port", proxyListenPort,
						"occupant_pid", newOccupant.PID,
						"message", "port now serves /health after kill; skipping spawn",
					)
					return nil
				case portStateForeign:
					return fmt.Errorf("fubonproxy: port %d still held after auto-kill "+
						"(pid=%d, cmd=%q); stop it with `kill %d`",
						proxyListenPort, newOccupant.PID, newOccupant.Command, newOccupant.PID)
				}
			} else if occupant.PID > 0 {
				return fmt.Errorf("fubonproxy: port %d held by foreign process "+
					"(pid=%d, cmd=%q); stop it with `kill %d` or change fubon-proxy port",
					proxyListenPort, occupant.PID, occupant.Command, occupant.PID)
			} else {
				return fmt.Errorf("fubonproxy: port %d held by unknown process; "+
					"identify it with `lsof -nP -iTCP:%d -sTCP:LISTEN` and stop it, "+
					"or change fubon-proxy port",
					proxyListenPort, proxyListenPort)
			}
		case portStateFree:
			// 走原本 spawn 路徑
		}
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
//
// 取消路徑：cancel() 會透過 exec.CommandContext 立即 SIGKILL cmd。
// 後續的 SIGINT 與 gracefulShutdownTimeout 是雙重安全網，
// 正常情況下不會觸發 — 程序已在前一步被終止。
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

// probePort8081 探測 port 8081 占用狀態（F9 pre-flight）。
// 同步執行（< 100ms + lsof ~50ms）。
// 用 wildcard (0.0.0.0, [::]) 而非 loopback (127.0.0.1, [::1]) probe：
// Docker Desktop port forwarder 綁 wildcard，loopback bind 在 wildcard
// 被佔時仍會成功，導致誤判 portStateFree。healthEndpoint 仍用 127.0.0.1
// 因為 Python fubon-proxy 實際 bind target 是 IPv4 wildcard 0.0.0.0。
func (m *ProcessManager) probePort8081() (portState, portOccupant, error) {
	addr := "127.0.0.1:" + strconv.Itoa(proxyListenPort)
	return portprobe.Probe(addr)
}

// lookupPortOccupant 用 lsof -F 機讀格式解析 port 占用者。
// lsof 不可用時回傳 error，呼叫端降級使用空 occupant。
func lookupPortOccupant(port int) (portOccupant, error) {
	addr := "127.0.0.1:" + strconv.Itoa(port)
	return portprobe.LookupOccupant(addr)
}

// isFubonZombie 判斷佔住 port 的程序是否為 fubon-proxy 的殭屍行程。
// 比對 lsof 回傳的 command name 是否包含 "python" 或 "uvicorn"。
// port 8081 是專用給 fubon-proxy 的端口，其他 Python 行程不應佔用此 port，
// 因此出現 Python/uvicorn → 幾乎可確定是 fubon 殭屍。
func isFubonZombie(occ portOccupant) bool {
	return portprobe.IsFubonZombie(occ)
}

// killOccupant 終止佔住 port 的程序。
// 先發送 SIGTERM 進行優雅關閉，等待 1 秒後檢查程序是否仍在運行，
// 若仍在運行則升級為 SIGKILL。
func killOccupant(pid int) error {
	return portprobe.KillOccupant(pid)
}

// preparePortForRestart 在 supervise() 重啟前檢查 port 8081 狀態。
// 回傳 (canProceed, shouldStop):
//   - canProceed=true, shouldStop=false: port 空閒，可以 spawn
//   - canProceed=false, shouldStop=true: port 由外部 healthy fubon-proxy 管理，supervisor 應退出
//   - canProceed=false, shouldStop=false: port 被佔用或探測失敗，應 retry/backoff
func (m *ProcessManager) preparePortForRestart() (bool, bool) {
	state, occupant, probeErr := m.probePort8081()
	if probeErr != nil {
		logging.Warn("fubonproxy", "restart_port_probe_failed",
			logging.Err(probeErr),
			"port", proxyListenPort,
			"fallback", "spawn_directly",
		)
		return true, false
	}

	switch state {
	case portStateFree:
		return true, false
	case portStateHealthy:
		logging.Info("fubonproxy", "restart_external_managed",
			"port", proxyListenPort,
			"occupant_pid", occupant.PID,
			"message", "port serves /health externally; supervisor will not respawn",
		)
		return false, true
	case portStateForeign:
		if occupant.PID > 0 && isFubonZombie(occupant) {
			logging.Warn("fubonproxy", "restart_zombie_detected",
				"pid", occupant.PID,
				"cmd", occupant.Command,
				"message", "auto-killing zombie fubon proxy on port "+strconv.Itoa(proxyListenPort),
			)
			if killErr := killOccupant(occupant.PID); killErr != nil {
				logging.Error("fubonproxy", "restart_zombie_kill_failed",
					logging.Err(killErr),
					"message", "auto-kill failed; will retry",
				)
				return false, false
			}
			newState, newOccupant, probeErr := m.probePort8081()
			if probeErr != nil {
				logging.Warn("fubonproxy", "restart_reprobe_failed_after_zombie_kill",
					logging.Err(probeErr),
					"port", proxyListenPort,
					"fallback", "spawn_directly",
				)
				return true, false
			}
			switch newState {
			case portStateFree:
				return true, false
			case portStateHealthy:
				logging.Info("fubonproxy", "restart_external_managed_after_kill",
					"port", proxyListenPort,
					"occupant_pid", newOccupant.PID,
					"message", "port serves /health after kill; supervisor will not respawn",
				)
				return false, true
			case portStateForeign:
				logging.Error("fubonproxy", "restart_port_still_held_after_kill",
					"port", proxyListenPort,
					"pid", newOccupant.PID,
					"cmd", newOccupant.Command,
					"message", "stop it manually with `kill "+strconv.Itoa(newOccupant.PID)+"`",
				)
				return false, false
			}
		} else if occupant.PID > 0 {
			logging.Error("fubonproxy", "restart_foreign_port",
				"port", proxyListenPort,
				"pid", occupant.PID,
				"cmd", occupant.Command,
				"message", "restart blocked; stop the process with `kill "+strconv.Itoa(occupant.PID)+"`",
			)
		} else {
			logging.Error("fubonproxy", "restart_foreign_port_unknown",
				"port", proxyListenPort,
				"message", "restart blocked; identify it with `lsof -nP -iTCP:"+strconv.Itoa(proxyListenPort)+" -sTCP:LISTEN` and stop it",
			)
		}
		return false, false
	}
	return true, false
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

	backoff := restartInitialDelayForTest

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

		// 重啟前再次探測 port，避免外部殭屍或外來進程佔用 8081 時陷入無限 EADDRINUSE loop
		canProceed, shouldStop := m.preparePortForRestart()
		if shouldStop {
			logging.Info("fubonproxy", "supervisor_yielding_to_external_proxy")
			return
		}
		if !canProceed {
			m.restartFailures++
			if m.restartFailures >= maxRestartFailures {
				logging.Error("fubonproxy", "max_restart_failures_reached",
					"count", m.restartFailures,
					"message", "giving up restarting fubon-proxy; port conflict or repeated start failures",
				)
				return
			}
			backoff = restartBackoffDelayForTest
			continue
		}

		// 在鎖外建構與啟動程序 — 若 exec.Cmd.Start() panic 不會卡死 mutex（F7）
		newCmd := exec.CommandContext(ctx, m.pythonBin, m.scriptPath)
		newCmd.Dir = filepath.Dir(m.scriptPath)
		newCmd.Env = os.Environ()
		newCmd.Stdout = &logWriter{component: "fubonproxy.stdout"}
		newCmd.Stderr = &logWriter{component: "fubonproxy.stderr"}

		if startErr := newCmd.Start(); startErr != nil {
			logging.Error("fubonproxy", "restart_failed", logging.Err(startErr))
			m.restartFailures++
			if m.restartFailures >= maxRestartFailures {
				logging.Error("fubonproxy", "max_restart_failures_reached",
					"count", m.restartFailures,
					"message", "giving up restarting fubon-proxy after repeated start failures",
				)
				return
			}
			backoff = restartBackoffDelayForTest
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
		m.restartFailures = 0
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
			backoff = restartInitialDelayForTest
			m.restartFailures = 0
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
