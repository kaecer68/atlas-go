// pages/risk-console.js — 持倉風控台（SSOT Phase 2, P2-2）
// 風控營運台(/admin/live) + 組合持倉(/admin/portfolio) 合併頁的資料編排與
// 渲染。目標資訊架構 S1–S9（見 .omo/plans/2026-09-04-risk-console-ssot-refactor.md
// Phase 2 表 + v1 重排設計）：
//   S1 今日風控結論（風控長評語橫幅）    S2 資金水位 4 卡
//   S3 淨值趨勢（稅前/稅後雙線）          S4 風險指標（VaR/CVaR/回撤/Sharpe/階段/MC）
//   S5 持倉明細 + 持倉結構（#holdings）   S6 市場背景（預設半折疊）
//   S7 控制層今日處置（funnel + 異常）     S8 交易時間軸（分頁）
//   S9 工程自監控（details 預設收合）
//
// 資料接線：先試 GET /api/dashboard/overview（Phase 1 聚合端點：live_status /
// portfolio_state / risk_exposure 巢狀子物件、欄位名與既有端點一致、snake_case），
// 404 / 缺欄位時逐節 fallback 到既有 live-status / portfolio-state / risk-exposure。
// portfolio_state 精華版不含 equity_curve → 淨值趨勢（S3）固定走
// /api/dashboard/portfolio-state。兩 PR 誰先合併都不會壞。
import { renderRiskCommentary, renderRiskCalibration, renderSemiconductorSentiment } from './risk.js';
import { renderDualEquityCurve } from '../components/sparkline.js';
import { renderStockCell, agentName, sectorName } from '../names.js';
import { escapeHtml, fmtInt, fmtNTD, pnlColor } from '../shared/utils.js';
import { fmtSafeNumber, fmtSafeDrawdown, fmtSafeSignedPct, fmtSafePct } from '../shared/format-metric.js';
import { capitalPhaseLabel, businessCycleLabel } from '../shared/constants.js';
import { renderEmptyState } from '../shared/app-utils.js';

const TRADE_PAGE_SIZE = 10;

function isObj(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v);
}
function isFiniteNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

async function safeGet(getJSON, url, dft) {
  try {
    const d = await getJSON(url);
    return (d === undefined || d === null) ? dft : d;
  } catch (e) {
    return dft;
  }
}

// ── S2 資金水位（4 卡：現金 / 曝險 / 當日損益 / 熔斷狀態）────────────
function renderCapitalCards(ls, degraded) {
  const el = document.getElementById('consoleCapitalCards');
  const note = document.getElementById('consoleDegradedNote');
  if (!el) return;
  if (!ls || !isObj(ls.circuit_breaker)) {
    el.innerHTML = renderEmptyState('資金水位暫無資料', '模擬引擎尚未啟動或 state 檔尚未建立');
    return;
  }
  const cb = ls.circuit_breaker || {};
  const pf = ls.portfolio || {};
  // B7 (risk-console Phase 1)：unknown 顯示「未知」灰燈，不再當「正常」綠燈
  const cbStateMap = {
    normal: { label: '正常', color: 'var(--color-success)' },
    paused: { label: '暫停', color: 'var(--color-warning)' },
    halted: { label: '已觸發', color: 'var(--color-danger)' },
    tripped: { label: '已觸發', color: 'var(--color-danger)' },
    unknown: { label: '未知', color: 'var(--status-unknown)' },
  };
  const cbInfo = cbStateMap[cb.state] || { label: '未知', color: 'var(--status-unknown)' };
  const dayPnl = pf.day_pnl;

  const cashVal = isFiniteNumber(pf.cash) ? fmtNTD(pf.cash) : '—';
  const exposureVal = isFiniteNumber(pf.total_exposure) ? fmtNTD(pf.total_exposure) : '—';
  const dayPnlVal = isFiniteNumber(dayPnl) ? fmtNTD(dayPnl) : '—';
  const pnlCls = isFiniteNumber(dayPnl) ? (dayPnl > 0 ? 'text-up' : (dayPnl < 0 ? 'text-down' : '')) : '';

  el.innerHTML = `
    <div class="kpi-card"><div class="kpi-label">現金</div><div class="kpi-value">${cashVal}</div>
      <div class="kpi-hint">${isFiniteNumber(pf.positions_count) ? '持倉 ' + fmtInt(pf.positions_count) + ' 檔' : '—'}</div></div>
    <div class="kpi-card"><div class="kpi-label">曝險（持倉市值）</div><div class="kpi-value">${exposureVal}</div>
      <div class="kpi-hint">${isFiniteNumber(pf.unrealized_pnl) ? '未實現損益 ' + fmtNTD(pf.unrealized_pnl) : '—'}</div></div>
    <div class="kpi-card"><div class="kpi-label">當日損益</div><div class="kpi-value ${pnlCls}">${dayPnlVal}</div>
      <div class="kpi-hint">${(cb.state_changed_at && cb.state_changed_at !== '0001-01-01T00:00:00Z') ? '熔斷狀態更新 ' + new Date(cb.state_changed_at).toLocaleTimeString('zh-TW') : '日內模擬損益'}</div></div>
    <div class="kpi-card"><div class="kpi-label">熔斷狀態</div><div class="kpi-value" style="color:${cbInfo.color}">${cbInfo.label}</div>
      <div class="kpi-hint">${(cb.cooldown_until && cb.cooldown_until !== '0001-01-01T00:00:00Z' && new Date(cb.cooldown_until) > new Date()) ? '冷卻至 ' + new Date(cb.cooldown_until).toLocaleTimeString('zh-TW') : (cb.consecutive_sl > 0 ? '連續止損 ' + fmtInt(cb.consecutive_sl) + ' 次' : '熔斷機制監控中')}</div></div>
  `;
  if (note) {
    if (degraded) {
      note.hidden = false;
      note.textContent = '⚠ 部分資料源降級（overview.degraded=true，PG 不可用回退 JSONL）';
    } else {
      note.hidden = true;
      note.textContent = '';
    }
  }
}

