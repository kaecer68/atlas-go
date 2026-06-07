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

function kpiCard(label, value, fmt, color, borderColor) {
  const c = color || 'var(--text)';
  const bc = borderColor || 'transparent';
  const borderStyle = bc !== 'transparent' ? `border-left:3px solid ${bc};` : '';
  const display = fmt ? fmt(value) : (value != null ? String(value) : '—');
  return `<div class="kpi-card" style="${borderStyle}"><div class="kpi-label">${label}</div><div class="kpi-value" style="color:${c}">${display}</div></div>`;
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

function renderUSIndices(status) {
  const el = document.getElementById('cm-us-indices');
  if (!el) return;
  if (!status || status.error) {
    el.innerHTML = emptyState('等待美台連動監控資料');
    return;
  }
  const spxColor = parseFloat(status.spx) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const ndxColor = parseFloat(status.ndx) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const djiColor = parseFloat(status.dji) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const soxColor = parseFloat(status.sox) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  el.innerHTML =
    kpiCard('S&P 500', status.spx, fmtNum, spxColor) +
    kpiCard('Nasdaq', status.ndx, fmtNum, ndxColor) +
    kpiCard('Dow Jones', status.dji, fmtNum, djiColor) +
    kpiCard('SOX 半導體', status.sox, fmtNum, soxColor);
}

function renderTechStocks(status) {
  const el = document.getElementById('cm-tech-stocks');
  if (!el) return;
  if (!status) {
    el.innerHTML = emptyState('等待科技股資料');
    return;
  }
  const nvdaColor = parseFloat(status.nvda) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const aaplColor = parseFloat(status.aapl) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const msftColor = parseFloat(status.msft) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  const tsmColor = parseFloat(status.tsm_adr) >= 0 ? 'var(--color-success)' : 'var(--color-danger)';
  el.innerHTML =
    kpiCard('NVDA', status.nvda, fmtNum, nvdaColor) +
    kpiCard('AAPL', status.aapl, fmtNum, aaplColor) +
    kpiCard('MSFT', status.msft, fmtNum, msftColor) +
    kpiCard('TSM ADR', status.tsm_adr, fmtNum, tsmColor, 'rgba(79,193,255,0.3)');
}

function renderMacro(status) {
  const el = document.getElementById('cm-macro');
  if (!el) return;
  if (!status) {
    el.innerHTML = emptyState('等待宏觀指標');
    return;
  }
  el.innerHTML =
    kpiCard('VIX 恐慌指數', status.vix, fmtNum) +
    kpiCard('DXY 美元指數', status.dxy, fmtNum) +
    kpiCard('USD/TWD 匯率', status.usd_twd, fmtNum) +
    kpiCard('US 10Y 殖利率', status.us10y, fmtPct);
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
  const vix = status.vix != null ? parseFloat(status.vix) : null;
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
