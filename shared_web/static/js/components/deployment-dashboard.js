/**
 * Fubon Proxy Deployment Dashboard
 * Fetches supervisor state from /api/admin/live/deployment/dashboard
 *
 * Mirror pattern: shared_web/static/js/components/sim-health.js
 * Lifecycle: constructor → renderSkeleton → fetchStatus → updateView → startAutoRefresh
 * Public API: start(), stop(), destroy()
 *
 * JSON contract (per docs/specs/phase3-5-spec.md §3.1 M1):
 * {
 *   "supervisor_running": bool,
 *   "process_alive": bool,
 *   "pid": int, "port": int,
 *   "started_at": ISO8601, "restart_count": int,
 *   "last_beat_at": ISO8601, "last_beat_age_sec": int,
 *   "last_error": string,
 *   "recent_events": [{"at": ISO8601, "kind": string, "detail": string}, ...],
 *   "config": {"binary": string, "args": [], "auto_restart": bool,
 *              "max_restarts": int, "listen_port": int}
 * }
 */

import { escapeHtml, fmt } from '../shared/utils.js';
// getJSON 走 app-utils fetch 鏈（PR-2: GET 有 key 就靜默帶 X-API-Key）—
// 讓「輸入管理員 API Key 解鎖」CTA 輸入的 key 真的生效。
import { getJSON } from '../shared/app-utils.js';

const TIMELINE_BADGE = {
  process_started:   { cls: 'badge info', label: '啟動', color: 'var(--color-info)' },
  process_exited:    { cls: 'badge err',  label: '停止', color: 'var(--color-danger)' },
  process_restarted: { cls: 'badge warn', label: '重啟', color: 'var(--color-warning)' },
  health_failed:     { cls: 'badge err',  label: '健康檢查失敗', color: 'var(--color-danger)' },
  health_passed:     { cls: 'badge ok',   label: '健康檢查通過', color: 'var(--color-success)' },
  restart_failed:    { cls: 'badge err',  label: '重啟失敗', color: 'var(--color-danger)' },
  error:             { cls: 'badge err',  label: '錯誤', color: 'var(--color-danger)' },
  info:              { cls: 'badge info', label: '資訊', color: 'var(--color-info)' },
};

function freshnessColor(ageSec) {
  if (ageSec == null || ageSec < 0) return 'var(--muted)';
  if (ageSec < 10) return 'var(--color-success)';
  if (ageSec < 30) return 'var(--color-warning)';
  return 'var(--color-danger)';
}

function formatRelativeAge(ageSec) {
  if (ageSec == null) return '從未';
  if (ageSec < 5) return '剛剛';
  if (ageSec < 60) return `${ageSec}s 前`;
  if (ageSec < 3600) return `${Math.floor(ageSec / 60)}m 前`;
  if (ageSec < 86400) return `${Math.floor(ageSec / 3600)}h 前`;
  return `${Math.floor(ageSec / 86400)}d 前`;
}

function formatTimestamp(iso) {
  if (!iso || iso.startsWith('0001-01-01')) return '-';
  try {
    return new Date(iso).toLocaleString('zh-TW');
  } catch (_) {
    return iso;
  }
}

export class DeploymentDashboard {
  /**
   * @param {string} containerId - DOM id of the mount point
   */
  constructor(containerId) {
    this.container = document.getElementById(containerId);
    if (!this.container) {
      console.warn(`DeploymentDashboard: container #${containerId} not found`);
      return;
    }

    /** @type {object|null} last successful status payload, kept for fallback rendering */
    this.lastStatus = null;

    /** @type {number|null} setInterval handle */
    this.refreshInterval = null;

    /** @type {boolean} flip to true in destroy() to abort in-flight fetches */
    this.isDestroyed = false;

    this.renderSkeleton();
    this.fetchStatus();
    this.startAutoRefresh();
  }

  // ──────────────────────────────────────────────────────────────────────
  // Skeleton — rendered synchronously in constructor to prevent FOUC.
  // ──────────────────────────────────────────────────────────────────────

