# Atlas Dashboard — Option 3 Phase 3B 設計書：組合持倉總覽

> 日期：2026-04-25
> 範圍：新增後端 API + 前端頁面 — 組合持倉總覽 + 淨值曲線
> 依賴：Option 2（前端重組）已完成

---

## 一、目標

為投資研究者提供一個「組合持倉總覽」頁面，回答三個核心問題：
1. **我現在持有什麼？** — 持倉明細（符號、數量、成本、現價、市值、損益）
2. **我賺賠多少？** — 累計損益、目前回撤、持倉數
3. **歷史表現如何？** — 淨值曲線圖

---

## 二、後端 API 設計

### 2.1 新端點

```
GET /api/dashboard/portfolio-state
```

### 2.2 回應格式

```json
{
  "snapshot_time": "2026-04-24T18:00:00Z",
  "cash": 1000000.0,
  "portfolio_value": 1250000.0,
  "cumulative_pnl": 250000.0,
  "cumulative_pnl_pct": 0.25,
  "current_drawdown": 0.03,
  "positions_count": 5,
  "positions": [
    {
      "symbol": "2330",
      "quantity": 10,
      "average_cost": 580.0,
      "current_price": 610.0,
      "market_value": 61000.0,
      "unrealized_pnl": 3000.0,
      "pnl_pct": 0.0517
    }
  ],
  "equity_curve": [
    {"date": "2026-04-01", "value": 1000000.0},
    {"date": "2026-04-02", "value": 1015000.0}
  ]
}
```

### 2.3 資料來源

| 欄位 | 來源 | 說明 |
|------|------|------|
| `current_drawdown` | 暫設為 0 | `live.PortfolioState` 無此欄位，待後續從 session history 計算 |
| `portfolio_value` | 計算：cash + sum(positions[].market_value) | |
| `cumulative_pnl` | `live.PortfolioState.CumulativePnL` 或計算：portfolio_value - initial_cash | |
| `positions[].pnl_pct` | 計算：unrealized_pnl / (average_cost * quantity)，quantity=0 時回傳 0 | |
| `equity_curve` | `buildEquityCurve()` 從 session summaries 建構 | 目前僅回傳最新 session 的單一點（需 ≥2 點才顯示圖表）；後續可從多個 session 的 PortfolioValue 建構完整曲線 |
| `snapshot_time` | 最新 session 的 recorded_at 或 time.Now() | |

**資料取得策略**：
1. 優先從 `live.LoadLastPortfolioState()` 讀取（已有此函式，在 `handleLiveStatus` 中使用）
2. 若 live state 不可用（無資料或解析失敗），回傳空持倉 + 空曲線，HTTP 200（非錯誤）
3. equity_curve 若為 `[]float64` 無日期，前端改用「Day N」作為 X 軸標籤

### 2.4 既有型別處理策略

**原則：不修改 `domain.Position` 和 `domain.SimulationState` 的 JSON tags。**

理由：
- 這些型別被 ledger JSONL 寫入、session 序列化等多處使用
- 新增 JSON tags 可能改變既有序列化行為（即使只是補上 snake_case）
- 使用 DTO 轉換更安全，且 API 回應可包含計算欄位（如 `pnl_pct`）

