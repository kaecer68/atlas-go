import { fetchStockBundle, fetchStockBundleWithCoverage, fetchStockMonthlyRevenue } from '../services/stock-api-client.js';
import { renderMissingState } from '../shared/app-utils.js';
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

  pageWrapper.appendChild(searchSection);
  pageWrapper.appendChild(_contentWrapper);
  pageWrapper.appendChild(disclaimerWrapper);

  _container.appendChild(pageWrapper);
}

export async function renderPage(container) {
  _container = container;

  initPageStructure();

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
