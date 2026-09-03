// shared_web/static/js/pages/period-matrix.js
//
// 「策略 × 時期」熱圖（admin 管理台，capital-flow Phase 2 PR-2c）。
// 資料源：GET /api/strategy/period-matrix（PR-2b 的唯讀聚合端點）。
// 每格 = 一個策略(agent) 在一個市場時期(7 期)的實績：
//   - 樣本 >= min_samples(30) → 依勝率上色，顯示勝率 + Sharpe + 樣本數
//   - 樣本 <  30 → 灰底「資料不足（n=N）」（API status=insufficient_data）
// 點格子 → 下方詳情面板顯示 sample_count / win_rate / sharpe / avg_return。

import { silentGetJSON, escapeHtml, renderEmptyState, renderErrorState } from '../shared/app-utils.js';
import { fmtSafePct, fmtSafeNumber } from '../shared/format-metric.js';

const RETRY_ID = 'period-matrix';

// 七時期中文標籤（docs/ATLAS_METHODOLOGY.md §3；與 methodology/home 一致）。
export const PERIOD_LABELS = {
  downturn: '低迷',
  turnaround_up: '轉折開高',
  bull: '上升',
  plateau: '高原',
  consolidation: '盤整',
  turnaround_down: '轉折下壓',
  black_swan: '黑天鵝',
};

// colorForWinRate maps a win rate (0..1) to a CSS background color.
// 0.5 = neutral; >0.5 green (多頭勝率高的時期策略), <0.5 red. Pure + tested.
export function colorForWinRate(winRate) {
  if (!(winRate >= 0)) return 'transparent';
  const ratio = Math.max(0, Math.min(1, winRate));
  // hue: 0 (red) → 120 (green) over the [0.35, 0.65] band, clamped outside.
  let t = (ratio - 0.35) / 0.3; // 0 at 35%, 1 at 65%
  t = Math.max(0, Math.min(1, t));
  const hue = Math.round(120 * t);
  const light = 32 + Math.round(8 * (1 - t)); // darker on weak side
  return 'hsl(' + hue + ', 55%, ' + light + '%)';
}

// cellLabel renders one cell's main text.
export function cellLabel(cell) {
  if (cell.status !== 'ok' || !(cell.win_rate >= 0)) {
    return '資料不足';
  }
  return fmtSafePct(cell.win_rate, 1);
}

// buildPeriodMatrixHtml renders the whole heat table (pure string builder,
// exercised by node --test without a DOM).
export function buildPeriodMatrixHtml(data) {
  if (!data || !Array.isArray(data.cells)) return '';
  const periods = Array.isArray(data.periods) && data.periods.length > 0 ? data.periods : Object.keys(PERIOD_LABELS);
  const byAgent = {};
  const agents = [];
  data.cells.forEach(function (cell) {
    const key = cell.agent_id || 'unknown';
    if (!byAgent[key]) { byAgent[key] = {}; agents.push(key); }
    byAgent[key][cell.market_period] = cell;
  });
  agents.sort();

  const esc = escapeHtml;
  const header = '<th class="pm-cell pm-corner">策略 \\ 時期</th>'
    + periods.map(function (p) {
        const zh = PERIOD_LABELS[p] || p;
        return '<th class="pm-cell pm-period-head" data-period="' + esc(p) + '">' + esc(zh) + '</th>';
      }).join('');

  const rows = agents.map(function (agent) {
    const cells = periods.map(function (p) {
      const cell = byAgent[agent] && byAgent[agent][p];
      if (!cell) {
        return '<td class="pm-cell pm-cell-empty" title="' + esc(agent) + ' · ' + esc(PERIOD_LABELS[p] || p) + ' 無樣本">—</td>';
      }
      const agentSafe = esc(agent);
      const periodSafe = esc(p);
      if (cell.status !== 'ok' || !(cell.win_rate >= 0)) {
        const n = typeof cell.sample_count === 'number' ? cell.sample_count : 0;
        return '<td class="pm-cell pm-cell-insufficient" data-agent="' + agentSafe + '" data-period="' + periodSafe + '"'
          + ' title="' + agentSafe + ' · ' + esc(PERIOD_LABELS[p] || p) + '：樣本不足（n=' + n + '）">'
          + '<span class="pm-cell-main">資料不足</span>'
          + '<span class="pm-cell-sub">n=' + n + '</span></td>';
      }
      const wr = cell.win_rate;
      const bg = colorForWinRate(wr);
      const sharpe = typeof cell.sharpe === 'number' ? cell.sharpe : null;
      const avg = typeof cell.avg_return === 'number' ? cell.avg_return : null;
      const title = agentSafe + ' · ' + esc(PERIOD_LABELS[p] || p)
        + '｜樣本 ' + cell.sample_count
        + '｜勝率 ' + fmtSafePct(wr, 1)
        + (sharpe !== null ? '｜Sharpe ' + fmtSafeNumber(sharpe, { decimals: 2 }) : '')
        + (avg !== null ? '｜平均報酬 ' + fmtSafePct(avg, 2) : '');
      return '<td class="pm-cell pm-cell-ok" data-agent="' + agentSafe + '" data-period="' + periodSafe + '"'
        + ' style="background-color:' + bg + '" title="' + title + '">'
        + '<span class="pm-cell-main">' + cellLabel(cell) + '</span>'
        + '<span class="pm-cell-sub">' + (sharpe !== null ? 'S ' + fmtSafeNumber(sharpe, { decimals: 2 }) : '') + '</span></td>';
    }).join('');
    return '<tr data-agent="' + esc(agent) + '"><th class="pm-cell pm-agent-head">' + esc(agent) + '</th>' + cells + '</tr>';
  }).join('');

  return '<table class="pm-heat-table"><thead><tr>' + header + '</tr></thead><tbody>' + rows + '</tbody></table>';
}

