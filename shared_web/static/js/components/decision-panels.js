// decision-panels.js — 決策鏈資料面板共用組件
// 資料源：GET /api/dashboard/decision-chain（後端聚合端點，與 strategies 頁面共用，
// 由 internal/monitoring/api/decision 提供）。
// 提供三個搬移自舊 decision-chain 頁面的面板渲染：
//   - 產業熱力圖（renderSectorHeatmap）→ industry 頁面
//   - 推薦標的（renderRecommendations）→ stock-quote 頁面
//   - 出場提醒（renderExitAlerts）→ stock-quote 頁面
import { getJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { fmtSafeSigned, fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';
import { renderStockCell, stockName } from '../names.js';

function fmtPrice(v) {
  return fmtSafeNumber(v, { decimals: 2, useGrouping: true });
}

function isNum(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

/** 抓取決策鏈聚合資料（含 sector_heatmap / recommendations / exit_alerts）。 */
export async function fetchDecisionChain() {
  return getJSON('/api/dashboard/decision-chain');
}

/** 空狀態 HTML（沿用 dc-empty 樣式）。 */
export function emptyState(msg) {
  return `<div class="dc-empty" style="padding:16px;color:var(--muted);text-align:center">${msg || '尚無資料'}</div>`;
}

// --- 產業熱力圖 ---
export function renderSectorHeatmap(data) {
  const sectors = data && data.sector_heatmap;
  if (!sectors || !sectors.length) return emptyState('尚無產業數據');

  const badges = sectors.map(s => {
    const score = typeof s.confidence_score === 'number' ? s.confidence_score : (s.confidence === 'high' ? 0.8 : s.confidence === 'medium' ? 0.5 : 0.2);
    const bg = s.confidence === 'high' ? 'color-mix(in srgb, var(--color-success) 15%, transparent)' : s.confidence === 'medium' ? 'color-mix(in srgb, var(--color-warning) 15%, transparent)' : 'color-mix(in srgb, var(--muted) 10%, transparent)';
    const border = s.confidence === 'high' ? 'var(--color-success)' : s.confidence === 'medium' ? 'var(--color-warning)' : 'var(--muted)';
    const emoji = s.confidence === 'high' ? '🔥' : s.confidence === 'medium' ? '🟡' : '⚪';
    return `<div class="dc-heat-badge" style="background:${bg};border:1px solid ${border};border-radius:8px;padding:8px 10px;display:flex;align-items:center;gap:8px">
      <span style="font-size:14px">${emoji}</span>
      <div style="flex:1">
        <div style="font-size:12px;font-weight:600">${escapeHtml(s.sector)}</div>
        <div style="font-size:10px;color:var(--muted)">${(s.reasons || []).map(escapeHtml).join(' · ')}</div>
        <div style="margin-top:4px;height:4px;background:var(--border);border-radius:2px;overflow:hidden">
          <div style="width:${Math.round(score * 100)}%;height:100%;background:${border}"></div>
        </div>
      </div>
      <span class="badge ${s.confidence === 'high' ? 'ok' : s.confidence === 'medium' ? 'warn' : 'muted'}" title="信心度 ${fmtSafeNumber(score, { percent: true, decimals: 0 })}">${s.confidence}</span>
    </div>`;
  }).join('');

  return `<div class="dc-heat-grid" style="display:flex;flex-wrap:wrap;gap:8px">${badges}</div>`;
}

// --- 推薦標的 ---
export function renderRecommendations(data) {
  const recs = data && data.recommendations;
  if (!recs || !recs.length) return emptyState('暫無推薦標的');

  const rows = recs.map(r => {
    const confValid = isNum(r.confidence);
    const confPct = confValid ? Math.round(r.confidence * 100) : null;
    const confColor = confPct === null ? 'var(--muted)' : confPct >= 80 ? 'var(--color-success)' : confPct >= 60 ? 'var(--color-warning)' : 'var(--muted)';
    const actionBadge = r.action === 'buy' ? 'ok' : r.action === 'sell' || r.action === 'short' ? 'err' : 'warn';
    const price = isNum(r.price) ? fmtPrice(r.price) : '—';
    const target = isNum(r.target_price) ? fmtPrice(r.target_price) : '—';
    const stop = isNum(r.stop_loss_price) ? fmtPrice(r.stop_loss_price) : '—';
    return `<tr>
      <td>${renderStockCell(r.symbol)}</td>
      <td>${escapeHtml(r.name || stockName(r.symbol))}</td>
      <td><span class="badge ${actionBadge}">${escapeHtml(r.action)}</span></td>
      <td style="font-size:11px;text-align:right">${price}</td>
      <td style="font-size:11px;text-align:right">${target}</td>
      <td style="font-size:11px;text-align:right">${stop}</td>
      <td><span style="color:${confColor};font-weight:600">${confValid ? fmtSafeNumber(r.confidence, { percent: true, decimals: 0 }) : '—'}</span></td>
      <td style="font-size:11px;color:var(--muted)">${(r.reasons || []).map(escapeHtml).join(', ')}</td>
    </tr>`;
  }).join('');

  return `<div class="dc-table-wrap"><table class="dc-table">
    <thead><tr><th>代號</th><th>名稱</th><th>方向</th><th>參考價</th><th>目標價</th><th>止損價</th><th>置信度</th><th>原因</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
}

// --- 出場提醒 ---
export function renderExitAlerts(data) {
  const alerts = data && data.exit_alerts;
  if (!alerts || !alerts.length) return emptyState('目前沒有需要出場提醒的持倉');

  const rows = alerts.map(a => {
    const pnlValid = isNum(a.pnl_pct);
    // ExitAlert.pnl_pct 為百分點（15.0 = +15%），後端 computeExitAlerts 已轉換。
    const pnlText = pnlValid ? fmtSafeSignedPct(a.pnl_pct) : '—';
    const badgeClass = !pnlValid ? 'muted' : a.pnl_pct >= 10 ? 'up' : a.pnl_pct <= -5 ? 'down' : 'warn';
    return `<div class="dc-exit-row">
      <span>🔔 ${renderStockCell(a.symbol)} ${escapeHtml(a.name && a.name !== a.symbol ? a.name : '')}</span>
      <span class="badge ${badgeClass}">
        ${pnlText}
      </span>
      <span style="font-size:11px;color:var(--muted);flex:1;text-align:right">${escapeHtml(a.suggestion)}</span>
    </div>`;
  }).join('');

  return `<div class="dc-section">${rows || emptyState('目前沒有需要出場提醒的持倉')}</div>`;
}
