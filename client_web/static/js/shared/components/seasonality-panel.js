// Shared seasonality panel component — used by industry ecosystem and macro narrative pages.
import { renderEmptyState } from '../app-utils.js';

export let seasonalityViewMode = "list"; // 'list' or 'calendar'

export function renderIndustrySeasonality(data) {
  const el = document.getElementById("industrySeasonality");
  if (!el) return;
  el.classList.remove("loading");

  const allPatterns = data && data.all_patterns ? data.all_patterns : [];
  const activePatterns =
    data && data.active_patterns ? data.active_patterns : [];

  const calibEvidence = data && data.calibration_evidence;
  let evidenceBanner;
  if (calibEvidence && calibEvidence.calibrated) {
    const ts = calibEvidence.timestamp ? new Date(calibEvidence.timestamp).toLocaleString("zh-TW") : "未知";
    const src = calibEvidence.data_source || "未知";
    evidenceBanner =
      `<div style="font-size:11px;color:var(--ok);margin-bottom:8px;padding:6px 10px;background:color-mix(in srgb, var(--accent) 8%, transparent);border:1px solid color-mix(in srgb, var(--accent) 20%, transparent);border-radius:6px">` +
      `✓ <strong>已校準：</strong>季節性模式數值已透過回測校準（校準時間：${ts}，資料來源：${src}）。校準結果已更新 HistoricalAccuracy 與 AdjustmentFactor。` +
      `</div>`;
  } else {
    evidenceBanner =
      '<div style="font-size:11px;color:var(--warn);margin-bottom:8px;padding:6px 10px;background:color-mix(in srgb, var(--warn) 8%, transparent);border:1px solid color-mix(in srgb, var(--warn) 20%, transparent);border-radius:6px">' +
      '⚠️ <strong>證據品質提示：</strong>以下季節性模式數值基於經驗法則（heuristic），尚未經過回測校準（evidence_quality: low）。請勿將 HistoricalAccuracy 與 AdjustmentFactor 視為實證數據。' +
      '</div>';
  }
  let html = evidenceBanner;
  html +=
    '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px">';
  html +=
    '<div style="font-size:11px;color:var(--muted)">顯示所有歷史季節性模式與統計數據</div>';
  html += '<div style="display:flex;gap:4px">';
  html += `<button onclick="seasonalityViewMode='list';renderIndustrySeasonality(window.seasonalityData)" style="background:${seasonalityViewMode === "list" ? "var(--accent)" : "var(--bg)"};color:${seasonalityViewMode === "list" ? "#fff" : "var(--text)"};border:1px solid var(--border);border-radius:4px;padding:3px 10px;font-size:11px;cursor:pointer">列表</button>`;
  html += `<button onclick="seasonalityViewMode='calendar';renderIndustrySeasonality(window.seasonalityData)" style="background:${seasonalityViewMode === "calendar" ? "var(--accent)" : "var(--bg)"};color:${seasonalityViewMode === "calendar" ? "#fff" : "var(--text)"};border:1px solid var(--border);border-radius:4px;padding:3px 10px;font-size:11px;cursor:pointer">日曆</button>`;
  html += "</div></div>";

  if (seasonalityViewMode === "calendar") {
    html += renderSeasonalityCalendar(data);
  } else {
    html += renderSeasonalityList(allPatterns, activePatterns, data);
  }

  el.innerHTML = html;
  window.seasonalityData = data;
}

