// shared_web/static/js/__tests__/auth-guest-mode.test.mjs
//
// GUEST_MODE=false（會員制上線）的 auth.js 分支測試：
//   - initAuth 不再自動把未登入訪客 promotion 成 guest
//   - isLoggedIn() 未登入 → false；getTier() → null
//   - renderNavState 走「非 guest」分支：未登入顯示帳戶 nav-guest（登入/註冊），
//     登入後顯示 nav-user + tier badge
//
// 執行：cd client_web && npm test（node --test ../shared_web/static/js/__tests__/*.mjs）

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ── DOM stubs ───────────────────────────────────────────────────────────────
const elements = new Map();

function makeEl(id) {
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    className: '',
    classSet: new Set(),
    classList: {
      add: (...cls) => cls.forEach(c => el.classSet.add(c)),
      remove: (...cls) => cls.forEach(c => el.classSet.delete(c)),
      contains: (c) => el.classSet.has(c),
      toggle: (c, force) => {
        const add = force === undefined ? !el.classSet.has(c) : !!force;
        if (add) el.classSet.add(c); else el.classSet.delete(c);
        return add;
      },
    },
    addEventListener() {},
  };
  elements.set(id, el);
  return el;
}

global.document = {
  cookie: '',
  getElementById(id) { return elements.get(id) || null; },
  querySelectorAll(sel) {
    const out = [];
    for (const el of elements.values()) {
      if (el._selectorTags && el._selectorTags.includes(sel)) out.push(el);
    }
    return out;
  },
  createElement(tag) { return makeEl(`created-${tag}-${Math.random()}`); },
  querySelector() { return null; },
  addEventListener() {},
  body: { appendChild() {}, addEventListener() {} },
};

// ── fetch stub ───────────────────────────────────────────────────────────────
let fetchImpl = null;
global.fetch = (url, init) => {
  const impl = fetchImpl || (async () => ({ ok: true, status: 200, json: async () => ({}) }));
  return impl(url, init);
};
function stubFetch(handler) { fetchImpl = handler; }
const ok = (body) => async () => ({ ok: true, status: 200, json: async () => body });
const unauthorized = () => async () => { const e = new Error('401'); e.status = 401; throw e; };

// 載入 auth.js（module singleton）
const auth = await import('../services/auth.js');

test('GUEST_MODE=false: isLoggedIn()/getTier() 未登入訪客不自動變 guest', async () => {
  auth.invalidateAuth();
  stubFetch(unauthorized());
  const loggedIn = await auth.isLoggedIn();
  assert.equal(loggedIn, false, '未登入訪客 isLoggedIn 應為 false');
  const tier = await auth.getTier();
  assert.equal(tier, null, '未登入訪客 getTier 應為 null');
});

test('GUEST_MODE=false: initAuth() 未登入不 promotion 成 guest', async () => {
  auth.invalidateAuth();
  stubFetch(unauthorized());
  const valid = await auth.initAuth();
  assert.equal(valid, false, 'initAuth 不應把未登入訪客 promotion 成 guest');
});

test('GUEST_MODE=false: 登入後 isLoggedIn() true、getTier() 回真實 tier', async () => {
  auth.invalidateAuth();
  stubFetch(ok({ email: 'member@example.com', effective_tier: 'registered' }));
  const loggedIn = await auth.isLoggedIn();
  assert.equal(loggedIn, true, '登入後 isLoggedIn 應為 true');
  const tier = await auth.getTier();
  assert.equal(tier, 'registered', 'getTier 應回傳後端 tier');
});

test('GUEST_MODE=false: renderNavState 未登入 → 顯示 nav-guest、隱藏 nav-user、帳戶 section 不隱藏', async () => {
  // setup nav elements
  const accountSection = makeEl('navAccountSection');
  const tierBadge = makeEl('navTierBadge');
  const tierLabel = makeEl('navTierLabel');
  const guest1 = makeEl('nav-guest-item-1'); guest1._selectorTags = ['.nav-guest'];
  const guest2 = makeEl('nav-guest-item-2'); guest2._selectorTags = ['.nav-guest'];
  const user1 = makeEl('nav-user-item-1'); user1._selectorTags = ['.nav-user'];

  auth.invalidateAuth();
  stubFetch(unauthorized());
  await auth.renderNavState();

  assert.equal(accountSection.classSet.has('hidden'), false, '非 guest 模式不應隱藏整個帳戶 section');
  assert.equal(guest1.classSet.has('hidden'), false, '未登入時 nav-guest（登入/註冊）應顯示');
  assert.equal(user1.classSet.has('hidden'), true, '未登入時 nav-user 應隱藏');
  assert.equal(tierBadge.textContent, '', '未登入 tier 為 null，badge 不應被 renderNavState 改寫');
});

test('GUEST_MODE=false: renderNavState 登入後 → 顯示 nav-user、隱藏 nav-guest、tier badge 對應 tier', async () => {
  elements.clear();
  const accountSection = makeEl('navAccountSection');
  const tierBadge = makeEl('navTierBadge');
  const guest1 = makeEl('nav-guest-item-1'); guest1._selectorTags = ['.nav-guest'];
  const user1 = makeEl('nav-user-item-1'); user1._selectorTags = ['.nav-user'];

  auth.invalidateAuth();
  stubFetch(ok({ email: 'premium@example.com', effective_tier: 'premium' }));
  await auth.renderNavState();

  assert.equal(accountSection.classSet.has('hidden'), false, '登入後不應隱藏帳戶 section');
  assert.equal(guest1.classSet.has('hidden'), true, '登入後 nav-guest（登入/註冊）應隱藏');
  assert.equal(user1.classSet.has('hidden'), false, '登入後 nav-user 應顯示');
  assert.equal(tierBadge.textContent, 'Premium', 'premium tier badge 應為 Premium');
});
