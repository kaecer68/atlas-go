// shared_web/static/js/__tests__/datachannels-disabled.test.mjs
//
// #1758 落地：intentionally-disabled channels（channels.json enabled=false，
// 如 tej / twse_etf）必須從主表格 + 摘要統計 + alerts 隱藏，僅留一行摘要。
import { test } from 'node:test';
import assert from 'node:assert/strict';

const { splitDisabledChannels } = await import('../pages/datachannels.js');

const CH = (id, enabled, status = 'ok') => ({ channel_id: id, enabled, status });

test('splitDisabledChannels: enabled=false 的通道被分到 disabled', () => {
  const { active, disabled } = splitDisabledChannels([
    CH('finmind', true),
    CH('tej', false),
    CH('twse_etf', false),
    CH('us_spx', true),
  ]);
  assert.deepEqual(active.map(c => c.channel_id), ['finmind', 'us_spx']);
  assert.deepEqual(disabled.map(c => c.channel_id), ['tej', 'twse_etf']);
});

test('splitDisabledChannels: 缺 enabled 欄位視為啟用（預設 on 契約）', () => {
  const { active, disabled } = splitDisabledChannels([{ channel_id: 'x' }]);
  assert.equal(active.length, 1);
  assert.equal(disabled.length, 0);
});

test('splitDisabledChannels: 空/undefined 輸入', () => {
  assert.deepEqual(splitDisabledChannels([]), { active: [], disabled: [] });
  assert.deepEqual(splitDisabledChannels(undefined), { active: [], disabled: [] });
});

test('splitDisabledChannels: disabled 通道即使 status=error 也歸 disabled', () => {
  const { active, disabled } = splitDisabledChannels([CH('tej', false, 'error')]);
  assert.equal(disabled.length, 1);
  assert.equal(active.length, 0);
});
