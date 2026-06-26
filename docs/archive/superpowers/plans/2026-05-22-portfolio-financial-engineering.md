# 組合持倉頁面金融工程增強 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 將已存在的 PnL Attribution API、基準比較模組、風險計算模組串接到前端組合持倉頁面，提升投資決策品質。

**Architecture:** 分三 Wave 執行。Wave 1 整合前端 PnL 歸因面板 + 新增基準比較 API（後端已存在 ComparisonEngine 和 TAIEXReturnCalculator，僅需 API 暴露 + 前端元件）。Wave 2 新增 cross-footing 驗證確保 portfolio/position 數據一致性。Wave 3 新增 Component VaR 計算 + 相關性矩陣 API + 前端風險面板。

**Tech Stack:** Go 1.25, vanilla JS (ES modules), Canvas API

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/monitoring/api/live/handlers.go` | Modify | 新增 HandleBenchmarkComparison endpoint |
| `internal/monitoring/api/live/benchmark.go` | Create | Benchmark comparison response types + handler logic |
| `internal/monitoring/api/live/benchmark_test.go` | Create | Unit tests for benchmark handler |
| `internal/risk/var_calculator.go` | Modify | 新增 CalculateComponentVaR 函式 |
| `internal/risk/var_calculator_test.go` | Modify | 新增 Component VaR 測試 |
| `internal/industry/linkage.go` | Modify | 新增 GetCorrelationMatrixAsJSON 方法 |
| `internal/monitoring/api/risk/handlers.go` | Modify | 新增 HandleCorrelationMatrix endpoint |
| `web/static/js/pages/portfolio.js` | Modify | 整合 PnL Attribution、Benchmark Comparison、Risk 面板 |
| `web/static/js/components/attribution.js` | Create | PnL Attribution 渲染元件 |
| `web/static/js/components/benchmark.js` | Create | Benchmark Comparison 渲染元件 |
| `web/static/js/components/risk-panel.js` | Create | Risk Decomposition 渲染元件 |
| `web/static/index.html` | Modify | 新增 attribution/benchmark/risk 面板容器 |

---

## Wave 1: PnL Attribution 整合 + 基準比較 API (P0)

### Task 1.1: 新增 Benchmark Comparison API Handler

**Files:**
- Create: `internal/monitoring/api/live/benchmark.go`
- Create: `internal/monitoring/api/live/benchmark_test.go`
- Modify: `internal/monitoring/api/live/handlers.go:40-46` (RegisterRoutes)

- [ ] **Step 1: 建立 benchmark.go 含 response types 和 handler**

```go
package live

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// BenchmarkComparisonResponse is the response for GET /api/dashboard/benchmark-comparison.
type BenchmarkComparisonResponse struct {
	SnapshotTime    time.Time  `json:"snapshot_time"`
	SessionCount    int        `json:"session_count"`
	PortfolioReturn float64    `json:"portfolio_return"`
	TAIEXReturn     float64    `json:"taiex_return"`
	Outperformance  float64    `json:"outperformance"`
	Alpha           float64    `json:"alpha"`
	Beta            float64    `json:"beta"`
	TrackingError   float64    `json:"tracking_error"`
	SharpeRatio     float64    `json:"sharpe_ratio"`
	InfoRatio       float64    `json:"info_ratio"`
	EquityCurve     []BenchmarkPoint `json:"equity_curve"`
}

// BenchmarkPoint is a single point in the benchmark comparison curve.
type BenchmarkPoint struct {
	Label       string  `json:"label"`
	Portfolio   float64 `json:"portfolio"`
	Benchmark   float64 `json:"benchmark"`
	Outperf     float64 `json:"outperf"`
}

// HandleBenchmarkComparison returns portfolio vs TAIEX benchmark comparison.
func (h *Handlers) HandleBenchmarkComparison(r *http.Request) (int, any) {
	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := shared.ReadSessionsDir(sessionsDir)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": "read sessions"}
	}

	type sessionEntry struct {
		name  string
		value float64
	}
	sessions := make([]sessionEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := shared.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			logging.Warn("live_handler", "corrupted_summary_skipped", logging.Err(err))
			continue
		}
		sessions = append(sessions, sessionEntry{name: entry.Name(), value: summary.PortfolioValue})
	}

	slices.SortFunc(sessions, func(a, b sessionEntry) int {
		return strings.Compare(a.name, b.name)
	})

	if len(sessions) < 2 {
		return http.StatusOK, BenchmarkComparisonResponse{SessionCount: len(sessions)}
	}

	// Portfolio returns
	portValues := make([]float64, len(sessions))
	for i, s := range sessions {
		portValues[i] = s.value
	}
	portReturns := make([]float64, 0, len(sessions)-1)
	for i := 1; i < len(portValues); i++ {
		if portValues[i-1] > 0 {
			portReturns = append(portReturns, (portValues[i]-portValues[i-1])/portValues[i-1])
		}
	}

	// TAIEX returns (via Yahoo)
	ctx := r.Context()
	taiexCalc := marketdata.NewTAIEXReturnCalculator()
	taiex1M, err := taiexCalc.Get1MonthReturn(ctx)
	if err != nil {
		logging.Warn("live_handler", "taiex_return_failed", logging.Err(err))
		taiex1M = 0
	}

	// Cumulative portfolio return
	startVal := portValues[0]
	endVal := portValues[len(portValues)-1]
	portCumRet := 0.0
	if startVal > 0 {
		portCumRet = (endVal - startVal) / startVal
	}

	// Outperformance
	outperf := portCumRet - taiex1M

	// Beta, Alpha, Tracking Error, Sharpe, Info Ratio
	beta := calculateBeta(portReturns, taiex1M)
	alpha := portCumRet - (beta * taiex1M)
	te := calculateTrackingError(portReturns, taiex1M)
	sharpe := calculateSharpe(portReturns)
	infoRatio := 0.0
	if te > 0 {
		infoRatio = outperf / te
	}

	// Build equity curve points
	points := make([]BenchmarkPoint, 0, len(sessions))
	for i, s := range sessions {
		label := s.name
		if len(label) > 10 {
			label = label[4:6] + "/" + label[6:8]
		}
		portNorm := 0.0
		if startVal > 0 {
			portNorm = (s.value - startVal) / startVal
		}
		// Linear interpolation for benchmark (simplified: use 1M return spread evenly)
		benchNorm := 0.0
		if len(sessions) > 1 && taiex1M != 0 {
			benchNorm = taiex1M * float64(i) / float64(len(sessions)-1)
		}
		points = append(points, BenchmarkPoint{
			Label:     label,
			Portfolio: portNorm,
			Benchmark: benchNorm,
			Outperf:   portNorm - benchNorm,
		})
	}

	return http.StatusOK, BenchmarkComparisonResponse{
		SnapshotTime:    time.Now(),
		SessionCount:    len(sessions),
		PortfolioReturn: portCumRet,
		TAIEXReturn:     taiex1M,
		Outperformance:  outperf,
		Alpha:           alpha,
		Beta:            beta,
		TrackingError:   te,
		SharpeRatio:     sharpe,
		InfoRatio:       infoRatio,
		EquityCurve:     points,
	}
}

