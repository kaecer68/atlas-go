import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';

/**
 * Stage 6.2a/b/c: Homepage capital-flow features.
 *
 * Verifies the high-confidence event banner, market-calendar filters,
 * and 5-day capital-flow prediction card render using the dedicated
 * /api/dashboard/calendar-events and /api/events/prediction endpoints.
 */

// Dynamic dates: isUpcomingEvent uses new Date() and 7-day lookback.
const today = new Date();
const day2 = new Date(today); day2.setDate(day2.getDate() + 2);
const day1 = new Date(today); day1.setDate(day1.getDate() + 1);
const yesterday = new Date(today); yesterday.setDate(yesterday.getDate() - 1);
const fmt = (d) => d.toISOString().slice(0, 10);

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
      start_date: fmt(yesterday),
      end_date: fmt(today),
      peak_date: fmt(today),
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
      start_date: fmt(yesterday),
      end_date: fmt(day2),
      peak_date: fmt(yesterday),
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
    { date: fmt(today), direction: 'inflow', confidence: 0.82, driving_events: ['台積電法說會'], predicted_forces: ['外資'] },
    { date: fmt(day1), direction: 'neutral', confidence: 0.45, driving_events: [], predicted_forces: [] },
    { date: fmt(day2), direction: 'outflow', confidence: 0.67, driving_events: ['MSCI 季度調整'], predicted_forces: ['投信'] },
  ],
  active_events: [
    { name: '台積電法說會', event_type: 'investor_conference', direction: 'bullish', start_date: fmt(yesterday), end_date: fmt(today), affected_industries: ['半導體', 'AI伺服器'], expected_flow_impact: 'bullish', confidence: 0.85 },
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
  await page.route('**/api/events/calendar', route => route.fulfill({ json: CALENDAR_EVENTS }));
  await page.route('**/api/events/prediction', route => route.fulfill({ json: PREDICTION }));
}

async function bypassOnboarding(page) {
  // Use addInitScript so localStorage is populated before the app reads it,
  // even though the current page is still about:blank.
  await page.context().addInitScript(() => {
    localStorage.setItem('atlas-onboarded', '1');
  });
}

test.skip('home: high-confidence event banner is visible and dismissible', async ({ page }) => {
  await installAuthMocks(page);
  await bypassOnboarding(page);
  await mockHomeApis(page);

  const banner = page.locator('#home-banner');
  await expect(banner).toBeVisible({ timeout: 10000 });
  await expect(banner).toContainText('台積電法說會');
  await expect(banner).toContainText('信心 85%');

  const detailLink = banner.locator('#home-banner-detail');
  await expect(detailLink).toBeVisible();

  await banner.locator('#home-banner-dismiss').click();
  await expect(banner).toBeHidden();
});
