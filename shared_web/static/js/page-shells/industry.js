export const template = `
  <details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
    本頁呈現 Atlas 對台灣產業的<strong>多維度分析</strong>，包含產業分類樹、週期定位、供應鏈連動與季節性模式。
    點擊產業卡片可查看詳細的週期羅盤與衝擊傳導分析。
  </details>
  <div class="two-col-grid">
    <div class="panel"><div class="panel-header"><h2>產業熱力圖</h2><div class="panel-header-actions"><span id="sectorLastUpdated" class="last-updated" title="最後更新時間">--</span><button type="button" id="sectorRefreshBtn" class="refresh-btn" title="手動刷新" aria-label="手動刷新">↻</button></div></div><div id="industryMap" class="empty loading">載入中…</div></div>
    <div class="panel"><h2>週期羅盤 <span class="cursor-pointer text-accent" data-open-modal="cycleLegend" title="週期燈號計算說明">ℹ️</span></h2><div id="industryCycle" class="empty loading">載入中…</div></div>
    <div class="panel"><h2>供應鏈連動</h2><div id="industryLinkage" class="empty loading">載入中…</div></div>
    <div class="panel"><h2>季節性模式</h2><div id="industrySeasonality" class="empty loading">載入中…</div></div>
  </div>
  <div class="panel wide mt-16">
    <h2>供應鏈網路圖</h2>
    <div id="industryGraph" class="empty loading graph-stub">載入中…</div>
  </div>
  <div class="panel mt-16"><h2>衝擊模擬</h2>
    <div class="shock-form-row">
      <select id="shockSource" class="shock-select"><option value="">選擇來源產業</option></select>
      <input type="number" id="shockMagnitude" placeholder="衝擊幅度 (例: -0.05)" step="0.01" class="shock-input shock-input--wide">
      <input type="number" id="shockDepth" value="3" min="1" max="5" class="shock-input shock-input--narrow" title="傳導深度">
      <button id="btnRunShockSim" class="shock-button">▶ 執行模擬</button>
    </div>
    <div id="industryShockSim" class="empty">選擇產業與衝擊幅度進行供應鏈衝擊傳導模擬</div>
  </div>
`;
