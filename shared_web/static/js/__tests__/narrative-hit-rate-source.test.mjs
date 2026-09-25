// shared_web/static/js/__tests__/narrative-hit-rate-source.test.mjs
//
// issue #1944 Batch 4 (item I23) 對外誠實測試：
// narrative「命中率」的來源映射與顯示文案不得把**手寫先驗常數**冒充成
// 歷史／回測／已量測的數字，且 `unavailable_*` / `not_populated` 的 0
// 不得呈現為「0%」（0 代表未知，不是零準確率）。
//
// 後端契約權威定義：internal/narrative/hitrate_provenance.go
// Run: node --test shared_web/static/js/__tests__/narrative-hit-rate-source.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

import {
  HIT_RATE_SOURCES,
  HIT_RATE_FIELD_LABEL,
  HIT_RATE_FIELD_TOOLTIP,
  HIT_RATE_UNKNOWN_SOURCE_LABEL,
  hitRateSourceLabel,
  hitRateSourceBadge,
  isKnownHitRateSource,
  isHitRateUnmeasurable,
  pickHitRateValue,
  pickHitRateSource,
  hitRateDisplayInfo,
  hitRateDisplay,
  hitRateTextColor,
} from '../shared/narrative-hit-rate.js';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO_JS = path.resolve(HERE, '..'); // shared_web/static/js

// 不得出現在「先驗」文案裡的宣稱字樣：這些字都會讓人以為數字已被量測。
const MEASUREMENT_CLAIM_WORDS = ['歷史', '回測', '實測', '勝率', '已量測'];

// ── (a) 來源標籤映射 ────────────────────────────────────────────────────────

test('hitRateSourceLabel maps every backend provenance label', () => {
  const cases = [
    ['handwritten_prior', '手寫先驗'],
    ['replay_eval_in_memory', '記憶體重算'],
    ['unavailable_no_samples', '無法量測'],
    ['unavailable_no_template', '無法量測'],
    ['not_populated', '未知'],
    ['mixed', '來源混合'],
  ];
  const seen = new Set();
  for (const [source, needle] of cases) {
    const label = hitRateSourceLabel(source);
    assert.equal(typeof label, 'string');
    assert.ok(label.includes(needle), `${source} label 應含「${needle}」，實際: ${label}`);
    assert.ok(!seen.has(label), `${source} 的 label 不可與其他來源相同: ${label}`);
    seen.add(label);
  }
  // 後端常數集合必須完整覆蓋（6 個合法值）
  assert.deepEqual(
    Object.values(HIT_RATE_SOURCES).sort(),
    ['handwritten_prior', 'mixed', 'not_populated', 'replay_eval_in_memory', 'unavailable_no_samples', 'unavailable_no_template'],
  );
});

test('hitRateSourceLabel/badge fall back to a honest label for unknown or missing source', () => {
  for (const bad of [undefined, null, '', '   ', 'some_new_backend_label', 42, {}]) {
    assert.equal(hitRateSourceLabel(bad), HIT_RATE_UNKNOWN_SOURCE_LABEL);
    assert.equal(hitRateSourceBadge(bad), HIT_RATE_UNKNOWN_SOURCE_LABEL);
    assert.equal(isKnownHitRateSource(bad), false);
  }
  assert.equal(HIT_RATE_UNKNOWN_SOURCE_LABEL, '來源不明');
});

test('hitRateSourceBadge gives a short badge for every known source', () => {
  const seen = new Set();
  for (const source of Object.values(HIT_RATE_SOURCES)) {
    const badge = hitRateSourceBadge(source);
    assert.ok(badge.length > 0 && badge.length <= 14, `${source} badge 過長: ${badge}`);
    assert.equal(seen.has(badge), false, `${source} 的 badge 不可與其他來源相同: ${badge}`);
    seen.add(badge);
  }
  // 不可量測的兩個來源要能從 badge 區分原因
  assert.notEqual(hitRateSourceBadge('unavailable_no_samples'), hitRateSourceBadge('unavailable_no_template'));
});