// ── S3 淨值趨勢（稅前/稅後雙線）──────────────────────────
function renderEquityTrend(equityState) {
  const msg = document.getElementById('consoleEquityEmptyMsg');
  const curve = (equityState && Array.isArray(equityState.equity_curve)) ? equityState.equity_curve : [];
  const hasPoints = curve.some(function (p) { return isFiniteNumber(p.value); });
  if (!hasPoints) {
    const panel = document.getElementById('equityCurvePanel');
    if (panel) panel.style.display = 'none';
    if (msg) msg.hidden = false;
    return;
  }
  if (msg) msg.hidden = true;
  const preTaxPoints = curve.map(function (p) { return { label: p.label, value: p.value }; });
  const afterTaxPoints = curve
    .filter(function (p) { return p.after_tax_value !== undefined && isFiniteNumber(p.after_tax_value); })
    .map(function (p) { return { label: p.label, value: p.after_tax_value }; });
  renderDualEquityCurve(preTaxPoints, afterTaxPoints);
}

// ── S4 風險指標列 ────────────────────────────────────────
function riskCard(label, valueHtml, hint, extraClass) {
  return `<div class="kpi-card console-risk-card ${extraClass || ''}">
    <div class="kpi-label">${label}</div>
    <div class="kpi-value">${valueHtml}</div>
    <div class="kpi-hint">${hint || ''}</div>
  </div>`;
}

