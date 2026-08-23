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
        <div class="waitlist-form" id="waitlistForm" hidden>
          <p class="waitlist-note">Premium 即將推出。留下您的 Email，開放時優先通知您。</p>
          <div class="waitlist-row">
            <input type="email" id="waitlistEmail" placeholder="you@example.com" autocomplete="email" />
            <button class="btn btn-primary" id="waitlistSubmit">通知我</button>
          </div>
          <p class="waitlist-msg" id="waitlistMsg" role="status"></p>
        </div>
      </div>
    </div>
    <div class="premium-mcp panel">
      <h3>MCP 外部 AI 整合</h3>
      <p>Premium 用戶可將 atlas-mcp 接入 Claude Desktop、OpenClaw、OpenCode 等外部 AI，直接透過自然語言查詢市場狀態、策略訊號與風險評估。</p>
      <p>升級後您的 MCP token 將自動啟用全部 80+ 工具權限，詳見 <a href="https://member.goluck.uk/mcp" target="_blank" rel="noopener">MCP 整合指南</a>。</p>
    </div>
  </div>
`;

async function submitWaitlist() {
  var input = document.getElementById('waitlistEmail');
  var msg = document.getElementById('waitlistMsg');
  var submit = document.getElementById('waitlistSubmit');
  if (!input || !msg || !submit) return;
  var email = (input.value || '').trim();
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    msg.textContent = '請輸入有效的 Email 格式。';
    msg.className = 'waitlist-msg waitlist-error';
    input.focus();
    return;
  }
  submit.disabled = true;
  try {
    const { postJSON } = await import('../shared/app-utils.js');
    await postJSON('/api/waitlist', { email: email, source: 'premium' });
    msg.textContent = '已登記！Premium 開放時將優先通知您。';
    msg.className = 'waitlist-msg waitlist-ok';
    input.disabled = true;
  } catch (e) {
    msg.textContent = '登記失敗，請稍後再試。';
    msg.className = 'waitlist-msg waitlist-error';
    submit.disabled = false;
  }
}

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
    // C05: 金流未上就緒前，升級鈕改為「即將推出 + 留 Email」等候名單。
    btn.addEventListener('click', function() {
      var form = document.getElementById('waitlistForm');
      if (form) {
        form.hidden = false;
        btn.hidden = true;
        var input = document.getElementById('waitlistEmail');
        if (input) input.focus();
      }
    });
    var submit = document.getElementById('waitlistSubmit');
    if (submit) submit.addEventListener('click', submitWaitlist);
    var emailInput = document.getElementById('waitlistEmail');
    if (emailInput) {
      emailInput.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') submitWaitlist();
      });
    }
  }
}
