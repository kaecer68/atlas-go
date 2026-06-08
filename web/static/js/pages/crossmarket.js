import { escapeHtml } from '../shared/utils.js';

export async function loadCrossMarketData() {
  const [status, usIndices, correlation] = await Promise.all([
    fetch('/api/cross-market/status').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/us-indices').then(r => r.json()).catch(() => null),
    fetch('/api/cross-market/correlation').then(r => r.json()).catch(() => null),
  ]);
  renderUSIndices(status);
  renderTechStocks(status);
  renderMacro(status);
  renderCorrelation(correlation, status);
  renderCrisis(status);
}

function kpiCard(label, value, fmt, color, borderColor, symbol) {
  const c = color || 'var(--text)';
  const bc = borderColor || 'transparent';
  const borderStyle = bc !== 'transparent' ? `border-left:3px solid ${bc};` : '';
  const display = fmt ? fmt(value) : (value != null ? String(value) : '—');
  const symbolHtml = symbol ? `<span style="font-size:11px;color:var(--muted);margin-left:6px">${escapeHtml(symbol)}</span>` : '';
  return `<div class="kpi-card" style="${borderStyle}"><div class="kpi-label">${label}${symbolHtml}</div><div class="kpi-value" style="color:${c}">${display}</div></div>`;
}

function fmtPct(v) {
  if (v == null) return '—';
  const n = Number(v);
  if (isNaN(n)) return '—';
  return n.toFixed(2) + '%';
}

function fmtNum(v, digits) {
  if (v == null) return '—';
  const n = Number(v);
  if (isNaN(n)) return '—';
  return n.toFixed(digits || 2);
}

function fmtTs(v) {
  if (!v) return '—';
  return new Date(v).toLocaleString();
}

function getField(status, key) {
  const raw = status[key];
  if (raw && typeof raw === 'object') {
    return { value: raw.value, changePct: raw.change_pct, symbol: raw.symbol, timestamp: raw.timestamp };
  }
  return { value: raw, changePct: raw, symbol: null, timestamp: null };
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
  const spxColor = parseFloat(spx.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const ndxColor = parseFloat(ndx.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const djiColor = parseFloat(dji.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const soxColor = parseFloat(sox.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  el.innerHTML =
    kpiCard('S&P 500', spx.value, fmtNum, spxColor, null, spx.symbol) +
    kpiCard('Nasdaq', ndx.value, fmtNum, ndxColor, null, ndx.symbol) +
    kpiCard('Dow Jones', dji.value, fmtNum, djiColor, null, dji.symbol) +
    kpiCard('SOX 半導體', sox.value, fmtNum, soxColor, null, sox.symbol);
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
  const nvdaColor = parseFloat(nvda.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const aaplColor = parseFloat(aapl.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const msftColor = parseFloat(msft.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const tsmColor = parseFloat(tsm.changePct) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  el.innerHTML =
    kpiCard('NVDA', nvda.value, fmtNum, nvdaColor, null, nvda.symbol) +
    kpiCard('AAPL', aapl.value, fmtNum, aaplColor, null, aapl.symbol) +
    kpiCard('MSFT', msft.value, fmtNum, msftColor, null, msft.symbol) +
    kpiCard('TSM ADR', tsm.value, fmtNum, tsmColor, 'rgba(79,193,255,0.3)', tsm.symbol);
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
    kpiCard('VIX 恐慌指數', vix.value, fmtNum, null, null, vix.symbol) +
    kpiCard('DXY 美元指數', dxy.value, fmtNum, null, null, dxy.symbol) +
    kpiCard('USD/TWD 匯率', usdTwd.value, fmtNum, null, null, usdTwd.symbol) +
    kpiCard('US 10Y 殖利率', us10y.value, fmtPct, null, null, us10y.symbol);
}

function renderCorrelation(correlation, status) {
  const el = document.getElementById('cm-correlation');
  if (!el) return;
  const rho = correlation && correlation.correlation != null ? correlation.correlation : (status && status.correlation_spx_twse != null ? status.correlation_spx_twse : null);
  const fallback = correlation && correlation.is_fallback;
  const observations = correlation && correlation.observations != null ? correlation.observations : '—';
  const windowSize = correlation && correlation.window_size != null ? correlation.window_size : '—';
  const generatedAt = correlation && correlation.generated_at ? new Date(correlation.generated_at).toLocaleString() : (status && status.generated_at ? new Date(status.generated_at).toLocaleString() : '—');

  let rhoColor = 'var(--text)';
  let rhoLabel = '—';
  if (rho != null && !isNaN(rho)) {
    rhoLabel = rho.toFixed(4);
    if (rho >= 0.8) rhoColor = 'var(--color-danger)';
    else if (rho >= 0.6) rhoColor = 'var(--color-warning)';
    else if (rho >= 0.3) rhoColor = 'var(--muted)';
    else rhoColor = 'var(--color-success)';
  }
  if (fallback) rhoColor = 'var(--muted)';

  el.innerHTML = `<div class="table-wrapper"><table>
    <thead><tr><th>指標</th><th>數值</th></tr></thead>
    <tbody>
      <tr><td>SPX-TWSE 動態相關性 ρ</td><td style="color:${rhoColor};font-weight:700;font-family:var(--font-mono)">${rhoLabel}${fallback ? ' <span style="font-size:11px;color:var(--muted)">(fallback)</span>' : ''}</td></tr>
      <tr><td>觀測筆數</td><td style="font-family:var(--font-mono)">${observations}</td></tr>
      <tr><td>滾動視窗</td><td style="font-family:var(--font-mono)">${windowSize} 日</td></tr>
      <tr><td>資料時間</td><td style="font-size:13px;color:var(--muted)">${escapeHtml(generatedAt)}</td></tr>
    </tbody>
  </table></div>`;
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
  const vixLabel = vix != null ? vix.toFixed(2) : '—';

  let bg, icon, title, desc;
  if (active) {
    bg = 'rgba(239,68,68,0.12)';
    icon = '🔴';
    title = '危機模式 — 啟動中';
    desc = `VIX 指數 ${vixLabel} ≥ 35，系統已觸發危機保護：電路熔斷器強制開啟、最佳化器協方差矩陣對角膨脹 1.5x、最大持倉比例減半。`;
  } else {
    bg = 'rgba(34,197,94,0.08)';
    icon = '🟢';
    title = '正常模式';
    desc = `VIX 指數 ${vixLabel} < 35，系統以標準參數運作。`;
  }

  el.innerHTML = `<div style="padding:16px 20px;background:${bg};border-radius:8px;border:1px solid ${active ? 'rgba(239,68,68,0.3)' : 'rgba(34,197,94,0.15)'}">
    <div style="display:flex;align-items:center;gap:var(--space-sm);margin-bottom:8px">
      <span style="font-size:24px">${icon}</span>
      <span style="font-weight:700;font-size:16px;color:${active ? 'var(--color-danger)' : 'var(--color-success)'}">${title}</span>
    </div>
    <div style="font-size:14px;color:var(--text);line-height:1.6">${desc}</div>
  </div>`;
}

function emptyState(msg) {
  return '<div class="empty" style="padding:24px;text-align:center;background:var(--panel-l2);border-radius:8px">' +
    '<div style="font-size:32px;margin-bottom:8px">📡</div>' +
    '<div style="color:var(--text);font-weight:600;margin-bottom:4px">' + escapeHtml(msg || '等待資料') + '</div>' +
    '<div style="color:var(--muted);font-size:13px">美台連動監控資料每 5 分鐘自動更新一次。</div>' +
    '</div>';
}

window.loadCrossMarketData = loadCrossMarketData;
