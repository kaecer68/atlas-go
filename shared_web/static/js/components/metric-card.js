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
    const { label, value, delta, tone = 'neutral', tooltip, href, extraClasses } = this.opts;
    const toneClass = tone === 'positive' ? 'kpi-value--positive'
      : tone === 'negative' ? 'kpi-value--negative'
      : tone === 'warning' ? 'kpi-value--warning'
      : '';
    const classes = ['kpi-card', extraClasses || ''].filter(Boolean).join(' ');
    const deltaHtml = delta ? `<span class="kpi-card__delta kpi-card__delta--${tone}">${delta}</span>` : '';
    const tag = href ? 'a' : 'div';
    const hrefAttr = href ? ` href="${escapeHtml(href)}"` : '';
    const titleAttr = tooltip ? ` title="${escapeHtml(tooltip)}"` : '';

    return `<${tag} class="${escapeHtml(classes)}"${hrefAttr}${titleAttr}>
      <div class="kpi-label">${escapeHtml(label)}</div>
      <div class="kpi-value ${toneClass}">${value != null ? escapeHtml(String(value)) : '--'}${deltaHtml}</div>
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
