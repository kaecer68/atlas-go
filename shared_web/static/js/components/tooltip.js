import { escapeHtml } from '../shared/app-utils.js';

/**
 * Lightweight tooltip helper for inline term explanations.
 */
export class Tooltip {
  constructor(term, explanation) {
    this.term = term;
    this.explanation = explanation;
  }

  render() {
    const id = 'tt-' + Math.random().toString(36).slice(2, 9);
    return `
      <span class="tooltip" aria-describedby="${id}">
        <span class="tooltip__term">${escapeHtml(this.term)}</span>
        <span class="tooltip__bubble" id="${id}" role="tooltip">${escapeHtml(this.explanation)}</span>
      </span>
    `;
  }
}

export function renderTooltip(term, explanation) {
  return new Tooltip(term, explanation).render();
}

const TOOLTIP_GLOSSARY = {
  'Sharpe': '衡量每承擔一單位風險能獲得的超額報酬，數值越高代表風險調整後報酬越好。',
  'Hit Rate': '策略建議正確的比例，例如 60% 代表 10 次建議中有 6 次方向正確。',
  '最大回撤': '投資組合從高點到低點的最大跌幅，數值越大代表曾經歷的虧損越深。',
  'HHI': '赫芬達爾指數，衡量持倉集中度；數值越高表示資金越集中在少數標的上。',
  'Regime': '市場狀態（如多頭、空頭、震盪），模型據此調整策略權重。',
};

export function glossaryTooltip(term) {
  const explanation = TOOLTIP_GLOSSARY[term];
  return explanation ? renderTooltip(term, explanation) : escapeHtml(term);
}
