// shared_web/static/js/__tests__/idle-monitor.test.mjs
//
// 閒置監測器單測（Track A）：mock timer 驗證 idle 觸發 / 互動重置 /
// 停止清理。邏輯在 client_web/static/js/services/idle-monitor.js。
//
// 執行：cd client_web && npm test

import { test, mock } from 'node:test';
import assert from 'node:assert/strict';
import {
  startIdleMonitor,
  resetIdleMonitor,
  stopIdleMonitor,
  isIdleMonitorRunning,
  DEFAULT_TIMEOUT_MS,
} from '../../../../client_web/static/js/services/idle-monitor.js';

function makeFakeWindow() {
  const listeners = {};
  return {
    listeners,
    addEventListener(ev, fn) {
      (listeners[ev] = listeners[ev] || []).push(fn);
    },
    removeEventListener(ev, fn) {
      if (!listeners[ev]) return;
      listeners[ev] = listeners[ev].filter(function (f) { return f !== fn; });
    },
  };
}

test('idle-monitor: 預設 timeout = 20 分鐘', () => {
  assert.equal(DEFAULT_TIMEOUT_MS, 20 * 60 * 1000);
});

test('idle-monitor: 無 window 環境 startIdleMonitor 回傳 null', () => {
  const ret = startIdleMonitor({ timeoutMs: 1000, onIdle: function () {} });
  assert.equal(ret, null, '無 window 不應啟動');
  assert.equal(isIdleMonitorRunning(), false);
});

test('idle-monitor: timeout 到期觸發 onIdle 一次，且不自動重 arm', () => {
  mock.timers.enable({ apis: ['setTimeout'] });
  try {
    const win = makeFakeWindow();
    let idleCount = 0;
    startIdleMonitor({ timeoutMs: 1000, onIdle: function () { idleCount++; }, windowObj: win });
    assert.equal(isIdleMonitorRunning(), true);
    mock.timers.tick(999);
    assert.equal(idleCount, 0, '未滿 timeout 不觸發');
    mock.timers.tick(1);
    assert.equal(idleCount, 1, '滿 timeout 觸發 onIdle');
    mock.timers.tick(5000);
    assert.equal(idleCount, 1, '觸發後不自動重新計時');
    stopIdleMonitor();
  } finally {
    mock.timers.reset();
  }
});

test('idle-monitor: 互動事件（mousemove）重置計時器', () => {
  mock.timers.enable({ apis: ['setTimeout'] });
  try {
    const win = makeFakeWindow();
    let idleCount = 0;
    startIdleMonitor({ timeoutMs: 1000, onIdle: function () { idleCount++; }, windowObj: win });
    assert.ok(win.listeners.mousemove.length >= 1, 'mousemove listener 已註冊');
    assert.ok(win.listeners.keydown.length >= 1, 'keydown listener 已註冊');
    assert.ok(win.listeners.scroll.length >= 1, 'scroll listener 已註冊');
    assert.ok(win.listeners.click.length >= 1, 'click listener 已註冊');
    assert.ok(win.listeners.touchstart.length >= 1, 'touchstart listener 已註冊');

    mock.timers.tick(600);
    // 模擬 mousemove → reset → deadline 延到 t=1600
    win.listeners.mousemove.forEach(function (fn) { fn(); });
    mock.timers.tick(600); // t=1200：若沒 reset 早該觸發
    assert.equal(idleCount, 0, '互動後計時器應被重置');
    mock.timers.tick(400); // t=1600 → 觸發
    assert.equal(idleCount, 1, '重置後的新 deadline 到齊應觸發');
    stopIdleMonitor();
  } finally {
    mock.timers.reset();
  }
});

test('idle-monitor: resetIdleMonitor 手動重置（「再逛一下」路徑）', () => {
  mock.timers.enable({ apis: ['setTimeout'] });
  try {
    const win = makeFakeWindow();
    let idleCount = 0;
    startIdleMonitor({ timeoutMs: 1000, onIdle: function () { idleCount++; }, windowObj: win });
    mock.timers.tick(900);
    resetIdleMonitor();
    mock.timers.tick(900);
    assert.equal(idleCount, 0, 'reset 後應重新計時');
    mock.timers.tick(100);
    assert.equal(idleCount, 1, '重置後 deadline 到齊應觸發');
    stopIdleMonitor();
  } finally {
    mock.timers.reset();
  }
});

test('idle-monitor: stopIdleMonitor 移除 listeners 且不再觸發', () => {
  mock.timers.enable({ apis: ['setTimeout'] });
  try {
    const win = makeFakeWindow();
    let idleCount = 0;
    startIdleMonitor({ timeoutMs: 1000, onIdle: function () { idleCount++; }, windowObj: win });
    stopIdleMonitor();
    assert.equal(isIdleMonitorRunning(), false, '停止後不應 running');
    assert.equal(win.listeners.mousemove.length, 0, 'listeners 應被移除');
    mock.timers.tick(5000);
    assert.equal(idleCount, 0, '停止後不應觸發');
  } finally {
    mock.timers.reset();
  }
});
