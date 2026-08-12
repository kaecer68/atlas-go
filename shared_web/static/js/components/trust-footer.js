/**
 * TrustFooter: displays data sources, model version, last update time,
 * and disclaimer for retail investor pages.
 */

export class TrustFooter {
  /**
   * @param {Object} opts
   * @param {string[]} opts.sources
   * @param {string} opts.version
   * @param {string} [opts.lastUpdate]
   * @param {string} [opts.disclaimer]
   */
  constructor(opts) {
    this.opts = opts || {};
  }

  render() {
    const {
      sources = [],
      version = '',
      lastUpdate = '',
      disclaimer = '本平台為研究模擬用途，不構成投資建議。投資人應自行判斷並承擔風險。'
    } = this.opts;

    const sourcesHtml = sources.length
      ? `<div class="trust-footer__row"><span class="trust-footer__label">資料來源</span>${sources.map(s => escapeHtml(s)).join('、')}</div>`
      : '';
    const versionHtml = version
      ? `<div class="trust-footer__row"><span class="trust-footer__label">模型版本</span>${escapeHtml(version)}</div>`
      : '';
    const updateHtml = lastUpdate
      ? `<div class="trust-footer__row"><span class="trust-footer__label">最後更新</span>${escapeHtml(lastUpdate)}</div>`
      : '';

    return `<footer class="trust-footer" role="contentinfo">
      ${sourcesHtml}
      ${versionHtml}
      ${updateHtml}
      <div class="trust-footer__disclaimer">${escapeHtml(disclaimer)}</div>
      <div class="trust-footer__links">
        <a href="https://github.com/kaecer68/atlas-go" target="_blank" rel="noopener noreferrer">GitHub</a> ·
        <a href="/client/strategies">投資心法</a>
      </div>
    </footer>`;
  }
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

export function trustFooter(opts) {
  return new TrustFooter(opts).render();
}
