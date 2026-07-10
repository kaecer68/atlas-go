import { stressLabel, regionName, sectorName, templateName, capitalFlowName, modelName, timeWindowName, confidenceSourceName, severityName, statusName } from '../names.js';
import { narrativeThemeLabel } from '../shared/constants.js';
import { renderEmptyState, sortNarrativeEvents } from '../shared/app-utils.js';
import { escapeHtml, fmtFloat, fmtPct, fmtInt } from '../shared/utils.js';
import { formatNumber, formatMaxDrawdown, fmtSignedPct, formatSigned } from '../shared/format-metric.js';

/**
 * 包裝 renderEmptyState，附加可執行的行動按鈕。
 * @param {string} title
 * @param {string} message
 * @param {Array<{label:string, page?:string, href?:string}>} [actions]
 * @returns {string}
 */
function renderActionEmptyState(title, message, actions) {
  let html = renderEmptyState(title, message);
  if (!actions || !actions.length) return html;
  const buttons = actions.map(a => {
    if (a.page) {
      return `<button class="btn btn-primary btn-sm action-empty-btn" onclick="window.switchPage('${escapeHtml(a.page)}')" type="button">${escapeHtml(a.label)}</button>`;
    }
    return `<a class="btn btn-primary btn-sm" href="${escapeHtml(a.href || '#')}" target="_blank" rel="noopener">${escapeHtml(a.label)}</a>`;
  }).join('');
  return html.replace('</div>', `<div class="empty-actions">${buttons}</div></div>`);
}

let templateAccordionState = { openTemplateId: null };
let modelAccordionState = { openModelId: null };

function toggleTemplateAccordion(idx) {
  const targetRow = document.getElementById('tmpl-rationale-' + idx);
  const targetBtn = document.getElementById('tmpl-btn-' + idx);
  if (!targetRow) return;

  const wasOpen = targetRow.style.display !== 'none';

  document.querySelectorAll('[id^="tmpl-rationale-"]').forEach(function(row) { row.style.display = 'none'; });
  document.querySelectorAll('[id^="tmpl-btn-"]').forEach(function(btn) { if (btn) btn.textContent = '展開 ▼'; });

  if (!wasOpen) {
    targetRow.style.display = 'table-row';
    targetBtn.textContent = '收起 ▲';
  }

  templateAccordionState.openTemplateId = wasOpen ? null : 'tmpl-rationale-' + idx;
}

function toggleSubIndicators() {
  var body = document.getElementById('subIndicatorBody');
  var arrow = document.getElementById('subIndicatorArrow');
  if (!body || !arrow) return;
  var isHidden = body.style.display === 'none' || body.style.display === '';
  body.style.display = isHidden ? 'block' : 'none';
  arrow.textContent = isHidden ? '▲' : '▼';
  arrow.style.transform = isHidden ? 'rotate(180deg)' : 'rotate(0deg)';
}

function toggleModelAccordion(idx) {
  const targetRow = document.getElementById('model-rationale-' + idx);
  const targetBtn = document.getElementById('model-btn-' + idx);
  if (!targetRow) return;

  const wasOpen = targetRow.style.display !== 'none';

  document.querySelectorAll('[id^="model-rationale-"]').forEach(function(row) { row.style.display = 'none'; });
  document.querySelectorAll('[id^="model-btn-"]').forEach(function(btn) { if (btn) btn.textContent = '展開完整論述 ▼'; });

  if (!wasOpen) {
    targetRow.style.display = 'block';
    targetBtn.textContent = '收起論述 ▲';
  }

  modelAccordionState.openModelId = wasOpen ? null : 'model-rationale-' + idx;
}

function toggleEventAccordion(idx, fieldName) {
  const targetRow = document.getElementById('event-' + fieldName + '-' + idx);
  const targetBtn = document.getElementById('event-btn-' + fieldName + '-' + idx);
  if (!targetRow) return;

  const isHidden = targetRow.style.display === 'none';
  if (isHidden) {
    targetRow.style.display = 'block';
    if (targetBtn) targetBtn.textContent = '收起說明 ▲';
  } else {
    targetRow.style.display = 'none';
    if (targetBtn) targetBtn.textContent = '展開說明 ▼';
  }
}

window.toggleEventAccordion = toggleEventAccordion;
window.toggleTemplateAccordion = toggleTemplateAccordion;
window.toggleModelAccordion = toggleModelAccordion;
window.toggleSubIndicators = toggleSubIndicators;