function renderRiskRow(re, cp, dd) {
  const el = document.getElementById('consoleRiskCards');
  const pathWrap = document.getElementById('consoleMonteCarloPath');
  if (!el) return;
  const emptyRisk = !re && !cp;
  if (emptyRisk) {
    el.innerHTML = renderEmptyState('風險指標暫無資料', '執行模擬場次後自動產生');
    if (pathWrap) pathWrap.style.display = 'none';
    return;
  }

  const insufficient = !!(re && (re.insufficient_data || (typeof re.data_points === 'number' && re.data_points < 30)));
  const varAvailable = !!(re && re.var_available !== false && !insufficient);
  const varTxt = function (v) {
    return varAvailable && isFiniteNumber(v)
      ? fmtSafeNumber(v, { percent: true, decimals: 1 })
      : '<span style="color:var(--muted)">觀察期中</span>';
  };
  const hintTxt = function () {
    if (!re) return '—';
    if (!varAvailable) return '需 252 個交易日觀察 · ' + fmtInt(re.data_points || 0) + '/252';
    return '日頻 · 95% 信賴水準';
  };

  const cpVal = cp || {};
  const phaseLabel = cpVal.phase ? capitalPhaseLabel(cpVal.phase) : '—';
  const rollingSharpeRaw = cpVal.rolling_sharpe;
  const rollingSharpe = fmtSafeNumber(rollingSharpeRaw, { decimals: 2, useGrouping: true });
  const rollingColor = isFiniteNumber(rollingSharpeRaw)
    ? (rollingSharpeRaw > 0.5 ? 'var(--up)' : (rollingSharpeRaw < 0 ? 'var(--down)' : 'var(--warn)'))
    : 'var(--muted)';

  let cards = '';
  cards += riskCard('VaR 95%', '<span style="color:var(--color-danger)">' + varTxt(re && re.var_95) + '</span>', hintTxt());
  cards += riskCard('VaR 99%', '<span style="color:var(--color-danger)">' + varTxt(re && re.var_99) + '</span>', '日頻 · 極端事件壓力');
  cards += riskCard('CVaR 95%', '<span style="color:var(--color-danger)">' + varTxt(re && re.cvar_95) + '</span>', '95% 條件期望虧損');
  cards += riskCard('最大回撤',
    re ? `<span style="color:var(--warn)">${insufficient ? '資料不足' : fmtSafeDrawdown(re.max_drawdown_pct, { asAbsolute: true })}</span>` : '—',
    '口徑：全期 · 跨場次獨立模擬峰值回撤');
  cards += riskCard('Rolling Sharpe', `<span style="color:${rollingColor}">${rollingSharpe}</span>`,
    isFiniteNumber(rollingSharpeRaw) ? '口徑：近 30 場次' : '尚無資金階段資料');
  cards += riskCard('資金階段', `<span style="font-size:16px">${phaseLabel}</span>`,
    (cpVal.days_in_phase > 0 ? '持續 ' + fmtInt(cpVal.days_in_phase) + ' 天' : '') +
    (cpVal.consecutive_losses > 0 ? ' · 連虧 ' + fmtInt(cpVal.consecutive_losses) + ' 次' : '') +
    (cpVal.can_advance ? ' · 可推進' : ''));

  const ddMax = dd && dd.status !== 'not_available' ? dd.max_drawdown : null;
  if (dd && dd.status !== 'not_available' && isFiniteNumber(ddMax)) {
    cards += riskCard('Monte Carlo 壓力',
      `<span style="color:var(--warn)">${fmtSafeDrawdown(ddMax, { asAbsolute: true })}</span>`,
      '模擬 · 1000 條壓力路徑最差值（非真實回撤）');
  }

  el.innerHTML = cards;

  // Monte Carlo 最差路徑（S4 下方 details）
  const worstPath = (dd && Array.isArray(dd.worst_path)) ? dd.worst_path : [];
  if (pathWrap) {
    if (!dd || dd.status === 'not_available' || worstPath.length < 2) {
      pathWrap.style.display = 'none';
    } else {
      pathWrap.style.display = '';
      const wp = document.getElementById('consoleWorstPath');
      if (wp) {
        const minV = Math.min.apply(null, worstPath);
        const maxV = Math.max.apply(null, worstPath);
        const range = maxV - minV || 1;
        const width = 300, height = 60;
        const points = worstPath.map(function (v, i) {
          const x = (i / (worstPath.length - 1)) * width;
          const y = height - ((v - minV) / range) * height;
          return x + ',' + y;
        }).join(' ');
        wp.classList.remove('loading');
        wp.innerHTML = `
          <svg viewBox="0 0 ${width} ${height}" style="width:100%;height:${height}px;background:var(--panel-l2);border-radius:4px">
            <polyline points="${points}" fill="none" stroke="var(--down)" stroke-width="2"/>
          </svg>
          <div style="font-size:11px;color:var(--muted);margin-top:4px">產生時間：${dd.generated ? new Date(dd.generated).toLocaleString('zh-TW') : '—'}</div>`;
      }
    }
  }
}

// ── S5 持倉明細 + 持倉結構（#holdings）───────────────────
const SECTOR_LABELS = {
  'semiconductor': '半導體', 'ai_supply_chain': 'AI 供應鏈', 'robotics': '機器人',
  'financials': '金融', 'shipping': '航運', 'energy': '能源', 'electronics': '電子',
  'consumer': '消費', 'industrial': '工業', 'other': '其他',
};

function holdingsActionEmpty(title, description) {
  const hasPipeline = !!document.getElementById('page-pipeline');
  return `<div class="action-empty-state">
    <div class="action-empty-state-icon">📂</div>
    <div class="action-empty-state-title">${escapeHtml(title)}</div>
    <div class="action-empty-state-description">${escapeHtml(description)}</div>
    ${hasPipeline ? '<button class="primary" onclick="switchPage(\'pipeline\')">查看投資管線 →</button>' : ''}
  </div>`;
}

function computeTopConcentration(positions, portfolioValue) {
  // risk-exposure.concentration 缺位時的 fallback：以持倉市值排序取前 5
  return positions
    .filter(function (p) { return isFiniteNumber(p.market_value); })
    .sort(function (a, b) { return b.market_value - a.market_value; })
    .slice(0, 5)
    .map(function (p) {
      return {
        symbol: p.symbol,
        market_value: p.market_value,
        weight: portfolioValue > 0 ? p.market_value / portfolioValue : 0,
      };
    });
}

