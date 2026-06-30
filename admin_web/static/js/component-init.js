import { CircuitBreakerPanel } from './components/circuit-breaker.js';
import { eventSource } from './services/event-source.js';
import { SimHealthPanel } from './components/sim-health.js';
import { DeploymentDashboard } from './components/deployment-dashboard.js';

// Module scripts are deferred; DOM is already ready.
const cbPanel = new CircuitBreakerPanel('circuitBreakerPanel');
eventSource.on('*', (ev) => cbPanel.handleSSE(ev));


// Initialize Simulation Health Panel on the metrics page
const simHealthContainer = document.getElementById('simHealthContainer');
if (simHealthContainer) {
  window.simHealthPanel = new SimHealthPanel('simHealthContainer');
}

const deploymentContainer = document.getElementById('deploymentDashboardContainer');
if (deploymentContainer) {
  window.deploymentDashboard = new DeploymentDashboard('deploymentDashboardContainer');
}
