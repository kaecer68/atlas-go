// Risk Control Page - Enhanced Risk Indicators
// Extracted from index.html - DO NOT EDIT inline
import { sectorName, renderStockCell } from '../names.js';
import { escapeHtml, fmtNTD, fmtInt, pnlColor } from '../shared/utils.js';
import { fmtSafeNumber, fmtSafeDrawdown, fmtSafeSignedPct } from '../shared/format-metric.js';
import { renderEmptyState, formatDate } from '../shared/app-utils.js';

function isFiniteNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

export function renderLiveStatus(data) {
  const el = document.getElementById('liveStatus');
  if (!el) return;
  el.classList.remove('loading');
  if (!data || !data.circuit_breaker) {
    el.innerHTML = '<div class="empty">即時狀態暫無資料</div>';
    return;
  }
  const cb = data.circuit_breaker;
  const pf = data.portfolio || {};
  const pnl = pf.unrealized_pnl;
  const dayPnl = pf.day_pnl;
  const cbState = cb.state === 'tripped' ? '已觸發' : '正常';
  const cbStateColor = cb.state === 'tripped' ? 'var(--color-danger)' : 'var(--color-success)';

  let cooldownInfo = '';
  if (cb.cooldown_until && cb.cooldown_until !== '0001-01-01T00:00:00Z') {
    const cd = new Date(cb.cooldown_until);
    const now = new Date();
    if (cd > now) {
      const mins = Math.ceil((cd - now) / 60000);
      cooldownInfo = `<div class="metric"><div class="label">冷卻中</div><div class="value warn">${mins} 分鐘</div></div>`;
    }
  }

  let slWarning = '';
  if (cb.consecutive_sl > 0) {
    const slColor = cb.consecutive_sl >= 3 ? 'var(--color-danger)' : 'var(--color-warning)';
    slWarning = `<div class="metric"><div class="label">連續止損</div><div class="value" style="color:${slColor}">${fmtInt(cb.consecutive_sl)} 次</div></div>`;
  }

  el.innerHTML = `
    <div class="metric"><div class="label">熔斷機制</div><div class="value" style="color:${cbStateColor}">${cbState}</div></div>
    <div class="metric"><div class="label">現金</div><div class="value">${fmtNTD(pf.cash)}</div></div>
    <div class="metric"><div class="label">持倉市值</div><div class="value">${fmtNTD(pf.total_exposure)}</div></div>
    <div class="metric"><div class="label">持倉數</div><div class="value">${fmtInt(pf.positions_count)}</div></div>
    <div class="metric"><div class="label">未實現損益</div><div class="value" style="color:${pnlColor(pnl)}">${fmtNTD(pnl)}</div></div>
    <div class="metric"><div class="label">當日損益</div><div class="value" style="color:${pnlColor(dayPnl)}">${fmtNTD(dayPnl)}</div></div>
    ${slWarning}
    ${cooldownInfo}
  `;
}