test('isHitRateUnmeasurable covers unavailable_* / not_populated / mixed only', () => {
  assert.equal(isHitRateUnmeasurable('unavailable_no_samples'), true);
  assert.equal(isHitRateUnmeasurable('unavailable_no_template'), true);
  assert.equal(isHitRateUnmeasurable('not_populated'), true);
  assert.equal(isHitRateUnmeasurable('mixed'), true);
  assert.equal(isHitRateUnmeasurable('handwritten_prior'), false);
  assert.equal(isHitRateUnmeasurable('replay_eval_in_memory'), false);
  assert.equal(isHitRateUnmeasurable(undefined), false);
});

// ── (b) 先驗文案不得宣稱已被量測 ────────────────────────────────────────────

test('handwritten_prior display text does not claim history / backtest / measurement', () => {
  const info = hitRateDisplayInfo(0.65, 'handwritten_prior');
  assert.equal(info.text, '65.0%');
  assert.equal(info.kind, 'prior');
  assert.equal(info.hasNumber, true);

  const strings = [info.text, info.badge, info.note, info.sourceLabel, HIT_RATE_FIELD_LABEL, HIT_RATE_FIELD_TOOLTIP];
  for (const s of strings) {
    for (const word of MEASUREMENT_CLAIM_WORDS) {
      assert.equal(s.includes(word), false, `先驗文案不可含「${word}」: ${s}`);
    }
  }
});

test('no source label or displayed string claims a measurement', () => {
  for (const source of Object.values(HIT_RATE_SOURCES)) {
    const info = hitRateDisplayInfo(0.55, source);
    const strings = [info.text, info.badge, info.note, info.sourceLabel,
      hitRateSourceLabel(source), hitRateSourceBadge(source), HIT_RATE_FIELD_LABEL];
    for (const s of strings) {
      for (const word of MEASUREMENT_CLAIM_WORDS) {
        assert.equal(s.includes(word), false, `${source} 文案不可含「${word}」: ${s}`);
      }
    }
  }
});

test('HIT_RATE_FIELD_LABEL is a prior label, not a history label', () => {
  assert.equal(HIT_RATE_FIELD_LABEL, '先驗命中率');
  assert.equal(HIT_RATE_FIELD_LABEL.includes('歷史'), false);
});

// ── (c) 未知來源 fallback ──────────────────────────────────────────────────

test('missing hit_rate_source never silently renders as a measured value', () => {
  const info = hitRateDisplayInfo(0.65, undefined);
  assert.equal(info.text, '65.0%'); // 值仍呈現…
  assert.equal(info.kind, 'unknown_source'); // …但標記為未知來源，不偽裝成量測
  assert.equal(info.badge, '來源不明');
  assert.equal(hitRateTextColor(info), 'var(--muted)');

  // 缺欄位（整個物件沒有 hit_rate_source）走同一條路
  const record = { hit_rate: 0.65 };
  assert.equal(pickHitRateSource(record), undefined);
  assert.equal(hitRateDisplay(pickHitRateValue(record), pickHitRateSource(record)), '65.0%');
});

test('pickHitRateValue / pickHitRateSource read model and template shapes', () => {
  assert.equal(pickHitRateValue({ hit_rate: 0.6 }), 0.6);
  assert.equal(pickHitRateValue({ historical_hit_rate: 0.72 }), 0.72);
  assert.equal(pickHitRateValue({ hit_rate: 0.6, historical_hit_rate: 0.72 }), 0.6);
  assert.equal(pickHitRateValue({}), null);
  assert.equal(pickHitRateValue(null), null);
  assert.equal(pickHitRateSource({ hit_rate_source: 'handwritten_prior' }), 'handwritten_prior');
  assert.equal(pickHitRateSource(null), undefined);
});

// ── (d) 0 值在 unavailable_* 時不得呈現為 0% ───────────────────────────────

