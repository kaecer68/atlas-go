// shared_web/static/js/shared/scroll-utils.js
// Scroll-to utility for signal-chip → market-card linking

/**
 * Scroll to a section element identified by a CSS selector (e.g. '#market-card-2330').
 * Respects prefers-reduced-motion: uses 'instant' when reduced motion is preferred,
 * 'smooth' otherwise. Silently no-ops if the element is not found.
 * @param {string} targetId - CSS selector string (must include leading '#')
 */
export function scrollToSection(targetId) {
  const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const behavior = prefersReduced ? 'instant' : 'smooth';
  document.querySelector(targetId)?.scrollIntoView({ behavior, block: 'start' });
}
