/**
 * my-signals.js — 「我的追蹤」頁 (Gap 3-R4).
 *
 * 產品定位 §9「觀測 → 解讀 → 追蹤 → 紀律」的「追蹤」UI 端:顯示目前
 * 使用者對市場訊號的追蹤狀態(新訊號 / 已讀 / 已忽略),並提供
 * 「標記已讀 / 不再顯示 / 恢復顯示」操作。操作結果寫回
 * /api/user/signals(R3),並以 notification-center toast 回饋。
 *
 * Backend: GET/PUT/DELETE /api/user/signals (R3 PR #1493)。
 * Auth: JWT cookie 自動帶(credentials: 'include');401 時顯示登入提示。
 */

import { renderErrorState } from '../shared/app-utils.js';
import { listSignals, ackSignal, dismissSignal, resetSignal, renderSignalsList } from '../services/user-signals.js';
import { showNotification } from '../services/notify.js';

const RETRY_ID = 'my-signals';

export const template = `
  <details class="help-details"><summary><strong>💡 我的追蹤在做什麼</strong></summary>
    <p>這裡記錄你對市場訊號的處理狀態：看到訊號後按「標記已讀」，就是為自己的投資決策留下一筆紀律紀錄（產品定位 §9 的「追蹤 → 紀律」）。已忽略的訊號不再顯示；「恢復顯示」可重置回新訊號。</p>
  </details>
  <section id="ms-list" class="ms-list" aria-live="polite">載入中…</section>
`;

export async function init() {
  const listEl = document.getElementById('ms-list');
  if (!listEl) return;
  listEl.innerHTML = '載入中…';
  await load(listEl);
}

/**
 * load — fetch current user's signal states and render.
 * 401 (guest/未登入) → 提示需登入;其他錯誤 → renderErrorState 可重試。
 */
async function load(listEl) {
  let payload;
  try {
    payload = await listSignals();
  } catch (err) {
    if (err && err.status === 401) {
      // 登入牆逃脫路徑：停留在本頁顯示原因，並提供回公開內容的連結
      // （UX audit P0：未登入點「我的追蹤」不應直接摔進登入頁）。
      listEl.innerHTML = `
        <div class="empty-state-guidance">
          <div class="icon">🔒</div>
          <div class="title">此功能需要登入</div>
          <div class="desc">登入後即可開始追蹤訊號、建立投資紀律紀錄</div>
          <div class="empty-actions">
            <a class="btn btn--primary btn-sm" data-page="login" href="/client/login">登入</a>
            <a class="btn" data-page="home" href="/client/home">← 先看看公開內容</a>
          </div>
        </div>`;
      return;
    }
    listEl.innerHTML = renderErrorState('載入追蹤清單失敗', RETRY_ID);
    listEl.querySelector('[data-retry="' + RETRY_ID + '"]')?.addEventListener('click', function () {
      load(listEl);
    });
    return;
  }
  const records = (payload && payload.signals) || [];
  if (records.length === 0) {
    listEl.innerHTML = renderEmptyState('還沒有追蹤紀錄', '訊號出現時按下「標記已讀」，就會記錄在這裡');
    return;
  }
  listEl.innerHTML = renderSignalsList(records);
  listEl.querySelectorAll('button[data-action]').forEach(function (btn) {
    btn.addEventListener('click', function () { handleAction(listEl, btn); });
  });
}

/**
 * handleAction — ack / dismiss / reset a signal, then reload the list.
 * Uses the same listEl capture to re-render in place.
 */
async function handleAction(listEl, btn) {
  const key = btn.getAttribute('data-key');
  const action = btn.getAttribute('data-action');
  if (!key) return;
  btn.disabled = true;
  const original = btn.textContent;
  btn.textContent = '處理中…';
  try {
    if (action === 'ack') {
      await ackSignal(key);
      showNotification('已標記為已讀：「' + key + '」', 'success');
    } else if (action === 'dismiss') {
      await dismissSignal(key);
      showNotification('已隱藏：「' + key + '」', 'info');
    } else if (action === 'reset') {
      await resetSignal(key);
      showNotification('已恢復顯示：「' + key + '」', 'info');
    }
    await load(listEl);
  } catch (err) {
    if (err && err.status === 401) {
      showNotification('操作需要登入', 'error');
    } else {
      showNotification('操作失敗，請稍後再試', 'error');
    }
    btn.disabled = false;
    btn.textContent = original;
  }
}