export function renderRiskCards(riskExposure, pipelineData, capitalPhase) {
  const el = document.getElementById('riskCards');
  const panel = document.getElementById('riskCardsPanel');
  const posEl = document.getElementById('riskPositionConcentration');
  const sectorEl = document.getElementById('riskSectorDistribution');

  if (!el || !riskExposure) {
    if (panel) panel.style.display = 'none';
    return;
  }

  panel.style.display = '';
  panel.classList.remove('loading');
  el.classList.remove('loading');

  const re = riskExposure;
  const insufficient = re.insufficient_data || (typeof re.data_points === 'number' && re.data_points < 30);

  const cp = capitalPhase || {};
  const phaseLabel = { advance: '🚀 推進', reduce: '🔻 縮減', standby: '⏸️ 觀望' };
  const phase = phaseLabel[cp.phase] || cp.phase || '—';
  const rollingSharpeRaw = cp.rolling_sharpe;
  const rollingSharpe = fmtSafeNumber(rollingSharpeRaw, { decimals: 2, useGrouping: true });
  const rollingSharpeColor = isFiniteNumber(rollingSharpeRaw)
    ? (rollingSharpeRaw > 0.5 ? 'var(--up)' : (rollingSharpeRaw < 0 ? 'var(--down)' : 'var(--warn)'))
    : 'var(--muted)';
  const consecLosses = cp.consecutive_losses;
  const daysInPhase = cp.days_in_phase;
  const canAdvance = cp.can_advance;

  let concentrationHtml = '';
  const conc = re.concentration || [];
  if (conc.length > 0) {
    const top5Weight = conc.reduce((s, c) => s + (isFiniteNumber(c.weight) ? c.weight : 0), 0);
    const top1Weight = isFiniteNumber(conc[0].weight) ? conc[0].weight : 0;
    const top3Weight = conc.slice(0, 3).reduce((s, c) => s + (isFiniteNumber(c.weight) ? c.weight : 0), 0);

    const rows = conc.map((c, idx) => {
      return `<tr><td style="padding:3px 8px;font-size:12px">${idx + 1}</td><td style="padding:3px 8px;font-size:12px">${c.symbol ? renderStockCell(c.symbol) : '—'}</td><td style="padding:3px 8px;font-size:12px;text-align:right">${fmtSafeNumber(c.weight, { percent: true, decimals: 1 })}</td><td style="padding:3px 8px;font-size:12px;text-align:right">${fmtNTD(c.market_value)}</td></tr>`;
    }).join('');

    concentrationHtml = `
      <div style="display:flex;gap:16px;flex-wrap:wrap;margin-top:12px">
        <div style="flex:1;min-width:180px">
          <div style="font-size:12px;color:var(--muted);margin-bottom:6px">持倉集中度（市值）</div>
          <div style="font-size:20px;font-weight:700;color:${top5Weight > 0.6 ? 'var(--color-danger)' : (top5Weight > 0.4 ? 'var(--warn)' : 'var(--color-success)')}">${fmtSafeNumber(top5Weight, { percent: true, decimals: 1 })}</div>
          <div style="font-size:11px;color:var(--muted);margin-top:4px">前 3 大 ${fmtSafeNumber(top3Weight, { percent: true, decimals: 1 })} · 最大 ${fmtSafeNumber(top1Weight, { percent: true, decimals: 1 })}</div>
        </div>
        <div style="flex:2;min-width:300px">
          <table style="width:100%;font-size:12px;border-collapse:collapse">
            <thead><tr style="border-bottom:1px solid var(--border)"><th style="text-align:left;padding:4px 8px">#</th><th style="text-align:left;padding:4px 8px">標的</th><th style="text-align:right;padding:4px 8px">權重</th><th style="text-align:right;padding:4px 8px">市值</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </div>
    `;
  } else {
    concentrationHtml = `<div style="font-size:12px;color:var(--muted);margin-top:12px">暫無持倉資料</div>`;
  }

  let sectorHtml = '';
  const sectors = (re.sector_exposure || [])
    .filter(s => isFiniteNumber(s.weight) && s.weight > 0)
    .sort((a, b) => b.weight - a.weight);

  if (sectors.length > 0) {
    const maxW = Math.max(...sectors.map(s => s.weight), 0.01);
    const sectorBars = sectors.map(s => {
      const w = s.weight;
      const pct = fmtSafeNumber(w, { percent: true, decimals: 1 });
      const barPct = fmtSafeNumber(w / maxW, { percent: true, decimals: 1 });
      const color = w > 0.3 ? 'var(--accent)' : (w > 0.15 ? 'var(--warn)' : 'var(--muted)');
      return `
        <div style="margin:4px 0">
          <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">
            <span>${escapeHtml(sectorName(s.sector) || s.sector)}</span>
            <span>${pct}</span>
          </div>
          <div style="width:100%;height:6px;background:var(--bg);border-radius:3px;overflow:hidden">
            <div style="width:${barPct}%;height:100%;background:${color};border-radius:3px;transition:width 0.3s"></div>
          </div>
        </div>
      `;
    }).join('');

    sectorHtml = `
      <div style="margin-top:16px">
        <div style="font-size:13px;font-weight:700;margin-bottom:8px">板塊曝險分布（市值權重）</div>
        ${sectorBars}
      </div>
    `;
  } else {
    sectorHtml = `<div style="font-size:12px;color:var(--muted);margin-top:16px">暫無板塊曝險資料</div>`;
  }

  const cashRatioPct = fmtSafeNumber(re.cash_ratio, { percent: true, decimals: 1 });
  const portfolioValue = fmtNTD(re.portfolio_value);
  const deployedCapital = cp.deployed_capital;
  const totalCapital = cp.total_capital;
  const exposureRatio = isFiniteNumber(deployedCapital) && isFiniteNumber(totalCapital) && totalCapital > 0
    ? fmtSafeNumber(deployedCapital / totalCapital, { percent: true, decimals: 1 })
    : null;

  el.innerHTML = `
    <div class="panel" style="text-align:center">
      <div class="kpi-label">VaR 95%</div>
      <div class="kpi-value" style="color:var(--color-danger)">${(insufficient || !re.var_available) ? '觀察期中' : fmtSafeNumber(re.var_95, { percent: true, decimals: 1 })}</div>
      <div class="kpi-hint">${re.var_available ? '日頻 · 95% 信賴水準' : '需 ' + 252 + ' 個交易日觀察 · ' + (re.data_points || 0) + '/252'}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">VaR 99%</div>
      <div class="kpi-value" style="color:var(--color-danger)">${(insufficient || !re.var_available) ? '觀察期中' : fmtSafeNumber(re.var_99, { percent: true, decimals: 1 })}</div>
      <div class="kpi-hint">日頻 · 極端事件壓力</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">CVaR 95%</div>
      <div class="kpi-value" style="color:var(--color-danger)">${(insufficient || !re.var_available) ? '觀察期中' : fmtSafeNumber(re.cvar_95, { percent: true, decimals: 1 })}</div>
      <div class="kpi-hint">95% 條件期望虧損</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">最大回撤</div>
      <div class="kpi-value" style="color:var(--warn)">${insufficient ? '資料不足' : fmtSafeDrawdown(re.max_drawdown_pct, { asAbsolute: true })}</div>
      <div class="kpi-hint">跨場次獨立模擬之歷史峰值回撤分布</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">Rolling Sharpe</div>
      <div class="kpi-value" style="color:${rollingSharpeColor}">${rollingSharpe}</div>
      <div class="kpi-hint">${isFiniteNumber(rollingSharpeRaw) ? '風險調整後收益' : '尚無資金階段資料'}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">投組淨值</div>
      <div class="kpi-value">${portfolioValue}</div>
      <div class="kpi-hint">${cashRatioPct !== null && cashRatioPct !== '—' ? '現金 ' + cashRatioPct : ''}${exposureRatio !== null ? ' · 曝險 ' + exposureRatio : ''}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">資金階段</div>
      <div class="kpi-value" style="font-size:16px">${phase}</div>
      <div class="kpi-hint">${daysInPhase > 0 ? '持續 ' + fmtInt(daysInPhase) + ' 天' : ''}${consecLosses > 0 ? ' · 連續虧損 ' + fmtInt(consecLosses) + ' 次' : ''}${canAdvance ? ' · 可推進' : ''}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">持倉數</div>
      <div class="kpi-value">${fmtInt(re.position_count)}</div>
      <div class="kpi-hint">${typeof re.data_points === 'number' && re.data_points >= 30 ? '資料點 ' + fmtInt(re.data_points) + ' · 可信' : '資料點 ' + fmtInt(re.data_points) + ' · 統計不足'}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">保留現金</div>
      <div class="kpi-value">${fmtNTD(cp.reserve_cash)}</div>
      <div class="kpi-hint">總資本 ${fmtNTD(totalCapital)}</div>
    </div>
  `;

  if (posEl) posEl.innerHTML = `
    <div style="border-top:1px solid var(--border);padding-top:12px">
      <div style="font-size:13px;font-weight:700;margin-bottom:6px">倉位集中度分析</div>
      ${concentrationHtml}
    </div>
  `;

  if (sectorEl) sectorEl.innerHTML = sectorHtml;
}

