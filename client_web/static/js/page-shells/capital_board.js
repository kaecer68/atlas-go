// capital_board shell — loaded by main.js SHELL_LOADERS
// The page container #page-capital_board is pre-rendered in index.html.
// Data loading handled by loadPageData('capital_board') in main.js.

export const template = `
  <div id="cb-root" class="capital-board-root">
    <div id="cb-summary" class="cb-summary">
      <h2>板塊方向彙總</h2>
      <div id="cb-summary-content">載入中…</div>
    </div>
    <div id="cb-grid" class="cb-grid">
      <div id="cb-grid-content">載入中…</div>
    </div>
  </div>
`;

export async function init() {
  // Data loading handled by loadPageData('capital_board') in main.js
}
