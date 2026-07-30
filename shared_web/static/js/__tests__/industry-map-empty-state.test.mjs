// shared_web/static/js/__tests__/industry-map-empty-state.test.mjs
//
// Regression tests for the "產業地圖" (industryMap) card empty state on
// /client/industry. The backend `/api/dashboard/sector-allocation-plan` returns
// a SectorAllocationSnapshot with `target: null, fallback_reason:
// "no_simulation_session"` whenever the SA08/SA09 closure pipeline has not
// yet emitted its first snapshot (e.g. fresh install, no replay data, or
// the FileClosureStore has not been seeded).
//
// Before this fix the card silently rendered the bare phrase "尚無產業資料",
// making the page look broken. These tests guard the new behavior:
//
//   1. SA08/SA09 empty snapshot → "產業配置尚未就緒" + the reason's friendly
//      hint ("等待首次模擬收盤產生產業配置" for no_simulation_session).
//   2. 503 snapshot_unavailable → similar message with its hint.
//   3. Unrecognised fallback_reason → "尚無產業資料" (legacy path preserved).
//   4. Populated target map → renders weight bars (no regression).

import { test } from 'node:test';
import assert from 'node:assert/strict';

function stubIndustryMapElement() {
  const el = {
    innerHTML: '',
    classList: { remove: () => {} },
  };
  return el;
}

function withStubs(stubEl, fn) {
  const originalDocument = globalThis.document;
  globalThis.document = { getElementById: (id) => (id === 'industryMap' ? stubEl : null) };
  try {
    return fn();
  } finally {
    if (originalDocument === undefined) {
      delete globalThis.document;
    } else {
      globalThis.document = originalDocument;
    }
  }
}

const { renderIndustryMap } = await import('../pages/industry.js');

test('renderIndustryMap: SA08/SA09 empty snapshot surfaces fallback_reason hint', () => {
  const el = stubIndustryMapElement();
  withStubs(el, () => {
    renderIndustryMap({
      as_of_trading_date: '',
      target: null,
      current: null,
      delta: null,
      model_version: '',
      calibration_status: '',
      weight_source: '',
      fallback_reason: 'no_simulation_session',
      applied: false,
    });
  });
  assert.ok(
    el.innerHTML.includes('產業配置尚未就緒'),
    `expected "產業配置尚未就緒" placeholder, got: ${el.innerHTML}`
  );
  assert.ok(
    el.innerHTML.includes('等待首次模擬收盤產生產業配置'),
    `expected the no_simulation_session hint, got: ${el.innerHTML}`
  );
});

test('renderIndustryMap: snapshot_unavailable renders its own friendly hint', () => {
  const el = stubIndustryMapElement();
  withStubs(el, () => {
    renderIndustryMap({
      target: {},
      current: {},
      delta: {},
      fallback_reason: 'snapshot_unavailable',
    });
  });
  // target is an empty object, so the SA08/SA09 branch fires the empty
  // state (entries.length === 0) and surfaces fallback_reason.
  assert.ok(
    el.innerHTML.includes('產業配置尚未就緒'),
    `expected "產業配置尚未就緒" placeholder, got: ${el.innerHTML}`
  );
  assert.ok(
    el.innerHTML.includes('產業配置服務暫時無法取得快照'),
    `expected the snapshot_unavailable hint, got: ${el.innerHTML}`
  );
});

test('renderIndustryMap: unknown fallback_reason keeps the generic empty state', () => {
  const el = stubIndustryMapElement();
  withStubs(el, () => {
    renderIndustryMap({ target: null, fallback_reason: 'something_else' });
  });
  // Unknown reason: hint is null, so we fall back to the legacy "尚無產業資料"
  // wording and do NOT show the "尚未就緒" copy (avoid misleading users about
  // a state we don't actually have a hint for).
  assert.ok(
    el.innerHTML.includes('尚無產業資料'),
    `expected legacy "尚無產業資料" wording, got: ${el.innerHTML}`
  );
  assert.ok(
    !el.innerHTML.includes('產業配置尚未就緒'),
    `must not advertise the "尚未就緒" copy for unknown reasons, got: ${el.innerHTML}`
  );
});

test('renderIndustryMap: data=null keeps the original "尚無產業資料" wording', () => {
  const el = stubIndustryMapElement();
  withStubs(el, () => {
    renderIndustryMap(null);
  });
  assert.ok(
    el.innerHTML.includes('尚無產業資料'),
    `expected legacy wording for null payload, got: ${el.innerHTML}`
  );
});

test('renderIndustryMap: populated target map renders the weight bars (no regression)', () => {
  const el = stubIndustryMapElement();
  withStubs(el, () => {
    renderIndustryMap({
      target: { semiconductor: 0.33, financials: 0.20 },
      current: { semiconductor: 0.30, financials: 0.22 },
      delta: { semiconductor: 0.03, financials: -0.02 },
    });
  });
  // Populated data must render sector names, not the empty-state placeholder.
  assert.ok(
    !el.innerHTML.includes('產業配置尚未就緒'),
    `populated snapshot must not show empty state, got: ${el.innerHTML}`
  );
  assert.ok(
    el.innerHTML.includes('半導體'),
    `expected 半導體 sector name in rendered HTML, got: ${el.innerHTML}`
  );
  assert.ok(
    el.innerHTML.includes('33%'),
    `expected rounded target percent 33% for semiconductor, got: ${el.innerHTML}`
  );
});
