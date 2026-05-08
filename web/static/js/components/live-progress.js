const STAGES = [
  { id: 'idle', label: '閒置中', icon: '⏸️' },
  { id: 'fetching_data', label: '獲取資料', icon: '📡' },
  { id: 'regime_detection', label: '體制偵測', icon: '🔍' },
  { id: 'agent_recommendations', label: 'AI 推薦', icon: '🤖' },
  { id: 'control_filtering', label: '風控過濾', icon: '🛡️' },
  { id: 'simulation_running', label: '模擬結算', icon: '📈' },
  { id: 'complete', label: '完成', icon: '✅' }
];

export function renderLiveProgress(container, currentState) {
  if (!container) return;

  const currentIndex = STAGES.findIndex(s => s.id === currentState);
  
  let html = '<div class="live-progress-container">';
  html += '<div class="progress-track">';
  
  STAGES.forEach((stage, index) => {
    const isPast = index < currentIndex;
    const isActive = index === currentIndex;
    const isFuture = index > currentIndex;
    
    let statusClass = isPast ? 'completed' : (isActive ? 'active' : 'pending');
    
    html += `
      <div class="progress-step ${statusClass}">
        <div class="step-icon">${stage.icon}</div>
        <div class="step-label">${stage.label}</div>
        ${isActive ? '<div class="step-shimmer"></div>' : ''}
      </div>
    `;
    
    if (index < STAGES.length - 1) {
      html += `<div class="progress-connector ${isPast ? 'completed' : ''}"></div>`;
    }
  });

  html += '</div></div>';
  container.innerHTML = html;
}
