export const template = `
  <div class="mcp-page">
    <div class="panel">
      <h2>MCP 外部 AI 整合</h2>
      <p class="mcp-intro">atlas-mcp 提供 80+ 工具讓外部 AI 直接查詢市場狀態、策略訊號與風險評估。</p>
    </div>

    <div class="mcp-setup panel">
      <h3>Claude Desktop</h3>
      <pre class="mcp-code">編輯 <code>claude_desktop_config.json</code>：

{
  "mcpServers": {
    "atlas-mcp": {
      "command": "/path/to/atlas-mcp",
      "env": {
        "ATLAS_WORK_DIR": "/path/to/atlas-go",
        "ATLAS_DATABASE_URL": "postgres://...",
        "ATLAS_API_TOKEN": "your-token"
      }
    }
  }
}</pre>
    </div>

    <div class="mcp-setup panel">
      <h3>OpenClaw / Hermes</h3>
      <pre class="mcp-code">編輯 <code>~/.openclaw/mcp.json</code>：

{
  "atlas-mcp": {
    "type": "stdio",
    "command": "/path/to/atlas-mcp",
    "env": { "ATLAS_API_TOKEN": "your-token" }
  }
}</pre>
    </div>

    <div class="mcp-setup panel">
      <h3>OpenCode CLI</h3>
      <pre class="mcp-code">在 <code>.opencode/opencode.json</code> 的 mcpServers 區塊新增：

{
  "name": "atlas-mcp",
  "command": "/path/to/atlas-mcp"
}</pre>
    </div>

    <div class="mcp-tools panel">
      <h3>常用工具</h3>
      <div class="mcp-tool-grid">
        <div class="mcp-tool-card">
          <h4>市場總覽</h4>
          <code>mcp_quickstart</code>
          <p>一站式開機摘要：macro、策略、風險、事件</p>
        </div>
        <div class="mcp-tool-card">
          <h4>資金流向</h4>
          <code>capital_flow_daily</code>
          <p>七大勢力 Z-score 分解 + 共振強度</p>
        </div>
        <div class="mcp-tool-card">
          <h4>事件預測</h4>
          <code>event_flow_prediction</code>
          <p>未來 5 天事件驅動資金流預測</p>
        </div>
        <div class="mcp-tool-card">
          <h4>策略排名</h4>
          <code>strategy_ranker</code>
          <p>5 策略歷史績效排名</p>
        </div>
      </div>
    </div>

    <div class="mcp-prompts panel">
      <h3>預設 Prompt 模板</h3>
      <ul class="mcp-prompt-list">
        <li><code>taiwan_quick_look</code> — 台股今日快覽</li>
        <li><code>strategy_advice</code> — 策略建議</li>
        <li><code>stock_health_check</code> — 持股健檢（輸入股票代號）</li>
        <li><code>daily_market_briefing</code> — 每日市場簡報</li>
        <li><code>risk_check</code> — 投資組合風險</li>
        <li><code>regime_interpretation</code> — 盤勢解讀</li>
      </ul>
    </div>
  </div>
`;

export async function init() {
  // Read-only page — no dynamic data needed at load time.
}