export function renderRiskCalibration(data) {
  var el = document.getElementById('riskCalibration');
  var panel = document.getElementById('riskCalibrationPanel');
  if (!el || !panel) return;

  if (!data || data.status === 'not_available' || !data.report) {
    panel.style.display = '';
    el.classList.remove('loading');
    el.innerHTML = '<div class="empty">尚無校準報告</div>';
    return;
  }

  panel.style.display = '';
  el.classList.remove('loading');

  var report = data.report;
  var generated = data.generated || '';
  var isCalibrated = report.verdict === 'calibrated';
  var hasChanges = report.changes && report.changes.length > 0;
  var statusIcon = isCalibrated && hasChanges ? '🔵' : (isCalibrated ? '⚪' : '⚪');
  var statusLabel = isCalibrated && hasChanges ? '已校準（本次有調整）'
    : (isCalibrated ? '已校準（本次無調整）' : '未校準 — 等待首次校準完成');
  var statusColor = isCalibrated && hasChanges ? 'var(--color-info)' : 'var(--muted)';

  var changesHtml = '';
  if (report.changes && report.changes.length > 0) {
    var rows = report.changes.map(function(c) {
      var confidenceColor = c.confidence === 'high' ? 'var(--up)' : (c.confidence === 'medium' ? 'var(--warn)' : 'var(--muted)');
      return '<tr>' +
        '<td style="padding:4px 8px;font-size:12px;font-family:monospace">' + escapeHtml(c.name) + '</td>' +
        '<td style="padding:4px 8px;font-size:12px;text-align:right">' + fmtSafeNumber(c.before, { decimals: 4 }) + '</td>' +
        '<td style="padding:4px 8px;font-size:12px;text-align:right;color:var(--up)">' + fmtSafeNumber(c.after, { decimals: 4 }) + '</td>' +
        '<td style="padding:4px 8px;font-size:12px;color:var(--muted)">' + escapeHtml(c.rationale) + '</td>' +
        '<td style="padding:4px 8px;font-size:12px;text-align:center"><span style="padding:1px 6px;border-radius:3px;font-size:11px;background:color-mix(in srgb, ' + confidenceColor + ' 13%, transparent);color:' + confidenceColor + '">' + escapeHtml(c.confidence) + '</span></td>' +
        '</tr>';
    }).join('');
    changesHtml =
      '<div style="margin-top:12px">' +
      '<div style="font-size:13px;font-weight:700;margin-bottom:6px">參數調整</div>' +
      '<table style="width:100%;font-size:12px;border-collapse:collapse">' +
      '<thead><tr style="border-bottom:1px solid var(--border)">' +
      '<th style="text-align:left;padding:4px 8px">參數</th>' +
      '<th style="text-align:right;padding:4px 8px">調整前</th>' +
      '<th style="text-align:right;padding:4px 8px">調整後</th>' +
      '<th style="text-align:left;padding:4px 8px">原因</th>' +
      '<th style="text-align:center;padding:4px 8px">信心</th>' +
      '</tr></thead><tbody>' + rows + '</tbody></table></div>';
  }

  el.innerHTML =
    '<div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap">' +
    '<span style="font-size:28px">' + statusIcon + '</span>' +
    '<div>' +
    '<div style="font-size:14px;font-weight:700;color:' + statusColor + '">' + statusLabel + '</div>' +
    '<div style="font-size:11px;color:var(--muted)">' + (generated ? '校準時間 ' + new Date(generated).toLocaleString('zh-TW') : '') + '</div>' +
    '</div>' +
    '<div style="margin-left:auto;text-align:right;font-size:12px;color:var(--muted)">' +
    '<div>評估 ' + fmtInt(report.orders_evaluated) + ' 筆訂單</div>' +
    '<div>區間 ' + (report.session_span || '—') + '</div>' +
    '</div>' +
    '</div>' +
    '<div style="margin-top:10px;font-size:13px;padding:8px 12px;background:var(--panel-l2);border-radius:6px;color:var(--text);line-height:1.5">' +
    escapeHtml(report.summary || '') +
    '</div>' +
    changesHtml;
}

