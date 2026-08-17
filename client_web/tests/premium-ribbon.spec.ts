import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';

/**
 * Premium 推薦 角標 ribbon 回歸測試（UX backlog: admin 右上角紅塊 之配套）。
 *
 * 修法把 auth-form.css 的角標 ribbon 樣式從裸 `.tier-badge` 收斂為
 * `.tier-card .tier-badge`。此測試確保 premium 卡片右上角的「推薦」
 * 角標仍然錨定在卡片上（position:absolute 於 .tier-card 內），
 * 而不是被一般化後失去角標定位。
 */
test('premium 推薦 badge stays anchored to the premium card corner', async ({ page }) => {
  await installAuthMocks(page);

  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.evaluate(() => window.switchPage('premium'));

  const ribbon = page.locator('.tier-card.tier-premium .tier-badge');
  await expect(ribbon).toHaveText('推薦', { timeout: 5000 });
  await expect(ribbon).toBeVisible();

  const ribbonBox = await ribbon.boundingBox();
  const cardBox = await page.locator('.tier-card.tier-premium').boundingBox();
  expect(ribbonBox).not.toBeNull();
  expect(cardBox).not.toBeNull();

  // 角標錨定在卡片右上角附近（top:-12px / right:16px 設計意圖）
  expect(ribbonBox!.y).toBeLessThan(cardBox!.y + 12);
  expect(ribbonBox!.x + ribbonBox!.width).toBeGreaterThan(cardBox!.x + cardBox!.width - 48);
  // 且不能飄到頁面最頂部（回歸舊 bug：裸 .tier-badge 全站 absolute 的形態）
  expect(ribbonBox!.y).toBeGreaterThan(40);
});
