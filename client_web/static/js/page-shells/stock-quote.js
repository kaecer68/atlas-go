// stock-quote shell — loaded by main.js SHELL_LOADERS
// The page container #page-stock-quote is pre-rendered in index.html.
// Data loading handled by loadPageData('stock-quote') in main.js.

export const template = `
  <div id="sq-root" class="stock-quote-root">
    <div class="sq-header">
      <h2>個股快查</h2>
      <div id="sq-symbol-display"></div>
    </div>
    <div class="sq-search-bar">
      <input type="text" id="sq-symbol-input" placeholder="輸入股票代碼，例如：2330" />
      <button id="sq-search-btn">查詢</button>
    </div>
    <div class="stock-quote-grid">
      <div class="sq-section sq-fundamentals">
        <h3>基本面</h3>
        <div class="sq-placeholder">載入基本面資料…</div>
      </div>
      <div class="sq-section sq-chips">
        <h3>籌碼</h3>
        <div class="sq-placeholder">載入籌碼資料…</div>
      </div>
      <div class="sq-section sq-technical">
        <h3>技術指標</h3>
        <div class="sq-placeholder">載入技術指標…</div>
      </div>
      <div class="sq-section sq-revenue">
        <h3>月營收</h3>
        <div class="sq-placeholder">載入月營收資料…</div>
      </div>
      <div class="sq-section sq-winrate">
        <h3>個股勝率</h3>
        <div class="sq-placeholder">載入勝率資料…</div>
      </div>
      <div class="sq-section sq-divergence">
        <h3>量價背離</h3>
        <div class="sq-placeholder">載入量價背離資料…</div>
      </div>
    </div>
  </div>
`;

export async function init() {
  const input = document.getElementById('sq-symbol-input');
  const btn = document.getElementById('sq-search-btn');
  if (input && btn) {
    btn.addEventListener('click', () => {
      const sym = input.value.trim();
      if (sym) window.switchPage('stock-quote');
      // Update URL for deep-link
      history.replaceState({}, '', '?symbol=' + encodeURIComponent(sym));
    });
    input.addEventListener('keydown', e => { if (e.key === 'Enter') btn.click(); });
  }
  // Read symbol from URL param on init
  const params = new URLSearchParams(window.location.search);
  const sym = params.get('symbol');
  if (sym) {
    if (input) input.value = sym;
    // Trigger data load for the symbol
    const event = new CustomEvent('sq-load-symbol', { detail: { symbol: sym } });
    document.dispatchEvent(event);
  }
}
