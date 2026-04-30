package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "login":
		return runLogin(args[1:])
	case "submit":
		return runSubmit(args[1:])
	case "logout":
		return runLogout(args[1:])
	case "ping":
		return runPing(args[1:])
	case "query":
		return runQuery(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printUsage() {
	fmt.Println("fubon-dma — 富邦 DMA 券商介面 CLI")
	fmt.Println()
	fmt.Println("指令：")
	fmt.Println("  login    登入富邦 DMA（API Key 模式）")
	fmt.Println("  submit   送出委託單")
	fmt.Println("  logout    登出")
	fmt.Println("  ping      檢查連線狀態")
	fmt.Println("  query     查詢今日委託")
	fmt.Println()
	fmt.Println("環境變數：")
	fmt.Println("  FUBON_DMA_PERSONAL_ID  身份證號")
	fmt.Println("  FUBON_DMA_API_KEY      API 金鑰")
	fmt.Println("  FUBON_DMA_SCRIPT_PATH  wrapper.py 路徑（預設 cmd/fubon-dma/wrapper.py）")
	fmt.Println("  FUBON_DMA_PYTHON_PATH  Python 執行檔路徑（預設 python3）")
}

func createAdapter(args []string) (*live.FubonDMAAdapter, error) {
	personalID := envOr("FUBON_DMA_PERSONAL_ID", "")
	apiKey := envOr("FUBON_DMA_API_KEY", "")
	scriptPath := envOr("FUBON_DMA_SCRIPT_PATH", "cmd/fubon-dma/wrapper.py")
	pythonPath := envOr("FUBON_DMA_PYTHON_PATH", "python3")

	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	personalIDFlag := fs.String("personal-id", personalID, "身份證號")
	apiKeyFlag := fs.String("api-key", apiKey, "API 金鑰")
	scriptPathFlag := fs.String("script-path", scriptPath, "wrapper.py 路徑")
	pythonPathFlag := fs.String("python-path", pythonPath, "Python 執行檔路徑")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	cfg := live.FubonDMAAdapterConfig{
		PersonalID: *personalIDFlag,
		APIKey:     *apiKeyFlag,
		ScriptPath: *scriptPathFlag,
		PythonPath: *pythonPathFlag,
	}

	if cfg.PersonalID == "" {
		return nil, fmt.Errorf("personal-id 為必填（使用 -personal-id 旗標或 FUBON_DMA_PERSONAL_ID 環境變數）")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("api-key 為必填（使用 -api-key 旗標或 FUBON_DMA_API_KEY 環境變數）")
	}

	return live.NewFubonDMAAdapter(cfg), nil
}

func runLogin(args []string) error {
	adapter, err := createAdapter(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := adapter.Connect(ctx); err != nil {
		return fmt.Errorf("登入失敗: %w", err)
	}
	defer adapter.Close()

	fmt.Println("登入成功")
	return nil
}

func runSubmit(args []string) error {
	fs := flag.NewFlagSet("submit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	symbol := fs.String("symbol", "", "股票代號（必填）")
	side := fs.String("side", "BUY", "買賣方向：BUY 或 SELL")
	quantity := fs.Int("qty", 0, "數量（必填，正整數）")
	price := fs.Float64("price", 0, "委託價格（0 表示市價）")
	personalID := fs.String("personal-id", envOr("FUBON_DMA_PERSONAL_ID", ""), "身份證號")
	apiKey := fs.String("api-key", envOr("FUBON_DMA_API_KEY", ""), "API 金鑰")
	scriptPath := fs.String("script-path", envOr("FUBON_DMA_SCRIPT_PATH", "cmd/fubon-dma/wrapper.py"), "wrapper.py 路徑")
	pythonPath := fs.String("python-path", envOr("FUBON_DMA_PYTHON_PATH", "python3"), "Python 執行檔路徑")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	if *symbol == "" {
		return fmt.Errorf("symbol 為必填")
	}
	if *quantity <= 0 {
		return fmt.Errorf("qty 必須為正整數")
	}
	if *personalID == "" {
		return fmt.Errorf("personal-id 為必填")
	}
	if *apiKey == "" {
		return fmt.Errorf("api-key 為必填")
	}

	cfg := live.FubonDMAAdapterConfig{
		PersonalID: *personalID,
		APIKey:     *apiKey,
		ScriptPath: *scriptPath,
		PythonPath: *pythonPath,
	}

	adapter := live.NewFubonDMAAdapter(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := adapter.Connect(ctx); err != nil {
		return fmt.Errorf("登入失敗: %w", err)
	}
	defer adapter.Close()

	domainSide := domain.SideBuy
	if strings.ToUpper(*side) == "SELL" {
		domainSide = domain.SideSell
	}

	order := domain.Order{
		Symbol:   *symbol,
		Side:     domainSide,
		Quantity: *quantity,
		Price:    *price,
		Reason:   "fubon-dma-cli",
	}

	result, err := adapter.SubmitOrder(ctx, order)
	if err != nil {
		return fmt.Errorf("下單失敗: %w", err)
	}

	enc, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(enc))
	return nil
}

func runLogout(args []string) error {
	adapter, err := createAdapter(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := adapter.Connect(ctx); err != nil {
		return fmt.Errorf("登入失敗: %w", err)
	}

	if err := adapter.Close(); err != nil {
		return fmt.Errorf("登出失敗: %w", err)
	}

	fmt.Println("已登出")
	return nil
}

func runPing(args []string) error {
	adapter, err := createAdapter(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := adapter.Connect(ctx); err != nil {
		return fmt.Errorf("連線失敗: %w", err)
	}
	defer adapter.Close()

	if err := adapter.Ping(ctx); err != nil {
		return fmt.Errorf("ping 失敗: %w", err)
	}

	fmt.Println("pong")
	return nil
}

func runQuery(args []string) error {
	return fmt.Errorf("query 尚未實作（需透過 wrapper.py 直接操作）")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
