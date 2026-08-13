/**
 * pipeline-narrative.spec.mjs — Frontend E2E: 推薦管線敘事歸因
 *
 * 驗證 pipeline 頁每筆推薦渲染「敘事歸因」區塊（reasoning_chain +
 * supporting_events），來自後端 applyNarrativeContextWithEvents 的填充。
 *
 * To run:
 *   node tests/frontend-e2e/pipeline-narrative.spec.mjs
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

async function pipelineNarrativeSmoke(browser) {
  console.log('\n=== Test: pipeline 頁敘事歸因 ===');
  const page = await browser.newPage();
  await page.setViewportSize({ width: 1280, height: 800 });

  const errors = [];
  page.on('pageerror', (err) => errors.push(err.message));

  // Navigate to client root, wait for SPA init, then route to pipeline page.
  await page.goto(`${BASE_URL}/client/`, { waitUntil: 'load', timeout: TIMEOUT });
  await page.waitForFunction(() => window.switchPage !== undefined, {}, { timeout: TIMEOUT });
  await page.evaluate(() => window.switchPage('pipeline', true));
  console.log('  called switchPage("pipeline")');

  // Wait for the pipeline table to render.
  try {
    await page.waitForSelector('#recommendationPipeline', { timeout: TIMEOUT });
    console.log('  ✓ #recommendationPipeline found');
  } catch (e) {
    console.warn('  ⚠ #recommendationPipeline not rendered');
  }

  const bodyText = await page.textContent('body');

  // 1. 敘事歸因區塊必須存在（mock 的 reasoning_chain 來自後端填充）。
  if (bodyText.includes('敘事歸因')) {
    console.log('  ✓ 「敘事歸因」 rendered');
  } else {
    console.warn('  ⚠ 「敘事歸因」 missing');
  }

  // 2. chain 內容（theme + region + confidence）應顯示。
  if (bodyText.includes('AI_capex_surge')) {
    console.log('  ✓ reasoning_chain theme "AI_capex_surge" rendered');
  } else {
    console.warn('  ⚠ reasoning_chain content missing');
  }

  // 3. supporting_events badge 應顯示。
  if (bodyText.includes('evt-ai-20260809')) {
    console.log('  ✓ supporting_events badge rendered');
  } else {
    console.warn('  ⚠ supporting_events badge missing');
  }

  const ssPath = path.join(SCREENSHOT_DIR, 'pipeline-narrative.png');
  await page.screenshot({ path: ssPath, fullPage: true });
  console.log(`  ✓ Screenshot saved: ${ssPath}`);

  if (errors.length > 0) {
    console.log(`  Console errors (${errors.length}): ${errors.slice(0, 5).join('; ')}`);
  }

  await page.close();
  return {
    narrative: bodyText.includes('敘事歸因'),
    theme: bodyText.includes('AI_capex_surge'),
    event: bodyText.includes('evt-ai-20260809'),
    errors,
  };
}

async function main() {
  try {
    const browser = await chromium.launch({
      headless: true,
      args: ['--no-sandbox', '--disable-setuid-sandbox'],
    });
    const r = await pipelineNarrativeSmoke(browser);
    await browser.close();

    const allPassed = r.narrative && r.theme && r.event && r.errors.length === 0;
    console.log(`\n══════════════════════════════════════════`);
    console.log(`  Verdict: ${allPassed ? 'PASS' : 'FAIL'}`);
    console.log(`  「敘事歸因」: ${r.narrative ? '✓' : '✗'}`);
    console.log(`  reasoning_chain: ${r.theme ? '✓' : '✗'}`);
    console.log(`  supporting_events: ${r.event ? '✓' : '✗'}`);
    console.log(`  Page errors: ${r.errors.length}`);
    console.log(`══════════════════════════════════════════\n`);
    process.exit(allPassed ? 0 : 1);
  } catch (err) {
    console.error('\n❌ Pipeline narrative test FAILED:', err.message);
    process.exit(1);
  }
}

main();
