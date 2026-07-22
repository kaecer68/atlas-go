// evolution_panel shell — loaded by main.js SHELL_LOADERS
// The page container #page-evolution_panel is pre-rendered in index.html.
// This shell provides the dynamic content area and wires event listeners.

export const template = `
  <div id="ev-root" class="evolution-root">
    <div id="ev-toolbar" class="ev-toolbar">
      <div class="ev-title-row">
        <h2>策略演化</h2>
        <div class="ev-view-toggle">
          <button id="evView-compact" class="ev-view-btn active">精簡</button>
          <button id="evView-full" class="ev-view-btn">完整</button>
        </div>
      </div>
    </div>
    <div id="ev-content" class="ev-content">
      <div id="ev-placeholder" class="ev-loading">載入策略演化資料…</div>
    </div>
  </div>
`;

export async function init() {
  // Event listeners for evolution_panel are registered in event-listeners.js
  // Data loading is handled by loadPageData('evolution_panel') in main.js
}
