/**
 * ActionCard component for empty states and CTAs.
 */

export class ActionCard {
  /**
   * @param {Object} opts
   * @param {string} opts.icon
   * @param {string} opts.title
   * @param {string} opts.description
   * @param {Array<{label:string, action:string}>} [opts.actions]
   */
  constructor(opts) {
    this.opts = opts || {};
  }

  render() {
    const { icon = '📋', title, description, actions = [] } = this.opts;
    const actionsHtml = actions.length ? `
      <div class="action-card__actions">
        ${actions.map(a => `<button class="btn btn--primary action-card__btn" data-action="${escapeHtml(a.action)}">${escapeHtml(a.label)}</button>`).join('')}
      </div>
    ` : '';

    return `<div class="action-card">
      <div class="action-card__icon">${icon}</div>
      <h3 class="action-card__title">${escapeHtml(title)}</h3>
      <p class="action-card__description">${escapeHtml(description)}</p>
      ${actionsHtml}
    </div>`;
  }
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

export function actionCard(opts) {
  return new ActionCard(opts).render();
}
