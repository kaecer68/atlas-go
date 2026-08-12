/**
 * pages/retail_sentiment.js — 散戶情緒
 *
 * 獨立選單頁面（PR #15xx）。原屬 narrative 頁的「散戶情緒」panel，
 * 為架構正確性（散戶情緒不屬於 narrative 敘事類別）拆出為獨立頁。
 *
 * 顯示 RSI-tw 綜合散戶情緒指數、融資餘額變化、當沖比率、融券餘額、
 * 歷史百分位與子指標明細（Part A/C/D）。
 *
 * Source: GET /api/dashboard/retail-sentiment
 * Backend package: internal/retail（RSI-tw 計算）
 *
 * 結構：單一 page module（template + init 同檔），與 home.js 一致。
 * 渲染邏輯抽到 shared/retail-sentiment-panel.js 與 narrative.js 共用。
 */

import { silentGetJSON, renderMissingState } from '../shared/app-utils.js';
import { renderRetailSentimentPanel, RETRY_ID_CONST } from '../shared/retail-sentiment-panel.js';

const RETRY_ID = 'retail-sentiment';

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  散戶情緒頁顯示 RSI-tw 綜合散戶情緒指數（Retail Sentiment Index — Taiwan），
  從融資餘額、當沖比率、融券餘額、子指標（Part A/C/D）等 9 個訊號計算
  出散戶市場情緒。點擊任一 KPI 卡可看詳細解讀標準。
</details>
<section id="retailSentimentContent" class="empty loading">載入中…</section>
`;

async function loadRetailSentiment() {
  const root = document.getElementById('retailSentimentContent');
  if (!root) return;
  root.classList.add('loading');
  try {
    const rs = await silentGetJSON('/api/dashboard/retail-sentiment', { timeoutMs: 20000, retry: 0 });
    if (rs === null) {
      root.classList.remove('loading');
      root.innerHTML = renderMissingState('散戶情緒指標', 'api-error');
      root.querySelector('.retry-btn')?.addEventListener('click', loadRetailSentiment);
      return;
    }
    renderRetailSentimentPanel(root, rs);
  } catch (err) {
    console.error('[retail-sentiment] load failed', err);
    root.classList.remove('loading');
    root.innerHTML = renderMissingState('散戶情緒指標', 'api-error');
    root.querySelector('.retry-btn')?.addEventListener('click', loadRetailSentiment);
  }
}

export async function init() {
  await loadRetailSentiment();
}
