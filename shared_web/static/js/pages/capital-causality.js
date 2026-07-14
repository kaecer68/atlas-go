// shared_web/static/js/pages/capital-causality.js
//
// Stage 6 PR#2：admin_web「錢潮因果」頁。
// 從 GET /api/narrative/templates 讀 24 個因果模板，
// 用 <details>/<summary> 展開每個 template 的因果鏈步驟。

import { silentGetJSON, escapeHtml, renderEmptyState } from '../shared/app-utils.js';

export async function loadCapitalCausality() {
  const data = await silentGetJSON('/api/narrative/templates');
  renderCapitalCausality(data);
}

export function renderCapitalCausality(data) {
  const el = document.getElementById('capitalCausalityContent');
  if (!el) return;
  const templates = data && Array.isArray(data.templates) ? data.templates : [];
  if (templates.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = renderEmptyState('尚無因果模板');
    return;
  }
  const items = templates.map(function (t) {
    const steps = Array.isArray(t.steps) && t.steps.length > 0
      ? t.steps.map(function (s, i) {
          var label = typeof s === 'string' ? s : (s.label || s.text || JSON.stringify(s));
          return '<li>' + escapeHtml(label) + '</li>';
        }).join('')
      : '<li>（未定義步驟）</li>';
    var hitRate = typeof t.hit_rate === 'number'
      ? (t.hit_rate * 100).toFixed(1) + '%'
      : '—';
    return '<details class="causality-item">'
      + '<summary><code>' + escapeHtml(t.trigger_theme || t.id || t.name || '-') + '</code>'
      + '<span class="hit-rate">命中率 ' + hitRate + '</span></summary>'
      + '<ol>' + steps + '</ol>'
      + '</details>';
  }).join('');
  el.classList.remove('loading');
  el.innerHTML = '<div class="causality-list">' + items + '</div>';
}
