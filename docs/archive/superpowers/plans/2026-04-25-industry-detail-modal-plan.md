# 產業詳細分析彈窗實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 擴展 `showIndustryDetail()` 函式，實作一個整合週期定位、供應鏈連動、季節性模式、風險分析四個維度的 Modal 彈窗。

**Architecture:** 前端在 `index.html` 新增 Modal HTML 結構、CSS 樣式、JavaScript 邏輯；後端調整 `/api/industry/risk` 支援產業層級查詢（symbol=ALL）。

**Tech Stack:** HTML/CSS/JavaScript (vanilla), Go 1.25

---

## 檔案結構

| 檔案 | 職責 |
|------|------|
| `web/static/index.html` | 新增 Modal HTML、CSS、JavaScript |
| `internal/monitoring/dashboard_api.go` | 調整 `handleIndustryRisk` 支援 `symbol=ALL` |

---

## Task 1: 新增產業詳細分析 Modal HTML 結構

**Files:**
- Modify: `web/static/index.html`（在現有 modal 之後插入）

- [ ] **Step 1: 在 `infoModal` 之後插入新的 `industryModal`**

找到 `infoModal` 的結束標籤 `</div>`（約第 580 行），在其後插入：

```html
<!-- Modal: Industry Detail -->
<div class="modal-overlay" id="industryModal" role="dialog" aria-modal="true" onclick="if(event.target===this)closeIndustryModal()">
  <div class="modal" style="width:min(720px,94vw)">
    <h3 id="industryModalTitle">產業詳細分析</h3>
    <div class="industry-tabs" id="industryTabs">
      <button class="tab-btn active" data-tab="cycle" onclick="switchIndustryTab('cycle')">週期定位</button>
      <button class="tab-btn" data-tab="linkage" onclick="switchIndustryTab('linkage')">供應鏈</button>
      <button class="tab-btn" data-tab="seasonality" onclick="switchIndustryTab('seasonality')">季節性</button>
      <button class="tab-btn" data-tab="risk" onclick="switchIndustryTab('risk')">風險</button>
    </div>
    <div id="industryModalContent"><div class="empty">載入中…</div></div>
    <div class="control-group" style="margin-top:14px;justify-content:flex-end">
      <button onclick="closeIndustryModal()">關閉</button>
    </div>
  </div>
</div>
```

- [ ] **Step 2: 驗證 HTML 結構正確**

檢查：
- `industryModal` 位於 `infoModal` 之後、`</body>` 之前
- 所有標籤正確閉合
- `onclick` 事件綁定正確

---

## Task 2: 新增產業詳細分析 Modal CSS 樣式

**Files:**
- Modify: `web/static/index.html`（在 `<style>` 區塊內新增）

- [ ] **Step 1: 在現有 CSS 之後新增 Tab 樣式**

找到 `.modal` 相關樣式（約第 86-91 行），在其後插入：

```css
  .industry-tabs { display: flex; gap: 4px; margin-bottom: 14px; border-bottom: 1px solid var(--border); flex-wrap: wrap; }
  .tab-btn { background: transparent; border: none; color: var(--muted); padding: 8px 14px; cursor: pointer; font-size: 13px; border-bottom: 2px solid transparent; margin-bottom: -1px; transition: color .15s, border-color .15s; }
  .tab-btn.active { color: var(--accent); border-bottom-color: var(--accent); }
  .tab-btn:hover { color: var(--text); }
  .industry-section { margin-bottom: 16px; }
  .industry-section h4 { margin: 0 0 8px; font-size: 13px; color: var(--accent); }
  .cycle-visual { display: flex; align-items: center; gap: 8px; margin: 12px 0; padding: 12px; background: var(--bg); border-radius: 8px; }
  .cycle-phase { padding: 4px 10px; border-radius: 6px; font-size: 12px; background: var(--border); color: var(--muted); }
  .cycle-phase.active { background: var(--accent); color: var(--bg); }
  .linkage-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
  .linkage-col h4 { margin: 0 0 6px; font-size: 12px; color: var(--muted); }
  .linkage-item { background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 6px 10px; font-size: 12px; margin-bottom: 6px; }
  .risk-item { background: var(--bg); border: 1px solid var(--border); border-radius: 6px; padding: 8px 10px; margin-bottom: 6px; }
  .risk-item .risk-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px; }
  .risk-severity { font-size: 11px; padding: 2px 8px; border-radius: 999px; }
  .risk-severity.high { background: rgba(217,58,58,.15); color: var(--down); border: 1px solid rgba(217,58,58,.3); }
  .risk-severity.medium { background: rgba(245,166,35,.15); color: var(--warn); border: 1px solid rgba(245,166,35,.3); }
  .risk-severity.low { background: rgba(38,161,123,.15); color: var(--up); border: 1px solid rgba(38,161,123,.3); }
  .seasonal-pattern { background: var(--bg); border: 1px solid var(--border); border-radius: 8px; padding: 10px; margin-bottom: 8px; }
  .seasonal-pattern .pattern-name { font-weight: 600; font-size: 13px; margin-bottom: 4px; }
  .seasonal-pattern .pattern-meta { font-size: 11px; color: var(--muted); }
  .metric-row { display: flex; justify-content: space-between; padding: 6px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
  .metric-row:last-child { border-bottom: none; }
  .metric-row .metric-label { color: var(--muted); }
  .metric-row .metric-value { font-weight: 600; }
```