export function renderLiveNarrativeStrip(events, stress, models, chains) {
  const el = document.getElementById('liveNarrativeStrip');
  if (!el) return;
  el.classList.remove('loading');
  const eventList = (events && events.events) || [];
  const sortedEvents = sortNarrativeEvents(eventList.slice());
  const topEvent = sortedEvents[0];
  const stressScore = stress && typeof stress.score === 'number' ? stress.score.toFixed(1) : '-';
  const sLabel = stress ? stressLabel(stress.regime || '-') : '-';
  const stressColor = stress && stress.regime === 'crisis' ? 'var(--color-danger)' : (stress && stress.regime === 'high' ? 'var(--warn)' : 'var(--color-success)');

  // 根據外資出逃等級產生具體說明
  let stressAdvice = '外資流出壓力小，資金面寬鬆，適合正常操作。';
  if (stress && stress.regime === 'alert') stressAdvice = '外資開始流出（警戒），可能伴隨：外資賣超擴大、台股波動增加。建議觀察外資動向，避免追高。';
  else if (stress && stress.regime === 'high') stressAdvice = '外資明顯出逃（高壓），通常伴隨：台股下跌機率高、成交量萎縮、強勢股補跌。建議降低持股，提高現金比重。';
  else if (stress && stress.regime === 'crisis') stressAdvice = '外資大量出逃（危機），通常伴隨：台股急跌、融資斷頭、 systemic risk。建議大幅減碼或空倉觀望，等待資金回流訊號。';

  // 從 narrative models / chains 找對應板塊
  let favored = [];
  let avoided = [];
  if (topEvent && topEvent.theme) {
    const modelList = (models && models.models) || [];
    const chainList = (chains && chains.chains) || [];
    const matchedModel = modelList.find(m => (m.active_themes || []).includes(topEvent.theme));
    const matchedChain = chainList.find(c => c.trigger_theme === topEvent.theme);
    if (matchedModel) {
      favored = matchedModel.favored_sectors || [];
      avoided = matchedModel.avoided_sectors || [];
    } else if (matchedChain) {
      favored = matchedChain.favored_sectors || [];
      avoided = matchedChain.avoided_sectors || [];
    }
  }

  const sentiment = topEvent ? (topEvent.sentiment || 0) : 0;
  const sentimentText = sentiment > 0 ? '偏多' : (sentiment < 0 ? '偏空' : '中性');
  const sentimentColor = sentiment > 0 ? 'var(--up)' : (sentiment < 0 ? 'var(--down)' : 'var(--warn)');

  const favoredBadges = favored.length ? favored.map(s => `<span class="badge ok">${sectorName(s)}</span>`).join(' ') : '<span class="badge" class="bg-muted text-muted">—</span>';
  const avoidedBadges = avoided.length ? avoided.map(s => `<span class="badge err">${sectorName(s)}</span>`).join(' ') : '<span class="badge" class="bg-muted text-muted">—</span>';

  const stressHelpTitle = '外商出逃指數說明';
  const stressHelpHtml = `<p><strong>外商出逃指數是什麼？</strong><br>這是追蹤<strong>外商及國際資金撤離台灣市場壓力</strong>的綜合指標，範圍 <strong>0 ~ 100</strong>。分數越高，代表外商賣超壓力越大，台股面臨資金流出風險。</p>
<p><strong>燈號說明：</strong></p>
<ul style='margin:6px 0;padding-left:18px;line-height:1.8'>
  <li><span style='color:var(--color-success)'><strong>🟢 綠燈（0~29分）</strong></span>：外商流出壓力小。資金面寬鬆，台股上漲機率高。</li>
  <li><span style='color:var(--warn)'><strong>🟡 黃燈（30~49分）</strong></span>：外商開始流出。可能伴隨台股波動增加，建議觀察。</li>
  <li><span style='color:var(--warn)'><strong>🟠 橙燈（50~69分）</strong></span>：外商明顯出逃。台股下跌機率高，建議降低持股。</li>
  <li><span style='color:var(--color-danger)'><strong>🔴 紅燈（70~100分）</strong></span>：外商大量出逃。台股急跌風險高，建議空倉觀望。</li>
</ul>
<p><strong>計算組成：</strong><br>指數由六項因子加權構成，核心為外商淨流向（權重25%），輔以美元指數、美債殖利率、VIX、日圓、地緣政治風險。</p>
<p><strong>重要提醒：</strong><br>此指數僅追蹤<strong>資金面壓力</strong>，不代表台股一定漲跌。外商流出時，內資或散戶可能承接，形成「價跌量縮」或「價穩量縮」等不同走勢。</p>`;

  el.innerHTML = `
    <div style="display:flex;gap:18px;flex-wrap:wrap;align-items:flex-start">
      <div class="metric">
        <div class="label" style="cursor:pointer;text-decoration:underline dotted;color:var(--accent)" data-help="${stressHelpHtml.replace(/"/g, '&quot;')}" data-title="${stressHelpTitle}">外商出逃指數 <span class="text-xs">ℹ️</span></div>
        <div class="value" style="color:${stressColor}">${stressScore}</div>
      </div>
      <div class="metric"><div class="label">壓力等級</div><div class="value">${sLabel}</div></div>
      <div class="metric" style="flex:1;min-width:260px">
        <div class="label">主要敘事事件</div>
        <div class="value" style="font-size:14px">${topEvent ? escapeHtml(narrativeThemeLabel(topEvent.theme)) : '無活躍事件'}</div>
        <div style="margin-top:6px;font-size:12px;color:var(--muted);line-height:1.6">
          情緒方向：<strong style="color:${sentimentColor}">${sentimentText}</strong> · ${stressAdvice}
        </div>
        <div style="margin-top:6px;font-size:12px;line-height:1.6">
          <div style="margin-bottom:3px">敘事看多板塊：${favoredBadges}</div>
          <div>敘事看空板塊：${avoidedBadges}</div>
        </div>
        ${topEvent && topEvent.explanation ? `<details class="mt-sm">
          <summary style="font-size:12px;color:var(--accent);cursor:pointer">LLM 解析（點擊展開）</summary>
          <div class="mt-xs" style="padding:10px;background:var(--bg);border-radius:6px;font-size:12px;line-height:1.7;color:var(--text);white-space:pre-wrap"><strong>事件說明：</strong>\n${escapeHtml(topEvent.explanation)}</div>
        </details>` : ''}
        ${topEvent && topEvent.sentiment_explanation ? `<details class="mt-sm">
          <summary style="font-size:12px;color:${sentimentColor};cursor:pointer">情緒解釋（點擊展開）</summary>
          <div class="mt-xs" style="padding:10px;background:var(--bg);border-radius:6px;font-size:12px;line-height:1.7;color:var(--text);white-space:pre-wrap"><strong style="color:${sentimentColor}">情緒說明：</strong>\n${escapeHtml(topEvent.sentiment_explanation)}</div>
        </details>` : ''}
      </div>
      <div style="margin-left:auto">
        <button class="pipeline-action" onclick="switchPage('narrative')">查看宏觀敘事 →</button>
      </div>
    </div>
  `;

  // Attach event listener for stress help icon
el.querySelectorAll('[data-help]').forEach(function(el) {
        el.addEventListener('click', function() {
          var title = this.getAttribute('data-title');
          var helpText = this.getAttribute('data-help');
          var htmlContent = '<p>' + helpText.replace(/\n\n/g, '</p><p>').replace(/\n/g, '<br>') + '</p>';
          if (typeof window.openInfoHelp === 'function') {
            window.openInfoHelp(title, htmlContent);
          } else if (typeof openInfoHelp === 'function') {
            openInfoHelp(title, htmlContent);
          }
        });
      });
}

