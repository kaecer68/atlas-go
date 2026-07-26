import { escapeHtml } from '../shared/utils.js';
import { fmtSafeNumber, fmtSafePct } from '../shared/format-metric.js';

export async function loadCrossMarketData() {
  const [status, usIndices, correlation, correlationMatrix] = await Promise.all([
    fetch('/api/cross-market/status').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/us-indices').then(r => r.json()).catch(() => null),
    fetch('/api/cross-market/correlation').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/correlation-matrix').then(r => r.json()).catch(() => null),
  ]);
  renderStaleBanner(status);
  renderDegradedBanner(status);
  renderUSIndices(status);
  renderTechStocks(status);
  renderMacro(status);
  renderCorrelation(correlation, status);
  renderCorrelationMatrix(correlationMatrix);
  renderCrisis(status);
}

function kpiCard(label, value, fmt, color, borderColor, symbol, helpKey, failed) {
  const c = color || 'var(--text)';
  const bc = borderColor || 'transparent';
  const borderStyle = bc !== 'transparent' ? `border-left:3px solid ${bc};` : '';
  const symbolHtml = symbol ? `<span style="font-size:var(--text-sm);color:var(--muted);margin-left:6px">${escapeHtml(symbol)}</span>` : '';
  const clickable = helpKey ? ` clickable" onclick="openKpiHelp('${helpKey}')` : '';
  let display;
  if (failed) {
    display = `<span class="cm-data-failed">資料獲取失敗</span>`;
  } else {
    display = fmt ? fmt(value) : (value != null ? String(value) : '—');
  }
  return `<div class="kpi-card${clickable}" style="${borderStyle}"><div class="kpi-label">${label}${symbolHtml}</div><div class="kpi-value" style="color:${c}">${display}</div></div>`;
}

function fmtRate(v) {
  return fmtSafePct(v, 2);
}

function fmtPrice(v) {
  return fmtSafeNumber(v, { decimals: 2, useGrouping: true });
}

function fmtTs(v) {
  if (!v) return '—';
  return new Date(v).toLocaleString();
}

function changeColor(pct) {
  const n = Number(pct);
  if (!Number.isFinite(n)) return null;
  return n >= 0 ? 'var(--bullish)' : 'var(--bearish)';
}

function getField(status, key) {
  const raw = status[key];
  if (raw && typeof raw === 'object') {
    // A field is "failed" when the symbol is empty or the value is ≤ 0
    // (Layer 4 of the data-visibility safeguard — surface channel failures
    // and garbage/zero data instead of silently rendering 0).
    const failed = !raw.symbol || raw.symbol === '' || (raw.value != null && Number(raw.value) <= 0);
    return {
      value: raw.value,
      changePct: raw.change_pct,
      symbol: raw.symbol,
      timestamp: raw.timestamp,
      failed,
    };
  }
  return { value: raw, changePct: raw, symbol: null, timestamp: null, failed: true };
}

function renderUSIndices(status) {
  const el = document.getElementById('cm-us-indices');
  if (!el) return;
  if (!status || status.error) {
    el.innerHTML = emptyState('等待美台連動監控資料');
    return;
  }
  const spx = getField(status, 'spx');
  const ndx = getField(status, 'ndx');
  const dji = getField(status, 'dji');
  const sox = getField(status, 'sox');
  const spxColor = spx.failed ? null : changeColor(spx.changePct);
  const ndxColor = ndx.failed ? null : changeColor(ndx.changePct);
  const djiColor = dji.failed ? null : changeColor(dji.changePct);
  const soxColor = sox.failed ? null : changeColor(sox.changePct);
  el.innerHTML =
    kpiCard('S&P 500', spx.value, fmtPrice, spxColor, null, spx.symbol, 'cm_spx', spx.failed) +
    kpiCard('Nasdaq', ndx.value, fmtPrice, ndxColor, null, ndx.symbol, 'cm_ndx', ndx.failed) +
    kpiCard('Dow Jones', dji.value, fmtPrice, djiColor, null, dji.symbol, 'cm_dji', dji.failed) +
    kpiCard('SOX 半導體', sox.value, fmtPrice, soxColor, null, sox.symbol, 'cm_sox', sox.failed);
}