- [ ] **Step 2: 驗證 CSS 語法**

檢查：
- 所有 CSS 規則語法正確
- 使用現有 CSS 變數（`var(--accent)`, `var(--bg)` 等）
- 與現有樣式無衝突

---

## Task 3: 實作 showIndustryDetail() 與相關 JavaScript 函式

**Files:**
- Modify: `web/static/index.html`（在 `showIndustryDetail` 函式位置）

- [ ] **Step 1: 替換現有的 `showIndustryDetail` 函式**

找到現有的：
```javascript
function showIndustryDetail(industryId) {
  notify(`產業詳細分析功能開發中: ${industryId}`, 'info');
}
```

替換為：

```javascript
let currentIndustryId = '';
let currentIndustryData = {};
let currentIndustryName = '';

async function showIndustryDetail(industryId) {
  currentIndustryId = industryId;
  document.getElementById('industryModal').classList.add('show');
  document.getElementById('industryModalTitle').textContent = '載入中...';
  document.getElementById('industryModalContent').innerHTML = '<div class="empty">載入中…</div>';
  
  // Reset tabs
  document.querySelectorAll('#industryTabs .tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.tab === 'cycle');
  });
  
  try {
    const [cycle, linkage, seasonality, risk] = await Promise.all([
      getJSON(`/api/industry/cycle?industry=${encodeURIComponent(industryId)}`).catch(() => null),
      getJSON(`/api/industry/linkage?industry=${encodeURIComponent(industryId)}`).catch(() => null),
      getJSON(`/api/industry/seasonality?industry=${encodeURIComponent(industryId)}`).catch(() => null),
      getJSON(`/api/industry/risk?industry=${encodeURIComponent(industryId)}&symbol=ALL`).catch(() => null),
    ]);
    
    currentIndustryData = { cycle, linkage, seasonality, risk };
    currentIndustryName = (cycle && cycle.industry_name) || industryId;
    document.getElementById('industryModalTitle').textContent = `${currentIndustryName} - 產業詳細分析`;
    renderIndustryTab('cycle');
  } catch (e) {
    console.error('showIndustryDetail error:', e);
    document.getElementById('industryModalContent').innerHTML = '<div class="empty">載入失敗，請稍後再試</div>';
  }
}

function closeIndustryModal() {
  document.getElementById('industryModal').classList.remove('show');
  currentIndustryId = '';
  currentIndustryData = {};
  currentIndustryName = '';
}

function switchIndustryTab(tab) {
  document.querySelectorAll('#industryTabs .tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.tab === tab);
  });
  renderIndustryTab(tab);
}

function renderIndustryTab(tab) {
  const data = currentIndustryData[tab];
  const el = document.getElementById('industryModalContent');
  
  if (!data) {
    el.innerHTML = '<div class="empty">無資料</div>';
    return;
  }
  
  switch (tab) {
    case 'cycle':
      el.innerHTML = renderIndustryCycleTab(data);
      break;
    case 'linkage':
      el.innerHTML = renderIndustryLinkageTab(data);
      break;
    case 'seasonality':
      el.innerHTML = renderIndustrySeasonalityTab(data);
      break;
    case 'risk':
      el.innerHTML = renderIndustryRiskTab(data);
      break;
  }
}

function renderIndustryCycleTab(data) {
  const cycleColors = {
    recovery: '#10b981',
    expansion: '#3b82f6',
    mature: '#f59e0b',
    recession: '#ef4444'
  };
  const cycleNames = {
    recovery: '復甦',
    expansion: '擴張',
    mature: '成熟',
    recession: '衰退'
  };
  
  const businessColor = cycleColors[data.business_cycle] || '#666';
  const businessName = cycleNames[data.business_cycle] || data.business_cycle || '未知';
  
  let html = '<div class="industry-section">';
  html += '<h4>週期狀態</h4>';
  html += '<div class="metric-row"><span class="metric-label">景氣循環</span><span class="metric-value" style="color:' + businessColor + '">' + businessName + '</span></div>';
  html += '<div class="metric-row"><span class="metric-label">庫存週期</span><span class="metric-value">' + (data.inventory_cycle || '-') + '</span></div>';
  html += '<div class="metric-row"><span class="metric-label">資本支出週期</span><span class="metric-value">' + (data.capex_cycle || '-') + '</span></div>';
  html += '<div class="metric-row"><span class="metric-label">信心度</span><span class="metric-value">' + Math.round((data.confidence || 0) * 100) + '%</span></div>';
  html += '<div class="metric-row"><span class="metric-label">趨勢</span><span class="metric-value">' + (data.trend || '-') + '</span></div>';
  html += '<div class="metric-row"><span class="metric-label">階段分數</span><span class="metric-value">' + (data.phase_score || 0).toFixed(2) + '</span></div>';
  html += '</div>';
  
  // Cycle visualization
  html += '<div class="industry-section">';
  html += '<h4>週期階段</h4>';
  html += '<div class="cycle-visual">';
  ['recovery', 'expansion', 'mature', 'recession'].forEach(phase => {
    const isActive = data.business_cycle === phase;
    html += '<div class="cycle-phase ' + (isActive ? 'active' : '') + '" style="' + (isActive ? 'background:' + cycleColors[phase] + ';color:#fff' : '') + '">' + cycleNames[phase] + '</div>';
    if (phase !== 'recession') {
      html += '<span style="color:var(--muted)">→</span>';
    }
  });
  html += '</div>';
  html += '</div>';
  
  return html;
}

function renderIndustryLinkageTab(data) {
  let html = '<div class="industry-section">';
  html += '<h4>連動分數</h4>';
  html += '<div class="metric-row"><span class="metric-label">總分</span><span class="metric-value">' + (data.linkage_score || 0).toFixed(2) + '</span></div>';
  html += '</div>';
  
  html += '<div class="linkage-grid">';
  
  // Upstream
  html += '<div class="linkage-col">';
  html += '<h4>上游產業</h4>';
  if (data.upstream && data.upstream.length > 0) {
    data.upstream.forEach(item => {
      html += '<div class="linkage-item">' + item + '</div>';
    });
  } else {
    html += '<div class="empty">無資料</div>';
  }
  html += '</div>';
  
  // Downstream
  html += '<div class="linkage-col">';
  html += '<h4>下游產業</h4>';
  if (data.downstream && data.downstream.length > 0) {
    data.downstream.forEach(item => {
      html += '<div class="linkage-item">' + item + '</div>';
    });
  } else {
    html += '<div class="empty">無資料</div>';
  }
  html += '</div>';
  
  html += '</div>';
  
  // Correlations
  if (data.correlations && data.correlations.length > 0) {
    html += '<div class="industry-section">';
    html += '<h4>相關性分析</h4>';
    html += '<table><thead><tr><th>產業</th><th>相關係數</th><th>強度</th></tr></thead><tbody>';
    data.correlations.forEach(c => {
      const corrColor = c.correlation > 0 ? 'var(--up)' : 'var(--down)';
      html += '<tr><td>' + c.industry + '</td><td style="color:' + corrColor + '"\u003e' + c.correlation.toFixed(2) + '</td><td>' + c.strength + '</td></tr>';
    });
    html += '</tbody></table>';
    html += '</div>';
  }
  
  return html;
}

function renderIndustrySeasonalityTab(data) {
  let html = '<div class="industry-section">';
  html += '<h4>季節性調整</h4>';
  html += '<div class="metric-row"><span class="metric-label">調整係數</span><span class="metric-value">' + (data.adjustment || 0).toFixed(2) + '</span></div>';
  html += '<div class="metric-row"><span class="metric-label">當前日期</span><span class="metric-value">' + (data.current_date || '-') + '</span></div>';
  html += '</div>';
  
  if (data.active_patterns && data.active_patterns.length > 0) {
    html += '<div class="industry-section">';
    html += '<h4>活躍模式 (' + data.pattern_count + ')</h4>';
    data.active_patterns.forEach(p => {
      html += '<div class="seasonal-pattern">';
      html += '<div class="pattern-name">' + p.name + '</div>';
      html += '<div class="pattern-meta">' + (p.description || '') + '</div>';
      html += '<div class="pattern-meta" style="margin-top:4px">';
      html += '期間: ' + p.start_month + '/' + p.start_day + ' ~ ' + p.end_month + '/' + p.end_day + ' | ';
      html += '歷史準確度: ' + Math.round((p.historical_accuracy || 0) * 100) + '% | ';
      html += '典型報酬: ' + (p.typical_return || 0).toFixed(1) + '%';
      html += '</div>';
      html += '</div>';
    });
    html += '</div>';
  } else {
    html += '<div class="empty">目前無活躍季節性模式</div>';
  }
  
  return html;
}

function renderIndustryRiskTab(data) {
  let html = '';
  
  if (data.highest_risk) {
    html += '<div class="industry-section">';
    html += '<h4>最高風險</h4>';
    html += '<div class="risk-item">';
    html += '<div class="risk-header">';
    html += '<span style="font-weight:600">' + data.highest_risk.type + '</span>';
    html += '<span class="risk-severity ' + data.highest_risk.severity + '"\u003e' + data.highest_risk.severity + '</span>';
    html += '</div>';
    html += '<div style="font-size:12px;color:var(--muted)">' + data.highest_risk.description + '</div>';
    html += '</div>';
    html += '</div>';
  }
  
  if (data.risks && data.risks.length > 0) {
    html += '<div class="industry-section">';
    html += '<h4>風險列表 (' + data.risks.length + ')</h4>';
    data.risks.forEach(r => {
      html += '<div class="risk-item">';
      html += '<div class="risk-header">';
      html += '<span style="font-weight:600">' + r.type + '</span>';
      html += '<span class="risk-severity ' + r.severity + '"\u003e' + r.severity + '</span>';
      html += '</div>';
      html += '<div style="font-size:12px;color:var(--muted);margin-bottom:4px">' + r.description + '</div>';
      html += '<div style="font-size:11px">影響估計: ' + (r.impact_estimate || 0).toFixed(2) + ' | 信心度: ' + Math.round((r.confidence || 0) * 100) + '%</div>';
      if (r.recommended_action) {
        html += '<div style="font-size:11px;color:var(--accent);margin-top:4px">建議: ' + r.recommended_action + '</div>';
      }
      html += '</div>';
    });
    html += '</div>';
  } else {
    html += '<div class="empty">目前無風險資料</div>';
  }
  
  return html;
}
```

