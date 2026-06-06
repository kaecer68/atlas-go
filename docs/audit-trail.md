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
