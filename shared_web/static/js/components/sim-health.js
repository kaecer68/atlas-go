/**
 * Simulation Health Dashboard Component
 * Fetches and displays simulation trace records from /api/traces/sim-latest
 */

const STATUS_COLORS = {
  OK: { class: 'badge ok', label: 'OK', color: 'var(--color-success)' },
  WARN: { class: 'badge warn', label: 'WARN', color: 'var(--color-warning)' },
  FAIL: { class: 'badge err', label: 'FAIL', color: 'var(--color-danger)' },
  START: { class: 'badge info', label: 'START', color: 'var(--color-info)' }
};

export class SimHealthPanel {
  constructor(containerId) {
    this.container = document.getElementById(containerId);
    if (!this.container) {
      console.warn(`SimHealthPanel: container #${containerId} not found`);
      return;
    }

    this.traces = [];
    this.refreshInterval = null;
    this.isDestroyed = false;

    this.renderSkeleton();
    this.fetchTraces();
    this.startAutoRefresh();
  }

  renderSkeleton() {
    this.container.innerHTML = `
      <div class="sim-health-panel">
        <div class="flex-between mb-sm">
          <h2 class="m-0">🩺 模擬健康度</h2>
          <div class="control-group m-0">
            <span id="simHealthLastUpdate" class="text-muted text-sm">載入中…</span>
            <button onclick="window.simHealthPanel?.fetchTraces()" title="手動刷新">🔄</button>
          </div>
        </div>
        <div class="sim-health-summary" id="simHealthSummary">
          <div class="sim-health-stat">
            <div class="sim-health-stat__value" id="simStatTotal">-</div>
            <div class="sim-health-stat__label">總步驟</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--ok" id="simStatOk">-</div>
            <div class="sim-health-stat__label">正常</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--warn" id="simStatWarn">-</div>
            <div class="sim-health-stat__label">警告</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--fail" id="simStatFail">-</div>
            <div class="sim-health-stat__label">失敗</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--running" id="simStatRunning">-</div>
            <div class="sim-health-stat__label">進行中</div>
          </div>
        </div>
        <div class="table-wrapper mt-sm">
          <table id="simHealthTable">
            <thead>
              <tr>
                <th>步驟</th>
                <th>層級</th>
                <th>狀態</th>
                <th>時間戳</th>
                <th>元數據</th>
              </tr>
            </thead>
            <tbody id="simHealthTableBody">
              <tr><td colspan="5" class="empty loading">載入中…</td></tr>
            </tbody>
          </table>
        </div>
      </div>
    `;
  }