function renderTechStocks(status) {
  const el = document.getElementById('cm-tech-stocks');
  if (!el) return;
  if (!status) {
    el.innerHTML = emptyState('等待科技股資料');
    return;
  }
  const nvda = getField(status, 'nvda');
  const aapl = getField(status, 'aapl');
  const msft = getField(status, 'msft');
  const tsm = getField(status, 'tsm_adr');
  const nvdaColor = nvda.failed ? null : changeColor(nvda.changePct);
  const aaplColor = aapl.failed ? null : changeColor(aapl.changePct);
  const msftColor = msft.failed ? null : changeColor(msft.changePct);
  const tsmColor = tsm.failed ? null : changeColor(tsm.changePct);
  el.innerHTML =
    kpiCard('NVDA', nvda.value, fmtPrice, nvdaColor, null, nvda.symbol, 'cm_nvda', nvda.failed) +
    kpiCard('AAPL', aapl.value, fmtPrice, aaplColor, null, aapl.symbol, 'cm_aapl', aapl.failed) +
    kpiCard('MSFT', msft.value, fmtPrice, msftColor, null, msft.symbol, 'cm_msft', msft.failed) +
    kpiCard('TSM ADR', tsm.value, fmtPrice, tsmColor, 'color-mix(in srgb, var(--accent) 30%, transparent)', tsm.symbol, 'cm_tsm', tsm.failed);
}

function renderMacro(status) {
  const el = document.getElementById('cm-macro');
  if (!el) return;
  if (!status) {
    el.innerHTML = emptyState('等待宏觀指標');
    return;
  }
  const vix = getField(status, 'vix');
  const dxy = getField(status, 'dxy');
  const usdTwd = getField(status, 'usd_twd');
  const us10y = getField(status, 'us10y');
  el.innerHTML =
    kpiCard('VIX 恐慌指數', vix.value, fmtPrice, null, null, vix.symbol, 'cm_vix', vix.failed) +
    kpiCard('DXY 美元指數', dxy.value, fmtPrice, null, null, dxy.symbol, 'cm_dxy', dxy.failed) +
    kpiCard('USD/TWD 匯率', usdTwd.value, fmtPrice, null, null, usdTwd.symbol, 'cm_usd_twd', usdTwd.failed) +
    kpiCard('US 10Y 殖利率', us10y.value, fmtRate, null, null, us10y.symbol, 'cm_us10y', us10y.failed);
}

function renderCorrelation(correlation, status) {
  const el = document.getElementById('cm-correlation');
  if (!el) return;
  const rho = correlation && correlation.correlation != null ? correlation.correlation : (status && status.correlation_spx_twse != null ? status.correlation_spx_twse : null);
  const fallback = correlation && correlation.is_fallback;
  const observations = correlation && correlation.observations != null ? correlation.observations : '—';
  const windowSize = correlation && correlation.window_size != null ? correlation.window_size : '—';
  const generatedAt = correlation && correlation.computed_at ? new Date(correlation.computed_at).toLocaleString() : (status && status.generated_at ? new Date(status.generated_at).toLocaleString() : '—');

  let rhoColor = 'var(--text)';
  let rhoLabel = '—';
  if (rho != null && Number.isFinite(rho)) {
    rhoLabel = fmtSafeNumber(rho, { decimals: 4 });
    if (rho >= 0.8) rhoColor = 'var(--color-danger)';
    else if (rho >= 0.6) rhoColor = 'var(--color-warning)';
    else if (rho >= 0.3) rhoColor = 'var(--muted)';
    else rhoColor = 'var(--color-success)';
  }
  if (fallback) rhoColor = 'var(--muted)';

  el.innerHTML = `<div class="table-wrapper"><table>
    <thead><tr><th>指標</th><th>數值</th><th>說明</th></tr></thead>
    <tbody>
      <tr><td>SPX-TWSE 動態相關性 ρ</td><td style="color:${rhoColor};font-weight:700;font-family:var(--font-mono)">${rhoLabel}${fallback ? ' <span style="font-size:var(--text-sm);color:var(--muted)">(fallback)</span>' : ''}</td><td>ρ 範圍 −1 到 +1：≥ 0.7 強正相關（美股跌台股跟跌）、0.3–0.7 中度相關、≤ 0.3 弱相關。當 ρ &gt; 0.8 系統視為「傳導放大」會自動降倉。</td></tr>
      <tr><td>觀測筆數</td><td style="font-family:var(--font-mono)">${observations}</td><td>計算 ρ 使用的歷史交易日數。少於 30 筆時 ρ 採 fallback（預設 0.5），不宜作為交易依據。</td></tr>
      <tr><td>滾動視窗</td><td style="font-family:var(--font-mono)">${windowSize} 日</td><td>計算相關性時回看的歷史天數（pearson correlation 的 window size）。視窗越長反應越平滑、越短反應越即時但雜訊多。</td></tr>
      <tr><td>資料時間</td><td style="font-size:var(--text-base);color:var(--muted)">${escapeHtml(generatedAt)}</td><td>此 ρ 值的計算時間（後端排程每 ${windowSize} 分鐘更新一次）。若超過 1 小時未更新請檢查 correlation 排程狀態。</td></tr>
    </tbody>
  </table></div>`;
}

