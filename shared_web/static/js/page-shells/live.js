export const template = `
  <details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
    本頁呈現<strong>風控層（風控長／投資長）對 AI 推薦的最終處置結果</strong>。它告訴你：在市場當前體制下，有多少推薦被放行、有多少被推回、以及系統是否觸發熔斷。
    若風控長阻擋率高，請檢查【宏觀敘事】的外資出逃指數是否飆升（紅燈）；若投資長過濾數量多，請檢查【投資管線】是否有板塊擁擠或風格衝突。
  </details>
  <div class="panel wide mb-sm panel-padded-sm"><h2 class="mb-4">總經敘事脈絡</h2><div id="liveNarrativeStrip" class="empty loading p-4-v">載入中…</div></div>
  <div class="live-grid">
    <div class="panel panel--compact"><h2>總經雷達</h2><div id="macroRadar" class="empty loading">載入中…</div></div>
    <div class="panel panel--compact"><h2>即時狀態</h2><div id="liveStatus" class="empty loading">載入中…</div></div>
  </div>
  <div class="panel wide hidden" id="riskCardsPanel">
    <h2>風險指標</h2>
    <div id="riskCards" class="kpi-grid"></div>
    <div id="riskPositionConcentration"></div>
    <div id="riskSectorDistribution"></div>
  </div>
  <div class="panel wide hidden" id="riskCalibrationPanel">
    <h2>自主校準</h2>
    <div id="riskCalibration" class="empty loading">載入中…</div>
  </div>
  <div class="panel wide hidden" id="liveRiskCommentaryPanel">
    <h2>🧠 風控長評語（Risk Commentary）</h2>
    <div id="liveRiskCommentary" class="empty loading">載入中…</div>
  </div>
`;
