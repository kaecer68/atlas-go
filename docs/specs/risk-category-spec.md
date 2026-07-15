# Risk Category — MCP Tool Spec

> **狀態**: v1.0 (2026-07-15)
> **Source**: `cmd/atlas-mcp/server/tools_risk_alert.go`(與 Alert 共檔)
> **Catalog ref**: [`docs/reference/tool-catalog.md` §Risk](../reference/tool-catalog.md)
> **Audit gap fill**: Round 3a

## 1. 目的

對外暴露 4 層風險架構(pre-trade / in-trade / post-trade / portfolio)的主要 metric、VaR、drawdown、相關性矩陣、敘事 commentary。

## 2. Tool 清單(5 個)

| Tool | Description | Handler | Backend |
|------|-------------|---------|---------|
| `risk_get_metrics` | 風險聚合指標 | `handleRiskGetMetrics` | `GET /api/risk/metrics` |
| `risk_get_correlation_matrix` | 跨策略相關性矩陣 | `handleRiskGetCorrelationMatrix` | `GET /api/risk/correlation_matrix` |
| `risk_get_drawdown` | 當前/最大 drawdown | `handleRiskGetDrawdown` | `GET /api/risk/drawdown` |
| `risk_get_calibration` | 風險模型校準(predicted vs realized VaR) | `handleRiskGetCalibration` | `GET /api/risk/calibration` |
| `risk_get_commentary` | 風險敘事 commentary | `handleRiskGetCommentary` | `GET /api/risk/commentary` |

## 3. 規格重點

- **架構**: 4-tier 風險決策(綠/黃/橙/紅 + 結構性趨勢 override),詳見 [`atlas-risk-management` skill](../../.claude/skills/atlas-risk-management/SKILL.md)
- **VaR 計算法**: 歷史模擬法(`internal/risk/var_calculator.go`)
- **Drawdown 核心**: `internal/risk/macro_aware_drawdown.go`(回撤防護 + 宏觀感知決策)
- **Wave 9 DriftDetector** 在 `internal/monitoring/service/drift_detector.go` 暴露 drift 事件由 `risk_get_metrics` 一併鏡像
- **DestructiveHint**: 全部 false(read-only)

## 4. 已知限制

- VaR 95/99 在 regime 變動下 re-calibration 需要 30min hot reload,短時間波動期間回應可能 slightly stale
- `risk_get_calibration` 需至少 60 天資料否則 insufficient-sample

## 5. 測試

- `tools_risk_alert_test.go`
- e2e: `curl /api/risk/drawdown` 回 200 + `{current_drawdown, peak_drawdown, recovery}`

## 6. 變更歷史

| 版本 | 日期 | 變更 |
|------|------|------|
| v1.0 | 2026-07-15 | 初版 |
