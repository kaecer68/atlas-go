// Industry ecosystem page
import { sectorName, stockName } from "../names.js";
import {
  silentGetJSON,
  postJSON,
  notify,
  renderEmptyState,
} from "../shared/app-utils.js";
import { renderIndustrySeasonality, renderSeasonalityList, renderSeasonalityCalendar } from '../shared/components/seasonality-panel.js';
import { fetchDecisionChain, renderSectorHeatmap } from '../components/decision-panels.js';
import { getThemeColor, fmtFloat } from "../shared/utils.js";
import {
  fmtSafeNumber, fmtSafePct, fmtSafeSignedPct, fmtSafeSigned,
} from "../shared/format-metric.js";
import { hexToRgba } from "../shared/color-tokens.js";

export async function loadIndustryData() {
  try {
    const [classification, overview, seasonality, calendar, graph, cycleStatus] = await Promise.all(
      [
        silentGetJSON("/api/dashboard/industry-classification"),
        silentGetJSON("/api/dashboard/industry-overview"),
        silentGetJSON("/api/dashboard/industry-seasonality"),
        silentGetJSON("/api/dashboard/industry-seasonality-calendar"),
        silentGetJSON("/api/dashboard/industry-graph"),
        silentGetJSON("/api/dashboard/cycle-status-card"),
      ],
    );
    populateShockSourceDropdown(classification);
    renderCycleStatusCard(cycleStatus && cycleStatus.card);
    renderIndustryLinkage(overview);
    if (seasonality && calendar) {
      seasonality.calendar = calendar;
    }
    renderIndustrySeasonality(seasonality);
    renderIndustryGraph(graph);
  } catch (e) {
    console.error("loadIndustryData error:", e);
  }
  await loadSectorHeatmap();
  startSectorHeatmapPolling();
  bindSectorRefreshButton();
}

const SECTOR_POLL_INTERVAL_MS = 60000;
const SECTOR_INDICATOR_TICK_MS = 1000;
let sectorPollTimer = null;
let sectorIndicatorTimer = null;
let lastSectorUpdate = null;
let sectorFetchInFlight = false;

// 產業熱力圖 — 資料源為決策鏈聚合端點（/api/dashboard/decision-chain）的
// sector_heatmap 區塊（取代舊的 sector-allocation-plan 權重卡片）。
export async function loadSectorHeatmap() {
  if (sectorFetchInFlight) return;
  sectorFetchInFlight = true;
  setSectorRefreshSpinning(true);
  try {
    const data = await fetchDecisionChain();
    const el = document.getElementById("industryMap");
    if (el) {
      el.classList.remove("loading");
      el.innerHTML = renderSectorHeatmap(data);
    }
    lastSectorUpdate = new Date();
    updateLastUpdatedIndicator();
  } catch (e) {
    console.error("loadSectorHeatmap error:", e);
    const el = document.getElementById("industryMap");
    if (el) {
      el.classList.remove("loading");
      el.innerHTML = renderEmptyState("產業熱力圖載入失敗", "");
    }
  } finally {
    sectorFetchInFlight = false;
    setSectorRefreshSpinning(false);
  }
}

function startSectorHeatmapPolling() {
  stopSectorHeatmapPolling();
  sectorPollTimer = setInterval(loadSectorHeatmap, SECTOR_POLL_INTERVAL_MS);
  if (sectorIndicatorTimer == null) {
    sectorIndicatorTimer = setInterval(updateLastUpdatedIndicator, SECTOR_INDICATOR_TICK_MS);
  }
  if (typeof document !== "undefined" && !document.__sectorVisibilityBound) {
    document.addEventListener("visibilitychange", handleSectorVisibilityChange);
    document.__sectorVisibilityBound = true;
  }
}

function stopSectorHeatmapPolling() {
  if (sectorPollTimer != null) {
    clearInterval(sectorPollTimer);
    sectorPollTimer = null;
  }
}

function handleSectorVisibilityChange() {
  if (typeof document === "undefined") return;
  if (document.hidden) {
    stopSectorHeatmapPolling();
  } else {
    loadSectorHeatmap();
    startSectorHeatmapPolling();
  }
}

function updateLastUpdatedIndicator() {
  const el = typeof document !== "undefined" ? document.getElementById("sectorLastUpdated") : null;
  if (!el) return;
  if (!lastSectorUpdate) {
    el.textContent = "--";
    el.title = "尚未載入";
    return;
  }
  const secs = Math.max(0, Math.floor((Date.now() - lastSectorUpdate.getTime()) / 1000));
  let text;
  if (secs < 5) text = "剛剛";
  else if (secs < 60) text = `${secs} 秒前`;
  else if (secs < 3600) text = `${Math.floor(secs / 60)} 分鐘前`;
  else text = `${Math.floor(secs / 3600)} 小時前`;
  el.textContent = text;
  el.title = lastSectorUpdate.toLocaleString();
}