function computeSectorExposure(positions, portfolioValue) {
  // risk-exposure.sector_exposure 缺位時的 fallback：依持倉 sector 加總市值權重
  const bySector = {};
  positions.forEach(function (p) {
    if (!isFiniteNumber(p.market_value)) return;
    const key = p.sector || 'other';
    bySector[key] = (bySector[key] || 0) + p.market_value;
  });
  return Object.keys(bySector).map(function (key) {
    return { sector: key, weight: portfolioValue > 0 ? bySector[key] / portfolioValue : 0 };
  }).filter(function (s) { return s.weight > 0; }).sort(function (a, b) { return b.weight - a.weight; });
}

function renderHoldings(ps, re) {
  const tableEl = document.getElementById('consolePositionsTable');
  const concEl = document.getElementById('consoleTopConcentration');
  const sectorEl = document.getElementById('consoleSectorDistribution');
  if (!tableEl && !concEl && !sectorEl) return;

  const positions = (ps && Array.isArray(ps.positions)) ? ps.positions : [];
  let portfolioValue = ps && isFiniteNumber(ps.portfolio_value) ? ps.portfolio_value : null;
  if (portfolioValue === null) {
    const mvSum = positions.reduce(function (s, p) { return s + (isFiniteNumber(p.market_value) ? p.market_value : 0); }, 0);
    const cash = ps && isFiniteNumber(ps.cash) ? ps.cash : 0;
    portfolioValue = mvSum + cash;
  }

  if (tableEl) {
    if (positions.length === 0) {
      tableEl.classList.remove('loading');
      tableEl.innerHTML = holdingsActionEmpty('尚無持倉資料', '執行模擬交易後，這裡會顯示持倉明細、權重與未實現損益');
    } else {
      const sectorLabel = function (s) {
        return (re && s && sectorName(s)) || SECTOR_LABELS[s] || s || '—';
      };
      const fmtF = function (v) { return fmtSafeNumber(v, { decimals: 2 }); };
      const fmtI = function (v) { return isFiniteNumber(v) ? v.toLocaleString('en-US') : '—'; };
      const rows = positions.map(function (pos) {
        const mv = isFiniteNumber(pos.market_value) ? pos.market_value : null;
        const pnl = isFiniteNumber(pos.unrealized_pnl) ? pos.unrealized_pnl : null;
        const pct = isFiniteNumber(pos.pnl_pct) ? pos.pnl_pct : null;
        const weight = portfolioValue > 0 && mv !== null ? mv / portfolioValue : null;
        const colorClass = pnl !== null ? (pnl > 0 ? 'text-up' : (pnl < 0 ? 'text-down' : '')) : '';
        return `<tr>
          <td>${renderStockCell(pos.symbol)}</td>
          <td>${escapeHtml(sectorLabel(pos.sector))}</td>
          <td style="text-align:right">${fmtI(pos.quantity)}</td>
          <td style="text-align:right">${fmtF(pos.average_cost)}</td>
          <td style="text-align:right">${fmtF(pos.current_price)}</td>
          <td style="text-align:right">${mv !== null ? fmtNTD(mv) : '—'}</td>
          <td style="text-align:right">${weight !== null ? fmtSafePct(weight, 1) : '—'}</td>
          <td style="text-align:right" class="${colorClass}">${pnl !== null ? fmtNTD(pnl) : '—'}</td>
          <td style="text-align:right" class="${colorClass}">${pct !== null ? fmtSafeSignedPct(pct, 2) : '—'}</td>
        </tr>`;
      }).join('');

      const totalMV = positions.reduce(function (s, p) { return s + (isFiniteNumber(p.market_value) ? p.market_value : 0); }, 0);
      const totalPnl = positions.reduce(function (s, p) { return s + (isFiniteNumber(p.unrealized_pnl) ? p.unrealized_pnl : 0); }, 0);
      const totalPct = portfolioValue > 0 ? (totalMV / portfolioValue) : null;
      const totalColor = totalPnl > 0 ? 'text-up' : (totalPnl < 0 ? 'text-down' : '');
      tableEl.classList.remove('loading');
      tableEl.innerHTML = `
        <div class="table-wrapper">
          <table class="text-sm">
            <thead><tr>
              <th style="text-align:left">標的</th>
              <th style="text-align:left">板塊</th>
              <th style="text-align:right">數量 (股)</th>
              <th style="text-align:right">均價</th>
              <th style="text-align:right">現價</th>
              <th style="text-align:right">市值</th>
              <th style="text-align:right">權重</th>
              <th style="text-align:right">未實現損益</th>
              <th style="text-align:right">損益率</th>
            </tr></thead>
            <tbody>${rows}</tbody>
            <tfoot><tr style="border-top:2px solid var(--border);font-weight:700">
              <td colspan="5">合計（${positions.length} 檔）</td>
              <td style="text-align:right">${fmtNTD(totalMV)}</td>
              <td style="text-align:right">${totalPct !== null ? fmtSafePct(totalPct, 1) : '—'}</td>
              <td style="text-align:right" class="${totalColor}">${fmtNTD(totalPnl)}</td>
              <td></td>
            </tr></tfoot>
          </table>
        </div>`;
    }
  }

  // 前 5 大權重（優先 risk-exposure.concentration）
  const conc = (re && Array.isArray(re.concentration) && re.concentration.length)
    ? re.concentration : computeTopConcentration(positions, portfolioValue);
  if (concEl) {
    concEl.classList.remove('loading');
    if (conc.length === 0) {
      concEl.innerHTML = '<div style="font-size:12px;color:var(--muted)">暫無持倉資料</div>';
    } else {
      const top1Weight = isFiniteNumber(conc[0].weight) ? conc[0].weight : 0;
      const top3Weight = conc.slice(0, 3).reduce(function (s, c) { return s + (isFiniteNumber(c.weight) ? c.weight : 0); }, 0);
      const top5Weight = conc.reduce(function (s, c) { return s + (isFiniteNumber(c.weight) ? c.weight : 0); }, 0);
      const rows = conc.map(function (c, idx) {
        return `<tr><td style="padding:3px 8px;font-size:12px">${idx + 1}</td>
          <td style="padding:3px 8px;font-size:12px">${c.symbol ? renderStockCell(c.symbol) : '—'}</td>
          <td style="padding:3px 8px;font-size:12px;text-align:right">${fmtSafePct(c.weight, 1)}</td>
          <td style="padding:3px 8px;font-size:12px;text-align:right">${fmtNTD(c.market_value)}</td></tr>`;
      }).join('');
      concEl.innerHTML = `
        <div style="display:flex;gap:16px;flex-wrap:wrap">
          <div style="flex:1;min-width:150px">
            <div style="font-size:12px;color:var(--muted);margin-bottom:6px">前 5 大持倉權重合計</div>
            <div style="font-size:20px;font-weight:700;color:${top5Weight > 0.6 ? 'var(--color-danger)' : (top5Weight > 0.4 ? 'var(--warn)' : 'var(--color-success)')}">${fmtSafePct(top5Weight, 1)}</div>
            <div style="font-size:11px;color:var(--muted);margin-top:4px">前 3 大 ${fmtSafePct(top3Weight, 1)} · 最大 ${fmtSafePct(top1Weight, 1)}</div>
          </div>
          <div style="flex:2;min-width:230px">
            <table style="width:100%;font-size:12px;border-collapse:collapse">
              <thead><tr style="border-bottom:1px solid var(--border)"><th style="text-align:left;padding:4px 8px">#</th><th style="text-align:left;padding:4px 8px">標的</th><th style="text-align:right;padding:4px 8px">權重</th><th style="text-align:right;padding:4px 8px">市值</th></tr></thead>
              <tbody>${rows}</tbody>
            </table>
          </div>
        </div>`;
    }
  }

  // 板塊曝險橫條圖
  const sectors = (re && Array.isArray(re.sector_exposure) && re.sector_exposure.length)
    ? re.sector_exposure.filter(function (s) { return isFiniteNumber(s.weight) && s.weight > 0; }).sort(function (a, b) { return b.weight - a.weight; })
    : computeSectorExposure(positions, portfolioValue);
  if (sectorEl) {
    sectorEl.classList.remove('loading');
    if (sectors.length === 0) {
      sectorEl.innerHTML = '<div style="font-size:12px;color:var(--muted);margin-top:8px">暫無板塊曝險資料</div>';
    } else {
      const maxW = Math.max.apply(null, sectors.map(function (s) { return s.weight; }));
      const bars = sectors.map(function (s) {
        const w = s.weight;
        const barWidthPct = Math.min(100, Math.round((w / maxW) * 1000) / 10);
        const color = w > 0.3 ? 'var(--accent)' : (w > 0.15 ? 'var(--warn)' : 'var(--muted)');
        const label = (s.sector && sectorName(s.sector)) || SECTOR_LABELS[s.sector] || s.sector || '其他';
        return `
          <div style="margin:5px 0">
            <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">
              <span>${escapeHtml(label)}</span><span>${fmtSafePct(w, 1)}</span>
            </div>
            <div style="width:100%;height:6px;background:var(--bg);border-radius:3px;overflow:hidden">
              <div style="width:${barWidthPct}%;height:100%;background:${color};border-radius:3px"></div>
            </div>
          </div>`;
      }).join('');
      sectorEl.innerHTML = `<div style="margin-top:14px">
        <div style="font-size:12px;font-weight:700;color:var(--muted);margin-bottom:2px">板塊曝險分布（市值權重）</div>
        ${bars}
      </div>`;
    }
  }
}

