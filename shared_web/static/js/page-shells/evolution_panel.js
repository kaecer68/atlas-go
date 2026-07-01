export const template = `
  <div class="panel wide mb-12">
    <div class="evo-views-row" id="evolutionViews">
      <span class="evo-views-label">視覺圖：</span>
      <button class="view-btn active" id="evView-compact">總覽儀表板</button>
    </div>
  </div>
  <div id="evolutionCatContent" class="hidden mt-12"></div>
  <div id="evolutionContent" class="empty loading">載入中…</div>
  <div class="panel wide hidden mt-12" id="evolutionEquityCurvePanel">
    <h3>權益曲線</h3>
    <canvas id="evolutionEquityChart" class="w-full"></canvas>
  </div>
`;
