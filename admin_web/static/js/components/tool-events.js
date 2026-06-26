import { escapeHtml } from '../main.js';

const narrativeThemeDesc = {
  'US_rates_up':                 '美國公債殖利率上升，可能引發資金流向調整',
  'JPY_carry_unwind':            '日圓套利平倉，顯示全球流動性收緊',
  'geopolitical_risk_spike':     '地緣政治風險攀升，市場避險情緒升溫',
  'oil_price_shock':             '油價劇烈波動，影響通膨預期',
  'USD_TWD_volatility':          '美元兌台幣波動，反映台灣出口競爭力變化',
  'semiconductor_downturn':      '半導體出口下滑，景氣放緩訊號',
  'AI_capex_surge':              'AI資本支出強勁，科技股展望正面',
  'retail_frenzy':               '散戶融資餘額飆升，市場過熱風險',
  'retail_fear':                 '散戶融資餘額低迷，市場情緒低迷',
  'retail_institutional_divergence': '散戶與法人方向分歧，可能出現轉向',
};

export function renderToolEvents(container, events) {
  if (!container) return;

  if (!events || events.length === 0) {
    container.innerHTML = '<div class="empty">暫無近期事件</div>';
    return;
  }

  let html = '<div class="tool-events-list">';
  
  events.forEach((ev, index) => {
    let icon = '⚡';
    let statusClass = 'pending';
    let statusText = '未知';
    let desc = ev.description || ev.type;

    if (ev.type === 'narrative.event' || ev.type.includes('narrative')) {
      icon = '📊'; statusClass = 'complete'; statusText = '已發生';
      const p = ev.payload || {};
      const theme = p.Theme || p.theme || '';
      desc = narrativeThemeDesc[theme] || theme || '宏觀事件';
    } else if (ev.type.includes('start') || ev.type.includes('snapshot')) {
      icon = '▶️'; statusClass = 'active'; statusText = '進行中';
    } else if (ev.type.includes('complete') || ev.type.includes('outcome') || ev.type.includes('update')) {
      icon = '✅'; statusClass = 'complete'; statusText = '完成';
    } else if (ev.type.includes('error') || ev.type.includes('rejected')) {
      icon = '❌'; statusClass = 'error'; statusText = '失敗';
    } else if (ev.type.includes('recommendation') || ev.type.includes('evaluation')) {
      icon = '🤖'; statusClass = 'complete'; statusText = '完成';
    } else if (ev.type.includes('regime')) {
      icon = '🌍'; statusClass = 'complete'; statusText = '完成';
    } else if (ev.type.includes('clamping') || ev.type.includes('triggered') || ev.type.includes('alert')) {
      icon = '🛡️'; statusClass = 'active'; statusText = '觸發';
    }

    const timeStr = new Date(ev.timestamp || Date.now()).toLocaleTimeString();
    desc = escapeHtml(desc);
    const payloadStr = ev.payload ? JSON.stringify(ev.payload, null, 2) : '{}';

    const animationClass = index === 0 ? 'animate-slide-in' : '';

    html += `
      <div class="tool-event-item ${animationClass}" onclick="this.classList.toggle('expanded')">
        <div class="tool-event-header">
          <span class="tool-event-time">${timeStr}</span>
          <span class="tool-event-icon">${icon}</span>
          <span class="tool-event-desc">${desc}</span>
          <span class="tool-event-badge ${statusClass}">
            ${statusClass === 'active' ? '<span class="pulse-dot"></span>' : ''}
            ${statusText}
          </span>
        </div>
        <div class="tool-event-details">
          <pre><code>${escapeHtml(payloadStr)}</code></pre>
        </div>
      </div>
    `;
  });

  html += '</div>';
  container.innerHTML = html;
}