// ── S6 市場背景（summary 行 + SOX/半導體；narrative strip 由 renderCore tick 餵）──
function renderMarketContext(stressData, snapshot, industryCycle) {
  const summary = document.getElementById('consoleMarketSummary');
  if (summary) {
    const stressScore = fmtSafeNumber(stressData && stressData.score, { decimals: 1 });
    const sox = snapshot && snapshot.sox_index ? snapshot.sox_index : null;
    const soxValue = fmtSafeNumber(sox && sox.value, { decimals: 2, useGrouping: true });
    const phase = industryCycle && industryCycle.business_cycle ? businessCycleLabel(industryCycle.business_cycle) : null;
    const parts = [];
    if (stressScore !== '—') parts.push('外資出逃指數 ' + stressScore);
    if (soxValue !== '—') parts.push('SOX ' + soxValue);
    if (phase && phase !== '-') parts.push('半導體週期 ' + phase);
    summary.textContent = parts.length ? parts.join(' · ') : '';
  }
  renderSemiconductorSentiment(snapshot, industryCycle || {});
}

// ── S7 控制層今日處置（funnel + 異常列）────────────────────
function funnelStageCounts(macro, pipeline) {
  const guard = (macro && Array.isArray(macro.guard_outcomes)) ? macro.guard_outcomes : [];
  if (guard.length) {
    const rawInputs = guard[0].input_count || 0;
    const finalOutputs = guard[guard.length - 1].output_count || 0;
    return { rawInputs: rawInputs, finalOutputs: finalOutputs, guard: guard };
  }
  const items = (pipeline && Array.isArray(pipeline.items)) ? pipeline.items : [];
  const rawInputs = items.length;
  const finalOutputs = items.filter(function (it) { return it.passed_guards !== false; }).length;
  return { rawInputs: rawInputs, finalOutputs: finalOutputs, guard: [] };
}

