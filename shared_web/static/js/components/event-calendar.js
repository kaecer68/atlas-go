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

export async function renderEventCalendar(container) {
  container.innerHTML = '<div class="home-loading-card">載入市場行事曆…</div>';

  try {
    const data = await silentGetJSON('/api/dashboard/calendar-events');
    const events = data && Array.isArray(data.events) ? data.events : [];

    if (!events.length) {
      container.innerHTML = '<div class="home-signal-empty">目前無近期市場事件</div>';
      return;
    }

    // Sort by start_date ascending, active events first
    const sorted = [...events].sort((a, b) => {
      if (a.active !== b.active) return a.active ? -1 : 1;
      return new Date(a.start_date) - new Date(b.start_date);
    });

    container.innerHTML = `
      <div class="cal-grid">
        ${sorted.map(renderEventCard).join('')}
      </div>
    `;
  } catch (err) {
    console.warn('[event-calendar] failed to load:', err);
    container.innerHTML = '<div class="home-signal-empty">行事曆資料暫時無法載入</div>';
  }
}
