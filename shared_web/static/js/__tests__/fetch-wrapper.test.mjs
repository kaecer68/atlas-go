// Tests for shared_web/static/js/shared/fetch-wrapper.js
// Run with: node --test ../shared_web/static/js/__tests__/*.mjs
// (from client_web/ or shared_web/ or admin_web/ root)

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { install401Interceptor, is401InterceptorInstalled } from '../shared/fetch-wrapper.js';

function makeFakeWindow() {
  const calls = [];
  const win = {
    location: { pathname: '/client/home' },
    fetch: function (input, init) {
      calls.push({ input, init });
      // Default: 200 OK. Override per test.
      return Promise.resolve({ status: 200, ok: true });
    },
  };
  return { win, calls };
}

test('install401Interceptor: 200 OK does not call onUnauthorized', async () => {
  const { win, calls } = makeFakeWindow();
  let called = false;
  install401Interceptor({
    windowObj: win,
    onUnauthorized: () => {
      called = true;
    },
    switchPage: () => {},
  });
  await win.fetch('/api/foo');
  assert.equal(called, false, 'onUnauthorized should not fire on 200');
  assert.equal(calls.length, 1, 'underlying fetch should be called once');
});

test('install401Interceptor: 401 triggers onUnauthorized + switchPage', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let unauthorized = 0;
  let switchedTo = null;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'login',
    excludedPages: ['login', 'register'],
    onUnauthorized: () => {
      unauthorized++;
    },
    switchPage: (id) => {
      switchedTo = id;
    },
  });
  await win.fetch('/api/control/active-overrides');
  assert.equal(unauthorized, 1, 'onUnauthorized should fire once');
  assert.equal(switchedTo, 'login', 'should redirect to login');
});

test('install401Interceptor: 401 on excluded page does NOT switchPage', async () => {
  const { win } = makeFakeWindow();
  win.location.pathname = '/client/login';
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let switched = 0;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'login',
    excludedPages: ['login', 'register'],
    onUnauthorized: () => {},
    switchPage: () => {
      switched++;
    },
  });
  await win.fetch('/api/auth/login');
  assert.equal(switched, 0, 'switchPage should not fire on excluded page');
});

test('install401Interceptor: idempotent — second call returns noop', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let count = 0;
  const opts = {
    windowObj: win,
    onUnauthorized: () => {
      count++;
    },
    switchPage: () => {},
  };
  install401Interceptor(opts);
  const uninstall1 = install401Interceptor(opts); // second call → noop
  await win.fetch('/api/x');
  assert.equal(count, 1, 'onUnauthorized should fire only once even with double install');
  assert.equal(typeof uninstall1, 'function', 'second install returns uninstall function');
});

test('install401Interceptor: uninstall restores original fetch', async () => {
  const { win } = makeFakeWindow();
  const origFetch = win.fetch;
  let unauthorized = 0;
  const uninstall = install401Interceptor({
    windowObj: win,
    onUnauthorized: () => {
      unauthorized++;
    },
    switchPage: () => {},
  });
  uninstall();
  assert.equal(win.fetch, origFetch, 'window.fetch should be restored to original');
  assert.equal(is401InterceptorInstalled(win), false, 'flag should be cleared');
  // 401 after uninstall should NOT call onUnauthorized
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  await win.fetch('/api/x');
  assert.equal(unauthorized, 0, 'after uninstall, 401 should not trigger onUnauthorized');
});

test('install401Interceptor: supports custom loginPageId for admin', async () => {
  const { win } = makeFakeWindow();
  win.location.pathname = '/admin/home';
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let switchedTo = null;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'admin-login',
    excludedPages: ['admin-login'],
    switchPage: (id) => {
      switchedTo = id;
    },
    onUnauthorized: () => {},
  });
  await win.fetch('/api/control/active-overrides');
  assert.equal(switchedTo, 'admin-login', 'admin should redirect to admin-login page');
});

test('is401InterceptorInstalled: returns false initially, true after install', () => {
  const { win } = makeFakeWindow();
  assert.equal(is401InterceptorInstalled(win), false, 'fresh window should not have interceptor');
  install401Interceptor({ windowObj: win, switchPage: () => {}, onUnauthorized: () => {} });
  assert.equal(is401InterceptorInstalled(win), true, 'after install, flag should be set');
});

test('install401Interceptor: 500 error does not trigger 401 path', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 500, ok: false });
  let unauthorized = 0;
  let switched = 0;
  install401Interceptor({
    windowObj: win,
    onUnauthorized: () => {
      unauthorized++;
    },
    switchPage: () => {
      switched++;
    },
  });
  await win.fetch('/api/foo');
  assert.equal(unauthorized, 0, '500 should not trigger 401 path');
  assert.equal(switched, 0, '500 should not switchPage');
});


