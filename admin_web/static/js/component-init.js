import { CircuitBreakerPanel } from './components/circuit-breaker.js';
import { eventSource } from './services/event-source.js';
import { SimHealthPanel } from './components/sim-health.js';
import { DeploymentDashboard } from './components/deployment-dashboard.js';

// Module scripts are deferred; DOM is already ready.
const cbPanel = new CircuitBreakerPanel('circuitBreakerPanel');
eventSource.on('*', (ev) => cbPanel.handleSSE(ev));

// 2026-08-31 (#1776 audit): SSE connection ownership moved here from the
// deleted 即時事件流 panel wiring (main.js initEventStream) — the circuit
// breaker panel is the surviving SSE consumer, so connection + status pill
// now live next to it.
eventSource.onStatusChange((status) => {
  const pill = document.getElementById('refreshPill');
  if (!pill) return;
  pill.classList.remove('sse-connected', 'sse-connecting', 'sse-error');
  if (status === 'connected') pill.classList.add('sse-connected');
  else if (status === 'connecting') pill.classList.add('sse-connecting');
  else if (status === 'error' || status === 'disconnected') pill.classList.add('sse-error');
});
eventSource.connect();


// Initialize Simulation Health Panel on the metrics page
const simHealthContainer = document.getElementById('simHealthContainer');
if (simHealthContainer) {
  window.simHealthPanel = new SimHealthPanel('simHealthContainer');
}

const deploymentContainer = document.getElementById('deploymentDashboardContainer');
if (deploymentContainer) {
  window.deploymentDashboard = new DeploymentDashboard('deploymentDashboardContainer');
}