export function renderSeasonalityList(allPatterns, activePatterns, data) {
  if (!allPatterns || allPatterns.length === 0) {
    return renderEmptyState("無季節性模式資料", "");
  }
  const calibEvidence = data && data.calibration_evidence;

  const activeIds = new Set(activePatterns.map((p) => p.id));
  const today = new Date().toLocaleDateString("zh-TW");

  let html =
    '<table style="font-size:12px"><thead><tr><th>模式</th><th>期間</th><th>歷史準確度</th><th>典型報酬</th><th>調整因子</th><th>狀態</th></tr></thead><tbody>';
  allPatterns.forEach((p) => {
    const isActive = activeIds.has(p.id);
    const statusBadge = isActive
      ? '<span class="badge ok">進行中</span>'
      : '<span class="badge info">非活躍</span>';
    const accuracy = Math.round((p.historical_accuracy || 0) * 100);
    const returnPct = ((p.avg_market_return || 0) * 100).toFixed(1);
    const adjustment = (p.adjustment_factor || 1.0).toFixed(2);
    const returnColor = returnPct < 0 ? 'var(--down)' : returnPct > 0 ? 'var(--up)' : '';
    const adjColor = adjustment < 0 ? 'var(--down)' : adjustment > 0 ? 'var(--up)' : '';
    const period = `${p.start_month}/${p.start_day} ~ ${p.end_month}/${p.end_day}`;

    html += `<tr style="${isActive ? "background:color-mix(in srgb, var(--accent) 5%, transparent)" : ""}">`;
    html += `<td><strong>${p.name}</strong><br><span style="font-size:11px;color:var(--muted)">${p.description || ""}</span></td>`;
    html += `<td>${period}</td>`;
    const evidenceBadge = calibEvidence && calibEvidence.calibrated
      ? `<span style="font-size:10px;color:var(--ok);background:color-mix(in srgb, var(--accent) 10%, transparent);padding:1px 4px;border-radius:3px" title="已透過回測校準">已校準</span>`
      : `<span style="font-size:10px;color:var(--warn);background:color-mix(in srgb, var(--warn) 10%, transparent);padding:1px 4px;border-radius:3px" title="evidence_quality: low — 尚未經過回測校準">待驗證</span>`;
    html += `<td>${accuracy}% ${evidenceBadge}</td>`;
    html += `<td style="color:${returnColor}">${returnPct}%</td>`;
    html += `<td style="color:${adjColor}">${adjustment}x</td>`;
    html += `<td>${statusBadge}</td>`;
    html += "</tr>";
  });
  html += "</tbody></table>";

  // Adjustment breakdown visualization
  const breakdown = data && data.adjustment_breakdown;
  if (breakdown) {
    const layers = [
      { key: "direct_match", label: "直接匹配" },
      { key: "supply_chain", label: "供應鏈傳導" },
      { key: "narrative",    label: "敘事事件" },
      { key: "dynamic_env",  label: "動態環境" },
    ];
    const comp = breakdown.composite || 1.0;
    html += '<div style="margin-top:12px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:12px">';
    html += '<div style="font-weight:700;font-size:13px;margin-bottom:8px">調整因子分解（複合值 ' + comp.toFixed(4) + 'x）</div>';
    layers.forEach(function(layer) {
      const val = breakdown[layer.key] || 1.0;
      const barW = Math.min(Math.abs((val - 1) * 100), 30);
      const color = val >= 1 ? "var(--up)" : "var(--down)";
      const direction = val >= 1 ? "+" : "";
      html += '<div style="display:flex;align-items:center;gap:8px;margin:4px 0;font-size:12px">';
      html += '<span style="width:80px;color:var(--muted)">' + layer.label + '</span>';
      html += '<div style="flex:1;height:16px;background:var(--border);border-radius:3px;overflow:hidden">';
      html += '<div style="width:' + barW + '%;height:100%;background:' + color + ';opacity:0.6;border-radius:3px"></div></div>';
      html += '<span style="width:60px;text-align:right;font-weight:600;color:' + color + '">' + direction + ((val - 1) * 100).toFixed(1) + '%</span>';
      html += '</div>';
    });
    html += '</div>';
  }

  // Narrative themes overlay
  const themes = data && data.narrative_themes;
  if (themes && themes.length > 0) {
    html += '<div style="margin-top:8px;font-size:11px;color:var(--muted);padding:6px 10px;background:color-mix(in srgb, var(--accent) 6%, transparent);border:1px solid color-mix(in srgb, var(--accent) 20%, transparent);border-radius:6px">';
    html += '<strong>活躍敘事主題：</strong>' + themes.join(", ");
    html += '</div>';
  }

  if (activePatterns.length === 0) {
    html += `<div style="margin-top:10px;padding:10px;background:var(--bg);border-radius:6px;font-size:12px;color:var(--muted)">
      今天是 ${today}，目前無活躍模式。上表列出所有追蹤中的季節性模式供參考。
    </div>`;
  }

  return html;
}

export function renderSeasonalityCalendar(data) {
  const calendar = data && data.calendar ? data.calendar : null;
  if (!calendar || !calendar.months || calendar.months.length === 0) {
    return renderEmptyState("無日曆視圖資料", "");
  }

  const monthNames = [
    "一月", "二月", "三月", "四月", "五月", "六月",
    "七月", "八月", "九月", "十月", "十一月", "十二月",
  ];

  let html =
    '<div style="display:grid;grid-template-columns:repeat(4,1fr);gap:8px;font-size:12px">';
  calendar.months.forEach((m) => {
    const monthIdx = (m.month || 1) - 1;
    const name = monthNames[monthIdx] || `M${m.month}`;
    const hasPatterns = m.patterns && m.patterns.length > 0;
    const bgColor = hasPatterns
      ? "color-mix(in srgb, var(--accent) 6%, transparent)"
      : "var(--bg)";

    html +=
      `<div style="background:${bgColor};border:1px solid var(--border);border-radius:8px;padding:8px">`;
    html +=
      `<div style="font-weight:700;font-size:13px;margin-bottom:4px;color:${hasPatterns ? "var(--accent)" : "var(--muted)"}">${name}</div>`;

    if (hasPatterns) {
      m.patterns.forEach((p) => {
        const accuracy = Math.round((p.historical_accuracy || 0) * 100);
    const returnPct = ((p.avg_market_return || 0) * 100).toFixed(1);
        html += `<div style="font-size:11px;padding:3px 0;border-bottom:1px solid var(--border)">`;
        html +=
          `<div style="font-weight:600">${p.name}</div>`;
        html +=
          `<div style="color:var(--muted)">準確度 ${accuracy}% · 報酬 ${returnPct}% · 因子 ${(p.adjustment_factor || 1).toFixed(2)}x</div>`;
        html += `</div>`;
      });
    } else {
      html +=
        '<div style="font-size:11px;color:var(--muted);padding:4px 0">無活躍模式</div>';
    }
    html += "</div>";
  });
  html += "</div>";

  return html;
}
