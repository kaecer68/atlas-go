package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	cfg := config.Load()

	if cfg.FugleAPIKey == "" {
		fmt.Println("❌ Fugle API Key 未设置")
		fmt.Println("请确保 .env 文件存在且包含 FUGLE_API_KEY")
		os.Exit(1)
	}

	fmt.Println("✅ Fugle API Key 已加载")
	fmt.Printf("   Key 前缀: %s...\n", cfg.FugleAPIKey[:20])

	// 创建 Fugle 客户端
	client := marketdata.NewFugleClient(cfg.FugleAPIKey)

	// 测试获取行情（demo key 仅限 1476）
	fmt.Println("\n📡 测试获取行情 (symbol: 1476)...")
	fmt.Println("   注: 当前 API Key 为 demo 版本，仅限访问 1476")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	quote, err := client.GetQuote(ctx, "1476")
	if err != nil {
		fmt.Printf("❌ 获取行情失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 成功获取行情")
	fmt.Printf("   代码: %s\n", quote.Symbol)
	fmt.Printf("   名称: 元大台灣50\n")
	fmt.Printf("   最新价: %.2f\n", quote.Last)
	fmt.Printf("   开盘价: %.2f\n", quote.Open)
	fmt.Printf("   最高价: %.2f\n", quote.High)
	fmt.Printf("   最低价: %.2f\n", quote.Low)
	fmt.Printf("   成交量: %d\n", quote.Volume)
	fmt.Printf("   时间: %s\n", quote.AsOf.Format("2006-01-02 15:04:05"))

	// 测试批量获取（demo key 仅限 1476）
	fmt.Println("\n📡 测试批量获取行情 (1476)...")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	quotes, err := client.GetQuotes(ctx2, []string{"1476"})
	if err != nil {
		fmt.Printf("❌ 批量获取失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 成功获取 %d 只股票行情\n", len(quotes))
	for _, q := range quotes {
		fmt.Printf("   %s: %.2f (成交量: %d)\n", q.Symbol, q.Last, q.Volume)
	}

	// 测试市场状态检查
	fmt.Println("\n📡 测试市场状态检查...")
	ctx3, cancel3 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel3()

	isOpen, err := client.CheckMarketStatus(ctx3)
	if err != nil {
		fmt.Printf("⚠️  市场状态检查失败: %v\n", err)
	} else if isOpen {
		fmt.Println("✅ 市场状态: 正常交易")
	} else {
		fmt.Println("⚠️  市场状态: 休市或停牌")
	}

	fmt.Println("\n🎉 所有测试通过！Fugle API 连接正常")
	fmt.Println("\n💡 提示: 当前为 demo API key，仅限访问 symbol 1476")
}
