// Performance Report page module — wires the static page-shell container
// (`<div id="performanceReportContainer">`) to the shared renderPerformanceReport
// component, which fetches `/api/dashboard/performance-report` and renders the
// KPI grid, top-agents, regime breakdown, and monthly returns tables.
//
// Pattern aligned with `pages/retail_sentiment.js` (dynamic-import + thin init).

import { renderPerformanceReport } from '../components/performance-report.js';

export function loadPerformanceReport() {
  renderPerformanceReport('performanceReportContainer');
}
