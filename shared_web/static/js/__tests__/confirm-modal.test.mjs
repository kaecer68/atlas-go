// confirm-modal 元件單元測試（P1-C 危險操作二次確認）
//
// 涵蓋：
//   1. 確認 → 執行 onConfirm + promise resolves true + modal 關閉
//   2. 取消 → 不執行 onConfirm + resolves false
//   3. Esc → 不執行 onConfirm + resolves false
//   4. 點背景 → 等同取消
//   5. danger 樣式預設開啟 / danger=false 走 info 樣式
//   6. 自訂 confirmLabel / cancelLabel / title / message
//   7. 重複呼叫會先關閉前一個（防疊框）
//
// 執行：node --test shared_web/static/js/__tests__/confirm-modal.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// 極簡 fake DOM（只實作元件用到的 API）
// ============================================================================

class FakeEl {
  constructor(tag) {
    this.tagName = String(tag).toUpperCase();
    this.children = [];
    this.style = {};
    this.dataset = {};
    this.parentNode = null;
    this._listeners = {};
    this._classes = new Set();
    this._text = '';
    this.classList = {
      add: (...cs) => cs.forEach((c) => this._classes.add(c)),
      remove: (...cs) => cs.forEach((c) => this._classes.delete(c)),
      toggle: (c, force) => {
        const on = force === undefined ? !this._classes.has(c) : !!force;
        if (on) this._classes.add(c); else this._classes.delete(c);
        return on;
      },
      contains: (c) => this._classes.has(c),
    };
  }
  set className(v) {
    this._className = String(v);
    String(v).split(/\s+/).filter(Boolean).forEach((c) => this._classes.add(c));
  }
  get className() { return this._className || ''; }
  set textContent(v) { this._text = String(v); }
  get textContent() { return this._text; }
  appendChild(c) { c.parentNode = this; this.children.push(c); return c; }
  remove() {
    if (this.parentNode) {
      this.parentNode.children = this.parentNode.children.filter((c) => c !== this);
      this.parentNode = null;
    }
  }
  addEventListener(type, fn) { (this._listeners[type] = this._listeners[type] || []).push(fn); }
  removeEventListener(type, fn) {
    if (this._listeners[type]) this._listeners[type] = this._listeners[type].filter((f) => f !== fn);
  }
  dispatch(type, ev = {}) {
    (this._listeners[type] || []).forEach((fn) => fn({ target: this, preventDefault() {}, ...ev }));
  }
  setAttribute(k, v) { this[k] = v; }
  focus() {
    this._focused = true;
    if (global.document) global.document.activeElement = this;
  }
  querySelector(sel) {
    let cls = null;
    let tag = null;
    if (sel.startsWith('.')) cls = sel.slice(1);
    else if (sel.includes('.')) { const [t, c] = sel.split('.'); tag = t.toUpperCase(); cls = c; }
    else if (sel) tag = sel.toUpperCase();
    // 深度優先搜尋（含後代）
    const stack = [...this.children];
    while (stack.length) {
      const child = stack.shift();
      if (cls && !child.classList.contains(cls)) { stack.unshift(...child.children); continue; }
      if (tag && child.tagName !== tag) { stack.unshift(...child.children); continue; }
      return child;
    }
    return null;
  }
}

function setupDocument() {
  const doc = {
    activeElement: null,
    body: null,
    listeners: {},
    createElement: (tag) => new FakeEl(tag),
    addEventListener(type, fn) { (this.listeners[type] = this.listeners[type] || []).push(fn); },
    removeEventListener(type, fn) {
      if (this.listeners[type]) this.listeners[type] = this.listeners[type].filter((f) => f !== fn);
    },
    dispatch(type, ev = {}) { (this.listeners[type] || []).forEach((fn) => fn({ preventDefault() {}, ...ev })); },
  };
  doc.body = new FakeEl('body');
  global.document = doc;
  return doc;
}

// 每個 test 都重新載入 module（清掉 singleton overlay 狀態）
async function reloadModule() {
  const url = `../components/confirm-modal.js?t=${Date.now()}-${Math.random()}`;
  return import(url);
}

function openOverlay(doc) {
  return doc.body.children[0];
}

// ============================================================================
// 1. 確認才執行
// ============================================================================

test('confirmAction: 點確認 → onConfirm 執行、resolves true、modal 關閉', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  let onConfirmCalls = 0;
  const p = confirmAction({
    title: '全部停用資料通道',
    message: '將停用 5 個通道，系統將停止接收資料，確認？',
    confirmLabel: '確認停用',
    onConfirm: () => { onConfirmCalls += 1; },
  });

  const overlay = openOverlay(global.document);
  assert.ok(overlay, 'overlay 應被 append 到 body');
  assert.ok(overlay.classList.contains('open'), 'overlay 應顯示');
  assert.equal(overlay.role, 'alertdialog', 'aria role 應為 alertdialog');

  const okBtn = overlay.querySelector('.confirm-modal__ok');
  assert.equal(okBtn.textContent, '確認停用');
  okBtn.dispatch('click');

  assert.equal(await p, true, 'promise 應 resolve true');
  assert.equal(onConfirmCalls, 1, 'onConfirm 應被執行一次');
  assert.ok(!overlay.classList.contains('open'), '確認後 overlay 應關閉');
});