function setSectorRefreshSpinning(spinning) {
  const btn = typeof document !== "undefined" ? document.getElementById("sectorRefreshBtn") : null;
  if (!btn) return;
  btn.disabled = spinning;
  btn.classList.toggle("spinning", spinning);
}

function bindSectorRefreshButton() {
  const btn = typeof document !== "undefined" ? document.getElementById("sectorRefreshBtn") : null;
  if (!btn || btn.__sectorBound) return;
  btn.__sectorBound = true;
  btn.addEventListener("click", () => {
    loadSectorHeatmap();
  });
}

function confidenceColor(hex, confidence) {
  // Phase indicator opacity reflects confidence (0.3 dim … 1.0 full)
  const alpha = 0.3 + (confidence || 0) * 0.7;
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return "rgba(" + r + "," + g + "," + b + "," + fmtSafeNumber(alpha, { decimals: 2 }) + ")";
}

function cycleStatusText(value) {
  if (value == null || value === "") return "-";
  return String(value)
    .replace("recovery", "復甦")
    .replace("expansion", "擴張")
    .replace("mature", "成熟")
    .replace("recession", "衰退")
    .replace("active_restocking", "主動補庫存")
    .replace("passive_restocking", "被動補庫存")
    .replace("active_destocking", "主動去庫存")
    .replace("passive_destocking", "被動去庫存")
    .replace("maintenance", "維護性支出")
    .replace("contraction", "緊縮");
}

function cycleNumber(value, digits) {
  return fmtSafeNumber(value, { decimals: digits });
}

function cycleDelta(value) {
  return {
    text: fmtSafeSigned(value, { decimals: 3, forceSign: true }),
    color: typeof value === "number" && Number.isFinite(value)
      ? value > 0 ? "var(--up)" : value < 0 ? "var(--down)" : "var(--muted)"
      : "var(--muted)",
  };
}

function cycleEventStyle(direction) {
  if (direction === "up" || direction === "bullish") return { icon: "↑", label: "上行", color: "var(--up)" };
  if (direction === "down" || direction === "bearish") return { icon: "↓", label: "下行", color: "var(--down)" };
  return { icon: "→", label: "中性", color: "var(--muted)" };
}

function cyclePhaseBadge(value) {
  const phase = String(value || "").toLowerCase();
  const colorMap = {
    // Business cycle
    "expansion": "var(--up)",
    "recovery": "var(--up)",
    "mature": "var(--warn)",
    "recession": "var(--down)",
    // Inventory cycle
    "active_restocking": "var(--up)",
    "passive_restocking": "var(--warn)",
    "active_destocking": "var(--down)",
    "passive_destocking": "var(--warn)",
    // Capex cycle
    "contraction": "var(--down)",
    "maintenance": "var(--warn)",
  };
  const color = colorMap[phase] || "var(--muted)";
  return `<span style="color:${color};border:1px solid ${color};border-radius:999px;padding:2px 8px;font-size:11px;font-weight:700;background:var(--bg)">${cycleStatusText(value)}</span>`;
}

export async function loadCycleStatusCard() {
  const el = document.getElementById("industryCycle");
  if (el) {
    el.classList.add("loading");
    el.innerHTML = '<div style="text-align:center;padding:20px;color:var(--muted)">載入週期邏輯中…</div>';
  }
  const data = await silentGetJSON("/api/dashboard/cycle-status-card");
  renderCycleStatusCard(data && data.card);
}

export function renderIndustryCycle(data) {
  loadCycleStatusCard();
}

