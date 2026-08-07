/**
 * notify.js — notification-center toast (Gap 3-R4).
 *
 * Revives the #notificationCenter div that has existed as an empty shell
 * in both admin_web and client_web index.html (the CSS keyframes were
 * authored but nothing ever wrote to it). Endpoints that mutate user state
 * (ack/dismiss/reset) surface their result here; any page may use it.
 *
 * Usage:
 *   import { showNotification } from '../services/notify.js';
 *   showNotification('已標記為已讀', 'success');
 *
 * Tone classes: success | error | info (default info).
 */

const AUTO_DISMISS_MS = 4000;
const LEAVE_ANIM_MS = 300;

export function showNotification(message, tone) {
  tone = tone || 'info';
  const center = document.getElementById('notificationCenter');
  if (!center) return;
  const item = document.createElement('div');
  item.className = 'notify-item notify-item--' + tone;
  item.setAttribute('role', 'status');
  item.textContent = message;
  center.appendChild(item);
  setTimeout(function () {
    item.classList.add('notify-item--leaving');
    setTimeout(function () { item.remove(); }, LEAVE_ANIM_MS);
  }, AUTO_DISMISS_MS);
}
