// shared_web/static/js/__tests__/channel-status.test.mjs
//
// fix/20260924-channel-status-truth — 前端 channel status SSOT 單元測試。
//
// 後端契約:
//   internal/monitoring/service/session.go   StatusText()  → 中文 label
//   internal/apigateway/channel_status.go                   → ok / warn / error /
//     degraded / inactive / stale / unknown
//   internal/monitoring/service/system.go                   → expected_delay
//
// 規則 (本次修復):
//   stale    → 「資料過期」amber (--status-stale)，計入非正常，不得算正常
//   degraded → 「降級」  amber，計入非正常
//   inactive → 「未啟用」muted，不算異常也不算正常
//   error/partial → err(紅)
//   未知值 / 缺 status → 「未知」muted，絕不變成正常，也不自行變紅
//
// 執行: node --test shared_web/static/js/__tests__/channel-status.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  CHANNEL_STATUS,
  CHANNEL_TONE,
  channelStatusMeta,
  isChannelNormal,
  isChannelAbnormal,
  worstChannelStatus,
} from '../shared/channel-status.js';

const TONES = ['ok', 'warn', 'err', 'muted'];

test('stale → 資料過期, amber (warn badge / --status-stale), 非正常', () => {
  const m = channelStatusMeta('stale');
  assert.equal(m.known, true);
  assert.equal(m.label, '資料過期');
  assert.equal(m.badgeClass, 'warn', 'stale 必須是 warn badge，不得用 err(紅)');
  assert.equal(m.light, 'var(--status-stale)');
  assert.equal(m.normal, false, 'stale 不得算「正常」');
  assert.equal(m.abnormal, true, 'stale 必須計入「非正常」');
  assert.equal(m.needsAttention, true);
});

test('degraded → 降級, amber (warn badge), 非正常', () => {
  const m = channelStatusMeta('degraded');
  assert.equal(m.known, true);
  assert.equal(m.label, '降級');
  assert.equal(m.badgeClass, 'warn');
  assert.equal(m.light, 'var(--status-warn)');
  assert.equal(m.normal, false);
  assert.equal(m.abnormal, true, 'degraded 必須計入「非正常」');
});

test('error / partial → err (紅), 非正常', () => {
  for (const s of ['error', 'partial']) {
    const m = channelStatusMeta(s);
    assert.equal(m.badgeClass, 'err', s + ' 應為紅');
    assert.equal(m.normal, false, s + ' 不得算正常');
    assert.equal(m.abnormal, true);
  }
  assert.equal(channelStatusMeta('error').label, '異常');
  assert.equal(channelStatusMeta('partial').label, '部分異常');
});

test('inactive → 未啟用, muted, 不算異常也不算正常', () => {
  const m = channelStatusMeta('inactive');
  assert.equal(m.label, '未啟用');
  assert.equal(m.badgeClass, 'muted');
  assert.equal(m.normal, false);
  assert.equal(m.abnormal, false, 'inactive 是操作決策，不列入異常統計');
  assert.equal(m.needsAttention, false, 'inactive 不掛需要關注徽章');
});

test('ok / expected_delay → 正常 (綠)', () => {
  for (const s of ['ok', 'expected_delay']) {
    const m = channelStatusMeta(s);
    assert.equal(m.normal, true, s + ' 應算正常');
    assert.equal(m.abnormal, false);
    assert.equal(m.badgeClass, 'ok');
  }
  assert.equal(channelStatusMeta('expected_delay').label, '正常延遲');
});

test('未知值 (缺 status / 拼錯 / 大小寫不同) → 未知 muted，絕不變正常也不變紅', () => {
  for (const raw of ['bogus', '', null, undefined, 'OK', 'Stale', 42]) {
    const m = channelStatusMeta(raw);
    assert.equal(m.known, false, String(raw) + ' 不應被視為已知狀態');
    assert.equal(m.label, '未知');
    assert.equal(m.badgeClass, 'muted');
    assert.equal(m.normal, false, String(raw) + ' 不得算正常');
    assert.equal(m.abnormal, false, String(raw) + ' 不自行變成異常(紅)');
  }
});

test('unknown (後端明確回報 unknown) → 未知 muted 但列入需處理', () => {
  const m = channelStatusMeta('unknown');
  assert.equal(m.known, true);
  assert.equal(m.label, '未知');
  assert.equal(m.badgeClass, 'muted');
  assert.equal(m.normal, false, 'unknown 不得算正常');
  assert.equal(m.needsAttention, true, 'unknown 應被標示出來，而非靜默通過');
});

test('isChannelNormal / isChannelAbnormal', () => {
  assert.equal(isChannelNormal('ok'), true);
  assert.equal(isChannelNormal('expected_delay'), true);
  for (const s of ['warn', 'stale', 'degraded', 'error', 'partial', 'inactive', 'unknown', 'bogus']) {
    assert.equal(isChannelNormal(s), false, s + ' 不得算正常');
  }
  assert.equal(isChannelAbnormal('stale'), true);
  assert.equal(isChannelAbnormal('degraded'), true);
  assert.equal(isChannelAbnormal('inactive'), false);
  assert.equal(isChannelAbnormal('unknown'), false);
  assert.equal(isChannelAbnormal('bogus'), false);
});

test('worstChannelStatus 取最嚴重 (error > partial > stale > degraded > warn > unknown)', () => {
  assert.equal(worstChannelStatus(['ok', 'warn', 'stale']), 'stale');
  assert.equal(worstChannelStatus(['stale', 'degraded']), 'stale');
  assert.equal(worstChannelStatus(['stale', 'error']), 'error');
  assert.equal(worstChannelStatus(['partial', 'error']), 'error');
  assert.equal(worstChannelStatus(['ok', 'unknown']), 'unknown');
  assert.equal(worstChannelStatus(['inactive', 'ok']), 'ok');
  assert.equal(worstChannelStatus([]), 'unknown');
  assert.equal(worstChannelStatus(undefined), 'unknown');
});

test('每個 status 的 tone 都是合法 tone（避免打錯字造成靜默 muted fallback）', () => {
  for (const [status, entry] of Object.entries(CHANNEL_STATUS)) {
    assert.ok(TONES.includes(entry.tone), status + ' 的 tone 應為 ' + TONES.join('/'));
    assert.ok(typeof entry.label === 'string' && entry.label.length > 0, status + ' 需有 label');
  }
  for (const [tone, def] of Object.entries(CHANNEL_TONE)) {
    assert.ok(typeof def.badgeClass === 'string', tone + ' 需有 badgeClass');
    assert.ok(/^var\(--status-/.test(def.light), tone + ' light 需為 --status-* token');
  }
});
