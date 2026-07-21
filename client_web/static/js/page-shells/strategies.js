// strategies shell — loaded by main.js SHELL_LOADERS
// The page container #page-strategies is pre-rendered in index.html.

export const template = `
  <div id="strategies-root" class="strategies-root">
    <div id="strategies-header" class="strategies-header">
      <h2>投資心法</h2>
    </div>
    <div id="strategies-content" class="strategies-content">
      <div id="strategies-placeholder" class="strategies-loading">載入投資心法…</div>
    </div>
  </div>
`;

export async function init() {
  // Data loading handled by loadPageData('strategies') in main.js
}
