/**
 * Glossary Popover Component
 * 滑鼠移入術語時，在術語下方顯示淺顯解釋泡泡
 */

import { GLOSSARY, glossaryLookup } from '../shared/glossary.js';

/** 當前顯示中的 popover 元素 */
let activePopover = null;

/**
 * 產生帶有 glossary 標記的 HTML 字串
 * @param {string} term - 術語（作為 data-glossary 屬性）
 * @param {string} [text] - 顯示文字，預設等於 term
 * @returns {string}
 */
export function renderGlossaryPopover(term, text) {
  const display = text ?? term;
  return `<span class="glossary-term" data-glossary="${escapeAttr(term)}">${escapeHtml(display)}</span>`;
}

/**
 * 初始化 glossary popover 行為
 * 把當前 DOM 中所有 .glossary-term 元素綁定事件
 */
export function initGlossaryPopovers() {
  document.removeEventListener('mousedown', handleOutsideClick);
  document.removeEventListener('keydown', handleKeyDown);
  dismissPopover();

  document.querySelectorAll('.glossary-term').forEach((el) => {
    el.addEventListener('mouseenter', handleTermEnter);
    el.addEventListener('mouseleave', handleTermLeave);
  });

  document.addEventListener('mousedown', handleOutsideClick);
  document.addEventListener('keydown', handleKeyDown);
}

// ─── 事件處理 ───────────────────────────────────────────────

let hoverTimer = null;

function handleTermEnter(e) {
  clearTimeout(hoverTimer);
  const el = e.currentTarget;
  hoverTimer = setTimeout(() => showPopover(el), 200);
}

function handleTermLeave(e) {
  clearTimeout(hoverTimer);
  const el = e.currentTarget;
  hoverTimer = setTimeout(() => dismissPopover(), 300);
}

function handleOutsideClick(e) {
  if (activePopover && !activePopover.contains(e.target) && !e.target.classList.contains('glossary-term')) {
    dismissPopover();
  }
}

function handleKeyDown(e) {
  if (e.key === 'Escape') {
    dismissPopover();
  }
}

// ─── Popover 操作 ────────────────────────────────────────────

/**
 * @param {HTMLElement} termEl
 */
function showPopover(termEl) {
  dismissPopover();

  const term = termEl.dataset.glossary;
  if (!term) return;

  const entry = glossaryLookup(term);
  if (!entry) return;

  const popover = document.createElement('span');
  popover.className = 'glossary-popover';
  popover.setAttribute('role', 'tooltip');
  popover.innerHTML = `
    <span class="glossary-popover__brief">${escapeHtml(entry.brief)}</span>
    <span class="glossary-popover__detail">${escapeHtml(entry.detail)}</span>
  `;

  document.body.appendChild(popover);
  activePopover = popover;

  // 定位：置於術語下方，確保不超出 viewport
  const termRect = termEl.getBoundingClientRect();
  const popRect = popover.getBoundingClientRect();

  let top = termRect.bottom + window.scrollY + 6;
  let left = termRect.left + window.scrollX;

  // 若右側空間不夠，往左移
  const viewportWidth = window.innerWidth;
  if (left + popRect.width > viewportWidth - 16) {
    left = viewportWidth - popRect.width - 16;
  }
  // 若左側為負值，也做修正
  if (left < 16) {
    left = 16;
  }

  popover.style.top = `${top}px`;
  popover.style.left = `${left}px`;
}

function dismissPopover() {
  if (activePopover) {
    activePopover.remove();
    activePopover = null;
  }
  clearTimeout(hoverTimer);
}

// ─── 工具 ────────────────────────────────────────────────────

/**
 * @param {string} str
 * @returns {string}
 */
function escapeHtml(str) {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/**
 * @param {string} str
 * @returns {string}
 */
function escapeAttr(str) {
  return str.replace(/"/g, '&quot;');
}
