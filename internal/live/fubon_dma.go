package live

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// fubonDMARequest 是傳送給 Python wrapper 的 JSONL 指令。
type fubonDMARequest struct {
	Cmd         string   `json:"cmd"`
	PersonalID  string   `json:"personal_id,omitempty"`
	APIKey      string   `json:"api_key,omitempty"`
	Symbol      string   `json:"symbol,omitempty"`
	Side        string   `json:"side,omitempty"`
	Quantity    int      `json:"quantity,omitempty"`
	Price       *float64 `json:"price,omitempty"`
	MarketType  string   `json:"market_type,omitempty"`
	PriceType   string   `json:"price_type,omitempty"`
	TimeInForce string   `json:"time_in_force,omitempty"`
	OrderType   string   `json:"order_type,omitempty"`
	UserDef     string   `json:"user_def,omitempty"`
}

// fubonDMAResponse 是 Python wrapper 回傳的 JSONL 回應。
type fubonDMAResponse struct {
	Status    string   `json:"status"`
	Code      string   `json:"code,omitempty"`
	Message   string   `json:"message,omitempty"`
	IsSuccess bool     `json:"is_success,omitempty"`
	OrderID   string   `json:"order_id,omitempty"`
	Accounts  []string `json:"accounts,omitempty"`
	Detail    any      `json:"detail,omitempty"`
}

// FubonDMAAdapter 透過 Python subprocess 與富邦 DMA SDK 通訊，
// 實作 LiveExecutionAdapter 介面。
type FubonDMAAdapter struct {
	personalID string
	apiKey     string
	scriptPath string
	pythonPath string

	mu        sync.RWMutex
	proc      *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	connected bool
}

// FubonDMAAdapterConfig 包含 FubonDMAAdapter 的設定。
type FubonDMAAdapterConfig struct {
	PersonalID string
	APIKey     string
	ScriptPath string
	PythonPath string
}

// NewFubonDMAAdapter 建立富邦 DMA adapter，但尚未啟動 subprocess。
// 呼叫 Connect() 以啟動 Python wrapper 並登入。
func NewFubonDMAAdapter(cfg FubonDMAAdapterConfig) *FubonDMAAdapter {
	pythonPath := cfg.PythonPath
	if pythonPath == "" {
		pythonPath = "python3"
	}
	scriptPath := cfg.ScriptPath
	if scriptPath == "" {
		scriptPath = "cmd/fubon-dma/wrapper.py"
	}
	return &FubonDMAAdapter{
		personalID: cfg.PersonalID,
		apiKey:     cfg.APIKey,
		scriptPath: scriptPath,
		pythonPath: pythonPath,
	}
}

// Connect 啟動 Python subprocess 並執行 DMA 登入。
func (a *FubonDMAAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return nil
	}

	cmd := exec.CommandContext(ctx, a.pythonPath, a.scriptPath)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("fubon dma: create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdinPipe.Close()
		return fmt.Errorf("fubon dma: create stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = stdinPipe.Close()
		_ = stdoutPipe.Close()
		return fmt.Errorf("fubon dma: start python process: %w", err)
	}

	a.proc = cmd
	a.stdin = stdinPipe
	a.stdout = bufio.NewReader(stdoutPipe)

	// 等待初始化訊息（mock 模式會送出 warn）
	initLine, err := a.stdout.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		a.killProcess()
		return fmt.Errorf("fubon dma: read init message: %w", err)
	}
	initLine = strings.TrimSpace(initLine)
	if initLine != "" {
		var initResp fubonDMAResponse
		if jsonErr := json.Unmarshal([]byte(initLine), &initResp); jsonErr == nil {
			if initResp.Status == "warn" {
				// SDK 未安裝的警告，可繼續
			}
		}
	}

	// 執行登入
	loginReq := fubonDMARequest{
		Cmd:        "login",
		PersonalID: a.personalID,
		APIKey:     a.apiKey,
	}
	resp, err := a.sendRequest(loginReq)
	if err != nil {
		a.killProcess()
		return fmt.Errorf("fubon dma: login request: %w", err)
	}

	if resp.Status != "ok" || !resp.IsSuccess {
		a.killProcess()
		msg := resp.Message
		if msg == "" {
			msg = resp.Code
		}
		return fmt.Errorf("fubon dma: login failed: %s", msg)
	}

	a.connected = true
	return nil
}

