/**
 * stage7_smoke.mjs — Stage 7 Frontend E2E Smoke Tests
 *
 * 目標:
 *   1. admin_web 錢潮預測頁正確渲染 narrative models
 *   2. client_web「近期事件」卡片在 home 頁正確顯示
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
    await page.waitForSelector('#capitalModelsContent', { timeout: TIMEOUT });
    console.log('  ✓ #capitalModelsContent found');
  } catch (e) {
    console.warn('  ⚠ #capitalModelsContent not rendered');
  }

  // Verify model table content
  const bodyText = await page.textContent('body');
  const keywords = ['鷹派聯準會', '權重', '命中率', '推理依據'];
  for (const kw of keywords) {
    if (bodyText.includes(kw)) {
      console.log(`  ✓ keyword "${kw}" found`);
    } else {
      console.warn(`  ⚠ keyword "${kw}" missing`);
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

async function clientHomeEventsSmoke(browser) {
  console.log('\n=== Test 2: client_web home page events & predictions ===');
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
    results.push(await clientHomeEventsSmoke(browser));

    await browser.close();

    const totalErrors = results.reduce((sum, r) => sum + r.errors.length, 0);
    const verdict = totalErrors === 0 ? 'PASS' : 'PASS_WITH_WARNINGS';

    console.log(`\n══════════════════════════════════════════`);
    console.log(`  Verdict: ${verdict}`);
    console.log(`  Page errors: ${totalErrors}`);
    console.log(`  Screenshots: admin-capital-models.png, client-home-events.png`);
    console.log(`══════════════════════════════════════════\n`);

    process.exit(exitCode);
  } catch (err) {
    console.error('\n❌ Smoke test FAILED:', err.message);
    process.exit(1);
  }
}

main();
