package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	cfg := config.Load()
	info, err := os.Stat(cfg.ReplayDataPath)
	if err != nil {
		return fmt.Errorf("無法存取 replay 數據: %w", err)
	}

	ds, err := replay.LoadTWSEOpenDataCSV(cfg.ReplayDataPath)
	if err != nil {
		return fmt.Errorf("無法載入 replay 數據: %w", err)
	}

	if len(ds.Dates) == 0 {
		return fmt.Errorf("replay 數據為空")
	}

	latestDate := ds.Dates[len(ds.Dates)-1]
	earliestDate := ds.Dates[0]
	daysDelayed := int(time.Since(latestDate).Hours() / 24)

	fmt.Println("[數據健康檢查]")
	fmt.Printf("✅ CSV 數據檔案存在: %s\n", cfg.ReplayDataPath)
	fmt.Printf("✅ 數據筆數: %d 個交易日\n", len(ds.Dates))
	fmt.Printf("✅ 日期範圍: %s 至 %s\n", earliestDate.Format("2006-01-02"), latestDate.Format("2006-01-02"))
	fmt.Printf("✅ 檔案最後修改: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))

	if daysDelayed > 2 {
		fmt.Printf("⚠️  數據延遲: %d 天（最後日期：%s）\n", daysDelayed, latestDate.Format("2006-01-02"))
		fmt.Println("   建議執行: go run ./cmd/daily-replay-sync")
	} else {
		fmt.Printf("✅ 數據延遲: %d 天（正常範圍內）\n", daysDelayed)
	}

	return nil
}