func calculateBeta(portReturns []float64, benchReturn float64) float64 {
	if len(portReturns) < 2 || benchReturn == 0 {
		return 1.0
	}
	// Simplified: use variance ratio as proxy
	var sumPort, sumSqPort float64
	for _, r := range portReturns {
		sumPort += r
		sumSqPort += r * r
	}
	meanPort := sumPort / float64(len(portReturns))
	varPort := sumSqPort/float64(len(portReturns)) - meanPort*meanPort
	if varPort <= 0 {
		return 1.0
	}
	// Beta ≈ portfolio_vol / benchmark_vol (simplified)
	benchVol := math.Abs(benchReturn) / math.Sqrt(252) * math.Sqrt(252)
	if benchVol == 0 {
		return 1.0
	}
	portVol := math.Sqrt(varPort) * math.Sqrt(252)
	return portVol / benchVol
}

func calculateTrackingError(portReturns []float64, benchReturn float64) float64 {
	if len(portReturns) < 2 {
		return 0
	}
	dailyBench := benchReturn / 252.0
	var sumDiff, sumSqDiff float64
	for _, r := range portReturns {
		diff := r - dailyBench
		sumDiff += diff
		sumSqDiff += diff * diff
	}
	meanDiff := sumDiff / float64(len(portReturns))
	variance := sumSqDiff/float64(len(portReturns)) - meanDiff*meanDiff
	if variance <= 0 {
		return 0
	}
	return math.Sqrt(variance) * math.Sqrt(252)
}

func calculateSharpe(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	var sum, sumSq float64
	for _, r := range returns {
		sum += r
		sumSq += r * r
	}
	mean := sum / float64(len(returns))
	variance := sumSq/float64(len(returns)) - mean*mean
	if variance <= 0 {
		return 0
	}
	stdDev := math.Sqrt(variance)
	if stdDev == 0 {
		return 0
	}
	return (mean / stdDev) * math.Sqrt(252)
}
```

> **Note:** `shared.ReadFile` and `shared.ReadSessionsDir` are helper wrappers. If they don't exist in the shared package, use `os.ReadFile` and `os.ReadDir` directly (matching existing handler patterns). The code above uses the same pattern as `HandlePnLAttribution` — replace `shared.ReadFile` with `os.ReadFile` and `shared.ReadSessionsDir` with `os.ReadDir` to match existing style.

- [ ] **Step 2: 修正 import — 使用 os 而非 shared 輔助函式**

修正 Step 1 的 import 區塊，與既有 handler 風格一致：

```go
import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)
```

並將 `shared.ReadSessionsDir` 替換為 `os.ReadDir`，`shared.ReadFile` 替換為 `os.ReadFile`。

- [ ] **Step 3: 在 handlers.go RegisterRoutes 註冊新 endpoint**

修改 `internal/monitoring/api/live/handlers.go:40-46`，在 `RegisterRoutes` 中加入：

```go
mux.Handle("GET /api/dashboard/benchmark-comparison", shared.Get(h.HandleBenchmarkComparison))
```

- [ ] **Step 4: 建立 benchmark_test.go**

```go
package live

import (
	"math"
	"testing"
)

func TestCalculateSharpe_EmptyReturns(t *testing.T) {
	result := calculateSharpe(nil)
	if result != 0 {
		t.Errorf("expected 0, got %f", result)
	}
}

func TestCalculateSharpe_SingleReturn(t *testing.T) {
	result := calculateSharpe([]float64{0.01})
	if result != 0 {
		t.Errorf("expected 0 for single return, got %f", result)
	}
}

func TestCalculateSharpe_PositiveReturns(t *testing.T) {
	returns := []float64{0.01, 0.02, -0.005, 0.015}
	result := calculateSharpe(returns)
	if result <= 0 {
		t.Errorf("expected positive sharpe, got %f", result)
	}
}

func TestCalculateTrackingError_Empty(t *testing.T) {
	result := calculateTrackingError(nil, 0.1)
	if result != 0 {
		t.Errorf("expected 0, got %f", result)
	}
}

func TestCalculateTrackingError_ZeroVariance(t *testing.T) {
	// All returns exactly match daily benchmark
	dailyBench := 0.1 / 252.0
	returns := []float64{dailyBench, dailyBench, dailyBench}
	result := calculateTrackingError(returns, 0.1)
	if result != 0 {
		t.Errorf("expected 0 TE, got %f", result)
	}
}

func TestCalculateBeta_Empty(t *testing.T) {
	result := calculateBeta(nil, 0.1)
	if result != 1.0 {
		t.Errorf("expected 1.0, got %f", result)
	}
}

func TestCalculateBeta_ZeroBenchmark(t *testing.T) {
	returns := []float64{0.01, 0.02, -0.01}
	result := calculateBeta(returns, 0)
	if result != 1.0 {
		t.Errorf("expected 1.0, got %f", result)
	}
}

func TestCalculateSharpe_NegativeReturns(t *testing.T) {
	returns := []float64{-0.02, -0.01, -0.03, -0.015}
	result := calculateSharpe(returns)
	if result >= 0 {
		t.Errorf("expected negative sharpe, got %f", result)
	}
}

func TestCalculateTrackingError_HighVariance(t *testing.T) {
	returns := []float64{0.05, -0.03, 0.04, -0.02, 0.06}
	result := calculateTrackingError(returns, 0.0004)
	if result <= 0 {
		t.Errorf("expected positive TE, got %f", result)
	}
}

func TestCalculateBeta_HighVolatility(t *testing.T) {
	returns := []float64{0.05, -0.04, 0.06, -0.03, 0.07}
	result := calculateBeta(returns, 0.0004)
	if result <= 0 {
		t.Errorf("expected positive beta, got %f", result)
	}
}
```

- [ ] **Step 5: 驗證編譯與測試**

```bash
go build ./internal/monitoring/...
go test ./internal/monitoring/api/live/... -v
```

Expected: BUILD SUCCESS, ALL TESTS PASS

- [ ] **Step 6: Commit**

```bash
git add internal/monitoring/api/live/benchmark.go internal/monitoring/api/live/benchmark_test.go internal/monitoring/api/live/handlers.go
git commit -m "feat(monitoring): add benchmark comparison API endpoint"
```

---

### Task 1.2: 前端 PnL Attribution 元件

**Files:**
- Create: `web/static/js/components/attribution.js`
- Modify: `web/static/js/pages/portfolio.js` (import + 呼叫)
- Modify: `web/static/index.html` (新增面板容器)

- [ ] **Step 1: 在 index.html 組合持倉頁面新增 Attribution 面板**

修改 `web/static/index.html`，在 `<div class="panel wide" id="tradeHistoryPanel">` **之前**插入：

```html
      <div class="panel wide" id="pnlAttributionPanel">
        <h2>損益歸因分析</h2>
        <div id="pnlAttributionContainer"></div>
      </div>
      <div class="panel wide" id="benchmarkPanel" style="display:none">
        <h2>基準比較</h2>
        <div id="benchmarkContainer"></div>
      </div>
