import { eventName, stressLabel, regionName, sectorName, templateName, capitalFlowName, modelName, timeWindowName } from '../names.js';
import { renderEmptyState } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

export function renderLiveNarrativeStrip(events, stress, models, chains) {
  const el = document.getElementById('liveNarrativeStrip');
  if (!el) return;
  el.classList.remove('loading');
  const eventList = (events && events.events) || [];
  const sortedEvents = eventList.slice().sort((a, b) => {
    const strengthA = Math.abs(a.sentiment || 0) * (a.confidence || 1);
    const strengthB = Math.abs(b.sentiment || 0) * (b.confidence || 1);
    return strengthB - strengthA;
  });
  const topEvent = sortedEvents[0];
  const stressScore = stress && typeof stress.score === 'number' ? stress.score.toFixed(1) : '-';
  const sLabel = stress ? stressLabel(stress.regime || '-') : '-';
  const stressColor = stress && stress.regime === 'crisis' ? 'var(--down)' : (stress && stress.regime === 'high' ? 'var(--warn)' : 'var(--up)');

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
      favored = matchedChain.affected_sectors || [];
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
  <li><span style='color:var(--up)'><strong>🟢 綠燈（0~29分）</strong></span>：外商流出壓力小。資金面寬鬆，台股上漲機率高。</li>
  <li><span style='color:var(--warn)'><strong>🟡 黃燈（30~49分）</strong></span>：外商開始流出。可能伴隨台股波動增加，建議觀察。</li>
  <li><span style='color:var(--warn)'><strong>🟠 橙燈（50~69分）</strong></span>：外商明顯出逃。台股下跌機率高，建議降低持股。</li>
  <li><span style='color:var(--down)'><strong>🔴 紅燈（70~100分）</strong></span>：外商大量出逃。台股急跌風險高，建議空倉觀望。</li>
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
        <div class="value" style="font-size:14px">${topEvent ? eventName(topEvent.theme) : '無活躍事件'}</div>
        <div style="margin-top:6px;font-size:12px;color:var(--muted);line-height:1.6">
          情緒方向：<strong style="color:${sentimentColor}">${sentimentText}</strong> · ${stressAdvice}
        </div>
        <div style="margin-top:6px;font-size:12px;line-height:1.6">
          <div style="margin-bottom:3px">敘事看多板塊：${favoredBadges}</div>
          <div>敘事看空板塊：${avoidedBadges}</div>
        </div>
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
    if (!snapshot) { macroEl.innerHTML = renderEmptyState('無可用快照', '執行回測後將自動產生'); }
    else {
      const rows = [
        ['DXY-美元指數', snapshot.dxy], ['US10Y-美債10年期', snapshot.us10y], ['VIX-波動率指數', snapshot.vix],
        ['USD/TWD-匯率', snapshot.usd_twd], ['原油', snapshot.oil], ['黃金', snapshot.gold], ['日圓', snapshot.jpy]
      ];
      const capitalRows = [
        ['外資淨買超(億)', snapshot.foreign_investor_net],
        ['投信淨買超(億)', snapshot.domestic_fund_net],
        ['自營商淨買超(億)', snapshot.dealer_net]
      ];
      let latestTs = 0;
      rows.forEach(([_, pt]) => { if (pt && pt.timestamp > latestTs) latestTs = pt.timestamp; });
      const updateTime = latestTs ? new Date(latestTs * 1000).toLocaleString('zh-TW') : '-';
      const validRows = rows.filter(([_, pt]) => pt && pt.symbol);
      const hasData = validRows.length > 0;
      const isFresh = latestTs && (Date.now()/1000 - latestTs) < 86400;
      const allPresent = validRows.length === rows.length;
      const channelStatus = hasData
        ? (isFresh && allPresent ? {color:'var(--up)',text:'🟢 資料通道正常'} 
           : isFresh ? {color:'var(--warn)',text:'🟡 部分指標延遲'} 
           : allPresent ? {color:'var(--warn)',text:'🟡 資料略有延遲'}
           : {color:'var(--warn)',text:'🟡 部分資料延遲'})
        : {color:'var(--down)',text:'🔴 資料通道異常'};
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
        ? (capitalIsFresh && capitalAllPresent ? {color:'var(--up)',text:'🟢 正常'} : {color:'var(--warn)',text:'🟡 延遲'})
        : {color:'var(--down)',text:'🔴 缺失'};
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
        </div>${renderEmptyState('暫無可用資料', '')}`;
      }
      macroEl.innerHTML = html;
    }
  }

  const stressEl = document.getElementById('narrativeStress');
  if (stressEl) {
    stressEl.classList.remove('loading');
    if (!stress) { stressEl.innerHTML = renderEmptyState('無可用壓力資料', ''); }
    else {
      const score = typeof stress.score === 'number' ? stress.score.toFixed(1) : '-';
      const sLabel = stressLabel(stress.regime || '-');
      const regimeColor = stress.regime === 'crisis' ? 'var(--down)' : (stress.regime === 'high' ? 'var(--warn)' : 'var(--up)');
      const comps = stress.components || {};
      const stressTime = stress.timestamp ? new Date(stress.timestamp * 1000).toLocaleString('zh-TW') : new Date().toLocaleString('zh-TW');
      let html = `<div class="mb-sm text-muted text-sm">資料更新時間：${stressTime}</div>`;
      html += `<div style="font-size:28px;font-weight:700;color:${regimeColor}">${score} <span style="font-size:14px;color:var(--muted)">/ 100</span></div>`;
      html += `<div style="margin:4px 0 10px;font-size:14px">出逃等級：<strong>${stressLabel(stress.regime || '-')}</strong>（${stress.score >= 70 ? '🔴 紅燈' : (stress.score >= 50 ? '🟠 橙燈' : (stress.score >= 30 ? '🟡 黃燈' : '🟢 綠燈'))}）</div>`;
      html += `<table><thead><tr><th>子項</th><th>壓力貢獻 <span class="cursor-pointer text-accent" data-help="<p><strong>分數代表什麼？</strong></p><p>外商出逃指數由六個因子加權構成。每個子項的分數代表該因子對「外商撤離台灣」這個現象的貢獻度。</p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>分數越高</strong>：該因子越可能導致外商賣超台股（例如美元走強→外商匯出獲利、VIX飆升→全球避險情緒）。</li><li><strong>分數為 0</strong>：該因子目前沒有施壓（例如外商買超時，外商流向因子為 0）。</li><li><strong>所有子項皆為正值</strong>：指數只累加壓力，不扣除「助力」。這是單向指標。</li></ul><p><strong>為什麼是單向指標？</strong><br>因為外商買超時，系統不會顯示「負壓力」，而是讓總分維持低位（綠燈）。這樣設計是為了突出「危險訊號」，而非平衡呈現多空。</p><p><strong>總分區間意義：</strong><br>• 0-29分（綠燈）：外商流出壓力小，資金面寬鬆<br>• 30-49分（黃燈）：外商開始流出，注意波動<br>• 50-69分（橙燈）：外商明顯出逃，台股下跌機率高<br>• 70-100分（紅燈）：外商大量出逃，系統性風險高</p>" data-title="外商出逃指數分數說明">ℹ️</span></th></tr></thead><tbody>`;
      const names = { dxy: 'DXY-美元指數', us10y: 'US10Y-美債10年期', foreign_flow: '外資流向', vix: 'VIX-波動率指數', jpy: '日圓-套利平倉壓力', geopolitical: '地緣政治風險' };
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
    const sortedList = list.slice().sort((a, b) => {
      const strengthA = Math.abs(a.sentiment || 0) * (a.confidence || 1);
      const strengthB = Math.abs(b.sentiment || 0) * (b.confidence || 1);
      return strengthB - strengthA;
    });
    if (!sortedList.length) { eventsEl.innerHTML = renderEmptyState('目前無觸發的宏觀敘事', ''); }
    else {
      eventsEl.innerHTML = sortedList.map(e => {
        const sClass = e.sentiment > 0 ? 'up' : 'down';
        const sText = e.sentiment > 0 ? '正面' : '負面';
        const tw = timeWindowName(e.time_window || '-');
        return `<div style="border-left:3px solid var(--accent);padding:10px 12px;margin:8px 0;background:#0d1015;border-radius:8px">
          <div class="font-bold">${escapeHtml(eventName(e.theme))} <span class="${sClass}">${sText} (${e.sentiment})</span></div>
          <div class="text-muted text-sm mt-xs">區域：${escapeHtml(regionName(e.region))} · 信心度：${((e.confident || e.confidence || 0) * 100).toFixed(0)}% · 資金流：${escapeHtml(capitalFlowName(e.capital_flow || '-'))} · 時間窗口：${escapeHtml(tw)}</div>
        </div>`;
      }).join('');
    }
  }

  const chainsEl = document.getElementById('narrativeChains');
  if (chainsEl) {
    chainsEl.classList.remove('loading');
    const list = (chains && chains.chains) || [];
    if (!list.length) { chainsEl.innerHTML = renderEmptyState('目前無匹配的因果鏈', ''); }
    else {
      chainsEl.innerHTML = list.map(c => `
        <div style="margin:10px 0">
          <div style="font-weight:700;margin-bottom:4px">${escapeHtml(templateName(c.template_id))} <span class="text-muted text-sm">分數 ${(c.score || 0).toFixed(3)}</span></div>
          ${(c.steps || []).map((s, i) => {
            const impClass = s.impact > 0 ? 'up' : 'down';
            const affected = (s.affected || []).map(a => `<span style="display:inline-block;font-size:11px;padding:2px 8px;margin:2px 4px 2px 0;border-radius:999px;background:var(--bg);color:var(--muted);border:1px solid var(--border)">${escapeHtml(sectorName(a) || a)}</span>`).join('');
            return `<div class="py-sm border-b-dashed">
              <div class="flex-center-gap-sm">
                <div class="layer-num">${i+1}</div>
                <div style="flex:1;font-size:12px">${escapeHtml(s.description)}</div>
                <div class="${impClass}" style="font-size:11px;padding:2px 8px;border-radius:999px;background:#0d1015;flex-shrink:0">影響力 ${s.impact}</div>
              </div>
              ${affected ? `<div style="margin-top:6px;padding-left:30px">${affected}</div>` : ''}
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
    if (!list.length) { modelsEl.innerHTML = renderEmptyState('目前無活躍模型', ''); }
    else {
      modelsEl.innerHTML = list.map((m, idx) => {
        const w = m.weight || 0;
        const e = m.recent_error || 0;
        const weightPct = Math.min(100, Math.max(0, w * 100)).toFixed(1);
        const weightColor = w >= 0.5 ? '#10b981' : (w >= 0.25 ? '#f59e0b' : '#ef4444');
        const errColor = e <= 0.3 ? '#10b981' : (e <= 0.5 ? '#f59e0b' : '#ef4444');
        const errText = e <= 0.3 ? '優秀' : (e <= 0.5 ? '一般' : '偏高');
        return `
        <div class="weight-panel" style="border-left-color:${weightColor}">
          <div style="display:flex;justify-content:space-between;align-items:center;gap:10px">
            <div style="font-weight:700;font-size:15px">${escapeHtml(modelName(m.name))}</div>
            <div style="font-size:12px;color:var(--muted);white-space:nowrap">誤差 <span style="color:${errColor};font-weight:700">${(e*100).toFixed(1)}% ${errText}</span></div>
          </div>
          <div style="margin:8px 0">
            <div style="display:flex;align-items:center;gap:8px;font-size:12px;color:var(--muted)">
              <span>模型權重</span>
              <div style="flex:1;height:6px;background:#1f2937;border-radius:3px;overflow:hidden">
                <div style="width:${weightPct}%;height:100%;background:${weightColor}"></div>
              </div>
              <span class="min-w-40 text-right">${w.toFixed(3)}</span>
            </div>
          </div>
          <div class="text-muted text-sm mt-xs">${escapeHtml(m.description || '')}</div>
          <div style="display:flex;flex-wrap:wrap;gap:4px;margin-top:8px">
            ${(m.favored_sectors || []).map(s => `<span class="badge ok">+ ${escapeHtml(sectorName(s))}</span>`).join('')}
            ${(m.avoided_sectors || []).map(s => `<span class="badge err">− ${escapeHtml(sectorName(s))}</span>`).join('')}
          </div>
          ${m.rationale ? `<div class="mt-sm">
            <button onclick="document.getElementById('model-rationale-${idx}').style.display=document.getElementById('model-rationale-${idx}').style.display==='none'?'block':'none';this.textContent=this.textContent==='展開完整論述 ▼'?'收起論述 ▲':'展開完整論述 ▼'" style="font-size:12px;color:var(--accent);background:none;border:none;padding:0;cursor:pointer">展開完整論述 ▼</button>
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
    if (!items.length) { templatesEl.innerHTML = renderEmptyState('無模板資料', ''); }
    else {
      templatesEl.innerHTML = `<table>
        <thead><tr><th>模板名稱</th><th>觸發主題</th><th>歷史命中率</th><th>資料來源</th><th class="w-90">操作</th></tr></thead>
        <tbody>
          ${items.map((t, idx) => `<tr>
            <td>${escapeHtml(templateName(t.name))}</td>
            <td>${escapeHtml(eventName(t.trigger_theme))}</td>
            <td>${((t.historical_hit_rate || 0) * 100).toFixed(0)}%</td>
            <td class="text-muted text-xs">${escapeHtml((t.source_references || []).join(', '))}</td>
            <td><button onclick="document.getElementById('tmpl-rationale-${idx}').style.display=document.getElementById('tmpl-rationale-${idx}').style.display==='none'?'table-row':'none';this.textContent=this.textContent==='展開 ▼'?'收起 ▲':'展開 ▼'" style="font-size:11px;padding:3px 8px;border-radius:4px;border:1px solid var(--accent);background:transparent;color:var(--accent);cursor:pointer">展開 ▼</button></td>
          </tr>
          <tr id="tmpl-rationale-${idx}" class="hidden">
            <td colspan="5" style="background:var(--bg);padding:12px 14px;font-size:12px;line-height:1.8;color:var(--text);white-space:pre-wrap;border-left:3px solid var(--accent)">${escapeHtml(t.rationale || '暫無論述')}</td>
          </tr>`).join('')}
        </tbody>
      </table>`;
    }
  }

  const retailEl = document.getElementById('narrativeRetailSentiment');
  if (retailEl) {
    retailEl.classList.remove('loading');
    if (!retailSentiment) { retailEl.innerHTML = renderEmptyState('無散戶情緒資料', ''); }
    else {
      const hasValidData = retailSentiment.margin_balance > 0;
      const readingMap = { frenzy: '狂熱', neutral: '中性', fear: '恐慌' };
      const readingClass = retailSentiment.extreme_reading === 'frenzy' ? 'err' : retailSentiment.extreme_reading === 'fear' ? 'warn' : 'ok';
      const score = (retailSentiment.sentiment_score || 0).toFixed(2);
      const changeStr = (retailSentiment.margin_change_pct || 0) >= 0 ? '+' + (retailSentiment.margin_change_pct * 100).toFixed(1) + '%' : (retailSentiment.margin_change_pct * 100).toFixed(1) + '%';
      const changeClass = (retailSentiment.margin_change_pct || 0) >= 0 ? 'up' : 'down';
      const dataStatusBadge = hasValidData
        ? '<span class="badge ok">🟢 資料正常</span>'
        : '<span class="badge">🟡 資料延遲</span>';

      const sentimentScore = retailSentiment.sentiment_score || 0;
      const marginChange = retailSentiment.margin_change_pct || 0;
      const marginBalance = retailSentiment.margin_balance || 0;
      const dayTradingRatio = retailSentiment.day_trading_ratio || 0;
      const marginPercentile = (retailSentiment.margin_percentile || 0) * 100;

      const sentimentHelp = `綜合融資餘額變化、當沖比率、散戶交易行為等指標計算出的散戶市場情緒指標。\\n\\n分數範圍：-1.0 ~ +1.0\\n• ＞+0.5（狂熱）：散戶過度樂觀，融資大增、當沖猖獗，市場可能接近短期頂部\\n• 0.0 ~ +0.5（偏多）：散戶積極參與，市場熱絡但尚未過熱\\n• -0.5 ~ 0.0（偏空）：散戶趨於保守，融資減少，市場觀望氣氛濃厚\\n• ＜-0.5（恐慌）：散戶極度悲觀，恐慌砍倉，歷史上常是階段性底部訊號\\n\\n當前數值：${score} — ${sentimentScore > 0.5 ? '市場狂熱，建議減碼' : sentimentScore > 0 ? '散戶偏多' : sentimentScore > -0.5 ? '散戶偏空觀望' : '市場恐慌，可能接近底部'}`;

      const marginChangeHelp = `融資餘額相對前一交易日的變化百分比。融資是散戶向券商借錢買股票的行為，是觀察散戶槓桿程度的重要指標。\\n\\n解讀標準：\\n• ＞+5%：散戶瘋狂加碼，槓桿急速攀升，系統性風險劇增\\n• +2% ~ +5%：散戶積極加槓桿，市場過熱跡象浮現\\n• -2% ~ +2%：正常波動區間，散戶情緒平穩\\n• -5% ~ -2%：散戶開始去槓桿，市場降溫\\n• ＜-5%：散戶恐慌砍倉，融資大減，常伴隨市場急跌，但也可能是底部訊號\\n\\n當前數值：${changeStr} — ${Math.abs(marginChange * 100) > 5 ? '散戶情緒劇烈波動' : Math.abs(marginChange * 100) > 2 ? '散戶情緒明顯變化' : '正常波動範圍'}`;

      const marginBalanceHelp = `全市場散戶向券商融資買股票的總金額（單位：億元）。融資餘額越高代表散戶槓桿越大，市場風險越高。\\n\\n歷史百分位解讀：\\n• ＞90th：極高水位，散戶槓桿處於歷史高檔，系統性回調風險極高\\n• 70th ~ 90th：偏高水位，市場過熱，建議逐步降低持股\\n• 30th ~ 70th：正常區間，風險可控\\n• 10th ~ 30th：偏低水位，市場冷清，但可能是佈局時機\\n• ＜10th：極低水位，散戶幾乎離場，歷史上常是長期底部區域\\n\\n當前數值：${marginBalance.toFixed(0)} 億（歷史 ${marginPercentile.toFixed(0)}th 百分位）\\n${marginPercentile > 90 ? '⚠️ 融資處於歷史極高水位，系統性風險極高，建議大幅減碼' : marginPercentile > 70 ? '⚡ 融資偏高，市場過熱，建議逐步獲利了結' : marginPercentile > 30 ? '✅ 融資水位正常，風險可控' : marginPercentile > 10 ? '💡 融資偏低，市場冷清，可關注佈局機會' : '📉 融資極低，散戶幾乎離場，可能是長期底部'}`;

      const dayTradingHelp = `當日沖銷（Day Trading）成交量占總成交量的比例。當沖是散戶在同一天內買進又賣出的交易行為，是觀察市場投機程度的重要指標。\\n\\n解讀標準：\\n• ＞40%：市場極度投機，散戶狂熱當沖，類似2021年航運股狂潮，短期崩盤風險極高\\n• 30% ~ 40%：當沖比率偏高，市場投機氣氛濃厚，注意追高空單風險\\n• 20% ~ 30%：正常偏高的當沖活動，市場熱絡但尚屬健康\\n• 15% ~ 20%：當沖比率正常，市場交易穩定\\n• ＜15%：當沖冷清，市場缺乏投機動能，散戶參與度低\\n\\n當前數值：${(dayTradingRatio * 100).toFixed(1)}% — ${dayTradingRatio * 100 > 40 ? '市場極度投機，高風險警戒！' : dayTradingRatio * 100 > 30 ? '當沖比率偏高，注意風險' : dayTradingRatio * 100 > 20 ? '當沖活躍，市場熱絡' : dayTradingRatio * 100 > 15 ? '當沖比率正常' : '當沖冷清，市場觀望'}`;

      retailEl.innerHTML = `
        <div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">
          <span class="text-muted text-sm">散戶情緒指標</span>
          <span class="badge ${readingClass}">${readingMap[retailSentiment.extreme_reading] || retailSentiment.extreme_reading}</span>
          ${dataStatusBadge}
        </div>
        <div style="display:grid;grid-template-columns:repeat(2,1fr);gap:10px">
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
            <div class="kpi-value" class="text-lg">${marginBalance.toFixed(0)} 億</div>
          </div>
          <div class="kpi-card" style="cursor:pointer;" data-help="${dayTradingHelp.replace(/"/g, '&quot;')}" data-title="當沖比率說明">
            <div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">當沖比率 ℹ️</div>
            <div class="kpi-value" class="text-lg">${(dayTradingRatio * 100).toFixed(1)}%</div>
          </div>
        </div>
        <div class="mt-sm text-muted text-sm">歷史百分位: ${marginPercentile.toFixed(0)}th</div>
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
    if (!seasonal || !seasonal.expectations || seasonal.expectations.length === 0) { seasonalEl.innerHTML = renderEmptyState('無季節性事件', ''); }
    else {
      const rows = seasonal.expectations.map(e => {
        const statusBadge = e.already_priced_in ? '<span class="badge">已反應</span>' : '<span class="badge ok">有驚喜潛力</span>';
        return `<tr><td>${escapeHtml(eventName(e.theme))}</td><td>${(e.historical_avg_return * 100).toFixed(1)}%</td><td>${(e.current_return * 100).toFixed(1)}%</td><td>${(e.expectation_gap * 100).toFixed(1)}%</td><td>${statusBadge}</td></tr>`;
      }).join('');
      seasonalEl.innerHTML = `<table><thead><tr><th>主題</th><th>歷史平均</th><th>當前報酬</th><th>預期差</th><th>狀態</th></tr></thead><tbody>${rows}</tbody></table>`;
    }
  }
}