  renderSkeleton() {
    this.container.innerHTML = `
      <div class="deployment-dashboard">
        <div class="flex-between mb-sm">
          <h2 class="m-0">🚀 部署健康度</h2>
          <div class="control-group m-0">
            <span id="deploymentLastUpdate" class="text-muted text-sm">載入中…</span>
            <button onclick="window.deploymentDashboard?.fetchStatus()" title="手動刷新">🔄</button>
          </div>
        </div>

        <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:var(--space-sm);" class="mb-sm">
          <div style="border:1px solid var(--border);border-radius:var(--editorial-radius-sm);padding:var(--space-sm);">
            <div class="text-sm text-muted">總體狀態</div>
            <div class="text-lg" id="deploymentSupervisorState">-</div>
          </div>
          <div style="border:1px solid var(--border);border-radius:var(--editorial-radius-sm);padding:var(--space-sm);">
            <div class="text-sm text-muted">程序存活</div>
            <div class="text-lg" id="deploymentProcessAlive">-</div>
          </div>
          <div style="border:1px solid var(--border);border-radius:var(--editorial-radius-sm);padding:var(--space-sm);">
            <div class="text-sm text-muted">重啟次數</div>
            <div class="text-lg" id="deploymentRestartCount">-</div>
          </div>
          <div style="border:1px solid var(--border);border-radius:var(--editorial-radius-sm);padding:var(--space-sm);">
            <div class="text-sm text-muted">最後心跳</div>
            <div class="text-lg" id="deploymentLastBeat">-</div>
          </div>
        </div>

        <h3 class="text-muted mb-sm">程序資訊</h3>
        <div id="deploymentProcessInfo" class="text-sm">載入中…</div>

        <h3 class="text-muted mb-sm mt-sm">最近事件 (timeline, 最新 10 筆)</h3>
        <div class="table-wrapper">
          <table id="deploymentTimelineTable">
            <thead>
              <tr><th>時間</th><th>類型</th><th>細節</th></tr>
            </thead>
            <tbody id="deploymentTimelineTableBody">
              <tr><td colspan="3" class="empty loading">載入中…</td></tr>
            </tbody>
          </table>
        </div>

        <h3 class="text-muted mb-sm mt-sm">最近錯誤 (最新 5 筆)</h3>
        <div id="deploymentErrorsList" class="text-sm">載入中…</div>
      </div>
    `;
  }

  // ──────────────────────────────────────────────────────────────────────
  // Fetch — single endpoint, handles auth/503/network states explicitly.
  // ──────────────────────────────────────────────────────────────────────

  async fetchStatus() {
    if (this.isDestroyed || !this.container) return;

    try {
      // 走 app-utils getJSON：授權者（localStorage 已有 ATLAS_API_KEY）的
      // GET 靜默帶 X-API-Key（PR-2），未授權者不帶 key、維持誠實未授權態。
      const status = await getJSON('/api/admin/live/deployment/dashboard');
      this.lastStatus = status;
      this.updateView(status);
      this.updateLastUpdateTime('剛剛');
    } catch (err) {
      if (err && (err.status === 401 || err.status === 403)) {
        this.showAuthMissingState();
        this.updateLastUpdateTime('未授權');
        return;
      }
      if (err && err.status === 503) {
        this.showUnwiredState();
        this.updateLastUpdateTime('管理員未連線');
        return;
      }
      console.error('DeploymentDashboard: failed to fetch status:', err);
      this.showNetworkErrorState();
      this.updateLastUpdateTime('錯誤');
    }
  }

  /**
   * 未授權態的 CTA（PR-7）：開管理員 apiKeyModal 輸入 key（admin main.js 以
   * window.__atlasPromptForApiKey 接線），儲存後重試 fetchStatus — key 已進
   * localStorage，getJSON 下一輪 GET 即靜默帶上 X-API-Key。
   */
  unlockWithApiKey() {
    const promptFn = (typeof window !== 'undefined' && typeof window.__atlasPromptForApiKey === 'function')
      ? window.__atlasPromptForApiKey
      : null;
    if (!promptFn) return;
    promptFn().then(() => {
      if (this.isDestroyed || !this.container) return;
      this.fetchStatus();
    });
  }

  // ──────────────────────────────────────────────────────────────────────
  // View — update DOM nodes in place; preserves outer skeleton structure.
  // ──────────────────────────────────────────────────────────────────────