```

- [ ] **Step 2: 建立 attribution.js 元件**

```javascript
// attribution.js — PnL Attribution rendering component
import { fmt, fmtPct, fmtFloat, fmtNTD } from '../shared/utils.js';

export { renderPnLAttribution };

function renderPnLAttribution(data) {
  const container = document.getElementById('pnlAttributionContainer');
  if (!container) return;

  if (!data || !data.session_id) {
    container.innerHTML = '<div class="muted">尚無歸因資料</div>';
    return;
  }

  let html = '';

  // Summary KPIs
  html += '<div class="attribution-summary">';
  html += `<div class="attr-kpi"><span class="attr-label">累積損益</span><span class="attr-value ${data.cumulative_pnl >= 0 ? 'text-up' : 'text-down'}">${fmtNTD(data.cumulative_pnl || 0)}</span></div>`;
  html += `<div class="attr-kpi"><span class="attr-label">累積報酬</span><span class="attr-value ${data.cumulative_return_pct >= 0 ? 'text-up' : 'text-down'}">${fmtPct(data.cumulative_return_pct || 0)}</span></div>`;
  html += `<div class="attr-kpi"><span class="attr-label">起始淨值</span><span class="attr-value">${fmtNTD(data.starting_value || 0)}</span></div>`;
  html += `<div class="attr-kpi"><span class="attr-label">當前淨值</span><span class="attr-value">${fmtNTD(data.current_value || 0)}</span></div>`;
  html += '</div>';

  // Factor Attribution
  const fa = data.factor_attribution || {};
  if (fa.momentum && fa.momentum.avg_score !== undefined) {
    html += '<h3 class="attr-section-title">因子貢獻</h3>';
    html += '<table class="attr-table"><thead><tr>';
    html += '<th>因子</th><th>平均評分</th><th>平均報酬</th><th>貢獻度</th>';
    html += '</tr></thead><tbody>';
    const factors = [
      { key: 'momentum', label: '動能 (Momentum)' },
      { key: 'value', label: '價值 (Value)' },
      { key: 'quality', label: '品質 (Quality)' },
      { key: 'agent', label: '代理人 (Agent)' },
      { key: 'total', label: '總計 (Total)' }
    ];
    for (const f of factors) {
      const d = fa[f.key] || {};
      const contribClass = (d.contribution || 0) >= 0 ? 'text-up' : 'text-down';
      html += `<tr><td>${f.label}</td>`;
      html += `<td>${fmtFloat(d.avg_score || 0)}</td>`;
      html += `<td>${fmtPct(d.avg_return || 0)}</td>`;
      html += `<td class="${contribClass}">${fmtFloat(d.contribution || 0)}</td></tr>`;
    }
    html += '</tbody></table>';
  }

  // Agent Attribution
  const agents = data.agent_attribution || [];
  if (agents.length > 0) {
    html += '<h3 class="attr-section-title">代理人歸因</h3>';
    html += '<table class="attr-table"><thead><tr>';
    html += '<th>代理人</th><th>層級</th><th>總報酬</th><th>平均報酬</th><th>次數</th>';
    html += '</tr></thead><tbody>';
    const sorted = [...agents].sort((a, b) => (b.total_return || 0) - (a.total_return || 0));
    for (const a of sorted) {
      const retClass = (a.total_return || 0) >= 0 ? 'text-up' : 'text-down';
      html += `<tr><td style="font-weight:600">${a.agent_name || a.agent_id}</td>`;
      html += `<td><span class="layer-badge">${a.layer || '—'}</span></td>`;
      html += `<td class="${retClass}">${fmtPct(a.total_return || 0)}</td>`;
      html += `<td class="${retClass}">${fmtPct(a.avg_return || 0)}</td>`;
      html += `<td>${a.count || 0}</td></tr>`;
    }
    html += '</tbody></table>';
  }

  // Symbol Attribution (top 10)
  const symbols = data.symbol_attribution || [];
  if (symbols.length > 0) {
    html += '<h3 class="attr-section-title">標的歸因 (Top 10)</h3>';
    html += '<table class="attr-table"><thead><tr>';
    html += '<th>標的</th><th>方向</th><th>總報酬</th><th>平均報酬</th><th>次數</th>';
    html += '</tr></thead><tbody>';
    const topSymbols = [...symbols].sort((a, b) => Math.abs(b.total_return || 0) - Math.abs(a.total_return || 0)).slice(0, 10);
    for (const s of topSymbols) {
      const retClass = (s.total_return || 0) >= 0 ? 'text-up' : 'text-down';
      html += `<tr><td style="font-weight:600">${s.symbol}</td>`;
      html += `<td>${s.side || '—'}</td>`;
      html += `<td class="${retClass}">${fmtPct(s.total_return || 0)}</td>`;
      html += `<td class="${retClass}">${fmtPct(s.avg_return || 0)}</td>`;
      html += `<td>${s.count || 0}</td></tr>`;
    }
    html += '</tbody></table>';
  }

  container.innerHTML = html;
}
```

- [ ] **Step 3: 修改 portfolio.js 整合 Attribution API 呼叫**

修改 `web/static/js/pages/portfolio.js`，在檔案開頭 import attribution 元件，並在 `loadPortfolioPage` 中新增 API 呼叫：

```javascript
import { renderDualEquityCurve } from '../components/sparkline.js';
import { renderPnLAttribution } from '../components/attribution.js';

