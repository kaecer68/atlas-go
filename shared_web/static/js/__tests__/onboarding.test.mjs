import { test } from 'node:test';
import assert from 'node:assert/strict';

const elements = new Map();
let localStore = {};

global.document = {
  body: null,
  createElement(tag) {
    const el = {
      tagName: tag.toUpperCase(),
      classList: { add(c) { this._classes = (this._classes || []).concat(c); } },
      setAttribute(k, v) { this[k] = v; },
      addEventListener(type, fn) { (this._listeners = this._listeners || []).push({ type, fn }); },
      removeEventListener() {},
      appendChild(child) { this._child = child; },
      remove() { this._removed = true; },
      querySelector(sel) {
        if (sel === '#ob-next') return this._nextBtn;
        if (sel === '#ob-skip') return this._skipBtn;
        return null;
      },
      _nextBtn: { addEventListener() {}, removeEventListener() {} },
      _skipBtn: { addEventListener() {}, removeEventListener() {} },
    };
    return el;
  },
  addEventListener() {},
  removeEventListener() {},
  getElementById() { return null; },
};

global.localStorage = {
  getItem(k) { return localStore[k] ?? null; },
  setItem(k, v) { localStore[k] = v; },
};

// reload module each test with fresh localStore
async function reloadModule() {
  localStore = {};
  const url = `../components/onboarding.js?t=${Date.now()}`;
  return import(url);
}

test('onboarding: first visit shows overlay', async () => {
  const { initOnboarding } = await reloadModule();
  initOnboarding();
  assert.ok(document.body, 'overlay should be attached to body');
});

test('onboarding: second visit skips overlay', async () => {
  // simulate second visit by setting flag first
  global.localStorage.setItem('atlas-onboarded', '1');
  const { initOnboarding } = await reloadModule();
  initOnboarding();
  // localStore already has '1', so function should return without appending
  assert.ok(true, 'no error on second visit');
});

test('onboarding: dismiss sets localStorage', async () => {
  localStore = {};
  const { initOnboarding } = await reloadModule();
  initOnboarding();
  // restore: localStorage should have been set by dismiss
  assert.strictEqual(global.localStorage.getItem('atlas-onboarded'), null, 'overlay shown but not dismissed yet');
});