test('zero value with unavailable_* / not_populated renders as unmeasurable, never 0%', () => {
  for (const source of ['unavailable_no_samples', 'unavailable_no_template', 'not_populated']) {
    for (const raw of [0, '0']) {
      const info = hitRateDisplayInfo(raw, source);
      assert.equal(info.hasNumber, false, `${source} 不應回報數值`);
      assert.equal(info.text.includes('%'), false, `${source} 不得輸出百分比: ${info.text}`);
      assert.equal(info.text.includes('0.0'), false, `${source} 不得輸出 0.0: ${info.text}`);
      assert.equal(info.unmeasured, true);
      assert.ok(
        info.text === '無法量測' || info.text === '未知',
        `${source} 應明示無法量測／未知，實際: ${info.text}`,
      );
    }
  }
});

test('mixed provenance never renders as a single percentage', () => {
  const info = hitRateDisplayInfo(0.6, 'mixed');
  assert.equal(info.text, '來源混合');
  assert.equal(info.text.includes('%'), false);
  assert.equal(info.hasNumber, false);
});

test('zero value with missing source renders as no-data, not 0% accuracy', () => {
  const info = hitRateDisplayInfo(0, undefined);
  assert.equal(info.text, '無資料');
  assert.equal(info.hasNumber, false);
  assert.equal(info.badge, '來源不明');
  // Playwright capital-pages.spec.js mock 走這條路（hit_rate: 0，無 source）
  assert.equal(hitRateDisplay(pickHitRateValue({ hit_rate: 0 }), pickHitRateSource({ hit_rate: 0 })), '無資料');
});

test('legacy "0/0" fraction renders as no-data, real fraction renders as percent', () => {
  assert.equal(hitRateDisplay('0/0', 'handwritten_prior'), '無資料');
  assert.equal(hitRateDisplay('3/4', 'handwritten_prior'), '75.0%');
  assert.equal(hitRateDisplay(null, 'handwritten_prior'), '—');
  assert.equal(hitRateDisplay(undefined, 'handwritten_prior'), '—');
  assert.equal(hitRateDisplay(NaN, 'handwritten_prior'), '—');
  assert.equal(hitRateDisplay('', 'handwritten_prior'), '—');
});

test('a declared prior of zero is shown as a prior value, not as a measurement', () => {
  const info = hitRateDisplayInfo(0, 'handwritten_prior');
  assert.equal(info.text, '0.0%');
  assert.equal(info.kind, 'prior');
  assert.equal(info.badge, '手寫先驗，未校準');
});

test('only in-memory measured values get performance colours', () => {
  const measured = hitRateDisplayInfo(0.72, 'replay_eval_in_memory');
  assert.equal(measured.kind, 'measured_in_memory');
  assert.equal(hitRateTextColor(measured), 'var(--color-success)');
  assert.equal(hitRateTextColor(hitRateDisplayInfo(0.55, 'replay_eval_in_memory')), 'var(--color-warning)');
  assert.equal(hitRateTextColor(hitRateDisplayInfo(0.4, 'replay_eval_in_memory')), 'var(--color-danger)');
  // 先驗不是量測值 → 不用績效色階暗示「表現好」
  assert.equal(hitRateTextColor(hitRateDisplayInfo(0.95, 'handwritten_prior')), 'var(--muted)');
  assert.equal(hitRateTextColor(hitRateDisplayInfo(0, 'unavailable_no_samples')), 'var(--muted)');
});

// ── 頁面接線（防止文案悄悄改回「歷史命中率」）──────────────────────────────

function readPage(rel) {
  return readFileSync(path.join(REPO_JS, 'pages', rel), 'utf8');
}

test('the three narrative pages import the shared provenance helper', () => {
  for (const page of ['narrative.js', 'capital-models.js', 'capital-causality.js']) {
    const src = readPage(page);
    assert.ok(
      src.includes("shared/narrative-hit-rate.js"),
      `${page} 必須共用 narrative-hit-rate.js 的來源映射`,
    );
  }
});

test('no narrative page claims "歷史命中率" anymore', () => {
  for (const page of ['narrative.js', 'capital-models.js', 'capital-causality.js']) {
    const src = readPage(page);
    assert.equal(src.includes('歷史命中率'), false, `${page} 仍宣稱「歷史命中率」`);
  }
});