export async function renderRiskCommentary(containerOrData) {
  var el = typeof containerOrData === 'string'
    ? document.getElementById(containerOrData)
    : (containerOrData || document.getElementById('liveRiskCommentary'));
  if (!el) return;
  el.classList.remove('loading');
  var panel = document.getElementById('liveRiskCommentaryPanel');
  if (panel) panel.style.display = '';

  var data = null;
  try {
    var resp = await fetch('/api/risk/commentary');
    if (!resp.ok) {
      el.innerHTML = '<div class="empty">風險評語取得失敗 (' + resp.status + ')</div>';
      return;
    }
    data = await resp.json();
  } catch (err) {
    console.error('[renderRiskCommentary] fetch failed', err);
    el.innerHTML = '<div class="empty">風險評語連線錯誤</div>';
    return;
  }

  if (!data || data.generated === false) {
    el.innerHTML = '<div class="empty">尚無風控長評語（等待下一次決策）</div>';
    return;
  }

  var verdict = data.verdict || 'UNKNOWN';
  var verdictLabel = {
    ALLOW: '✅ 放行',
    REDUCE: '🔻 縮減',
    BLOCK: '🛑 阻擋',
    HALT: '⛔ 暫停',
    ALERT_ONLY: '⚠️ 警報'
  }[verdict] || verdict;
  var verdictColor = verdict === 'ALLOW' ? 'var(--color-success)'
    : verdict === 'BLOCK' || verdict === 'HALT' || verdict === 'REDUCE' ? 'var(--color-danger)'
    : verdict === 'ALERT_ONLY' ? 'var(--color-warning)' : 'var(--muted)';

  var phase = escapeHtml(data.phase || '—');
  var mode = escapeHtml(data.mode || '—');
  var symbol = escapeHtml(data.symbol || '—');
  var reason = escapeHtml(data.reason || '—');
  var actionType = escapeHtml(data.action_type || '—');
  var actionDesc = escapeHtml(data.action_description || '—');
  var recordedAt = data.recorded_at
    ? new Date(data.recorded_at).toLocaleString('zh-TW')
    : '—';
  var commentary = data.confidence_commentary
    ? escapeHtml(data.confidence_commentary)
    : '<span style="color:var(--muted)">（LLM 尚未產生信心評語）</span>';

  el.innerHTML =
    '<div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap;margin-bottom:10px">' +
      '<span style="padding:2px 10px;border-radius:4px;font-size:12px;font-weight:700;background:color-mix(in srgb, ' + verdictColor + ' 15%, transparent);color:' + verdictColor + '">' + verdictLabel + '</span>' +
      '<span style="font-size:13px;color:var(--muted)">' + phase + ' · ' + mode + ' · ' + symbol + '</span>' +
      '<span style="margin-left:auto;font-size:11px;color:var(--muted)">' + recordedAt + '</span>' +
    '</div>' +
    '<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:8px;margin-bottom:12px">' +
      '<div class="metric"><div class="label">原因</div><div class="value" style="font-size:13px;font-weight:400">' + reason + '</div></div>' +
      '<div class="metric"><div class="label">行動類型</div><div class="value" style="font-size:13px;font-weight:400">' + actionType + '</div></div>' +
      '<div class="metric"><div class="label">行動說明</div><div class="value" style="font-size:13px;font-weight:400">' + actionDesc + '</div></div>' +
    '</div>' +
    '<details open>' +
      '<summary style="font-size:13px;font-weight:700;cursor:pointer;padding:6px 0;color:var(--text)">🧠 LLM 信心評語（Confidence Commentary）</summary>' +
      '<div style="margin-top:6px;padding:10px 14px;background:var(--panel-l2);border-left:3px solid ' + verdictColor + ';border-radius:4px;font-size:13px;line-height:1.6;color:var(--text)">' + commentary + '</div>' +
    '</details>';
}

