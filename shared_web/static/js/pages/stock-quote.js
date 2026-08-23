import { fetchStockBundle, fetchStockBundleWithCoverage, fetchStockMonthlyRevenue } from '../services/stock-api-client.js';
import { renderMissingState } from '../shared/app-utils.js';
import { fetchDecisionChain, renderRecommendations, renderExitAlerts, emptyState } from '../components/decision-panels.js';
import { renderSearch } from '../components/stock-quote-search.js';
import { renderHeader } from '../components/stock-quote-header.js';
import { renderFundamentals } from '../components/stock-quote-fundamentals.js';
import { renderChips } from '../components/stock-quote-chips.js';
import { renderTechnical } from '../components/stock-quote-technical.js';
import { renderRevenue } from '../components/stock-quote-revenue.js';

let state = {
  currentSymbol: null,
  status: 'idle', // idle | loading | loaded | error
  results: null
};

let _container = null;
let _searchInput = null;
let _contentWrapper = null;

function updateSearchInput(symbol) {
  if (_searchInput) {
    _searchInput.value = symbol || '';
  }
}

function renderContent() {
  if (!_contentWrapper) return;

  let contentHtml = '';
  if (state.status === 'idle') {
    contentHtml = `
      <div class="sq-empty-state">
        <div class="sq-empty-state__title">探索台股市場</div>
        <p class="sq-empty-state__desc">輸入股票代號，查看即時報價、基本面、籌碼與技術指標。</p>
        <div class="sq-empty-state__section">
          <span class="sq-empty-state__label">熱門搜尋：</span>
          <span class="sq-search-tag" data-symbol="2330">2330 台積電</span>
          <span class="sq-search-tag" data-symbol="2454">2454 聯發科</span>
          <span class="sq-search-tag" data-symbol="2317">2317 鴻海</span>
          <span class="sq-search-tag" data-symbol="0050">0050 台灣50</span>
        </div>
      </div>
    `;
  } else if (state.status === 'error') {
    contentHtml = renderMissingState('股票報價', 'api-error');
  } else {
    // loading or loaded
    const res = state.results || {};
    contentHtml = `
      ${renderHeader(state.status, res.quote, res.chips, res.coverage)}
      <div class="stock-quote-grid">
        ${renderFundamentals(state.status, res.fundamentals, res.coverage)}
        ${renderChips(state.status, res.chips, res.coverage)}
        ${renderTechnical(state.status, res.technical, res.coverage)}
        ${renderRevenue(state.status, res.monthlyRevenue)}
      </div>
    `;
  }

  _contentWrapper.innerHTML = contentHtml;

  // Bind hot tags inside empty state
  _contentWrapper.querySelectorAll('.sq-empty-state .sq-search-tag').forEach(tag => {
    tag.addEventListener('click', () => {
      const sym = tag.getAttribute('data-symbol');
      if (_searchInput) _searchInput.value = sym;
      doSearch(sym);
    });
  });
}

async function doSearch(symbol) {
  state.currentSymbol = symbol;
  state.status = 'loading';
  state.results = null;
  updateSearchInput(symbol);
  try {
    const results = await fetchStockBundleWithCoverage(symbol);
    state.status = 'loaded';
    state.results = results;
  } catch (e) {
    state.status = 'error';
    state.results = null;
  }

  // Monthly revenue is fetched independently (NOT part of the 4-endpoint
  // bundle): it hits the FinMind-backed /api/stock/monthly_revenue endpoint
  // which has a quota-aware 503 path, and its 7-day TTL differs from the
  // bundle's per-endpoint TTLs. The section degrades gracefully to a
  // "暫時無法取得" box on 503 so the page doesn't look broken.
  // Only runs when the bundle succeeded — on bundle failure the whole
  // page is in error state and the revenue section isn't rendered anyway.
  if (state.status === 'loaded' && state.results) {
    const revenueState = {
      status: 'loading',
      data: null,
      error: null
    };
    state.results.monthlyRevenue = revenueState;
    renderContent();
    try {
      const revenue = await fetchStockMonthlyRevenue(symbol);
      revenueState.status = 'loaded';
      revenueState.data = revenue;
    } catch (e) {
      revenueState.status = 'error';
      revenueState.error = e.message;
    }
    renderContent();
  }

  // Update URL if supported
  if (window.history && window.history.pushState) {
    const newUrl = window.location.pathname + '?symbol=' + encodeURIComponent(symbol);
    window.history.pushState({ path: newUrl }, '', newUrl);
  }

  updateSearchInput(symbol);
  renderContent();
}

