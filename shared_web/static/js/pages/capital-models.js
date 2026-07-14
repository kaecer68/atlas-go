// shared_web/static/js/pages/capital-models.js
//
// Stage 6 PR#2：admin_web「錢潮模型」頁。
// 從 GET /api/narrative/models 讀 narrative engine 的 active models，
// 列出每個模型的權重、近期誤差、歷史命中率與推理依據。

import { silentGetJSON, escapeHtml, renderEmptyState } from '../shared/app-utils.js';

export async function loadCapitalModels() {
  const data = await silentGetJSON('/api/narrative/models');
  renderCapitalModels(data);
}

export function renderCapitalModels(data) {
  const el = document.getElementById('capitalModelsContent');
  if (!el) return;
  const models = data && Array.isArray(data.models) ? data.models : [];
  if (models.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = renderEmptyState('目前沒有 narrative model 啟用');
    return;
  }
  const rows = models.map(function (m) {
    return '<tr>'
      + '<td><code>' + escapeHtml(m.name || m.id || '-') + '</code></td>'
      + '<td>' + (typeof m.weight === 'number' ? m.weight.toFixed(4) : '—') + '</td>'
      + '<td>' + escapeHtml(m.recent_error == null ? '-' : String(m.recent_error)) + '</td>'
      + '<td>' + (typeof m.hit_rate === 'number' ? (m.hit_rate * 100).toFixed(1) + '%' : '—') + '</td>'
      + '<td>' + escapeHtml(m.rationale || '-') + '</td>'
      + '</tr>';
  }).join('');
  el.classList.remove('loading');
  el.innerHTML = '<div class="table-scroll mt-sm">'
    + '<table class="ranker-table">'
    +   '<thead><tr><th>模型名稱</th><th>權重</th><th>近期誤差</th><th>歷史命中率</th><th>推理依據</th></tr></thead>'
    +   '<tbody>' + rows + '</tbody>'
    + '</table>'
    + '</div>';
}
