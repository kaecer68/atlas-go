package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: test-hybrid [--help]")
		fmt.Println("Tests the Hybrid market-data provider (Fugle primary, TWSE fallback).")
		os.Exit(0)
	}

	cfg := config.Load()

	fmt.Println("🔄 測試 Hybrid Provider (Fubon → Fugle → TWSE)")
	fmt.Println("==================================================")

	twseClient := marketdata.NewTWSEClient()
	provider := marketdata.NewHybridProvider(cfg.FinMindAPIKey, cfg.FugleAPIKey)
	fmt.Printf("✅ Provider 創建成功: %s\n\n", provider.Name())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 測試 1: 嘗試獲取 0050, 2330（TWSE 免費）
	testSymbols := []string{"0050", "2330", "2317"}
	fmt.Printf("📡 測試獲取 %d 只股票行情...\n", len(testSymbols))

	quotes, err := provider.GetQuotes(ctx, time.Now(), testSymbols)
	if err != nil {
		fmt.Printf("❌ 獲取失敗: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 成功獲取 %d 只股票行情\n\n", len(quotes))

	fmt.Println("行情詳情:")
	fmt.Println("----------")
	for _, q := range quotes {
		fmt.Printf("  %-6s | 最新: %7.2f | 開盤: %7.2f | 最高: %7.2f | 最低: %7.2f | 量: %10d | 來源: %s\n",
			q.Symbol, q.Last, q.Open, q.High, q.Low, q.Volume, q.Source)
	}

	// 測試 2: 檢查當前使用的數據源
	fmt.Printf("\n📊 當前使用數據源: ")
	if provider.IsUsingTWSE() {
		fmt.Println("Fugle (付費/限制)")
	} else {
		fmt.Println("TWSE OpenAPI (免費)")
	}

	// 測試 3: 直接測試 TWSE Client（獲取全部上市股票）
	fmt.Println("\n📡 測試 TWSE Client 獲取全部上市股票...")
	allQuotes, err := twseClient.GetQuotes(ctx)
	if err != nil {
		fmt.Printf("⚠️  TWSE 獲取全部失敗: %v\n", err)
	} else {
		fmt.Printf("✅ TWSE 獲取全部上市股票: %d 只\n", len(allQuotes))
		if len(allQuotes) > 0 {
			fmt.Printf("   第一只: %s (%s) - %.2f\n",
				allQuotes[0].Symbol, allQuotes[0].Market, allQuotes[0].Last)
			fmt.Printf("   最后一只: %s (%s) - %.2f\n",
				allQuotes[len(allQuotes)-1].Symbol,
				allQuotes[len(allQuotes)-1].Market,
				allQuotes[len(allQuotes)-1].Last)
		}
	}

	// 測試 4: 測試單獨獲取 1476（v1.0 全市場可查任意 symbol；1476 為範例）
	fmt.Println("\n📡 測試獲取 1476 (v1.0 Fugle API, 全市場)...")
	provider.UseFugle() // 強制使用 Fugle
	quotes1476, err := provider.GetQuotes(ctx, time.Now(), []string{"1476"})
	if err != nil {
		fmt.Printf("⚠️  獲取 1476 失敗: %v\n", err)
	} else if len(quotes1476) > 0 {
		fmt.Printf("✅ 1476 行情: %.2f (來源: %s)\n", quotes1476[0].Last, quotes1476[0].Source)
	}

	fmt.Println("\n🎉 所有測試完成！")
	fmt.Println("\n💡 使用說明:")
	fmt.Println("   - Hybrid Provider 優先順序: TWSE → FinMind → Fubon → Fugle")
	fmt.Println("   - TWSE 免費但有 rate limit (3 req/5s)")
	fmt.Println("   - FinMind 免費，需 API Key")
	fmt.Println("   - Fubon 免費，需富邦證券帳戶")
	fmt.Println("   - Fugle 付費，circuit breaker 保護")
}