  updateView(status) {
    if (!this.container) return;
    if (status.never_started) {
      // Supervisor 從未啟動（模擬階段的正常姿態）——顯示中性「未啟用」，
      // 不渲染成 DOWN/死亡 假警報。
      const stateEl = this.container.querySelector('#deploymentSupervisorState');
      const aliveEl = this.container.querySelector('#deploymentProcessAlive');
      const beatEl = this.container.querySelector('#deploymentLastBeat');
      if (stateEl) stateEl.innerHTML = '<span class="badge">未啟用</span>';
      if (aliveEl) aliveEl.innerHTML = '<span class="badge">—</span>';
      if (beatEl) beatEl.textContent = '—';
      const restartEl = this.container.querySelector('#deploymentRestartCount');
      if (restartEl) restartEl.textContent = '0';
      this.renderProcessInfo(status);
      this.renderTimeline(status);
      this.renderRecentErrors(status);
      return;
    }
    this.renderSummaryStats(status);
    this.renderProcessInfo(status);
    this.renderTimeline(status);
    this.renderRecentErrors(status);
  }

  renderSummaryStats(status) {
    const stateEl = this.container.querySelector('#deploymentSupervisorState');
    const aliveEl = this.container.querySelector('#deploymentProcessAlive');
    const restartEl = this.container.querySelector('#deploymentRestartCount');
    const beatEl = this.container.querySelector('#deploymentLastBeat');

    if (stateEl) {
      if (status.supervisor_running && status.process_alive) {
        stateEl.innerHTML = `<span class="badge ok">UP</span>`;
      } else if (status.supervisor_running) {
        stateEl.innerHTML = `<span class="badge warn">DEGRADED</span>`;
      } else {
        stateEl.innerHTML = `<span class="badge err">DOWN</span>`;
      }
    }
    if (aliveEl) {
      aliveEl.innerHTML = status.process_alive
        ? `<span class="badge ok">存活</span>`
        : `<span class="badge err">死亡</span>`;
    }
    if (restartEl) {
      restartEl.textContent = fmt(status.restart_count);
    }
    if (beatEl) {
      const ageSec = status.last_beat_age_sec;
      const label = formatRelativeAge(ageSec);
      const color = freshnessColor(ageSec);
      // textContent for safety (numeric only via formatters, but defense in depth)
      beatEl.textContent = label;
      beatEl.style.color = color;
    }
  }

  renderProcessInfo(status) {
    const el = this.container.querySelector('#deploymentProcessInfo');
    if (!el) return;

    const started = formatTimestamp(status.started_at);
    const cfg = status.config || {};
    const configBits = [
      `binary: ${escapeHtml(cfg.binary || '-')}`,
      `listen_port: ${cfg.listen_port ?? '-'}`,
      `auto_restart: ${cfg.auto_restart ? 'true' : 'false'}`,
      `max_restarts: ${cfg.max_restarts ?? '-'}`,
    ];

    const errBlock = status.last_error
      ? `<div class="error-banner mt-sm"><strong>最後錯誤:</strong> ${escapeHtml(status.last_error)}</div>`
      : '';

    el.innerHTML = `
      <div class="flex-between text-sm">
        <span>PID: <strong>${fmt(status.pid)}</strong></span>
        <span>Port: <strong>${fmt(status.port)}</strong></span>
        <span>啟動時間: <strong>${escapeHtml(started)}</strong></span>
      </div>
      <div class="text-muted mt-sm" style="font-family:var(--font-mono);font-size:var(--text-sm);">
        ${configBits.map(s => escapeHtml(s)).join(' &middot; ')}
      </div>
      ${errBlock}
    `;
  }

  renderTimeline(status) {
    const tbody = this.container.querySelector('#deploymentTimelineTableBody');
    if (!tbody) return;

    const events = Array.isArray(status.recent_events) ? status.recent_events : [];
    if (events.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" class="empty">尚無事件</td></tr>';
      return;
    }

    // latest 10, newest first
    const recent = events.slice(-10).reverse();
    const rows = recent.map(ev => {
      const kind = (ev.kind || 'info').toLowerCase();
      const badge = TIMELINE_BADGE[kind] || TIMELINE_BADGE.info;
      const ts = formatTimestamp(ev.at);
      return `
        <tr>
          <td>${escapeHtml(ts)}</td>
          <td><span class="${badge.cls}">${badge.label}</span></td>
          <td>${escapeHtml(ev.detail || '-')}</td>
        </tr>
      `;
    }).join('');

    tbody.innerHTML = rows;
  }

