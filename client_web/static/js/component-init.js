import { renderPerformanceReport } from './components/performance-report.js';
import { eventSource } from './services/event-source.js';

// Module scripts are deferred; DOM is already ready.
renderPerformanceReport('performanceReportContainer');
