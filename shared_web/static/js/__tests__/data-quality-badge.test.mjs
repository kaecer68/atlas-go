// shared_web/static/js/__tests__/data-quality-badge.test.mjs
//
// fix/20260924-channel-status-truth — 首頁資料品質徽章。
//
// 修復前: `ch.status !== 'ok'` 一律掛紅色「資料異常」。
//   → stale(資料過期) 被說成「異常」，inactive(未啟用) 也被算進去。
// 修復後: 徽章文字 = 最嚴重狀態自己的 label（異常 / 資料過期 / 降級 / 待更新 /
//   未知），inactive 不掛徽章，unknown 仍要露面（不得靜默當正常）。
//
// 執行: node --test shared_web/static/js/__tests__/data-quality-badge.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { dataQualityBadge, buildChannelMap } from '../components/data-quality-badge.js';

const MAP = buildChannelMap([
  { channel_id: 'twse_oddlot', label: 'TWSE 零股', status: 'stale', status_text: '資料過期' },
  { channel_id: 'finmind', label: 'FinMind', status: 'ok', status_text: '正常' },
  { channel_id: 'tej', label: 'TEJ', status: 'inactive', status_text: '未啟用' },
  { channel_id: 'twse_etf', label: 'TWSE ETF', status: 'error', status_text: '異常' },
  { channel_id: 'fugle', label: 'Fugle', status: 'warn', status_text: '待更新' },
  { channel_id: 'mystery', label: '神秘通道', status: 'no-such-status' },
]);

test('全部正常 → 不掛徽章', () => {
  assert.equal(dataQualityBadge(MAP, ['finmind']), '');
});

test('未啟用 (inactive) 不算資料問題 → 不掛徽章', () => {
  assert.equal(dataQualityBadge(MAP, ['tej']), '');
});

test('stale → 徽章文字「資料過期」，不再是「資料異常」', () => {
  const html = dataQualityBadge(MAP, ['twse_oddlot']);
  assert.ok(html.includes('>資料過期</span>'), '徽章應顯示 資料過期，實際: ' + html);
  assert.ok(!html.includes('>資料異常</span>'), 'stale 不得顯示為資料異常');
  assert.ok(html.includes('TWSE 零股（資料過期）'), 'title 應列出通道與其狀態: ' + html);
});

test('warn → 徽章文字「待更新」', () => {
  const html = dataQualityBadge(MAP, ['fugle']);
  assert.ok(html.includes('>待更新</span>'), '實際: ' + html);
});

test('error 優先於 stale → 文字「異常」且 title 同時列出兩者', () => {
  const html = dataQualityBadge(MAP, ['twse_oddlot', 'twse_etf']);
  assert.ok(html.includes('>異常</span>'), '最嚴重為 error → 異常，實際: ' + html);
  assert.ok(html.includes('TWSE ETF（異常）') && html.includes('TWSE 零股（資料過期）'), '實際: ' + html);
});

test('未知狀態仍會掛徽章（不得靜默當正常）', () => {
  const html = dataQualityBadge(MAP, ['mystery']);
  assert.ok(html.includes('>未知</span>'), '實際: ' + html);
  assert.ok(html.includes('神秘通道（未知）'), '實際: ' + html);
});

test('未知狀態不會把已知的 stale 蓋成未知 → 取最嚴重', () => {
  const html = dataQualityBadge(MAP, ['mystery', 'twse_oddlot']);
  assert.ok(html.includes('>資料過期</span>'), 'stale 比 unknown 嚴重，實際: ' + html);
});

test('channelIds 中的未知 id / 空輸入 → 不 throw 且不掛徽章', () => {
  assert.equal(dataQualityBadge(MAP, ['not-in-map']), '');
  assert.equal(dataQualityBadge(MAP, []), '');
  assert.equal(dataQualityBadge(null, ['finmind']), '');
  assert.equal(dataQualityBadge(MAP, undefined), '');
});

test('title 有做 HTML 脫逸（通道名稱含 < > 不會溢出）', () => {
  const map = buildChannelMap([{ channel_id: 'x', label: '<img>', status: 'stale' }]);
  const html = dataQualityBadge(map, ['x']);
  assert.ok(!html.includes('<img>'), '實際: ' + html);
  assert.ok(html.includes('&lt;img&gt;'), '實際: ' + html);
});