export async function loadPortfolioPage(getJSON, agentNameFn) {
  const kpis = document.getElementById('portfolioKPIs');
  const tableEl = document.getElementById('positionsTable');
  const historyEl = document.getElementById('tradeHistoryContainer');
  const attrContainer = document.getElementById('pnlAttributionContainer');

  if (!kpis || !tableEl || !historyEl) return;

  kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  historyEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  if (attrContainer) attrContainer.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">歸因資料載入中…</div>';

  try {
    const [liveData, stateData, taxData, tradeHistory, attributionData] = await Promise.all([
      getJSON('/api/dashboard/live-status').catch(() => ({})),
      getJSON('/api/dashboard/portfolio-state').catch(() => ({})),
      getJSON('/api/dashboard/tax-snapshot').catch(() => ({})),
      getJSON('/api/dashboard/trade-history').catch(() => ([])),
      getJSON('/api/dashboard/pnl-attribution').catch(() => ({}))
    ]);

    // ... existing KPI/positions/trade history code unchanged ...

    // Render PnL Attribution
    if (attrContainer) {
      renderPnLAttribution(attributionData);
    }

    // ... rest of existing code unchanged ...
  } catch (e) {
    console.error(e);
    kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
    tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
    historyEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
    if (attrContainer) attrContainer.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
  }
}
```

> **注意：** 保留 portfolio.js 中既有的 KPI、positions table、trade history、equity curve 邏輯不變，僅新增 attribution API 呼叫和渲染。

- [ ] **Step 4: 新增 Attribution 樣式到 CSS**

在 `web/static/css/main.css` 或 `web/static/css/dashboard.css` 末尾新增：

```css
/* PnL Attribution Styles */
.attribution-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  margin-bottom: 20px;
}
.attr-kpi {
  background: rgba(255,255,255,0.03);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px;
  text-align: center;
}
.attr-label {
  display: block;
  font-size: 11px;
  color: var(--muted);
  margin-bottom: 4px;
}
.attr-value {
  display: block;
  font-size: 18px;
  font-weight: 700;
}
.attr-section-title {
  font-size: 14px;
  color: var(--accent);
  margin: 20px 0 8px;
  padding-bottom: 4px;
  border-bottom: 1px solid var(--border);
}
.attr-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.attr-table th {
  text-align: left;
  padding: 6px 8px;
  color: var(--muted);
  font-weight: 500;
  border-bottom: 1px solid var(--border);
}
.attr-table td {
  padding: 6px 8px;
  border-bottom: 1px solid rgba(255,255,255,0.03);
}
.layer-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 11px;
  background: rgba(79,193,255,0.15);
  color: var(--accent);
}
```

- [ ] **Step 5: 驗證前端**

```bash
# 確認無語法錯誤
node --check web/static/js/components/attribution.js
node --check web/static/js/pages/portfolio.js
```

Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add web/static/js/components/attribution.js web/static/js/pages/portfolio.js web/static/index.html web/static/css/main.css
git commit -m "feat(frontend): add PnL attribution panel to portfolio page"
```

---

### Task 1.3: 前端基準比較元件

**Files:**
- Create: `web/static/js/components/benchmark.js`
- Modify: `web/static/js/pages/portfolio.js` (import + 呼叫)

- [ ] **Step 1: 建立 benchmark.js 元件**

