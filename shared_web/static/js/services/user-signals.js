/**
 * user-signals.js — per-user signal state service (Gap 3-R3/R4).
 *
 * Backend: /api/user/signals (R3, PR #1493). The JWT rides the HttpOnly
 * cookie (fetchWithRetry sends credentials: 'include'), so no manual
 * Authorization header is needed here.
 *
 * State record shape (internal/userstate.UserSignalState):
 *   { user_id, signal_key, acknowledged_at?, dismissed, updated_at }
 *   acknowledged_at absent (omitempty) = "new"; dismissed true = hidden.
 */

import { getJSON, putJSON, delJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

const BASE = '/api/user/signals';

/** GET /api/user/signals — all states for the current user. */
export async function listSignals() {
  return getJSON(BASE);
}

/** PUT /api/user/signals/{key}/ack — mark 已讀 (idempotent). */
export async function ackSignal(signalKey) {
  return putJSON(BASE + '/' + encodeURIComponent(signalKey) + '/ack', {});
}

/** PUT /api/user/signals/{key}/dismiss — stop showing this signal. */
export async function dismissSignal(signalKey) {
  return putJSON(BASE + '/' + encodeURIComponent(signalKey) + '/dismiss', {});
}

/** DELETE /api/user/signals/{key} — reset back to "new". */
export async function resetSignal(signalKey) {
  return delJSON(BASE + '/' + encodeURIComponent(signalKey));
}

/**
 * signalStatusMeta — pure mapping of a state record to badge label/class.
 * Used by renderSignalsList and unit-tested directly.
 */
export function signalStatusMeta(state) {
  if (!state) return { label: '新訊號', className: 'badge--new' };
  if (state.dismissed) return { label: '已忽略', className: 'badge--dismissed' };
  if (state.acknowledged_at) return { label: '已讀', className: 'badge--ack' };
  return { label: '新訊號', className: 'badge--new' };
}

/**
 * renderSignalsList — HTML table/list of signal states.
 * Pure function (no DOM access) so it is unit-testable in node --test.
 */
export function renderSignalsList(records) {
  if (!records || records.length === 0) {
    return '<div class="empty-state">還沒有追蹤紀錄。訊號出現時按下「標記已讀」，就會記錄在這裡。</div>';
  }
  const rows = records.map(function (rec) {
    const meta = signalStatusMeta(rec);
    const key = escapeHtml(rec.signal_key || 'unknown');
    const time = rec.updated_at
      ? new Date(rec.updated_at).toLocaleString('zh-TW')
      : '-';
    let actions = '<span class="muted">無可用操作</span>';
    if (!rec.dismissed) {
      actions = '';
      if (!rec.acknowledged_at) {
        actions += '<button class="btn btn--small btn--primary" data-action="ack" data-key="' + key + '">標記已讀</button>';
      }
      actions += '<button class="btn btn--small" data-action="dismiss" data-key="' + key + '">不再顯示</button>';
    } else {
      actions = '<button class="btn btn--small" data-action="reset" data-key="' + key + '">恢復顯示</button>';
    }
    return '<tr data-signal="' + key + '">'
      + '<td><code class="signal-key">' + key + '</code></td>'
      + '<td><span class="badge ' + meta.className + '">' + meta.label + '</span></td>'
      + '<td class="muted">' + time + '</td>'
      + '<td class="signal-actions">' + actions + '</td>'
      + '</tr>';
  }).join('');
  return '<table class="signal-table"><thead><tr>'
    + '<th>訊號</th><th>狀態</th><th>更新時間</th><th>操作</th>'
    + '</tr></thead><tbody>' + rows + '</tbody></table>';
}