function initPageStructure() {
  if (!_container) return;

  const searchSection = renderSearch(doSearch, state.currentSymbol || '');
  _searchInput = searchSection.querySelector('#sqSearchInput');

  _contentWrapper = document.createElement('div');
  _contentWrapper.className = 'stock-quote-content';

  const disclaimerWrapper = document.createElement('div');
  disclaimerWrapper.innerHTML = `
    <div class="sq-disclaimer">
      本系統資料僅供研究參考，不構成投資建議。投資決策應自行評估風險，並諮詢專業顧問。
    </div>
  `;

  _container.innerHTML = '';
  const pageWrapper = document.createElement('div');
  pageWrapper.className = 'stock-quote-page';

  // 決策面板（推薦標的 + 出場提醒）— 資料來自共用聚合端點
  // /api/dashboard/decision-chain；填補查詢區下方空白。
  const decisionPanels = document.createElement('div');
  decisionPanels.className = 'sq-decision-panels';
  decisionPanels.innerHTML = `
    <section class="sq-section sq-recommendations">
      <h3>推薦標的</h3>
      <div id="sq-recommendations" class="sq-placeholder">載入推薦標的…</div>
    </section>
    <section class="sq-section sq-exit-alerts">
      <h3>出場提醒</h3>
      <div id="sq-exit-alerts" class="sq-placeholder">載入出場提醒…</div>
    </section>
  `;

  pageWrapper.appendChild(searchSection);
  pageWrapper.appendChild(_contentWrapper);
  pageWrapper.appendChild(decisionPanels);
  pageWrapper.appendChild(disclaimerWrapper);

  _container.appendChild(pageWrapper);
}

// 空態引導（未輸入代號 / 尚無資料時，不只用「暫無」打發訪客，
// 而是給出下一步 CTA：推薦標的 → 登入查看；出場提醒 → 建立持倉）。
function renderGuidedEmpty(kind) {
  if (kind === 'recommendations') {
    return `
      <div class="dc-empty" style="padding:20px;color:var(--muted);text-align:center">
        <div class="empty-state-guidance">
          <div class="icon">📌</div>
          <div class="title">還沒有推薦標的</div>
          <div class="desc">登入後即可查看依目前市場時期生成的個人化推薦標的。</div>
          <div class="empty-actions">
            <a class="btn btn--primary btn-sm" href="https://member.goluck.uk/login">登入查看</a>
          </div>
        </div>
      </div>`;
  }
  return `
    <div class="dc-empty" style="padding:20px;color:var(--muted);text-align:center">
      <div class="empty-state-guidance">
        <div class="icon">🔔</div>
        <div class="title">還沒有出場提醒</div>
        <div class="desc">建立持倉後，這裡會顯示需要留意的出場提醒。</div>
        <div class="empty-actions">
          <a class="btn btn--primary btn-sm" data-page="strategies" href="/client/strategies">前往投資心法建立持倉</a>
        </div>
      </div>
    </div>`;
}

// 推薦標的 + 出場提醒：獨立於個股查詢流程載入（市場層級資料，不隨查詢重繪）。
async function loadDecisionPanels() {
  const recEl = document.getElementById('sq-recommendations');
  const exitEl = document.getElementById('sq-exit-alerts');
  if (!recEl || !exitEl) return;
  try {
    const data = await fetchDecisionChain();
    const recs = data && data.recommendations;
    const alerts = data && data.exit_alerts;
    recEl.innerHTML = recs && recs.length ? renderRecommendations(data) : renderGuidedEmpty('recommendations');
    exitEl.innerHTML = alerts && alerts.length ? renderExitAlerts(data) : renderGuidedEmpty('exit-alerts');
  } catch (e) {
    console.warn('[stock-quote] decision panels load failed:', e);
    recEl.innerHTML = emptyState('推薦標的載入失敗');
    exitEl.innerHTML = emptyState('出場提醒載入失敗');
  }
}

export async function renderPage(container) {
  _container = container;

  initPageStructure();
  loadDecisionPanels();

  // Check URL params
  const urlParams = new URLSearchParams(window.location.search);
  const symbol = urlParams.get('symbol');

  if (symbol && /^\d{4,6}$/.test(symbol)) {
    state.currentSymbol = symbol;
    updateSearchInput(symbol);
    await doSearch(symbol);
  } else {
    state.status = 'idle';
    renderContent();
  }
}
