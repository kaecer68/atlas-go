/**
 * Event Calendar component — renders upcoming Taiwan market calendar events
 * from /api/dashboard/calendar-events (EventCalendar engine, wired via dashboard_api.go).
 */

import { silentGetJSON } from '../shared/app-utils.js';
import { financialColor } from '../shared/color-tokens.js';

const EVENT_TYPE_LABELS = {
  ex_dividend: '除權息',
  shareholder_meeting: '股東會',
  spring_festival: '春節',
  window_dressing: '作帳行情',
  election: '選舉',
  msci_rebalance: 'MSCI 調整',
  financial_report: '財報公布',
  investor_conference: '法說會',
  monthly_revenue: '營收公布',
  long_holiday: '長假',
  dividend_payout: '股利發放',
  taiwan50_rebalance: '0050 調整',
  futures_settlement: '期貨結算',
  position_building: '法人布局',
};

const DIRECTION_ICONS = {
  bullish: '📈',
  bearish: '📉',
  mixed: '⚖️',
  neutral: '➖',
};

// Default calendar window: ±15 days from today
const CALENDAR_WINDOW_DAYS = 15;

function daysFromToday(dateStr) {
  const d = new Date(dateStr);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return Math.round((d - today) / (1000 * 60 * 60 * 24));
}

function isWithinWindow(event, windowDays) {
  const sd = daysFromToday(event.start_date);
  const ed = daysFromToday(event.end_date);
  // Show if event starts within window or spans the window
  return (sd >= -windowDays && sd <= windowDays) || (ed >= -windowDays && ed <= windowDays) || (sd <= -windowDays && ed >= windowDays);
}

function fmtDateRange(start, end) {
  const s = new Date(start);
  const e = new Date(end);
  const mmdd = (d) => `${d.getMonth() + 1}/${d.getDate()}`;
  return `${mmdd(s)}–${mmdd(e)}`;
}

function renderEventCard(evt) {
  const direction = evt.direction || 'neutral';
  const icon = DIRECTION_ICONS[direction] || '';
  const typeLabel = EVENT_TYPE_LABELS[evt.event_type] || evt.event_type;
  const activeClass = evt.active ? 'cal-card--active' : '';

  const industries = Array.isArray(evt.affected_industries) && evt.affected_industries.length > 0
    ? `<div class="cal-card__industries">${evt.affected_industries.slice(0, 3).map(id => `<span class="cal-tag">${id}</span>`).join('')}</div>`
    : '';

  return `
    <div class="cal-card ${activeClass}" title="${evt.description || ''}">
      <div class="cal-card__header">
        <span class="cal-card__type">${typeLabel}</span>
        <span class="cal-card__direction" style="color:${financialColor(direction === 'bullish' ? 1 : direction === 'bearish' ? -1 : 0, 'trend')}">${icon}</span>
      </div>
      <div class="cal-card__name">${evt.name}</div>
      <div class="cal-card__date">${fmtDateRange(evt.start_date, evt.end_date)}</div>
      ${industries}
    </div>
  `;
}

function partitionEvents(events) {
  const visible = [];
  const hidden = [];
  const today = new Date();
  today.setHours(0, 0, 0, 0);

  for (const evt of events) {
    const sd = new Date(evt.start_date);
    const ed = new Date(evt.end_date);
    const dayStart = Math.round((sd - today) / (1000 * 60 * 60 * 24));
    const dayEnd = Math.round((ed - today) / (1000 * 60 * 60 * 24));

    if (ed < today) {
      hidden.push(evt);
    } else if (dayStart >= -CALENDAR_WINDOW_DAYS && dayStart <= CALENDAR_WINDOW_DAYS) {
      visible.push(evt);
    } else if (dayEnd >= -CALENDAR_WINDOW_DAYS && dayEnd <= CALENDAR_WINDOW_DAYS) {
      visible.push(evt);
    } else if (dayStart <= -CALENDAR_WINDOW_DAYS && dayEnd >= CALENDAR_WINDOW_DAYS) {
      visible.push(evt);
    } else {
      hidden.push(evt);
    }
  }
  return { visible, hidden };
}

function sortEvents(events) {
  return [...events].sort((a, b) => {
    if (a.active !== b.active) return a.active ? -1 : 1;
    return new Date(a.start_date) - new Date(b.start_date);
  });
}

export async function renderEventCalendar(container) {
  container.innerHTML = '<div class="home-loading-card">載入市場行事曆…</div>';

  try {
    const data = await silentGetJSON('/api/dashboard/calendar-events');
    const events = data && Array.isArray(data.events) ? data.events : [];

    if (!events.length) {
      container.innerHTML = '<div class="home-signal-empty">目前無近期市場事件</div>';
      return;
    }

    const { visible, hidden } = partitionEvents(events);
    const sortedVisible = sortEvents(visible);

    const buildGrid = (evts) => `<div class="cal-grid">${evts.map(renderEventCard).join('')}</div>`;

    const expandHTML = hidden.length > 0
      ? `<button class="btn btn--secondary cal-expand-btn" id="cal-expand-toggle">
         展開全部（+${hidden.length} 個事件）
       </button>`
      : '';

    const allHiddenHTML = hidden.length > 0
      ? `<div id="cal-hidden-events" style="display:none">
          <div class="cal-section-label">其他事件</div>
          ${buildGrid(sortEvents(hidden))}
         </div>`
      : '';

    container.innerHTML = buildGrid(sortedVisible) + allHiddenHTML + expandHTML;

    if (hidden.length > 0) {
      const btn = document.getElementById('cal-expand-toggle');
      const hiddenDiv = document.getElementById('cal-hidden-events');
      let expanded = false;
      btn.addEventListener('click', () => {
        expanded = !expanded;
        hiddenDiv.style.display = expanded ? 'block' : 'none';
        btn.textContent = expanded ? '收起' : `展開全部（+${hidden.length} 個事件）`;
      });
    }
  } catch (err) {
    console.warn('[event-calendar] failed to load:', err);
    container.innerHTML = '<div class="home-signal-empty">行事曆資料暫時無法載入</div>';
  }
}
