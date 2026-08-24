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

test('renderSectorHeatmap: populated entries render sector names + localized confidence/reasons', () => {
  const html = renderSectorHeatmap({
    sector_heatmap: [
      { sector: '半導體', confidence: 'high', confidence_score: 0.9, reasons: ['產業處於有利階段', '週期: recession', '季節性: 科技旺季'] },
      { sector: '金融', confidence: 'medium', confidence_score: 0.5, reasons: ['avg across industries'] },
      { sector: '散戶', confidence: 'low', confidence_score: 0.2, reasons: [] },
    ],
  });
  assert.ok(html.includes('半導體'), `expected 半導體 in rendered HTML, got: ${html}`);
  assert.ok(html.includes('金融'), `expected 金融 in rendered HTML, got: ${html}`);
  // 2026-08-24 UI audit P2：confidence 中文化（high→高 / medium→中 / low→低）
  assert.ok(html.includes('>高<'), `expected 高 confidence badge, got: ${html}`);
  assert.ok(html.includes('>中<'), `expected 中 confidence badge, got: ${html}`);
  assert.ok(html.includes('>低<'), `expected 低 confidence badge, got: ${html}`);
  // reasons 中文化（recession→衰退、avg across industries→跨產業平均）
  assert.ok(html.includes('週期：衰退') || html.includes('週期： 衰退'), `expected localized reason, got: ${html}`);
  assert.ok(html.includes('跨產業平均'), `expected avg across industries localized, got: ${html}`);
  assert.ok(html.includes('產業處於有利階段'), `expected reason text, got: ${html}`);
  // 不應殘留 raw enum
  assert.ok(!html.includes('recession'), `raw enum leaked: ${html}`);
  assert.ok(!html.includes('avg across industries'), `raw enum leaked: ${html}`);
  // Empty state must not appear when data is present.
  assert.ok(!html.includes('尚無產業數據'), `populated heatmap must not show empty state, got: ${html}`);
});
