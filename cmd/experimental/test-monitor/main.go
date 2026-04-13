package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: test-monitor [--help]")
		fmt.Println("Starts the monitoring system in test mode. Press Ctrl+C to stop.")
		os.Exit(0)
	}

	fmt.Println("🔍 Atlas-Go 监控系统测试")
	fmt.Println("========================")

	// 加载配置
	cfg := config.Load()

	// 1. 创建监控系统
	monitor := monitoring.NewMonitor()
	monitor.RegisterHandler(monitoring.ConsoleHandler)

	// 2. 创建指标收集器
	metricsCollector := monitoring.NewMetricsCollector()
	tradingMetrics := monitoring.NewTradingMetrics(metricsCollector, monitor)

	// 3. 创建状态存储
	stateStore := live.NewStateStore("data/state/live")
	if err := stateStore.Load(); err != nil {
		fmt.Printf("⚠️  状态存储加载失败（首次运行）: %v\n", err)
	}

	// 4. 创建数据提供者
	provider := marketdata.NewHybridProvider(cfg.FugleAPIKey)

	// 5. 创建健康检查器
	healthChecker := monitoring.NewHealthChecker(monitor, provider, stateStore)

	// 6. 创建规则引擎
	ruleEngine := monitoring.NewRuleEngine(monitor)
	for _, rule := range monitoring.DefaultRules() {
		ruleEngine.RegisterRule(rule)
	}

	// 7. 启动所有监控组件
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("\n📡 启动监控组件...")

	// 启动健康检查
	go healthChecker.Start(ctx)

	// 启动规则引擎
	go ruleEngine.Start(ctx, stateStore)

	// 启动系统指标收集
	systemMetrics := monitoring.NewSystemMetrics(metricsCollector, monitor)
	go systemMetrics.Start(ctx)

	// 8. 模拟一些交易指标
	fmt.Println("\n📊 模拟交易指标...")
	go simulateTrading(ctx, tradingMetrics, monitor)

	fmt.Println("\n✅ 监控系统已启动")
	fmt.Println("按 Ctrl+C 停止")
	fmt.Println()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\n🛑 正在停止监控系统...")
	cancel()
	time.Sleep(1 * time.Second)

	// 打印最终统计
	fmt.Println("\n📈 监控统计:")
	history := monitor.GetHistory(10)
	for _, alert := range history {
		fmt.Printf("  [%s] %s: %s\n", alert.Timestamp.Format("15:04:05"), alert.Level.String(), alert.Message)
	}

	fmt.Println("\n🎉 监控系统测试完成")
}

func simulateTrading(ctx context.Context, tradingMetrics *monitoring.TradingMetrics, monitor *monitoring.Monitor) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	orderCount := 0
	cash := 1000000.0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			orderCount++

			// 模拟订单
			order := domain.Order{
				Symbol:   "0050",
				Side:     domain.SideBuy,
				Quantity: 100,
				Price:    75.5,
				Reason:   "test",
			}

			tradingMetrics.RecordOrder(order, "filled")

			// 减少现金
			cash -= order.Price * float64(order.Quantity)

			// 记录投资组合
			tradingMetrics.RecordPortfolio(cash, cash+50000)

			// 发送信息告警
			monitor.Info("trading", fmt.Sprintf("Order executed: %s %d @ %.2f", order.Symbol, order.Quantity, order.Price), map[string]interface{}{
				"order_count": orderCount,
				"cash":        cash,
			})

			// 每5个订单发送一个警告测试
			if orderCount%5 == 0 {
				monitor.Warning("trading", "Order frequency high", map[string]interface{}{
					"order_count": orderCount,
				})
			}
		}
	}
}