- [ ] **Step 2: 驗證 JavaScript 語法**

檢查：
- 所有函式定義正確
- 無語法錯誤（括號配對、引號配對）
- 使用現有 `getJSON` 輔助函式
- 錯誤處理完善

---

## Task 4: 調整後端 API 支援產業層級風險查詢

**Files:**
- Modify: `internal/monitoring/dashboard_api.go`

- [ ] **Step 1: 修改 `handleIndustryRisk` 函式**

找到 `handleIndustryRisk` 函式（約第 3322 行），將 `symbol` 參數從 required 改為 optional：

```go
func (a *DashboardAPI) handleIndustryRisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	symbol := r.URL.Query().Get("symbol")
	industryID := r.URL.Query().Get("industry")
	
	var risks []industry.RiskItem
	if symbol == "ALL" && industryID != "" {
		// 產業層級查詢：取得該產業下所有標的的風險
		risks = a.riskMonitor.GetAllRisksForIndustry(industryID)
	} else if symbol != "" {
		risks = a.riskMonitor.GetAllRisks(symbol, industryID, 0, 0)
	} else {
		writeJSONError(w, http.StatusBadRequest, "symbol or industry parameter required")
		return
	}
	
	// ... 其餘邏輯不變
```

**注意**: 需要確認 `industry.RiskMonitor` 是否有 `GetAllRisksForIndustry` 方法。如果沒有，需要：

