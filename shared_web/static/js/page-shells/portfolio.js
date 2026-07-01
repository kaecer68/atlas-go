export const template = `
  <h1 id="portfolioPageTitle">📂 組合持倉</h1>
  <div class="kpi-grid" id="portfolioKPIs"></div>
  <div class="panel wide hidden" id="equityCurvePanel">
    <h2>淨值趨勢</h2>
    <canvas id="equityChart" height="200"></canvas>
  </div>
  <div class="panel wide" id="positionsPanel">
    <h2>持倉明細</h2>
    <div id="positionsTable"></div>
  </div>
  <div class="panel wide" id="tradeHistoryPanel">
    <h2>交易時間軸</h2>
    <div id="tradeHistoryContainer"></div>
  </div>
  <div class="panel wide" id="pnlAttributionPanel">
    <h2>損益歸因 (PnL Attribution)</h2>
    <div id="pnlAttribution" class="empty loading">載入中…</div>
  </div>
  <div class="panel wide"><h2>基準比較</h2><div id="benchmarkComparison" class="loading">載入中…</div></div>
  <div class="panel wide"><h2>風控閘道</h2><div id="riskGatePanel" class="loading">載入中…</div></div>
  <div class="panel wide"><h2>風險分析</h2><div id="riskPanel" class="loading">載入中…</div></div>
  <details class="help-details">
    <summary><strong>📖 如何解讀本頁</strong></summary>
    <p><strong>投組稅前淨值</strong> = 可用現金 + 所有持倉市值（未扣除稅負）</p>
    <p><strong>投組稅後淨值</strong> = 稅前淨值 - 累積稅負（已實現獲利所產生的稅負）</p>
    <p><strong>淨值趨勢</strong> = 每日回測的投組總值變化曲線（含稅前與稅後）</p>
    <p>損益為未實現損益，實際損益需待部位平倉後確認。</p>
    <p class="text-muted text-sm">資料來源：live state store（data/state/live/）。若無持倉資料，代表尚未執行模擬或所有部位已平倉。</p>
  </details>
`;
