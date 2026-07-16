/**
 * Event Calendar component — renders upcoming Taiwan market calendar events
 * from /api/dashboard/calendar-events (EventCalendar engine, wired via dashboard_api.go).
 */

import { silentGetJSON } from '../shared/app-utils.js';
import { financialColor } from '../shared/color-tokens.js';
import { displayZH as sectorLabel } from '../shared/sector-display.js';

export const EVENT_TYPE_LABELS = {
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

// Category mapping: event_type → filter category
const EVENT_TYPE_CATEGORIES = {
  ex_dividend: '除權息',
  dividend_payout: '除權息',
  financial_report: '法說會/財報',
  investor_conference: '法說會/財報',
  monthly_revenue: '法說會/財報',
  shareholder_meeting: '其他',
  spring_festival: '其他',
  window_dressing: '其他',
  election: '總經事件',
  msci_rebalance: '總經事件',
  taiwan50_rebalance: '總經事件',
  futures_settlement: '總經事件',
  position_building: '總經事件',
  long_holiday: '其他',
};

const DEFAULT_VISIBLE_LIMIT = 7;

const DIRECTION_ICONS = {
  bullish: '偏多',
  bearish: '偏空',
  mixed: '中性',
  neutral: '觀望',
};

// Default calendar window: ±15 days from today
const CALENDAR_WINDOW_DAYS = 15;

function getCategory(event) {
  return EVENT_TYPE_CATEGORIES[event.event_type] || '其他';
}

function filterByCategory(events, activeCategories) {
  if (!activeCategories || activeCategories.length === 0) return events;
  return events.filter(evt => activeCategories.includes(getCategory(evt)));
}

function filterBySectors(events, activeSectors) {
  if (!activeSectors || activeSectors.length === 0) return events;
  return events.filter(evt => {
    const inds = Array.isArray(evt.affected_industries) ? evt.affected_industries : [];
    return activeSectors.some(s => inds.includes(s));
  });
}

function filterByTriggerThemes(events, activeThemes) {
  if (!activeThemes || activeThemes.length === 0) return events;
  return events.filter(evt => activeThemes.includes(evt.trigger_theme || evt.event_type));
}

function collectTriggerThemes(events) {
  const set = new Set();
  for (const evt of events) {
    if (evt.trigger_theme) set.add(evt.trigger_theme);
    else if (evt.event_type) set.add(evt.event_type);
  }
  return Array.from(set).sort();
}

function filterByDateRange(events, range) {
  if (!range) return events;
  const start = range.start ? new Date(range.start) : null;
  const end = range.end ? new Date(range.end) : null;
  if (start && Number.isNaN(start.getTime())) return events;
  if (end && Number.isNaN(end.getTime())) return events;
  return events.filter(evt => {
    const sd = new Date(evt.start_date);
    if (Number.isNaN(sd.getTime())) return true;
    if (start && sd < start) return false;
    if (end && sd > end) return false;
    return true;
  });
}

function eventConfidence(evt) {
  if (evt && typeof evt.confidence === 'number') return evt.confidence;
  if (evt && typeof evt.base_weight === 'number') return evt.base_weight;
  return null;
}

function collectSectors(events) {
  const set = new Set();
  for (const evt of events) {
    if (Array.isArray(evt.affected_industries)) {
      for (const s of evt.affected_industries) set.add(s);
    }
  }
  return Array.from(set).sort();
}

export function gatherCalFilterOptions(events) {
  return { triggerThemes: collectTriggerThemes(events), sectors: collectSectors(events) };
}

function sortByImportance(events) {
  return [...events].sort((a, b) => {
    if (a.active !== b.active) return a.active ? -1 : 1;
    const aDays = Math.abs(daysFromToday(a.start_date));
    const bDays = Math.abs(daysFromToday(b.start_date));
    if (aDays !== bDays) return aDays - bDays;
    const MAGNITUDE = { ex_dividend: 3, dividend_payout: 3, financial_report: 3, investor_conference: 3, monthly_revenue: 2, shareholder_meeting: 2, taiwan50_rebalance: 2, msci_rebalance: 2, futures_settlement: 1, position_building: 1, election: 1, window_dressing: 1, spring_festival: 0, long_holiday: 0 };
    const aMag = MAGNITUDE[a.event_type] ?? 0;
    const bMag = MAGNITUDE[b.event_type] ?? 0;
    if (aMag !== bMag) return bMag - aMag;
    return new Date(a.start_date) - new Date(b.start_date);
  });
}

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
  const conf = eventConfidence(evt);
  const confLabel = typeof conf === 'number' ? Math.round(conf * 100) + '%' : '';

  const industries = Array.isArray(evt.affected_industries) && evt.affected_industries.length > 0
    ? `<div class="cal-card__industries">${evt.affected_industries.slice(0, 3).map(id => `<span class="cal-tag">${sectorLabel(id)}</span>`).join('')}</div>`
    : '';

  return `
    <div class="cal-card ${activeClass}" title="${evt.description || ''}">
      <div class="cal-card__header">
        <span class="cal-card__type">${typeLabel}</span>
        <span class="cal-card__direction" style="color:${financialColor(direction === 'bullish' ? 1 : direction === 'bearish' ? -1 : 0, 'trend')}">${icon}</span>
      </div>
      <div class="cal-card__name">${evt.name}</div>
      <div class="cal-card__date">${fmtDateRange(evt.start_date, evt.end_date)}</div>
      ${confLabel ? `<div class="cal-card__confidence"><span class="cal-conf-badge">信心 ${confLabel}</span></div>` : ''}
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

function buildCalendarUrl(range) {
  const base = (typeof window !== 'undefined' && window.location && window.location.origin)
    ? window.location.origin
    : 'http://localhost';
  const url = new URL('/api/dashboard/calendar-events', base);
  if (range && range.start) url.searchParams.set('start', range.start);
  if (range && range.end) url.searchParams.set('end', range.end);
  return url.pathname + url.search;
}

export async function renderEventCalendar(container, activeCategories = [], extraFilters = {}, dateRange = {}, prefetchedData = null) {
  container.innerHTML = '<div class="home-loading-card">載入市場行事曆…</div>';

  try {
    let events = [];
    if (prefetchedData && Array.isArray(prefetchedData.events)) {
      events = prefetchedData.events;
    } else {
      const data = await silentGetJSON(buildCalendarUrl(dateRange));
      events = data && Array.isArray(data.events) ? data.events : [];
    }

    if (!events.length) {
      container.innerHTML = '<div class="home-signal-empty">目前無近期市場事件</div>';
      return;
    }

    const filtered = filterByCategory(events, activeCategories);
    const byTheme = filterByTriggerThemes(filtered, extraFilters.triggerThemes);
    const byDate = filterByDateRange(byTheme, dateRange);
    const finalEvents = filterBySectors(byDate, extraFilters.sectors);
    const { visible, hidden } = partitionEvents(finalEvents);
    const sortedVisible = sortByImportance(visible);

    const defaultVisible = sortedVisible.slice(0, DEFAULT_VISIBLE_LIMIT);
    const defaultHidden = sortedVisible.slice(DEFAULT_VISIBLE_LIMIT);
    const allHidden = [...defaultHidden, ...hidden];

    const buildGrid = (evts) => `<div class="cal-grid">${evts.map(renderEventCard).join('')}</div>`;

    const expandHTML = allHidden.length > 0
      ? `<button class="btn btn--secondary cal-expand-btn" id="cal-expand-toggle">
         展開全部（+${allHidden.length} 個事件）
       </button>`
      : '';

    const allHiddenHTML = allHidden.length > 0
      ? `<div id="cal-hidden-events" style="display:none">
          ${buildGrid(sortByImportance(allHidden))}
         </div>`
      : '';

    container.innerHTML = buildGrid(defaultVisible) + allHiddenHTML + expandHTML;

    if (allHidden.length > 0) {
      const btn = document.getElementById('cal-expand-toggle');
      const hiddenDiv = document.getElementById('cal-hidden-events');
      let expanded = false;
      btn.addEventListener('click', () => {
        expanded = !expanded;
        hiddenDiv.style.display = expanded ? 'block' : 'none';
        btn.textContent = expanded ? '收起' : `展開全部（+${allHidden.length} 個事件）`;
      });
    }
  } catch (err) {
    console.warn('[event-calendar] failed to load:', err);
    container.innerHTML = '<div class="home-signal-empty">行事曆資料暫時無法載入</div>';
  }
}
