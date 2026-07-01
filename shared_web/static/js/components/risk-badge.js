import { escapeHtml } from '../shared/app-utils.js';

/**
 * Risk badge component.
 * Renders a localized risk level badge with semantic color.
 */
export class RiskBadge {
  constructor(level, label) {
    this.level = level || 'medium';
    this.label = label || this.defaultLabel(level);
  }

  defaultLabel(level) {
    const map = {
      low: '低風險',
      medium: '中風險',
      high: '高風險',
      extreme: '極高風險',
    };
    return map[level] || map.medium;
  }

  render() {
    return `<span class="risk-badge risk-badge--${escapeHtml(this.level)}">${escapeHtml(this.label)}</span>`;
  }
}

export function renderRiskBadge(level, label) {
  return new RiskBadge(level, label).render();
}