export function renderNarrativePage(snapshot, stress, events, chains, models, templates, retailSentiment, seasonal) {
  const macroEl = document.getElementById('narrativeMacro');
  if (macroEl) {
    macroEl.classList.remove('loading');
    if (!snapshot) { macroEl.innerHTML = renderActionEmptyState('無可用快照', '執行回測後將自動產生。', [{label: '執行模擬', page: 'pipeline'}, {label: '查看示範', page: 'crossmarket'}]); }
    else {
      const rows = [
        ['DXY-美元指數', snapshot.dxy], ['US10Y-美債10年期', snapshot.us10y], ['VIX-波動率指數', snapshot.vix],
        ['USD/TWD-匯率', snapshot.usd_twd], ['原油', snapshot.oil], ['黃金', snapshot.gold], ['日圓', snapshot.jpy]
      ];
      const capitalRows = [
        ['外資淨買超(億)', snapshot.foreign_investor_net],
        ['投信淨買超(億)', snapshot.domestic_fund_net],
        ['自營商淨買超(億)', snapshot.dealer_net],
        ['融資餘額(億)', snapshot.retail_margin_balance],
        ['融券餘額(億)', snapshot.retail_short_balance]
      ];
      let latestTs = 0;
      rows.forEach(([_, pt]) => { if (pt && pt.timestamp > latestTs) latestTs = pt.timestamp; });
      const updateTime = latestTs ? new Date(latestTs * 1000).toLocaleString('zh-TW') : '-';
      const validRows = rows.filter(([_, pt]) => pt && pt.symbol);
      const hasData = validRows.length > 0;
      const isFresh = latestTs && (Date.now()/1000 - latestTs) < 86400;
      const allPresent = validRows.length === rows.length;
      const channelStatus = hasData
        ? (isFresh && allPresent ? {color:'var(--color-success)',text:'🟢 資料通道正常'}
           : isFresh ? {color:'var(--warn)',text:'🟡 部分指標待更新'}
           : allPresent ? {color:'var(--warn)',text:'🟡 資料待更新'}
           : {color:'var(--warn)',text:'🟡 部分資料待更新'})
        : {color:'var(--color-danger)',text:'🔴 資料通道異常'};
      let html = `<div style="margin-bottom:8px;display:flex;align-items:center;gap:10px">
        <span style="font-size:12px;color:${channelStatus.color};font-weight:700">${channelStatus.text}</span>
        <span class="text-muted text-sm">更新於 ${updateTime}</span>
      </div>`;
      html += '<table><thead><tr><th>指標</th><th>數值</th><th>日變動%</th></tr></thead><tbody>' +
        rows.map(([name, pt]) => {
          if (!pt) return '';
          const val = typeof pt.value === 'number' ? pt.value.toFixed(3) : '-';
          const chg = typeof pt.change_pct === 'number' ? pt.change_pct.toFixed(2) + '%' : '-';
          const cls = pt.change_pct > 0 ? 'up' : (pt.change_pct < 0 ? 'down' : '');
          return `<tr><td>${name}</td><td>${val}</td><td class="${cls}">${chg}</td></tr>`;
        }).join('') + '</tbody></table>';
      const capitalValidRows = capitalRows.filter(([_, pt]) => pt && pt.symbol);
      const capitalLatestTs = capitalRows.reduce((max, [_, pt]) => (pt && pt.timestamp > max) ? pt.timestamp : max, 0);
      const capitalHasData = capitalValidRows.length > 0;
      const capitalIsFresh = capitalLatestTs && (Date.now()/1000 - capitalLatestTs) < 86400;
      const capitalAllPresent = capitalValidRows.length === capitalRows.length;
      const capitalStatus = capitalHasData
        ? (capitalIsFresh && capitalAllPresent ? {color:'var(--color-success)',text:'🟢 正常'} : {color:'var(--warn)',text:'🟡 待更新'})
        : {color:'var(--color-danger)',text:'🔴 缺失'};
      const capitalTimeStr = capitalLatestTs ? new Date(capitalLatestTs * 1000).toLocaleString('zh-TW') : '-';
      if (capitalRows.some(([_, pt]) => pt && typeof pt.value === 'number')) {
        html += `<div class="mt-sm flex-center-gap">
          <span class="font-bold text-sm text-accent">台股三大法人資金流</span>
          <span class="text-sm font-bold" style="color:${capitalStatus.color}">${capitalStatus.text}</span>
          <span class="text-muted text-sm">更新於 ${capitalTimeStr}</span>
        </div>`;
        html += '<table><thead><tr><th>法人</th><th>淨買超(億)</th></tr></thead><tbody>' +
          capitalRows.map(([name, pt]) => {
            if (!pt) return '';
            const val = typeof pt.value === 'number' ? pt.value.toFixed(2) : '-';
            const cls = pt.value > 0 ? 'up' : (pt.value < 0 ? 'down' : '');
            return `<tr><td>${name}</td><td class="${cls}">${val}</td></tr>`;
          }).join('') + '</tbody></table>';
      } else {
        html += `<div class="mt-sm flex-center-gap">
          <span class="font-bold text-sm text-accent">台股三大法人資金流</span>
          <span class="text-sm font-bold" style="color:${capitalStatus.color}">${capitalStatus.text}</span>
          <span class="text-muted text-sm">更新於 ${capitalTimeStr}</span>
        </div>${renderActionEmptyState('暫無可用資料', '執行第一次宏觀分析後將自動更新。', [{label: '執行宏觀分析', page: 'pipeline'}])}`;
      }
      macroEl.innerHTML = html;
    }
  }

  const stressEl = document.getElementById('narrativeStress');
  if (stressEl) {
    stressEl.classList.remove('loading');
    if (!stress) { stressEl.innerHTML = renderActionEmptyState('無可用壓力資料', '執行回測或模擬後將顯示壓力測試結果。', [{label: '執行模擬', page: 'pipeline'}]); }
    else {
      const score = typeof stress.score === 'number' ? stress.score.toFixed(1) : '-';
      const sLabel = stressLabel(stress.regime || '-');
      const regimeColor = stress.regime === 'crisis' ? 'var(--color-danger)' : (stress.regime === 'high' ? 'var(--warn)' : 'var(--color-success)');
      const comps = stress.components || {};
      const stressTime = stress.timestamp ? new Date(stress.timestamp * 1000).toLocaleString('zh-TW') : new Date().toLocaleString('zh-TW');
      let html = `<div class="mb-sm text-muted text-sm">資料更新時間：${stressTime}</div>`;
      html += `<div style="font-size:28px;font-weight:700;color:${regimeColor}">${score} <span style="font-size:14px;color:var(--muted)">/ 100</span></div>`;
      html += `<div style="margin:4px 0 10px;font-size:14px">出逃等級：<strong>${stressLabel(stress.regime || '-')}</strong>（${stress.score >= 70 ? '🔴 紅燈' : (stress.score >= 50 ? '🟠 橙燈' : (stress.score >= 30 ? '🟡 黃燈' : '🟢 綠燈'))}）</div>`;
      html += `<table><thead><tr><th>子項</th><th>壓力貢獻 <span class="cursor-pointer text-accent" data-help="<p><strong>分數代表什麼？</strong></p><p>外商出逃指數由六個因子加權構成。每個子項的分數代表該因子對「外商撤離台灣」這個現象的貢獻度。</p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>分數越高</strong>：該因子越可能導致外商賣超台股（例如美元走強→外商匯出獲利、VIX飆升→全球避險情緒）。</li><li><strong>分數為 0</strong>：該因子目前沒有施壓（例如外商買超時，外商流向因子為 0）。</li><li><strong>所有子項皆為正值</strong>：指數只累加壓力，不扣除「助力」。這是單向指標。</li></ul><p><strong>為什麼是單向指標？</strong><br>因為外商買超時，系統不會顯示「負壓力」，而是讓總分維持低位（綠燈）。這樣設計是為了突出「危險訊號」，而非平衡呈現多空。</p><p><strong>總分區間意義：</strong><br>• 0-29分（綠燈）：外商流出壓力小，資金面寬鬆<br>• 30-49分（黃燈）：外商開始流出，注意波動<br>• 50-69分（橙燈）：外商明顯出逃，台股下跌機率高<br>• 70-100分（紅燈）：外商大量出逃，系統性風險高</p>" data-title="外商出逃指數分數說明">ℹ️</span></th></tr></thead><tbody>`;
      const names = { dxy: 'DXY-美元指數', us10y: 'US10Y-美債10年期', foreign_flow: '外資流向', vix: 'VIX-波動率指數', jpy: '日圓-套利平倉壓力', geopolitical: '地緣政治風險', oil: '原油價格衝擊', gold: '黃金避險需求' };
      for (const k of Object.keys(comps)) {
        html += `<tr><td>${names[k] || k}</td><td>${typeof comps[k] === 'number' ? comps[k].toFixed(2) : '-'}</td></tr>`;
      }
      html += '</tbody></table>';
      stressEl.innerHTML = html;
      stressEl.querySelectorAll('[data-help]').forEach(function(el) {
        el.addEventListener('click', function() {
          var title = this.getAttribute('data-title');
          var helpText = this.getAttribute('data-help');
          var htmlContent = '<p>' + helpText.replace(/\n\n/g, '</p><p>').replace(/\n/g, '<br>') + '</p>';
          if (typeof window.openInfoHelp === 'function') {
            window.openInfoHelp(title, htmlContent);
          } else if (typeof openInfoHelp === 'function') {
            openInfoHelp(title, htmlContent);
          }
        });
      });
    }
  }

  const eventsEl = document.getElementById('narrativeEvents');
  if (eventsEl) {
    eventsEl.classList.remove('loading');
    const list = (events && events.events) || [];
    // 同樣按強度排序，讓最劇烈的事件優先呈現
    const sortedList = sortNarrativeEvents(list.slice());
    if (!sortedList.length) { eventsEl.innerHTML = renderActionEmptyState('目前無觸發的宏觀敘事', '當市場出現顯著事件時，此處會列出敘事與影響。', [{label: '查看市場概覽', page: 'home'}]); }
    else {
      eventsEl.innerHTML = sortedList.map((e, idx) => {
        const sClass = e.sentiment > 0 ? 'up' : 'down';
        const sColor = e.sentiment > 0 ? 'var(--up)' : (e.sentiment < 0 ? 'var(--down)' : 'var(--warn)');
        const sText = e.sentiment > 0 ? '正面' : '負面';
        const tw = timeWindowName(e.time_window || '-');
        const sev = severityName(e.severity || '-');
        const st = statusName(e.status || '-');
        const cs = confidenceSourceName(e.confidence_source || '-');
        // Build source data display (translated keys)
        let sourceDataHtml = '';
        if (e.source_data && Object.keys(e.source_data).length > 0) {
          const keyMap = {
            'us10y_change_bps': '美債10年變動(bps)',
            'dxy_change_pct': '美元指數變動%',
            'vix_level': 'VIX 水位',
            'usd_twd_change_pct': '台幣變動%',
            'oil_change_pct': '油價變動%',
            'gold_change_pct': '黃金變動%',
            'gold_level': '黃金價位',
            'jpy_change_pct': '日圓變動%',
            'jpy_level': '日圓價位',
            'ai_capex_sentiment': 'AI 資本支出情緒',
            'geopolitical_gpr': '地緣政治風險指數',
            'margin_zscore': '融資 Z-score',
            'retail_institutional_divergence': '散戶機構分歧',
            'earnings_surprise_pct': '財報驚喜%',
            'cpi_yoy': 'CPI 年增率%',
            'bdi_change_pct': 'BDI 變動%',
            'copper_change_pct': '銅價變動%',
            'export_electronics_change_pct': '電子出口變動%',
            'sox_index_change_pct': 'SOX 指數變動%',
            'dram_spot_price_change_pct': 'DRAM 現貨變動%'
          };
          const sdItems = Object.entries(e.source_data).map(([k, v]) => {
            const label = keyMap[k] || k;
            const val = typeof v === 'number' ? v.toFixed(2) : v;
            return `${label}: ${val}`;
          }).join(' · ');
          sourceDataHtml = `<div class="text-muted text-sm mt-xs" style="font-size:11px">觸發條件：${escapeHtml(sdItems)}</div>`;
        }
        return `<div style="border-left:3px solid var(--accent);padding:10px 12px;margin:8px 0;background:var(--panel-l2);border-radius:8px">
          <div class="font-bold">${escapeHtml(narrativeThemeLabel(e.theme))} <span class="${sClass}">${sText} (${e.sentiment})</span></div>
          <div class="text-muted text-sm mt-xs">區域：${escapeHtml(regionName(e.region))} · 信心度：${fmtPct(e.confidence)} · 嚴重程度：${escapeHtml(sev)} · 狀態：${escapeHtml(st)}</div>
          <div class="text-muted text-sm mt-xs">資金流：${escapeHtml(capitalFlowName(e.capital_flow || '-'))} · 時間窗口：${escapeHtml(tw)} · 信心來源：${escapeHtml(cs)}</div>
          ${sourceDataHtml}
          ${e.explanation ? `<details class="mt-sm">
            <summary style="font-size:12px;color:var(--accent);cursor:pointer">LLM 解析（點擊展開）</summary>
            <div class="mt-xs" style="padding:10px;background:var(--bg);border-radius:6px;font-size:12px;line-height:1.7;color:var(--text);white-space:pre-wrap"><strong>事件說明：</strong>\n${escapeHtml(e.explanation)}</div>
          </details>` : ''}
          ${e.sentiment_explanation ? `<details class="mt-sm">
            <summary style="font-size:12px;color:${sColor};cursor:pointer">情緒解釋（點擊展開）</summary>
            <div class="mt-xs" style="padding:10px;background:var(--bg);border-radius:6px;font-size:12px;line-height:1.7;color:var(--text);white-space:pre-wrap"><strong style="color:${sColor}">情緒說明：</strong>\n${escapeHtml(e.sentiment_explanation)}</div>
          </details>` : ''}
        </div>`;
      }).join('');
    }
  }

  const chainsEl = document.getElementById('narrativeChains');
  if (chainsEl) {
    chainsEl.classList.remove('loading');
    const list = (chains && chains.chains) || [];
    if (!list.length) { chainsEl.innerHTML = renderActionEmptyState('目前無匹配的因果鏈', '因果鏈會在執行宏觀分析後產生。', [{label: '執行宏觀分析', page: 'pipeline'}]); }
    else {
      chainsEl.innerHTML = list.map(c => `
        <div style="margin:12px 0;padding:12px;background:var(--panel-l2);border-radius:10px;border:1px solid var(--border)">
          <div style="font-weight:700;font-size:14px;margin-bottom:10px;color:var(--text)">${escapeHtml(templateName(c.template_id))}</div>
          <div style="font-size:11px;color:var(--muted);margin-bottom:12px">匹配分數 <span style="color:var(--accent);font-weight:600">${formatNumber(c.score, { decimals: 3 })}</span></div>
          ${(c.favored_sectors || []).length || (c.avoided_sectors || []).length ? `
            <div style="margin-bottom:12px;display:flex;flex-wrap:wrap;gap:6px;align-items:center">
              ${(c.favored_sectors || []).map(s => '<span class="badge ok">+ ' + escapeHtml(sectorName(s) || s) + '</span>').join('')}
              ${(c.avoided_sectors || []).map(s => '<span class="badge err">- ' + escapeHtml(sectorName(s) || s) + '</span>').join('')}
            </div>
          ` : ''}
          ${(c.steps || []).map((s, i) => {
            const impactVal = typeof s.impact === 'number' && Number.isFinite(s.impact) ? s.impact : null;
            const impClass = impactVal != null && impactVal > 0 ? 'positive' : (impactVal != null && impactVal < 0 ? 'negative' : '');
            const impLabel = impactVal != null ? formatSigned(impactVal, { decimals: 0, forceSign: true }) : '—';
            const affected = (s.affected || []).map(a => `<span class="sector-tag">${escapeHtml(sectorName(a) || a)}</span>`).join('');
            return `<div class="chain-step">
              <div style="display:flex;align-items:center;gap:8px">
                <div class="chain-step-num">${i+1}</div>
                <div style="flex:1;font-size:12px;line-height:1.6;color:var(--text)">${escapeHtml(s.description)}</div>
                <div class="chain-impact ${impClass}">${impLabel}</div>
              </div>
              ${affected ? `<div style="margin-top:8px;padding-left:32px">${affected}</div>` : ''}
            </div>`;
          }).join('')}
        </div>
      `).join('');
    }
  }

  const modelsEl = document.getElementById('narrativeModels');
  if (modelsEl) {
    modelsEl.classList.remove('loading');
    const list = (models && models.models) || [];
    if (!list.length) { modelsEl.innerHTML = renderActionEmptyState('目前無活躍模型', '模型會在回測或模擬運行後啟動。', [{label: '執行模擬', page: 'pipeline'}]); }
    else {
      modelsEl.innerHTML = list.map((m, idx) => {
        const w = m.weight || 0;
        const e = m.recent_error || 0;
        const weightPct = Math.min(100, Math.max(0, w * 100)).toFixed(1);
        const weightColor = w >= 0.5 ? 'var(--color-success)' : (w >= 0.25 ? 'var(--color-warning)' : 'var(--color-danger)');
        const errColor = e <= 0.3 ? 'var(--color-success)' : (e <= 0.5 ? 'var(--color-warning)' : 'var(--color-danger)');
        const errText = e <= 0.3 ? '優秀' : (e <= 0.5 ? '一般' : '偏高');
        return `
        <div class="weight-panel" style="border-left-color:${weightColor}">
          <div style="display:flex;justify-content:space-between;align-items:center;gap:10px">
            <div style="font-weight:700;font-size:15px">${escapeHtml(modelName(m.name))}</div>
            <div style="font-size:12px;color:var(--muted);white-space:nowrap">誤差 <span style="color:${errColor};font-weight:700">${formatNumber(m.recent_error, { percent: true, decimals: 1, suffix: '%' })} ${errText}</span></div>
          </div>
          <div style="margin:8px 0">
            <div style="display:flex;align-items:center;gap:8px;font-size:12px;color:var(--muted)">
              <span>模型權重</span>
              <div style="flex:1;height:6px;background:var(--bg);border-radius:3px;overflow:hidden">
                <div style="width:${weightPct}%;height:100%;background:${weightColor}"></div>
              </div>
              <span class="min-w-40 text-right">${fmtFloat(m.weight)}</span>
            </div>
          </div>
          <div style="margin:6px 0;display:flex;align-items:center;gap:8px;font-size:12px;color:var(--muted)">
            <span>歷史命中率</span>
            <span style="color:${typeof m.hit_rate === 'number' && Number.isFinite(m.hit_rate) ? ((m.hit_rate >= 0.7 ? 'var(--color-success)' : (m.hit_rate >= 0.5 ? 'var(--color-warning)' : 'var(--color-danger)'))) : 'var(--muted)'};font-weight:700">${fmtPct(m.hit_rate)}</span>
          </div>
          <div style="margin:6px 0;display:flex;align-items:center;gap:8px;font-size:12px;color:var(--muted)">
            <span>近期預測報酬差</span>
            <span style="color:${typeof m.recent_prediction === 'number' && Number.isFinite(m.recent_prediction) ? (m.recent_prediction > 0 ? 'var(--up)' : (m.recent_prediction < 0 ? 'var(--down)' : 'var(--muted)')) : 'var(--muted)'};font-weight:700">${fmtFloat(m.recent_prediction)}</span>
          </div>
          <div class="text-muted text-sm mt-xs">${escapeHtml(m.description || '')}</div>
          <div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:8px">
            ${(m.favored_sectors || []).map(s => `<span class="badge ok">+ ${escapeHtml(sectorName(s))}</span>`).join('')}
            ${(m.avoided_sectors || []).map(s => `<span class="badge err">− ${escapeHtml(sectorName(s))}</span>`).join('')}
          </div>
          ${m.rationale ? `<div class="mt-sm">
            <button id="model-btn-${idx}" onclick="toggleModelAccordion(${idx})" style="font-size:12px;color:var(--accent);background:none;border:none;padding:0;cursor:pointer">展開完整論述 ▼</button>
            <div id="model-rationale-${idx}" style="display:none;margin-top:8px;padding:10px;background:var(--bg);border-radius:6px;font-size:12px;line-height:1.7;color:var(--text);white-space:pre-wrap">${escapeHtml(m.rationale)}</div>
          </div>` : ''}
        </div>
      `;
      }).join('');
    }
  }

  const templatesEl = document.getElementById('narrativeTemplates');
  if (templatesEl) {
    templatesEl.classList.remove('loading');
    const items = (templates && templates.templates) || [];
    if (!items.length) { templatesEl.innerHTML = renderActionEmptyState('無模板資料', '模板庫會隨著策略演化自動累積。', [{label: '查看策略演化', page: 'evolution_panel'}]); }
    else {
      templatesEl.innerHTML = `<table class="template-table">
        <thead><tr><th style="width:40%">模板名稱</th><th style="width:12%">歷史命中率</th><th style="width:36%">資料來源</th><th style="width:12%">操作</th></tr></thead>
        <tbody>
          ${items.map((t, idx) => `<tr>
            <td><span style="font-weight:600;color:var(--text)">${escapeHtml(templateName(t.name))}</span></td>
            <td><span style="font-weight:500;color:var(--color-success)">${fmtPct(t.historical_hit_rate)}</span></td>
            <td class="text-muted text-xs">${escapeHtml((t.source_references || []).join(', '))}</td>
            <td><button id="tmpl-btn-${idx}" onclick="toggleTemplateAccordion(${idx})" style="font-size:11px;padding:3px 8px;border-radius:4px;border:1px solid var(--accent);background:transparent;color:var(--accent);cursor:pointer">展開 ▼</button></td>
          </tr>
          <tr id="tmpl-rationale-${idx}" class="hidden">
            <td colspan="4" style="background:var(--bg);padding:12px 14px;font-size:12px;line-height:1.8;color:var(--text);white-space:pre-wrap;border-left:3px solid var(--accent)">${escapeHtml(t.rationale || '暫無論述')}</td>
          </tr>`).join('')}
        </tbody>
      </table>`;
    }
  }

  const retailEl = document.getElementById('narrativeRetailSentiment');
  if (retailEl) {
    retailEl.classList.remove('loading');
    if (!retailSentiment) { retailEl.innerHTML = renderActionEmptyState('無散戶情緒資料', '執行市場分析後將顯示散戶情緒指標。', [{label: '執行分析', page: 'pipeline'}]); }
    else {
      const hasValidData = typeof retailSentiment.margin_balance === 'number' && retailSentiment.margin_balance > 0;
      const readingMap = { frenzy: '狂熱', neutral: '中性', fear: '恐慌' };
      const readingClass = retailSentiment.extreme_reading === 'frenzy' ? 'err' : retailSentiment.extreme_reading === 'fear' ? 'warn' : 'ok';

      const sentimentScoreRaw = retailSentiment.sentiment_score;
      const marginChangeRaw = retailSentiment.margin_change_pct;
      const marginBalanceRaw = retailSentiment.margin_balance;
      const dayTradingRatioRaw = retailSentiment.day_trading_ratio;
      const marginPercentileRaw = retailSentiment.margin_percentile;
      const shortBalanceRaw = retailSentiment.short_balance;
      const shortChangeRaw = retailSentiment.short_change_pct;
      const compositeScoreRaw = retailSentiment.composite_sentiment;

      const score = fmtFloat(sentimentScoreRaw);
      const changeStr = fmtSignedPct(marginChangeRaw);
      const changeClass = marginChangeRaw != null && marginChangeRaw >= 0 ? 'up' : 'down';
      const dataStatusBadge = hasValidData
        ? '<span class="badge ok">🟢 資料正常</span>'
        : '<span class="badge">🟡 資料待更新</span>';

      const marginPercentileValue = marginPercentileRaw != null ? marginPercentileRaw * 100 : null;
      const marginPercentileStr = formatNumber(marginPercentileValue, { decimals: 0 });
      const marginBalanceStr = formatNumber(marginBalanceRaw, { decimals: 0 });
      const dayTradingRatioValue = dayTradingRatioRaw != null ? dayTradingRatioRaw * 100 : null;
      const dayTradingRatioStr = formatNumber(dayTradingRatioValue, { decimals: 1, suffix: '%' });
      const shortBalanceStr = formatNumber(shortBalanceRaw, { decimals: 0 });
      const shortChangeStr = fmtSignedPct(shortChangeRaw);
      const shortChangeClass = shortChangeRaw != null && shortChangeRaw >= 0 ? 'up' : 'down';
      const compositeScore = fmtFloat(compositeScoreRaw);

      const sentimentHelp = `綜合融資餘額變化、當沖比率、散戶交易行為等指標計算出的散戶市場情緒指標。\\n\\n分數範圍：-1.0 ~ +1.0\\n• ＞+0.5（狂熱）：散戶過度樂觀，融資大增、當沖猖獗，市場可能接近短期頂部\\n• 0.0 ~ +0.5（偏多）：散戶積極參與，市場熱絡但尚未過熱\\n• -0.5 ~ 0.0（偏空）：散戶趨於保守，融資減少，市場觀望氣氛濃厚\\n• ＜-0.5（恐慌）：散戶極度悲觀，恐慌砍倉，歷史上常是階段性底部訊號\\n\\n當前數值：${score} — ${sentimentScoreRaw != null && sentimentScoreRaw > 0.5 ? '市場狂熱，建議減碼' : sentimentScoreRaw != null && sentimentScoreRaw > 0 ? '散戶偏多' : sentimentScoreRaw != null && sentimentScoreRaw > -0.5 ? '散戶偏空觀望' : '市場恐慌，可能接近底部'}`;

      const marginChangeHelp = `融資餘額相對前一交易日的變化百分比。融資是散戶向券商借錢買股票的行為，是觀察散戶槓桿程度的重要指標。\\n\\n解讀標準：\\n• ＞+5%：散戶瘋狂加碼，槓桿急速攀升，系統性風險劇增\\n• +2% ~ +5%：散戶積極加槓桿，市場過熱跡象浮現\\n• -2% ~ +2%：正常波動區間，散戶情緒平穩\\n• -5% ~ -2%：散戶開始去槓桿，市場降溫\\n• ＜-5%：散戶恐慌砍倉，融資大減，常伴隨市場急跌，但也可能是底部訊號\\n\\n當前數值：${changeStr} — ${marginChangeRaw != null && Math.abs(marginChangeRaw * 100) > 5 ? '散戶情緒劇烈波動' : marginChangeRaw != null && Math.abs(marginChangeRaw * 100) > 2 ? '散戶情緒明顯變化' : '正常波動範圍'}`;

      const marginBalanceHelp = `全市場散戶向券商融資買股票的總金額（單位：億元）。融資餘額越高代表散戶槓桿越大，市場風險越高。\\n\\n歷史百分位解讀：\\n• ＞90th：極高水位，散戶槓桿處於歷史高檔，系統性回調風險極高\\n• 70th ~ 90th：偏高水位，市場過熱，建議逐步降低持股\\n• 30th ~ 70th：正常區間，風險可控\\n• 10th ~ 30th：偏低水位，市場冷清，但可能是佈局時機\\n• ＜10th：極低水位，散戶幾乎離場，歷史上常是長期底部區域\\n\\n當前數值：${marginBalanceStr} 億（歷史 ${marginPercentileStr}th 百分位）\\n${marginPercentileValue != null && marginPercentileValue > 90 ? '⚠️ 融資處於歷史極高水位，系統性風險極高，建議大幅減碼' : marginPercentileValue != null && marginPercentileValue > 70 ? '⚡ 融資偏高，市場過熱，建議逐步獲利了結' : marginPercentileValue != null && marginPercentileValue > 30 ? '✅ 融資水位正常，風險可控' : marginPercentileValue != null && marginPercentileValue > 10 ? '💡 融資偏低，市場冷清，可關注佈局機會' : '📉 融資極低，散戶幾乎離場，可能是長期底部'}`;

      const dayTradingHelp = `當日沖銷（Day Trading）成交量占總成交量的比例。當沖是散戶在同一天內買進又賣出的交易行為，是觀察市場投機程度的重要指標。\\n\\n解讀標準：\\n• ＞40%：市場極度投機，散戶狂熱當沖，類似2021年航運股狂潮，短期崩盤風險極高\\n• 30% ~ 40%：當沖比率偏高，市場投機氣氛濃厚，注意追高空單風險\\n• 20% ~ 30%：正常偏高的當沖活動，市場熱絡但尚屬健康\\n• 15% ~ 20%：當沖比率正常，市場交易穩定\\n• ＜15%：當沖冷清，市場缺乏投機動能，散戶參與度低\\n\\n當前數值：${dayTradingRatioStr} — ${dayTradingRatioValue != null && dayTradingRatioValue > 40 ? '市場極度投機，高風險警戒！' : dayTradingRatioValue != null && dayTradingRatioValue > 30 ? '當沖比率偏高，注意風險' : dayTradingRatioValue != null && dayTradingRatioValue > 20 ? '當沖活躍，市場熱絡' : dayTradingRatioValue != null && dayTradingRatioValue > 15 ? '當沖比率正常' : '當沖冷清，市場觀望'}`;

      const shortBalanceHelp = `全市場散戶向券商融券賣股票的總金額（單位：億元）。融券餘額越高代表散戶看空力道越強，是觀察市場空方情緒的重要指標。\\n\\n解讀標準：\\n• 融券餘額大幅上升：散戶積極做空，市場看空情緒濃厚\\n• 融券餘額大幅下降：散戶回補空單，空方力道減弱，可能出現軋空行情\\n• 融資/融券比率異常：若融資高但融券也高，代表市場分歧加大\\n\\n當前數值：${shortBalanceStr} 億（變化 ${shortChangeStr}）\\n${shortChangeRaw != null && shortChangeRaw > 0.05 ? '⚠️ 融券大幅增加，散戶積極做空' : shortChangeRaw != null && shortChangeRaw < -0.05 ? '📈 融券大幅減少，空方回補，注意軋空風險' : '✅ 融券變化正常'}`;

      const hasSubIndicators = retailSentiment.sentiment_sub_indicators &&
          (retailSentiment.sentiment_sub_indicators.category_a || retailSentiment.sentiment_sub_indicators.category_c);

      const compositeClass = compositeScoreRaw != null && compositeScoreRaw > 0 ? 'up' : compositeScoreRaw != null && compositeScoreRaw < 0 ? 'down' : 'warn';
      const compositeLabel = compositeScoreRaw != null && compositeScoreRaw > 0.5 ? '極度樂觀' : compositeScoreRaw != null && compositeScoreRaw > 0 ? '偏多' : compositeScoreRaw != null && compositeScoreRaw > -0.5 ? '偏空' : '極度恐慌';
      const compositeHelp = `RSI-tw 綜合散戶情緒指數（Retail Sentiment Index — Taiwan）\\n\\nPart A（40%）：融資維持率、當沖比率、融資餘額變化、VIX風險映射、週選擇權PCR、零股失衡\\nPart C（25%）：散戶期貨未平倉、券商分點流向、ETF申購\\nPart D：事件調整乘數（0.8-1.2）\\n\\n分數範圍：-1.0 ~ +1.0\\n• ＞+0.5：散戶狂熱，市場接近短期頂部\\n• +0.2 ~ +0.5：散戶偏多\\n• -0.2 ~ +0.2：中性\\n• -0.5 ~ -0.2：散戶偏空\\n• ＜-0.5：散戶恐慌，可能是底部訊號\\n\\n當前數值：${compositeScore} — ${compositeLabel}`;

      var subIndicatorHTML = '';
      if (hasSubIndicators) {
        var si = retailSentiment.sentiment_sub_indicators;
        var ca = si.category_a || {};
        var cc = si.category_c || {};
        var cd = si.category_d || {};

        var aIndicatorRows = [
          ['維持率 Z-score', ca.margin_maintenance_z],
          ['當沖 Z-score', ca.day_trading_z],
          ['融資餘額 Z-score', ca.margin_balance_z],
          ['VIX 風險分數', ca.vix_risk_score],
          ['週選擇權 PCR', ca.weekly_pcr],
          ['零股交易失衡', ca.odd_lot_imbalance]
        ].map(function(r) {
          var raw = r[1];
          var v = fmtFloat(raw);
          var cls = raw != null && raw > 0.5 ? 'up' : raw != null && raw < -0.5 ? 'down' : '';
          return '<tr><td style="font-size:12px;padding:3px 8px">' + r[0] + '</td><td style="font-size:12px;text-align:right;padding:3px 8px" class="' + cls + '">' + v + '</td></tr>';
        }).join('');

        var cIndicatorRows = [
          ['散戶期貨 OI', cc.futures_retail_oi],
          ['券商分點流向', cc.broker_flow_score],
          ['ETF 申購分數', cc.etf_subscription_score]
        ].map(function(r) {
          var raw = r[1];
          var v = fmtFloat(raw);
          var cls = raw != null && raw > 0.5 ? 'up' : raw != null && raw < -0.5 ? 'down' : '';
          return '<tr><td style="font-size:12px;padding:3px 8px">' + r[0] + '</td><td style="font-size:12px;text-align:right;padding:3px 8px" class="' + cls + '">' + v + '</td></tr>';
        }).join('');

        var dEvents = (cd.active_events && cd.active_events.length > 0) ? cd.active_events.join('、') : '無觸發事件';
        var dAdj = cd.adjustment_factor || cd.d_multiplier || 1.0;
        var dAdjClass = dAdj < 0.95 ? 'warn' : dAdj > 1.05 ? 'up' : '';

        subIndicatorHTML = `
          <div class="mt-sm" style="border:1px solid var(--border);border-radius:6px;overflow:hidden">
            <div id="subIndicatorToggle" style="display:flex;align-items:center;justify-content:space-between;padding:8px 12px;cursor:pointer;background:var(--bg);user-select:none" onclick="toggleSubIndicators()">
              <span style="font-size:12px;font-weight:600;color:var(--accent)">📊 子指標明細</span>
              <span id="subIndicatorArrow" style="font-size:11px;transition:transform 0.2s">▼</span>
            </div>
            <div id="subIndicatorBody" style="display:none;padding:10px 12px;border-top:1px solid var(--border)">
              <div style="margin-bottom:10px">
                <div style="font-size:12px;font-weight:600;margin-bottom:6px;color:var(--accent)">Part A（40%）— 散戶情緒 <span style="font-weight:400;font-size:11px;color:var(--text-muted)">A Score: ${formatNumber(ca.a_score, { decimals: 3 })}</span></div>
                <table style="width:100%;border-collapse:collapse">${aIndicatorRows}</table>
              </div>
              <div style="margin-bottom:10px">
                <div style="font-size:12px;font-weight:600;margin-bottom:6px;color:var(--accent)">Part C（25%）— 機構/衍生品流向 <span style="font-weight:400;font-size:11px;color:var(--text-muted)">C Score: ${formatNumber(cc.c_score, { decimals: 3 })}</span></div>
                <table style="width:100%;border-collapse:collapse">${cIndicatorRows}</table>
              </div>
              <div>
                <div style="font-size:12px;font-weight:600;margin-bottom:6px;color:var(--accent)">Part D — 事件調整 <span style="font-weight:400;font-size:11px;color:var(--text-muted)">乘數: <span class="${dAdjClass}">${formatNumber(dAdj, { decimals: 3 })}</span></span></div>
                <div style="font-size:11px;color:var(--text)">${dEvents}</div>
              </div>
            </div>
          </div>`;
      }

      retailEl.innerHTML = `
        <div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">
          <span class="text-muted text-sm">散戶情緒指標</span>
          <span class="badge ${readingClass}">${readingMap[retailSentiment.extreme_reading] || retailSentiment.extreme_reading}</span>
          ${dataStatusBadge}
        </div>
        <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:10px">
          <div class="kpi-card" style="cursor:pointer;" data-help="${compositeHelp.replace(/"/g, '&quot;')}" data-title="RSI-tw 綜合指數說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">RSI-tw 綜合 ℹ️</div>
            <div class="kpi-value ${compositeClass}" class="text-lg">${compositeScore}</div>
          </div>
          <div class="kpi-card" style="cursor:pointer;" data-help="${sentimentHelp.replace(/"/g, '&quot;')}" data-title="情緒分數說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">情緒分數 ℹ️</div>
            <div class="kpi-value" class="text-lg">${score}</div>
          </div>
          <div class="kpi-card" style="cursor:pointer;" data-help="${marginChangeHelp.replace(/"/g, '&quot;')}" data-title="融資變化說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">融資變化 ℹ️</div>
            <div class="kpi-value ${changeClass}" class="text-lg">${changeStr}</div>
          </div>
          <div class="kpi-card" style="cursor:pointer;" data-help="${marginBalanceHelp.replace(/"/g, '&quot;')}" data-title="融資餘額說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">融資餘額 ℹ️</div>
            <div class="kpi-value" class="text-lg">${marginBalanceStr} 億</div>
          </div>
          <div class="kpi-card" style="cursor:pointer;" data-help="${dayTradingHelp.replace(/"/g, '&quot;')}" data-title="當沖比率說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">當沖比率 ℹ️</div>
            <div class="kpi-value" class="text-lg">${dayTradingRatioStr}</div>
          </div>
          <div class="kpi-card" style="cursor:pointer;" data-help="${shortBalanceHelp.replace(/"/g, '&quot;')}" data-title="融券餘額說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">融券餘額 ℹ️</div>
            <div class="kpi-value ${shortChangeClass}" class="text-lg">${shortBalanceStr} 億</div>
          </div>
        </div>
        <div class="mt-sm text-muted text-sm">歷史百分位: ${marginPercentileStr}th</div>
        ${subIndicatorHTML}
      `;

      retailEl.querySelectorAll('.kpi-card[data-help]').forEach(function(card) {
        card.addEventListener('click', function() {
          var title = this.getAttribute('data-title');
          var helpText = this.getAttribute('data-help');
          var htmlContent = '<p>' + helpText.replace(/\\n\\n/g, '</p><p>').replace(/\\n/g, '<br>') + '</p>';
          if (typeof window.openInfoHelp === 'function') {
            window.openInfoHelp(title, htmlContent);
          } else if (typeof openInfoHelp === 'function') {
            openInfoHelp(title, htmlContent);
          }
        });
      });
    }
  }

  const seasonalEl = document.getElementById('narrativeSeasonal');
  if (seasonalEl) {
    seasonalEl.classList.remove('loading');
    if (!seasonal || !seasonal.expectations || seasonal.expectations.length === 0) { seasonalEl.innerHTML = renderActionEmptyState('無季節性事件', '執行回測後將顯示季節性預期與事件。', [{label: '執行模擬', page: 'pipeline'}]); }
    else {
      const rows = seasonal.expectations.map(e => {
        const statusBadge = e.already_priced_in ? '<span class="badge">已反應</span>' : '<span class="badge ok">有驚喜潛力</span>';
        return `<tr><td>${escapeHtml(narrativeThemeLabel(e.theme))}</td><td>${fmtSignedPct(e.historical_avg_return)}</td><td>${fmtSignedPct(e.current_return)}</td><td>${fmtSignedPct(e.expectation_gap)}</td><td>${statusBadge}</td></tr>`;
      }).join('');
      seasonalEl.innerHTML = `<table><thead><tr><th>主題</th><th>歷史平均</th><th>當前報酬</th><th>預期差</th><th>狀態</th></tr></thead><tbody>${rows}</tbody></table>`;
    }
  }
}
