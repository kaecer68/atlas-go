// shared_web/static/js/__tests__/api-key.test.mjs
//
// 驗證 admin_web services/api-key.js（PR-7 去重後）:
//   - getApiKey/setApiKey/hasApiKey/ensureApiKey 是 app-utils
//     getAtlasApiKey/setAtlasApiKey 的薄 re-export（同一 localStorage key,
//     不再有重複的 localStorage 實作）
//   - modal 邏輯（show/hide/initApiKeyPrompt）保留為活碼
//
// 執行：node --test shared_web/static/js/__tests__/api-key.test.mjs
// （api-key.js 用跨樹相對 import 指向 shared app-utils，node 可直接解析）

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import {
  getApiKey, setApiKey, hasApiKey, ensureApiKey,
  showApiKeyPrompt, hideApiKeyPrompt, initApiKeyPrompt,
} from '../../../../admin_web/static/js/services/api-key.js';
import { getAtlasApiKey, setAtlasApiKey } from '../shared/app-utils.js';

// Minimal localStorage mock（與 app-utils.test.mjs 同一份實作）。
const store = new Map();
global.localStorage = {
  getItem: (k) => store.get(k) ?? null,
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
};

// document mock：apiKeyModal / apiKeySave / apiKeyInput
function makeFakeDocument() {
  const modal = { classList: { _set: new Set(['hidden']), add(c) { this._set.add(c); }, remove(c) { this._set.delete(c); } } };
  const saveBtn = { listeners: {}, addEventListener(type, fn) { this.listeners[type] = fn; }, removeEventListener() {}, click() { if (this.listeners.click) this.listeners.click(); } };
  const input = { value: '', listeners: {}, addEventListener(type, fn) { this.listeners[type] = fn; } };
  const doc = {
    modal,
    saveBtn,
    input,
    getElementById(id) {
      if (id === 'apiKeyModal') return modal;
      if (id === 'apiKeySave') return saveBtn;
      if (id === 'apiKeyInput') return input;
      return null;
    },
  };
  return doc;
}

const originalDocument = global.document;
function setDocument(doc) { global.document = doc; }
function restoreDocument() {
  if (originalDocument === undefined) delete global.document;
  else global.document = originalDocument;
}
afterEach(() => {
  store.clear();
  restoreDocument();
});

// ---- re-export 語意（與 app-utils 共用同一 localStorage key）----

test('getApiKey 讀取 app-utils 的 getAtlasApiKey（同一 key 單一來源）', () => {
  setAtlasApiKey('secret-abc');
  assert.equal(getApiKey(), 'secret-abc');
  assert.equal(getAtlasApiKey(), getApiKey());
});

test('setApiKey 寫入 app-utils 的 setAtlasApiKey（無重複 localStorage 邏輯）', () => {
  setApiKey('secret-xyz');
  assert.equal(getAtlasApiKey(), 'secret-xyz');
  assert.equal(getApiKey(), 'secret-xyz');
});

test('setApiKey(空字串) 清除 key', () => {
  setApiKey('secret-xyz');
  setApiKey('');
  assert.equal(getApiKey(), '');
  assert.equal(hasApiKey(), false);
});

test('hasApiKey 反映儲存狀態', () => {
  assert.equal(hasApiKey(), false);
  setApiKey('k1');
  assert.equal(hasApiKey(), true);
});

test('ensureApiKey: 已有 key 時直接回 true 不開 modal', () => {
  setApiKey('k1');
  let shown = 0;
  const origShow = showApiKeyPrompt;
  // 不 stub — ensureApiKey 只會在無 key 時才呼叫 showApiKeyPrompt
  const doc = makeFakeDocument();
  setDocument(doc);
  assert.equal(ensureApiKey(), true);
  assert.equal(shown, 0);
  assert.equal(origShow, showApiKeyPrompt);
});

test('ensureApiKey: 無 key 時開 modal 並回 false（未輸入前）', () => {
  const doc = makeFakeDocument();
  setDocument(doc);
  assert.equal(ensureApiKey(), false);
  assert.equal(doc.modal.classList._set.has('hidden'), false, 'modal 應已顯示');
});

// ---- modal 邏輯（活碼，保留）----

test('showApiKeyPrompt / hideApiKeyPrompt 切換 modal visible', () => {
  const doc = makeFakeDocument();
  setDocument(doc);
  showApiKeyPrompt();
  assert.equal(doc.modal.classList._set.has('hidden'), false, 'show → modal 可見');
  hideApiKeyPrompt();
  assert.equal(doc.modal.classList._set.has('hidden'), true, 'hide → modal 隱藏');
});

test('initApiKeyPrompt: 存檔按鈕寫入 key 並關閉 modal', () => {
  const doc = makeFakeDocument();
  setDocument(doc);
  initApiKeyPrompt();
  doc.input.value = 'typed-key';
  doc.saveBtn.click();
  assert.equal(getApiKey(), 'typed-key', 'save 應寫入 key（經 setApiKey → setAtlasApiKey）');
  assert.equal(doc.modal.classList._set.has('hidden'), true, 'save 後 modal 應關閉');
});