// renderSummaryChips renders source/degraded/min-samples meta above the table.
export function renderSummaryChips(container, data) {
  if (!container) return;
  if (!data) { container.innerHTML = ''; return; }
  const source = data.source || '—';
  const degraded = data.degraded ? '（PG 不可用，降級 JSONL）' : '';
  const minSamples = typeof data.min_samples === 'number' ? data.min_samples : 30;
  container.innerHTML = ''
    + '<span class="badge">資料源 ' + escapeHtml(source) + degraded + '</span>'
    + '<span class="badge">最小樣本 ' + minSamples + '</span>'
    + '<span class="badge">細胞數 ' + (Array.isArray(data.cells) ? data.cells.length : 0) + '</span>';
}

// renderPeriodMatrix paints the heat table + wires click-to-detail delegation.
// container: element with #periodMatrixDetail sibling expected by loadPeriodMatrix.
export function renderPeriodMatrix(container, data, detailEl) {
  if (!container) return;
  if (!data || !Array.isArray(data.cells) || data.cells.length === 0) {
    container.innerHTML = renderEmptyState('目前沒有已分類的 outcomes（market_period 資料不足或尚未回填）');
    return;
  }
  container.innerHTML = buildPeriodMatrixHtml(data);
  container.classList.remove('loading');
  if (container.onCellClick) { /* no-op: delegation handled by loadPeriodMatrix */ }
  if (detailEl) { detailEl.innerHTML = ''; }
}

export async function loadPeriodMatrix() {
  const tableEl = document.getElementById('periodMatrixHeatmap');
  const detailEl = document.getElementById('periodMatrixDetail');
  const metaEl = document.getElementById('periodMatrixMeta');
  if (!tableEl) return;
  tableEl.classList.add('loading');
  try {
    const data = await silentGetJSON('/api/strategy/period-matrix');
    if (data === null) {
      tableEl.classList.remove('loading');
      tableEl.innerHTML = renderErrorState('策略×時期熱圖', RETRY_ID);
      const btn = tableEl.querySelector('[data-retry="' + RETRY_ID + '"]');
      if (btn) btn.addEventListener('click', loadPeriodMatrix);
      return;
    }
    renderSummaryChips(metaEl, data);
    renderPeriodMatrix(tableEl, data, detailEl);
    attachCellDetail(tableEl, detailEl);
  } catch (err) {
    console.error('[period-matrix] load failed', err);
    tableEl.classList.remove('loading');
    tableEl.innerHTML = renderErrorState('策略×時期熱圖', RETRY_ID);
    const btn = tableEl.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadPeriodMatrix);
  }
}

// attachCellDetail shows the clicked cell's full stats in the detail panel.
export function attachCellDetail(tableEl, detailEl) {
  if (!tableEl || !detailEl) return;
  tableEl.addEventListener('click', function (ev) {
    const td = ev.target && ev.target.closest ? ev.target.closest('td.pm-cell[data-agent]') : null;
    if (!td) return;
    const agent = td.getAttribute('data-agent');
    const period = td.getAttribute('data-period');
    const title = td.getAttribute('title') || '';
    detailEl.innerHTML = '<strong>' + escapeHtml(agent) + ' × ' + escapeHtml(PERIOD_LABELS[period] || period) + '</strong>'
      + '<span class="text-muted"> ' + escapeHtml(title) + '</span>';
  });
}
