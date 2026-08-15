// Package scheduler — Stage 5 PR#4 template_detector_scan BackgroundTask.
//
// Stage 1 .omo/audit/2026-07-12-atlas-event-flow-gap-plan.md PR#4 規劃了每 1h 掃描 trigger 條件的
// template_detector_scan 排程但從未實作。本檔落地：每 1h 呼叫
// narrative.DetectorRegistry.RunAll(),結果寫到 ledger.DetectorScanStore
// (不發 bus event 以避免污染 eventdriven 既有路徑)。
//
// Art.4 (internal/apigateway/CONSTITUTION.md) 強制:background task 須透過
// BackgroundTaskManager 註冊;呼叫端 (cmd/atlas) 負責 wiring。
package scheduler

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// RegisterTemplateDetectorScanTasks: fire-and-register convention (Register 的
// 重複名稱 error 刻意忽略,對齊 cmd/atlas/capital_tasks.go);per-detector
// 錯誤從 RunAll 直接 drop (不中止掃描),只有 store.AppendScan 錯誤傳到 manager。
func RegisterTemplateDetectorScanTasks(
	btm *apigateway.BackgroundTaskManager,
	registry *narrative.DetectorRegistry,
	store ledger.DetectorScanStore,
	macroProvider func() marketdata.MacroDataSnapshot,
	marketProvider func() narrative.MarketNarrativeData,
) {
	if btm == nil || registry == nil || store == nil {
		return
	}

	_ = btm.Register(&apigateway.ScheduledTask{
		Name:     "template_detector_scan",
		Interval: time.Hour,
		// Jitter 留 0,由 BackgroundTaskManager 合約自動設為 6min。
		Task: func(ctx context.Context) error {
			in := narrative.DetectorInput{Now: time.Now().UTC()}
			if macroProvider != nil {
				in.MacroSnapshot = macroProvider()
			}
			if marketProvider != nil {
				in.MarketData = marketProvider()
			}
			results, errs := registry.RunAll(ctx, in)
			_ = errs
			if len(results) == 0 {
				return nil
			}
			_, err := store.AppendScan(ctx, results)
			return err
		},
		Enabled: true,
	})
}
