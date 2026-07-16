// shared_web/static/js/__tests__/seven-force-board.test.mjs
//
// Unit tests for seven-force-board.js renderSevenForceBoard().
//
// 七大錢潮勢力卡片渲染：每個 force 渲染成 force-card，含方向、Z-score、權重。
// 對應前端 C04/C05 修復後新元件（PR #1198）。
//
// Test strategy：renderSevenForceBoard takes (container, summary) 並寫到
// container.innerHTML。沒有 jsdom 的情況下，用 plain object mock container
// ({ innerHTML: '' }) 即可驗收 innerHTML 字串。
//
// 執行：node --test shared_web/static/js/__tests__/seven-force-board.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderSevenForceBoard } from '../components/seven-force-board.js';

// helper: 跑 render function 後回傳 mock container 的 innerHTML
function renderToString(summary) {
  const container = { innerHTML: '' };
  renderSevenForceBoard(container, summary);
  return container.innerHTML;
}

// ---- Defensive: null container / missing data ----

test('container=null → silently no-op', () => {
  // 不應該 throw
  assert.doesNotThrow(() => renderSevenForceBoard(null, { forces: [{ force: 'foreign', trend: 'bullish' }] }));
  assert.doesNotThrow(() => renderSevenForceBoard(undefined, { forces: [] }));
});

test('summary=null → 顯示「尚無 7-Force 資料」placeholder', () => {
  const html = renderToString(null);
  assert.match(html, /尚無 7-Force 資料/);
  assert.match(html, /home-loading-card/);
});

test('summary undefined → placeholder', () => {
  const html = renderToString(undefined);
  assert.match(html, /尚無 7-Force 資料/);
});

test('summary.forces 缺失 → placeholder', () => {
  const html = renderToString({ score: 50 });
  assert.match(html, /尚無 7-Force 資料/);
});

test('summary.forces 不是 array → placeholder', () => {
  const html = renderToString({ forces: 'not-an-array' });
  assert.match(html, /尚無 7-Force 資料/);
});

test('summary.forces 是空 array → placeholder', () => {
  const html = renderToString({ forces: [] });
  assert.match(html, /尚無 7-Force 資料/);
});

// ---- Happy path: 三種 trend 各自 tone ----

test('bullish trend → 偏多 + force-card--positive tone', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish', z_score: 1.5, raw_value: 120.3, weight: 0.35 }],
  });
  assert.match(html, /force-card--positive/);
  assert.match(html, /偏多/);
  assert.match(html, /外資/); // FORCE_LABELS mapping
});

test('bearish trend → 偏空 + force-card--negative tone', () => {
  const html = renderToString({
    forces: [{ force: 'retail', trend: 'bearish', z_score: -2.0, raw_value: -50.0, weight: 0.25 }],
  });
  assert.match(html, /force-card--negative/);
  assert.match(html, /偏空/);
  assert.match(html, /散戶/);
});

test('neutral trend → 觀望 + force-card--neutral tone', () => {
  const html = renderToString({
    forces: [{ force: 'government', trend: 'neutral', z_score: 0, raw_value: 0, weight: 0.05 }],
  });
  assert.match(html, /force-card--neutral/);
  assert.match(html, /觀望/);
  assert.match(html, /政府\/公股行庫/);
});

test('trend 缺失 → 預設 neutral', () => {
  const html = renderToString({
    forces: [{ force: 'dealer' }],
  });
  assert.match(html, /force-card--neutral/);
  assert.match(html, /觀望/);
});

test('trend=unknown 字串(非 bullish/bearish) → 落入 neutral', () => {
  const html = renderToString({
    forces: [{ force: 'futures', trend: 'sideways' }],
  });
  assert.match(html, /force-card--neutral/);
});

// ---- Z-score → strength bar ----
// strength = clamp(|z_score|/3, 0..1) → percent

test('z_score=0 → strength bar 寬度 = 0%', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'neutral', z_score: 0 }],
  });
  assert.match(html, /width: 0%/);
});

test('z_score=1.5 → strength bar 寬度 = 50%', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish', z_score: 1.5 }],
  });
  assert.match(html, /width: 50%/);
});

test('z_score=-2.4 → strength bar 寬度 = 80% (取絕對值)', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bearish', z_score: -2.4 }],
  });
  assert.match(html, /width: 80%/);
});

test('z_score=3 → strength bar 寬度 = 100% (capped at 1)', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish', z_score: 3.0 }],
  });
  assert.match(html, /width: 100%/);
});

