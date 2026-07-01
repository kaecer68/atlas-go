import { cycleLegendContent } from './cycle-legend.js';

const CONTENT_BY_ID = {
  cycleLegend: cycleLegendContent
};

function findOverlay(id) {
  return document.getElementById(id + 'Modal') || document.getElementById(id);
}

function injectContent(overlay, id) {
  const target = overlay.querySelector('[data-modal-content]') || overlay.querySelector('.modal') || overlay;
  if (!target || target.dataset.filled === '1') return;
  const html = CONTENT_BY_ID[id];
  if (typeof html === 'string') {
    target.innerHTML = html;
    target.dataset.filled = '1';
  }
}

function lockScroll(lock) {
  document.body.style.overflow = lock ? 'hidden' : '';
}

export const Modal = {
  open(id) {
    const overlay = findOverlay(id);
    if (!overlay) { console.warn('[Modal] overlay not found:', id); return false; }
    injectContent(overlay, id);
    overlay.style.display = 'flex';
    lockScroll(true);
    document.dispatchEvent(new CustomEvent('modal:open', { detail: { id } }));
    return true;
  },
  close(id) {
    const overlay = findOverlay(id);
    if (!overlay) return;
    overlay.style.display = '';
    lockScroll(false);
    document.dispatchEvent(new CustomEvent('modal:close', { detail: { id } }));
  },
  closeAll() {
    document.querySelectorAll('.modal-overlay').forEach(el => { el.style.display = ''; });
    lockScroll(false);
  }
};

window.Modal = Modal;

document.addEventListener('click', (e) => {
  const trigger = e.target.closest('[data-open-modal]');
  if (trigger) {
    e.preventDefault();
    Modal.open(trigger.dataset.openModal);
    return;
  }
  const closer = e.target.closest('[data-close-modal]');
  if (closer) {
    const overlay = closer.closest('.modal-overlay');
    if (overlay) Modal.close(overlay.id.replace(/Modal$/, ''));
    return;
  }
  if (e.target.classList && e.target.classList.contains('modal-overlay')) {
    Modal.close(e.target.id.replace(/Modal$/, ''));
  }
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') Modal.closeAll();
});