  renderRecentErrors(status) {
    const el = this.container.querySelector('#deploymentErrorsList');
    if (!el) return;

    const events = Array.isArray(status.recent_events) ? status.recent_events : [];
    const errors = events
      .filter(ev => {
        const k = (ev.kind || '').toLowerCase();
        return k === 'health_failed' || k === 'restart_failed' || k === 'process_exited';
      })
      .slice(-5)
      .reverse();

    if (errors.length === 0) {
      el.innerHTML = `<span class="text-success">無最近錯誤</span>`;
      return;
    }

    const items = errors.map(ev => {
      const ts = formatTimestamp(ev.at);
      return `
        <div class="flex-between" style="padding:var(--space-xs) 0;border-bottom:1px solid var(--border);">
          <span class="text-muted text-sm" style="font-family:var(--font-mono);">${escapeHtml(ts)}</span>
          <span class="text-danger text-sm" style="flex:1;margin-left:var(--space-sm);">${escapeHtml(ev.detail || '-')}</span>
        </div>
      `;
    }).join('');

    el.innerHTML = items;
  }

  // ──────────────────────────────────────────────────────────────────────
  // Error states — replace entire content with banner; stops further fetches
  // until user navigates back / retries.
  // ──────────────────────────────────────────────────────────────────────

  showAuthMissingState() {
    if (!this.container) return;
    // 未授權文案 + CTA（PR-7）：admin 有 apiKeyModal 輸入流程（非死路），
    // 點「點此輸入解鎖」開 modal，輸入 key 後重試即解鎖（key 存 localStorage，
    // getJSON 下一輪 GET 靜默帶 X-API-Key）。無 prompt hook（client_web 等
    // 未接 modal 的環境）時退回誠實文案。
    const hasPrompt = typeof window !== 'undefined' && typeof window.__atlasPromptForApiKey === 'function';
    this.container.innerHTML = `
      <div class="deployment-dashboard">
        <h2 class="m-0">🚀 部署健康度</h2>
        <div class="error-banner mt-sm">
          <span>⚠️ 需要管理員 API Key，${hasPrompt
            ? '<button class="retry-btn" onclick="window.deploymentDashboard && window.deploymentDashboard.unlockWithApiKey()">點此輸入解鎖</button>'
            : '瀏覽器環境無法提供'}</span>
        </div>
      </div>
    `;
  }

  showUnwiredState() {
    if (!this.container) return;
    this.container.innerHTML = `
      <div class="deployment-dashboard">
        <h2 class="m-0">🚀 部署健康度</h2>
        <div class="error-banner mt-sm">
          <span>⚠️ 部署管理器尚未連線</span>
        </div>
      </div>
    `;
  }

  showNetworkErrorState() {
    if (!this.container) return;
    // Retain last good view if available; just update the timestamp.
    if (this.lastStatus && this.container.querySelector('#deploymentSupervisorState')) {
      this.updateLastUpdateTime('錯誤(下次重試)');
      return;
    }
    this.container.innerHTML = `
      <div class="deployment-dashboard">
        <h2 class="m-0">🚀 部署健康度</h2>
        <div class="error-banner mt-sm">
          <span>⚠️ 無法取得部署狀態,將於下次輪詢重試</span>
          <button class="retry-btn" onclick="window.deploymentDashboard?.fetchStatus()">重試</button>
        </div>
      </div>
    `;
  }

  // ──────────────────────────────────────────────────────────────────────
  // Lifecycle — public hooks for app-level control.
  // ──────────────────────────────────────────────────────────────────────

  updateLastUpdateTime(text) {
    const el = this.container?.querySelector('#deploymentLastUpdate');
    if (el) el.textContent = text;
  }

  startAutoRefresh() {
    this.stopAutoRefresh();
    this.refreshInterval = setInterval(() => {
      this.fetchStatus();
    }, 5000);
  }

  stopAutoRefresh() {
    if (this.refreshInterval) {
      clearInterval(this.refreshInterval);
      this.refreshInterval = null;
    }
  }

  /** @public manual start (no-op if already started) */
  start() {
    if (!this.container || this.isDestroyed) return;
    this.startAutoRefresh();
    this.fetchStatus();
  }

  /** @public manual stop */
  stop() {
    this.stopAutoRefresh();
  }

  /** @public tear-down */
  destroy() {
    this.isDestroyed = true;
    this.stopAutoRefresh();
  }
}

// Expose to window so inline onclick handlers (e.g. in renderSkeleton) work.
window.DeploymentDashboard = DeploymentDashboard;
