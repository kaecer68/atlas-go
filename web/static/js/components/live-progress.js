const STAGES = [
  { id: 'idle', label: '系統就緒', icon: '⏸️', hint: '系統已就緒，等待執行回測或手動觸發模擬交易' },
  { id: 'fetching_data', label: '獲取資料', icon: '📡', hint: '正在從資料提供者載入市場資料與回測數據' },
  { id: 'regime_detection', label: '體制偵測', icon: '🔍', hint: '分析當前市場體制（多頭/空頭/盤整）與宏觀敘事脈絡' },
  { id: 'agent_recommendations', label: 'AI 推薦', icon: '🤖', hint: '各產業與風格 Agent 產生推薦標的與信心度評分' },
  { id: 'control_filtering', label: '風控過濾', icon: '🛡️', hint: '風控長與投資長執行最終放行/否決決策' },
  { id: 'simulation_running', label: '模擬結算', icon: '📈', hint: '模擬引擎執行部位調整與損益計算' },
  { id: 'complete', label: '完成', icon: '✅', hint: '本時段處理完成，結果已寫入 Ledger 與儀表板' }
];

export function renderLiveProgress(container, currentState) {
  if (!container) return;

  const currentIndex = STAGES.findIndex(s => s.id === currentState);
  const actualIndex = currentIndex >= 0 ? currentIndex : 0;
  const currentStage = STAGES[actualIndex] || STAGES[0];
  
  let html = '<div class="live-progress-container">';
  html += '<div class="progress-track">';
  
  STAGES.forEach((stage, index) => {
    const isPast = index < actualIndex;
    const isActive = index === actualIndex;
    const isFuture = index > actualIndex;
    
    let statusClass = isPast ? 'completed' : (isActive ? 'active' : 'pending');
    
    html += `
      <div class="progress-step ${statusClass}" title="${stage.hint}">
        <div class="step-icon">${stage.icon}</div>
        <div class="step-label">${stage.label}</div>
        ${isActive ? '<div class="step-shimmer"></div>' : ''}
      </div>
    `;
    
    if (index < STAGES.length - 1) {
      html += `<div class="progress-connector ${isPast ? 'completed' : ''}"></div>`;
    }
  });

  html += '</div>';
  
  html += `<div class="stage-hint">${currentStage.hint}</div>`;
  
  html += '</div>';
  container.innerHTML = html;
}

export function getStageHint(stageId) {
  const stage = STAGES.find(s => s.id === stageId);
  return stage ? stage.hint : '';
}