test('install401Interceptor: mutating 401 (POST) with onApiKeyRequired fires modal, no login redirect', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let apiKeyRequired = 0;
  let unauthorized = 0;
  let switched = 0;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'login',
    excludedPages: ['login'],
    onUnauthorized: () => {
      unauthorized++;
    },
    onApiKeyRequired: () => {
      apiKeyRequired++;
    },
    switchPage: () => {
      switched++;
    },
  });
  await win.fetch('/api/scheduler/toggle', { method: 'POST', body: '{}' });
  assert.equal(apiKeyRequired, 1, 'onApiKeyRequired should fire once for mutating 401');
  assert.equal(unauthorized, 0, 'mutating 401 should NOT fire onUnauthorized when onApiKeyRequired is set');
  assert.equal(switched, 0, 'mutating 401 should NOT redirect to login (admin opens apiKeyModal instead)');
});

test('install401Interceptor: mutating 401 without onApiKeyRequired falls back to onUnauthorized + login (client_web)', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let unauthorized = 0;
  let switchedTo = null;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'login',
    excludedPages: ['login', 'register'],
    onUnauthorized: () => {
      unauthorized++;
    },
    switchPage: (id) => {
      switchedTo = id;
    },
  });
  await win.fetch('/api/scheduler/toggle', { method: 'POST', body: '{}' });
  assert.equal(unauthorized, 1, 'client_web keeps onUnauthorized on mutating 401');
  assert.equal(switchedTo, 'login', 'client_web keeps login redirect on mutating 401');
});

test('install401Interceptor: GET 401 with onApiKeyRequired still uses user-auth path (onUnauthorized + login)', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let apiKeyRequired = 0;
  let unauthorized = 0;
  let switchedTo = null;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'login',
    excludedPages: ['login'],
    onUnauthorized: () => {
      unauthorized++;
    },
    onApiKeyRequired: () => {
      apiKeyRequired++;
    },
    switchPage: (id) => {
      switchedTo = id;
    },
  });
  await win.fetch('/api/user/profile');
  assert.equal(apiKeyRequired, 0, 'GET 401 must not open apiKeyModal');
  assert.equal(unauthorized, 1, 'GET 401 is user-auth failure → onUnauthorized fires');
  assert.equal(switchedTo, 'login', 'GET 401 redirects to login');
});

test('install401Interceptor: PUT/DELETE 401 also classified as mutating → onApiKeyRequired', async () => {
  for (const method of ['PUT', 'DELETE', 'PATCH']) {
    const { win } = makeFakeWindow();
    win.fetch = () => Promise.resolve({ status: 401, ok: false });
    let apiKeyRequired = 0;
    install401Interceptor({
      windowObj: win,
      onUnauthorized: () => {},
      onApiKeyRequired: () => {
        apiKeyRequired++;
      },
      switchPage: () => {},
    });
    await win.fetch('/api/foo', { method: method });
    assert.equal(apiKeyRequired, 1, method + ' 401 should fire onApiKeyRequired');
  }
});

test('install401Interceptor: response object is still returned to caller', async () => {
  const { win } = makeFakeWindow();
  const expected = { status: 200, body: { ok: true } };
  win.fetch = () => Promise.resolve(expected);
  install401Interceptor({ windowObj: win, switchPage: () => {}, onUnauthorized: () => {} });
  const res = await win.fetch('/api/foo');
  assert.equal(res, expected, 'caller should get the original response object back');
});

test('install401Interceptor: loginRedirectUrl redirects externally instead of switchPage', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let unauthorized = 0;
  let switched = 0;
  let redirectedTo = null;
  install401Interceptor({
    windowObj: win,
    loginPageId: 'login',
    loginRedirectUrl: 'https://member.goluck.uk/login?redirect=https%3A%2F%2Fatlas.goluck.uk%2Fclient%2Fhome',
    excludedPages: ['login', 'register'],
    onUnauthorized: () => {
      unauthorized++;
    },
    switchPage: () => {
      switched++;
    },
  });
  await win.fetch('/api/user/profile');
  assert.equal(unauthorized, 1, 'onUnauthorized should fire once');
  assert.equal(switched, 0, 'should NOT switchPage when loginRedirectUrl provided');
  assert.equal(redirectedTo, null, 'fake window has no location.href assignment; just verify no throw');
});

test('install401Interceptor: loginRedirectUrl as function is evaluated lazily', async () => {
  const { win } = makeFakeWindow();
  win.fetch = () => Promise.resolve({ status: 401, ok: false });
  let evaluated = 0;
  const captured = [];
  const fakeWin = {
    location: {
      pathname: '/client/home',
      set href(v) { captured.push(v); },
      get href() { return 'http://localhost/client/home'; },
    },
    fetch: win.fetch,
  };
  install401Interceptor({
    windowObj: fakeWin,
    loginPageId: 'login',
    loginRedirectUrl: () => {
      evaluated++;
      return 'https://member.goluck.uk/login?redirect=x';
    },
    excludedPages: ['login'],
    onUnauthorized: () => {},
  });
  await fakeWin.fetch('/api/user/profile');
  assert.equal(evaluated, 1, 'function loginRedirectUrl should be called on 401');
  assert.deepEqual(captured, ['https://member.goluck.uk/login?redirect=x'], 'should set location.href');
});
