// shared_web/static/js/__tests__/login-reminder-modal.test.mjs
//
// 登入提醒 modal 單測（Track A）：24h localStorage 去重 + 彈出/關閉。
// 邏輯在 client_web/static/js/components/login-reminder-modal.js。
//
// 執行：cd client_web && npm test

import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  shouldShowReminder,
  showLoginReminderModal,
  closeLoginReminderModal,
  REMINDER_STORAGE_KEY,
  REMINDER_INTERVAL_MS,
} from '../../../../client_web/static/js/components/login-reminder-modal.js';

test('login-reminder: 24h 間隔常數 = 24 小時', () => {
  assert.equal(REMINDER_INTERVAL_MS, 24 * 60 * 60 * 1000);
});

test('login-reminder: shouldShowReminder — 無紀錄 → 彈', () => {
  assert.equal(shouldShowReminder(1000000, null), true, '無 localStorage 紀錄應可彈');
  assert.equal(shouldShowReminder(1000000, ''), true, '空字串視為無紀錄');
});

test('login-reminder: shouldShowReminder — 24h 內去重，超過才再彈', () => {
  const now = Date.parse('2026-08-21T12:00:00Z');
  const recent = new Date(now - 23 * 60 * 60 * 1000).toISOString();
  assert.equal(shouldShowReminder(now, recent), false, '23h 內不應再彈');
  const older = new Date(now - 25 * 60 * 60 * 1000).toISOString();
  assert.equal(shouldShowReminder(now, older), true, '超過 24h 可再彈');
  assert.equal(shouldShowReminder(now, 'not-a-date'), true, '壞格式視為無紀錄');
});

test('login-reminder: 未登入 → 彈出並寫 localStorage；24h 內第二次不彈', async () => {
  // fake document + localStorage + 401 profile（未登入）
  const created = [];
  const fakeButtons = {
    addEventListener() {},
  };
  const fakeOverlay = {
    id: '',
    className: '',
    innerHTML: '',
    setAttribute() {},
    addEventListener() {},
    querySelector() { return fakeButtons; },
    parentNode: null,
  };
  globalThis.document = {
    createElement(tag) { const el = { tagName: tag, classList: { add() {} }, appendChild() {}, setAttribute() {}, addEventListener() {}, querySelector() { return fakeButtons; } }; created.push(el); return el; },
    getElementById() { return null; },
    body: { appendChild() {} },
  };
  const store = {};
  globalThis.localStorage = {
    getItem(k) { return k in store ? store[k] : null; },
    setItem(k, v) { store[k] = String(v); },
    removeItem(k) { delete store[k]; },
  };
  globalThis.fetch = async function () {
    return new Response('', { status: 401 });
  };
  try {
    const shown1 = await showLoginReminderModal();
    assert.equal(shown1, true, '未登入且無紀錄應彈出');
    assert.ok(store[REMINDER_STORAGE_KEY], '應寫入 localStorage 去重 flag');
    // 24h 內第二次（同 timestamp）不彈
    const shown2 = await showLoginReminderModal();
    assert.equal(shown2, false, '24h 內不應重複彈出');
    closeLoginReminderModal();
    assert.ok(true, 'close 不拋錯');
  } finally {
    delete globalThis.document;
    delete globalThis.localStorage;
    delete globalThis.fetch;
  }
});
