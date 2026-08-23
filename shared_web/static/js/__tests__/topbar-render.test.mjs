// shared_web/static/js/__tests__/topbar-render.test.mjs
//
// top bar 雙模式渲染單測（Track A）：mock isLoggedIn()（透過 stub fetch
// /api/user/profile）測 renderTopBar 的 marketing（未登入）與 member
// （已登入）兩種 HTML，以及 getRedirectUrl / renderNavState 串接。
//
// 執行：cd client_web && npm test

import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  renderTopBar,
  renderNavState,
  getRedirectUrl,
  invalidateAuth,
  MEMBER_BASE_URL,
} from '../services/auth.js';

function makeFakeDom() {
  const topbarMenu = {
    _html: '',
    set innerHTML(v) { this._html = v; },
    get innerHTML() { return this._html; },
    querySelector() { return null; },
    addEventListener() {},
  };
  const els = {
    topbarMenu,
    navAccountSection: { classList: { add() {}, remove() {}, toggle() {} } },
    navTierBadge: { textContent: '', className: '' },
    navTierLabel: { textContent: '', className: '', classList: { remove() {} } },
  };
  globalThis.document = {
    getElementById(id) { return els[id] || null; },
    querySelectorAll() { return []; },
  };
  return { els };
}

function stubProfileFetch(email, tier) {
  globalThis.fetch = async function (url) {
    if (String(url).includes('/api/user/profile')) {
      return new Response(JSON.stringify({ email, effective_tier: tier }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    }
    return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
  };
}

function stub401Fetch() {
  globalThis.fetch = async function () {
    return new Response('', { status: 401 });
  };
}

function restoreFetch() {
  delete globalThis.fetch;
}

test('getRedirectUrl: 組 member.goluck.uk/login?redirect=<encoded current>', () => {
  const url = getRedirectUrl('https://atlas.goluck.uk/client/home?foo=1&bar=%20');
  assert.ok(url.startsWith(MEMBER_BASE_URL + '/login?redirect='), 'prefix 應為 member login');
  const redirect = url.split('redirect=')[1];
  assert.equal(decodeURIComponent(redirect), 'https://atlas.goluck.uk/client/home?foo=1&bar=%20', 'redirect 值應為原 URL');
});

test('getRedirectUrl: 無 window 時 fallback 到 atlas 首頁', () => {
  const savedWindow = globalThis.window;
  delete globalThis.window;
  try {
    const url = getRedirectUrl();
    assert.equal(url, MEMBER_BASE_URL + '/login?redirect=' + encodeURIComponent('https://atlas.goluck.uk/client/home'));
  } finally {
    if (savedWindow !== undefined) globalThis.window = savedWindow;
  }
});

test('getRedirectUrl: 有 window 時用目前 location（origin + path + search）', () => {
  const savedWindow = globalThis.window;
  globalThis.window = { location: { origin: 'https://atlas.goluck.uk', pathname: '/client/stock-quote', search: '?symbol=2330' } };
  try {
    const url = getRedirectUrl();
    assert.equal(decodeURIComponent(url.split('redirect=')[1]), 'https://atlas.goluck.uk/client/stock-quote?symbol=2330');
  } finally {
    if (savedWindow !== undefined) globalThis.window = savedWindow;
  }
});

test('renderTopBar: 未登入 → marketing menu + 金色 Login 按鈕', async () => {
  makeFakeDom();
  stub401Fetch();
  invalidateAuth();
  try {
    await renderTopBar();
    const html = globalThis.document.getElementById('topbarMenu').innerHTML;
    assert.ok(html.includes('topbar__menu--marketing'), '應為 marketing 模式');
    for (const label of ['Why Atlas', '會員方案', '社群學習', '問答提示']) {
      assert.ok(html.includes(label), '應含 menu: ' + label);
    }
    assert.ok(html.includes('topbar__login-btn'), '應含 Login 按鈕');
    assert.ok(html.includes('href="' + getRedirectUrl() + '"'), 'Login 指向 member login?redirect');
    assert.ok(!html.includes('topbar__menu--member'), '不應出現 member 模式');
  } finally {
    restoreFetch();
    delete globalThis.document;
  }
});

test('renderTopBar: 已登入 → 只有【會員中心】入口', async () => {
  makeFakeDom();
  stubProfileFetch('user@example.com', 'premium');
  invalidateAuth();
  try {
    await renderTopBar();
    const html = globalThis.document.getElementById('topbarMenu').innerHTML;
    assert.ok(html.includes('topbar__menu--member'), '應為 member 模式');
    assert.ok(html.includes('會員中心'), '應含會員中心入口');
    assert.ok(html.includes('href="' + MEMBER_BASE_URL + '/dashboard"'), '會員中心應指向 member dashboard');
    // 2026-08-24：top bar 只保留會員中心 — 不應再有行銷 menu / user chip / 登出
    assert.ok(!html.includes('topbar__login-btn'), '已登入不應顯示 Login 按鈕');
    assert.ok(!html.includes('topbar__user-email'), '不應顯示 user email');
    assert.ok(!html.includes('topbar__logout-btn'), '登出改由側欄負責，top bar 不顯示');
    assert.ok(!html.includes('Why Atlas'), '不應出現行銷 menu');
  } finally {
    restoreFetch();
    delete globalThis.document;
  }
});

test('renderNavState: 末尾串接 renderTopBar（單一入口）', async () => {
  makeFakeDom();
  stubProfileFetch('nav@example.com', 'registered');
  invalidateAuth();
  try {
    await renderNavState();
    const html = globalThis.document.getElementById('topbarMenu').innerHTML;
    assert.ok(html.includes('topbar__menu--member'), 'renderNavState 後 topbar 應同步渲染');
    assert.ok(html.includes('會員中心'), 'topbar 應顯示會員中心入口');
  } finally {
    restoreFetch();
    delete globalThis.document;
  }
});