function renderCorrelationMatrix(matrixData) {
  const el = document.getElementById('cm-correlation-matrix');
  if (!el) return;
  if (!matrixData || !matrixData.symbols || !Array.isArray(matrixData.matrix) || matrixData.matrix.length === 0) {
    el.innerHTML = emptyState('等待產業相關性矩陣資料');
    return;
  }
  const symbols = matrixData.symbols;
  const labels = matrixData.labels || symbols;
  const matrix = matrixData.matrix;
  const n = symbols.length;
  const labelStyle = 'font-family:var(--font-base);font-size:var(--text-sm);font-weight:600;letter-spacing:0.02em';

  function corrColor(v) {
    if (v == null || isNaN(v)) return 'var(--panel-l2)';
    const abs = Math.abs(v);
    if (abs >= 0.7) return v >= 0 ? 'var(--color-danger)' : 'var(--color-success)';
    if (abs >= 0.3) return v >= 0 ? 'color-mix(in srgb, var(--color-warning) 60%, transparent)' : 'color-mix(in srgb, var(--color-success) 40%, transparent)';
    return 'var(--panel-l2)';
  }

  function corrText(v) {
    return fmtSafeNumber(v, { decimals: 2, useGrouping: true });
  }

  let html = '<div style="overflow-x:auto"><table style="font-size:var(--text-sm);border-collapse:collapse;min-width:100%"><thead><tr><th style="position:sticky;left:0;background:var(--panel-l1);padding:6px 8px;text-align:left;border-bottom:1px solid var(--panel-l3);' + labelStyle + '">產業</th>';
  for (let i = 0; i < n; i++) {
    html += '<th style="padding:6px 8px;text-align:center;border-bottom:1px solid var(--panel-l3);white-space:nowrap;writing-mode:vertical-rl;transform:rotate(180deg);min-width:32px;' + labelStyle + '">' + escapeHtml(labels[i] || symbols[i]) + '</th>';
  }
  html += '</tr></thead><tbody>';
  for (let i = 0; i < n; i++) {
    html += '<tr><td style="position:sticky;left:0;background:var(--panel-l1);padding:6px 8px;border-bottom:1px solid var(--panel-l3);white-space:nowrap;' + labelStyle + '">' + escapeHtml(labels[i] || symbols[i]) + '</td>';
    for (let j = 0; j < n; j++) {
      const v = matrix[i] ? matrix[i][j] : null;
      const bg = corrColor(v);
      const txt = corrText(v);
      const isDiagonal = i === j;
      const style = 'padding:4px 6px;text-align:center;font-family:var(--font-mono);font-size:var(--text-xs);background:' + bg + ';color:' + (isDiagonal || Math.abs(v || 0) >= 0.7 ? '#fff' : 'var(--text)') + ';border-bottom:1px solid var(--panel-l3)';
      html += '<td style="' + style + '">' + txt + '</td>';
    }
    html += '</tr>';
  }
  html += '</tbody></table></div>';
  html += '<div style="margin-top:var(--space-sm);font-size:var(--text-sm);color:var(--text)">色階：|ρ| ≥ 0.7 深色（強相關）、0.3–0.7 淡色（中等相關）、< 0.3 灰色（弱相關）。正相關偏紅，負相關偏綠。</div>';
  el.innerHTML = html;
}

