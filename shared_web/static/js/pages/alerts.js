// Alert management page — Decision-assistance interface
// Principle: machine handles noise, humans only make decisions.
import { getJSON, postJSON, notify } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

const STATUS_MAP = {
  triggered: '觸發中',
  acknowledged: '已確認',
  resolved: '已解決',
  silenced: '已抑制',
};

const STATUS_CLASS = {
  triggered: 'err',
  acknowledged: 'warn',
  resolved: 'ok',
  silenced: 'muted',
};

const SEVERITY_MAP = { critical: '嚴重', error: '錯誤', warning: '警告', info: '資訊' };
const SEVERITY_CLASS = { critical: 'err', error: 'err', warning: 'warn', info: 'info' };

const PAGE_SIZE = 20;

let currentAlerts = [];
let currentStats = null;
let currentFilter = 'triggered'; // default: only show triggered
let currentPage = 0;

function formatTime(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleString('zh-TW');
}

function formatDate(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleDateString('zh-TW');
}

function formatTimeShort(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' });
}

// ── Rule display labels ──
const RULE_LABELS = {
  'narrative_theme_detected': '敘事主題警示',
};

// ruleLabel returns the Traditional-Chinese display label for an alert
// rule id; unknown rules fall back to the raw id.
function ruleLabel(rule) {
  return RULE_LABELS[rule] || rule;
}

// ── Machine-recommended actions per rule ──
function getMachineAdvice(alert) {
  const rule = alert.rule || '';
  const sev = (alert.severity || '').toLowerCase();

  const adviceMap = {
    'background_task': {
      problem: '背景任務連續失敗，可能影響資料同步',
      tried: ['自動重試 3 次', '檢查通道健康狀態'],
      rootCause: alert.message || '任務執行異常',
      actions: [
        '檢查對應資料通道是否正常',
        '手動觸發任務：go run ./cmd/daily-replay-sync',
        '查看背景任務日誌定位具體錯誤',
      ],
      impact: '若未處理，相關資料將無法更新，影響策略信號',
      duration: '持續直到任務恢復',
    },
    'simulation': {
      problem: '模擬交易產生異常結果',
      tried: ['檢查 regime 狀態', '驗證訂單邏輯'],
      rootCause: alert.message || '模擬異常',
      actions: [
        '檢查當日 regime 是否為 RISK_OFF（0 訂單為預期行為）',
        '查看模擬日誌確認具體異常點',
        '若為資料缺失，執行回補任務',
      ],
      impact: '當日策略信號可能不完整',
      duration: '單日影響',
    },
    'experiment': {
      problem: '實驗評估需要更多資料',
      tried: ['檢查樣本數量', '驗證統計顯著性'],
      rootCause: alert.message || '資料不足',
      actions: [
        '等待累積更多回測場次',
        '檢查實驗配置是否合理',
        '手動執行額外回測窗口',
      ],
      impact: '實驗結論可信度不足，暫不建議晉升',
      duration: '直到樣本數達標',
    },
    'data_staleness': {
      problem: '資料檔案超過更新週期',
      tried: ['檢查最後修改時間', '比對資料源狀態'],
      rootCause: alert.message || '資料過期',
      actions: [
        '執行對應回補指令（見警報訊息）',
        '檢查資料通道是否運作正常',
      ],
      impact: '過期資料可能導致策略信號偏差',
      duration: '直到資料更新',
    },
    'state_store': {
      problem: '狀態存儲無法讀取投組資訊',
      tried: ['檢查檔案系統權限', '驗證 JSON 格式'],
      rootCause: alert.message || '狀態存儲異常',
      actions: [
        '檢查 data/state/ 目錄權限',
        '確認 live state store JSON 格式正確',
        '若損毀，從備份恢復或重新初始化',
      ],
      impact: 'Live 交易模式無法獲取持倉資訊',
      duration: '直到存儲恢復',
    },
    'etf_nav': {
      problem: 'ETF 淨值追蹤異常',
      tried: ['檢查 NAV 資料源', '比對市場價格'],
      rootCause: alert.message || 'NAV 異常',
      actions: [
        '檢查 ETF 資料通道狀態',
        '手動驗證 NAV 計算邏輯',
      ],
      impact: 'ETF 相關策略可能產生錯誤信號',
      duration: '直到 NAV 資料恢復',
    },
  };

  // Default fallback for unknown rules
  const fallback = {
    problem: alert.message || '系統異常',
    tried: ['自動檢測異常', '記錄詳細資訊'],
    rootCause: alert.message || '未知原因',
    actions: [
      '查看詳細日誌',
      '檢查相關系統模組狀態',
      '必要時手動介入排查',
    ],
    impact: '影響範圍待評估',
    duration: '待觀察',
  };

  return adviceMap[rule] || fallback;
}

