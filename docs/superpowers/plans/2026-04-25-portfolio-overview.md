# 組合持倉總覽 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增「📂 組合持倉」頁面，顯示持倉明細、KPI 摘要、淨值趨勢，讓投資研究者一覽目前投資組合狀態。

**Architecture:** 新增後端 API handler 從 live state store 讀取持倉資料，組合為 DTO 回傳；前端新增側邊欄連結 + 頁面 HTML + JS 渲染函式。

**Tech Stack:** Go (net/http), HTML/CSS/JS (vanilla, Canvas 2D)

**Spec:** `docs/superpowers/specs/2026-04-25-portfolio-overview-design.md`

---

## File Structure

| 檔案 | 職責 |
|------|------|
| `internal/monitoring/dashboard_api.go` | 新增 `handlePortfolioState` handler + DTO 型別 + 註冊路由 |
| `web/static/index.html` | 新增側邊欄連結、portfolio 頁面 HTML、CSS、JS 渲染函式 |

---

### Task 1: 後端 DTO 型別定義

**Files:**
- Modify: `internal/monitoring/dashboard_api.go`（在檔案頂部型別定義區新增）

- [ ] **Step 1: 新增 DTO 型別**

在 `MacroRadarResponse` 型別定義之後（約 line 62 附近），新增以下三個 DTO 型別：

```go
// PortfolioStateResponse is the response for GET /api/dashboard/portfolio-state.
type PortfolioStateResponse struct {
	SnapshotTime    time.Time          `json:"snapshot_time"`
	Cash            float64            `json:"cash"`
	PortfolioValue  float64            `json:"portfolio_value"`
	CumulativePnL   float64            `json:"cumulative_pnl"`
	CumulativePnLPct float64           `json:"cumulative_pnl_pct"`
	CurrentDrawdown float64            `json:"current_drawdown"`
	PositionsCount  int                `json:"positions_count"`
	Positions       []PositionDTO      `json:"positions"`
	EquityCurve     []EquityCurvePoint `json:"equity_curve"`
}

// PositionDTO represents a single position with computed P&L percentage.
type PositionDTO struct {
	Symbol        string  `json:"symbol"`
	Quantity      int     `json:"quantity"`
	AverageCost   float64 `json:"average_cost"`
	CurrentPrice  float64 `json:"current_price"`
	MarketValue   float64 `json:"market_value"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
	PnlPct        float64 `json:"pnl_pct"`
}

// EquityCurvePoint is a single point on the equity curve.
type EquityCurvePoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}
```

- [ ] **Step 2: 驗證編譯**

Run: `go build ./cmd/atlas/...`
Expected: 無錯誤

---

### Task 2: 後端 Handler 實作

**Files:**
- Modify: `internal/monitoring/dashboard_api.go`

- [ ] **Step 1: 實作 handlePortfolioState handler**

在 `handleLiveStatus` 函式之後（約 line 323 之後），新增：

```go
func (a *DashboardAPI) handlePortfolioState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	resp := PortfolioStateResponse{
		SnapshotTime: time.Now().UTC(),
	}

	liveBasePath := filepath.Join(a.workDir, live.DefaultLiveStateBasePath)

	// Load portfolio state
	if p, err := live.LoadLastPortfolioState(liveBasePath); err == nil {
		resp.Cash = p.Cash
		resp.CurrentDrawdown = 0 // live.PortfolioState 沒有 drawdown 欄位
		// CumulativePnL = RealizedPnL + UnrealizedPnL
		resp.CumulativePnL = p.RealizedPnL + p.UnrealizedPnL
	} else {
		log.Printf("[DashboardAPI] warn: failed to read portfolio state: %v", err)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Load positions
	posMap, err := live.LoadLastPositions(liveBasePath)
	if err != nil {
		log.Printf("[DashboardAPI] warn: failed to read positions: %v", err)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Convert positions to DTO
	positions := make([]PositionDTO, 0, len(posMap))
	totalMarketValue := 0.0
	for _, pos := range posMap {
		cost := float64(pos.Quantity) * pos.AverageCost
		pnlPct := 0.0
		if cost > 0 {
			pnlPct = pos.UnrealizedPnL / cost
		}
		positions = append(positions, PositionDTO{
			Symbol:        pos.Symbol,
			Quantity:      pos.Quantity,
			AverageCost:   pos.AverageCost,
			CurrentPrice:  pos.CurrentPrice,
			MarketValue:   pos.MarketValue,
			UnrealizedPnL: pos.UnrealizedPnL,
			PnlPct:        pnlPct,
		})
		totalMarketValue += pos.MarketValue
	}

	// Sort positions by symbol for consistent display
	slices.SortFunc(positions, func(a, b PositionDTO) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})

	resp.Positions = positions
	resp.PositionsCount = len(positions)
	resp.PortfolioValue = resp.Cash + totalMarketValue

	// Calculate cumulative P&L percentage
	if resp.Cash > 0 {
		resp.CumulativePnLPct = resp.CumulativePnL / resp.Cash
	}

	// Build equity curve from session summaries
	if curve, err := a.buildEquityCurve(); err == nil {
		resp.EquityCurve = curve
	}

	writeJSON(w, http.StatusOK, resp)
}

