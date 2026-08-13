/**
 * capital-pages.spec.mjs — Frontend E2E: 錢潮模型 ↔ 錢潮因果 表裡互鏈
 *
 * 驗證（Wave 6）：
 *   1. capital_models 頁點 theme chip → 切到 capital_causality 且已篩選該 theme
 *   2. capital_causality 頁 active template 顯示「對應模型」badge，點擊切回 capital_models
 *
 * To run:
 *   node tests/frontend-e2e/capital-pages.spec.mjs
 *
 * Prerequisites:
 *   - playwright installed (npm install --no-save playwright)
 *   - Mock server on port 8001 (node tests/frontend-e2e/mock-server.mjs)
 *     OR real atlas on :18080 (BASE_URL override)
 */

import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const DIR = path.dirname(fileURLToPath(import.meta.url));
const SCREENSHOT_DIR = path.join(DIR, 'screenshots');
const BASE_URL = process.env.BASE_URL || 'http://localhost:8001';
const TIMEOUT = 15000;

// navigateToAdmin routes the SPA to an admin page and waits for its content.
async function navigateToAdmin(page, pageId, contentId) {
  await page.goto(`${BASE_URL}/admin/`, { waitUntil: 'load', timeout: TIMEOUT });
  await page.waitForFunction(() => window.switchPage !== undefined, {}, { timeout: TIMEOUT });
  await page.evaluate((id) => window.switchPage(id, true), pageId);
  try {
    await page.waitForSelector(contentId, { timeout: TIMEOUT });
    return true;
  } catch (e) {
    console.warn(`  ⚠ ${contentId} not rendered`);
    return false;
  }
}

async function themeChipToCausality(browser) {
  console.log('\n=== Test 1: models theme chip → causality filtered ===');
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1280, height: 800 });
  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));

  const ok = await navigateToAdmin(page, 'capital_models', '#capitalModelsContent');
  if (!ok) {
    await page.close();
    return { errors, passed: false };
  }

  // Theme chips live inside the collapsible card detail — expand the first
  // card so the chips become visible and clickable.
  try {
    await page.waitForSelector('.cm-card', { timeout: TIMEOUT });
    await page.$eval('.cm-card', (card) => card.click());
    await page.waitForSelector('.cm-theme-chip', { state: 'visible', timeout: TIMEOUT });
    console.log('  ✓ .cm-theme-chip visible after expanding card');
  } catch (e) {
    console.warn('  ⚠ no visible theme chip');
  }

  // Click the first theme chip and remember which theme it carried.
  const chipCount = await page.$$('.cm-theme-chip');
  if (chipCount.length === 0) {
    await page.close();
    return { errors, passed: false };
  }
  const clickedTheme = await chipCount[0].getAttribute('data-theme');
  await chipCount[0].click();
  console.log(`  clicked first theme chip (${clickedTheme})`);

  // Expect SPA to switch to capital_causality and load it.
  try {
    await page.waitForSelector('#capitalCausalityContent', { timeout: TIMEOUT });
    console.log('  ✓ switched to capital_causality');
  } catch (e) {
    console.warn('  ⚠ capital_causality content not rendered after chip click');
  }

  // The filter select should be set to the clicked theme.
  let filterValue = null;
  try {
    filterValue = await page.$eval('#cc-theme-filter', (sel) => sel.value);
  } catch (e) { /* select may not exist */ }
  console.log(`  filter value: ${filterValue}`);
  const filtered = filterValue === clickedTheme;

  const ssPath = path.join(SCREENSHOT_DIR, 'capital-models-to-causality.png');
  await page.screenshot({ path: ssPath, fullPage: true });
  await page.close();
  return { errors, passed: filtered };
}

async function modelBadgeToModels(browser) {
  console.log('\n=== Test 2: causality model badge → models page ===');
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1280, height: 800 });
  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));

  const ok = await navigateToAdmin(page, 'capital_causality', '#capitalCausalityContent');
  if (!ok) {
    await page.close();
    return { errors, passed: false };
  }

  // The AI_capex_surge template has a corresponding model in the mock →
  // a .cc-model-badge should render.
  try {
    await page.waitForSelector('.cc-model-badge', { timeout: TIMEOUT });
    console.log('  ✓ .cc-model-badge (對應模型) found');
  } catch (e) {
    console.warn('  ⚠ no 對應模型 badge rendered');
    const body = await page.textContent('body');
    console.log(`  body preview: ${body.slice(0, 200)}`);
  }

  const badges = await page.$$('.cc-model-badge');
  if (badges.length === 0) {
    await page.close();
    return { errors, passed: false };
  }
  await badges[0].click();
  console.log('  clicked 對應模型 badge');

  try {
    await page.waitForSelector('#capitalModelsContent', { timeout: TIMEOUT });
    console.log('  ✓ switched back to capital_models');
    await page.close();
    return { errors, passed: true };
  } catch (e) {
    console.warn('  ⚠ capital_models content not rendered after badge click');
    await page.close();
    return { errors, passed: false };
  }
}

async function main() {
  try {
    const browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox'],
    });
    const r1 = await themeChipToCausality(browser);
    const r2 = await modelBadgeToModels(browser);
    await browser.close();

    const allPassed = r1.passed && r2.passed && r1.errors.length === 0 && r2.errors.length === 0;
    console.log(`\n══════════════════════════════════════════`);
    console.log(`  Verdict: ${allPassed ? 'PASS' : 'FAIL'}`);
    console.log(`  theme chip → causality filtered: ${r1.passed ? '✓' : '✗'}`);
    console.log(`  model badge → models page: ${r2.passed ? '✓' : '✗'}`);
    console.log(`  Page errors: ${r1.errors.length + r2.errors.length}`);
    console.log(`══════════════════════════════════════════\n`);
    process.exit(allPassed ? 0 : 1);
  } catch (err) {
    console.error('\n❌ Capital pages test FAILED:', err.message);
    process.exit(1);
  }
}

main();
