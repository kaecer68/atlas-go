// Decision-chain shell — wraps the shared decision-chain page module
// (shared_web/static/js/pages/decision-chain.js) so the home page
// 「查看決策鏈」 button (switchPage('decision-chain')) has a real target.
import { loadDecisionChain } from '../pages/decision-chain.js';

export const template = `
  <div class="panel">
    <h2>決策鏈</h2>
    <p class="text-muted">從事件雷達 → 規則觸發 → 產業 → 個股 → 出場的完整因果追蹤。</p>
    <div id="decisionChain"></div>
  </div>
`;

export async function init() {
  await loadDecisionChain();
}
