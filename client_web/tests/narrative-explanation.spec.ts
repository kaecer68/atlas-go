import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';


test('narrative event card renders explanation and sentiment_explanation', async ({ page }) => {
  await installAuthMocks(page);
  await page.route('**/api/system/status', route => route.fulfill({ json: { status: "ok" } }));
  await page.route('**/api/dashboard/snapshot', route => route.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', route => route.fulfill({ json: { score: 50, regime: "high" } }));
  
  await page.route('**/api/narrative/events', route => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        events: [
          {
            theme: "THEME_A",
            explanation: "REGIME_EXPLANATION_TEXT",
            sentiment_explanation: "SENTIMENT_TEXT",
            sentiment: 0.8,
            confidence: 0.9,
            region: "tw",
            severity: "high",
            status: "active"
          },
          {
            theme: "THEME_B",
            sentiment: 0,
            confidence: 0.5,
            region: "tw",
            severity: "medium",
            status: "active"
          }
        ]
      })
    });
  });

  await page.route('**/api/narrative/chains', route => route.fulfill({ json: { chains: [] } }));
  await page.route('**/api/narrative/models', route => route.fulfill({ json: { models: [] } }));
  await page.route('**/api/narrative/templates', route => route.fulfill({ json: { templates: [] } }));
  await page.route('**/api/dashboard/retail-sentiment', route => route.fulfill({ json: {} }));
  await page.route('**/api/narrative/seasonal', route => route.fulfill({ json: { expectations: [] } }));

  await page.goto('/client/narrative');

  // Wait for the onboarding overlay, then dismiss it. The overlay
  // intercepts all pointer events so we must remove it before clicking.
  const obOverlay = page.locator('.onboard-overlay');
  await obOverlay.waitFor({ state: 'attached', timeout: 10000 }).catch(() => {});
  if (await obOverlay.isVisible().catch(() => false)) {
    await page.keyboard.press('Escape');
    await obOverlay.waitFor({ state: 'detached', timeout: 5000 }).catch(() => {});
  }


  const eventsContainer = page.locator('#narrativeEvents');
  await expect(eventsContainer).toBeVisible({ timeout: 5000 });

  // Debug: Print html if missing
  // console.log(await eventsContainer.innerHTML());

  await expect(eventsContainer).toContainText('REGIME_EXPLANATION_TEXT');
  await expect(eventsContainer).toContainText('SENTIMENT_TEXT');
  
  const details = await eventsContainer.locator('details').allTextContents();
  expect(details.length).toBe(2);
  expect(details.some(text => text.includes('LLM 解析') && text.includes('REGIME_EXPLANATION_TEXT'))).toBe(true);
  expect(details.some(text => text.includes('情緒解釋') && text.includes('SENTIMENT_TEXT'))).toBe(true);
});
