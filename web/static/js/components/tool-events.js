import { escapeHtml } from '../main.js';

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

    if (ev.type.includes('start') || ev.type.includes('snapshot')) {
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
    const desc = escapeHtml(ev.description || ev.type);
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
