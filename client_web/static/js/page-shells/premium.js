export const template = `
  <div class="premium-page">
    <div class="premium-hero">
      <h2>升級 Premium</h2>
      <p class="premium-subtitle">解鎖完整策略訊號、深度回測報告、MCP 全部工具權限</p>
    </div>
    <div class="premium-tiers">
      <div class="tier-card panel">
        <h3>免費</h3>
        <div class="tier-price">$0</div>
        <ul class="tier-features">
          <li>大盤狀態燈號（Risk-On/Off）</li>
          <li>資金流向摘要</li>
          <li>當日事件提醒</li>
          <li class="tier-disabled">策略排名</li>
          <li class="tier-disabled">個股事件預警</li>
          <li class="tier-disabled">完整策略訊號</li>
          <li class="tier-disabled">MCP 部分工具</li>
        </ul>
        <div class="tier-current">目前方案</div>
      </div>
      <div class="tier-card panel tier-premium">
        <div class="tier-badge">推薦</div>
        <h3>Premium</h3>
        <div class="tier-price">$299 <small>/月</small></div>
        <ul class="tier-features">
          <li>完整策略訊號（含進出場點位）</li>
          <li>深度回測報告</li>
          <li>產業資金流向分析</li>
          <li>個股事件預警</li>
          <li>MCP 全部 80+ tools 權限</li>
          <li>每日市場報告（email）</li>
          <li>優先客服支援</li>
        </ul>
        <button class="btn btn-primary btn-full" id="upgradeBtn">立即升級</button>
      </div>
    </div>
    <div class="premium-mcp panel">
      <h3>MCP 外部 AI 整合</h3>
      <p>Premium 用戶可將 atlas-mcp 接入 Claude Desktop、OpenClaw、OpenCode 等外部 AI，直接透過自然語言查詢市場狀態、策略訊號與風險評估。</p>
      <p>升級後您的 MCP token 將自動啟用全部 80+ 工具權限，詳見 <a href="/client/mcp" data-page="mcp" onclick="event.preventDefault();window.switchPage('mcp')">MCP 整合指南</a>。</p>
    </div>
  </div>
`;

export async function init() {
  const { getTier } = await import('../services/auth.js');
  const tier = await getTier();
  const btn = document.getElementById('upgradeBtn');
  if (tier === 'premium') {
    const cards = document.querySelectorAll('.tier-current');
    cards.forEach(function(c) {
      c.textContent = '已訂閱';
      c.classList.add('tier-active');
    });
    if (btn) {
      btn.textContent = '已啟用 Premium';
      btn.disabled = true;
    }
  } else if (btn) {
    btn.addEventListener('click', function() {
      alert('Premium 升級功能開發中，敬請期待！');
    });
  }
}
