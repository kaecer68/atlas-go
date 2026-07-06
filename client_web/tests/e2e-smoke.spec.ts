import { test, expect } from '@playwright/test';

test.describe('Atlas E2E Smoke', () => {
  test('home page loads without console errors', async ({ page }) => {
    const errors = [];
    page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()); });
    page.on('pageerror', err => errors.push(err.message));

    await page.goto('/home', { waitUntil: 'networkidle' });
    await page.waitForSelector('#page-home', { timeout: 15000 });

    expect(errors.filter(e => !e.startsWith('Failed to load resource:'))).toEqual([]);
  });

  test('mode buttons switch content without reload', async ({ page }) => {
    await page.goto('/home', { waitUntil: 'networkidle' });
    await page.waitForSelector('.mode-btn--simple');

    await page.click('[data-mode="standard"]');
    await expect(page.locator('.advanced-only').first()).toBeVisible({ timeout: 3000 });

    await page.click('[data-mode="simple"]');
    await expect(page.locator('.advanced-only').first()).not.toBeVisible({ timeout: 3000 });

    await page.click('[data-mode="pro"]');
    await expect(page.locator('.pro-only').first()).toBeVisible({ timeout: 3000 });
  });

  test('today summary renders key indicators', async ({ page }) => {
    await page.goto('/home', { waitUntil: 'networkidle' });
    await expect(page.locator('#home-summary')).not.toHaveText('載入市場摘要…', { timeout: 10000 });
    await expect(page.locator('.home-today-indicator').first()).toBeVisible({ timeout: 5000 });
  });

  test('key navigation pages load without crash', async ({ page }) => {
    const pages = ['/home', '/live', '/crossmarket', '/portfolio'];
    for (const path of pages) {
      await page.goto(path, { waitUntil: 'networkidle' });
      await expect(page.locator('#content')).toBeVisible({ timeout: 10000 });
    }
  });
});