function renderControlDisposition(macro, pipeline) {
  const el = document.getElementById('consoleControlDisposition');
  if (!el) return;
  if (!macro || !macro.session_id) {
    el.innerHTML = holdingsActionEmpty('尚無回測處置資料', '執行模擬回測後，這裡會顯示控制層對 AI 推薦的放行漏斗與異常');
    return;
  }
  const counts = funnelStageCounts(macro, pipeline);
  const guard = counts.guard;
  const rawInputs = counts.rawInputs;
  const finalOutputs = counts.finalOutputs;
  const filteredCount = Math.max(0, rawInputs - finalOutputs);
  const regimeColor = macro.regime === 'RISK_ON' ? 'var(--up)' : (macro.regime === 'RISK_OFF' ? 'var(--down)' : (macro.regime === 'NEUTRAL' ? 'var(--warn)' : 'var(--muted)'));

  // 漏斗視覺：AI 推薦 → 各控制層關卡 → 模擬投組
  let stages = '<div class="console-funnel-step"><div class="console-funnel-count">' + fmtInt(rawInputs) + '</div><div class="console-funnel-label">AI 推薦</div></div>';
  guard.forEach(function (g, i) {
    const out = g.output_count || 0;
    const inCount = g.input_count || 0;
    const dropColor = (out < inCount || g.passed === false) ? 'var(--color-danger)' : 'var(--color-success)';
    stages += '<span class="console-funnel-arrow" style="color:' + dropColor + '">→</span>' +
      '<div class="console-funnel-step"><div class="console-funnel-count" style="color:' + dropColor + '">' + fmtInt(out) + '</div><div class="console-funnel-label">' + escapeHtml(agentName(g.guard_id)) + '</div></div>';
  });
  stages += '<span class="console-funnel-arrow">→</span>' +
    '<div class="console-funnel-step"><div class="console-funnel-count">' + fmtInt(finalOutputs) + '</div><div class="console-funnel-label">模擬投組</div></div>';

  // 異常列：只列有過濾 / 強制阻擋的關卡
  const anomalyLines = guard
    .filter(function (g) { return g.passed === false || ((g.input_count || 0) - (g.output_count || 0)) > 0; })
    .map(function (g) {
      const inputCount = g.input_count || 0;
      const outputCount = g.output_count || 0;
      const filtered = inputCount - outputCount;
      if (g.passed === false) {
        return `<div class="console-anomaly console-anomaly--danger">● ${escapeHtml(agentName(g.guard_id))} 強制阻擋全部推薦（${fmtInt(inputCount)} → 0）</div>`;
      }
      return `<div class="console-anomaly console-anomaly--warn">● ${escapeHtml(agentName(g.guard_id))} 過濾 ${fmtInt(filtered)} 筆（${fmtInt(inputCount)} → ${fmtInt(outputCount)}）</div>`;
    });
  const anomalyHtml = anomalyLines.length
    ? '<div class="console-anomaly-list">' + anomalyLines.join('') + '</div>'
    : (guard.length ? '<div class="console-anomaly console-anomaly--ok">● 所有控制層關卡皆放行</div>' : '');

  const hasPipelinePage = !!document.getElementById('page-pipeline');
  el.classList.remove('loading');
  el.innerHTML = `
    <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap;margin-bottom:10px">
      <span style="font-size:12px;color:var(--muted)">場次 <code>${escapeHtml(macro.session_id)}</code></span>
      <span style="font-size:12px;color:${regimeColor}">市場狀態 ${escapeHtml(macro.regime || '—')}</span>
      ${macro.recorded_at ? `<span style="font-size:11px;color:var(--muted)">${new Date(macro.recorded_at).toLocaleString('zh-TW')}</span>` : ''}
      ${filteredCount > 0 ? `<span class="text-warn" style="font-size:12px">共過濾 ${fmtInt(filteredCount)} 筆</span>` : ''}
    </div>
    <div class="console-funnel">${stages}</div>
    ${anomalyHtml}
    ${hasPipelinePage ? '<div style="margin-top:10px"><a href="#" onclick="switchPage(\'pipeline\');return false;" style="color:var(--accent);text-decoration:underline;font-size:13px">📋 查看投資管線完整明細 →</a></div>' : ''}
  `;
}