**Handler 實作方式**：
```go
func (a *DashboardAPI) handlePortfolioState(w http.ResponseWriter, r *http.Request) {
    // 1. 從 live state 讀取
    state, err := live.LoadLastPortfolioState()
    if err != nil || state == nil {
        writeJSON(w, http.StatusOK, PortfolioStateResponse{}) // 空回應
        return
    }

    // 2. 轉換為 DTO
    positions := make([]PositionDTO, len(state.Positions))
    for i, p := range state.Positions {
        cost := float64(p.Quantity) * p.AverageCost
        pnlPct := 0.0
        if cost > 0 {
            pnlPct = p.UnrealizedPnL / cost
        }
        positions[i] = PositionDTO{
            Symbol: p.Symbol, Quantity: p.Quantity,
            AverageCost: p.AverageCost, CurrentPrice: p.CurrentPrice,
            MarketValue: p.MarketValue, UnrealizedPnL: p.UnrealizedPnL,
            PnlPct: pnlPct,
        }
    }

    // 3. 產生 equity_curve
    curve := make([]EquityCurvePoint, len(state.EquityCurve))
    for i, v := range state.EquityCurve {
        curve[i] = EquityCurvePoint{Date: fmt.Sprintf("Day %d", i+1), Value: v}
    }

    // 4. 計算加總
    portfolioValue := state.Cash
    for _, p := range state.Positions { portfolioValue += p.MarketValue }
    cumulativePnL := portfolioValue - state.Cash // 假設初始現金 = state.Cash（需確認）

    writeJSON(w, http.StatusOK, PortfolioStateResponse{
        Cash: state.Cash, PortfolioValue: portfolioValue,
        CumulativePnL: cumulativePnL, CurrentDrawdown: state.CurrentDrawdown,
        PositionsCount: len(positions), Positions: positions,
        EquityCurve: curve,
    })
}
```

### 2.5 新 DTO 型別

```go
// PortfolioStateResponse 是 /api/dashboard/portfolio-state 的回應結構
type PortfolioStateResponse struct {
    SnapshotTime   time.Time         `json:"snapshot_time"`
    Cash           float64           `json:"cash"`
    PortfolioValue float64           `json:"portfolio_value"`
    CumulativePnL  float64           `json:"cumulative_pnl"`
    CumulativePnLPct float64         `json:"cumulative_pnl_pct"`
    CurrentDrawdown float64          `json:"current_drawdown"`
    PositionsCount int               `json:"positions_count"`
    Positions      []PositionDTO     `json:"positions"`
    EquityCurve    []EquityCurvePoint `json:"equity_curve"`
}

type PositionDTO struct {
    Symbol       string  `json:"symbol"`
    Quantity     int     `json:"quantity"`
    AverageCost  float64 `json:"average_cost"`
    CurrentPrice float64 `json:"current_price"`
    MarketValue  float64 `json:"market_value"`
    UnrealizedPnL float64 `json:"unrealized_pnl"`
    PnlPct       float64 `json:"pnl_pct"`
}

type EquityCurvePoint struct {
    Date  string  `json:"date"`
    Value float64 `json:"value"`
}
```

### 2.6 Handler 實作要點

- 從 `handleLiveStatus` 的既有邏輯中複用 `live.LoadLastPortfolioState()` 或 `live.LoadLastPositions()`
- 如果 live state 不可用，fallback 到最新 session summary
- equity_curve 是 `[]float64`，需要搭配 session dates 產生 `EquityCurvePoint[]`
- 註冊路由：在 `RegisterRoutes` 中新增 `mux.HandleFunc("/api/dashboard/portfolio-state", a.handlePortfolioState)`

---

## 三、前端頁面設計

### 3.1 側邊欄位置

在「風控結果」之後、「AI 觀測台」之前插入：

```
📊 總覽
🌍 宏觀敘事
🏭 產業生態系
📋 投資管線
🔗 決策鏈
🛡️ 風控結果
📂 組合持倉    ← 新增
🤖 AI 觀測台
...
```

### 3.2 頁面結構

```html
<div id="portfolio" class="page">
  <h1 id="pageTitle">📂 組合持倉</h1>

  <!-- KPI 摘要列 -->
  <div class="kpi-grid" id="portfolioKPIs"></div>

  <!-- 淨值曲線 -->
  <div class="panel wide" id="equityCurvePanel">
    <h2>淨值曲線</h2>
    <canvas id="equityChart" height="200"></canvas>
  </div>

  <!-- 持倉明細表 -->
  <div class="panel wide" id="positionsPanel">
    <h2>持倉明細</h2>
    <div id="positionsTable"></div>
  </div>

  <!-- 幫助文字（收合） -->
  <details class="help-details">
    <summary><strong>📖 如何解讀本頁</strong></summary>
    <p>顯示最新模擬場次的持倉狀態與淨值變化。損益為未實現損益，實際損益需待部位平倉後確認。</p>
  </details>
</div>
```

### 3.3 KPI 卡片