function renderHealthSummary() {
  const el = document.getElementById('alertHealthSummary');
  if (!el) return;

  // Fetch channel health from the existing endpoint
  getJSON('/api/dashboard/channel-health').then(data => {
    const channels = data?.channels || [];
    const total = channels.length;
    const ok = channels.filter(c => c.status === 'ok').length;
    const warn = channels.filter(c => c.status === 'warn').length;
    const err = channels.filter(c => c.status === 'error').length;

    const badChannels = channels
      .filter(c => c.status !== 'ok' && c.status !== 'inactive')
      .map(c => `<span class="badge err" style="font-size:10px;padding:1px 5px">${escapeHtml(c.channel_id)}</span>`)
      .join(' ') || '<span style="color:var(--muted);font-size:12px">全部正常</span>';

    el.innerHTML = `
      <div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap">
        <div style="display:flex;gap:4px;align-items:center">
          <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:var(--color-success)"></span>
          <span class="text-sm">${ok}/${total} 正常</span>
        </div>
        ${warn > 0 ? `<div style="display:flex;gap:4px;align-items:center"><span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:var(--color-warning)"></span><span class="text-sm">${warn} 警告</span></div>` : ''}
        ${err > 0 ? `<div style="display:flex;gap:4px;align-items:center"><span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:var(--color-danger)"></span><span class="text-sm">${err} 異常</span></div>` : ''}
        <div style="margin-left:auto;font-size:11px;color:var(--muted)">上次更新: ${formatTimeShort(data?.updated_at)}</div>
      </div>
      ${(warn > 0 || err > 0) ? `<div style="margin-top:6px">${badChannels}</div>` : ''}
    `;
  }).catch(() => {
    el.innerHTML = '<span style="color:var(--muted);font-size:12px">健康狀態載入失敗</span>';
  });
}

function renderStats() {
  const el = document.getElementById('alertSummary');
  if (!el || !currentStats) return;

  const triggered = currentStats.triggered || 0;
  const autoResolved = currentStats.acknowledged || 0;
  const resolved = currentStats.resolved || 0;
  const total24h = currentStats.last_24h || 0;

  el.innerHTML = `
    <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center">
      ${triggered > 0
        ? `<span class="badge ${STATUS_CLASS.triggered}">⚠️ 需決策 ${triggered}</span>`
        : `<span class="badge ok">✓ 無需決策</span>`}
      <span class="badge" style="background:var(--bg);color:var(--muted);border:1px solid var(--border)">機器已處理 ${autoResolved}</span>
      <span class="badge ${STATUS_CLASS.resolved}">已解決 ${resolved}</span>
      <span class="badge" style="background:var(--bg);color:var(--muted);border:1px solid var(--border)">24h 內 ${total24h}</span>
    </div>
  `;
}

function updateFilterButtons() {
  document.querySelectorAll('#alertFilters .view-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.filter === currentFilter);
  });
}

function getFilteredAlerts() {
  if (currentFilter === 'all') return currentAlerts;
  return currentAlerts.filter(a => (a.status || 'triggered') === currentFilter);
}

function renderPagination(totalItems, page) {
  const totalPages = Math.ceil(totalItems / PAGE_SIZE);
  if (totalPages <= 1) return '';
  return `
    <div class="table-pagination" style="margin-top:10px">
      <span>顯示 <strong>${Math.min(page * PAGE_SIZE + 1, totalItems)}-${Math.min((page + 1) * PAGE_SIZE, totalItems)}</strong> / 共 <strong>${totalItems}</strong> 筆</span>
      <div style="display:flex;gap:6px;align-items:center">
        <button onclick="window._alertPage=0;window._alertRender()" ${page===0?'disabled':''}>« 首頁</button>
        <button onclick="window._alertPage=${page-1};window._alertRender()" ${page===0?'disabled':''}>‹ 上一頁</button>
        <span style="padding:0 8px">第 ${page + 1} / ${totalPages} 頁</span>
        <button onclick="window._alertPage=${page+1};window._alertRender()" ${page>=totalPages-1?'disabled':''}>下一頁 ›</button>
        <button onclick="window._alertPage=${totalPages-1};window._alertRender()" ${page>=totalPages-1?'disabled':''}>末頁 »</button>
      </div>
    </div>
  `;
}

function renderAlertCard(a) {
  const sevClass = SEVERITY_CLASS[a.severity] || 'info';
  const statusClass = STATUS_CLASS[a.status] || 'err';
  const statusLabel = STATUS_MAP[a.status] || (a.status || '觸發中');
  const advice = getMachineAdvice(a);

  const actionList = advice.actions.map(act =>
    `<li style="margin-bottom:4px">• ${escapeHtml(act)}</li>`
  ).join('');

  return `
    <div class="panel" style="margin-bottom:12px;border-left:4px solid var(--color-${sevClass === 'err' ? 'danger' : sevClass === 'warn' ? 'warning' : 'info'})">
      <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:12px;flex-wrap:wrap">
        <div style="flex:1;min-width:200px">
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:6px;flex-wrap:wrap">
            <span class="badge ${statusClass}" style="font-size:11px;padding:2px 7px">${escapeHtml(statusLabel)}</span>
            <span class="badge ${sevClass}" style="font-size:11px;padding:2px 7px">${escapeHtml(SEVERITY_MAP[a.severity]) || escapeHtml(a.severity)}</span>
            <span style="font-size:12px;color:var(--muted)">${escapeHtml(ruleLabel(a.rule))}</span>
            ${a.count > 1 ? `<span class="badge warn" style="font-size:10px;padding:1px 5px">×${a.count}</span>` : ''}
          </div>
          <div style="font-size:13px;font-weight:600;margin-bottom:8px">${escapeHtml(advice.problem)}</div>
          <div style="font-size:12px;color:var(--muted);margin-bottom:4px">
            <strong>根因：</strong>${escapeHtml(advice.rootCause)}
          </div>
          <div style="font-size:12px;color:var(--muted);margin-bottom:8px">
            <strong>機器已嘗試：</strong>${advice.tried.join('、')}
          </div>
        </div>
        <div style="text-align:right;white-space:nowrap">
          <div style="font-size:12px;color:var(--muted)">${formatTime(a.timestamp)}</div>
        </div>
      </div>

      <div style="background:var(--bg);border-radius:6px;padding:10px 12px;margin:8px 0">
        <div style="font-size:12px;font-weight:600;margin-bottom:6px">🤖 建議行動</div>
        <ul style="font-size:12px;color:var(--text);margin:0;padding-left:16px;line-height:1.6">${actionList}</ul>
      </div>

      <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;font-size:11px;color:var(--muted);margin-bottom:10px">
        <span>📍 影響：${escapeHtml(advice.impact)}</span>
        <span>|</span>
        <span>⏱️ 預計持續：${escapeHtml(advice.duration)}</span>
      </div>

      <div style="display:flex;gap:8px;flex-wrap:wrap">
        ${a.status === 'triggered' ? `
          <button class="primary" style="font-size:12px;padding:6px 14px" onclick="acknowledgeAlert('${escapeHtml(a.id)}')">✓ 已知悉</button>
          <button style="font-size:12px;padding:6px 14px;background:var(--color-danger);color:#fff;border:none;border-radius:4px;cursor:pointer" onclick="resolveAlert('${escapeHtml(a.id)}')">🔧 已修復</button>
        ` : ''}
        ${a.status === 'acknowledged' ? `
          <button style="font-size:12px;padding:6px 14px;background:var(--color-danger);color:#fff;border:none;border-radius:4px;cursor:pointer" onclick="resolveAlert('${escapeHtml(a.id)}')">🔧 已修復</button>
          <span style="font-size:12px;color:var(--muted)">已於 ${formatDate(a.acknowledged_at)} 確認</span>
        ` : ''}
        ${a.status === 'resolved' ? `
          <span style="font-size:12px;color:var(--color-success)">✓ 已解決 (${escapeHtml(a.resolved_by || 'unknown')} ${formatDate(a.resolved_at)})</span>
        ` : ''}
      </div>
    </div>
  `;
}

function renderAlertTable() {
  const el = document.getElementById('alertsPanel');
  if (!el) return;

  renderStats();
  renderHealthSummary();

  if (!currentAlerts || currentAlerts.length === 0) {
    el.innerHTML = `<div class="empty" style="padding:20px 0;text-align:center">
      <div style="font-size:14px;margin-bottom:8px">🎉 系統運行正常</div>
      <div style="font-size:12px;color:var(--muted)">目前沒有需要人類決策的警報。機器已自動處理所有已知噪音。</div>
    </div>`;
    el.classList.remove('loading');
    return;
  }

  el.classList.remove('loading');

  const filtered = getFilteredAlerts();

  if (filtered.length === 0) {
    el.innerHTML = `<div class="empty" style="padding:20px 0;text-align:center;color:var(--muted)">
      此狀態下沒有警報
    </div>`;
    return;
  }

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  if (currentPage >= totalPages) currentPage = Math.max(0, totalPages - 1);
  const startIdx = currentPage * PAGE_SIZE;
  const pagedItems = filtered.slice(startIdx, startIdx + PAGE_SIZE);

  const cards = pagedItems.map(renderAlertCard).join('');
  const paginationHtml = renderPagination(filtered.length, currentPage);

  el.innerHTML = cards + paginationHtml;
}

export function renderAlerts(data) {
  const el = document.getElementById('alertsPanel');
  if (!el) return;
  if (!data || !data.alerts) {
    currentAlerts = [];
  } else {
    currentAlerts = data.alerts;
  }
  currentPage = 0;
  renderAlertTable();
}

export async function acknowledgeAlert(alertId) {
  try {
    await postJSON('/api/alerts/acknowledge', { alert_id: alertId, user: 'human' });
    notify('已記錄確認', 'success');
    loadAlerts();
  } catch (e) {
    notify('確認失敗: ' + e.message, 'error');
  }
}

export async function resolveAlert(alertId) {
  try {
    await postJSON('/api/alerts/resolve', { alert_id: alertId, user: 'human' });
    notify('已標記為已修復', 'success');
    loadAlerts();
  } catch (e) {
    notify('標記失敗: ' + e.message, 'error');
  }
}

export async function loadAlerts() {
  try {
    // Default: only load triggered alerts (human decision needed)
    const data = await getJSON('/api/alerts?status=triggered&page_size=50');
    currentAlerts = data.alerts || [];
    // Also fetch stats
    const stats = await getJSON('/api/alerts/stats');
    currentStats = stats;
    currentPage = 0;
    renderAlertTable();
  } catch (e) {
    console.error(e);
  }
}

export async function loadAllAlerts() {
  try {
    const data = await getJSON('/api/alerts?page_size=200');
    currentAlerts = data.alerts || [];
    const stats = await getJSON('/api/alerts/stats');
    currentStats = stats;
    currentPage = 0;
    renderAlertTable();
  } catch (e) {
    console.error(e);
  }
}

export async function showUnacknowledgedOnly() {
  try {
    const data = await getJSON('/api/alerts/unacknowledged');
    currentAlerts = data.alerts || [];
    currentFilter = 'all';
    currentPage = 0;
    updateFilterButtons();
    renderAlertTable();
  } catch (e) {
    console.error(e);
  }
}

export function setAlertFilter(filter) {
  currentFilter = filter;
  currentPage = 0;
  updateFilterButtons();
  renderAlertTable();
}

// Expose render helper for pagination callbacks
if (typeof window !== "undefined") {
  window._alertPage = 0;
  window._alertRender = () => {
    currentPage = window._alertPage || 0;
    renderAlertTable();
  };
  Object.assign(window, {
    loadAlerts, loadAllAlerts, acknowledgeAlert, resolveAlert,
    showUnacknowledgedOnly, setAlertFilter,
  });
}
