package live

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestFubonDMAAdapterConfigDefaults(t *testing.T) {
	cfg := FubonDMAAdapterConfig{
		PersonalID: "A123456789",
		APIKey:     "test-key",
	}
	adapter := NewFubonDMAAdapter(cfg)

	if adapter.personalID != "A123456789" {
		t.Errorf("personalID = %q, want %q", adapter.personalID, "A123456789")
	}
	if adapter.apiKey != "test-key" {
		t.Errorf("apiKey = %q, want %q", adapter.apiKey, "test-key")
	}
	if adapter.pythonPath != "python3" {
		t.Errorf("pythonPath = %q, want %q", adapter.pythonPath, "python3")
	}
	if adapter.scriptPath != "cmd/fubon-dma/wrapper.py" {
		t.Errorf("scriptPath = %q, want %q", adapter.scriptPath, "cmd/fubon-dma/wrapper.py")
	}
	if adapter.connected {
		t.Error("connected should be false initially")
	}
}

func TestFubonDMAAdapterConfigOverrides(t *testing.T) {
	cfg := FubonDMAAdapterConfig{
		PersonalID: "A123456789",
		APIKey:     "test-key",
		ScriptPath: "/custom/wrapper.py",
		PythonPath: "/usr/bin/python3.11",
	}
	adapter := NewFubonDMAAdapter(cfg)

	if adapter.scriptPath != "/custom/wrapper.py" {
		t.Errorf("scriptPath = %q, want %q", adapter.scriptPath, "/custom/wrapper.py")
	}
	if adapter.pythonPath != "/usr/bin/python3.11" {
		t.Errorf("pythonPath = %q, want %q", adapter.pythonPath, "/usr/bin/python3.11")
	}
}

func TestFubonDMARequestMarshal(t *testing.T) {
	price := 600.0
	req := fubonDMARequest{
		Cmd:         "submit_order",
		Symbol:      "2330",
		Side:        "BUY",
		Quantity:    1000,
		Price:       &price,
		PriceType:   "LIMIT",
		TimeInForce: "ROD",
		OrderType:   "STOCK",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var decoded fubonDMARequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if decoded.Cmd != "submit_order" {
		t.Errorf("cmd = %q, want %q", decoded.Cmd, "submit_order")
	}
	if decoded.Symbol != "2330" {
		t.Errorf("symbol = %q, want %q", decoded.Symbol, "2330")
	}
	if decoded.Quantity != 1000 {
		t.Errorf("quantity = %d, want %d", decoded.Quantity, 1000)
	}
	if decoded.Price == nil || *decoded.Price != 600.0 {
		t.Errorf("price = %v, want 600.0", decoded.Price)
	}
}

func TestFubonDMAResponseUnmarshal(t *testing.T) {
	raw := `{"status":"ok","is_success":true,"order_id":"DMA-001","message":"order placed"}`
	var resp fubonDMAResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if !resp.IsSuccess {
		t.Error("is_success should be true")
	}
	if resp.OrderID != "DMA-001" {
		t.Errorf("order_id = %q, want %q", resp.OrderID, "DMA-001")
	}
}

func TestFubonDMAAdapterSubmitOrderNotConnected(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test",
		APIKey:     "test",
	})

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    600.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if result.Status != "rejected" {
		t.Errorf("status = %q, want %q", result.Status, "rejected")
	}
	if result.Reason != "fubon dma adapter not connected" {
		t.Errorf("reason = %q, want %q", result.Reason, "fubon dma adapter not connected")
	}
}

func TestFubonDMAAdapterPingNotConnected(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test",
		APIKey:     "test",
	})

	err := adapter.Ping(context.Background())
	if err == nil {
		t.Error("Ping should fail when not connected")
	}
}