// ── S8 交易時間軸（分頁）─────────────────────────────────
let _tradeHistoryCache = [];

function renderTradeTable(trades, page) {
  const el = document.getElementById('consoleTradeHistory');
  if (!el) return;
  if (!trades.length) {
    el.classList.remove('loading');
    el.innerHTML = '<div class="empty">尚無交易歷史</div>';
    return;
  }
  const totalPages = Math.max(1, Math.ceil(trades.length / TRADE_PAGE_SIZE));
  const safePage = Math.max(0, Math.min(page, totalPages - 1));
  const slice = trades.slice(safePage * TRADE_PAGE_SIZE, (safePage + 1) * TRADE_PAGE_SIZE);
  const fmtI = function (v) { return isFiniteNumber(v) ? v.toLocaleString('en-US') : '—'; };
  const rows = slice.map(function (trade) {
    const quantity = isFiniteNumber(trade.quantity) ? trade.quantity : null;
    const price = isFiniteNumber(trade.price) ? trade.price : null;
    const amount = isFiniteNumber(trade.amount) ? trade.amount
      : (quantity !== null && price !== null ? quantity * price : null);
    const sideClass = trade.side === 'BUY' ? 'text-up' : 'text-down';
    const sideLabel = trade.side === 'BUY' ? '買入' : (trade.side === 'SELL' ? '賣出' : escapeHtml(trade.side || '—'));
    const ts = trade.timestamp ? new Date(trade.timestamp).toLocaleString('zh-TW') : '—';
    return `<tr>
      <td>${ts}</td>
      <td>${trade.symbol ? renderStockCell(trade.symbol) : '—'}</td>
      <td class="${sideClass}">${sideLabel}</td>
      <td style="text-align:right">${fmtI(quantity)}</td>
      <td style="text-align:right">${fmtSafeNumber(price, { decimals: 2 })}</td>
      <td style="text-align:right">${amount !== null ? fmtNTD(amount) : '—'}</td>
      <td>${escapeHtml(trade.reason || '—')}</td>
    </tr>`;
  }).join('');

  const startIdx = safePage * TRADE_PAGE_SIZE + 1;
  const endIdx = Math.min((safePage + 1) * TRADE_PAGE_SIZE, trades.length);
  const pagination = totalPages > 1 ? `
    <div class="table-pagination" style="margin-top:10px">
      <span>顯示 <strong>${startIdx}-${endIdx}</strong> / 共 <strong>${trades.length}</strong> 筆</span>
      <div style="display:flex;gap:6px;align-items:center">
        <button data-tp="0" ${safePage === 0 ? 'disabled' : ''}>« 首頁</button>
        <button data-tp="${safePage - 1}" ${safePage === 0 ? 'disabled' : ''}>‹ 上一頁</button>
        <span style="padding:0 8px">第 ${safePage + 1} / ${totalPages} 頁</span>
        <button data-tp="${safePage + 1}" ${safePage >= totalPages - 1 ? 'disabled' : ''}>下一頁 ›</button>
        <button data-tp="${totalPages - 1}" ${safePage >= totalPages - 1 ? 'disabled' : ''}>末頁 »</button>
      </div>
    </div>` : '';

  el.classList.remove('loading');
  el.innerHTML = `
    <div class="table-wrapper">
      <table class="text-sm">
        <thead><tr>
          <th style="text-align:left">時間</th><th style="text-align:left">標的</th><th style="text-align:left">方向</th>
          <th style="text-align:right">數量</th><th style="text-align:right">成交價</th>
          <th style="text-align:right">成交金額</th><th style="text-align:left">原因</th>
        </tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>
    ${pagination}`;

  el.querySelectorAll('button[data-tp]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      renderTradeTable(trades, parseInt(btn.getAttribute('data-tp'), 10));
    });
  });
}

