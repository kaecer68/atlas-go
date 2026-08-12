// shared_web/static/js/__tests__/industry-heatmap.test.mjs
//
// Regression tests for the 產業熱力圖 (sector heatmap) card on /client/industry.
// The card renders the `sector_heatmap` block of the shared aggregate endpoint
// GET /api/dashboard/decision-chain (see components/decision-panels.js).
//
// Replaces the former 產業地圖 (sector-allocation-plan) card + its SA08/SA09
// empty-state tests (2026-08-12, decision-chain 頁面拆分).

import { test } from 'node:test';
import assert from 'node:assert/strict';

const { renderSectorHeatmap } = await import('../components/decision-panels.js');

test('renderSectorHeatmap: missing sector_heatmap renders empty state', () => {
  const html = renderSectorHeatmap({});
  assert.ok(
    html.includes('尚無產業數據'),
    `expected "尚無產業數據" placeholder, got: ${html}`
  );
});

test('renderSectorHeatmap: empty array renders empty state', () => {
  const html = renderSectorHeatmap({ sector_heatmap: [] });
  assert.ok(
    html.includes('尚無產業數據'),
    `expected "尚無產業數據" placeholder, got: ${html}`
  );
});

test('renderSectorHeatmap: null payload renders empty state', () => {
  const html = renderSectorHeatmap(null);
  assert.ok(
    html.includes('尚無產業數據'),
    `expected "尚無產業數據" placeholder, got: ${html}`
  );
});

test('renderSectorHeatmap: populated entries render sector names + confidence', () => {
  const html = renderSectorHeatmap({
    sector_heatmap: [
      { sector: '半導體', confidence: 'high', confidence_score: 0.9, reasons: ['產業處於有利階段', '週期: 擴張'] },
      { sector: '金融', confidence: 'low', confidence_score: 0.2, reasons: [] },
    ],
  });
  assert.ok(html.includes('半導體'), `expected 半導體 in rendered HTML, got: ${html}`);
  assert.ok(html.includes('金融'), `expected 金融 in rendered HTML, got: ${html}`);
  assert.ok(html.includes('high'), `expected high confidence badge, got: ${html}`);
  assert.ok(html.includes('產業處於有利階段'), `expected reason text, got: ${html}`);
  // Empty state must not appear when data is present.
  assert.ok(!html.includes('尚無產業數據'), `populated heatmap must not show empty state, got: ${html}`);
});