// SubmitOrder 實作 LiveExecutionAdapter 介面，透過 DMA 下單。
func (a *FubonDMAAdapter) SubmitOrder(ctx context.Context, order domain.Order) (BrokerResult, error) {
	if err := validateOrder(order); err != nil {
		return BrokerResult{}, err
	}

	a.mu.RLock()
	if !a.connected {
		a.mu.RUnlock()
		return BrokerResult{
			Status: "rejected",
			Reason: "fubon dma adapter not connected",
		}, nil
	}
	a.mu.RUnlock()

	side := "BUY"
	if order.Side == domain.SideSell {
		side = "SELL"
	}

	req := fubonDMARequest{
		Cmd:         "submit_order",
		Symbol:      order.Symbol,
		Side:        side,
		Quantity:    order.Quantity,
		PriceType:   "LIMIT",
		TimeInForce: "ROD",
		OrderType:   "STOCK",
	}
	if order.Price > 0 {
		p := order.Price
		req.Price = &p
	}

	resp, err := a.sendRequest(req)
	if err != nil {
		return BrokerResult{}, fmt.Errorf("fubon dma: submit order: %w", err)
	}

	if resp.Status != "ok" {
		reason := resp.Message
		if reason == "" {
			reason = resp.Code
		}
		return BrokerResult{
			Status: "rejected",
			Reason: reason,
		}, nil
	}

	return BrokerResult{
		OrderID:   resp.OrderID,
		Status:    "placed",
		FillPrice: 0,
		Reason:    "fubon dma order placed",
	}, nil
}

// Ping 檢查 Python subprocess 是否仍在運作。
func (a *FubonDMAAdapter) Ping(ctx context.Context) error {
	a.mu.RLock()
	if !a.connected {
		a.mu.RUnlock()
		return fmt.Errorf("fubon dma: not connected")
	}
	a.mu.RUnlock()

	resp, err := a.sendRequest(fubonDMARequest{Cmd: "ping"})
	if err != nil {
		return fmt.Errorf("fubon dma: ping: %w", err)
	}
	if resp.Status != "ok" {
		return fmt.Errorf("fubon dma: ping failed: %s", resp.Message)
	}
	return nil
}

// Close 登出並關閉 Python subprocess。
func (a *FubonDMAAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return nil
	}

	_, _ = a.sendRequest(fubonDMARequest{Cmd: "logout"})
	a.connected = false
	a.killProcess()
	return nil
}

// sendRequest 傳送 JSONL 指令至 Python subprocess 並讀取回應。
// 注意：此方法假設呼叫端已持有適當的鎖。
func (a *FubonDMAAdapter) sendRequest(req fubonDMARequest) (fubonDMAResponse, error) {
	if a.stdin == nil || a.stdout == nil {
		return fubonDMAResponse{}, fmt.Errorf("fubon dma: subprocess not running")
	}

	payload, err := json.Marshal(req) //nolint:gosec // APIKey must be serialized for the Python proxy
	if err != nil {
		return fubonDMAResponse{}, fmt.Errorf("fubon dma: marshal request: %w", err)
	}

	if _, err := fmt.Fprintf(a.stdin, "%s\n", payload); err != nil {
		return fubonDMAResponse{}, fmt.Errorf("fubon dma: write to subprocess: %w", err)
	}

	line, err := a.stdout.ReadString('\n')
	if err != nil {
		return fubonDMAResponse{}, fmt.Errorf("fubon dma: read from subprocess: %w", err)
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return fubonDMAResponse{}, fmt.Errorf("fubon dma: empty response from subprocess")
	}

	var resp fubonDMAResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return fubonDMAResponse{}, fmt.Errorf("fubon dma: unmarshal response: %w (raw: %s)", err, line)
	}

	return resp, nil
}

// killProcess 強制終止 Python subprocess。
func (a *FubonDMAAdapter) killProcess() {
	if a.proc == nil || a.proc.Process == nil {
		return
	}
	if a.stdin != nil {
		_ = a.stdin.Close()
	}
	// 給予短暫寬限期再 kill
	done := make(chan error, 1)
	go func() {
		done <- a.proc.Wait()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = a.proc.Process.Kill()
	}
}
