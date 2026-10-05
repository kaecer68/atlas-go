// shared_web/static/js/__tests__/datachannels-attention-categories.test.mjs
//
// 2026-10-05 — 前端「資料通道」分類缺陷（業主發現；三件一 PR）。
//
// 驗收（對應後端分類器 internal/monitoring/service/channel_attention.go）:
//   ② 退役通道（設計退休）不出現在「需關注」，理由降為資訊級
//   ③ 「需關注」分為 系統錯誤／已知上游限制／預期等待 三類，管理者一眼可辨
//
// 執行: node --test shared_web/static/js/__tests__/datachannels-attention-categories.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

const elements = new Map();

function makeEl(id) {
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    classList: { add() {}, remove() {}, contains() { return false; } },
    addEventListener() {},
    querySelector() { return null; },
  };
  elements.set(id, el);
  return el;
}

globalThis.document = {
  getElementById(id) {
    if (!elements.has(id)) makeEl(id);
    return elements.get(id);
  },
  createElement() {
    return {
      _t: '',
      get textContent() { return this._t; },
      set textContent(v) { this._t = String(v); },
      get innerHTML() {
        return this._t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
      },
    };
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
};

const { renderDataChannels } = await import('../pages/datachannels.js');
const { channelStatusMeta, attentionCategoryMeta, ATTENTION_CATEGORY_ORDER } = await import('../shared/channel-status.js');

function channel(id, status, statusText, extra) {
  return Object.assign({
    channel_id: id,
    country: '台灣',
    platform: id + ' 平台',
    api_format: 'REST JSON',
    path: '/x',
    storage: 'db',
    status,
    status_text: statusText,
    updated_at: '2026-10-05 10:00:00',
    enabled: true,
  }, extra || {});
}

function render(channels, alerts) {
  renderDataChannels({ channels, alerts: alerts || [], generated: '2026-10-05 12:00:00' });
  return elements.get('dataChannels').innerHTML;
}

function attentionBlock(html) {
  const i = html.indexOf('需要關注的通道');
  return i === -1 ? '' : html.slice(i);
}

// ============================================================================
// ② 退役通道 = 決策，不是事故
// ============================================================================

test('retired 狀態：SSOT 標為已退役、muted、且不屬於需關注', () => {
  const meta = channelStatusMeta('retired');
  assert.equal(meta.label, '已退役');
  assert.equal(meta.tone, 'muted');
  assert.equal(meta.badgeClass, 'muted');
  assert.equal(meta.needsAttention, false, '退役是設計決策，不得進需關注');
  assert.equal(meta.normal, false, '退役不是「正常」（它根本不抓取）');
  assert.equal(meta.abnormal, false, '退役也不是異常');
});

test('退役通道：列上顯示 muted「已退役」，理由為資訊級（info），且計入另一行的已退役 N', () => {
  const html = render([
    channel('us_yahoo', 'ok', '正常'),
    channel('twse_oddlot', 'retired', '已退役', {
      last_error: '已退役（2026-09-29）：TWSE 零股交易報表已移除；替代輸入：twse_capital_flow 代理',
      error_severity: 'info',
    }),
  ]);
  assert.ok(html.includes('<span class="badge muted">已退役</span>'), '實際: ' + html.slice(0, 500));
  assert.ok(html.includes('var(--muted)'), '資訊級理由必須用 muted 色，不是紅/黃');
  assert.ok(html.includes('已退役 1'), '摘要必須明示已退役 N（不算正常也不算異常）');
});

test('退役通道即使被後端放進 alerts 也不進「需關注」', () => {
  const html = render(
    [channel('twse_oddlot', 'retired', '已退役')],
    [{ channel_id: 'twse_oddlot', status: 'retired', error: '已退役（2026-09-29）', category: 'retired' }],
  );
  assert.equal(attentionBlock(html), '', '退役列不得出現在需關注區塊: ' + html.slice(-500));
});

// ============================================================================
// ③ 三分類
// ============================================================================

test('三分類的標題／順序是固定的（我們的問題 → 上游的問題 → 時間還沒到）', () => {
  assert.deepEqual(ATTENTION_CATEGORY_ORDER, ['system_error', 'upstream_limit', 'expected_wait']);
  assert.equal(attentionCategoryMeta('system_error').title, '系統錯誤（我們的問題）');
  assert.equal(attentionCategoryMeta('upstream_limit').title, '已知上游限制（配額／tier）');
  assert.equal(attentionCategoryMeta('expected_wait').title, '預期等待（日曆未到）');
  // 未分類一律退回「系統錯誤」（fail-safe：寧可誤指自己，也不靜默略過）。
  assert.equal(attentionCategoryMeta(undefined).key, 'system_error');
  assert.equal(attentionCategoryMeta('nonsense').key, 'system_error');
});

test('finmind／tdcc 顯示為「已知上游限制」，不是泛用警告', () => {
  const html = render(
    [
      channel('finmind', 'warn', '待更新'),
      channel('tdcc_equity_dispersion', 'warn', '待更新'),
    ],
    [
      { channel_id: 'finmind', status: 'warn', error: 'finmind: daily quota exhausted (402)', category: 'upstream_limit' },
      { channel_id: 'tdcc_equity_dispersion', status: 'warn', error: 'finmind: daily quota exhausted (402)', category: 'upstream_limit' },
    ],
  );
  const block = attentionBlock(html);
  assert.ok(block.includes('已知上游限制（配額／tier）（2）'), '實際: ' + block);
  assert.ok(block.includes('配額重置'), '必須說明「等上游恢復或配額重置」');
  assert.ok(!block.includes('系統錯誤（我們的問題）'), '上游限制不得被歸成我們的問題');
});

test('週末的 twse_sbl／government_broker 顯示為「預期等待」，且區塊不標紅', () => {
  const html = render(
    [
      channel('twse_sbl', 'stale', '資料過期'),
      channel('government_broker', 'stale', '資料過期'),
    ],
    [
      { channel_id: 'twse_sbl', status: 'stale', error: '', category: 'expected_wait' },
      { channel_id: 'government_broker', status: 'stale', error: '', category: 'expected_wait' },
    ],
  );
  const block = attentionBlock(html);
  assert.ok(block.includes('預期等待（日曆未到）（2）'), '實際: ' + block);
  assert.ok(block.includes('沒有發布機會'), '必須說明「上游在該期間沒有發布機會」');
  assert.ok(!block.includes('系統錯誤（我們的問題）'), '日曆未到不是我們的問題');
});

test('真正的故障留在「系統錯誤」，且區塊維持紅色（最嚴重者決定）', () => {
  const html = render(
    [channel('us_yahoo', 'error', '異常')],
    [{ channel_id: 'us_yahoo', status: 'error', error: 'dial tcp: connection refused', category: 'system_error' }],
  );
  const block = attentionBlock(html);
  assert.ok(block.includes('系統錯誤（我們的問題）（1）'), '實際: ' + block);
  assert.ok(html.includes('border-left:3px solid var(--color-danger)'), '含我們的問題 ⇒ 紅色框');
});

test('三類同時存在時：三段都出現，且區塊顏色由最嚴重者決定', () => {
  const html = render(
    [
      channel('us_yahoo', 'error', '異常'),
      channel('finmind', 'warn', '待更新'),
      channel('twse_sbl', 'stale', '資料過期'),
    ],
    [
      { channel_id: 'us_yahoo', status: 'error', error: 'boom', category: 'system_error' },
      { channel_id: 'finmind', status: 'warn', error: 'quota', category: 'upstream_limit' },
      { channel_id: 'twse_sbl', status: 'stale', error: '', category: 'expected_wait' },
    ],
  );
  const block = attentionBlock(html);
  const iSystem = block.indexOf('系統錯誤（我們的問題）');
  const iUpstream = block.indexOf('已知上游限制（配額／tier）');
  const iWait = block.indexOf('預期等待（日曆未到）');
  assert.ok(iSystem !== -1 && iUpstream !== -1 && iWait !== -1, '三段都必須出現: ' + block);
  assert.ok(iSystem < iUpstream && iUpstream < iWait, '順序必須是 系統錯誤 → 上游限制 → 預期等待');
  assert.ok(html.includes('border-left:3px solid var(--color-danger)'), '有系統錯誤 ⇒ 紅色框');
});

test('缺 category 的舊 payload：仍顯示（歸在系統錯誤）但區塊顏色沿用狀態色（不因預設而變紅）', () => {
  const html = render(
    [channel('twse_sbl', 'stale', '資料過期')],
    [{ channel_id: 'twse_sbl', status: 'stale', error: '' }],
  );
  const block = attentionBlock(html);
  assert.ok(block.includes('系統錯誤（我們的問題）（1）'), '未分類不得被隱藏: ' + block);
  assert.ok(html.includes('border-left:3px solid var(--warn)'), 'stale-only 區塊不得用紅色框（2026-09-24 既有規則）: ' + block);
  assert.ok(!html.includes('border-left:3px solid var(--color-danger)'), 'stale 不得觸發紅色區塊');
});