test('narrative pages do not format a raw hit_rate without provenance', () => {
  for (const page of ['narrative.js', 'capital-models.js', 'capital-causality.js']) {
    const src = readPage(page);
    for (const bad of [/fmtSafePct\(\s*m\.hit_rate\s*\)/, /fmtSafePct\(\s*t\.historical_hit_rate\s*\)/, /fmtSafePct\(\s*hitRate\s*,/]) {
      assert.equal(bad.test(src), false, `${page} 仍有未帶來源的 hit rate 格式化: ${bad}`);
    }
  }
});

// ── 渲染層（stub DOM，不需瀏覽器）：頁面輸出真的帶上來源標記 ──────────────

function withStubDom(ids, fn) {
  const prev = globalThis.document;
  const store = {};
  for (const id of ids) {
    store[id] = {
      innerHTML: '',
      classList: { remove() {} },
      querySelector: () => null,
      querySelectorAll: () => [],
      addEventListener() {},
    };
  }
  globalThis.document = {
    getElementById: (id) => store[id] || null,
    body: { classList: { contains: () => false } },
  };
  try {
    return fn(store);
  } finally {
    globalThis.document = prev;
  }
}

test('renderCapitalModels: prior label + provenance, 0 never rendered as 0.0%', async () => {
  const { renderCapitalModels } = await import('../pages/capital-models.js');
  const html = withStubDom(['capitalModelsContent'], (store) => {
    renderCapitalModels({ models: [
      { name: 'A', weight: 1, hit_rate: 0.65, hit_rate_source: 'handwritten_prior' },
      { name: 'B', weight: 1, hit_rate: 0, hit_rate_source: 'unavailable_no_samples' },
      { name: 'C', weight: 1, hit_rate: 0 },
    ] });
    return store.capitalModelsContent.innerHTML;
  });
  assert.ok(html.includes('先驗命中率'), 'card metric 應使用先驗命中率標籤');
  assert.equal(html.includes('歷史命中率'), false);
  assert.ok(html.includes('65.0%'));
  assert.ok(html.includes('手寫先驗，未校準'));
  assert.ok(html.includes('無法量測'));
  assert.ok(html.includes('無資料'));
  assert.equal(/>0\.0+%</.test(html), false, '命中率欄位不得出現 0.0%');
});

test('renderCapitalCausality: template badge carries prior label and provenance', async () => {
  const { renderCapitalCausality } = await import('../pages/capital-causality.js');
  const html = withStubDom(['capitalCausalityContent', 'cc-list'], (store) => {
    renderCapitalCausality({ templates: [
      { name: 'T1', trigger_theme: 'AI_capex_surge', historical_hit_rate: 0.65, hit_rate_source: 'handwritten_prior', steps: [] },
      { name: 'T2', trigger_theme: 'US_rates_up', historical_hit_rate: 0, hit_rate_source: 'unavailable_no_samples', steps: [] },
    ] }, { models: [] });
    return store['cc-list'].innerHTML;
  });
  assert.ok(html.includes('先驗命中率 65.0%'), 'badge 應為先驗命中率');
  assert.ok(html.includes('手寫先驗，未校準'));
  assert.ok(html.includes('無法量測'));
  assert.equal(html.includes('歷史命中率'), false);
  assert.equal(/命中率 0\.0+%/.test(html), false, 'unavailable 的 0 不得顯示為 0.0%');
});

// ── 靜態 HTML 說明文案（client shell）──────────────────────────────────────
//
// client_web/static/index.html 的「如何解讀本頁」原本寫 hit_rate（歷史命中率），
// 同一個 false claim 只是換了一個檔案。逐字守門，避免它漂回去。

test('client shell help text does not call the narrative hit rate historical', () => {
  const htmlPath = path.resolve(HERE, '../../../../client_web/static/index.html');
  const html = readFileSync(htmlPath, 'utf8');
  assert.equal(
    html.includes('hit_rate（歷史命中率）') || html.includes('hit_rate(歷史命中率)'),
    false,
    'client_web/static/index.html 不得再把 narrative hit_rate 寫成「歷史命中率」',
  );
  assert.ok(
    html.includes('先驗命中率'),
    'client_web/static/index.html 的因果頁說明應標明 hit_rate 是先驗命中率',
  );
});
