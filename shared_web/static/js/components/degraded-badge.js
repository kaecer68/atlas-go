// shared_web/static/js/components/degraded-badge.js
//
// 低調的「部分資料源暫時不可用」badge（P1-B）。
// - 掛在全頁右下/左下角（固定定位，不影響任何 page layout）
// - 有 degraded endpoint 時顯示 pill：⚠ 部分資料源暫時不可用 (N)
// - 點擊展開列出哪些 endpoint；恢復後（getDegradedEndpoints() 為空）自動隱藏
// - 透過 app-utils 的 onDegradedChange 訂閱，與 choke point 解耦
//
// 使用：
//   import { initDegradedBadge } from './components/degraded-badge.js';
//   initDegradedBadge();   // 在兩個 shell 的 boot 區各呼叫一次（冪等）

import { getDegradedEndpoints, onDegradedChange } from '../shared/app-utils.js';

let initialized = false;

/**
 * 初始化 degraded badge（冪等，可在兩個 shell 重複呼叫）。
 * 需要 document（瀏覽器環境）；node --test 下無 document 時為 no-op。
 */
export function initDegradedBadge() {
  if (initialized) return;
  initialized = true;
  if (typeof document === 'undefined') return;
  if (!document.body) {
    document.addEventListener('DOMContentLoaded', mount);
    return;
  }
  mount();
}

function mount() {
  const root = document.createElement('div');
  root.className = 'degraded-badge';
  root.setAttribute('role', 'status');
  root.setAttribute('aria-live', 'polite');

  const pill = document.createElement('button');
  pill.type = 'button';
  pill.className = 'degraded-badge__pill';
  pill.title = '點擊查看受影響的資料源';
  pill.innerHTML = '⚠ <span class="degraded-badge__count">0</span>';

  const label = document.createElement('span');
  label.className = 'degraded-badge__label';
  label.textContent = '部分資料源暫時不可用';
  pill.prepend(label);

  const list = document.createElement('div');
  list.className = 'degraded-badge__list';
  list.hidden = true;

  root.appendChild(pill);
  root.appendChild(list);
  document.body.appendChild(root);

  let expanded = false;
  pill.addEventListener('click', function () {
    expanded = !expanded;
    list.hidden = !expanded;
    pill.classList.toggle('degraded-badge__pill--expanded', expanded);
  });

  function render(endpoints) {
    const count = endpoints.length;
    const countEl = root.querySelector('.degraded-badge__count');
    if (countEl) countEl.textContent = String(count);
    root.classList.toggle('degraded-badge--visible', count > 0);
    list.innerHTML = '';
    if (count > 0) {
      const title = document.createElement('div');
      title.className = 'degraded-badge__list-title';
      title.textContent = '以下資料源暫時不可用，系統已記錄並將自動恢復：';
      list.appendChild(title);
      endpoints.forEach(function (ep) {
        const item = document.createElement('div');
        item.className = 'degraded-badge__item';
        item.textContent = ep;
        list.appendChild(item);
      });
    }
  }

  onDegradedChange(render);
  render(getDegradedEndpoints());
}
