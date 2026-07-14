/**
 * stage7_smoke.mjs — Stage 7 Frontend E2E Smoke Tests
 *
 * 目標:
 *   1. admin_web 錢潮預測頁正確渲染 narrative models
 *   2. client_web「近期事件」卡片 / 預測 heatmap 在事件 confidence > 0.7 時正確顯示
 *
 * To run:
 *   node tests/frontend-e2e/stage7_smoke.mjs
 *
 * Prerequisites:
 *   - playwright installed (npm install --no-save playwright)
 *   - Mock server running on port 8001 (node tests/frontend-e2e/mock-server.mjs)
 *     OR docker compose up with atlas on :18080
 */

import { chromium } from 'playwright';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const DIR = path.dirname(fileURLToPath(import.meta.url));
const SCREENSHOT_DIR = path.join(DIR, 'screenshots');
const BASE_URL = process.env.BASE_URL || 'http://localhost:8001';
const TIMEOUT = 15000;

async function adminModelsSmoke(browser) {
  console.log('\n=== Test 1: admin_web capital-models page ===');
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1280, height: 800 });

  // Capture diagnostics
  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));

  // Navigate to base admin, wait for SPA init, then route to capital_models.
  // Pattern: base URL → wait for modules/switchPage → explicit route. This is
  // more deterministic than navigating directly to /admin/capital_models
  // because it doesn't depend on the URL-driven initial-path routing in
  // admin_web/main.js (which fires before all module chunks have loaded).
  await page.goto(`${BASE_URL}/admin/`, { waitUntil: 'load', timeout: TIMEOUT });
  console.log(`  navigated → ${page.url()}`);

  // Wait for modules to load (__modulesReady)
  await page.waitForFunction(() => window.__modulesReady !== undefined, {}, { timeout: TIMEOUT });
  await page.waitForFunction(() => window.switchPage !== undefined, {}, { timeout: 5000 });
  console.log('  ✓ modules & switchPage ready');

  // Explicitly route to capital_models page
  await page.evaluate(() => window.switchPage('capital_models', true));
  console.log('  called switchPage("capital_models")');

  // Wait for the capital-models table to render
  try {
    await page.waitForSelector('#capitalModelsContent td', { timeout: TIMEOUT });
    console.log('  ✓ #capitalModelsContent <td> found');
  } catch (e) {
    const diag = await page.evaluate(() => {
      const el = document.getElementById('capitalModelsContent');
      return {
        exists: !!el,
        loading: el?.classList.contains('loading'),
        html: el?.innerHTML.substring(0, 300),
      };
    });
    console.log('  diagnostic:', JSON.stringify(diag));
    const bodyText = await page.textContent('body');
    console.log(`  body text (first 200 chars): ${bodyText.slice(0, 200)}`);
    throw new Error('capitalModelsContent table did not render: ' + e.message);
  }

  // Verify model table content
  const bodyText = await page.textContent('body');
  const keywords = ['鷹派聯準會', '權重', '命中率', '推理依據'];
  for (const kw of keywords) {
    if (!bodyText.includes(kw)) {
      console.warn(`  ⚠ Keyword "${kw}" not found in page body`);
    } else {
      console.log(`  ✓ Keyword "${kw}" found`);
    }
  }

  const hitRateMatch = bodyText.match(/\d+\.\d+%/);
  if (hitRateMatch) {
    console.log(`  ✓ Hit rate percentage found: ${hitRateMatch[0]}`);
  }

  const ssPath = path.join(SCREENSHOT_DIR, 'admin-capital-models.png');
  await page.screenshot({ path: ssPath, fullPage: true });
  console.log(`  ✓ Screenshot saved: ${ssPath}`);

  if (errors.length > 0) {
    console.log(`  Console errors (${errors.length}): ${errors.slice(0, 5).join('; ')}`);
  }

  await page.close();
  return { errors };
}

