// client_web/static/js/services/idle-monitor.js
//
// 閒置監測器（Track A：20 分鐘未互動 → 提醒自願登入）。
//
// 行為：
//   - startIdleMonitor({ timeoutMs, onIdle })：監聽 mousemove/keydown/scroll/
//     click/touchstart，任何互動都重設計時器；timeout 到期觸發 onIdle()。
//   - onIdle 觸發後不會自動重 arm（由 modal 的「再逛一下」呼叫
//     resetIdleMonitor() 重新計時）。
//   - resetIdleMonitor()：用戶互動或「再逛一下」時重設計時。
//   - stopIdleMonitor()：移除 listeners + 清除 timer（測試/頁面卸載用）。
//
// 純瀏覽器依賴注入（windowObj）設計，node --test 可用 fake window 測。

export const DEFAULT_TIMEOUT_MS = 20 * 60 * 1000; // 20 分鐘

const IDLE_EVENTS = ['mousemove', 'keydown', 'scroll', 'click', 'touchstart'];

let _running = false;
let _timer = null;
let _timeoutMs = DEFAULT_TIMEOUT_MS;
let _onIdle = null;
let _win = null;
let _handler = null;

function _clearTimer() {
  if (_timer !== null) {
    clearTimeout(_timer);
    _timer = null;
  }
}

function _armTimer() {
  _clearTimer();
  _timer = setTimeout(function () {
    _timer = null;
    if (typeof _onIdle === 'function') {
      try {
        _onIdle();
      } catch (e) {
        // listener 異常不得影響 idle 系統本身
        // eslint-disable-next-line no-console
        console.warn('[idle-monitor] onIdle handler error:', e);
      }
    }
  }, _timeoutMs);
}

function _activityHandler() {
  if (_running) resetIdleMonitor();
}

/**
 * 啟動閒置監測。
 * @param {object} [options]
 * @param {number} [options.timeoutMs=DEFAULT_TIMEOUT_MS] 閒置逾時（ms）
 * @param {() => void} [options.onIdle] 逾時觸發的 callback
 * @param {Window} [options.windowObj] 測試用 fake window；預設 globalThis.window
 * @returns {(() => void) | null} stopIdleMonitor；無 window 環境回傳 null
 */
export function startIdleMonitor(options = {}) {
  const timeoutMs = options.timeoutMs !== undefined ? options.timeoutMs : DEFAULT_TIMEOUT_MS;
  const onIdle = options.onIdle || null;
  const win = options.windowObj || (typeof window !== 'undefined' ? window : null);

  if (!win) return null;
  if (_running) stopIdleMonitor();

  _timeoutMs = timeoutMs;
  _onIdle = onIdle;
  _win = win;
  _running = true;
  _handler = _activityHandler;
  IDLE_EVENTS.forEach(function (ev) {
    win.addEventListener(ev, _handler, { passive: true });
  });
  _armTimer();
  return stopIdleMonitor;
}

/**
 * 重置閒置計時（用戶互動 / modal「再逛一下」時呼叫）。
 */
export function resetIdleMonitor() {
  if (!_running) return;
  _armTimer();
}

/**
 * 停止閒置監測並清理 listeners。
 */
export function stopIdleMonitor() {
  if (!_running) return;
  _clearTimer();
  if (_win && _handler) {
    IDLE_EVENTS.forEach(function (ev) {
      _win.removeEventListener(ev, _handler);
    });
  }
  _running = false;
  _win = null;
  _handler = null;
  _onIdle = null;
}

/**
 * 目前是否在監測中（測試用）。
 * @returns {boolean}
 */
export function isIdleMonitorRunning() {
  return _running;
}