  async fetchTraces() {
    if (this.isDestroyed) return;

    try {
      const res = await fetch('/api/traces/sim-latest');

      if (res.status === 404) {
        this.showEmptyState('尚無模擬追蹤記錄');
        this.updateLastUpdateTime('無資料');
        return;
      }

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }

      const data = await res.json();
      this.traces = Array.isArray(data) ? data : (data.traces || []);
      this.updateUI();
      this.updateLastUpdateTime('剛剛');
    } catch (err) {
      console.error('SimHealthPanel: failed to fetch traces:', err);
      this.showErrorState('無法載入模擬追蹤資料');
      this.updateLastUpdateTime('錯誤');
    }
  }

  updateUI() {
    if (!this.container) return;

    // 2026-08-24 UI audit P3：步驟依時間戳排序（新→舊），避免 1,1,2,3,4,5,2,3,4,5
    // 混排難以追蹤單一步驟。
    const sorted = Array.from(this.traces).sort(function (a, b) {
      return new Date(b.ts || 0) - new Date(a.ts || 0);
    });
    this.traces = sorted;

    const summary = this.computeSummary();
    this.updateSummary(summary);
    this.renderTable();
  }

  computeSummary() {
    // 追蹤事件是「layer 發 START 再發 OK/WARN/FAIL」成對出現——逐事件統計
    // 會把每個 layer 多算一筆 START（看起來像 7 個步驟永遠不完成）。
    // 正確語意：以 layer 為單位，取最終狀態 FAIL > WARN > OK；只有 START
    // 的 layer 才是進行中。
    const byLayer = new Map();
    for (const trace of this.traces) {
      const layer = trace.layer || `step-${trace.step}`;
      const status = (trace.status || '').toUpperCase();
      const prev = byLayer.get(layer);
      // 優先序：FAIL > WARN > OK > START
      const rank = { FAIL: 3, WARN: 2, OK: 1, START: 0 };
      if (!prev || (rank[status] || 0) > (rank[prev] || 0)) {
        byLayer.set(layer, status);
      }
    }
    const summary = { total: 0, ok: 0, warn: 0, fail: 0, running: 0 };
    for (const status of byLayer.values()) {
      summary.total++;
      if (status === 'OK') summary.ok++;
      else if (status === 'WARN') summary.warn++;
      else if (status === 'FAIL') summary.fail++;
      else summary.running++;
    }
    return summary;
  }

  updateSummary(summary) {
    const totalEl = this.container.querySelector('#simStatTotal');
    const okEl = this.container.querySelector('#simStatOk');
    const warnEl = this.container.querySelector('#simStatWarn');
    const failEl = this.container.querySelector('#simStatFail');

    const runningEl = this.container.querySelector('#simStatRunning');
    if (totalEl) totalEl.textContent = summary.total;
    if (okEl) okEl.textContent = summary.ok;
    if (warnEl) warnEl.textContent = summary.warn;
    if (failEl) failEl.textContent = summary.fail;
    if (runningEl) runningEl.textContent = summary.running || 0;
  }

  renderTable() {
    const tbody = this.container.querySelector('#simHealthTableBody');
    if (!tbody) return;

    if (this.traces.length === 0) {
      tbody.innerHTML = '<tr><td colspan="5" class="empty">尚無追蹤記錄</td></tr>';
      return;
    }

    const rows = this.traces.map(trace => {
      const status = (trace.status || 'UNKNOWN').toUpperCase();
      const statusConfig = STATUS_COLORS[status] || { class: 'badge', label: status, color: 'var(--muted)' };
      const timestamp = trace.ts
        ? new Date(trace.ts).toLocaleString('zh-TW')
        : '-';
      const metadata = this.formatMetadata(trace.metadata);

      return `
        <tr class="sim-health-row sim-health-row--${status.toLowerCase()}">
          <td class="sim-health-cell sim-health-cell--step">${escapeHtml(trace.step || '-')}</td>
          <td class="sim-health-cell sim-health-cell--layer">${escapeHtml(trace.layer || '-')}</td>
          <td class="sim-health-cell sim-health-cell--status">
            <span class="${statusConfig.class}">${statusConfig.label}</span>
          </td>
          <td class="sim-health-cell sim-health-cell--time">${escapeHtml(timestamp)}</td>
          <td class="sim-health-cell sim-health-cell--meta">${metadata}</td>
        </tr>
      `;
    }).join('');

    tbody.innerHTML = rows;
  }

  formatMetadata(metadata) {
    if (!metadata || typeof metadata !== 'object') {
      return '<span class="text-muted">-</span>';
    }

    const entries = Object.entries(metadata);
    if (entries.length === 0) {
      return '<span class="text-muted">-</span>';
    }

    // Show first 2 key-value pairs, truncate if more
    const maxEntries = 2;
    const visible = entries.slice(0, maxEntries);
    const remaining = entries.length - maxEntries;

    const parts = visible.map(([key, value]) => {
      const displayValue = typeof value === 'object'
        ? JSON.stringify(value)
        : String(value);
      const truncated = displayValue.length > 30
        ? displayValue.substring(0, 30) + '…'
        : displayValue;
      return `<span class="sim-health-meta-item"><span class="sim-health-meta-key">${escapeHtml(key)}:</span> <span class="sim-health-meta-val">${escapeHtml(truncated)}</span></span>`;
    });

    if (remaining > 0) {
      parts.push(`<span class="sim-health-meta-more">+${remaining} 項</span>`);
    }

    return `<div class="sim-health-meta">${parts.join('')}</div>`;
  }

  showEmptyState(message) {
    const tbody = this.container?.querySelector('#simHealthTableBody');
    if (tbody) {
      tbody.innerHTML = `<tr><td colspan="5" class="empty">${escapeHtml(message)}</td></tr>`;
    }
    this.updateSummary({ total: 0, ok: 0, warn: 0, fail: 0 });
  }

  showErrorState(message) {
    const tbody = this.container?.querySelector('#simHealthTableBody');
    if (tbody) {
      tbody.innerHTML = `
        <tr>
          <td colspan="5">
            <div class="error-banner">
              <span>⚠️ ${escapeHtml(message)}</span>
              <button class="retry-btn" onclick="window.simHealthPanel?.fetchTraces()">重試</button>
            </div>
          </td>
        </tr>
      `;
    }
    this.updateSummary({ total: 0, ok: 0, warn: 0, fail: 0 });
  }

  updateLastUpdateTime(text) {
    const el = this.container?.querySelector('#simHealthLastUpdate');
    if (el) el.textContent = text;
  }

  startAutoRefresh() {
    this.refreshInterval = setInterval(() => {
      this.fetchTraces();
    }, 5000);
  }

  stopAutoRefresh() {
    if (this.refreshInterval) {
      clearInterval(this.refreshInterval);
      this.refreshInterval = null;
    }
  }

  destroy() {
    this.isDestroyed = true;
    this.stopAutoRefresh();
  }
}

function escapeHtml(str) {
  if (str == null) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

// Expose to window for inline onclick handlers
window.SimHealthPanel = SimHealthPanel;