| 卡片 | 欄位 | 格式化 |
|------|------|--------|
| 總資產 | `portfolio_value` | `$#,###` |
| 可用現金 | `cash` | `$#,###` |
| 累計損益 | `cumulative_pnl` | `+/-$#,###`（紅綠色） |
| 目前回撤 | `current_drawdown` | `+/-X.X%` |
| 持倉數 | `positions_count` | `N 檔` |

### 3.4 淨值曲線圖

- 使用 Canvas 2D 繪製（不引入外部圖表庫）
- X 軸：日期（從 equity_curve[].date）
- Y 軸：淨值（從 equity_curve[].value）
- 線條顏色：上漲用 `var(--up)`，下跌段用 `var(--down)`（可選）
- 如果沒有 equity_curve 資料，顯示「尚無歷史淨值資料」

### 3.5 持倉明細表

| 欄位 | 來源 | 格式化 |
|------|------|--------|
| 符號 | `symbol` | 靠左 |
| 數量 | `quantity` | 整數 |
| 成本 | `average_cost` | `#,###.##` |
| 現價 | `current_price` | `#,###.##` |
| 市值 | `market_value` | `#,###` |
| 損益 | `unrealized_pnl` | `+/-#,###`（紅綠色） |
| 損益% | `pnl_pct` | `+/-X.XX%`（紅綠色） |

- 損益為正：綠色；為負：紅色
- 無持倉時顯示「目前無持倉」

### 3.6 JS 函式

```javascript
async function loadPortfolioState() {
  // GET /api/dashboard/portfolio-state
  // renderPortfolioKPIs(data)
  // renderEquityCurve(data.equity_curve)
  // renderPositionsTable(data.positions)
}

function renderPortfolioKPIs(data) {
  // 5 張 KPI 卡片
}

function renderEquityCurve(points) {
  // Canvas 2D 繪製
}

function renderPositionsTable(positions) {
  // 表格渲染，含紅綠色損益
}
```

### 3.7 switchPage 整合

- 在 `switchPage()` 的 `titles` 物件新增 `portfolio: '組合持倉'`
- 在 `loadPageData()` 的 switch-case 新增 `'portfolio': loadPortfolioState()`

---

## 四、改動檔案清單

| 檔案 | 改動類型 | 說明 |
|------|----------|------|
| `internal/monitoring/dashboard_api.go` | 新增 | `handlePortfolioState` + DTO 型別（`PortfolioStateResponse`, `PositionDTO`, `EquityCurvePoint`）+ 註冊路由 |
| `web/static/index.html` | 新增 | 側邊欄連結 + portfolio 頁面 HTML + CSS + JS（`loadPortfolioState`, `renderPortfolioKPIs`, `renderEquityCurve`, `renderPositionsTable`） |

**不改動既有 domain 型別** — 使用 DTO 轉換，避免影響 ledger JSONL 序列化。

---

## 五、風險與注意事項

| 風險 | 預防 |
|------|------|
| live state 不可用 | fallback 回傳空回應（HTTP 200），UI 顯示「尚無持倉資料」 |
| equity_curve 資料不足 | 目前僅有單一點，Canvas 圖表需 ≥2 點才顯示；面板自動隱藏 |
| `live.PortfolioState` 無 `current_drawdown` | 暫回傳 0，待後續從 session history 計算 |
| Canvas 圖表在暗色主題下不明顯 | 使用 CSS 變數 `var(--accent)` 作為線條顏色 |
| `live.LoadLastPortfolioState()` 回傳的結構可能與預期不同 | 已確認：Cash, TotalExposure, AvailableCash, DayPnL, UnrealizedPnL, RealizedPnL, LastUpdated |

---

## 六、驗證標準

- [ ] `GET /api/dashboard/portfolio-state` 回傳正確 JSON
- [ ] `go build ./cmd/atlas/...` 通過
- [ ] `go test ./...` 通過
- [ ] 側邊欄出現「📂 組合持倉」連結
- [ ] 點擊後顯示 KPI 卡片、淨值曲線、持倉明細表
- [ ] 無持倉時顯示適當空狀態
- [ ] 瀏覽器重新整理（Cmd+Shift+R）後可見所有改動
