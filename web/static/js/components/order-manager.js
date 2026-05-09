import { getJSON, notify, escapeHtml, formatDate } from '../shared/app-utils.js';
import { eventSource } from '../services/event-source.js';

let ordersCache = [];
let currentPage = 1;
let currentFilter = { status: 'all', symbol: '', startDate: '', endDate: '' };
let totalPages = 1;

export function renderOrderManager(container) {
  container.innerHTML = `
    <div class="order-manager">
      <div class="om-header">
        <h2>📝 訂單管理</h2>
        <div class="om-filters">
          <input type="text" id="omSymbolFilter" placeholder="過濾代號..." />
          <select id="omStatusFilter">
            <option value="all">所有狀態</option>
            <option value="pending">Pending</option>
            <option value="submitted">Submitted</option>
            <option value="filled">Filled</option>
            <option value="rejected">Rejected</option>
            <option value="cancelled">Cancelled</option>
          </select>
          <input type="date" id="omStartDate" />
          <span style="color:var(--muted);font-size:var(--text-sm)">~</span>
          <input type="date" id="omEndDate" />
          <button id="omRefreshBtn" class="primary">刷新</button>
        </div>
      </div>
      
      <div class="om-table-container">
        <table class="om-table">
          <thead>
            <tr>
              <th>Order ID</th>
              <th>Symbol</th>
              <th>Side</th>
              <th>Quantity</th>
              <th>Price</th>
              <th>Fill Price</th>
              <th>Status</th>
              <th>Created At</th>
            </tr>
          </thead>
          <tbody id="omTableBody">
            <tr><td colspan="8" style="text-align:center">載入中...</td></tr>
          </tbody>
        </table>
      </div>
      
      <div class="om-pagination">
        <span id="omPageInfo">Page 1 / 1</span>
        <button id="omPrevPage">上一頁</button>
        <button id="omNextPage">下一頁</button>
      </div>
    </div>
    
    <div class="om-modal-overlay" id="omDetailModal">
      <div class="om-modal">
        <div class="om-modal-header">
          <h3>訂單詳細資訊</h3>
          <button class="om-modal-close" id="omCloseModal">&times;</button>
        </div>
        <div class="om-modal-body" id="omModalBody">
          <!-- Details will be injected here -->
        </div>
      </div>
    </div>
  `;

  // Bind events
  document.getElementById('omSymbolFilter').addEventListener('input', (e) => {
    currentFilter.symbol = e.target.value.trim().toUpperCase();
    currentPage = 1;
    fetchOrders();
  });
  
  document.getElementById('omStatusFilter').addEventListener('change', (e) => {
    currentFilter.status = e.target.value;
    currentPage = 1;
    fetchOrders();
  });
  
  document.getElementById('omStartDate').addEventListener('change', (e) => {
    currentFilter.startDate = e.target.value;
    currentPage = 1;
    fetchOrders();
  });

  document.getElementById('omEndDate').addEventListener('change', (e) => {
    currentFilter.endDate = e.target.value;
    currentPage = 1;
    fetchOrders();
  });
  
  document.getElementById('omRefreshBtn').addEventListener('click', () => fetchOrders());
  
  document.getElementById('omPrevPage').addEventListener('click', () => {
    if (currentPage > 1) { currentPage--; fetchOrders(); }
  });
  
  document.getElementById('omNextPage').addEventListener('click', () => {
    if (currentPage < totalPages) { currentPage++; fetchOrders(); }
  });
  
  document.getElementById('omCloseModal').addEventListener('click', closeDetailModal);
  document.getElementById('omDetailModal').addEventListener('click', (e) => {
    if (e.target.id === 'omDetailModal') closeDetailModal();
  });

  // Setup SSE
  setupSSE();

  // Initial fetch
  fetchOrders();
}

async function fetchOrders() {
  const tbody = document.getElementById('omTableBody');
  try {
    let url = `/api/dashboard/orders?page=${currentPage}&page_size=20`;
    if (currentFilter.status !== 'all') url += `&status=${currentFilter.status}`;
    if (currentFilter.symbol) url += `&symbol=${currentFilter.symbol}`;
    if (currentFilter.startDate) url += `&start_date=${currentFilter.startDate}`;
    if (currentFilter.endDate) url += `&end_date=${currentFilter.endDate}`;
    
    const data = await getJSON(url).catch(() => ({ orders: [], total: 0, page: 1, page_size: 20 }));
    ordersCache = data.orders || [];
    
    // Default to 1 page if total is missing or 0
    totalPages = Math.max(1, Math.ceil((data.total || 0) / (data.page_size || 20)));
    currentPage = data.page || 1;
    
    updatePaginationUI();
    renderTable();
  } catch (err) {
    notify('獲取訂單失敗: ' + err.message, 'error');
    tbody.innerHTML = `<tr><td colspan="8" style="text-align:center;color:var(--danger)">載入失敗</td></tr>`;
  }
}