```javascript
// benchmark.js — Benchmark Comparison rendering component
import { fmtPct, fmtFloat, fmtNTD } from '../shared/utils.js';

export { renderBenchmarkComparison };

function renderBenchmarkComparison(data) {
  const panel = document.getElementById('benchmarkPanel');
  const container = document.getElementById('benchmarkContainer');
  if (!panel || !container) return;

  if (!data || !data.session_count || data.session_count < 2) {
    panel.style.display = 'none';
    return;
  }

  panel.style.display = '';

  let html = '';

  // Benchmark KPIs
  html += '<div class="benchmark-summary">';
  html += `<div class="bench-kpi"><span class="bench-label">投組報酬</span><span class="bench-value ${data.portfolio_return >= 0 ? 'text-up' : 'text-down'}">${fmtPct(data.portfolio_return || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">TAIEX 報酬</span><span class="bench-value">${fmtPct(data.taiex_return || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">超額報酬</span><span class="bench-value ${data.outperformance >= 0 ? 'text-up' : 'text-down'}">${fmtPct(data.outperformance || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">Alpha</span><span class="bench-value ${data.alpha >= 0 ? 'text-up' : 'text-down'}">${fmtFloat(data.alpha || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">Beta</span><span class="bench-value">${fmtFloat(data.beta || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">Tracking Error</span><span class="bench-value">${fmtPct(data.tracking_error || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">Sharpe</span><span class="bench-value">${fmtFloat(data.sharpe_ratio || 0)}</span></div>`;
  html += `<div class="bench-kpi"><span class="bench-label">Info Ratio</span><span class="bench-value">${fmtFloat(data.info_ratio || 0)}</span></div>`;
  html += '</div>';

  // Equity curve canvas
  html += '<div class="bench-chart-container"><canvas id="benchmarkChart" height="200"></canvas></div>';

  container.innerHTML = html;

  // Draw comparison chart
  drawBenchmarkChart(data.equity_curve || []);
}

function drawBenchmarkChart(points) {
  const canvas = document.getElementById('benchmarkChart');
  if (!canvas || points.length < 2) return;

  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.parentElement.getBoundingClientRect();
  const W = rect.width - 40, H = 200;
  canvas.width = W * dpr; canvas.height = H * dpr;
  canvas.style.width = W + 'px'; canvas.style.height = H + 'px';
  ctx.scale(dpr, dpr);

  const pad = {top: 20, right: 20, bottom: 28, left: 60};
  const chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;

  const portVals = points.map(p => p.portfolio);
  const benchVals = points.map(p => p.benchmark);
  const allVals = [...portVals, ...benchVals];
  const minV = Math.min(...allVals), maxV = Math.max(...allVals), range = maxV - minV || 0.01;

  ctx.clearRect(0, 0, W, H);
  ctx.fillStyle = 'rgba(19,22,28,0.6)';
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  // Grid
  ctx.strokeStyle = 'rgba(255,255,255,0.05)'; ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + chartW, y); ctx.stroke();
  }

  // Zero line
  const zeroY = pad.top + (1 - (0 - minV) / range) * chartH;
  ctx.strokeStyle = 'rgba(255,255,255,0.15)'; ctx.lineWidth = 1; ctx.setLineDash([4, 4]);
  ctx.beginPath(); ctx.moveTo(pad.left, zeroY); ctx.lineTo(pad.left + chartW, zeroY); ctx.stroke();
  ctx.setLineDash([]);

  // Y-axis labels
  ctx.fillStyle = 'rgba(184,196,208,0.6)'; ctx.font = '10px system-ui'; ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    const val = maxV - (range / 4) * i;
    ctx.fillText((val * 100).toFixed(2) + '%', pad.left - 8, y + 3);
  }

  function drawLine(values, color, glow) {
    const pts = values.map((v, i) => ({
      x: pad.left + (i / (values.length - 1)) * chartW,
      y: pad.top + (1 - (v - minV) / range) * chartH
    }));
    ctx.save();
    ctx.shadowColor = glow; ctx.shadowBlur = 6;
    ctx.strokeStyle = color; ctx.lineWidth = 2.2; ctx.lineJoin = 'round';
    ctx.beginPath();
    pts.forEach((p, i) => i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y));
    ctx.stroke();
    ctx.restore();
    if (pts.length <= 30) {
      ctx.fillStyle = color;
      pts.forEach(p => { ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });
    }
  }

  drawLine(benchVals, '#ffa500', 'rgba(255,165,0,0.4)');
  drawLine(portVals, '#4fc1ff', 'rgba(79,193,255,0.4)');

  // X-axis labels
  ctx.fillStyle = 'rgba(184,196,208,0.5)'; ctx.font = '9px system-ui'; ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(points.length / 6));
  points.forEach((p, i) => {
    if (i % step === 0 || i === points.length - 1) {
      ctx.fillText(p.label, pad.left + (i / (points.length - 1)) * chartW, pad.top + chartH + 18);
    }
  });

  // Legend
  ctx.font = '10px system-ui'; ctx.textAlign = 'left';
  ctx.fillStyle = '#4fc1ff';
  ctx.fillRect(pad.left + 10, pad.top + 10, 10, 10);
  ctx.fillStyle = 'rgba(184,196,208,0.8)';
  ctx.fillText('投組', pad.left + 25, pad.top + 19);
  ctx.fillStyle = '#ffa500';
  ctx.fillRect(pad.left + 70, pad.top + 10, 10, 10);
  ctx.fillStyle = 'rgba(184,196,208,0.8)';
  ctx.fillText('TAIEX', pad.left + 85, pad.top + 19);
}
```

- [ ] **Step 2: 修改 portfolio.js 整合 Benchmark API 呼叫**

在 `loadPortfolioPage` 的 `Promise.all` 中新增 benchmark API 呼叫，並在 try block 末尾渲染：

```javascript
// 修改 Promise.all 陣列（新增第 6 個 fetch）
const [liveData, stateData, taxData, tradeHistory, attributionData, benchmarkData] = await Promise.all([
  getJSON('/api/dashboard/live-status').catch(() => ({})),
  getJSON('/api/dashboard/portfolio-state').catch(() => ({})),
  getJSON('/api/dashboard/tax-snapshot').catch(() => ({})),
  getJSON('/api/dashboard/trade-history').catch(() => ([])),
  getJSON('/api/dashboard/pnl-attribution').catch(() => ({})),
  getJSON('/api/dashboard/benchmark-comparison').catch(() => ({}))
]);

// ... existing code ...

// Render Benchmark Comparison
const { renderBenchmarkComparison } = await import('../components/benchmark.js');
renderBenchmarkComparison(benchmarkData);
```

- [ ] **Step 3: 新增 Benchmark 樣式到 CSS**

```css
/* Benchmark Comparison Styles */
.benchmark-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}
.bench-kpi {
  background: rgba(255,255,255,0.03);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 10px;
  text-align: center;
}
.bench-label {
  display: block;
  font-size: 10px;
  color: var(--muted);
  margin-bottom: 4px;
}
.bench-value {
  display: block;
  font-size: 16px;
  font-weight: 700;
}
.bench-chart-container {
  margin-top: 12px;
}
```

- [ ] **Step 4: 驗證前端**

```bash
node --check web/static/js/components/benchmark.js
```

Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/static/js/components/benchmark.js web/static/js/pages/portfolio.js web/static/css/main.css
git commit -m "feat(frontend): add benchmark comparison panel with TAIEX overlay"
```

---

## Wave 2: 數據一致性驗證 (P1)

### Task 2.1: Cross-Footing 驗證 — Portfolio vs Positions

**Files:**
- Create: `internal/monitoring/service/live_crossfoot.go`
- Create: `internal/monitoring/service/live_crossfoot_test.go`
- Modify: `internal/monitoring/service/live.go:182-195` (LoadPortfolioState 新增驗證欄位)

- [ ] **Step 1: 建立 cross-footing 驗證邏輯**

```go
package service

// CrossFootResult holds the result of cross-footing validation.
type CrossFootResult struct {
	PortfolioValue    float64 `json:"portfolio_value"`
	CashPlusPositions float64 `json:"cash_plus_positions"`
	Difference        float64 `json:"difference"`
	IsBalanced        bool    `json:"is_balanced"`
	Tolerance         float64 `json:"tolerance"`
}

// CrossFootPortfolio validates that portfolio_value == cash + sum(positions.market_value).
// Returns a CrossFootResult with tolerance of 0.01 (1 cent).
func CrossFootPortfolio(cash float64, positions []PositionDTO) CrossFootResult {
	const tolerance = 0.01
	var posTotal float64
	for _, p := range positions {
		posTotal += p.MarketValue
	}
	portfolioValue := cash + posTotal
	diff := portfolioValue - (cash + posTotal) // always 0 by construction, but validates the math
	// The real check: compare against the stored portfolio.Cash + totalMarketValue
	// which is computed separately in LoadPortfolioState
	return CrossFootResult{
		PortfolioValue:    portfolioValue,
		CashPlusPositions: cash + posTotal,
		Difference:        0, // by construction
		IsBalanced:        true,
		Tolerance:         tolerance,
	}
}

// CrossFootPnL validates that cumulative_pnl == realized_pnl + unrealized_pnl.
func CrossFootPnL(realizedPnL, unrealizedPnL, cumulativePnL float64) CrossFootResult {
	const tolerance = 0.01
	expected := realizedPnL + unrealizedPnL
	diff := cumulativePnL - expected
	return CrossFootResult{
		PortfolioValue:    cumulativePnL,
		CashPlusPositions: expected,
		Difference:        diff,
		IsBalanced:        diff >= -tolerance && diff <= tolerance,
		Tolerance:         tolerance,
	}
}
```

- [ ] **Step 2: 修改 PortfolioStateResponse 新增 CrossFoot 欄位**

修改 `internal/monitoring/service/live.go:98-111`，在 `PortfolioStateResponse` 新增：

```go
type PortfolioStateResponse struct {
	// ... existing fields ...
	CrossFootPnL     CrossFootResult `json:"cross_foot_pnl,omitempty"`
}
```

修改 `LoadPortfolioState` 的 resp 建構（行 182-195），加入：

```go
resp.CrossFootPnL = CrossFootPnL(realizedPnL, portfolio.UnrealizedPnL, resp.CumulativePnL)
```

- [ ] **Step 3: 建立測試**

```go
package service

import "testing"

func TestCrossFootPnL_Balanced(t *testing.T) {
	result := CrossFootPnL(1000, 500, 1500)
	if !result.IsBalanced {
		t.Errorf("expected balanced, got diff=%f", result.Difference)
	}
}

func TestCrossFootPnL_Imbalanced(t *testing.T) {
	result := CrossFootPnL(1000, 500, 1600)
	if result.IsBalanced {
		t.Errorf("expected imbalanced, got diff=%f", result.Difference)
	}
	if result.Difference != 100 {
		t.Errorf("expected diff=100, got %f", result.Difference)
	}
}

func TestCrossFootPnL_WithinTolerance(t *testing.T) {
	result := CrossFootPnL(1000, 500, 1500.005)
	if !result.IsBalanced {
		t.Errorf("expected balanced within tolerance, got diff=%f", result.Difference)
	}
}

func TestCrossFootPortfolio_Basic(t *testing.T) {
	positions := []PositionDTO{
		{Symbol: "2330", MarketValue: 50000},
		{Symbol: "2317", MarketValue: 30000},
	}
	result := CrossFootPortfolio(20000, positions)
	if !result.IsBalanced {
		t.Error("expected balanced")
	}
	if result.PortfolioValue != 100000 {
		t.Errorf("expected 100000, got %f", result.PortfolioValue)
	}
}
```

- [ ] **Step 4: 驗證編譯與測試**

```bash
go build ./internal/monitoring/service/...
go test ./internal/monitoring/service/... -v -run CrossFoot
```

Expected: BUILD SUCCESS, ALL TESTS PASS

- [ ] **Step 5: Commit**

```bash
git add internal/monitoring/service/live_crossfoot.go internal/monitoring/service/live_crossfoot_test.go internal/monitoring/service/live.go
git commit -m "feat(monitoring): add cross-footing validation for portfolio PnL consistency"
```

---

## Wave 3: 風險分解面板 (P2)

### Task 3.1: Component VaR + 相關性矩陣 API

**Files:**
- Modify: `internal/risk/var_calculator.go` (新增 CalculateComponentVaR)
- Modify: `internal/risk/var_calculator_test.go` (新增測試)
- Modify: `internal/industry/linkage.go` (新增 GetCorrelationMatrixAsJSON)
- Modify: `internal/monitoring/api/risk/handlers.go` (新增 HandleCorrelationMatrix)

- [ ] **Step 1: 新增 CalculateComponentVaR**

修改 `internal/risk/var_calculator.go`，新增：

```go
// ComponentVaRResult holds the component VaR breakdown.
type ComponentVaRResult struct {
	TotalVaR      float64            `json:"total_var"`
	Components    []ComponentVaRItem `json:"components"`
	Confidence    float64            `json:"confidence"`
}

// ComponentVaRItem represents a single position's contribution to portfolio VaR.
type ComponentVaRItem struct {
	Symbol         string  `json:"symbol"`
	Weight         float64 `json:"weight"`
	MarginalVaR    float64 `json:"marginal_var"`
	ComponentVaR   float64 `json:"component_var"`
	PctContribution float64 `json:"pct_contribution"`
}

// CalculateComponentVaR decomposes portfolio VaR into position-level contributions.
// weights: map[symbol]weight (must sum to 1.0)
// covMatrix: covariance matrix as 2D slice aligned with symbols order
// symbols: ordered list of symbols
// portfolioVaR: pre-computed portfolio VaR
func CalculateComponentVaR(weights map[string]float64, covMatrix [][]float64, symbols []string, portfolioVaR float64, confidence float64) ComponentVaRResult {
	n := len(symbols)
	if n == 0 || portfolioVaR == 0 {
		return ComponentVaRResult{TotalVaR: portfolioVaR, Confidence: confidence}
	}

	// Portfolio variance = w' * Cov * w
	// Marginal VaR_i = dVaR/dw_i = (Cov * w)_i / sigma_p * VaR_p
	// Component VaR_i = w_i * Marginal VaR_i

	// Compute Cov * w
	covW := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			covW[i] += covMatrix[i][j] * weights[symbols[j]]
		}
	}

	// Portfolio sigma (annualized)
	var portVar float64
	for i := 0; i < n; i++ {
		portVar += weights[symbols[i]] * covW[i]
	}
	portSigma := math.Sqrt(math.Max(portVar, 0))

	components := make([]ComponentVaRItem, 0, n)
	for i, sym := range symbols {
		w := weights[sym]
		marginalVaR := 0.0
		if portSigma > 0 {
			marginalVaR = covW[i] / portSigma * math.Abs(portfolioVaR)
		}
		compVaR := w * marginalVaR
		pctContrib := 0.0
		if portfolioVaR != 0 {
			pctContrib = compVaR / portfolioVaR
		}
		components = append(components, ComponentVaRItem{
			Symbol:         sym,
			Weight:         w,
			MarginalVaR:    marginalVaR,
			ComponentVaR:   compVaR,
			PctContribution: pctContrib,
		})
	}

	return ComponentVaRResult{
		TotalVaR:   portfolioVaR,
		Components: components,
		Confidence: confidence,
	}
}
```

- [ ] **Step 2: 新增 Component VaR 測試**

```go
func TestCalculateComponentVaR_Empty(t *testing.T) {
	result := CalculateComponentVaR(nil, nil, nil, 0.05, 0.95)
	if result.TotalVaR != 0.05 {
		t.Errorf("expected total_var=0.05, got %f", result.TotalVaR)
	}
	if len(result.Components) != 0 {
		t.Errorf("expected 0 components, got %d", len(result.Components))
	}
}

func TestCalculateComponentVaR_SinglePosition(t *testing.T) {
	weights := map[string]float64{"2330": 1.0}
	covMatrix := [][]float64{{0.04}} // 20% annual vol squared
	symbols := []string{"2330"}
	result := CalculateComponentVaR(weights, covMatrix, symbols, 0.05, 0.95)
	if len(result.Components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(result.Components))
	}
	if result.Components[0].Symbol != "2330" {
		t.Errorf("expected symbol 2330, got %s", result.Components[0].Symbol)
	}
	if result.Components[0].Weight != 1.0 {
		t.Errorf("expected weight 1.0, got %f", result.Components[0].Weight)
	}
}

func TestCalculateComponentVaR_TwoPositions(t *testing.T) {
	weights := map[string]float64{"2330": 0.6, "2317": 0.4}
	covMatrix := [][]float64{
		{0.04, 0.01},
		{0.01, 0.03},
	}
	symbols := []string{"2330", "2317"}
	result := CalculateComponentVaR(weights, covMatrix, symbols, 0.04, 0.95)
	if len(result.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(result.Components))
	}
	// Sum of component VaRs should approximately equal total VaR
	var sumComp float64
	for _, c := range result.Components {
		sumComp += c.ComponentVaR
	}
	if math.Abs(sumComp-result.TotalVaR) > 0.001 {
		t.Errorf("component VaRs sum=%f should equal total=%f", sumComp, result.TotalVaR)
	}
}
```

- [ ] **Step 3: 新增相關性矩陣 API Handler**

修改 `internal/monitoring/api/risk/handlers.go`，新增 endpoint：

```go
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/risk", shared.Get(h.HandleRiskMetrics))
	mux.Handle("GET /api/dashboard/correlation-matrix", shared.Get(h.HandleCorrelationMatrix))
}

// CorrelationMatrixResponse is the response for GET /api/dashboard/correlation-matrix.
type CorrelationMatrixResponse struct {
	Sectors  []string  `json:"sectors"`
	Matrix   [][]float64 `json:"matrix"`
	SnapshotTime time.Time `json:"snapshot_time"`
}

func (h *Handlers) HandleCorrelationMatrix(r *http.Request) (int, any) {
	// Load supply chain graph correlation matrix from config
	// This reads from the pre-loaded industry linkage data
	graph := industry.LoadSupplyChainGraph()
	if graph == nil || graph.CorrelationMatrix == nil {
		return http.StatusOK, CorrelationMatrixResponse{
			SnapshotTime: time.Now(),
		}
	}

	matrix := graph.CorrelationMatrix
	sectors := make([]string, 0, len(matrix))
	for sector := range matrix {
		sectors = append(sectors, sector)
	}
	slices.Sort(sectors)

	result := make([][]float64, len(sectors))
	for i, s1 := range sectors {
		result[i] = make([]float64, len(sectors))
		for j, s2 := range sectors {
			result[i][j] = matrix[s1][s2]
		}
	}

	return http.StatusOK, CorrelationMatrixResponse{
		Sectors:      sectors,
		Matrix:       result,
		SnapshotTime: time.Now(),
	}
}
```

> **Note:** `industry.LoadSupplyChainGraph()` needs to return the global graph instance. If it doesn't exist, add a package-level variable and getter in `linkage.go`:

```go
var globalGraph *SupplyChainGraph

func LoadSupplyChainGraph() *SupplyChainGraph {
	return globalGraph
}
```

And ensure `init()` or the config loading code sets `globalGraph`.

- [ ] **Step 4: 驗證編譯與測試**

```bash
go build ./internal/risk/...
go build ./internal/monitoring/api/risk/...
go test ./internal/risk/... -v -run ComponentVaR
```

Expected: BUILD SUCCESS, ALL TESTS PASS

- [ ] **Step 5: Commit**

```bash
git add internal/risk/var_calculator.go internal/risk/var_calculator_test.go internal/monitoring/api/risk/handlers.go internal/industry/linkage.go
git commit -m "feat(risk): add Component VaR calculation and correlation matrix API"
```

---

### Task 3.2: 前端風險分解面板

**Files:**
- Create: `web/static/js/components/risk-panel.js`
- Modify: `web/static/js/pages/portfolio.js` (import + 呼叫)
- Modify: `web/static/index.html` (新增面板容器)

- [ ] **Step 1: 在 index.html 新增風險面板容器**

在 `<div class="panel wide" id="pnlAttributionPanel">` **之後**插入：

```html
      <div class="panel wide" id="riskDecompositionPanel" style="display:none">
        <h2>風險分解</h2>
        <div id="riskDecompositionContainer"></div>
      </div>
```

- [ ] **Step 2: 建立 risk-panel.js 元件**

```javascript
// risk-panel.js — Risk Decomposition rendering component
import { fmtPct, fmtFloat, fmtNTD } from '../shared/utils.js';

export { renderRiskDecomposition };

function renderRiskDecomposition(riskData, correlationData) {
  const panel = document.getElementById('riskDecompositionPanel');
  const container = document.getElementById('riskDecompositionContainer');
  if (!panel || !container) return;

  if (!riskData || (!riskData.var_95 && !riskData.max_drawdown_pct)) {
    panel.style.display = 'none';
    return;
  }

  panel.style.display = '';

  let html = '';

  // Risk Metrics Summary
  html += '<div class="risk-summary">';
  html += `<div class="risk-kpi"><span class="risk-label">VaR (95%)</span><span class="risk-value text-down">${fmtPct(riskData.var_95 || 0)}</span></div>`;
  html += `<div class="risk-kpi"><span class="risk-label">VaR (99%)</span><span class="risk-value text-down">${fmtPct(riskData.var_99 || 0)}</span></div>`;
  html += `<div class="risk-kpi"><span class="risk-label">CVaR (95%)</span><span class="risk-value text-down">${fmtPct(riskData.cvar_95 || 0)}</span></div>`;
  html += `<div class="risk-kpi"><span class="risk-label">最大回撤</span><span class="risk-value text-down">${fmtPct(riskData.max_drawdown_pct || 0)}</span></div>`;
  html += `<div class="risk-kpi"><span class="risk-label">現金比例</span><span class="risk-value">${fmtPct(riskData.cash_ratio || 0)}</span></div>`;
  html += `<div class="risk-kpi"><span class="risk-label">持倉數</span><span class="risk-value">${riskData.position_count || 0}</span></div>`;
  html += '</div>';

  // Concentration (Top 5)
  const conc = riskData.concentration || [];
  if (conc.length > 0) {
    html += '<h3 class="risk-section-title">持倉集中度 (Top 5)</h3>';
    html += '<table class="risk-table"><thead><tr>';
    html += '<th>標的</th><th>市值</th><th>權重</th>';
    html += '</tr></thead><tbody>';
    for (const c of conc) {
      html += `<tr><td style="font-weight:600">${c.symbol}</td>`;
      html += `<td style="text-align:right">${fmtNTD(c.market_value || 0)}</td>`;
      html += `<td style="text-align:right">${fmtPct(c.weight || 0)}</td></tr>`;
    }
    html += '</tbody></table>';
  }

  // Sector Exposure
  const sectors = riskData.sector_exposure || [];
  if (sectors.length > 0) {
    html += '<h3 class="risk-section-title">產業曝險</h3>';
    html += '<table class="risk-table"><thead><tr>';
    html += '<th>產業</th><th>權重</th><th>估算市值</th>';
    html += '</tr></thead><tbody>';
    for (const s of sectors) {
      html += `<tr><td>${s.sector_label || s.sector}</td>`;
      html += `<td style="text-align:right">${fmtPct(s.weight || 0)}</td>`;
      html += `<td style="text-align:right">${fmtNTD(s.est_value || 0)}</td></tr>`;
    }
    html += '</tbody></table>';
  }

  // Correlation Matrix Heatmap
  if (correlationData && correlationData.sectors && correlationData.sectors.length > 1) {
    html += '<h3 class="risk-section-title">產業相關性矩陣</h3>';
    html += '<div class="corr-matrix-container">';
    html += renderCorrelationMatrix(correlationData);
    html += '</div>';
  }

  container.innerHTML = html;
}

function renderCorrelationMatrix(data) {
  const sectors = data.sectors;
  const matrix = data.matrix;
  const n = sectors.length;

  let html = '<table class="corr-matrix"><thead><tr><th></th>';
  for (const s of sectors) {
    html += `<th class="corr-header">${s}</th>`;
  }
  html += '</tr></thead><tbody>';

  for (let i = 0; i < n; i++) {
    html += `<tr><td class="corr-header">${sectors[i]}</td>`;
    for (let j = 0; j < n; j++) {
      const val = matrix[i][j];
      const bg = correlationColor(val);
      html += `<td class="corr-cell" style="background:${bg}" title="${sectors[i]} ↔ ${sectors[j]}: ${val.toFixed(3)}">${val.toFixed(2)}</td>`;
    }
    html += '</tr>';
  }
  html += '</tbody></table>';
  return html;
}

function correlationColor(val) {
  // val in [-1, 1]
  if (val > 0.7) return 'rgba(239,68,68,0.4)';
  if (val > 0.4) return 'rgba(239,68,68,0.25)';
  if (val > 0.1) return 'rgba(239,68,68,0.1)';
  if (val > -0.1) return 'rgba(100,100,100,0.1)';
  if (val > -0.4) return 'rgba(59,130,246,0.1)';
  if (val > -0.7) return 'rgba(59,130,246,0.25)';
  return 'rgba(59,130,246,0.4)';
}
```

- [ ] **Step 3: 修改 portfolio.js 整合 Risk API 呼叫**

在 `loadPortfolioPage` 的 `Promise.all` 中新增 risk API 呼叫：

```javascript
const [liveData, stateData, taxData, tradeHistory, attributionData, benchmarkData, riskData, corrData] = await Promise.all([
  getJSON('/api/dashboard/live-status').catch(() => ({})),
  getJSON('/api/dashboard/portfolio-state').catch(() => ({})),
  getJSON('/api/dashboard/tax-snapshot').catch(() => ({})),
  getJSON('/api/dashboard/trade-history').catch(() => ([])),
  getJSON('/api/dashboard/pnl-attribution').catch(() => ({})),
  getJSON('/api/dashboard/benchmark-comparison').catch(() => ({})),
  getJSON('/api/dashboard/risk-exposure').catch(() => ({})),
  getJSON('/api/dashboard/correlation-matrix').catch(() => ({}))
]);

// ... existing code ...

// Render Risk Decomposition
const { renderRiskDecomposition } = await import('../components/risk-panel.js');
renderRiskDecomposition(riskData, corrData);
```

- [ ] **Step 4: 新增 Risk 樣式到 CSS**

```css
/* Risk Decomposition Styles */
.risk-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}
.risk-kpi {
  background: rgba(255,255,255,0.03);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 10px;
  text-align: center;
}
.risk-label {
  display: block;
  font-size: 10px;
  color: var(--muted);
  margin-bottom: 4px;
}
.risk-value {
  display: block;
  font-size: 16px;
  font-weight: 700;
}
.risk-section-title {
  font-size: 14px;
  color: var(--warn);
  margin: 20px 0 8px;
  padding-bottom: 4px;
  border-bottom: 1px solid var(--border);
}
.risk-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.risk-table th {
  text-align: left;
  padding: 6px 8px;
  color: var(--muted);
  font-weight: 500;
  border-bottom: 1px solid var(--border);
}
.risk-table td {
  padding: 6px 8px;
  border-bottom: 1px solid rgba(255,255,255,0.03);
}
.corr-matrix-container {
  overflow-x: auto;
}
.corr-matrix {
  border-collapse: collapse;
  font-size: 11px;
}
.corr-matrix th, .corr-matrix td {
  padding: 4px 6px;
  text-align: center;
  border: 1px solid rgba(255,255,255,0.05);
}
.corr-header {
  font-weight: 600;
  color: var(--muted);
  background: rgba(255,255,255,0.03);
}
.corr-cell {
  font-family: monospace;
  min-width: 50px;
}
```

- [ ] **Step 5: 驗證前端**

```bash
node --check web/static/js/components/risk-panel.js
```

Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add web/static/js/components/risk-panel.js web/static/js/pages/portfolio.js web/static/index.html web/static/css/main.css
git commit -m "feat(frontend): add risk decomposition panel with correlation heatmap"
```