test('confirmAction: 取消 → onConfirm 不執行、resolves false', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  let onConfirmCalls = 0;
  const p = confirmAction({ title: 'T', message: 'M', onConfirm: () => { onConfirmCalls += 1; } });

  const overlay = openOverlay(global.document);
  const cancelBtn = overlay.querySelector('.confirm-modal__cancel');
  cancelBtn.dispatch('click');

  assert.equal(await p, false, 'promise 應 resolve false');
  assert.equal(onConfirmCalls, 0, 'onConfirm 不應執行');
  assert.ok(!overlay.classList.contains('open'));
});

test('confirmAction: Esc → onConfirm 不執行、resolves false、overlay 關閉', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  let onConfirmCalls = 0;
  const p = confirmAction({ title: 'T', message: 'M', onConfirm: () => { onConfirmCalls += 1; } });

  global.document.dispatch('keydown', { key: 'Escape' });

  assert.equal(await p, false);
  assert.equal(onConfirmCalls, 0);
  assert.ok(!openOverlay(global.document).classList.contains('open'));
});

test('confirmAction: 非 Esc 按鍵不關閉', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  let onConfirmCalls = 0;
  const p = confirmAction({ title: 'T', message: 'M', onConfirm: () => { onConfirmCalls += 1; } });

  global.document.dispatch('keydown', { key: 'Enter' });

  const overlay = openOverlay(global.document);
  assert.ok(overlay.classList.contains('open'), 'Enter 不應關閉 modal');
  overlay.querySelector('.confirm-modal__ok').dispatch('click');
  assert.equal(await p, true);
  assert.equal(onConfirmCalls, 1);
});

test('confirmAction: 點背景 → 等同取消', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  let onConfirmCalls = 0;
  const p = confirmAction({ title: 'T', message: 'M', onConfirm: () => { onConfirmCalls += 1; } });

  const overlay = openOverlay(global.document);
  overlay.dispatch('click', { target: overlay }); // e.target === overlay = 點背景

  assert.equal(await p, false);
  assert.equal(onConfirmCalls, 0);
  assert.ok(!overlay.classList.contains('open'));
});

// ============================================================================
// 5. danger 樣式
// ============================================================================

test('confirmAction: danger 預設 true → 確認鈕帶 danger class、modal 帶 danger class', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  const p = confirmAction({ title: 'T', message: 'M' });

  const overlay = openOverlay(global.document);
  const modal = overlay.querySelector('.confirm-modal');
  const okBtn = overlay.querySelector('.confirm-modal__ok');
  assert.ok(modal.classList.contains('confirm-modal--danger'));
  assert.ok(!modal.classList.contains('confirm-modal--info'));
  assert.ok(okBtn.classList.contains('danger'));

  okBtn.dispatch('click');
  await p;
});

test('confirmAction: danger=false → info class、確認鈕非 danger', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  const p = confirmAction({ title: 'T', message: 'M', danger: false });

  const overlay = openOverlay(global.document);
  const modal = overlay.querySelector('.confirm-modal');
  const okBtn = overlay.querySelector('.confirm-modal__ok');
  assert.ok(modal.classList.contains('confirm-modal--info'));
  assert.ok(!modal.classList.contains('confirm-modal--danger'));
  assert.ok(!okBtn.classList.contains('danger'));

  okBtn.dispatch('click');
  await p;
});

// ============================================================================
// 6. 內容與按鈕文字
// ============================================================================

test('confirmAction: title/message/confirmLabel/cancelLabel 正確帶入', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  const p = confirmAction({
    title: '啟動回測',
    message: '將對 2026-08-01 ~ 2026-08-07 執行完整回測',
    confirmLabel: '確認啟動',
    cancelLabel: '先不要',
  });

  const overlay = openOverlay(global.document);
  assert.equal(overlay.querySelector('.confirm-modal__title').textContent, '啟動回測');
  assert.equal(overlay.querySelector('.confirm-modal__message').textContent, '將對 2026-08-01 ~ 2026-08-07 執行完整回測');
  assert.equal(overlay.querySelector('.confirm-modal__ok').textContent, '確認啟動');
  assert.equal(overlay.querySelector('.confirm-modal__cancel').textContent, '先不要');

  overlay.querySelector('.confirm-modal__ok').dispatch('click');
  await p;
});

// ============================================================================
// 7. 重複呼叫防疊框
// ============================================================================

test('confirmAction: 前一個還開著時再次呼叫 → 前一個以取消收尾', async () => {
  setupDocument();
  const { confirmAction } = await reloadModule();
  let firstOnConfirm = 0;
  const first = confirmAction({ title: 'A', message: 'M1', onConfirm: () => { firstOnConfirm += 1; } });
  const second = confirmAction({ title: 'B', message: 'M2', onConfirm: () => {} });

  // 第一個應已被 settle(false)，第二個正常開著
  assert.equal(await first, false, '前一個應 resolve false');
  assert.equal(firstOnConfirm, 0, '前一個 onConfirm 不應執行');
  const overlay = openOverlay(global.document);
  assert.ok(overlay.classList.contains('open'));
  assert.equal(overlay.querySelector('.confirm-modal__title').textContent, 'B');

  overlay.querySelector('.confirm-modal__ok').dispatch('click');
  assert.equal(await second, true);
});
