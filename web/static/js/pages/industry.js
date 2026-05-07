// Industry ecosystem page
import { sectorName } from '../names.js';
import { getJSON, notify } from '../shared/app-utils.js';

export async function loadIndustryData() {
  try {
    const [classification, overview, seasonality, calendar] = await Promise.all([
      getJSON('/api/dashboard/industry-classification').catch(() => null),
      getJSON('/api/dashboard/industry-overview').catch(() => null),
      getJSON('/api/dashboard/industry-seasonality').catch(() => null),
      getJSON('/api/dashboard/industry-seasonality-calendar').catch(() => null),
    ]);
    renderIndustryMap(classification);
    renderIndustryCycle(overview);
    renderIndustryLinkage(overview);
    if (seasonality && calendar) {
      seasonality.calendar = calendar;
    }
    renderIndustrySeasonality(seasonality);
  } catch (e) { console.error('loadIndustryData error:', e); }
}

export function renderIndustryMap(data) {
  const el = document.getElementById('industryMap');
  if (!data || !data.industries) { el.innerHTML = renderEmptyState('尚無產業資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const industries = data.industries;
  let html = '<div style="display:flex;flex-wrap:wrap;gap:10px">';
  industries.forEach(ind => {
    const weightPct = Math.round((ind.weight || 0) * 100);
    html += `<div style="flex:1;min-width:140px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;cursor:pointer" onclick="showIndustryDetail('${ind.id}')">`;
    html += `<div style="font-weight:700;font-size:14px;margin-bottom:4px">${ind.name}</div>`;
    html += `<div style="font-size:12px;color:var(--muted)">權重 ${weightPct}%</div>`;
    html += `<div style="margin-top:6px;height:4px;background:var(--border);border-radius:2px;overflow:hidden">`;
    html += `<div style="width:${weightPct}%;height:100%;background:var(--accent)"></div></div>`;
    html += `</div>`;
  });
  html += '</div>';
  el.innerHTML = html;
}

export function renderIndustryCycle(data) {
  const el = document.getElementById('industryCycle');
  if (!data || !data.industries) { el.innerHTML = renderEmptyState('尚無週期資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const industries = data.industries;
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
  let html = '<div style="display:flex;flex-wrap:wrap;gap:10px">';
  industries.forEach(ind => {
    const color = cycleColors[ind.cycle_phase] || '#666';
    const name = cycleNames[ind.cycle_phase] || ind.cycle_phase;
    html += `<div style="flex:1;min-width:140px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px">`;
    html += `<div style="font-weight:700;font-size:14px;margin-bottom:4px">${ind.name}</div>`;
    html += `<div style="display:flex;align-items:center;gap:6px;margin:4px 0">`;
    html += `<span style="width:10px;height:10px;border-radius:50%;background:${color};display:inline-block"></span>`;
    html += `<span style="font-size:12px">${name}</span>`;
    html += `</div>`;
    html += `<div style="font-size:11px;color:var(--muted)">信心度 ${Math.round((ind.cycle_confidence || 0) * 100)}%</div>`;
    html += `</div>`;
  });
  html += '</div>';
  el.innerHTML = html;
}

export function renderIndustryLinkage(data) {
  const el = document.getElementById('industryLinkage');
  if (!data || !data.industries) { el.innerHTML = renderEmptyState('尚無產業關聯資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const industries = data.industries;

  // Calculate historical averages across all industries
  let totalSystemicImportance = 0;
  let totalPropagationSpeed = 0;
  let maxSystemic = 0;
  let maxPropagation = 0;
  let count = 0;

  industries.forEach(ind => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance || 0;
    const sp = score.shock_propagation_speed || 0;
    totalSystemicImportance += si;
    totalPropagationSpeed += sp;
    if (si > maxSystemic) maxSystemic = si;
    if (sp > maxPropagation) maxPropagation = sp;
    count++;
  });

  const avgSystemic = count > 0 ? totalSystemicImportance / count : 0;
  const avgPropagation = count > 0 ? totalPropagationSpeed / count : 0;

  let html = '<div style="font-size:11px;color:var(--muted);margin-bottom:10px;padding:8px;background:var(--bg);border-radius:6px">' +
    '<strong>數據說明：</strong>「系統重要性」衡量該產業在整體經濟中的關鍵程度（0-1）；「連動分數」反映衝擊傳導速度，數值越高表示該產業受外部衝擊影響越快擴散至其他產業。' +
    '</div>';

  // Summary stats
  html += '<div style="display:flex;gap:8px;margin-bottom:12px">';
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">平均系統重要性</div>`;
  html += `<div style="font-size:16px;font-weight:700">${avgSystemic.toFixed(2)}</div>`;
  html += `</div>`;
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">平均連動分數</div>`;
  html += `<div style="font-size:16px;font-weight:700">${avgPropagation.toFixed(2)}</div>`;
  html += `</div>`;
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">最高系統重要性</div>`;
  html += `<div style="font-size:16px;font-weight:700">${maxSystemic.toFixed(2)}</div>`;
  html += `</div>`;
  html += '</div>';

  html += '<table><thead><tr><th>產業</th><th>系統重要性</th><th>連動分數</th><th>相對強度</th></tr></thead><tbody>';
  industries.forEach(ind => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance || 0;
    const sp = score.shock_propagation_speed || 0;
    const siRelative = avgSystemic > 0 ? (si / avgSystemic) : 1;
    const spRelative = avgPropagation > 0 ? (sp / avgPropagation) : 1;
    const overallStrength = (siRelative + spRelative) / 2;

    let strengthLabel = '平均';
    let strengthColor = 'var(--muted)';
    if (overallStrength > 1.3) { strengthLabel = '高'; strengthColor = 'var(--up)'; }
    else if (overallStrength < 0.7) { strengthLabel = '低'; strengthColor = 'var(--down)'; }

    html += `<tr><td>${ind.name}</td><td>${si.toFixed(2)}</td><td>${sp.toFixed(2)}</td><td style="color:${strengthColor}">${strengthLabel}</td></tr>`;
  });
  html += '</tbody></table>';
  el.innerHTML = html;
}

export let seasonalityViewMode = 'list'; // 'list' or 'calendar'

export function renderIndustrySeasonality(data) {
  const el = document.getElementById('industrySeasonality');
  el.classList.remove('loading');

  const allPatterns = data && data.all_patterns ? data.all_patterns : [];
  const activePatterns = data && data.active_patterns ? data.active_patterns : [];

  let html = '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px">';
  html += '<div style="font-size:11px;color:var(--muted)">顯示所有歷史季節性模式與統計數據</div>';
  html += '<div style="display:flex;gap:4px">';
  html += `<button onclick="seasonalityViewMode='list';renderIndustrySeasonality(window.seasonalityData)" style="background:${seasonalityViewMode==='list'?'var(--accent)':'var(--bg)'};color:${seasonalityViewMode==='list'?'#fff':'var(--text)'};border:1px solid var(--border);border-radius:4px;padding:3px 10px;font-size:11px;cursor:pointer">列表</button>`;
  html += `<button onclick="seasonalityViewMode='calendar';renderIndustrySeasonality(window.seasonalityData)" style="background:${seasonalityViewMode==='calendar'?'var(--accent)':'var(--bg)'};color:${seasonalityViewMode==='calendar'?'#fff':'var(--text)'};border:1px solid var(--border);border-radius:4px;padding:3px 10px;font-size:11px;cursor:pointer">日曆</button>`;
  html += '</div></div>';

  if (seasonalityViewMode === 'calendar') {
    html += renderSeasonalityCalendar(data);
  } else {
    html += renderSeasonalityList(allPatterns, activePatterns, data);
  }

  el.innerHTML = html;
  window.seasonalityData = data;
}

export function renderSeasonalityList(allPatterns, activePatterns, data) {
  if (!allPatterns || allPatterns.length === 0) {
    return renderEmptyState('無季節性模式資料', '');
  }

  const activeIds = new Set(activePatterns.map(p => p.id));
  const today = new Date().toLocaleDateString('zh-TW');

  let html = '<table style="font-size:12px"><thead><tr><th>模式</th><th>期間</th><th>歷史準確度</th><th>典型報酬</th><th>調整因子</th><th>狀態</th></tr></thead><tbody>';
  allPatterns.forEach(p => {
    const isActive = activeIds.has(p.id);
    const statusBadge = isActive
      ? '<span class="badge ok">進行中</span>'
      : '<span class="badge info">非活躍</span>';
    const accuracy = Math.round((p.historical_accuracy || 0) * 100);
    const returnPct = ((p.typical_return || 0) * 100).toFixed(1);
    const adjustment = (p.adjustment_factor || 1.0).toFixed(2);
    const period = `${p.start_month}/${p.start_day} ~ ${p.end_month}/${p.end_day}`;

    html += `<tr style="${isActive ? 'background:rgba(79,193,255,0.05)' : ''}">`;
    html += `<td><strong>${p.name}</strong><br><span style="font-size:11px;color:var(--muted)">${p.description || ''}</span></td>`;
    html += `<td>${period}</td>`;
    html += `<td>${accuracy}%</td>`;
    html += `<td>${returnPct}%</td>`;
    html += `<td>${adjustment}x</td>`;
    html += `<td>${statusBadge}</td>`;
    html += '</tr>';
  });
  html += '</tbody></table>';

  if (activePatterns.length === 0) {
    html += `<div style="margin-top:10px;padding:10px;background:var(--bg);border-radius:6px;font-size:12px;color:var(--muted)">
      今天是 ${today}，目前無活躍模式。上表列出所有追蹤中的季節性模式供參考。
    </div>`;
  }

  return html;
}

export function renderSeasonalityCalendar(data) {
  if (!data || !data.calendar) {
    return renderEmptyState('無日曆資料', '');
  }

  const months = ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月'];
  const calendar = data.calendar;

  let html = '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:8px">';
  calendar.months.forEach(m => {
    const monthName = months[m.month - 1];
    html += `<div style="background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px">`;
    html += `<div style="font-weight:700;font-size:13px;margin-bottom:6px">${monthName}</div>`;
    if (m.patterns && m.patterns.length > 0) {
      m.patterns.forEach(p => {
        const accuracy = Math.round((p.historical_accuracy || 0) * 100);
        html += `<div style="font-size:11px;margin:3px 0;padding:4px;background:var(--panel);border-radius:4px">`;
        html += `<strong>${p.name}</strong> <span style="color:var(--muted)">(${accuracy}%)</span>`;
        html += `</div>`;
      });
    } else {
      html += `<div style="font-size:11px;color:var(--muted)">無相關模式</div>`;
    }
    html += `</div>`;
  });
  html += '</div>';

  return html;
}

export function showIndustryDetail(industryId) {
  notify(`產業詳細分析功能開發中: ${industryId}`, 'info');
