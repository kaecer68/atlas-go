// Backtest rendering module
import { getJSON, postJSON, notify, escapeHtml, formatDate, renderEmptyState } from '../shared/app-utils.js';
import { agentName, stockName } from '../names.js';
import { fmtSafeNumber, fmtSafePct, fmtSafeSignedPct } from '../shared/format-metric.js';
import { confirmAction } from '../components/confirm-modal.js';

export function initBacktestDates() {
  const today = new Date();
  // Find most recent trading day (Monday=1 to Friday=5)
  const mostRecentTradingDay = new Date(today);
  while (mostRecentTradingDay.getDay() === 0 || mostRecentTradingDay.getDay() === 6) {
    mostRecentTradingDay.setDate(mostRecentTradingDay.getDate() - 1);
  }
  // Default to 5 trading days back
  const fiveTradingDaysAgo = new Date(mostRecentTradingDay);
  let daysBack = 0;
  while (daysBack < 5) {
    fiveTradingDaysAgo.setDate(fiveTradingDaysAgo.getDate() - 1);
    if (fiveTradingDaysAgo.getDay() !== 0 && fiveTradingDaysAgo.getDay() !== 6) {
      daysBack++;
    }
  }
  const fmt2 = (d) => d.toISOString().split('T')[0];
  const startEl = document.getElementById('backtestStart');
  const endEl = document.getElementById('backtestEnd');
  if (startEl) startEl.value = fmt2(fiveTradingDaysAgo);
  if (endEl) endEl.value = fmt2(mostRecentTradingDay);
}

