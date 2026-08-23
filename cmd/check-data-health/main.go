package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// gapLookbackDays is the calendar-day window (excluding today) scanned for
// missing trading days. 14 days covers two full weeks, so a single missed
// trading day stays visible long enough for daily-replay-sync to backfill it.
// TWSE holidays not present in marketdata.IsTaiwanTradingDay are not enforced
// (假日未列不強制); an unknown holiday may surface as a missing day.
const gapLookbackDays = 14

func main() {
	cfg := config.Load()
	if err := run(cfg.ReplayDataPath, time.Now(), os.Stdout); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(dataPath string, now time.Time, out io.Writer) error {
	info, err := os.Stat(dataPath)
	if err != nil {
		return fmt.Errorf("無法存取 replay 數據: %w", err)
	}

	ds, err := replay.LoadTWSEOpenDataCSV(dataPath)
	if err != nil {
		return fmt.Errorf("無法載入 replay 數據: %w", err)
	}

	if len(ds.Dates) == 0 {
		return fmt.Errorf("replay 數據為空")
	}

	latestDate := ds.Dates[len(ds.Dates)-1]
	earliestDate := ds.Dates[0]
	daysDelayed := int(now.Sub(latestDate).Hours() / 24)

	// ── 缺日偵測：CSV 日期集合 vs 預期交易日 ──────────────────────
	// 預期交易日 = 往前 14 個日曆日（不含今天）中 marketdata 判定的
	// 台灣交易日（排除週六日與已知國定假日）。
	present := make(map[string]bool, len(ds.Dates))
	for _, d := range ds.Dates {
		present[d.Format("2006-01-02")] = true
	}
	var missing []string
	for i := gapLookbackDays; i >= 1; i-- {
		d := now.AddDate(0, 0, -i)
		if !marketdata.IsTaiwanTradingDay(d) {
			continue
		}
		key := d.Format("2006-01-02")
		if !present[key] {
			missing = append(missing, key)
		}
	}

	_, _ = fmt.Fprintln(out, "[數據健康檢查]")
	_, _ = fmt.Fprintf(out, "✅ CSV 數據檔案存在: %s\n", dataPath)
	_, _ = fmt.Fprintf(out, "✅ 數據筆數: %d 個交易日\n", len(ds.Dates))
	_, _ = fmt.Fprintf(out, "✅ 日期範圍: %s 至 %s\n", earliestDate.Format("2006-01-02"), latestDate.Format("2006-01-02"))
	_, _ = fmt.Fprintf(out, "✅ 檔案最後修改: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))

	if len(missing) > 0 {
		_, _ = fmt.Fprintf(out, "⚠️  缺日: %s\n", strings.Join(missing, ", "))
		_, _ = fmt.Fprintln(out, "   建議執行: go run ./cmd/daily-replay-sync（它會自動補抓缺日）")
	} else {
		_, _ = fmt.Fprintf(out, "✅ 缺日檢查: 近 %d 日無缺日\n", gapLookbackDays)
	}

	if daysDelayed > 2 {
		_, _ = fmt.Fprintf(out, "⚠️  數據延遲: %d 天（最後日期：%s）\n", daysDelayed, latestDate.Format("2006-01-02"))
		_, _ = fmt.Fprintln(out, "   建議執行: go run ./cmd/daily-replay-sync")
	} else {
		_, _ = fmt.Fprintf(out, "✅ 數據延遲: %d 天（正常範圍內）\n", daysDelayed)
	}

	if daysDelayed > 2 || len(missing) > 0 {
		return fmt.Errorf("replay 數據異常: 延遲 %d 天, 缺 %d 個交易日 (%s)", daysDelayed, len(missing), strings.Join(missing, ", "))
	}
	return nil
}
