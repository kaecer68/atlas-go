# Workspace: 擴充 premarket + W4 事件邏輯庫

## 目標

1. 擴充 `/api/dashboard/decision-chain` 的 `premarket` 區塊，加入 stress-index 和更多總經指標
2. 實作 W4 事件邏輯庫（`internal/eventlogic/` 模組）

## 背景

W4 定義在 `.omo/phase6/W4-event-logic-lib.md`，包含：
- 6 條種子規則（SOX+外資、美股收盤→台股開盤、DXY+BDI→航運、外資連賣→偏空、NVIDIA財報→AI鏈、USD/TWD>32→出口股）
- RuleRegistry + RuleValidator + SelfCorrector + PatternDetector
- 7 個 API 端點

目前 W3 decision-chain 的 `premarket` 區塊僅有基礎欄位（SOX、USD/TWD、外資淨買超、BDI）。需要補充：
- 台股壓力指數（TaiwanStressIndex）
- S&P 500 / NASDAQ 變化（目前僅有 SOX）
- 外資連續買賣超天數
- 融資餘額 Z-score

## 執行步驟

### Phase 1: 擴充 premarket（先做，風險低）

1. 修改 `internal/monitoring/api/decision/handlers.go`：
   - 在 `PremarketData` struct 加入 `StressIndex`、`SP500`、`NASDAQ`、`ConsecutiveDays` 等欄位
   - 在 premarket goroutine 中從 `MacroDataSnapshot` 提取更多指標
   - 若 provider 不支援則 graceful degradation（nil-safe）
2. 同步更新 `decision-chain.js` 前端渲染

### Phase 2: W4 事件邏輯庫（核心）

#### 2.1 基礎架構

在 `internal/eventlogic/` 下建立（**W3 已建立 `rule.go`、`registry.go` 基礎**）：

```
internal/eventlogic/
├── rule.go           # EventRule 定義（已存在）
├── registry.go       # RuleRegistry CRUD + 種子規則（已存在）
├── detector.go       # PatternDetector — 根據 market data 觸發規則
├── validator.go      # RuleValidator — 基於 forward return 驗證命中率
├── corrector.go      # SelfCorrector — 10失敗→degraded, 20→expired, 5命中→恢復
├── registry_test.go  # 測試
└── rule_test.go      # 測試（已存在）
```

#### 2.2 API Handler

在 `internal/monitoring/api/eventlogic/handlers.go`（已存在基礎）擴充：

```go
GET  /api/eventlogic/rules           # 全部規則（已存在）
GET  /api/eventlogic/rules/active    # 活躍規則（已存在）
GET  /api/eventlogic/rules/expired   # 過期規則（已存在）
GET  /api/eventlogic/rules/{id}      # 單一規則（已存在）
POST /api/eventlogic/rules/{id}/validate  # 手動驗證（已存在）
GET  /api/eventlogic/stats           # 統計（已存在）
POST /api/eventlogic/discover        # 自動發現新規則（已存在）
```

#### 2.3 6 條種子規則

```go
// 已在 NewRegistry() 的 seedRules() 中定義，驗證其完整性：
1. "sox_foreignflow_semiconductor" — SOX > +3% 且外資連續買超≥3日 → 半導體上漲
2. "us_close_tw_open_lag" — 美股收盤 → 台股開盤落後反應
3. "dxy_strong_bdi_shipping" — DXY強勢+BDI上升 → 航運股上漲
4. "foreign_outflow_bearish" — 外資連續賣超≥5日 → 市場偏空
5. "nvidia_ai_supply_chain" — NVIDIA財報超預期 → AI供應鏈上漲
6. "usdtwd_export_stocks" — USD/TWD > 32 → 出口股受益
```

#### 2.4 自我修正機制

```go
// SelfCorrector 邏輯：
// - 5 次連續命中 → 提升 confidence
// - 10 次連續失敗 → degraded 狀態
// - 20 次連續失敗 → expired 狀態
// - 從 degraded 恢復：5 次命中 → active
```

### Phase 3: 整合至 decision-chain

確保 W4 種子規則透過 `eventlogic.RuleRegistry.ListActive()` 正確流入 decision-chain 的 `logic_rules` 欄位（目前已有骨架，需確認資料完整性）。

## 不可碰

- `internal/portfolio/`、`internal/orchestrator/`、`internal/live/`
- W3 已有的 decision-chain 核心邏輯（僅擴充，不重構）

## 驗收標準

- [ ] Premarket 區塊新增 stress-index、S&P 500、NASDAQ
- [ ] 6 條種子規則在 API 回應中出現
- [ ] `curl localhost:8080/api/eventlogic/rules/active | jq '.total'` → 6
- [ ] `curl localhost:8080/api/dashboard/decision-chain | jq '.logic_rules | length'` → ≥ 6
- [ ] SelfCorrector 測試：模擬連續失敗後狀態正確轉換
- [ ] `go build ./...` ✅ `go test ./...` ✅