1. 在 `internal/industry/` 套件新增該方法，或
2. 使用現有方法組合實現

- [ ] **Step 2: 檢查 `internal/industry` 套件**

執行：
```bash
grep -r "GetAllRisks" internal/industry/
grep -r "RiskMonitor" internal/industry/
```

確認：
- `RiskMonitor` 結構體定義
- `GetAllRisks` 方法簽名
- 是否需要新增 `GetAllRisksForIndustry` 方法

---

## Task 5: 編譯與測試

- [ ] **Step 1: 編譯 Go 程式**

```bash
cd /Users/kaecer/workspace/atlas
go build ./...
```

預期：編譯成功，無錯誤

- [ ] **Step 2: 執行測試**

```bash
go test ./internal/monitoring/...
go test ./internal/industry/...
```

預期：測試通過

- [ ] **Step 3: 格式檢查**

```bash
test -z "$(gofmt -l .)"
```

預期：無輸出（表示所有檔案已格式化）

---

## Task 6: 重啟應用服務

- [ ] **Step 1: 停止現有服務**

```bash
pkill -f "go run ./cmd/atlas" || true
pkill -f "./atlas" || true
```

- [ ] **Step 2: 啟動服務**

```bash
cd /Users/kaecer/workspace/atlas
go run ./cmd/atlas &
```

