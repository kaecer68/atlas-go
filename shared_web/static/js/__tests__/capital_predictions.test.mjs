// shared_web/static/js/__tests__/capital_predictions.test.mjs
//
// Unit tests for capital_predictions.js mapPredictionForDisplay() helper.
//
// FU-2 fix 對 master doc § 5.5 確認的 DOM shape mismatch:
//   - Backend FlowPrediction 不含 reasons[] / sectors[] 欄位(只有 driving_events / predicted_forces)
//   - Capital PredictionReport 同時回 active_events[] 含 affected_industries
//   - Frontend 原碼 silent 讀 reasons/sectors → 永遠拿到 [] → detail panel 顯示「無觸發原因」
//
// 修法:extract mapPredictionForDisplay 純函數,做兩件事:
//   1. reasons ← prediction.driving_events
//   2. sectors ← name-match driving_events 與 active_events.name,union 對應的 affected_industries
// 結果保留原本 UI 顯示結構(reasons[] as `<li>`,sectors[] as `<span class="chip">`)不變。
//
// 執行:node --test shared_web/static/js/__tests__/capital_predictions.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mapPredictionForDisplay } from '../pages/capital_predictions.js';

// ---- Defensive / 空輸入 ----

test('prediction=null → reasons=[], sectors=[]', () => {
  const r = mapPredictionForDisplay(null, []);
  assert.deepEqual(r, { reasons: [], sectors: [] });
});

test('prediction=undefined → reasons=[], sectors=[]', () => {
  const r = mapPredictionForDisplay(undefined, []);
  assert.deepEqual(r, { reasons: [], sectors: [] });
});

test('prediction=非物件(string) → reasons=[], sectors=[]', () => {
  const r = mapPredictionForDisplay('not-a-prediction', []);
  assert.deepEqual(r, { reasons: [], sectors: [] });
});

test('activeEvents=null → reasons 仍來自 driving_events,sectors=[]', () => {
  const r = mapPredictionForDisplay(
    { driving_events: ['MSCI 季調', '0050 配息'] },
    null
  );
  assert.deepEqual(r.reasons, ['MSCI 季調', '0050 配息']);
  assert.deepEqual(r.sectors, []);
});

test('activeEvents 不是 array → sectors=[]', () => {
  const r = mapPredictionForDisplay(
    { driving_events: ['MSCI 季調'] },
    { not: 'an array' }
  );
  assert.deepEqual(r, { reasons: ['MSCI 季調'], sectors: [] });
});

// ---- 空 driving_events ----

test('driving_events 缺失 → reasons=[], sectors=[]', () => {
  const r = mapPredictionForDisplay({ direction: 'inflow' }, [
    { name: 'MSCI 季調', affected_industries: ['半導體'] },
  ]);
  assert.deepEqual(r, { reasons: [], sectors: [] });
});

test('driving_events=空 array → reasons=[], sectors=[]', () => {
  const r = mapPredictionForDisplay({ driving_events: [] }, [
    { name: 'MSCI 季調', affected_industries: ['半導體'] },
  ]);
  assert.deepEqual(r, { reasons: [], sectors: [] });
});

// ---- Happy path ----

test('driving_events 全部都有 matching active_event → sectors 是 union', () => {
  const activeEvents = [
    { name: 'MSCI 季調', affected_industries: ['半導體', '電子'] },
    { name: '0050 配息', affected_industries: ['金融'] },
    { name: '投信季底做帳', affected_industries: ['鋼鐵', '塑化'] },
  ];
  const prediction = { driving_events: ['MSCI 季調', '0050 配息', '投信季底做帳'] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['MSCI 季調', '0050 配息', '投信季底做帳']);
  // Union of affected_industries sorted by Unicode codepoint:
  // 半(534A) < 塑(5851) < 金(91D1) < 鋼(92FC) < 電(96FB)
  assert.deepEqual(r.sectors.sort(), ['半導體', '塑化', '金融', '鋼鐵', '電子']);
});