function renderTradeHistory(trades) {
  _tradeHistoryCache = Array.isArray(trades) ? trades : [];
  renderTradeTable(_tradeHistoryCache, 0);
}

// ── 主載入 ───────────────────────────────────────────────
export async function loadRiskConsolePage(getJSON, agentNameFn, opts) {
  opts = opts || {};
  const fresh = !!opts.fresh;
  const root = document.getElementById('page-live');
  if (!root || !document.getElementById('consoleCapitalCards')) return;

  if (fresh) {
    // 首次進入：讓面板回到「載入中」狀態（background tick 則原地更新避免閃爍）
    ['consoleCapitalCards', 'consoleRiskCards', 'consolePositionsTable', 'consoleControlDisposition', 'consoleTradeHistory']
      .forEach(function (id) {
        const elx = document.getElementById(id);
        if (elx) elx.innerHTML = '<div class="loading">載入中…</div>';
      });
  }

  // overview（Phase 1 聚合端點；未上線時 404 → 全 fallback）
  const overview = await safeGet(getJSON, '/api/dashboard/overview', null);
  const hasOverview = isObj(overview) && (isObj(overview.live_status) || isObj(overview.portfolio_state) || isObj(overview.risk_exposure));
  const ovLS = hasOverview && isObj(overview.live_status) ? overview.live_status : null;
  const ovPS = hasOverview && isObj(overview.portfolio_state) ? overview.portfolio_state : null;
  const ovRE = hasOverview && isObj(overview.risk_exposure) ? overview.risk_exposure : null;
  const degraded = !!((hasOverview && overview.degraded));

  const results = await Promise.all([
    ovLS ? Promise.resolve(ovLS) : safeGet(getJSON, '/api/dashboard/live-status', null),
    ovRE ? Promise.resolve(ovRE) : safeGet(getJSON, '/api/dashboard/risk-exposure', null),
    safeGet(getJSON, '/api/dashboard/portfolio-state', null),   // S3 權益曲線（overview 精華版刻意不含）
    safeGet(getJSON, '/api/dashboard/trade-history', []),
    safeGet(getJSON, '/api/dashboard/macro-radar', null),
    safeGet(getJSON, '/api/dashboard/recommendation-pipeline', null),
    safeGet(getJSON, '/api/dashboard/capital-phase', null),
    safeGet(getJSON, '/api/dashboard/risk-calibration', null),
    safeGet(getJSON, '/api/dashboard/drawdown', null),
    safeGet(getJSON, '/api/taiwan/stress-index', null),
    safeGet(getJSON, '/api/macro/snapshot/latest', null),
    safeGet(getJSON, '/api/dashboard/industry-cycle?industry=semiconductor', null),
  ]);
  const [liveStatus, riskExposure, equityState, trades, macroRadar, pipelineData, capitalPhase, calibration, drawdown, stressData, snapshot, industryCycle] = results;

  const ps = ovPS || equityState || {};

  renderCapitalCards(liveStatus, degraded);
  renderEquityTrend(equityState);
  renderRiskRow(riskExposure, capitalPhase, drawdown);
  renderHoldings(ps, riskExposure);
  renderMarketContext(stressData, snapshot, industryCycle);
  renderControlDisposition(macroRadar, pipelineData);
  renderTradeHistory(trades);
  if (document.getElementById('riskCalibration')) renderRiskCalibration(calibration);

  // S1 風控長評語（self-fetch；background tick 亦更新）
  if (!opts.skipCommentary) {
    renderRiskCommentary(document.getElementById('liveRiskCommentary'), { banner: true }).catch(function (e) {
      console.warn('[risk-console] commentary fetch failed', e);
    });
  }
}

export async function refreshRiskConsole(getJSON, agentNameFn) {
  return loadRiskConsolePage(getJSON, agentNameFn, { background: true });
}
