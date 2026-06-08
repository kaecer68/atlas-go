// Package scheduler 擴充:策略自我進化背景任務。
//
// 本檔提供 StrategyEvolutionTaskFunc 工廠函式,將
// `internal/orchestrator.StrategyEvolver.Evaluate` 包裝成
// `apigateway.BackgroundTaskManager` 可消費的 `func(context.Context) error`。
//
// 設計理由:
//   - `StrategyEvolver` 已存在於 `internal/orchestrator`,但只在 regime
//     變化時被動觸發(由 orchestrator 內部 pipeline 驅動)。系統在 CALIBRATING
//     階段的「未變化」天數中,evolver 永遠不會被呼叫,導致
//     `currentState` 與 `history` 長期停留在初始化值。
//   - 本任務提供「每日主動觸發」路徑,即使 macro/structural 沒有變化也會
//     重新評估一次,確保 cooldown 倒數與 history 持續推進。
//   - 任務本質是「審計式觸發」,實際決策仍由 system pipeline 主導,故
//     結果僅寫入 logging,不下達任何 portfolio 變更。
//
// 成熟度閘門 (maturity gate):
//   - BURN_IN: 跳過(避免冷啟動期的 false signal 污染 history)。
//   - CALIBRATING/FULL_AUTO: 正常執行。
//
// 失敗處理:交由 main.go 的 `taskMgr.SetFailureHandler` 統一處理
// (見 internal/apigateway/CONSTITUTION.md Art. 4.5.6)。
package scheduler

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// StrategyEvolutionDeps 集中注入策略進化任務的依賴。
//
// 設計原則:工廠於啟動期一次性構造 StrategyEvolver / SectorDataProvider
// 與三個評估引擎(均無狀態),並透過 closure 在每次 BTM 觸發時重用。
// StrategyEvolver 的 history + cooldown 因此得以跨呼叫累積,
// 不會因每日新實例而重置。
//
// SectorDataDir 為 sector_data.json 所在目錄(由 marketdata.SectorDataProvider
// 讀取);檔案不存在時 provider 會回傳零值 snapshot,實作 graceful degradation,
// 不會讓 BTM 因缺少 sector 資料而失敗。
//
// MaturityTracker 為選填;nil 表示不啟用閘門(供測試或單機除錯使用)。
// 生產環境建議注入以避免 BURN_IN 期間污染 evolver.history。
type StrategyEvolutionDeps struct {
	Dashboard       *monitoring.DashboardAPI
	SectorDataDir   string
	MaturityTracker *domain.MaturityTracker
}

// StrategyEvolutionTaskFunc 構造 BTM 可消費的 closure。
//
// 呼叫端需確保 deps.Dashboard 與 deps.MaturityTracker 已在執行期就緒
// (即 dashboard 與 maturityTracker 皆已透過 builder 初始化)。
// 若於 BurnIn 期間執行,本函式僅記錄 burn_in_skip 並回傳 nil,
// 不視為失敗(對齊 internal/scheduler/auto_calibration.go 行為)。
func StrategyEvolutionTaskFunc(deps StrategyEvolutionDeps) func(context.Context) error {
	// Closure-scoped singletons: 跨呼叫保留 evolver 狀態(history + cooldown),
	// 並避免每 24h 重新建構無狀態引擎。
	evolver := orchestrator.NewStrategyEvolver()
	sectorProvider := marketdata.NewSectorDataProvider(deps.SectorDataDir)
	macroEngine := narrative.NewMacroRiskAssessmentEngine()
	structuralEngine := narrative.NewStructuralTrendEngine()
	drawdownEngine := risk.NewMacroAwareDrawdownEngine()

	return func(ctx context.Context) error {
		// Maturity gate: BURN_IN 期間直接跳過,避免冷啟動 false signal
		// 寫入 evolver.history。
		if deps.MaturityTracker != nil {
			m := deps.MaturityTracker.Current()
			if m == domain.MaturityBurnIn {
				logging.Info("strategy_evolution", "burn_in_skip",
					"maturity", string(m),
				)
				return nil
			}
		}

		if deps.Dashboard == nil {
			return fmt.Errorf("strategy_evolution: dashboard dependency is required")
		}

		// 1. Macro 攝入:dashboard 會內部呼叫各 MacroDataProvider 並更新
		//    內部狀態,同時回傳最新 snapshot 給本任務。
		_, macroSnapshot, err := deps.Dashboard.IngestAndUpdateMacro(ctx)
		if err != nil {
			return fmt.Errorf("strategy_evolution: ingest macro: %w", err)
		}

		// 2. Sector 攝入:sectorProvider 對單一 JSON 檔做 graceful degradation;
		//    缺少檔案時回傳零值 snapshot,本任務不視為錯誤。
		//    對應 internal/orchestrator/system.go:1592-1597 的欄位映射。
		sectorSnap, _ := sectorProvider.FetchSnapshot(ctx)
		sectorData := narrative.SectorDataSnapshot{
			AIRevenueGrowth:    sectorSnap.TSMCRevenue.Value,
			CoWoSUtilization:   sectorSnap.CoWoSUtilization.Value,
			CapexGrowth:        sectorSnap.CapexGrowth.Value,
			SemiconductorIndex: sectorSnap.SOXIndex.Value,
		}

		// 3-4. Pipeline 評估: macro risk → structural trend → drawdown。
		macroAssessment := macroEngine.Assess(macroSnapshot)
		structuralAssessment := structuralEngine.Assess(macroSnapshot, sectorData)
		drawdownDecision := drawdownEngine.Evaluate(macroAssessment, structuralAssessment)

		// 5. StrategyEvolution: 內部檢查 cooldown + emergency bypass。
		//    回傳 nil 表示無狀態變更(可能是被 cooldown 抑制或評估結果
		//    與 currentState 一致),不視為錯誤。
		evolution := evolver.Evaluate(macroAssessment, structuralAssessment, drawdownDecision)
		if evolution == nil {
			logging.Info("strategy_evolution", "no_change",
				"state", evolver.GetCurrentState().String(),
			)
			return nil
		}

		logging.Info("strategy_evolution", "evolved",
			"from", evolution.FromState.String(),
			"to", evolution.ToState.String(),
			"reason", evolution.Reason,
		)
		return nil
	}
}