export function renderCycleStatusCard(card) {
  const el = document.getElementById("industryCycle");
  if (!el) return;
  if (!card) {
    el.innerHTML = renderEmptyState("尚無週期資料", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.remove("loading");

  const breakdown = card.breakdown || [];
  const breakdownByLayer = new Map(breakdown.map((item) => [item.layer, item]));
  const layerDefs = [
    { key: "silicon", label: "矽循環" },
    { key: "business_cycle", label: "商業週期" },
    { key: "seasonal", label: "季節性" },
    { key: "events", label: "日曆事件" },
    { key: "supply_chain", label: "供應鏈" },
  ];
  const sentimentColors = {
    強烈看多: "var(--trend-bullish)",
    偏多: "color-mix(in srgb, var(--trend-bullish) 65%, transparent)",
    中性: "var(--muted)",
    偏空: "color-mix(in srgb, var(--trend-bearish) 65%, transparent)",
    強烈看空: "var(--trend-bearish)",
  };
  const sentiment = card.sentiment_label || "中性";
  const sentimentColor = sentimentColors[sentiment] || sentimentColors["中性"];
  const generatedAt = card.generated_at ? new Date(card.generated_at).toLocaleString("zh-TW") : "-";
  const confidence = card.cycle_confidence;
  const phaseIndex = Math.max(0, Math.min(3, Number(card.silicon_phase || 0)));
  const activeEvents = card.active_events || [];
  const activePatterns = card.active_patterns || [];

  const INDICATOR_LABELS = {
    tsmc_monthly_revenue_yoy: "台積電月營收年增率",
    global_semiconductor_billings_yoy: "全球半導體出貨年增率",
    dram_spot_price_trend: "DRAM 現貨價格趨勢",
    taiwan_semiconductor_index_ma: "半導體指數偏離季線",
    tsmc_capex_guidance: "台積電資本支出指引",
    philadelphia_sox_index_yoy: "費城半導體指數年增率",
  };
  let html = `<div style="display:flex;flex-direction:column;gap:12px">`;

  html += `<div style="position:relative;overflow:hidden;background:linear-gradient(135deg,${sentimentColor},rgba(255,255,255,0.04));border:1px solid ${sentimentColor};border-radius:14px;padding:18px;color:#fff;box-shadow:0 12px 32px rgba(0,0,0,0.18)">`;
  html += `<div style="position:absolute;right:-42px;top:-52px;width:160px;height:160px;border-radius:50%;background:rgba(255,255,255,0.14)"></div>`;
  html += `<div style="position:relative;display:flex;justify-content:space-between;gap:12px;align-items:center;flex-wrap:wrap">`;
  html += `<div><div style="font-size:12px;opacity:0.86;margin-bottom:6px">Composite Sentiment Gauge</div><div style="font-size:46px;line-height:1;font-weight:800;letter-spacing:-1px">${cycleNumber(card.composite_coefficient, 3)}x</div><div style="font-size:20px;font-weight:800;margin-top:6px">${sentiment}</div></div>`;
  html += `<div style="min-width:190px;background:rgba(0,0,0,0.16);border:1px solid rgba(255,255,255,0.26);border-radius:12px;padding:12px"><div style="font-size:11px;opacity:0.8;margin-bottom:4px">生成時間</div><div style="font-size:12px;font-weight:700">${generatedAt}</div><div style="margin-top:10px;font-size:11px;opacity:0.8">有利狀態</div><div style="font-size:13px;font-weight:800">${card.is_favorable ? "✅ 有利" : "⚠️ 保守"}</div></div>`;
  html += `</div></div>`;

  html += `<div style="display:flex;gap:8px;flex-wrap:wrap">`;
  layerDefs.forEach((layer) => {
    const item = breakdownByLayer.get(layer.key) || {};
    const delta = cycleDelta(item.contribution);
    const weight = fmtSafePct(item.weight, 0);
    html += `<div style="flex:1;min-width:132px;background:var(--bg);border:1px solid var(--border);border-radius:10px;padding:10px">`;
    html += `<div style="font-size:12px;color:var(--muted);margin-bottom:6px">${layer.label}</div>`;
    html += `<div style="display:flex;justify-content:space-between;align-items:flex-end;gap:6px"><span style="font-size:18px;font-weight:800">${cycleNumber(item.raw_value, 3)}</span><span style="font-size:13px;font-weight:800;color:${delta.color}">${delta.text}</span></div>`;
    html += `<div style="font-size:10px;color:var(--muted);margin-top:4px">權重 ${weight}</div>`;
    html += `<div style="font-size:10px;color:var(--muted);margin-top:6px;line-height:1.35;min-height:28px">${item.reason || "尚無原因說明"}</div>`;
    html += `</div>`;
  });
  html += `</div>`;

  html += `<div style="display:flex;flex-direction:column;gap:12px">`;
  html += `<div style="background:var(--bg);border:1px solid var(--border);border-radius:12px;padding:12px">`;
  html += `<div style="display:flex;justify-content:space-between;align-items:flex-start;gap:8px;margin-bottom:10px"><div><div style="font-weight:800;font-size:14px">矽循環時鐘</div><div style="font-size:11px;color:var(--muted);margin-top:2px">Phase ${phaseIndex} · ${card.silicon_phase_name || "-"}</div></div><div style="text-align:right"><div style="font-size:11px;color:var(--muted)">Score</div><div style="font-size:18px;font-weight:800;color:var(--accent)">${cycleNumber(card.silicon_score, 3)}</div></div></div>`;
  html += `<div style="display:flex;gap:5px;margin:8px 0 12px">`;
  [0, 1, 2, 3].forEach((idx) => {
    const active = idx <= phaseIndex;
    const successColor = getThemeColor("--color-success") || "#10b981";
    const color = active ? confidenceColor(successColor, 0.55 + idx * 0.1) : "var(--border)";
    html += `<div style="flex:1;height:12px;border-radius:999px;background:${color};border:1px solid var(--border)"></div>`;
  });
  html += `</div>`;
  const indicators = card.silicon_indicators || {};
  const indicatorEntries = Object.entries(indicators);
  if (indicatorEntries.length > 0) {
    html += `<div style="overflow-x:auto;max-width:100%"><table style="font-size:11px;width:100%"><thead><tr><th>指標</th><th>值</th><th>趨勢</th></tr></thead><tbody>`;
    indicatorEntries.forEach(([key, raw]) => {
      const value = raw && typeof raw === "object" ? raw.value : raw;
      const trend = raw && typeof raw === "object" && raw.trend !== undefined ? raw.trend : value;
      let arrow = "—";
      let color = "var(--muted)";
      if (trend === "down" || (typeof trend === "number" && trend < 0)) {
        arrow = "↓";
        color = "var(--down)";
      } else if (trend === "up" || (typeof trend === "number" && trend > 0)) {
        arrow = "↑";
        color = "var(--up)";
      } else if (trend === "neutral" || trend === 0) {
        arrow = "→";
        color = "var(--muted)";
      }
      html += `<tr><td>${INDICATOR_LABELS[key] || key}</td><td style="white-space:nowrap">${fmtFloat(value)}</td><td style="color:${color};font-weight:800">${arrow}</td></tr>`;
    });
    html += `</tbody></table></div>`;
  } else {
    html += `<div style="font-size:11px;color:var(--muted);padding:8px;border:1px dashed var(--border);border-radius:8px">尚無矽循環指標明細</div>`;
  }
  html += `</div>`;

  html += `<div style="background:var(--bg);border:1px solid var(--border);border-radius:12px;padding:12px">`;
  html += `<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px"><div style="font-weight:800;font-size:14px">活躍台股日曆事件</div><span style="font-size:11px;background:var(--accent);color:#fff;border-radius:999px;padding:2px 8px">${activeEvents.length} 件</span></div>`;
  if (activeEvents.length > 0) {
    activeEvents.forEach((event) => {
      const style = cycleEventStyle(event.direction);
      html += `<div style="display:flex;gap:8px;align-items:flex-start;padding:8px 0;border-top:1px solid var(--border)">`;
      html += `<div style="width:24px;height:24px;border-radius:50%;display:flex;align-items:center;justify-content:center;color:${style.color};border:1px solid ${style.color};font-weight:800">${style.icon}</div>`;
      html += `<div style="flex:1"><div style="font-size:12px;font-weight:800">${event.name || event.event_type || "未命名事件"}</div><div style="font-size:10px;color:var(--muted);margin-top:2px">${style.label} · 權重 ${cycleNumber(event.base_weight, 2)} · 情緒 ${cycleNumber(event.sentiment_adjustment, 3)}x</div></div>`;
      html += `</div>`;
    });
  } else {
    html += `<div style="font-size:11px;color:var(--muted);padding:8px;border:1px dashed var(--border);border-radius:8px">目前無活躍日曆事件</div>`;
  }
  html += `</div></div>`;

  html += `<div style="background:var(--bg);border:1px solid var(--border);border-radius:12px;padding:12px">`;
  html += `<div style="display:flex;justify-content:space-between;gap:10px;flex-wrap:wrap;margin-bottom:10px"><div style="font-weight:800;font-size:14px">商業週期與季節性</div><div style="font-size:11px;color:var(--muted)">季節調整 ${cycleNumber(card.seasonal_adjustment, 3)}x · 事件情緒 ${cycleNumber(card.event_sentiment, 3)}x · 供應鏈 ${cycleNumber(card.supply_chain_signal, 3)}</div></div>`;
  html += `<div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:10px"><div style="font-size:12px">商業 ${cyclePhaseBadge(card.business_cycle)}</div><div style="font-size:12px">庫存 ${cyclePhaseBadge(card.inventory_cycle)}</div><div style="font-size:12px">資本支出 ${cyclePhaseBadge(card.capex_cycle)}</div></div>`;
  html += `<div style="display:flex;align-items:center;gap:8px;margin-bottom:10px"><span style="font-size:11px;color:var(--muted);width:70px">信心度</span><div style="flex:1;height:10px;background:var(--border);border-radius:999px;overflow:hidden"><div style="width:${typeof confidence === "number" && Number.isFinite(confidence) ? Math.min(100, Math.max(0, confidence * 100)) : 0}%;height:100%;background:var(--accent)"></div></div><span style="font-size:12px;font-weight:800">${fmtSafePct(confidence, 0)}</span></div>`;
  if (activePatterns.length > 0) {
    html += `<div style="display:flex;flex-wrap:wrap;gap:6px">`;
    activePatterns.forEach((pattern) => {
      html += `<span style="font-size:11px;background:rgba(79,193,255,0.08);border:1px solid rgba(79,193,255,0.24);color:var(--accent);border-radius:999px;padding:3px 8px">${pattern.name || pattern.id || "季節模式"} ${fmtSafeNumber(pattern.adjustment_factor, { decimals: 2, suffix: "x" })}</span>`;
    });
    html += `</div>`;
  } else {
    html += `<div style="font-size:11px;color:var(--muted)">目前無活躍季節性模式</div>`;
  }
  html += `</div>`;

  html += `<details style="background:var(--bg);border:1px solid var(--border);border-radius:12px;padding:12px">`;
  html += `<summary style="cursor:pointer;font-weight:800;font-size:14px">決策鏈分解 (Decision Chain Breakdown)</summary>`;
  if (breakdown.length > 0) {
    html += `<table style="font-size:11px;margin-top:10px"><thead><tr><th>層級</th><th>原始值</th><th>權重</th><th>貢獻值</th><th>原因</th></tr></thead><tbody>`;
    const layerLabels = {silicon:"矽循環",business_cycle:"商業週期",seasonal:"季節性",events:"事件",supply_chain:"供應鏈"};
    breakdown.forEach((item) => {
      const delta = cycleDelta(item.contribution);
      let reason = item.reason || "";
      reason = reason.replace(/silicon phase=/,"矽階段=").replace(/^phase=/,"階段=").replace(/score=/g,"評分=").replace(/confidence=/g,"信賴度=").replace(/(\d+) active patterns/,"$1 個活躍模式").replace(/(\d+) active events/,"$1 個活躍事件").replace("upstream-downstream momentum","上下游動能") || "-";
      html += `<tr><td>${layerLabels[item.layer] || item.layer || "-"}</td><td>${cycleNumber(item.raw_value, 3)}</td><td>${fmtSafePct(item.weight, 0)}</td><td style="color:${delta.color};font-weight:800">${delta.text}</td><td>${reason}</td></tr>`;
    });
    html += `</tbody></table>`;
  } else {
    html += `<div style="font-size:11px;color:var(--muted);margin-top:10px">尚無決策鏈分解資料</div>`;
  }
  html += `</details>`;

  html += `</div>`;
  el.innerHTML = html;
}

export function renderIndustryLinkage(data) {
  const el = document.getElementById("industryLinkage");
  if (!el) return;
  if (!data || !data.industries) {
    el.innerHTML = renderEmptyState("尚無產業關聯資料", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.remove("loading");
  const industries = data.industries;

  // Calculate historical averages across all industries (skip missing values)
  let totalSystemicImportance = 0;
  let totalPropagationSpeed = 0;
  let maxSystemic = -Infinity;
  let maxPropagation = -Infinity;
  let siCount = 0;
  let spCount = 0;

  industries.forEach((ind) => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance;
    const sp = score.shock_propagation_speed;
    if (typeof si === "number" && Number.isFinite(si)) {
      totalSystemicImportance += si;
      if (si > maxSystemic) maxSystemic = si;
      siCount++;
    }
    if (typeof sp === "number" && Number.isFinite(sp)) {
      totalPropagationSpeed += sp;
      if (sp > maxPropagation) maxPropagation = sp;
      spCount++;
    }
  });

  const avgSystemic = siCount > 0 ? totalSystemicImportance / siCount : null;
  const avgPropagation = spCount > 0 ? totalPropagationSpeed / spCount : null;

  let html =
    '<div style="font-size:11px;color:var(--muted);margin-bottom:10px;padding:8px;background:var(--bg);border-radius:6px">' +
    "<strong>數據說明：</strong>「系統重要性」衡量該產業在整體經濟中的關鍵程度（0-1）；「連動分數」反映衝擊傳導速度，數值越高表示該產業受外部衝擊影響越快擴散至其他產業。" +
    "</div>";

  // Summary stats
  html += '<div style="display:flex;gap:8px;margin-bottom:12px">';
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">平均系統重要性</div>`;
  html += `<div style="font-size:16px;font-weight:700">${fmtFloat(avgSystemic)}</div>`;
  html += `</div>`;
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">平均連動分數</div>`;
  html += `<div style="font-size:16px;font-weight:700">${fmtFloat(avgPropagation)}</div>`;
  html += `</div>`;
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">最高系統重要性</div>`;
  html += `<div style="font-size:16px;font-weight:700">${fmtFloat(maxSystemic !== -Infinity ? maxSystemic : null)}</div>`;
  html += `</div>`;
  html += "</div>";

  html +=
    "<table><thead><tr><th>產業</th><th>系統重要性</th><th>連動分數</th><th>相對強度</th></tr></thead><tbody>";
  industries.forEach((ind) => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance;
    const sp = score.shock_propagation_speed;
    const siRelative = avgSystemic != null && avgSystemic > 0 && typeof si === "number" ? si / avgSystemic : null;
    const spRelative = avgPropagation != null && avgPropagation > 0 && typeof sp === "number" ? sp / avgPropagation : null;
    const overallStrength = siRelative != null && spRelative != null ? (siRelative + spRelative) / 2 : null;

    let strengthLabel = "—";
    let strengthColor = "var(--muted)";
    if (overallStrength != null) {
      strengthLabel = "平均";
      strengthColor = "var(--muted)";
      if (overallStrength > 1.3) {
        strengthLabel = "高";
        strengthColor = "var(--up)";
      } else if (overallStrength < 0.7) {
        strengthLabel = "低";
        strengthColor = "var(--down)";
      }
    }

    html += `<tr><td>${ind.name}</td><td>${fmtFloat(si)}</td><td>${fmtFloat(sp)}</td><td style="color:${strengthColor}">${strengthLabel}</td></tr>`;
  });
  html += "</tbody></table>";
  el.innerHTML = html;
}

export function renderIndustryGraph(data) {
  const el = document.getElementById("industryGraph");
  if (!el) return;
  if (!data || !data.nodes || data.nodes.length === 0) {
    el.innerHTML = renderEmptyState("尚無網路圖資料", "");
    el.classList.remove("loading");
    return;
  }
  el.classList.remove("loading");

  const rect = el.getBoundingClientRect();
  const width = rect.width || 800;
  const height = rect.height || 400;

  const dpr = window.devicePixelRatio || 1;
  el.innerHTML =
    `<div class="industry-graph__title">產業關聯網路<div class="industry-graph__subtitle">節點大小反映系統重要性，線條顏色代表相關性方向</div></div>` +
    `<div class="industry-graph__canvas-wrap" style="position:relative;width:100%;flex:1;min-height:0;">` +
      `<canvas width="${width * dpr}" height="${height * dpr}" style="width:${width}px;height:${height}px;display:block;"></canvas>` +
      `<div class="industry-graph__tooltip" id="industryGraphTip"></div>` +
    `</div>` +
    `<div class="industry-graph__legend">` +
      `<span class="industry-graph__legend-item"><span class="industry-graph__legend-line" style="background:var(--pnl-profit);"></span>紅線：正向相關</span>` +
      `<span class="industry-graph__legend-item"><span class="industry-graph__legend-line" style="background:var(--pnl-loss);"></span>綠線：負向相關</span>` +
      `<span class="industry-graph__legend-item">節點大小：系統重要性</span>` +
    `</div>`;
  const canvas = el.querySelector("canvas");
  const ctx = canvas.getContext("2d");
  ctx.scale(dpr, dpr);

  const nodes = data.nodes.map(n => ({
    ...n,
    x: width / 2 + (Math.random() - 0.5) * width * 0.5,
    y: height / 2 + (Math.random() - 0.5) * height * 0.5,
    vx: 0,
    vy: 0,
    radius: 8 + (n.systemic_importance || 0) * 15
  }));

  const nodeMap = new Map(nodes.map(n => [n.id, n]));

  const edges = (data.edges || []).map(e => ({
    ...e,
    sourceNode: nodeMap.get(e.source),
    targetNode: nodeMap.get(e.target)
  })).filter(e => e.sourceNode && e.targetNode);

  const iterations = 150;
  const k = Math.sqrt((width * height) / nodes.length);
  const attraction = 0.05;
  const repulsion = k * k * 0.8;

  for (let i = 0; i < iterations; i++) {
    for (let j = 0; j < nodes.length; j++) {
      for (let l = j + 1; l < nodes.length; l++) {
        const n1 = nodes[j];
        const n2 = nodes[l];
        const dx = n1.x - n2.x;
        const dy = n1.y - n2.y;
        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const force = repulsion / dist;
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;
        n1.vx += fx;
        n1.vy += fy;
        n2.vx -= fx;
        n2.vy -= fy;
      }
    }

    for (const edge of edges) {
      const n1 = edge.sourceNode;
      const n2 = edge.targetNode;
      const dx = n1.x - n2.x;
      const dy = n1.y - n2.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const corr = typeof edge.correlation === "number" && Number.isFinite(edge.correlation)
        ? Math.abs(edge.correlation)
        : 0.5;
      const force = (dist * dist) / k * attraction * (0.5 + corr);
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      n1.vx -= fx;
      n1.vy -= fy;
      n2.vx += fx;
      n2.vy += fy;
    }

    for (const n of nodes) {
      const dx = width / 2 - n.x;
      const dy = height / 2 - n.y;
      n.vx += dx * 0.02;
      n.vy += dy * 0.02;
    }

    for (const n of nodes) {
      const speed = Math.sqrt(n.vx * n.vx + n.vy * n.vy);
      const maxSpeed = 15;
      if (speed > maxSpeed) {
        n.vx = (n.vx / speed) * maxSpeed;
        n.vy = (n.vy / speed) * maxSpeed;
      }
      n.x += n.vx;
      n.y += n.vy;
      n.vx *= 0.85;
      n.vy *= 0.85;

      n.x = Math.max(n.radius + 20, Math.min(width - n.radius - 20, n.x));
      n.y = Math.max(n.radius + 20, Math.min(height - n.radius - 20, n.y));
    }
  }

  ctx.clearRect(0, 0, width, height);

  const upColor = getThemeColor("--up") || "#ef4444";
  const downColor = getThemeColor("--down") || "#10b981";

  for (const edge of edges) {
    const corr = edge.correlation || 0;
    ctx.beginPath();
    ctx.moveTo(edge.sourceNode.x, edge.sourceNode.y);
    ctx.lineTo(edge.targetNode.x, edge.targetNode.y);
    ctx.lineWidth = 1 + Math.abs(corr) * 3;
    ctx.strokeStyle = corr > 0 ? hexToRgba(upColor, 0.3) : hexToRgba(downColor, 0.3);
    ctx.stroke();
  }

  ctx.font = "11px sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";

  for (const n of nodes) {
    ctx.beginPath();
    ctx.arc(n.x, n.y, n.radius, 0, Math.PI * 2);
    
    const upCount = n.upstream_count || 0;
    const hue = Math.max(0, 220 - upCount * 30);
    ctx.fillStyle = `hsla(${hue}, 80%, 60%, 0.8)`;
    ctx.fill();
    ctx.strokeStyle = "rgba(255,255,255,0.3)";
    ctx.lineWidth = 1.5;
    ctx.stroke();

    ctx.fillStyle = getThemeColor("--text") || "#e2e8f0";
    const name = sectorName(n.id) || n.id;
    ctx.fillText(name, n.x, n.y + n.radius + 12);
  }

  // Tooltip: hover detection via distance check
  const tip = document.getElementById("industryGraphTip");
  canvas.addEventListener("mousemove", function(e) {
    const cr = canvas.getBoundingClientRect();
    const mx = e.clientX - cr.left;
    const my = e.clientY - cr.top;
    let found = null;
    for (const n of nodes) {
      const dx = mx - n.x;
      const dy = my - n.y;
      if (Math.sqrt(dx * dx + dy * dy) < n.radius + 4) { found = n; break; }
    }
    if (found && tip) {
      // Count connected edges
      const connectedCount = edges.filter(
        e => e.sourceNode === found || e.targetNode === found
      ).length;
      tip.innerHTML =
        `<strong>${sectorName(found.id) || found.id}</strong>` +
        `<div class="ind-tip-row"><span>系統重要性</span><span>${fmtFloat(found.systemic_importance)}</span></div>` +
        `<div class="ind-tip-row"><span>連接產業</span><span>${connectedCount} 個</span></div>`;
      tip.style.display = "block";
      tip.style.left = Math.min(mx + 14, width - 140) + "px";
      tip.style.top = Math.max(my - 40, 4) + "px";
    } else if (tip) {
      tip.style.display = "none";
    }
  });
  canvas.addEventListener("mouseleave", function() {
    if (tip) tip.style.display = "none";
  });
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
      value: fmtSafeNumber(cp.phase_score, { decimals: 2 }),
    },
    {
      label: "信心度",
      value: fmtSafePct(cp.confidence, 0),
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
    html += '<div style="font-size:11px;color:var(--muted);margin-bottom:8px">各維度信心分數 × 配置權重 → 複合信心 <strong>' + fmtSafePct(cb.composite, 0) + '</strong></div>';
    dims.forEach(function(d) {
      const val = cb[d.key];
      const valStr = fmtSafePct(val, 0);
      if (valStr === '—') return;
      const barW = Math.min(Math.max(0, val) * 100, 100);
      const wPct = fmtSafePct(d.weight, 0);
      html += '<div style="display:flex;align-items:center;gap:6px;margin:3px 0;font-size:11px">';
      html += '<span style="width:80px;color:var(--muted)">' + d.label + '</span>';
      html += '<div style="flex:1;height:14px;background:rgba(0,0,0,0.04);border-radius:3px;overflow:hidden">';
      html += '<div style="width:' + barW + '%;height:100%;background:var(--accent);opacity:0.5;border-radius:3px"></div></div>';
      html += '<span style="width:40px;text-align:right">' + valStr + '</span>';
      html += '<span style="width:30px;color:var(--muted);font-size:10px;text-align:right">w=' + wPct + '</span>';
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
      html += `<div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="color:var(--muted)">目標權重</span><span>${fmtSafePct(rec.target_weight, 1)}</span></div>`;
    if (rec.delta != null)
      html += `<div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="color:var(--muted)">調整幅度</span><span>${fmtSafeSignedPct(rec.delta, 1)}</span></div>`;
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
    html += `<div style="font-size:16px;font-weight:700">${fmtFloat(ls.systemic_importance)}</div></div>`;
    html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
    html += `<div style="font-size:11px;color:var(--muted)">衝擊傳導速度</div>`;
    html += `<div style="font-size:16px;font-weight:700">${fmtFloat(ls.shock_propagation_speed)}</div></div>`;
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
      const corr = c.correlation;
      const absCorr = typeof corr === "number" && Number.isFinite(corr) ? Math.abs(corr) : null;
      const strength = absCorr == null ? "—" : absCorr > 0.7 ? "高" : absCorr > 0.4 ? "中" : "低";
      const color = typeof corr === "number" && Number.isFinite(corr)
        ? corr > 0 ? "var(--up)" : corr < 0 ? "var(--down)" : "var(--muted)"
        : "var(--muted)";
      html += `<tr><td>${sectorName(c.industry) || c.industry || "-"}</td>`;
      html += `<td style="color:${color}">${fmtFloat(corr)}</td>`;
      html += `<td>${strength}</td></tr>`;
    });
    html += "</tbody></table></div>";
  }

  return html;
}

export function renderSeasonalityTab(detail) {
  const patterns = detail.seasonal_patterns;
  if (!patterns || patterns.length === 0)
    return renderEmptyState("尚無季節性模式資料", "");

  let html = '<div class="industry-section"><h4>📅 季節性模式</h4>';
  html += '<div style="font-size:11px;color:var(--warn);margin-bottom:8px;padding:6px 10px;background:rgba(245,158,11,0.08);border:1px solid rgba(245,158,11,0.2);border-radius:6px">' +
    '⚠️ 以下數值基於經驗法則，尚未經過回測校準。' +
    '</div>';
  patterns.forEach((p) => {
    const accuracyStr = fmtSafePct(p.historical_accuracy, 0);
    const returnColor = typeof p.avg_market_return === "number" && Number.isFinite(p.avg_market_return)
      ? p.avg_market_return > 0 ? "var(--up)" : p.avg_market_return < 0 ? "var(--down)" : "var(--muted)"
      : "var(--muted)";
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
    const calEvidence = (typeof window !== 'undefined' && window.seasonalityData && window.seasonalityData.calibration_evidence);
    const accBadge = calEvidence && calEvidence.calibrated
      ? `<span style="font-size:10px;color:var(--ok);background:rgba(79,193,255,0.1);padding:1px 4px;border-radius:3px">已校準</span>`
      : `<span style="font-size:10px;color:var(--warn);background:rgba(245,158,11,0.1);padding:1px 4px;border-radius:3px">待驗證</span>`;
    html += `<span class="metric-label">歷史準確度</span><span class="metric-value">${accuracyStr} ${accBadge}</span></div>`;
    html += '<div class="metric-row">';
    html += `<span class="metric-label">典型報酬</span><span class="metric-value" style="color:${returnColor}">${fmtSafeSignedPct(p.avg_market_return, 1)}</span></div>`;
    html += '<div class="metric-row">';
    html += `<span class="metric-label">調整因子</span><span class="metric-value">${fmtSafeNumber(p.adjustment_factor, { decimals: 2, suffix: "x" })}</span></div>`;
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
        ? "var(--risk-high)"
        : severity === "medium"
          ? "var(--warn)"
          : "var(--risk-low)";

    html += '<div class="risk-item">';
    html += '<div class="risk-header">';
    html += `<span style="font-weight:600;font-size:13px">${r.type || "未知風險"}</span>`;
    html += `<span class="risk-severity ${severity}" style="color:${impactColor}">${severityLabel}</span>`;
    html += "</div>";
    if (r.description)
      html += `<div style="font-size:12px;margin-bottom:4px">${r.description}</div>`;
    if (r.impact_estimate != null) {
      html += '<div class="metric-row">';
      html += `<span class="metric-label">預估衝擊</span><span class="metric-value" style="color:${impactColor}">${fmtSafePct(r.impact_estimate, 1)}</span></div>`;
    }
    if (r.confidence != null) {
      html += '<div class="metric-row">';
      html += `<span class="metric-label">信心度</span><span class="metric-value">${fmtSafePct(r.confidence, 0)}</span></div>`;
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

async function runShockSimulation() {
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
}

function renderShockSimulationResult(data) {
  const el = document.getElementById("industryShockSim");
  el.classList.remove("loading");
  if (!data || !data.impacts || data.impacts.length === 0) {
    el.innerHTML = renderEmptyState("該衝擊未影響其他產業", "");
    return;
  }
  const sorted = [...data.impacts].sort((a, b) => Math.abs(b.impact) - Math.abs(a.impact));
  let html = '<div style="font-size:11px;color:var(--muted);margin-bottom:8px">';
  html += `<strong>${data.source}</strong> 遭受 <strong>${fmtSafePct(data.shock, 1)}</strong> 衝擊 → 影響 ${data.impact_count} 個關聯產業</div>`;
  html += '<table style="font-size:12px"><thead><tr><th>產業</th><th>影響幅度</th><th>衝擊傳導</th></tr></thead><tbody>';
  for (const imp of sorted) {
    const pctStr = fmtSafeSignedPct(imp.impact, 1);
    const absPct = typeof imp.impact === "number" && Number.isFinite(imp.impact) ? Math.abs(imp.impact * 100) : 0;
    const color = typeof imp.impact === "number" && Number.isFinite(imp.impact)
      ? imp.impact < 0 ? "var(--down)" : imp.impact > 0 ? "var(--up)" : "var(--muted)"
      : "var(--muted)";
    const barW = Math.min(100, absPct * 8);
    html += `<tr>`;
    html += `<td>${imp.industry}</td>`;
    html += `<td style="color:${color};font-weight:600">${pctStr}</td>`;
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