- [ ] **Step 3: 驗證服務啟動**

```bash
curl -s http://localhost:8080/health
```

預期：回傳健康狀態 JSON

- [ ] **Step 4: 驗證 API**

```bash
curl -s "http://localhost:8080/api/industry/cycle?industry=semi"
curl -s "http://localhost:8080/api/industry/risk?industry=semi&symbol=ALL"
```

預期：回傳對應 JSON 資料

---

## Task 7: 前端驗證

- [ ] **Step 1: 開啟瀏覽器驗證**

開啟 `http://localhost:8080`，切換到「產業生態系」頁面

- [ ] **Step 2: 點擊產業卡片**

點擊任意產業卡片，驗證：
- 彈窗正確開啟
- 顯示「載入中…」
- 載入完成後顯示週期定位內容
- 產業名稱正確顯示在標題

- [ ] **Step 3: 切換 Tab**

點擊「供應鏈」、「季節性」、「風險」Tab，驗證：
- 內容正確切換
- 資料正確顯示

- [ ] **Step 4: 關閉彈窗**

點擊「關閉」按鈕或彈窗外部，驗證彈窗正確關閉

---

## 驗收標準

- [ ] 點擊產業卡片開啟彈窗
- [ ] 彈窗顯示產業名稱
- [ ] 四個 Tab 可正常切換
- [ ] 週期定位 Tab 顯示：景氣循環、庫存週期、資本支出週期、信心度、趨勢
- [ ] 供應鏈 Tab 顯示：上游、下游、相關性矩陣
- [ ] 季節性 Tab 顯示：活躍模式、歷史準確度
- [ ] 風險 Tab 顯示：風險列表、最高風險
- [ ] 點擊彈窗外部或關閉按鈕可關閉
- [ ] 支援深色/淺色主題
- [ ] 行動裝置友善（寬度自適應）
- [ ] Go 編譯通過
- [ ] 測試通過

## 風險與注意事項

1. **API 相容性**: `/api/industry/risk` 目前要求 `symbol` 參數，調整後需確保現有呼叫不受影響
2. **效能**: 四個 API 並行呼叫，任一失敗不影響其他資料顯示
3. **錯誤處理**: 每個 API 失敗時顯示對應錯誤訊息，不讓整個彈窗崩潰
4. **industry 套件**: 若 `RiskMonitor` 無 `GetAllRisksForIndustry` 方法，需額外實作