test('driving_events 部分 match → 只有 match 的 sectors', () => {
  const activeEvents = [
    { name: 'MSCI 季調', affected_industries: ['半導體', '電子'] },
    { name: '0050 配息', affected_industries: ['金融'] },
  ];
  // 0050 配息 在 driving_events 但 active_events 沒 match(故意測試匹配嚴謹度)
  const prediction = { driving_events: ['MSCI 季調', 'NotInActiveEvents'] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['MSCI 季調', 'NotInActiveEvents']);
  // Union codepoint-sorted: 半(534A) < 電(96FB)
  assert.deepEqual(r.sectors.sort(), ['半導體', '電子']);
});

// ---- Dedup ----

test('多個 driving_events 引用同一個 sector → dedup', () => {
  const activeEvents = [
    { name: 'A', affected_industries: ['半導體', '電子'] },
    { name: 'B', affected_industries: ['半導體'] },  // 重複「半導體」
    { name: 'C', affected_industries: ['金融'] },
  ];
  const prediction = { driving_events: ['A', 'B', 'C'] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['A', 'B', 'C']);
  // Union codepoint-sort: 半(534A) < 金(91D1) < 電(96FB)
  assert.deepEqual(r.sectors.sort(), ['半導體', '金融', '電子']);
});

// ---- Defensive / 欄位缺失容忍 ----

test('active_event 缺 name → 跳過該 event', () => {
  const activeEvents = [
    { name: 'MSCI 季調', affected_industries: ['半導體'] },
    { affected_industries: ['金融'] },  // 缺 name
    null,                                  // null entry
    'string-not-object',                   // 不是 object
  ];
  const prediction = { driving_events: ['MSCI 季調'] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['MSCI 季調']);
  assert.deepEqual(r.sectors, ['半導體']);
});

test('active_event 缺 affected_industries → 跳過,其他 event 正常', () => {
  const activeEvents = [
    { name: 'A' },  // 缺 affected_industries
    { name: 'B', affected_industries: ['半導體'] },
  ];
  const prediction = { driving_events: ['A', 'B'] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['A', 'B']);
  assert.deepEqual(r.sectors, ['半導體']);
});

test('affected_industries 包含非字串元素 → 過濾掉', () => {
  const activeEvents = [
    { name: 'A', affected_industries: ['半導體', null, 42, { foo: 1 }, ''] },
  ];
  const prediction = { driving_events: ['A'] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['A']);
  assert.deepEqual(r.sectors, ['半導體']);  // 只有「半導體」是 non-empty string
});

test('driving_events 含空字串或非字串 → 過濾', () => {
  const activeEvents = [
    { name: 'MSCI 季調', affected_industries: ['半導體'] },
  ];
  const prediction = { driving_events: ['MSCI 季調', null, '', 42] };
  const r = mapPredictionForDisplay(prediction, activeEvents);

  assert.deepEqual(r.reasons, ['MSCI 季調']);  // 過濾 empty string + non-string
  assert.deepEqual(r.sectors, ['半導體']);
});

// ---- 真實 backend shape smoke test (對齊 internal/eventdriven/types.go) ----

test('跟 backend 真實 shape 對齊:FlowPrediction + EventCalendarItem', () => {
  // 模擬 internal/eventdriven/types.go FlowPrediction + EventCalendarItem 的 JSON 結構
  const data = {
    predictions: [
      {
        date: '2026-07-15T00:00:00Z',
        direction: 'inflow',
        confidence: 0.85,
        driving_events: ['MSCI 季調', '0050 配息'],
        predicted_forces: ['foreign', 'institutional'],
      },
    ],
    active_events: [
      { name: 'MSCI 季調', event_type: 'MSCI', direction: 'inflow', affected_industries: ['半導體', '電子'] },
      { name: '0050 配息', event_type: 'ETF_DIVIDEND', direction: 'inflow', affected_industries: ['金融'] },
    ],
  };

  const r = mapPredictionForDisplay(data.predictions[0], data.active_events);

  assert.deepEqual(r.reasons, ['MSCI 季調', '0050 配息']);
  // Union codepoint-sort: 半(534A) < 金(91D1) < 電(96FB)
  assert.deepEqual(r.sectors.sort(), ['半導體', '金融', '電子']);
});
