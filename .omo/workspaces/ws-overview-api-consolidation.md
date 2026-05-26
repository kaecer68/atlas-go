# Workspace: overview 頁面 API 整併（loadAll → decision-chain）

## 目標

將 `web/static/js/main.js` 中 `loadAll()` 函數的 27 個並行 API 呼叫減少，利用已完成的 W3 聚合端點 `/api/dashboard/decision-chain` 取代部分重疊呼叫。

## 背景

- W3 完成後，`/api/dashboard/decision-chain` 一次回傳：
  - `events.today` + `events.recent`（覆蓋 `/api/narrative/events`）
  - `sector_heatmap`（覆蓋 `/api/dashboard/industry-overview` 的部分數據）
  - `recommendations`（覆蓋 `/api/dashboard/recommendation-pipeline` 的部分數據）

- 目前 `loadAll()` 對這些 API 有獨立呼叫。整併後可減少 3-5 個 API 呼叫。

## ⚠️ 關鍵注意事項

1. **loadAll() 是全域自動刷新核心**：每 30 秒呼叫一次，影響所有頁面模組。
2. **現有模組依賴特定 API 回應格式**：`m.dash.renderOverview()`、`m.pipe.renderPipeline()`、`m.risk.renderRiskCards()` 等的參數簽名不能改變。
3. **不可破壞 overview 頁面**：overview 頁面是系統登陸頁，必須確保所有卡片正常顯示。
4. **先讀後改**：必須先完整理解 `loadAll()` 中每個 API 回應被哪些模組如何消費，才能決定哪些可以安全替換。

## 執行步驟

### Phase 1: 依賴分析（唯讀）

1. 追蹤 `loadAll()` 中 27 個 API 的每一個如何被消費：
   - `results[0]` (system-health) → ?
   - `results[1]` (macro-radar) → `m.dash.renderMacroRadar` + `m.risk.renderRiskCards`
   - `results[2]` (agent-observatory) → `m.dash.renderOverview`, `m.dash.renderAgentObservatory`
   - ...（完整列出 27 個）

2. 對比 decision-chain 的回應欄位與現有 API 回應欄位，確認可直接替換的欄位。

### Phase 2: 漸進替換

1. **只替換確認安全的 API**：先用 decision-chain 取代 `/api/narrative/events`（風險最低，欄位完全一致）
2. **逐個測試**：每替換一個就驗證 overview 頁面無異常
3. **保留向後相容**：若某個 API 的額外欄位仍被其他模組使用，保留該 API 呼叫

### Phase 3: 性能驗證

```bash
# 比較前後的 API 呼叫數與載入時間
curl -w "\nTime: %{time_total}s\n" localhost:8080/api/dashboard/decision-chain
```

## 不可碰

- `internal/portfolio/`、`internal/orchestrator/`、`internal/live/`、`internal/eventlogic/`
- `web/static/js/modules/` 中的共享邏輯（除非確認無副作用）
- 現有模組的 render 函數簽名

## 驗收標準

- [ ] overview 頁面所有卡片正常顯示
- [ ] API 呼叫數從 27 減少至 ≤ 22
- [ ] 30 秒自動刷新正常運作
- [ ] `go build ./...` ✅
- [ ] 無 JS console error
