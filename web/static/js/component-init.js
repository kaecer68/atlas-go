import { CircuitBreakerPanel } from './components/circuit-breaker.js';
import { renderPerformanceReport } from './components/performance-report.js';
import { eventSource } from './services/event-source.js';
import { SimHealthPanel } from './components/sim-health.js';

// Module scripts are deferred; DOM is already ready.
const cbPanel = new CircuitBreakerPanel('circuitBreakerPanel');
eventSource.on('*', (ev) => cbPanel.handleSSE(ev));

renderPerformanceReport('performanceReportContainer');

// Initialize Simulation Health Panel on the metrics page
const simHealthContainer = document.getElementById('simHealthContainer');
if (simHealthContainer) {
  window.simHealthPanel = new SimHealthPanel('simHealthContainer');
}
