// evolution_panel shell — loaded by main.js SHELL_LOADERS
// The page container #page-evolution_panel is pre-rendered in index.html.
// This shell provides the dynamic content area and wires event listeners.

export const template = `
  <div class="panel wide mb-12">
    <div class="evo-views-row" id="evolutionViews">
      <span class="evo-views-label">視覺圖：</span>
      <button class="view-btn active" id="evView-compact">精簡</button>
      <button class="view-btn" id="evView-full">完整</button>
    </div>
  </div>
  <div id="evolutionCatContent" style="display:none;margin-top:12px"></div>
  <div id="evolutionContent" class="empty loading">載入策略演化資料…</div>
`;

export async function init() {
  // Event listeners for evolution_panel are registered in event-listeners.js
  // Data loading is handled by loadPageData('evolution_panel') in main.js
}