// buildEquityCurve constructs an equity curve from session summaries.
func (a *DashboardAPI) buildEquityCurve() ([]EquityCurvePoint, error) {
	summary, err := a.loadSessionSummary("")
	if err != nil || summary == nil {
		return nil, fmt.Errorf("no session summary: %w", err)
	}

	// Use the latest session's portfolio value as the endpoint
	// For now, return a single point; future: aggregate multiple sessions
	return []EquityCurvePoint{
		{Label: summary.SessionID, Value: summary.PortfolioValue},
	}, nil
}
```

注意：需要在檔案頂部 import 中加入 `"strings"`（如果尚未存在）。

- [ ] **Step 2: 註冊路由**

在 `RegisterRoutes` 函式中（搜尋 `mux.HandleFunc("/api/dashboard/risk"`），新增：

```go
mux.HandleFunc("/api/dashboard/portfolio-state", a.handlePortfolioState)
```

- [ ] **Step 3: 驗證編譯**

Run: `go build ./cmd/atlas/...`
Expected: 無錯誤

- [ ] **Step 4: 驗證 API 回應**

Run: `go run ./cmd/atlas -api &` 然後 `curl -s http://localhost:8080/api/dashboard/portfolio-state | python3 -m json.tool`
Expected: 回傳 JSON，包含 cash, positions_count, positions[], equity_curve[] 等欄位

---

### Task 3: 前端 — 側邊欄 + 頁面 HTML

**Files:**
- Modify: `web/static/index.html`

- [ ] **Step 1: 新增側邊欄連結**

在 `<a data-page="live" ...>🛡️ 風控結果</a>` 之後（約 line 635），新增：

```html
<a data-page="portfolio" onclick="switchPage('portfolio')" tabindex="0" title="組合持倉">📂 組合持倉</a>
```

- [ ] **Step 2: 新增 portfolio 頁面 HTML**

在 `<div id="live" class="page" ...>` 區塊之後（搜尋 `<div id="agents" class="page"` 並在其之前），新增：

```html
<div id="portfolio" class="page" style="display:none">
  <h1 id="pageTitle">📂 組合持倉</h1>
  <div class="kpi-grid" id="portfolioKPIs"></div>
  <div class="panel wide" id="equityCurvePanel" style="display:none">
    <h2>淨值趨勢</h2>
    <canvas id="equityChart" height="200"></canvas>
  </div>
  <div class="panel wide" id="positionsPanel">
    <h2>持倉明細</h2>
    <div id="positionsTable"></div>
  </div>
  <details class="help-details">
    <summary><strong>📖 如何解讀本頁</strong></summary>
    <p>顯示最新模擬場次的持倉狀態與淨值變化。損益為未實現損益，實際損益需待部位平倉後確認。</p>
    <p class="text-muted text-sm">資料來源：live state store（data/state/live/）。若無持倉資料，代表尚未執行模擬或所有部位已平倉。</p>
  </details>
</div>
```

- [ ] **Step 3: 驗證 HTML 結構**

Run: 開啟 `web/static/index.html` 確認沒有語法錯誤（配對的標籤、正確的引號）

---

### Task 4: 前端 — JS 渲染函式

**Files:**
- Modify: `web/static/index.html`（`<script>` 區塊）

- [ ] **Step 1: 新增 loadPortfolioState 函式**

在 `loadPageData` 函式的 switch-case 中（搜尋 `case 'live':`），新增：

```javascript
case 'portfolio': loadPortfolioState(); break;
```

然後在 `switchPage` 的 `titles` 物件中新增 `portfolio: '組合持倉'`。

- [ ] **Step 2: 新增 renderPortfolioKPIs 函式**

```javascript
function renderPortfolioKPIs(data) {
  const el = document.getElementById('portfolioKPIs');
  if (!el) return;
  const fmt = v => typeof v === 'number' ? v.toLocaleString('en-US', {minimumFractionDigits: 0, maximumFractionDigits: 0}) : '-';
  const fmtPct = v => typeof v === 'number' ? (v >= 0 ? '+' : '') + (v * 100).toFixed(1) + '%' : '-';
  const pnlColor = v => typeof v === 'number' ? (v >= 0 ? 'var(--up)' : 'var(--down)') : '';
  el.innerHTML = `
    <div class="panel" style="text-align:center"><div class="kpi-label">總資產</div><div class="kpi-value">${fmt(data.portfolio_value)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">可用現金</div><div class="kpi-value">${fmt(data.cash)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">累計損益</div><div class="kpi-value" style="color:${pnlColor(data.cumulative_pnl)}">${fmtPct(data.cumulative_pnl_pct)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">目前回撤</div><div class="kpi-value" style="color:var(--warn)">${fmtPct(data.current_drawdown)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">持倉數</div><div class="kpi-value">${data.positions_count || 0} 檔</div></div>
  `;
}
```

- [ ] **Step 3: 新增 renderEquityCurve 函式**

```javascript
function renderEquityCurve(points) {
  const panel = document.getElementById('equityCurvePanel');
  const canvas = document.getElementById('equityChart');
  if (!panel || !canvas) return;
  if (!points || points.length < 2) {
    panel.style.display = 'none';
    return;
  }
  panel.style.display = '';
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.parentElement.getBoundingClientRect();
  canvas.width = (rect.width - 40) * dpr;
  canvas.height = 200 * dpr;
  canvas.style.width = (rect.width - 40) + 'px';
  canvas.style.height = '200px';
  ctx.scale(dpr, dpr);
  const w = rect.width - 40, h = 200;
  const pad = {top: 20, right: 20, bottom: 30, left: 60};
  const chartW = w - pad.left - pad.right, chartH = h - pad.top - pad.bottom;
  const values = points.map(p => p.value);
  const minV = Math.min(...values), maxV = Math.max(...values);
  const range = maxV - minV || 1;

  ctx.clearRect(0, 0, w, h);

  // Grid lines
  ctx.strokeStyle = '#333';
  ctx.lineWidth = 0.5;
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    const val = maxV - (range / 4) * i;
    ctx.fillStyle = '#888'; ctx.font = '10px sans-serif'; ctx.textAlign = 'right';
    ctx.fillText(val.toFixed(0), pad.left - 5, y + 3);
  }

  // Line
  ctx.strokeStyle = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#3b82f6';
  ctx.lineWidth = 2;
  ctx.beginPath();
  points.forEach((p, i) => {
    const x = pad.left + (i / (points.length - 1)) * chartW;
    const y = pad.top + (1 - (p.value - minV) / range) * chartH;
    i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
  });
  ctx.stroke();

  // X-axis labels (show max 6)
  ctx.fillStyle = '#888'; ctx.font = '9px sans-serif'; ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(points.length / 6));
  points.forEach((p, i) => {
    if (i % step === 0 || i === points.length - 1) {
      const x = pad.left + (i / (points.length - 1)) * chartW;
      ctx.fillText(p.label, x, h - 5);
    }
  });
}
```

- [ ] **Step 4: 新增 renderPositionsTable 函式**

```javascript
function renderPositionsTable(positions) {
  const el = document.getElementById('positionsTable');
  if (!el) return;
  if (!positions || positions.length === 0) {
    el.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">目前無持倉</div>';
    return;
  }
  const fmt = v => typeof v === 'number' ? v.toLocaleString('en-US', {minimumFractionDigits: 2, maximumFractionDigits: 2}) : '-';
  const fmtInt = v => typeof v === 'number' ? v.toLocaleString('en-US') : '-';
  const pnlColor = v => typeof v === 'number' ? (v >= 0 ? 'var(--up)' : 'var(--down)') : 'var(--muted)';
  const pnlSign = v => typeof v === 'number' ? (v >= 0 ? '+' : '') : '';
  el.innerHTML = `
    <div class="table-wrapper">
    <table id="positionsTableEl">
      <thead><tr><th>符號</th><th>數量</th><th>成本</th><th>現價</th><th>市值</th><th>損益</th><th>損益%</th></tr></thead>
      <tbody>
        ${positions.map(p => `<tr>
          <td><strong>${p.symbol}</strong></td>
          <td>${fmtInt(p.quantity)}</td>
          <td>${fmt(p.average_cost)}</td>
          <td>${fmt(p.current_price)}</td>
          <td>${fmt(p.market_value)}</td>
          <td style="color:${pnlColor(p.unrealized_pnl)};font-weight:700">${pnlSign(p.unrealized_pnl)}${fmt(p.unrealized_pnl)}</td>
          <td style="color:${pnlColor(p.pnl_pct)};font-weight:700">${pnlSign(p.pnl_pct)}${(p.pnl_pct * 100).toFixed(2)}%</td>
        </tr>`).join('')}
      </tbody>
    </table>
    </div>
  `;
}
```

- [ ] **Step 5: 新增 loadPortfolioState 函式**

```javascript
async function loadPortfolioState() {
  const el = document.getElementById('portfolioKPIs');
  if (el) el.innerHTML = '<div class="loading">載入中...</div>';
  try {
    const data = await getJSON('/api/dashboard/portfolio-state');
    renderPortfolioKPIs(data);
    renderEquityCurve(data.equity_curve);
    renderPositionsTable(data.positions);
  } catch (e) {
    if (el) el.innerHTML = '<div class="text-muted">載入失敗：' + e.message + '</div>';
  }
}
```

- [ ] **Step 6: 更新 switchPage titles**

在 `switchPage` 函式的 `titles` 物件中（搜尋 `const titles = {`），確認加入 `portfolio: '組合持倉'`。

---

### Task 5: 驗證與提交

- [ ] **Step 1: 編譯檢查**

Run: `go build ./cmd/atlas/...`
Expected: 無錯誤

- [ ] **Step 2: 格式檢查**

Run: `test -z "$(gofmt -l .)"`
Expected: 無輸出

- [ ] **Step 3: 啟動伺服器並測試 API**

Run: `go run ./cmd/atlas -api &` 等待 2 秒
Then: `curl -s http://localhost:8080/api/dashboard/portfolio-state | python3 -m json.tool`
Expected: JSON 回應包含所有欄位

- [ ] **Step 4: 驗證前端**

Run: `curl -s http://localhost:8080/ | grep -c '組合持倉'`
Expected: 至少 2（側邊欄 + 頁面標題）

- [ ] **Step 5: 提交**

```bash
git add internal/monitoring/dashboard_api.go web/static/index.html
git commit -m "feat: add portfolio overview page with holdings, KPIs, and equity curve"
```
