// Industry ecosystem page
import { sectorName, stockName } from "../names.js";
import {
  silentGetJSON,
  postJSON,
  notify,
  renderEmptyState,
} from "../shared/app-utils.js";

export async function loadIndustryData() {
  try {
    const [classification, overview, seasonality, calendar] = await Promise.all(
      [
        silentGetJSON("/api/dashboard/industry-classification"),
        silentGetJSON("/api/dashboard/industry-overview"),
        silentGetJSON("/api/dashboard/industry-seasonality"),
        silentGetJSON("/api/dashboard/industry-seasonality-calendar"),
      ],
    );
    renderIndustryMap(classification);
    populateShockSourceDropdown(classification);
    renderIndustryCycle(overview);
    renderIndustryLinkage(overview);
    if (seasonality && calendar) {
      seasonality.calendar = calendar;
    }
    renderIndustrySeasonality(seasonality);
  } catch (e) {
    console.error("loadIndustryData error:", e);
  }
}

export function renderIndustryMap(data) {
  const el = document.getElementById("industryMap");
  if (!data || !data.industries) {
    el.innerHTML = renderEmptyState("尚無產業資料", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.remove("loading");
  const industries = data.industries;
  let html = '<div style="display:flex;flex-wrap:wrap;gap:10px">';
  industries.forEach((ind) => {
    const weightPct = Math.round((ind.weight || 0) * 100);
    html += `<div style="flex:1;min-width:140px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;cursor:pointer" onclick="showIndustryDetail('${ind.id}')">`;
    html += `<div style="font-weight:700;font-size:14px;margin-bottom:4px">${ind.name}</div>`;
    html += `<div style="font-size:12px;color:var(--muted)">權重 ${weightPct}%</div>`;
    html += `<div style="margin-top:6px;height:4px;background:var(--border);border-radius:2px;overflow:hidden">`;
    html += `<div style="width:${weightPct}%;height:100%;background:var(--accent)"></div></div>`;
    html += `</div>`;
  });
  html += "</div>";
  el.innerHTML = html;
}

function confidenceColor(hex, confidence) {
  // Phase indicator opacity reflects confidence (0.3 dim … 1.0 full)
  const alpha = 0.3 + (confidence || 0) * 0.7;
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return "rgba(" + r + "," + g + "," + b + "," + alpha.toFixed(2) + ")";
}

export function renderIndustryCycle(data) {
  const el = document.getElementById("industryCycle");
  if (!data || !data.industries) {
    el.innerHTML = renderEmptyState("尚無週期資料", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.remove("loading");
  const industries = data.industries;
  const cycleColors = {
    recovery: "#10b981",
    expansion: "#3b82f6",
    mature: "#f59e0b",
    recession: "#ef4444",
  };
  const cycleNames = {
    recovery: "復甦",
    expansion: "擴張",
    mature: "成熟",
    recession: "衰退",
  };
  let html = '<div style="display:flex;flex-wrap:wrap;gap:10px">';
  industries.forEach((ind) => {
    const confidence = ind.cycle_confidence || 0;
    const baseColor = cycleColors[ind.cycle_phase] || "#666";
    const color = confidenceColor(baseColor, confidence);
    const name = cycleNames[ind.cycle_phase] || ind.cycle_phase;
    html += `<div style="flex:1;min-width:140px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px">`;
    html += `<div style="font-weight:700;font-size:14px;margin-bottom:4px">${ind.name}</div>`;
    html += `<div style="display:flex;align-items:center;gap:6px;margin:4px 0">`;
    html += `<span style="width:10px;height:10px;border-radius:50%;background:${color};display:inline-block"></span>`;
    html += `<span style="font-size:12px">${name}</span>`;
    html += `</div>`;
    html += `<div style="font-size:11px;color:var(--muted)">信心度 ${Math.round(confidence * 100)}%</div>`;
    html += `</div>`;
  });
  html += "</div>";
  el.innerHTML = html;
}

export function renderIndustryLinkage(data) {
  const el = document.getElementById("industryLinkage");
  if (!data || !data.industries) {
    el.innerHTML = renderEmptyState("尚無產業關聯資料", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.remove("loading");
  const industries = data.industries;

  // Calculate historical averages across all industries
  let totalSystemicImportance = 0;
  let totalPropagationSpeed = 0;
  let maxSystemic = 0;
  let maxPropagation = 0;
  let count = 0;

  industries.forEach((ind) => {
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

  let html =
    '<div style="font-size:11px;color:var(--muted);margin-bottom:10px;padding:8px;background:var(--bg);border-radius:6px">' +
    "<strong>數據說明：</strong>「系統重要性」衡量該產業在整體經濟中的關鍵程度（0-1）；「連動分數」反映衝擊傳導速度，數值越高表示該產業受外部衝擊影響越快擴散至其他產業。" +
    "</div>";

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
  html += "</div>";

  html +=
    "<table><thead><tr><th>產業</th><th>系統重要性</th><th>連動分數</th><th>相對強度</th></tr></thead><tbody>";
  industries.forEach((ind) => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance || 0;
    const sp = score.shock_propagation_speed || 0;
    const siRelative = avgSystemic > 0 ? si / avgSystemic : 1;
    const spRelative = avgPropagation > 0 ? sp / avgPropagation : 1;
    const overallStrength = (siRelative + spRelative) / 2;

    let strengthLabel = "平均";
    let strengthColor = "var(--muted)";
    if (overallStrength > 1.3) {
      strengthLabel = "高";
      strengthColor = "var(--up)";
    } else if (overallStrength < 0.7) {
      strengthLabel = "低";
      strengthColor = "var(--down)";
    }

    html += `<tr><td>${ind.name}</td><td>${si.toFixed(2)}</td><td>${sp.toFixed(2)}</td><td style="color:${strengthColor}">${strengthLabel}</td></tr>`;
  });
  html += "</tbody></table>";
  el.innerHTML = html;
}

export let seasonalityViewMode = "list"; // 'list' or 'calendar'

export function renderIndustrySeasonality(data) {
  const el = document.getElementById("industrySeasonality");
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
      `<div style="font-size:11px;color:var(--ok);margin-bottom:8px;padding:6px 10px;background:rgba(79,193,255,0.08);border:1px solid rgba(79,193,255,0.2);border-radius:6px">` +
      `✓ <strong>已校準：</strong>季節性模式數值已透過回測校準（校準時間：${ts}，資料來源：${src}）。校準結果已更新 HistoricalAccuracy 與 AdjustmentFactor。` +
      `</div>`;
  } else {
    evidenceBanner =
      '<div style="font-size:11px;color:var(--warn);margin-bottom:8px;padding:6px 10px;background:rgba(245,158,11,0.08);border:1px solid rgba(245,158,11,0.2);border-radius:6px">' +
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
    const returnPct = ((p.typical_return || 0) * 100).toFixed(1);
    const adjustment = (p.adjustment_factor || 1.0).toFixed(2);
    const period = `${p.start_month}/${p.start_day} ~ ${p.end_month}/${p.end_day}`;

    html += `<tr style="${isActive ? "background:rgba(79,193,255,0.05)" : ""}">`;
    html += `<td><strong>${p.name}</strong><br><span style="font-size:11px;color:var(--muted)">${p.description || ""}</span></td>`;
    html += `<td>${period}</td>`;
    const evidenceBadge = calibEvidence && calibEvidence.calibrated
      ? `<span style="font-size:10px;color:var(--ok);background:rgba(79,193,255,0.1);padding:1px 4px;border-radius:3px" title="已透過回測校準">已校準</span>`
      : `<span style="font-size:10px;color:var(--warn);background:rgba(245,158,11,0.1);padding:1px 4px;border-radius:3px" title="evidence_quality: low — 尚未經過回測校準">待驗證</span>`;
    html += `<td>${accuracy}% ${evidenceBadge}</td>`;
    html += `<td>${returnPct}%</td>`;
    html += `<td>${adjustment}x</td>`;
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
      html += '<div style="flex:1;height:16px;background:rgba(0,0,0,0.05);border-radius:3px;overflow:hidden">';
      html += '<div style="width:' + barW + '%;height:100%;background:' + color + ';opacity:0.6;border-radius:3px"></div></div>';
      html += '<span style="width:60px;text-align:right;font-weight:600;color:' + color + '">' + direction + ((val - 1) * 100).toFixed(1) + '%</span>';
      html += '</div>';
    });
    html += '</div>';
  }

  // Narrative themes overlay
  const themes = data && data.narrative_themes;
  if (themes && themes.length > 0) {
    html += '<div style="margin-top:8px;font-size:11px;color:var(--muted);padding:6px 10px;background:rgba(79,193,255,0.06);border:1px solid rgba(79,193,255,0.2);border-radius:6px">';
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
      ? "rgba(79,193,255,0.06)"
      : "var(--bg)";

    html +=
      `<div style="background:${bgColor};border:1px solid var(--border);border-radius:8px;padding:8px">`;
    html +=
      `<div style="font-weight:700;font-size:13px;margin-bottom:4px;color:${hasPatterns ? "var(--accent)" : "var(--muted)"}">${name}</div>`;

    if (hasPatterns) {
      m.patterns.forEach((p) => {
        const accuracy = Math.round((p.historical_accuracy || 0) * 100);
        const returnPct = ((p.typical_return || 0) * 100).toFixed(1);
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

function renderCycleTab(detail) {
  const cp = detail.cycle_position;
  if (!cp) return renderEmptyState("尚無週期定位資料", "");

  const phases = ["復甦", "擴張", "成熟", "衰退"];
  const phaseKeys = ["recovery", "expansion", "mature", "recession"];
  const activeIdx = phaseKeys.indexOf((cp.business_cycle || "").toLowerCase());

  let html = '<div class="industry-section">';
  html += "<h4>📊 產業生命週期定位</h4>";
  html += '<div class="cycle-visual">';
  phases.forEach((name, i) => {
    const active = i === activeIdx ? " active" : "";
    html += `<span class="cycle-phase${active}">${name}</span>`;
    if (i < phases.length - 1)
      html += '<span style="color:var(--muted);font-size:11px">→</span>';
  });
  html += "</div></div>";

  const metrics = [
    { label: "商業週期", value: cp.business_cycle || "-" },
    { label: "庫存週期", value: cp.inventory_cycle || "-" },
    { label: "資本支出週期", value: cp.capex_cycle || "-" },
    { label: "趨勢方向", value: cp.trend || "-" },
    {
      label: "週期分數",
      value: cp.phase_score != null ? cp.phase_score.toFixed(2) : "-",
    },
    {
      label: "信心度",
      value:
        cp.confidence != null ? `${(cp.confidence * 100).toFixed(0)}%` : "-",
    },
    { label: "是否有利", value: cp.is_favorable ? "✅ 有利" : "⚠️ 不利" },
  ];

  html += '<div class="industry-section"><h4>📈 關鍵指標</h4>';
  metrics.forEach((m) => {
    html += `<div class="metric-row"><span class="metric-label">${m.label}</span><span class="metric-value">${m.value}</span></div>`;
  });
  html += "</div>";

  // Confidence breakdown visualization
  const cb = cp.confidence_breakdown;
  if (cb && (cb.boundary || cb.seasonal || cb.linkage || cb.narrative)) {
    const dims = [
      { key: "boundary", label: "邊界信號", weight: cb.weights ? cb.weights.boundary : 0 },
      { key: "freshness", label: "數據新鮮度", weight: cb.weights ? cb.weights.freshness : 0 },
      { key: "seasonal", label: "季節性", weight: cb.weights ? cb.weights.seasonal : 0 },
      { key: "linkage", label: "供應鏈連動", weight: cb.weights ? cb.weights.linkage : 0 },
      { key: "narrative", label: "宏觀敘事", weight: cb.weights ? cb.weights.narrative : 0 },
    ];
    html += '<div class="industry-section"><h4>📐 信心度分解</h4>';
    html += '<div style="font-size:11px;color:var(--muted);margin-bottom:8px">各維度信心分數 × 配置權重 → 複合信心 <strong>' + ((cb.composite || 0) * 100).toFixed(0) + '%</strong></div>';
    dims.forEach(function(d) {
      const val = cb[d.key] || 0;
      if (val <= 0) return;
      const barW = Math.min(val * 100, 100);
      const wPct = d.weight ? (d.weight * 100).toFixed(0) : 0;
      html += '<div style="display:flex;align-items:center;gap:6px;margin:3px 0;font-size:11px">';
      html += '<span style="width:80px;color:var(--muted)">' + d.label + '</span>';
      html += '<div style="flex:1;height:14px;background:rgba(0,0,0,0.04);border-radius:3px;overflow:hidden">';
      html += '<div style="width:' + barW + '%;height:100%;background:var(--accent);opacity:0.5;border-radius:3px"></div></div>';
      html += '<span style="width:40px;text-align:right">' + (val * 100).toFixed(0) + '%</span>';
      html += '<span style="width:30px;color:var(--muted);font-size:10px;text-align:right">w=' + wPct + '%</span>';
      html += '</div>';
    });
    html += "</div>";
  }

  if (detail.recommendation) {
    const rec = detail.recommendation;
    const actionColor =
      rec.action === "overweight"
        ? "var(--up)"
        : rec.action === "underweight"
          ? "var(--down)"
          : "var(--warn)";
    html += '<div class="industry-section"><h4>🎯 建議</h4>';
    html += `<div style="background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px">`;
    html += `<div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="color:var(--muted)">操作</span><span style="color:${actionColor};font-weight:700">${rec.action}</span></div>`;
    if (rec.conviction)
      html += `<div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="color:var(--muted)">信念</span><span>${rec.conviction}</span></div>`;
    if (rec.target_weight != null)
      html += `<div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="color:var(--muted)">目標權重</span><span>${(rec.target_weight * 100).toFixed(1)}%</span></div>`;
    if (rec.delta != null)
      html += `<div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="color:var(--muted)">調整幅度</span><span>${(rec.delta * 100).toFixed(1)}%</span></div>`;
    if (rec.rationale)
      html += `<div style="margin-top:6px;font-size:12px;color:var(--muted);line-height:1.6">${rec.rationale}</div>`;
    html += "</div></div>";
  }

  if (detail.representative_stocks && detail.representative_stocks.length > 0) {
    html +=
      '<div class="industry-section"><h4>🏢 代表性個股</h4><div style="display:flex;flex-wrap:wrap;gap:6px">';
    detail.representative_stocks.forEach((s) => {
      const name = stockName(s) || s;
      html += `<span style="background:var(--bg);border:1px solid var(--border);border-radius:4px;padding:3px 8px;font-size:12px">${name}</span>`;
    });
    html += "</div></div>";
  }

  return html;
}

function renderLinkageTab(detail) {
  const li = detail.linkage_info;
  if (!li) return renderEmptyState("尚無供應鏈資料", "");

  let html = '<div class="industry-section"><h4>🔗 供應鏈關係</h4>';

  if (li.linkage_score) {
    const ls = li.linkage_score;
    html += '<div style="display:flex;gap:8px;margin-bottom:12px">';
    html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
    html += `<div style="font-size:11px;color:var(--muted)">系統重要性</div>`;
    html += `<div style="font-size:16px;font-weight:700">${(ls.systemic_importance || 0).toFixed(2)}</div></div>`;
    html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
    html += `<div style="font-size:11px;color:var(--muted)">衝擊傳導速度</div>`;
    html += `<div style="font-size:16px;font-weight:700">${(ls.shock_propagation_speed || 0).toFixed(2)}</div></div>`;
    html += "</div>";
  }

  html += '<div class="linkage-grid">';
  html += '<div class="linkage-col"><h4>⬆️ 上游供應</h4>';
  if (li.upstream && li.upstream.length > 0) {
    li.upstream.forEach(
      (u) => (html += `<div class="linkage-item">${sectorName(u) || u}</div>`),
    );
  } else {
    html +=
      '<div class="linkage-item" style="color:var(--muted)">無上游資料</div>';
  }
  html += "</div>";

  html += '<div class="linkage-col"><h4>⬇️ 下游需求</h4>';
  if (li.downstream && li.downstream.length > 0) {
    li.downstream.forEach(
      (d) => (html += `<div class="linkage-item">${sectorName(d) || d}</div>`),
    );
  } else {
    html +=
      '<div class="linkage-item" style="color:var(--muted)">無下游資料</div>';
  }
  html += "</div></div></div>";

  if (li.correlations && li.correlations.length > 0) {
    html += '<div class="industry-section"><h4>📊 相關性分析</h4>';
    html +=
      '<table style="font-size:12px"><thead><tr><th>產業</th><th>相關性</th><th>強度</th></tr></thead><tbody>';
    li.correlations.forEach((c) => {
      const corr = c.correlation != null ? c.correlation : 0;
      const absCorr = Math.abs(corr);
      const strength = absCorr > 0.7 ? "高" : absCorr > 0.4 ? "中" : "低";
      const color = corr > 0 ? "var(--up)" : "var(--down)";
      html += `<tr><td>${sectorName(c.industry) || c.industry || "-"}</td>`;
      html += `<td style="color:${color}">${corr.toFixed(2)}</td>`;
      html += `<td>${strength}</td></tr>`;
    });
    html += "</tbody></table></div>";
  }

  return html;
}

function renderSeasonalityTab(detail) {
  const patterns = detail.seasonal_patterns;
  if (!patterns || patterns.length === 0)
    return renderEmptyState("尚無季節性模式資料", "");

  let html = '<div class="industry-section"><h4>📅 季節性模式</h4>';
  html += '<div style="font-size:11px;color:var(--warn);margin-bottom:8px;padding:6px 10px;background:rgba(245,158,11,0.08);border:1px solid rgba(245,158,11,0.2);border-radius:6px">' +
    '⚠️ 以下數值基於經驗法則，尚未經過回測校準。' +
    '</div>';
  patterns.forEach((p) => {
    const accuracy = Math.round((p.historical_accuracy || 0) * 100);
    const returnPct = ((p.typical_return || 0) * 100).toFixed(1);
    const period = `${p.start_month}/${p.start_day} ~ ${p.end_month}/${p.end_day}`;
    const impactColor =
      p.impact === "positive"
        ? "var(--up)"
        : p.impact === "negative"
          ? "var(--down)"
          : "var(--warn)";

    html += '<div class="seasonal-pattern">';
    html += `<div class="pattern-name">${p.name}</div>`;
    html += `<div class="pattern-meta">${period}</div>`;
    html += '<div class="metric-row" style="margin-top:6px">';
    const calEvidence = (window.seasonalityData && window.seasonalityData.calibration_evidence);
    const accBadge = calEvidence && calEvidence.calibrated
      ? `<span style="font-size:10px;color:var(--ok);background:rgba(79,193,255,0.1);padding:1px 4px;border-radius:3px">已校準</span>`
      : `<span style="font-size:10px;color:var(--warn);background:rgba(245,158,11,0.1);padding:1px 4px;border-radius:3px">待驗證</span>`;
    html += `<span class="metric-label">歷史準確度</span><span class="metric-value">${accuracy}% ${accBadge}</span></div>`;
    html += '<div class="metric-row">';
    html += `<span class="metric-label">典型報酬</span><span class="metric-value" style="color:${returnPct >= 0 ? "var(--up)" : "var(--down)"}">${returnPct}%</span></div>`;
    html += '<div class="metric-row">';
    html += `<span class="metric-label">調整因子</span><span class="metric-value">${(p.adjustment_factor || 1.0).toFixed(2)}x</span></div>`;
    if (p.impact) {
      html += '<div class="metric-row">';
      html += `<span class="metric-label">影響方向</span><span class="metric-value" style="color:${impactColor}">${p.impact}</span></div>`;
    }
    if (p.description) {
      html += `<div style="margin-top:6px;font-size:11px;color:var(--muted)">${p.description}</div>`;
    }
    html += "</div>";
  });
  html += "</div>";

  return html;
}

function renderRiskTab(detail) {
  const ri = detail.risk_info;
  if (!ri || !ri.risks || ri.risks.length === 0)
    return renderEmptyState("目前無偵測到風險", "");

  let html = `<div class="industry-section"><h4>⚠️ 風險概覽（共 ${ri.risk_count || ri.risks.length} 項）</h4>`;

  ri.risks.forEach((r) => {
    const severity = (r.severity || "low").toLowerCase();
    const severityLabel =
      severity === "high" ? "高" : severity === "medium" ? "中" : "低";
    const impactColor =
      severity === "high"
        ? "var(--down)"
        : severity === "medium"
          ? "var(--warn)"
          : "var(--up)";

    html += '<div class="risk-item">';
    html += '<div class="risk-header">';
    html += `<span style="font-weight:600;font-size:13px">${r.type || "未知風險"}</span>`;
    html += `<span class="risk-severity ${severity}" style="color:${impactColor}">${severityLabel}</span>`;
    html += "</div>";
    if (r.description)
      html += `<div style="font-size:12px;margin-bottom:4px">${r.description}</div>`;
    if (r.impact_estimate != null) {
      html += '<div class="metric-row">';
      html += `<span class="metric-label">預估衝擊</span><span class="metric-value" style="color:${impactColor}">${(r.impact_estimate * 100).toFixed(1)}%</span></div>`;
    }
    if (r.confidence != null) {
      html += '<div class="metric-row">';
      html += `<span class="metric-label">信心度</span><span class="metric-value">${(r.confidence * 100).toFixed(0)}%</span></div>`;
    }
    if (r.source) {
      html += '<div class="metric-row">';
      html += `<span class="metric-label">來源</span><span class="metric-value">${r.source}</span></div>`;
    }
    html += "</div>";
  });
  html += "</div>";

  return html;
}

async function showIndustryDetail(id) {
  const titleEl = document.getElementById("industryModalTitle");
  const contentEl = document.getElementById("industryModalContent");
  const modal = document.getElementById("industryModal");
  if (!titleEl || !contentEl || !modal) return;

  titleEl.textContent = "載入中…";
  contentEl.innerHTML = '<div class="empty">載入中…</div>';
  modal.classList.add("show");

  const detail = await silentGetJSON("/api/dashboard/industry-detail?industry=" + encodeURIComponent(id));
  if (!detail) {
    contentEl.innerHTML = '<div class="empty">無法載入產業詳細資料</div>';
    titleEl.textContent = "產業詳細分析";
    return;
  }

  window._industryDetail = detail;
  titleEl.textContent = (detail.name || id) + " 詳細分析";
  switchIndustryTab("cycle");
}

function closeIndustryModal() {
  const modal = document.getElementById("industryModal");
  if (modal) modal.classList.remove("show");
}

function switchIndustryTab(tab) {
  const detail = window._industryDetail;
  const contentEl = document.getElementById("industryModalContent");
  if (!detail || !contentEl) return;

  document.querySelectorAll("#industryTabs .tab-btn").forEach(function(btn) {
    btn.classList.toggle("active", btn.getAttribute("data-tab") === tab);
  });

  switch (tab) {
    case "cycle":
      contentEl.innerHTML = renderCycleTab(detail);
      break;
    case "linkage":
      contentEl.innerHTML = renderLinkageTab(detail);
      break;
    case "seasonality":
      contentEl.innerHTML = renderSeasonalityTab(detail);
      break;
    case "risk":
      contentEl.innerHTML = renderRiskTab(detail);
      break;
    default:
      contentEl.innerHTML = '<div class="empty">未知的分頁</div>';
  }
}

function toggleCycleLegend() {
  const modal = document.getElementById("cycleLegendModal");
  if (modal) {
    modal.classList.toggle("show");
  }
}

function closeCycleLegend() {
  const modal = document.getElementById("cycleLegendModal");
  if (modal) modal.classList.remove("show");
}

function populateShockSourceDropdown(classification) {
  const sel = document.getElementById("shockSource");
  if (!sel || !classification || !classification.industries) return;
  const industries = classification.industries;
  for (const ind of industries) {
    const id = ind.id || "";
    const name = ind.name || id;
    if (!id) continue;
    const opt = document.createElement("option");
    opt.value = id;
    opt.textContent = name;
    sel.appendChild(opt);
  }
}

window.runShockSimulation = async function () {
  const source = document.getElementById("shockSource").value;
  const magnitude = parseFloat(document.getElementById("shockMagnitude").value);
  const depth = parseInt(document.getElementById("shockDepth").value) || 3;
  const el = document.getElementById("industryShockSim");
  if (!source) {
    el.innerHTML = renderEmptyState("請選擇來源產業", "");
    el.classList.remove("loading");
    return;
  }
  if (isNaN(magnitude)) {
    el.innerHTML = renderEmptyState("請輸入衝擊幅度", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.add("loading");
  el.innerHTML = '<div style="text-align:center;padding:20px;color:var(--muted)">模擬中…</div>';
  try {
    const result = await postJSON("/api/dashboard/industry-shock-simulation", {
      source_industry: source,
      shock_magnitude: magnitude,
      max_depth: depth,
    });
    renderShockSimulationResult(result);
  } catch (e) {
    el.classList.remove("loading");
    el.innerHTML = renderEmptyState("模擬失敗: " + (e.message || e), "");
  }
};

function renderShockSimulationResult(data) {
  const el = document.getElementById("industryShockSim");
  el.classList.remove("loading");
  if (!data || !data.impacts || data.impacts.length === 0) {
    el.innerHTML = renderEmptyState("該衝擊未影響其他產業", "");
    return;
  }
  const sorted = [...data.impacts].sort((a, b) => Math.abs(b.impact) - Math.abs(a.impact));
  let html = '<div style="font-size:11px;color:var(--muted);margin-bottom:8px">';
  html += `<strong>${data.source}</strong> 遭受 <strong>${(data.shock * 100).toFixed(1)}%</strong> 衝擊 → 影響 ${data.impact_count} 個關聯產業</div>`;
  html += '<table style="font-size:12px"><thead><tr><th>產業</th><th>影響幅度</th><th>衝擊傳導</th></tr></thead><tbody>';
  for (const imp of sorted) {
    const pct = (imp.impact * 100).toFixed(1);
    const absPct = Math.abs(imp.impact * 100);
    const color = imp.impact < 0 ? "var(--down)" : "var(--up)";
    const barW = Math.min(100, absPct * 8);
    html += `<tr>`;
    html += `<td>${imp.industry}</td>`;
    html += `<td style="color:${color};font-weight:600">${pct}%</td>`;
    html += `<td><div style="width:${barW}%;height:8px;background:${color};border-radius:4px;min-width:4px"></div></td>`;
    html += `</tr>`;
  }
  html += "</tbody></table>";
  el.innerHTML = html;
}

if (typeof window !== "undefined")
  Object.assign(window, {
    showIndustryDetail,
    closeIndustryModal,
    switchIndustryTab,
    toggleCycleLegend,
    closeCycleLegend,
    runShockSimulation,
  });
