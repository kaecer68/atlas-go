// confirm-modal.js
// 共用「危險操作二次確認」Modal 元件（P1-C）。
//
// 用途：任何一鍵可能造成重大影響的操作（停用資料通道、啟動資源密集回測等）
//       都必須先彈出本元件要求使用者確認，避免誤觸。
//
// API：
//   import { confirmAction } from '../components/confirm-modal.js';
//   const ok = await confirmAction({
//     title: '全部停用',            // 標題（必填）
//     message: '將停用 5 個通道…',  // 說明（必填）
//     danger: true,                // true → 確認鈕紅色危險樣式（預設 true）
//     confirmLabel: '確認停用',    // 確認鈕文字（預設「確認」）
//     cancelLabel: '取消',         // 取消鈕文字（預設「取消」）
//     onConfirm: () => { ... },    // 選用：確認後回呼（亦可改用 await 回傳值）
//   });
//   if (!ok) return;               // 取消 / Esc / 點背景 → false，不執行
//
// 行為：
//   - 確認 → 回傳 true 並（若提供）呼叫 onConfirm
//   - 取消 / 點背景 / Esc → 回傳 false，不呼叫 onConfirm
//   - 單例 overlay：第一次呼叫時建立，之後重用；重複呼叫會先關閉前一個
//   - danger 樣式走既有 CSS token（--color-danger / --danger），見
//     shared_web/static/css/components/confirm-modal.css

let overlayEl = null;
let current = null;

function createOverlay() {
  const overlay = document.createElement('div');
  overlay.className = 'confirm-modal-overlay';
  overlay.setAttribute('role', 'alertdialog');
  overlay.setAttribute('aria-modal', 'true');

  const modal = document.createElement('div');
  modal.className = 'confirm-modal';

  const titleEl = document.createElement('h3');
  titleEl.className = 'confirm-modal__title';

  const messageEl = document.createElement('p');
  messageEl.className = 'confirm-modal__message';

  const actionsEl = document.createElement('div');
  actionsEl.className = 'confirm-modal__actions';

  const cancelBtn = document.createElement('button');
  cancelBtn.type = 'button';
  cancelBtn.className = 'confirm-modal__cancel';

  const okBtn = document.createElement('button');
  okBtn.type = 'button';
  okBtn.className = 'confirm-modal__ok';

  actionsEl.appendChild(cancelBtn);
  actionsEl.appendChild(okBtn);
  modal.appendChild(titleEl);
  modal.appendChild(messageEl);
  modal.appendChild(actionsEl);
  overlay.appendChild(modal);

  // 取消鈕 / 確認鈕 / 點背景 → 都只 settle 當下這次呼叫
  cancelBtn.addEventListener('click', () => settle(false));
  okBtn.addEventListener('click', () => settle(true));
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) settle(false);
  });

  document.addEventListener('keydown', onKeydown);
  document.body.appendChild(overlay);
  return overlay;
}

function onKeydown(e) {
  if (e.key !== 'Escape' || !current) return;
  e.preventDefault();
  settle(false);
}

function settle(confirmed) {
  if (!current) return;
  const { resolve, onConfirm, previousFocus, overlay } = current;
  current = null;
  overlay.classList.remove('open');
  document.body.style.overflow = '';
  if (previousFocus && typeof previousFocus.focus === 'function') previousFocus.focus();
  resolve(confirmed);
  if (confirmed && typeof onConfirm === 'function') {
    try {
      onConfirm();
    } catch (err) {
      console.error('[confirmAction] onConfirm failed:', err);
    }
  }
}

export function confirmAction(opts = {}) {
  const {
    title,
    message,
    danger = true,
    confirmLabel = '確認',
    cancelLabel = '取消',
    onConfirm,
  } = opts;

  // 若前一個確認框還開著，先以「取消」關閉它（防疊框）
  if (current) settle(false);

  const overlay = overlayEl || (overlayEl = createOverlay());
  const modal = overlay.querySelector('.confirm-modal');
  const titleEl = overlay.querySelector('.confirm-modal__title');
  const messageEl = overlay.querySelector('.confirm-modal__message');
  const okBtn = overlay.querySelector('.confirm-modal__ok');
  const cancelBtn = overlay.querySelector('.confirm-modal__cancel');

  titleEl.textContent = title || '確認操作';
  messageEl.textContent = message || '';
  okBtn.textContent = confirmLabel;
  cancelBtn.textContent = cancelLabel;
  modal.classList.toggle('confirm-modal--danger', !!danger);
  modal.classList.toggle('confirm-modal--info', !danger);
  okBtn.classList.toggle('danger', !!danger);

  const previousFocus = document.activeElement;
  overlay.classList.add('open');
  document.body.style.overflow = 'hidden';
  okBtn.focus();

  return new Promise((resolve) => {
    current = { resolve, onConfirm, previousFocus, overlay };
  });
}

export default confirmAction;
