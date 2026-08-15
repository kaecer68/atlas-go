# 決策鏈透明化（Audit Trail）

系統已實作三階段透明度機制，將後端決策鏈的完整計算過程攤開在「決策鏈」前端頁面。

## 第一階段：個股因子分數透明化

- `FactorScores`（含 `Breakdown *FactorScoreBreakdown`）附加於每筆 `Recommendation` 與 `ScreeningReject`
- 每因子含：`Score`、`Weight`、`Formula`、`RawInputs`、`IsFallback`
- 實作：`internal/portfolio/factor_engine.go` 的 `CalculateAllScoresWithBreakdown()`
- 觸發時機：`collectRecommendations()`（`internal/orchestrator/executors.go`）

## 第二階段：行業信念計算透明化

- `ConvictionBreakdown`（含 `Base`/`Floor`/`Final` 與 `Steps[]`）附加於每筆 `Recommendation`
- 每步含：`Rule`、`Delta`、`Reason`
- 實作：`internal/orchestrator/conviction_builder.go` 的 `convictionBuilder`

## 第三階段：宏觀事件信心度透明化

- `NarrativeEvent`（`internal/narrative/types.go`）新增 `ConfidenceSource` 與 `HitRate`
- 內建命中率：`US_rates_up: 0.72`、`JPY_carry_unwind: 0.68`、`geopolitical_risk: 0.65`、`AI_capex_surge: 0.81`

## 資料流驗證

- API `/api/dashboard/recommendation-pipeline` 回傳含完整 breakdown 的 items
- `screened_items[].factor_scores` 含被篩選標的之因子分數

## 決策鏈進化 v2（ConvictionStep 參數溯源）

決策鏈的三個穩定階段依序是「因子 → 信念 → 敘事」：第一階段揭露每個因子的分數、權重、公式與原始輸入；第二階段以 `ConvictionBreakdown` 記錄基礎信念、floor、final 及每個調整步驟；第三階段揭露宏觀事件的主題、信心度與命中率。v1 已能回答「算出什麼」，但 config-driven 的 `delta` 仍只能從結果反推來源，因此 v2 在不改變三階段與計算邏輯的前提下，加入逐步參數溯源，讓決策可稽核、可比較，也可依保存的參數快照重播。

- **v1**：`ConvictionStep` 只有 `Rule`、`Delta`、`Reason`；`factor`、hardcoded fallback 與 runtime heuristic 的差異無法由同一份 API payload 判定。
- **v2**：保留既有三欄，新增 `Source`、`ParamRef`、`ParamValue`、`Sensitivity`；`addWithProvenance(rule, delta, reason, source, paramRef, paramValue)` 同時更新 `ConvictionBreakdown.Final`。`RecommendationOutcome` 並可附帶 `ParameterSnapshot`。

### ConvictionStep 參數溯源表

| 參數名稱 | 來源模組 | 計算公式 | 預設值 | 可調範圍 |
|---|---|---|---|---|
| `NarrativeConviction.ThemeHitRates.<theme>` | `internal/config/defaults_narrative.go` 的 `defaultNarrativeConvictionParameters`；`narrative_conviction_modulator.go` | 先取已設定且非零的 theme hit rate，否則回退 `NarrativeEvent.HitRate`；`delta = round(10 × hitRate)` | `AI_capex_surge=0.81`、`US_rates_up=0.72`、`JPY_carry_unwind=0.68`、`geopolitical_risk_spike=0.65`、`oil_price_shock=0.58` | 語意範圍為 `0–1`；runtime 沒有 clamp 或驗證，越界值仍會被接受 |
| `Industry.PhaseScores.ScoreExpansion` / `ScoreRecovery` / `ScoreMature` / `ScoreRecession` | `defaultIndustryParameters`；`industry_cycle_modulator.go:phaseDelta`、`IndustryCycleModulator.CollectModulationSteps` | `phaseDelta = round(phaseScore)`；`adj = round(phaseDelta × CyclePosition.Confidence)`；最終信念再加總各 modulator 的 `adj` | `20 / 10 / 0 / -20` | 沒有 runtime 範圍驗證；值以 conviction points 解讀，應以整數或近似整數調校並由回測／觀察期驗證 |
| `Source` | `convictionBuilder.addWithProvenance` 及各 modulator | 不參與計算；config-driven step 標示 `config`、`hardcoded` 或 `heuristic` | 空字串，JSON `omitempty` | provenance 步驟主要使用 `config｜hardcoded｜heuristic`；其他 runtime step 可用 `CycleStatusCard` 等模組識別字，新增類型需同步前端標示與測試 |
| `ParamRef` | `convictionBuilder.addWithProvenance` | 寫入完整參考路徑，例如 `NarrativeConviction.ThemeHitRates.AI_capex_surge` | 空字串，JSON `omitempty` | 無固定格式驗證；非空值應指向唯一、可由 config 定位的參數 |
| `ParamValue` | 產生 `ConvictionStep` 的 config-driven modulator | 以字串保存本次實際使用的值；phase score 以 `%.0f`、narrative hit rate 以 `%.2f` 顯示 | 空字串，JSON `omitempty` | 無數值範圍驗證；可供 UI 顯示，若可解析為 `float64` 才會產生 sensitivity marker |
| `Sensitivity` | `internal/orchestrator/industry_cycle_modulator.go:paramSensitivity` | `abs(paramValue × 0.1)`，表示參數變動 10% 的量級 | `nil`，無值時不序列化 | 僅在 `ParamValue` 可解析時存在；結果恆為非負數，沒有額外上限 |

### 資料流：因子、信念與敘事信心度

1. **因子 → 信念**：`FactorEngine.CalculateAllScoresWithBreakdown()` 為各因子保存 `Score`、`Weight`、`Formula`、`RawInputs` 與 fallback 狀態，再計算加權 total。`Agent` 因子的基礎值是 `weighted_avg(Conviction / 100)`；narrative、industry-cycle 與其他可用 provider 也可進 total。因子分數是可追溯證據，不直接替代 recommendation 的 `Conviction`。
2. **信念 → 敘事參與**：active `NarrativeEvent` 的 `Confidence × HitRate` 會形成 narrative factor score（多事件時為各事件乘積和並 clamp 到 1），再由 `NarrativeConvictionModulator` 依有效 hit rate 追加 `round(10 × hitRate)` 的 conviction step。`Confidence` 不再只存在於敘事頁，而是透過 narrative factor 與帶 provenance 的 step 參與推薦決策鏈。
3. **config 溯源 → snapshot**：`NarrativeConviction.ThemeHitRates.<theme>` 與 `Industry.PhaseScores.*` 的來源、參考路徑、實際值及 10% sensitivity 隨 step 保存；`ParameterSnapshot` 另外記錄 factor weights、narrative hit rates、industry phase scores、config version 與 `CapturedAt`，供 ledger/API/重播使用。

### 與既有三階段概述的關係

本文前述三階段仍是對外概念模型；v2 **不是新增第四階段**，而是替第二階段的 `ConvictionStep` 增加 provenance contract，並把第一、三階段的證據與當次參數版本連到同一個 recommendation outcome。舊 session 沒有新增欄位時仍可正常反序列化，API consumer 不應把空的 `source`、`param_ref`、`param_value` 或 `sensitivity` 視為錯誤。
