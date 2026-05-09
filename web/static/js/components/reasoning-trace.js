import { getJSON, escapeHtml } from '../main.js';

export async function loadReasoningTrace(sessionId) {
  const container = document.getElementById('page-reasoning-trace');
  if (!container) return;

  if (!sessionId) {
    container.innerHTML = '<div class="help-panel">請先選擇一個 Session</div>';
    return;
  }

  container.innerHTML = '<div class="help-panel">載入中...</div>';

  try {
    const data = await getJSON('/api/dashboard/reasoning-trace?session_id=' + encodeURIComponent(sessionId));
    renderReasoningTimeline('page-reasoning-trace', data);
  } catch (e) {
    console.error('Failed to load reasoning trace:', e);
    container.innerHTML = '<div class="help-panel text-down">載入失敗: ' + escapeHtml(e.message) + '</div>';
  }
}

export function renderReasoningTimeline(containerId, data) {
  const container = document.getElementById(containerId);
  if (!container) return;

  if (!data || !data.traces || data.traces.length === 0) {
    container.innerHTML = '<div class="help-panel">目前沒有決策追蹤資料</div>';
    return;
  }

  const phases = {
    'regime_detection': { label: '盤勢判定', color: '#3b82f6' }, // blue
    'agent_recommendation': { label: '代理推薦', color: '#22c55e' }, // green
    'control_filter': { label: '控制層過濾', color: '#f97316' }, // orange
    'portfolio_build': { label: '組合構建', color: '#a855f7' } // purple
  };

  let html = '<div><h2 style="font-size: 18px; margin-bottom: 20px;">決策追蹤 (Session: ' + escapeHtml(data.session_id) + ')</h2>';
  html += '<div style="position: relative; border-left: 2px solid var(--border); margin-left: 10px; padding-left: 20px;">';

  data.traces.forEach(trace => {
    const phaseInfo = phases[trace.phase] || { label: trace.phase, color: '#9ca3af' };
    const confidencePct = Math.round((trace.confidence || 0) * 100);
    const fallbackBadge = trace.is_fallback ? '<span style="margin-left: 8px; padding: 2px 6px; font-size: 11px; font-weight: bold; background-color: #facc15; color: #854d0e; border-radius: 4px;">備援</span>' : '';
    
    html += `
      <div style="position: relative; margin-bottom: 24px;">
        <div style="position: absolute; width: 12px; height: 12px; border-radius: 50%; left: -27px; top: 4px; background-color: ${phaseInfo.color}; box-shadow: 0 0 0 3px var(--bg);"></div>
        
        <div class="card" style="padding: 16px;">
          <div class="flex-between mb-sm">
            <h3 style="margin: 0; font-size: 15px; color: ${phaseInfo.color};">${escapeHtml(phaseInfo.label)} ${fallbackBadge}</h3>
          </div>
          
          <div class="mb-sm" style="display: flex; align-items: center; gap: 10px;">
            <span class="text-sm text-muted" style="min-width: 60px;">信心水準</span>
            <div style="flex-grow: 1; height: 6px; background-color: var(--panel-l3); border-radius: 3px; overflow: hidden;">
              <div style="height: 100%; width: ${confidencePct}%; background-color: ${phaseInfo.color};"></div>
            </div>
            <span class="text-sm text-muted" style="min-width: 40px; text-align: right;">${confidencePct}%</span>
          </div>
          
          ${trace.explanation ? `
            <div class="mb-md" style="padding: 12px; background-color: rgba(59, 130, 246, 0.1); border: 1px solid rgba(59, 130, 246, 0.2); border-radius: 4px; font-size: 13px; color: var(--text); white-space: pre-line; line-height: 1.5;">
              ${escapeHtml(trace.explanation)}
            </div>
          ` : ''}
          
          <details>
            <summary style="cursor: pointer; font-size: 12px; color: var(--muted); user-select: none;">原始資料 (Raw JSON)</summary>
            <pre style="margin-top: 8px; padding: 10px; background-color: var(--panel-l2); border-radius: 4px; overflow-x: auto; font-size: 11px; font-family: var(--font-mono); color: var(--muted);">${escapeHtml(JSON.stringify(trace.raw_data || {}, null, 2))}</pre>
          </details>
        </div>
      </div>
    `;
  });

  html += '</div></div>';
  container.innerHTML = html;
}
