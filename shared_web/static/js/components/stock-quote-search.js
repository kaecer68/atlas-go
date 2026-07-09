import { escapeHtml } from '../shared/app-utils.js';

export function renderSearch(onSearch, initialSymbol = '') {
  const container = document.createElement('div');
  container.className = 'sq-search-container';

  let recentSearches = [];
  try {
    recentSearches = JSON.parse(localStorage.getItem('sq-recent-searches') || '[]');
  } catch (e) {
    recentSearches = [];
  }

  const html = `
    <div class="sq-search-row">
      <input type="text" class="sq-search-input" id="sqSearchInput" placeholder="輸入台股代號 (如: 2330)" value="${escapeHtml(initialSymbol)}" maxlength="6" />
      <button class="sq-search-btn" id="sqSearchBtn">查詢</button>
    </div>
    <div class="sq-search-hot">
      <span class="sq-search-tag" data-symbol="2330">2330 台積電</span>
      <span class="sq-search-tag" data-symbol="2454">2454 聯發科</span>
      <span class="sq-search-tag" data-symbol="2317">2317 鴻海</span>
      <span class="sq-search-tag" data-symbol="0050">0050 台灣50</span>
    </div>
    ${recentSearches.length > 0 ? `
    <div class="sq-search-hot" style="margin-top:8px">
      <span style="font-size:var(--text-sm);color:var(--text-secondary)">最近查詢:</span>
      ${recentSearches.map(s => `<span class="sq-search-tag" data-symbol="${escapeHtml(s)}">${escapeHtml(s)}</span>`).join('')}
    </div>` : ''}
  `;

  container.innerHTML = html;

  const input = container.querySelector('#sqSearchInput');
  const btn = container.querySelector('#sqSearchBtn');

  const doSearch = (symbol) => {
    const clean = symbol.trim();
    if (/^\d{4,6}$/.test(clean)) {
      // Save recent
      let recents = recentSearches.filter(s => s !== clean);
      recents.unshift(clean);
      if (recents.length > 5) recents = recents.slice(0, 5);
      localStorage.setItem('sq-recent-searches', JSON.stringify(recents));
      onSearch(clean);
    } else {
      alert('請輸入 4 到 6 碼的台股代號');
    }
  };

  btn.addEventListener('click', () => doSearch(input.value));
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') doSearch(input.value);
  });

  const tags = container.querySelectorAll('.sq-search-tag');
  tags.forEach(tag => {
    tag.addEventListener('click', () => {
      const sym = tag.getAttribute('data-symbol');
      input.value = sym;
      doSearch(sym);
    });
  });

  let debounceTimer;
  input.addEventListener('input', () => {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      const val = input.value.trim();
      if (/^\d{4,6}$/.test(val)) {
        doSearch(val);
      }
    }, 500);
  });

  return container;
}