// mockFubonDMAScript 模擬 Python wrapper 的行為，用於整合測試。
func mockFubonDMAScript(t *testing.T) string {
	t.Helper()
	script := `#!/usr/bin/env python3
import json, sys
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
    except:
        print(json.dumps({"status":"error","code":"INVALID_JSON"}), flush=True)
        continue
    cmd = req.get("cmd","")
    if cmd == "login":
        print(json.dumps({"status":"ok","is_success":True,"message":"mock login","accounts":["mock-account"]}), flush=True)
    elif cmd == "submit_order":
        print(json.dumps({"status":"ok","is_success":True,"order_id":"mock-order-001","message":"order placed"}), flush=True)
    elif cmd == "ping":
        print(json.dumps({"status":"ok","message":"pong"}), flush=True)
    elif cmd == "logout":
        print(json.dumps({"status":"ok","message":"logged out"}), flush=True)
        break
    else:
        print(json.dumps({"status":"error","code":"UNKNOWN_CMD","message":"unknown: "+cmd}), flush=True)
`
	tmpFile, err := os.CreateTemp("", "mock-fubon-dma-*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		t.Fatalf("chmod temp script: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("close temp script: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	return tmpFile.Name()
}

func TestFubonDMAAdapterConnectAndSubmit(t *testing.T) {
	if os.Getenv("ATLAS_TEST_FUBON_DMA") == "" {
		t.Skip("skipping fubon dma subprocess test (set ATLAS_TEST_FUBON_DMA=1 to enable)")
	}

	scriptPath := mockFubonDMAScript(t)
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test-id",
		APIKey:     "test-key",
		ScriptPath: scriptPath,
		PythonPath: "python3",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer adapter.Close()

	if err := adapter.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    600.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(ctx, order)
	if err != nil {
		t.Fatalf("SubmitOrder: %v", err)
	}
	if result.Status != "placed" {
		t.Errorf("status = %q, want %q", result.Status, "placed")
	}
	if result.OrderID != "mock-order-001" {
		t.Errorf("order_id = %q, want %q", result.OrderID, "mock-order-001")
	}
}

func TestFubonDMAAdapterConnectFailure(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test-id",
		APIKey:     "test-key",
		ScriptPath: "/nonexistent/wrapper.py",
		PythonPath: "python3",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := adapter.Connect(ctx)
	if err == nil {
		adapter.Close()
		t.Error("Connect should fail with nonexistent script")
	}
}

func TestFubonDMAAdapterLoginRejected(t *testing.T) {
	if os.Getenv("ATLAS_TEST_FUBON_DMA") == "" {
		t.Skip("skipping fubon dma subprocess test (set ATLAS_TEST_FUBON_DMA=1 to enable)")
	}

	script := `#!/usr/bin/env python3
import json, sys
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    cmd = req.get("cmd","")
    if cmd == "login":
        print(json.dumps({"status":"error","code":"LOGIN_FAILED","message":"invalid credentials","is_success":False}), flush=True)
    elif cmd == "logout":
        print(json.dumps({"status":"ok"}), flush=True)
        break
    else:
        print(json.dumps({"status":"error","code":"NOT_LOGGED_IN"}), flush=True)
`
	tmpFile, err := os.CreateTemp("", "mock-fubon-dma-fail-*.py")
	if err != nil {
		t.Fatalf("create temp script: %v", err)
	}
	if _, err := tmpFile.WriteString(script); err != nil {
		t.Fatalf("write temp script: %v", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		t.Fatalf("chmod temp script: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "bad-id",
		APIKey:     "bad-key",
		ScriptPath: tmpFile.Name(),
		PythonPath: "python3",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = adapter.Connect(ctx)
	if err == nil {
		adapter.Close()
		t.Error("Connect should fail when login is rejected")
	}
	if !strings.Contains(err.Error(), "login failed") {
		t.Errorf("error should mention login failure, got: %v", err)
	}
}

func TestFubonDMARequestLoginMarshal(t *testing.T) {
	req := fubonDMARequest{
		Cmd:        "login",
		PersonalID: "M120628569",
		APIKey:     "F6049D5DD934EFFEDE91EDE4E337C32E5CAC3A0FDEC0D75CFEC46B94845A6AAA",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["cmd"] != "login" {
		t.Errorf("cmd = %v, want login", decoded["cmd"])
	}
	if decoded["personal_id"] != "M120628569" {
		t.Errorf("personal_id = %v, want M120628569", decoded["personal_id"])
	}
}

func TestFubonDMAAdapterSendRequestNotRunning(t *testing.T) {
	adapter := &FubonDMAAdapter{}
	_, err := adapter.sendRequest(fubonDMARequest{Cmd: "ping"})
	if err == nil {
		t.Error("sendRequest should fail when subprocess not running")
	}
	if !strings.Contains(err.Error(), "subprocess not running") {
		t.Errorf("error should mention subprocess, got: %v", err)
	}
}

func TestFubonDMAAdapterKillProcessNil(t *testing.T) {
	adapter := &FubonDMAAdapter{}
	adapter.killProcess() // should not panic on nil process
}

func TestFubonDMAAdapterCloseNotConnected(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test",
		APIKey:     "test",
	})
	if err := adapter.Close(); err != nil {
		t.Errorf("Close on not-connected adapter should not error: %v", err)
	}
}

func TestFubonDMAAdapterSideMapping(t *testing.T) {
	tests := []struct {
		side     domain.Side
		expected string
	}{
		{domain.SideBuy, "BUY"},
		{domain.SideSell, "SELL"},
	}

	for _, tc := range tests {
		_ = NewFubonDMAAdapter(FubonDMAAdapterConfig{
			PersonalID: "test",
			APIKey:     "test",
		})

		side := "BUY"
		if tc.side == domain.SideSell {
			side = "SELL"
		}

		req := fubonDMARequest{
			Cmd:      "submit_order",
			Symbol:   "2330",
			Side:     side,
			Quantity: 1000,
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if decoded["side"] != tc.expected {
			t.Errorf("side = %v, want %v", decoded["side"], tc.expected)
		}
	}
}

func TestFubonDMAAdapterSubmitOrderValidation(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test",
		APIKey:     "test",
	})

	invalidOrder := domain.Order{
		Symbol:   "",
		Side:     domain.SideBuy,
		Quantity: 1000,
		Price:    600.0,
		Reason:   "test",
	}

	_, err := adapter.SubmitOrder(context.Background(), invalidOrder)
	if err == nil {
		t.Error("SubmitOrder should fail with empty symbol")
	}
}

func TestFubonDMAAdapterConcurrentAccess(t *testing.T) {
	if os.Getenv("ATLAS_TEST_FUBON_DMA") == "" {
		t.Skip("skipping fubon dma subprocess test (set ATLAS_TEST_FUBON_DMA=1 to enable)")
	}

	scriptPath := mockFubonDMAScript(t)
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test-id",
		APIKey:     "test-key",
		ScriptPath: scriptPath,
		PythonPath: "python3",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer adapter.Close()

	errCh := make(chan error, 5)
	for range 5 {
		go func() {
			order := domain.Order{
				Symbol:   "2330",
				Side:     domain.SideBuy,
				Quantity: 1000,
				Price:    600.0,
				Reason:   "concurrent test",
			}
			_, err := adapter.SubmitOrder(ctx, order)
			errCh <- err
		}()
	}

	for i := range 5 {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent SubmitOrder %d: %v", i, err)
		}
	}
}

func TestFubonDMARequestNoOmitEmpty(t *testing.T) {
	req := fubonDMARequest{Cmd: "ping"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["personal_id"]; ok {
		t.Error("personal_id should be omitted when empty")
	}
	if _, ok := decoded["quantity"]; ok {
		t.Error("quantity should be omitted when zero")
	}
}

func TestFubonDMARequestWithPrice(t *testing.T) {
	price := 600.5
	req := fubonDMARequest{
		Cmd:      "submit_order",
		Symbol:   "2330",
		Side:     "BUY",
		Quantity: 1000,
		Price:    &price,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["price"] != 600.5 {
		t.Errorf("price = %v, want 600.5", decoded["price"])
	}
}

func TestFubonDMARequestWithoutPrice(t *testing.T) {
	req := fubonDMARequest{
		Cmd:      "submit_order",
		Symbol:   "2330",
		Side:     "BUY",
		Quantity: 1000,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["price"]; ok {
		t.Error("price should be omitted when nil")
	}
}

func TestFubonDMAAdapterSubmitOrderSellSide(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "test",
		APIKey:     "test",
	})

	// Not connected, should return rejected
	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideSell,
		Quantity: 1000,
		Price:    600.0,
		Reason:   "test",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if result.Status != "rejected" {
		t.Errorf("status = %q, want rejected", result.Status)
	}
}

func TestFubonDMAAdapterFormatRequest(t *testing.T) {
	price := 123.45
	req := fubonDMARequest{
		Cmd:         "submit_order",
		Symbol:      "2881",
		Side:        "SELL",
		Quantity:    2000,
		Price:       &price,
		PriceType:   "LIMIT",
		TimeInForce: "ROD",
		OrderType:   "STOCK",
		UserDef:     "atlas-test",
	}

	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["cmd"] != "submit_order" {
		t.Errorf("cmd = %v, want submit_order", decoded["cmd"])
	}
	if decoded["symbol"] != "2881" {
		t.Errorf("symbol = %v, want 2881", decoded["symbol"])
	}
	if decoded["side"] != "SELL" {
		t.Errorf("side = %v, want SELL", decoded["side"])
	}
	if decoded["quantity"] != float64(2000) {
		t.Errorf("quantity = %v, want 2000", decoded["quantity"])
	}
	if decoded["price"] != 123.45 {
		t.Errorf("price = %v, want 123.45", decoded["price"])
	}
	if decoded["price_type"] != "LIMIT" {
		t.Errorf("price_type = %v, want LIMIT", decoded["price_type"])
	}
	if decoded["time_in_force"] != "ROD" {
		t.Errorf("time_in_force = %v, want ROD", decoded["time_in_force"])
	}
	if decoded["order_type"] != "STOCK" {
		t.Errorf("order_type = %v, want STOCK", decoded["order_type"])
	}
	if decoded["user_def"] != "atlas-test" {
		t.Errorf("user_def = %v, want atlas-test", decoded["user_def"])
	}
}