test('z_score=10 → strength bar 寬度 = 100% (即使超出也 clamp)', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish', z_score: 10.0 }],
  });
  assert.match(html, /width: 100%/);
});

test('z_score 缺失 → bar 寬度 0%', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish' }],
  });
  assert.match(html, /width: 0%/);
});

// ---- 權重 (weight) ----

test('weight=0.35 → 顯示 35%', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish', weight: 0.35 }],
  });
  assert.match(html, /權重 35%/);
});

test('weight=0 → 顯示 0%', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'neutral', weight: 0 }],
  });
  assert.match(html, /權重 0%/);
});

test('weight 缺失 → 顯示 —', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish' }],
  });
  assert.match(html, /權重 —/);
});

// ---- raw_value (signed 億) ----

test('raw_value=120.3 → 顯示 +120.3 億', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', trend: 'bullish', raw_value: 120.3 }],
  });
  assert.match(html, /\+120\.3.*億/);
});

test('raw_value=-50.7 → 顯示 −50.7 億 (Unicode minus U+2212 保留)', () => {
  const html = renderToString({
    forces: [{ force: 'retail', trend: 'bearish', raw_value: -50.7 }],
  });
  // formatSigned() 用 Unicode 負號 (−) 而非半形 hyphen (-)
  assert.match(html, /−50\.7.*億/);
  assert.doesNotMatch(html, /-50\.7 億/);  // 確保不是 ASCII hyphen
});

test('raw_value=0 → 顯示 0.0 億', () => {
  const html = renderToString({
    forces: [{ force: 'government', trend: 'neutral', raw_value: 0 }],
  });
  assert.match(html, /0\.0.*億/);
});

test('raw_value 缺失 → 顯示 —', () => {
  const html = renderToString({
    forces: [{ force: 'futures', trend: 'bullish' }],
  });
  assert.match(html, /force-card__value"[^>]*>—</);
});

// ---- PascalCase 後備欄位 ----

test('backend 用 PascalCase (Trend/ZScore/Weight/RawValue) 也能渲染', () => {
  const html = renderToString({
    forces: [{
      Force: 'foreign',
      Trend: 'BULLISH',  // 大寫
      ZScore: 1.8,
      RawValue: 88.5,
      Weight: 0.42,
    }],
  });
  assert.match(html, /force-card--positive/);
  assert.match(html, /偏多/);
  assert.match(html, /width: 60%/); // 1.8/3 = 0.6 → 60%
  assert.match(html, /\+88\.5.*億/);
  assert.match(html, /權重 42%/);
  assert.match(html, /外資/);
});

// ---- FORCE_LABELS mapping ----

test('未知 force 名稱 → 直接顯示原名 (沒有 mapping)', () => {
  const html = renderToString({
    forces: [{ force: 'mystery_force', trend: 'bullish' }],
  });
  assert.match(html, /mystery_force/);
  assert.match(html, /force-card__name[^>]*>mystery_force</);
});

test('所有 7 個 force 都有中文 label', () => {
  const allForces = [
    { force: 'foreign' },
    { force: 'institutional' },
    { force: 'dealer' },
    { force: 'retail' },
    { force: 'government' },
    { force: 'futures' },
    { force: 'tsm_adr' },
  ];
  const html = renderToString({ forces: allForces });
  assert.match(html, /外資/);
  assert.match(html, /投信/);
  assert.match(html, /自營商/);
  assert.match(html, /散戶/);
  assert.match(html, /政府\/公股行庫/);
  assert.match(html, /期貨/);
  assert.match(html, /TSM ADR/);
});

// ---- 多 force 渲染順序 ----

test('多個 forces 依序渲染,snake_case 容錯', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish', z_score: 1.0 },
      { force: 'institutional', trend: 'bullish', z_score: 1.5 },
      { force: 'retail', trend: 'bearish', z_score: -0.5 },
    ],
  });
  // 確認有三張卡
  const matches = html.match(/class="force-card /g) || [];
  assert.equal(matches.length, 3);
  // 多張卡以 wrapper 包起來
  assert.match(html, /seven-force-board/);
});

// ---- HTML escaping ----

test('force label 含 XSS payload → escape 成純文字', () => {
  const html = renderToString({
    forces: [{ force: '<script>alert(1)</script>', trend: 'bullish' }],
  });
  // escapeHtml 應轉義 < > 與引號
  assert.doesNotMatch(html, /<script>alert/);
  assert.match(html, /&lt;script&gt;/);
});
