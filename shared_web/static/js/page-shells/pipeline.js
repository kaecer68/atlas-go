export const template = `
  <div class="workflow mb-sm">
    <div class="step">AI 推薦</div><span class="arrow">→</span>
    <div class="step clickable" id="workflowScreening" title="點擊顯示/隱藏被篩選層排除的標的">篩選層</div><span class="arrow">→</span>
    <div class="step">控制層</div><span class="arrow">→</span>
    <div class="step">模擬投組</div>
  </div>
  <button class="filter-toggle" id="filterToggle" title="點擊展開/收合進階篩選">
    <span class="icon">⚙</span> 進階篩選
    <span id="filterBadge"></span>
  </button>
  <div class="filter-panel" id="filterPanel">
    <div class="filter-row">
      <div class="filter-group">
        <label>P/E 比（最小～最大）</label>
        <div class="flex-gap-6">
          <input type="number" id="peMin" placeholder="0" min="0" step="0.1" aria-label="P/E 最小值">
          <span class="text-xs-muted">～</span>
          <input type="number" id="peMax" placeholder="無上限" min="0" step="0.1" aria-label="P/E 最大值">
        </div>
      </div>
      <div class="filter-group">
        <label>P/B 比（最小～最大）</label>
        <div class="flex-gap-6">
          <input type="number" id="pbMin" placeholder="0" min="0" step="0.1" aria-label="P/B 最小值">
          <span class="text-xs-muted">～</span>
          <input type="number" id="pbMax" placeholder="無上限" min="0" step="0.1" aria-label="P/B 最大值">
        </div>
      </div>
      <div class="filter-group">
        <label>股息率（最小～最大）%</label>
        <div class="flex-gap-6">
          <input type="number" id="dyMin" placeholder="0" min="0" step="0.1" aria-label="股息率最小值">
          <span class="text-xs-muted">～</span>
          <input type="number" id="dyMax" placeholder="無上限" min="0" step="0.1" aria-label="股息率最大值">
        </div>
      </div>
      <div class="filter-group">
        <label>報酬率範圍 %</label>
        <div class="flex-gap-6">
          <input type="number" id="retMin" placeholder="-100" step="0.1" aria-label="報酬率最小值">
          <span class="text-xs-muted">～</span>
          <input type="number" id="retMax" placeholder="100" step="0.1" aria-label="報酬率最大值">
        </div>
      </div>
    </div>
    <div class="filter-actions">
      <button class="primary">套用篩選</button>
      <button>清除條件</button>
      <span id="filterResultCount" class="ml-auto text-xs-muted"></span>
    </div>
  </div>
  <details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
    以下為最新回測場次中，<strong>控制層已放行的推薦標的</strong>。每筆資料包含策略來源、方向、收盤價與隔日回測報酬。價量標籤與推薦理由供您快速評估是否放行。
    若勾選「顯示全部被過濾項目」，<span class="text-up">紅色邊框列</span>為被控制層擋下的標的，您可點擊「補追」進行人工覆寫。
    <br><span class="text-muted">註：收盤價為回測當日收盤，作為模擬進場的參考基準。目標價與停損價由 Agent 推薦產生，並在 Simulator 中優先於固定百分比止盈止損。</span>
  </details>
  <div class="panel wide"><h2>最新場次推薦明細</h2><div id="recommendationPipeline" class="empty loading">載入中…</div></div>
`;
