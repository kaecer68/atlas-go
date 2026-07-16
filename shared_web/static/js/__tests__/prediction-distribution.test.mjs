// 對應 renderDistributionSegments 的單元測試。
// 涵蓋 C03 修復（inflow / neutral / outflow 三段累進 100%）常見分佈 + 缺欄位 fallback。
//
// 執行：node --test shared_web/static/js/__tests__/prediction-distribution.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// DOM / browser stubs required to render home.js in Node
// (home.js 在 module scope 直接呼叫 window.scrollToSection = ...)
// ============================================================================

global.window = {
  matchMedia() { return { matches: false, media: '' }; },
  switchPage() {},
  dispatchEvent() {},
  addEventListener() {},
  removeEventListener() {},
  scrollToSection() {},
};

global.document = {
  getElementById() { return null; },
  createElement() {
    return { innerHTML: '', textContent: '', style: {}, setAttribute() {}, appendChild() {} };
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
  removeEventListener() {},
};

const { renderDistributionSegments } = await import('../pages/home.js');

// ============================================================================
// Defensive
// ============================================================================

test('renderDistributionSegments: null → 全 0%', () => {
  const html = renderDistributionSegments(null);
  assert.match(html, /class="pred-row__dist"/);
  // 三段 segment,每一段 class 是 pred-row__dist-segment 加上 modifier --<direction>
  assert.match(html, /class="pred-row__dist-segment pred-row__dist-segment--inflow"/);
  assert.match(html, /class="pred-row__dist-segment pred-row__dist-segment--neutral"/);
  assert.match(html, /class="pred-row__dist-segment pred-row__dist-segment--outflow"/);
  // 都 0%
  assert.match(html, /width:0%"/);
  // 三個 label
  assert.match(html, /流入 0%/);
  assert.match(html, /觀望 0%/);
  assert.match(html, /流出 0%/);
});

test('renderDistributionSegments: undefined → 全 0%', () => {
  const html = renderDistributionSegments(undefined);
  assert.match(html, /流入 0%/);
  assert.match(html, /觀望 0%/);
  assert.match(html, /流出 0%/);
});

test('renderDistributionSegments: 非 object → 全 0%', () => {
  const html1 = renderDistributionSegments('not-an-object');
  assert.match(html1, /流入 0%/);
  assert.match(html1, /觀望 0%/);
  const html2 = renderDistributionSegments(42);
  assert.match(html2, /流出 0%/);
});

test('renderDistributionSegments: 缺欄位 → 該段 0%', () => {
  const html = renderDistributionSegments({});
  assert.match(html, /width:0%"/);
  assert.match(html, /流入 0%/);
  assert.match(html, /觀望 0%/);
  assert.match(html, /流出 0%/);
});

test('renderDistributionSegments: 部分欄位 → 缺失段為 0%', () => {
  const html = renderDistributionSegments({ inflow: 0.7 });
  // inflow 70%
  assert.match(html, /pred-row__dist-segment--inflow" style="width:70%"/);
  // neutral/outflow 0%
  assert.match(html, /pred-row__dist-segment--neutral" style="width:0%"/);
  assert.match(html, /pred-row__dist-segment--outflow" style="width:0%"/);
  assert.match(html, /流入 70%/);
  assert.match(html, /觀望 0%/);
  assert.match(html, /流出 0%/);
});

// ============================================================================
// 型別過濾
// ============================================================================

test('renderDistributionSegments: 非數值 → fallback 0%', () => {
  const html = renderDistributionSegments({
    inflow: 'high',
    neutral: null,
    outflow: { value: 0.5 },
  });
  assert.match(html, /pred-row__dist-segment--inflow" style="width:0%"/);
  assert.match(html, /pred-row__dist-segment--neutral" style="width:0%"/);
  assert.match(html, /pred-row__dist-segment--outflow" style="width:0%"/);
});

test('renderDistributionSegments: 負數 → fallback 0%', () => {
  const html = renderDistributionSegments({ inflow: -0.5, neutral: -0.1, outflow: -0.2 });
  assert.match(html, /width:0%"/);
  assert.match(html, /width:0%"/);
  assert.match(html, /width:0%"/);
});

// ============================================================================
// 四捨五入
// ============================================================================

test('renderDistributionSegments: 0.654 → 65% (Math.round 規則)', () => {
  const html = renderDistributionSegments({ inflow: 0.654 });
  assert.match(html, /pred-row__dist-segment--inflow" style="width:65%"/);
  assert.match(html, /流入 65%/);
});

test('renderDistributionSegments: 三段合計 ≈ 100%', () => {
  const html = renderDistributionSegments({ inflow: 0.4, neutral: 0.3, outflow: 0.3 });
  assert.match(html, /width:40%"/);
  assert.match(html, /width:30%"/);
  assert.match(html, /width:30%"/);
  assert.match(html, /流入 40%/);
  assert.match(html, /觀望 30%/);
  assert.match(html, /流出 30%/);
});

test('renderDistributionSegments: 1.0 不會 trigger overflow（流出 100%）', () => {
  const html = renderDistributionSegments({ outflow: 1.0 });
  assert.match(html, /width:100%"/);
  assert.match(html, /流出 100%/);
});

test('renderDistributionSegments: 0 → 0%（真正的零保留）', () => {
  const html = renderDistributionSegments({ inflow: 0, neutral: 0, outflow: 0 });
  assert.match(html, /width:0%"/);
  assert.match(html, /流入 0%/);
  assert.match(html, /觀望 0%/);
  assert.match(html, /流出 0%/);
});

// ============================================================================
// 結構輸出
// ============================================================================

test('renderDistributionSegments: 包含 pred-row__dist + pred-row__dist-label 兩個區塊', () => {
  const html = renderDistributionSegments({ inflow: 0.5, neutral: 0.25, outflow: 0.25 });
  assert.match(html, /<div class="pred-row__dist" aria-hidden="true">/);
  assert.match(html, /<div class="pred-row__dist-label">/);
  // 3 個 label span
  const labelSpanCount = (html.match(/<span>流入 |<span>觀望 |<span>流出 /g) || []).length;
  assert.equal(labelSpanCount, 3);
});

test('renderDistributionSegments: 三段 CSS 順序為 inflow / neutral / outflow', () => {
  const html = renderDistributionSegments({ inflow: 0.3, neutral: 0.4, outflow: 0.3 });
  const inflowIdx = html.indexOf('pred-row__dist-segment--inflow');
  const neutralIdx = html.indexOf('pred-row__dist-segment--neutral');
  const outflowIdx = html.indexOf('pred-row__dist-segment--outflow');
  assert.ok(inflowIdx >= 0);
  assert.ok(neutralIdx >= 0);
  assert.ok(outflowIdx >= 0);
  assert.ok(inflowIdx < neutralIdx);
  assert.ok(neutralIdx < outflowIdx);
});
