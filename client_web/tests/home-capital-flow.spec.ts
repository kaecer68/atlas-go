import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';


/**
 * Stage 6.2a/b/c: Homepage capital-flow features.
 *
 * Verifies the high-confidence event banner, market-calendar filters,
 * and 5-day capital-flow prediction card render using the dedicated
 * /api/dashboard/calendar-events and /api/events/prediction endpoints.
 */

const CALENDAR_EVENTS = {
  events: [
    {
      id: 'evt-001',
      name: '台積電法說會',
      event_type: 'investor_conference',
      description: 'Q2 財報與下半年展望',
      direction: 'bullish',
      base_weight: 0.85,
      active: true,
      start_date: '2026-07-16',
      end_date: '2026-07-16',
      peak_date: '2026-07-16',
      decay_days: 1,
      affected_industries: ['半導體', 'AI伺服器'],
      sentiment_adjustment: 0.1,
      data_source: 'twse',
      evidence_quality: 'high',
      generated_at: new Date().toISOString(),
    },
    {
      id: 'evt-002',
      name: 'MSCI 季度調整',
      event_type: 'msci_rebalance',
      description: 'MSCI Taiwan 成分股調整生效',
      direction: 'mixed',
      base_weight: 0.55,
      active: true,
      start_date: '2026-07-18',
      end_date: '2026-07-18',
      peak_date: '2026-07-18',
      decay_days: 1,
      affected_industries: ['金融', '電子零組件'],
      sentiment_adjustment: 0,
      data_source: 'msci',
      evidence_quality: 'medium',
      generated_at: new Date().toISOString(),
    },
  ],
  count: 2,
};

const PREDICTION = {
  generated_at: new Date().toISOString(),
  window: '5-day forward',
  predictions: [
    { date: '2026-07-16', direction: 'inflow', confidence: 0.82, driving_events: ['台積電法說會'], predicted_forces: ['外資'] },
    { date: '2026-07-17', direction: 'neutral', confidence: 0.45, driving_events: [], predicted_forces: [] },
    { date: '2026-07-18', direction: 'outflow', confidence: 0.67, driving_events: ['MSCI 季度調整'], predicted_forces: ['投信'] },
    { date: '2026-07-19', direction: 'inflow', confidence: 0.71, driving_events: ['台積電法說會'], predicted_forces: ['外資'] },
    { date: '2026-07-20', direction: 'neutral', confidence: 0.38, driving_events: [], predicted_forces: [] },
  ],
  active_events: [
    { name: '台積電法說會', event_type: 'investor_conference', direction: 'bullish', start_date: '2026-07-16', end_date: '2026-07-16', affected_industries: ['半導體', 'AI伺服器'], expected_flow_impact: 'bullish', confidence: 0.85 },
  ],
  summary: '事件驅動資金流預測：法說會主導短期流入',
};

async function mockHomeApis(page) {
  await page.route('**/api/system/status', route => route.fulfill({ json: { status: 'ok' } }));
  await page.route('**/api/dashboard/snapshot', route => route.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', route => route.fulfill({ json: { score: 50, regime: 'high' } }));
  await page.route('**/api/dashboard/retail-sentiment', route => route.fulfill({ json: {} }));
  await page.route('**/api/dashboard/regime-history', route => route.fulfill({ json: { history: [] } }));
  await page.route('**/api/dashboard/system-health', route => route.fulfill({ json: { status: 'ok' } }));
  await page.route('**/api/macro/snapshot/latest', route => route.fulfill({ json: { taiex: { value: 23000, change_pct: 0.2 } } }));
  await page.route('**/api/dashboard/recommendation-pipeline', route => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/narrative/bundle', route => route.fulfill({ json: { events: [], chains: [], models: [], templates: [] } }));
  await page.route('**/api/dashboard/portfolio-state', route => route.fulfill({ json: { positions: [] } }));
  await page.route('**/api/dashboard/calendar-events', route => route.fulfill({ json: CALENDAR_EVENTS }));
  await page.route('**/api/events/prediction', route => route.fulfill({ json: PREDICTION }));
}

async function bypassOnboarding(page) {
  await page.addInitScript(() => {
    localStorage.setItem('atlas-onboarded', '1');
  });
}

test('home: high-confidence event banner is visible and dismissible', async ({ page }) => {
  await installAuthMocks(page);
  await bypassOnboarding(page);
  await mockHomeApis(page);
  await page.goto('/');

  const banner = page.locator('#home-banner');
  await expect(banner).toBeVisible({ timeout: 5000 });
  await expect(banner).toContainText('台積電法說會');
  await expect(banner).toContainText('信心 85%');

  const detailLink = banner.locator('#home-banner-detail');
  await expect(detailLink).toBeVisible();

  await banner.locator('#home-banner-dismiss').click();
  await expect(banner).toBeHidden();
});

test('home: market calendar renders events, filters, and confidence badges', async ({ page }) => {
  await bypassOnboarding(page);
  await mockHomeApis(page);
  await page.goto('/');

  const calendarSection = page.locator('#home-event-calendar');
  await expect(calendarSection).toBeVisible();
  await expect(calendarSection).toContainText('台積電法說會');
  await expect(calendarSection).toContainText('信心 85%');

  // Filters are present
  await expect(calendarSection.locator('#cal-filter-trigger-theme')).toBeVisible();
  await expect(calendarSection.locator('#cal-filter-sector')).toBeVisible();
  await expect(calendarSection.locator('#cal-filter-start')).toBeVisible();
  await expect(calendarSection.locator('#cal-filter-end')).toBeVisible();

  // Sector filter should hide non-matching event
  await calendarSection.locator('#cal-filter-sector').selectOption('金融');
  await expect(calendarSection).toContainText('MSCI 季度調整');
  await expect(calendarSection).not.toContainText('台積電法說會');
});

test('home: 5-day capital flow prediction card renders 5 dates with bars', async ({ page }) => {
  await installAuthMocks(page);
  await bypassOnboarding(page);
  await mockHomeApis(page);
  await page.goto('/');

  const predSection = page.locator('#home-predictions');
  await expect(predSection).toBeVisible();

  const rows = predSection.locator('.pred-row');
  await expect(rows).toHaveCount(5);

  // First day: inflow
  await expect(rows.nth(0)).toContainText('資金流入');
  await expect(rows.nth(0)).toContainText('82%');
  await expect(rows.nth(0)).toContainText('台積電法說會');

  // Outflow day
  await expect(rows.nth(2)).toContainText('資金流出');
  await expect(rows.nth(2)).toContainText('67%');
});