function renderCrisis(status) {
  const el = document.getElementById('cm-crisis');
  if (!el) return;
  if (!status) {
    el.innerHTML = emptyState('等待危機信號');
    return;
  }
  const active = status.crisis_active;
  const vixField = getField(status, 'vix');
  const vix = vixField.value != null ? parseFloat(vixField.value) : null;
  const vixLabel = Number.isFinite(vix) ? fmtPrice(vix) : '—';

  let bg, icon, title, desc;
  if (active) {
    bg = 'color-mix(in srgb, var(--color-danger) 12%, transparent)';
    icon = '🔴';
    title = '危機模式 — 啟動中';
    desc = `VIX 指數 ${vixLabel} ≥ 35，系統已觸發危機保護：電路熔斷器強制開啟、最佳化器協方差矩陣對角膨脹 1.5x、最大持倉比例減半。`;
  } else {
    bg = 'color-mix(in srgb, var(--color-success) 8%, transparent)';
    icon = '🟢';
    title = '正常模式';
    desc = `VIX 指數 ${vixLabel} < 35，系統以標準參數運作。`;
  }

  el.innerHTML = `<div class="kpi-card clickable" onclick="openKpiHelp('cm_crisis')" style="padding:var(--space-md);background:${bg};border-radius:8px;border:1px solid ${active ? 'color-mix(in srgb, var(--color-danger) 30%, transparent)' : 'color-mix(in srgb, var(--color-success) 15%, transparent)'}">
    <div style="display:flex;align-items:center;gap:var(--space-sm);margin-bottom:8px">
      <span style="font-size:var(--text-xl)">${icon}</span>
      <span style="font-weight:var(--font-semibold);font-size:var(--text-base);color:${active ? 'var(--color-danger)' : 'var(--color-success)'}">${title}</span>
    </div>
    <div style="font-size:var(--text-sm);color:var(--text);line-height:1.6">${desc}</div>
    <div style="font-size:var(--text-xs);color:var(--muted);margin-top:8px">點擊查看完整風險分級說明 →</div>
  </div>`;
}

function emptyState(msg) {
  return '<div class="empty" style="padding:24px;text-align:center;background:var(--panel-l2);border-radius:8px">' +
    '<div style="font-size:32px;margin-bottom:8px">📡</div>' +
    '<div style="color:var(--text);font-weight:600;margin-bottom:4px">' + escapeHtml(msg || '等待資料') + '</div>' +
    '<div style="color:var(--muted);font-size:var(--text-base)">美台連動監控資料每 5 分鐘自動更新一次。</div>' +
    '</div>';
}

function renderStaleBanner(status) {
  if (!status || status.data_status !== 'stale') return;

  const stale = status.stale_channels || [];
  const staleList = stale.length > 0
    ? stale.map(ch => `<code style="background:var(--panel-l2);padding:2px 6px;border-radius:3px;margin:0 2px">${escapeHtml(ch)}</code>`).join('')
    : '<em>(部分美國市場通道)</em>';

  const html = `<div class="cm-stale-banner" style="background:color-mix(in srgb, var(--status-stale) 14%, transparent);border:1px solid color-mix(in srgb, var(--status-stale) 30%, transparent);border-radius:8px;padding:var(--space-md);margin-bottom:var(--space-md)">
    <strong style="color:var(--status-stale)">⏱ 部分美國市場資料為快取值</strong>
    <div class="cm-stale-list" style="font-size:var(--text-sm);color:var(--text);margin-top:6px">以下通道回傳快取（電路熔斷器開啟或 fallback），數值仍顯示但可能過時: ${staleList}</div>
    <div class="cm-stale-recovery" style="font-size:var(--text-xs);color:var(--muted);margin-top:6px">系統正在使用 CB 開啟前的最後一筆快取，等待通道恢復後將自動更新為新鮮資料。</div>
  </div>`;

  const firstGrid = document.querySelector('#cm-us-indices');
  if (firstGrid && firstGrid.parentNode) {
    const existing = firstGrid.parentNode.querySelector('.cm-stale-banner');
    if (existing) existing.remove();
    const wrapper = document.createElement('div');
    wrapper.innerHTML = html;
    firstGrid.parentNode.insertBefore(wrapper.firstChild, firstGrid);
  }
}

function renderDegradedBanner(status) {
  if (!status || status.data_status !== 'degraded') return;

  const failed = status.failed_channels || [];
  const failedList = failed.length > 0
    ? failed.map(ch => `<code style="background:var(--panel-l2);padding:2px 6px;border-radius:3px;margin:0 2px">${escapeHtml(ch)}</code>`).join('')
    : '<em>(部分美國市場通道)</em>';

  const html = `<div class="cm-degraded-banner">
    <strong>⚠ 部分美國市場資料獲取失敗</strong>
    <div class="cm-failed-list">以下通道回傳失敗,相關卡片已標示 <span style="color:var(--color-danger);font-weight:600">資料獲取失敗</span>: ${failedList}</div>
    <div class="cm-failed-recovery">系統已記錄 degraded 狀態,後續 fetch 成功將自動恢復。</div>
  </div>`;

  const firstGrid = document.querySelector('#cm-us-indices');
  if (firstGrid && firstGrid.parentNode) {
    const existing = firstGrid.parentNode.querySelector('.cm-degraded-banner');
    if (existing) existing.remove();
    const wrapper = document.createElement('div');
    wrapper.innerHTML = html;
    firstGrid.parentNode.insertBefore(wrapper.firstChild, firstGrid);
  }
}

window.loadCrossMarketData = loadCrossMarketData;
