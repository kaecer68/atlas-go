import { test, expect } from '@playwright/test';
import { skipIfAtlasOffline } from '../../tests-shared/atlas-check';

/**
 * Route & deep-link regression tests for client_web.
 *
 * Verifies:
 *   - evolution_panel no longer falls to 404
 *   - direct /client/<page> deep-links activate the correct page container
 *   - capital_board page container is visible
 *   - stock-quote page container is visible
 */
test.beforeAll(async () => { await skipIfAtlasOffline(test); });
test.describe.configure({ mode: 'parallel' });

test('evolution_panel deep-link does NOT fall to 404', async ({ page }) => {
  await page.goto('/client/evolution_panel');
  // The page container must exist and be active
  await expect(page.locator('#page-evolution_panel')).toBeVisible({ timeout: 10000 });
  // Title must be '策略演化', not '404'
  await expect(page.locator('#pageTitle')).toHaveText('策略演化', { timeout: 5000 });
  // Must NOT be 404 page
  const body = await page.locator('body').innerText();
  expect(body).not.toContain('404');
  expect(body).not.toContain('找不到這個頁面');
});

test('capital_board deep-link activates page container', async ({ page }) => {
  await page.goto('/client/capital_board');
  await expect(page.locator('#page-capital_board')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('#pageTitle')).toHaveText('錢潮看板', { timeout: 5000 });
});

test('stock-quote deep-link activates page container', async ({ page }) => {
  await page.goto('/client/stock-quote');
  await expect(page.locator('#page-stock-quote')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('#pageTitle')).toHaveText('個股快查', { timeout: 5000 });
});

test('strategies deep-link activates page container', async ({ page }) => {
  await page.goto('/client/strategies');
  await expect(page.locator('#page-strategies')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('#pageTitle')).toHaveText('投資心法', { timeout: 5000 });
});

test('capital_predictions deep-link activates page container', async ({ page }) => {
  await page.goto('/client/capital_predictions');
  await expect(page.locator('#page-capital_predictions')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('#pageTitle')).toHaveText('錢潮預測', { timeout: 5000 });
});

test('sidebar nav click routes to correct page', async ({ page }) => {
  await page.goto('/client/home');
  await expect(page.locator('#page-home')).toBeVisible({ timeout: 10000 });
  // Click evolution_panel nav link
  await page.click('a[data-page="evolution_panel"]');
  await expect(page.locator('#page-evolution_panel')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('#pageTitle')).toHaveText('策略演化', { timeout: 5000 });
  // Click capital_board nav link
  await page.click('a[data-page="capital_board"]');
  await expect(page.locator('#page-capital_board')).toBeVisible({ timeout: 10000 });
  await expect(page.locator('#pageTitle')).toHaveText('錢潮看板', { timeout: 5000 });
});
