# Atlas 投資人 UI — 實作路線圖、基準比較、情境匹配

**版本**: 1.0  
**日期**: 2026-06-02  
**成熟度**: X（experimental）  
**父技能**: `atlas-investor-ui`

---

## 一、描述

本技能規範 Atlas 投資人 UI 的**實作階段、基準比較系統（BenchmarkProvider）、歷史情境匹配層、以及驗證要求**。為 `atlas-investor-ui` 的子技能，聚焦實作路線而非架構。

---

## 二、實作階段

### Phase A：建立信任基礎（P0 — 必須最先做）

| 優先級 | 項目 | 產出 | 依賴 | 子技能 |
|--------|------|------|------|--------|
| P0 | 投資人儀表板 v1 | `./client_web/index.html` | 無 | `atlas-investor-pages` |
| P0 | 歷史績效追蹤 | 累積報酬曲線 vs TAIEX/0050、Sharpe 時間序列 | ledger outcomes API | — |
| P0 | 每日盤前摘要 | API endpoint + 儀表板（基於 `report_generator.go`） | narrative | — |
| P1 | 推薦命中率追蹤 | 推薦記錄 → N 天後對比 → 命中率儀表板 | ledger + ForwardReturn | — |
| P1 | NLG 推薦解釋 | FactorScoreBreakdown → 繁體中文 | `nlg_templates.go` 擴展 | `atlas-investor-nlg` |

### Phase B：強化投資人信任（P1-P2）

| 優先級 | 項目 | 產出 | 依賴 | 子技能 |
|--------|------|------|------|--------|
| P1 | 信任分數系統 | TrustScore 模組 | 多模組整合 | `atlas-investor-trustscore` |
| P1 | 基準比較 | BenchmarkProvider（TAIEX/0050/台灣 50） | marketdata | 見 §三 |
| P2 | ETF 分析強化 | 折溢價/NAV/追蹤誤差 | etf-rotation executor | — |
| P2 | 情境模擬 | What-if 分析 | PortfolioSimulator | — |
| P2 | 歷史情境匹配 | Layer 1.5（宏觀 → 個股行為） | janus + narrative + ledger | 見 §四 |
| P2 | 紙上交易追蹤 | Paper trading 公開記錄 | Phase A 全部完成 | — |

### Phase C：不在本次範圍

- 投資人 AI Agent 技能（`atlas-investor-advisor`）
- MCP Tools（自然語言查詢 Atlas）
- 每月透明度報告自動生成

---

## 三、基準比較系統（BenchmarkProvider）

### 為何需要

投資人不會單看「組合賺 3%」，會問「大盤賺多少？」。

### 基準指數（優先級）

1. TAIEX（加權指數）— 全市場基準
2. 0050.TW（元大台灣 50）— 大型股基準
3. 0056.TW（元大高股息）— 高股息策略對標
4. 台灣 50 指數 — 更精確的市值加權基準

### 資料來源

`internal/marketdata/` provider（TWSE OpenAPI → FinMind → Fugle）

### 核心方法

```go
type BenchmarkProvider interface {
    GetTAIEXReturns(from, to time.Time) ([]DailyReturn, error)
    GetETFReturns(symbol string, from, to time.Time) ([]DailyReturn, error)
    GetTrackingError(portfolioReturns []DailyReturn, benchmark string) (float64, error)
}
```

### 實作位置

可擴展現有 `internal/marketdata/` 或新增 `internal/benchmark/`（Maturity: X）。

---

## 四、歷史情境匹配層（Layer 1.5）

### 為何需要

Atlas 的宏觀 → 選股鏈是**戰略式**（調整因子權重、調整環境）+ **戰術式**（產業桌挑個股）。

缺的是**反向驗證迴路**：宏觀判斷「外資要撤」→ 過去類似情境下，哪些股票表現好/差？→ 直接建議調整倉位。

### 插入位置

```
Layer 0: 宏觀數據攝入（現有）
Layer 1: Context Agents（現有）
    ↓
Layer 1.5: 歷史情境匹配層 ← 🆕
  「現在的美債/VIX/外資組合，過去 5 年出現過 12 次」
  「9 次台積電 5 天均報酬 -2.3%，8 次黃金 ETF +1.1%」
    ↓
Layer 2: 產業層（現有，多了歷史情境輸入）
```

### 資料來源（已存在，需串聯）

| 模組 | 現有能力 | 需擴展 |
|------|----------|--------|
| `janus/engine.go` | 跨 cohort regime 對比 | 擴展為「找出歷史上相似的 macro snapshot」 |
| `narrative/knowledge_base.go` | 模板匹配 | 擴展為「匹配後輸出 specific stock-level 行為模式」 |
| `ledger/` | 所有歷史 outcome | 查詢「regime=X, narrative=Y → agent 表現」 |

### 前端呈現

在推薦詳情頁（`atlas-investor-pages` §頁面 2）的「歷史類似情境」區塊展示。

---

## 五、驗證要求

### 實作後驗證清單

- [ ] `./client_web/index.html` 在瀏覽器中正常渲染
- [ ] 所有 `/api/client/*` 端點回傳正確格式
- [ ] 累積報酬曲線數據與 `ledger` outcomes 一致
- [ ] 推薦命中率計算與 ForwardReturn 對比正確
- [ ] NLG 輸出為繁體中文，無技術術語
- [ ] 信任分數計算邏輯可追溯（每個子分數有明確資料來源）
- [ ] fallback 數據都有 UI 標記
- [ ] 頁面載入時間 < 2 秒

### GitNexus 驗證

- 修改後執行 `gitnexus_detect_changes()` 確認變更僅限 `web/client_web/` + 新增 API handler
- 不得意外影響 `internal/orchestrator/`、`internal/sim/`、`internal/portfolio/` 核心模組

### CI 檢查

```bash
go build ./...
go test ./...
gofmt -l .
staticcheck ./...
```

---

## 六、與其他技能關聯

| 技能 | 關聯 |
|------|------|
| `atlas-investor-ui` | 父技能，定義架構與設計原則 |
| `atlas-investor-pages` | 頁面實作依據 |
| `atlas-investor-nlg` | Phase A P1 項目 |
| `atlas-investor-trustscore` | Phase B P1 項目 |
| `atlas-core-architecture` | 理解架構避免破壞現有模組 |
| `atlas-macro-narrative` | 歷史情境匹配依賴 narrative 模組 |
