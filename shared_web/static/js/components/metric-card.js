/**
 * MetricCard component for editorial dashboard.
 * Displays a labelled metric with optional delta, tooltip, and trend colour.
 */

export class MetricCard {
  /**
   * @param {Object} opts
   * @param {string} opts.label
   * @param {string} opts.value
   * @param {string} [opts.delta]
   * @param {string} [opts.tone] 'positive' | 'negative' | 'neutral'
   * @param {string} [opts.tooltip]
   * @param {string} [opts.href]
   */
  constructor(opts) {
    this.opts = opts || {};
  }

  render() {
    const { label, value, delta, tone = 'neutral', tooltip, href } = this.opts;
    const toneClass = tone === 'positive' ? 'metric-card__value--positive'
      : tone === 'negative' ? 'metric-card__value--negative'
      : '';
    const deltaHtml = delta ? `<span class="metric-card__delta metric-card__delta--${tone}">${delta}</span>` : '';
    const tooltipHtml = tooltip ? `<span class="metric-card__tooltip" data-tooltip="${escapeHtml(tooltip)}">?</span>` : '';
    const tag = href ? 'a' : 'div';
    const hrefAttr = href ? ` href="${escapeHtml(href)}"` : '';

    return `<${tag} class="metric-card"${hrefAttr}>
      <div class="metric-card__header">
        <span class="metric-card__label">${escapeHtml(label)}${tooltipHtml}</span>
      </div>
      <div class="metric-card__value ${toneClass}">${value != null ? escapeHtml(String(value)) : '--'}</div>
      ${deltaHtml}
    </${tag}>`;
  }
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

export function metricCard(opts) {
  return new MetricCard(opts).render();
}