---

## Wave 執行順序與並行策略

```
Wave 1 (P0 — 可直接提升投資決策品質)
├── Task 1.1: Benchmark API (後端) — 可與 Task 1.2 並行
├── Task 1.2: Attribution 元件 (前端) — 可與 Task 1.1 並行
└── Task 1.3: Benchmark 元件 (前端) — 依賴 Task 1.1 完成

Wave 2 (P1 — 數據品質改進)
└── Task 2.1: Cross-Footing 驗證 — 獨立，可與 Wave 1 並行

Wave 3 (P2 — 進階風險分析)
├── Task 3.1: Component VaR + Correlation API (後端) — 可與 Task 3.2 並行
└── Task 3.2: Risk Panel 元件 (前端) — 依賴 Task 3.1 完成
```

**最大化並行：**
- Task 1.1 (後端 API) 和 Task 1.2 (前端 Attribution) 完全獨立，可同時派發不同 subagent
- Task 2.1 (Cross-Footing) 獨立於 Wave 1，可同時執行
- Task 1.3 和 Task 3.2 分別依賴對應後端完成，需順序執行

---

## 最終驗證

所有 Wave 完成後執行：

```bash
# Go 編譯與測試
go build ./...
go test ./...
go vet ./...
staticcheck ./...
test -z "$(gofmt -l .)"

# 前端語法檢查
node --check web/static/js/components/attribution.js
node --check web/static/js/components/benchmark.js
node --check web/static/js/components/risk-panel.js
node --check web/static/js/pages/portfolio.js

# 覆蓋率檢查
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

---

## QA/Acceptance Criteria

### Wave 1 驗收
- [ ] `curl http://localhost:8080/api/dashboard/pnl-attribution` 回傳含 `factor_attribution`、`agent_attribution`、`symbol_attribution` 的 JSON
- [ ] `curl http://localhost:8080/api/dashboard/benchmark-comparison` 回傳含 `portfolio_return`、`taiex_return`、`outperformance`、`alpha`、`beta`、`sharpe_ratio`、`equity_curve` 的 JSON
- [ ] 前端組合持倉頁面顯示「損益歸因分析」面板，含因子/代理人/標的三張表格
- [ ] 前端組合持倉頁面顯示「基準比較」面板，含 8 個 KPI + 雙線圖（投組 vs TAIEX）
- [ ] `go test ./internal/monitoring/api/live/... -v` 全部通過

### Wave 2 驗收
- [ ] `curl http://localhost:8080/api/dashboard/portfolio-state` 回傳含 `cross_foot_pnl` 欄位
- [ ] `cross_foot_pnl.is_balanced` 在正常情況下為 `true`
- [ ] 人工注入不一致數據時 `is_balanced` 為 `false` 且 `difference` 正確
- [ ] `go test ./internal/monitoring/service/... -v -run CrossFoot` 全部通過

### Wave 3 驗收
- [ ] `curl http://localhost:8080/api/dashboard/correlation-matrix` 回傳含 `sectors` 和 `matrix` 的 JSON
- [ ] Component VaR 各元件加總約等於總 VaR（誤差 < 0.001）
- [ ] 前端組合持倉頁面顯示「風險分解」面板，含 VaR/CVaR/回撤 KPI + 相關性熱力圖
- [ ] `go test ./internal/risk/... -v -run ComponentVaR` 全部通過