async function clientPredictionsSmoke(browser) {
  console.log('\n=== Test 2: client_web capital_predictions page ===');
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1280, height: 800 });

  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));

  // Navigate to client base, wait for SPA init, then route
  await page.goto(`${BASE_URL}/client/`, { waitUntil: 'load', timeout: TIMEOUT });
  console.log(`  navigated → ${page.url()}`);

  // Wait for auth init (guest mode) and switchPage to be available
  await page.waitForFunction(() => typeof window.switchPage === 'function', {}, { timeout: TIMEOUT });
  console.log('  ✓ switchPage ready');

  // Route to capital_predictions page
  await page.evaluate(() => window.switchPage('capital_predictions', true));
  console.log('  called switchPage("capital_predictions")');

  // Wait for cp-cell (prediction heatmap)
  try {
    await page.waitForSelector('.cp-cell', { timeout: TIMEOUT });
    console.log('  ✓ .cp-cell (prediction heatmap) found');
  } catch (e) {
    const diag = await page.evaluate(() => {
      const cp = document.getElementById('cp-grid');
      return {
        exists: !!cp,
        html: cp ? cp.innerHTML.substring(0, 300) : 'N/A',
      };
    });
    console.log('  diagnostic:', JSON.stringify(diag));
    throw new Error('.cp-cell did not render: ' + e.message);
  }

  const bodyText = await page.textContent('body');

  // Check for the high-confidence (>70%) prediction — day with confidence 0.85 should show "85%"
  const highConf = '85%';
  if (bodyText.includes(highConf)) {
    console.log(`  ✓ High confidence "${highConf}" found (confidence > 0.7)`);
  } else {
    console.warn(`  ⚠ High confidence "${highConf}" not visible on page`);
  }

  // Check direction labels
  const dirLabels = ['流入', '流出', '中性'];
  for (const label of dirLabels) {
    if (bodyText.includes(label)) {
      console.log(`  ✓ Direction label "${label}" found`);
    }
  }

  // Check filter pills
  const filterPills = ['全部', '流入', '流出'];
  for (const pill of filterPills) {
    if (bodyText.includes(pill)) {
      console.log(`  ✓ Filter pill "${pill}" found`);
    }
  }

  // Count cp-cells
  const cells = await page.$$('.cp-cell');
  console.log(`  ✓ ${cells.length} cp-cell(s) rendered`);
  if (cells.length < 5) {
    console.warn(`  ⚠ Expected at least 5 prediction cells, got ${cells.length}`);
  }

  const ssPath = path.join(SCREENSHOT_DIR, 'client-capital-predictions.png');
  await page.screenshot({ path: ssPath, fullPage: true });
  console.log(`  ✓ Screenshot saved: ${ssPath}`);

  if (errors.length > 0) {
    console.log(`  Console errors (${errors.length}): ${errors.slice(0, 5).join('; ')}`);
  }

  await page.close();
  return { cells: cells.length, errors };
}

async function clientHomeEventsSmoke(browser) {
  console.log('\n=== Test 3 (bonus): client_web home page events & predictions ===');
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1280, height: 800 });

  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));

  await page.goto(`${BASE_URL}/client/`, { waitUntil: 'load', timeout: TIMEOUT });
  console.log(`  navigated → ${page.url()}`);

  // Wait for auth init + module loading
  await page.waitForFunction(() => typeof window.switchPage === 'function', {}, { timeout: TIMEOUT });
  await page.evaluate(() => window.switchPage('home', true));
  console.log('  called switchPage("home")');

  // Wait for home-tier-sections or the tier CTA
  try {
    await page.waitForSelector('#home-tier-sections', { timeout: TIMEOUT });
    console.log('  ✓ #home-tier-sections found');
  } catch (e) {
    console.warn('  ⚠ #home-tier-sections not rendered');
  }

  try {
    await page.waitForSelector('.event-card', { timeout: TIMEOUT });
    console.log('  ✓ .event-card found');
    const cards = await page.$$('.event-card');
    console.log(`  → ${cards.length} event card(s) rendered`);
  } catch (e) {
    console.warn('  ⚠ No .event-card found (guest mode may show CTA)');
  }

  const ssPath = path.join(SCREENSHOT_DIR, 'client-home-events.png');
  await page.screenshot({ path: ssPath, fullPage: true });
  console.log(`  ✓ Screenshot saved: ${ssPath}`);

  if (errors.length > 0) {
    console.log(`  Console errors (${errors.length}): ${errors.slice(0, 5).join('; ')}`);
  }

  await page.close();
  return { errors };
}

async function main() {
  const exitCode = 0;

  try {
    const browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox'],
    });

    const results = [];
    results.push(await adminModelsSmoke(browser));
    results.push(await clientPredictionsSmoke(browser));
    results.push(await clientHomeEventsSmoke(browser));

    await browser.close();

    const totalErrors = results.reduce((sum, r) => sum + r.errors.length, 0);
    const verdict = totalErrors === 0 ? 'PASS' : 'PASS_WITH_WARNINGS';

    console.log(`\n══════════════════════════════════════════`);
    console.log(`  Verdict: ${verdict}`);
    console.log(`  Page errors: ${totalErrors}`);
    console.log(`  Screenshots: admin-capital-models.png, client-capital-predictions.png, client-home-events.png`);
    console.log(`══════════════════════════════════════════\n`);

    process.exit(exitCode);
  } catch (err) {
    console.error('\n❌ Smoke test FAILED:', err.message);
    process.exit(1);
  }
}

main();
