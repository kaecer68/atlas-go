// Package fubonproxy manages the Fubon MarketData Proxy lifecycle.
//
// Maturity: evolving
//
// The manager auto-starts the Python FastAPI service on atlas boot,
// monitors its health, restarts on failure, and gracefully shuts it
// down on atlas exit.
package fubonproxy

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// Manager controls the fubon-proxy Python process lifecycle.
type Manager struct {
	workDir    string
	pythonPath string
	scriptPath string
	proxyURL   string

	cmd     *exec.Cmd
	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewManager creates a new fubon-proxy manager.
// It auto-detects the Python virtual-env and script paths.
func NewManager(workDir string) *Manager {
	// Resolve to absolute path so relative workDir (e.g. ".") does not break
	// the script path when combined with cmd.Dir.
	absWorkDir, _ := filepath.Abs(workDir)
	if absWorkDir == "" {
		absWorkDir = workDir
	}

	// Prefer the dedicated venv under ~/.config/atlas-go/.fubon-env
	venvPython := filepath.Join(os.Getenv("HOME"), ".config", "atlas-go", ".fubon-env", "bin", "python")
	if _, err := os.Stat(venvPython); os.IsNotExist(err) {
		venvPython = "python3"
	}

	script := filepath.Join(absWorkDir, "services", "fubon-proxy", "main.py")

	return &Manager{
		workDir:    absWorkDir,
		pythonPath: venvPython,
		scriptPath: script,
		proxyURL:   "http://localhost:8081",
		stopCh:     make(chan struct{}),
	}
}

// Start launches the fubon-proxy process in the background and begins
// health-monitoring. If the process exits unexpectedly it is restarted
// automatically (with a short back-off).
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	if err := m.spawn(ctx); err != nil {
		return fmt.Errorf("fubonproxy: initial spawn failed: %w", err)
	}

	m.started = true
	m.wg.Add(1)
	go m.supervise()

	// Wait a moment for the service to bind its port before declaring success.
	if err := m.waitHealthy(ctx, 10*time.Second); err != nil {
		logging.Warn("fubonproxy", "initial_health_check_failed", "err", err)
		// We keep running — the supervisor will retry.
	}

	logging.Info("fubonproxy", "started", "url", m.proxyURL)
	return nil
}

// Stop terminates the fubon-proxy process gracefully.
func (m *Manager) Stop() error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	close(m.stopCh)
	m.mu.Unlock()

	m.wg.Wait()
	logging.Info("fubonproxy", "stopped")
	return nil
}

// IsHealthy reports whether the proxy is currently reachable.
func (m *Manager) IsHealthy() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(m.proxyURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Manager) spawn(ctx context.Context) error {
	m.cmd = exec.CommandContext(ctx, m.pythonPath, m.scriptPath)
	m.cmd.Dir = filepath.Dir(m.scriptPath)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	// Pass through the env so python-dotenv can read ~/.config/atlas-go/.env.
	m.cmd.Env = os.Environ()

	if err := m.cmd.Start(); err != nil {
		return err
	}
	logging.Info("fubonproxy", "spawned", "pid", m.cmd.Process.Pid)
	return nil
}

func (m *Manager) supervise() {
	defer m.wg.Done()

	for {
		select {
		case <-m.stopCh:
			m.kill()
			return
		default:
		}

		if m.cmd != nil && m.cmd.Process != nil {
			_, err := m.cmd.Process.Wait()
			if err != nil {
				logging.Warn("fubonproxy", "process_exited", "err", err)
			} else {
				logging.Warn("fubonproxy", "process_exited_cleanly")
			}
		}

		select {
		case <-m.stopCh:
			return
		case <-time.After(3 * time.Second):
			// back-off before restart
		}

		logging.Info("fubonproxy", "restarting")
		if err := m.spawn(context.Background()); err != nil {
			logging.Error("fubonproxy", "restart_failed", "err", err)
			select {
			case <-m.stopCh:
				return
			case <-time.After(10 * time.Second):
				// longer back-off on repeated failures
			}
		}
	}
}

func (m *Manager) kill() {
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Signal(os.Interrupt)
		// Give it a grace period, then force-kill.
		time.AfterFunc(5*time.Second, func() {
			_ = m.cmd.Process.Kill()
		})
	}
}

func (m *Manager) waitHealthy(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.IsHealthy() {
				return nil
			}
		}
	}
}