function updatePaginationUI() {
  document.getElementById('omPageInfo').textContent = `Page ${currentPage} / ${totalPages}`;
  document.getElementById('omPrevPage').disabled = currentPage <= 1;
  document.getElementById('omNextPage').disabled = currentPage >= totalPages;
}

function renderTable() {
  const tbody = document.getElementById('omTableBody');
  if (!ordersCache.length) {
    tbody.innerHTML = `<tr><td colspan="8" style="text-align:center;color:var(--muted)">無訂單記錄</td></tr>`;
    return;
  }
  
  tbody.innerHTML = ordersCache.map(o => `
    <tr onclick="window.omOpenDetail('${o.order_id}')">
      <td style="font-family:var(--font-mono)">${escapeHtml(o.order_id.substring(0,8))}...</td>
      <td style="font-weight:600">${escapeHtml(o.symbol)}</td>
      <td class="${o.side === 'buy' ? 'text-up' : 'text-down'}">${escapeHtml(o.side.toUpperCase())}</td>
      <td>${o.quantity}</td>
      <td>${o.price || 'Market'}</td>
      <td>${o.fill_price || '-'}</td>
      <td><span class="om-badge ${escapeHtml(o.status)}">${escapeHtml(o.status)}</span></td>
      <td>${formatDate(o.created_at)}</td>
    </tr>
  `).join('');
}

// Attach to window so onclick works
window.omOpenDetail = async function(orderId) {
  const modal = document.getElementById('omDetailModal');
  const body = document.getElementById('omModalBody');
  
  modal.classList.add('active');
  body.innerHTML = '<div style="text-align:center;padding:20px;">載入中...</div>';
  
  try {
    const data = await getJSON(`/api/dashboard/orders/${orderId}`);
    if (!data || !data.order) throw new Error('訂單不存在');
    
    const o = data.order;
    const events = data.events || [];
    
    let html = `
      <div class="om-detail-grid">
        <div class="om-detail-item">
          <span class="om-detail-label">Order ID</span>
          <span class="om-detail-val">${escapeHtml(o.order_id)}</span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Symbol</span>
          <span class="om-detail-val">${escapeHtml(o.symbol)}</span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Side</span>
          <span class="om-detail-val ${o.side === 'buy' ? 'text-up' : 'text-down'}">${escapeHtml(o.side.toUpperCase())}</span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Status</span>
          <span class="om-detail-val"><span class="om-badge ${escapeHtml(o.status)}">${escapeHtml(o.status)}</span></span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Quantity</span>
          <span class="om-detail-val">${o.quantity}</span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Price</span>
          <span class="om-detail-val">${o.price || 'Market'}</span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Fill Price</span>
          <span class="om-detail-val">${o.fill_price || '-'}</span>
        </div>
        <div class="om-detail-item">
          <span class="om-detail-label">Broker Mode</span>
          <span class="om-detail-val">${escapeHtml(o.broker_mode || '-')}</span>
        </div>
      </div>
      
      <h4>生命週期</h4>
      <div class="om-timeline">
    `;
    
    if (!events.length) {
      // Fallback timeline
      html += `
        <div class="om-timeline-item ${escapeHtml(o.status)}">
          <div class="om-timeline-dot"></div>
          <div class="om-timeline-content">
            <div class="om-timeline-title">${escapeHtml(o.status.toUpperCase())}</div>
            <div class="om-timeline-meta">${formatDate(o.updated_at || o.created_at)}</div>
          </div>
        </div>
      `;
    } else {
      html += events.map(e => `
        <div class="om-timeline-item ${escapeHtml(e.status)}">
          <div class="om-timeline-dot"></div>
          <div class="om-timeline-content">
            <div class="om-timeline-title">${escapeHtml(e.status.toUpperCase())} ${e.fill_price ? '@ ' + e.fill_price : ''}</div>
            <div class="om-timeline-meta">${formatDate(e.timestamp)}</div>
          </div>
        </div>
      `).join('');
    }
    
    html += `</div>`;
    body.innerHTML = html;
    
  } catch (err) {
    body.innerHTML = `<div style="text-align:center;padding:20px;color:var(--danger)">獲取詳細資訊失敗: ${err.message}</div>`;
  }
}

function closeDetailModal() {
  document.getElementById('omDetailModal').classList.remove('active');
}

function setupSSE() {
  if (!eventSource) return;
  
  eventSource.addEventListener('order', (e) => {
    try {
      const orderUpdate = JSON.parse(e.data);
      // Determine if we need to refresh the table
      // To be efficient, we can just re-fetch if we are on page 1
      // or if the updated order is currently in our view.
      if (currentPage === 1) {
        fetchOrders();
      } else {
        const idx = ordersCache.findIndex(o => o.order_id === orderUpdate.order_id);
        if (idx !== -1) {
          fetchOrders();
        }
      }
    } catch(err) {
      console.error('SSE order parse error:', err);
    }
  });
}