export function inferSectorFromAgent(agentID, layer) {
  const agentSectorMap = {
    'semiconductor': 'semiconductor',
    'ai_supply_chain': 'ai_supply_chain',
    'financials': 'financials',
    'shipping': 'shipping',
    'value_yield': 'high_dividend',
    'etf_rotation': 'etf_rotation',
    'technical_breakout': 'small_cap',
    'growth_momentum': 'small_cap',
    'macro': 'TAIEX',
    'cro': 'control',
    'cio': 'control'
  };
  return agentSectorMap[agentID] || (layer === 'sector' ? agentID : null);
}

// --- Semiconductor sentiment & market breadth panel (admin_web live page) ---
export function renderSemiconductorSentiment(snapshot, industryCycle) {
  const el = document.getElementById('semiconductorSentiment');
  if (!el) return;
  el.classList.remove('loading');

  const sox = snapshot && snapshot.sox_index ? snapshot.sox_index : null;
  const soxChangeRaw = sox ? sox.change_pct : null;
  const soxValue = fmtSafeNumber(sox && sox.value, { decimals: 2, useGrouping: true });
  const soxChange = fmtSafeSignedPct(soxChangeRaw, 2);
  const soxColor = isFiniteNumber(soxChangeRaw)
    ? (soxChangeRaw > 0 ? 'var(--up)' : (soxChangeRaw < 0 ? 'var(--down)' : 'var(--muted)'))
    : 'var(--muted)';

  const cycle = industryCycle || {};
  const phaseMap = {
    expansion: { label: '擴張', color: 'var(--up)' },
    recovery: { label: '復甦', color: 'var(--color-success)' },
    mature: { label: '成熟', color: 'var(--warn)' },
    downturn: { label: '衰退', color: 'var(--down)' }
  };
  const phase = phaseMap[cycle.business_cycle] || { label: cycle.business_cycle || '—', color: 'var(--muted)' };

  const rows = [
    { label: '庫存週期', value: cycle.inventory_cycle || '—' },
    { label: '資本支出週期', value: cycle.capex_cycle || '—' },
    { label: '信心指數', value: fmtSafeNumber(cycle.confidence, { decimals: 2, useGrouping: true }) },
    { label: '趨勢', value: cycle.trend || '—' }
  ];

  el.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:12px;margin-bottom:12px">
      <div class="panel" style="text-align:center">
        <div class="kpi-label">SOX 費城半導體</div>
        <div class="kpi-value" style="color:${soxColor};font-size:20px">${soxValue}</div>
        <div class="kpi-hint" style="color:${soxColor}">${soxChange}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">半導體景氣週期</div>
        <div class="kpi-value" style="color:${phase.color};font-size:20px">${phase.label}</div>
        <div class="kpi-hint">business_cycle</div>
      </div>
    </div>
    <table style="width:100%;font-size:12px;border-collapse:collapse">
      <thead><tr style="border-bottom:1px solid var(--border)">
        <th style="text-align:left;padding:4px 8px">指標</th>
        <th style="text-align:right;padding:4px 8px">數值</th>
      </tr></thead>
      <tbody>
        ${rows.map(r => `<tr><td style="padding:4px 8px">${r.label}</td><td style="padding:4px 8px;text-align:right;font-family:monospace">${escapeHtml(String(r.value))}</td></tr>`).join('')}
      </tbody>
    </table>
  `;
}

// --- Drawdown stress-test panel (admin_web live page) ---
export function renderDrawdownPanel(data) {
  const el = document.getElementById('drawdownPanel');
  if (!el) return;
  el.classList.remove('loading');

  if (!data || data.status === 'not_available') {
    el.innerHTML = renderEmptyState('尚無回撤模擬資料', '等待回測完成後將自動產生');
    return;
  }

  const maxDD = data.max_drawdown != null ? data.max_drawdown : data.maxDrawdown;
  const var95 = data.var_95 != null ? data.var_95 : data.var95;
  const worstPath = Array.isArray(data.worst_path) ? data.worst_path : [];

  let pathHtml = '';
  if (worstPath.length > 1) {
    const minV = Math.min(...worstPath);
    const maxV = Math.max(...worstPath);
    const range = maxV - minV || 1;
    const width = 300;
    const height = 60;
    const points = worstPath.map((v, i) => {
      const x = (i / (worstPath.length - 1)) * width;
      const y = height - ((v - minV) / range) * height;
      return `${x},${y}`;
    }).join(' ');
    pathHtml = `
      <div style="margin-top:10px">
        <div style="font-size:12px;color:var(--muted);margin-bottom:4px">Worst Path（累積報酬）</div>
        <svg viewBox="0 0 ${width} ${height}" style="width:100%;height:${height}px;background:var(--panel-l2);border-radius:4px">
          <polyline points="${points}" fill="none" stroke="var(--down)" stroke-width="2"/>
        </svg>
      </div>
    `;
  }

  el.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:12px">
      <div class="panel" style="text-align:center">
        <div class="kpi-label">模擬最大回撤</div>
        <div class="kpi-value" style="color:var(--down);font-size:20px">${fmtSafeDrawdown(maxDD, { asAbsolute: true })}</div>
        <div class="kpi-hint">Monte Carlo · 1000 條壓力路徑之最差值</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">模擬 VaR 95</div>
        <div class="kpi-value" style="color:var(--color-danger);font-size:20px">${fmtSafeNumber(var95, { percent: true, decimals: 1 })}</div>
        <div class="kpi-hint">5% 尾端損失</div>
      </div>
    </div>
    ${pathHtml}
    <div style="font-size:11px;color:var(--muted);margin-top:8px;text-align:right">產生時間：${data.generated ? formatDate(data.generated) : '—'}</div>
  `;
}