// CSV export utility
export function exportTableToCSV(tableId, filename) {
  const table = document.getElementById(tableId);
  if (!table) { notify('找不到表格', 'error'); return; }
  const rows = table.querySelectorAll('tr');
  if (!rows.length) { notify('表格無資料', 'warning'); return; }
  let csv = [];
  rows.forEach(row => {
    const cells = row.querySelectorAll('th, td');
    const vals = [];
    cells.forEach(cell => {
      let text = cell.textContent.trim().replace(/"/g, '""');
      vals.push(`"${text}"`);
    });
    csv.push(vals.join(','));
  });
  const blob = new Blob(['\uFEFF' + csv.join('\n')], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename || 'export.csv';
  a.click();
  URL.revokeObjectURL(url);
  notify('CSV 匯出完成', 'success');
}

// Table pagination utility
export function paginateTable(containerId, rows, pageSize=50) {
  const container = document.getElementById(containerId);
  if (!container || rows.length <= pageSize) return null;
  const totalPages = Math.ceil(rows.length / pageSize);
  const paginationDiv = document.createElement('div');
  paginationDiv.className = 'table-pagination';
  paginationDiv.innerHTML = `
    <span>顯示 <strong>1-${pageSize}</strong> / 共 <strong>${rows.length}</strong> 筆</span>
    <div style="display:flex;gap:6px">
      <button disabled id="prevPageBtn">‹ 上一頁</button>
      <span id="pageInfo" style="line-height:24px">1 / ${totalPages}</span>
      <button id="nextPageBtn">下一頁 ›</button>
    </div>
  `;
  let currentPage = 0;
  const updatePage = (page) => {
    currentPage = page;
    const start = page * pageSize;
    const end = Math.min(start + pageSize, rows.length);
    const visibleRows = rows.slice(start, end);
    container.innerHTML = '';
    visibleRows.forEach(r => container.appendChild(r));
    const info = document.getElementById('pageInfo');
    const prevBtn = document.getElementById('prevPageBtn');
    const nextBtn = document.getElementById('nextPageBtn');
    if (info) info.textContent = `${page + 1} / ${totalPages}`;
    if (prevBtn) { prevBtn.disabled = page === 0; prevBtn.onclick = () => updatePage(page - 1); }
    if (nextBtn) { nextBtn.disabled = page >= totalPages - 1; nextBtn.onclick = () => updatePage(page + 1); }
    const span = paginationDiv.querySelector('span');
    if (span) span.innerHTML = `顯示 <strong>${start + 1}-${end}</strong> / 共 <strong>${rows.length}</strong> 筆`;
  };
  paginationDiv.querySelector('#prevPageBtn').onclick = () => updatePage(currentPage - 1);
  paginationDiv.querySelector('#nextPageBtn').onclick = () => updatePage(currentPage + 1);
  updatePage(0);
  return paginationDiv;
}

// Simple markdown-to-HTML
export function mdToHtml(md) {
  if (!md) return '';
  return String(md)
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/^### (.+)/gm, '<h3>$1</h3>')
    .replace(/^## (.+)/gm, '<h2>$1</h2>')
    .replace(/^# (.+)/gm, '<h1>$1</h1>')
    .replace(/\n/g, '<br>');
}

export async function renderBacktestReport() {
  const el = document.getElementById('backtestReport');
  try {
    const r = await fetch('/api/report/latest');
    if (!r.ok) { el.innerHTML = renderEmptyState('尚無回測報告', '執行回測後將自動產生'); el.classList.remove('loading'); return; }
    const text = await r.text();
    el.innerHTML = `<div style="font-size:13px;line-height:1.6">${mdToHtml(text)}</div>`;
    el.classList.remove('loading');
  } catch (e) {
    el.innerHTML = '<div class="empty">載入失敗: ' + e.message + '</div>';
    el.classList.remove('loading');
  }
}

export async function runBacktest() {
  const start = document.getElementById('backtestStart').value;
  const end = document.getElementById('backtestEnd').value;
  // P1-C: 回測是資源密集操作（全歷史重算 + 大量 sessions/outcomes），
  // 啟動前必須二次確認並顯示預估影響。
  const confirmed = await confirmAction({
    title: '啟動回測',
    message: `將對 ${start} ~ ${end} 期間執行完整回測：重算歷史 window、產生大量 sessions/outcomes，`
      + '屬資源密集操作，可能耗時數分鐘並佔用後台算力。確認啟動？',
    danger: true,
    confirmLabel: '確認啟動',
  });
  if (!confirmed) return;
  const statusEl = document.getElementById('backtestStatusText');
  const detailEl = document.getElementById('backtestStatusDetail');
  statusEl.textContent = '執行中…';
  detailEl.textContent = '';
  try {
    const res = await postJSON('/api/backtest/run', { start, end });
    statusEl.textContent = '已提交';
    detailEl.innerHTML = `<span class="text-accent">後台執行中，請稍後按 🔄 重新整理查看最新結果</span>`;
    notify(`回測已啟動：${start} ~ ${end}`, 'info');
    pollBacktestStatus();
  } catch (e) {
    statusEl.textContent = '失敗';
    detailEl.textContent = e.message;
    notify('回測啟動失敗: ' + e.message, 'err');
  }
}

export async function pollBacktestStatus() {
  const detailEl = document.getElementById('backtestStatusDetail');
  const statusEl = document.getElementById('backtestStatusText');
  const maxAttempts = 60;
  const pollInterval = 3000; // 3 seconds
  let attempt = 0;

  // Show initial progress indicator
  detailEl.innerHTML = `
    <div class="text-muted" style="margin-top:8px">
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px">
        <span>⏳ 等待回測完成…</span>
        <span id="pollProgressPct" style="font-size:11px;color:var(--accent)">0%</span>
      </div>
      <div style="background:var(--border);border-radius:4px;height:6px;overflow:hidden">
        <div id="pollProgressBar" style="background:var(--accent);height:100%;width:0%;transition:width 0.3s ease"></div>
      </div>
      <div style="font-size:11px;margin-top:4px">已等待 <span id="pollElapsed">0</span> 秒 / 最多 ${Math.round(maxAttempts * pollInterval / 1000)} 秒</div>
    </div>
  `;

  const startTime = Date.now();

  for (let i = 0; i < maxAttempts; i++) {
    attempt = i + 1;
    await new Promise(r => setTimeout(r, pollInterval));

    // Update progress bar
    const pct = Math.round((attempt / maxAttempts) * 100);
    const elapsed = Math.round((Date.now() - startTime) / 1000);
    const progressBar = document.getElementById('pollProgressBar');
    const progressPct = document.getElementById('pollProgressPct');
    const pollElapsed = document.getElementById('pollElapsed');
    if (progressBar) progressBar.style.width = pct + '%';
    if (progressPct) progressPct.textContent = pct + '%';
    if (pollElapsed) pollElapsed.textContent = elapsed;

    try {
      const st = await getJSON('/api/backtest/status');
      if (!st.running) {
        if (st.error) {
          statusEl.textContent = '失敗';
          detailEl.innerHTML = `<span class="text-danger">錯誤：${escapeHtml(st.error)}</span>`;
          notify('回測執行失敗: ' + st.error, 'err');
        } else {
          const totalElapsed = Math.round((Date.now() - startTime) / 1000);
          statusEl.textContent = '完成';
          detailEl.innerHTML = `<span class="text-success">完成：window ${escapeHtml(st.window_id || '')} · sessions ${st.sessions || 0} · outcomes ${st.outcomes || 0}（耗時 ${totalElapsed} 秒）</span>`;
          notify('回測已完成', 'info');
          await renderBacktestReport();
        }
        return;
      }
    } catch (e) {
      // ignore polling errors, continue retrying
    }
  }

  // Timeout reached
  statusEl.textContent = '執行中';
  detailEl.innerHTML = `
    <div class="text-muted">
      <div>⏱️ 輪詢逾時（${Math.round(maxAttempts * pollInterval / 1000)} 秒），回測可能仍在執行中</div>
      <div style="margin-top:6px">
        <button onclick="pollBacktestStatus()" style="background:var(--accent);color:var(--bg);border:none;padding:4px 12px;border-radius:4px;cursor:pointer;font-size:12px">繼續輪詢</button>
        <button onclick="renderBacktestReport()" style="background:var(--border);color:var(--fg);border:none;padding:4px 12px;border-radius:4px;cursor:pointer;font-size:12px;margin-left:8px">手動整理</button>
      </div>
    </div>
  `;
}

if (typeof window !== "undefined") Object.assign(window, { runBacktest });

// --- Backtest signals panel (admin_web reports page) ---
export function renderBacktestSignals(data) {
  const el = document.getElementById('backtestSignals');
  if (!el) return;
  el.classList.remove('loading');

  if (!data || data.status === 'not_available' || !data.active_signals) {
    el.innerHTML = renderEmptyState('尚無回測信號', '執行回測後將自動產生');
    return;
  }

  const fmtBacktestPct = v => fmtSafePct(v, 1);
  const active = Array.isArray(data.active_signals) ? data.active_signals : [];
  const signalBadges = active.length
    ? active.map(s => `<span class="badge" style="background:color-mix(in srgb,var(--accent) 15%,transparent);color:var(--accent)">${escapeHtml(String(s))}</span>`).join(' ')
    : '<span class="text-muted text-sm">無活躍信號</span>';

  el.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:12px;margin-bottom:12px">
      <div class="panel" style="text-align:center">
        <div class="kpi-label">活躍信號數</div>
        <div class="kpi-value" style="font-size:20px">${active.length}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">VaR 95</div>
        <div class="kpi-value" style="color:var(--color-danger);font-size:20px">${fmtBacktestPct(data.var_95)}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">VaR 99</div>
        <div class="kpi-value" style="color:var(--color-danger);font-size:20px">${fmtBacktestPct(data.var_99)}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">短期 Sharpe</div>
        <div class="kpi-value" style="font-size:20px">${fmtSafeNumber(data.sharpe_short, { decimals: 2 })}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">長期 Sharpe</div>
        <div class="kpi-value" style="font-size:20px">${fmtSafeNumber(data.sharpe_long, { decimals: 2 })}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">回撤 %</div>
        <div class="kpi-value" style="color:var(--down);font-size:20px">${fmtBacktestPct(data.drawdown_pct)}</div>
      </div>
    </div>
    <div style="font-size:12px;color:var(--muted)">活躍信號：${signalBadges}</div>
  `;
}

// --- Forecast vs Reality symbol-level table (admin_web reports page) ---
export function renderForecastVsRealityTable(data) {
  const el = document.getElementById('forecastVsRealityTable');
  if (!el) return;
  el.classList.remove('loading');

  const predictions = data && Array.isArray(data.symbol_predictions) ? data.symbol_predictions : [];
  if (!predictions.length) {
    el.innerHTML = renderEmptyState('尚無預測 vs 實際資料', '');
    return;
  }

  const rows = predictions.map(p => {
    const retCls = p.forward_return > 0 ? 'up' : (p.forward_return < 0 ? 'down' : '');
    const hitCls = p.hit === true ? 'ok' : (p.hit === false ? 'err' : '');
    return `<tr>
      <td style="padding:4px 8px;font-size:12px">${escapeHtml(p.symbol)}</td>
      <td style="padding:4px 8px;font-size:12px">${escapeHtml(stockName(p.symbol) || '—')}</td>
      <td style="padding:4px 8px;font-size:12px">${escapeHtml(agentName(p.agent_id) || p.agent_id || '—')}</td>
      <td style="padding:4px 8px;font-size:12px;text-align:center">${escapeHtml(p.side || '—')}</td>
      <td style="padding:4px 8px;font-size:12px;text-align:right">${fmtSafeNumber(p.conviction, { decimals: 1 })}</td>
      <td style="padding:4px 8px;font-size:12px;text-align:right">${fmtSafeNumber(p.target_price, { decimals: 2 })}</td>
      <td style="padding:4px 8px;font-size:12px;text-align:right" class="${retCls}">${fmtSafeSignedPct(p.forward_return != null ? p.forward_return * 100 : null, 1)}</td>
      <td style="padding:4px 8px;font-size:12px;text-align:center"><span class="badge ${hitCls}">${p.hit === true ? '命中' : (p.hit === false ? '未命中' : '—')}</span></td>
      <td style="padding:4px 8px;font-size:12px;text-align:center">${p.passed_guards === true ? '✓' : (p.passed_guards === false ? '✕' : '—')}</td>
      <td style="padding:4px 8px;font-size:12px">${p.recorded_at ? formatDate(p.recorded_at) : '—'}</td>
    </tr>`;
  }).join('');

  el.innerHTML = `
    <table style="width:100%;font-size:12px;border-collapse:collapse">
      <thead><tr style="border-bottom:1px solid var(--border)">
        <th style="text-align:left;padding:4px 8px">標的</th>
        <th style="text-align:left;padding:4px 8px">名稱</th>
        <th style="text-align:left;padding:4px 8px">AI</th>
        <th style="text-align:center;padding:4px 8px">方向</th>
        <th style="text-align:right;padding:4px 8px">信念</th>
        <th style="text-align:right;padding:4px 8px">目標價</th>
        <th style="text-align:right;padding:4px 8px">遠期報酬</th>
        <th style="text-align:center;padding:4px 8px">命中</th>
        <th style="text-align:center;padding:4px 8px">控制層</th>
        <th style="text-align:left;padding:4px 8px">時間</th>
      </tr></thead>
      <tbody>${rows}</tbody>
    </table>
  `;
}
