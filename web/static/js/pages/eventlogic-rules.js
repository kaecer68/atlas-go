import { escapeHtml } from '../shared/app-utils.js';

let rulesData = [];
let statsData = {};
let activeFilter = 'all';

export function renderEventLogicPage() {
  loadEventLogicData();
}

async function loadEventLogicData() {
  try {
    const [rulesResp, statsResp] = await Promise.all([
      fetch('/api/eventlogic/rules').then(r => r.json()).catch(() => null),
      fetch('/api/eventlogic/stats').then(r => r.json()).catch(() => null),
    ]);
    rulesData = (rulesResp && rulesResp.rules) ? rulesResp.rules : [];
    statsData = statsResp || {};
    render();
  } catch (e) {
    document.getElementById('page-eventlogic-rules').innerHTML = '<div class="empty">載入失敗</div>';
  }
}

function render() {
  const content = document.getElementById('page-eventlogic-rules');
  const filtered = filterRules();
  content.innerHTML = renderStatsBar() + renderToolbar() + renderTable(filtered) + renderCreateModal();
}

function renderStatsBar() {
  const total = statsData.total_rules || rulesData.length;
  const active = statsData.active_rules || 0;
  const degraded = statsData.degraded_rules || 0;
  const expired = statsData.expired_rules || 0;
  const avgHit = statsData.average_hit_rate || 0;
  return `<div class="kpi-grid" style="display:flex;gap:12px;margin-bottom:16px">
    <div class="kpi-card" style="flex:1"><div class="kpi-label">總規則</div><div class="kpi-value">${total}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--color-success)"><div class="kpi-label">活躍</div><div class="kpi-value">${active}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--color-warning)"><div class="kpi-label">降級</div><div class="kpi-value">${degraded}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--color-danger)"><div class="kpi-label">過期</div><div class="kpi-value">${expired}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--accent)"><div class="kpi-label">平均命中率</div><div class="kpi-value">${(avgHit*100).toFixed(1)}%</div></div>
  </div>`;
}

function renderToolbar() {
  return `<div style="display:flex;gap:8px;margin-bottom:12px;align-items:center;flex-wrap:wrap">
    <select onchange="window._elSetFilter(this.value)" style="padding:6px 10px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
      <option value="all" ${activeFilter==='all'?'selected':''}>全部</option>
      <option value="active" ${activeFilter==='active'?'selected':''}>活躍</option>
      <option value="degraded" ${activeFilter==='degraded'?'selected':''}>降級</option>
      <option value="expired" ${activeFilter==='expired'?'selected':''}>過期</option>
    </select>
    <button onclick="window._elCreate()" style="padding:6px 14px;background:var(--accent);color:#fff;border:none;border-radius:6px;cursor:pointer">＋ 新增規則</button>
    <button onclick="window._elDiscover()" style="padding:6px 14px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px;cursor:pointer">🔄 觸發發現</button>
  </div>`;
}

function renderTable(rules) {
  if (!rules || rules.length === 0) return `<div class="empty" style="padding:40px;text-align:center;color:var(--muted)">尚無規則</div>`;
  let rows = '';
  for (const r of rules) {
    const hp = (r.hit_rate*100).toFixed(1);
    const bc = r.hit_rate>=0.70?'var(--color-success)':r.hit_rate>=0.55?'var(--color-warning)':'var(--color-danger)';
    const sec = (r.affected_sectors||[]).map(s=>`<span class="badge">${escapeHtml(s)}</span>`).join(' ');
    const di = r.direction==='up'?'📈':r.direction==='down'?'📉':'↔️';
    rows += `<tr>
      <td style="font-family:var(--font-mono);font-size:11px;max-width:200px;overflow:hidden;text-overflow:ellipsis">${escapeHtml(r.id)}</td>
      <td>${di} ${escapeHtml(r.pattern)}</td>
      <td>${sec}</td>
      <td><div style="display:flex;align-items:center;gap:6px"><div style="flex:1;height:6px;background:var(--border);border-radius:3px"><div style="width:${hp}%;height:100%;background:${bc};border-radius:3px"></div></div><span style="font-size:12px;min-width:50px;text-align:right">${hp}%</span></div></td>
      <td><span class="badge ${r.status==='active'?'badge-success':r.status==='degraded'?'badge-warning':'badge-danger'}">${r.status}</span></td>
      <td style="font-size:11px;color:var(--muted)">${r.total_tests||0}/${r.total_hits||0}</td>
      <td style="white-space:nowrap">
        <button onclick="window._elValidate('${r.id}')" title="驗證" style="background:none;border:none;cursor:pointer;font-size:14px">✅</button>
        <button onclick="window._elEdit('${r.id}')" title="編輯" style="background:none;border:none;cursor:pointer;font-size:14px">✏️</button>
        <button onclick="window._elDelete('${r.id}')" title="刪除" style="background:none;border:none;cursor:pointer;font-size:14px">🗑️</button>
      </td>
    </tr>`;
  }
  return `<div class="table-wrapper"><table><thead><tr><th>ID</th><th>規則</th><th>板塊</th><th>命中率</th><th>狀態</th><th>測試/命中</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function renderCreateModal() {
  return `<div id="elModal" class="modal" style="display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);z-index:1000;justify-content:center;align-items:center">
    <div style="background:var(--panel);border-radius:12px;padding:24px;max-width:500px;width:90%">
      <h3 id="elModalTitle" style="margin-top:0">新增規則</h3>
      <div style="display:flex;flex-direction:column;gap:10px">
        <input id="el-id" placeholder="規則 ID" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
        <input id="el-pattern" placeholder="規則描述" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
        <select id="el-dir" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px"><option value="up">📈 上漲</option><option value="down">📉 下跌</option></select>
        <input id="el-sec" placeholder="板塊（逗號分隔）" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
        <input id="el-hr" type="number" step="0.01" min="0" max="1" value="0.5" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
      </div>
      <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
        <button onclick="document.getElementById('elModal').style.display='none'" style="padding:8px 16px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px;cursor:pointer">取消</button>
        <button onclick="window._elSave()" style="padding:8px 16px;background:var(--accent);color:#fff;border:none;border-radius:6px;cursor:pointer">儲存</button>
      </div>
    </div>
  </div>`;
}

function filterRules() { return activeFilter==='all'?rulesData:rulesData.filter(r=>r.status===activeFilter); }

window._elSetFilter = function(f) { activeFilter=f; render(); };
window._elCreate = function() {
  document.getElementById('elModal').style.display='flex';
  document.getElementById('elModalTitle').textContent='新增規則';
  ['el-id','el-pattern','el-sec'].forEach(id=>document.getElementById(id).value='');
  document.getElementById('el-dir').value='up';
  document.getElementById('el-hr').value='0.5';
};
window._elEdit = function(id) {
  const r = rulesData.find(x=>x.id===id); if(!r) return;
  document.getElementById('elModal').style.display='flex';
  document.getElementById('elModalTitle').textContent='編輯: '+id;
  document.getElementById('el-id').value=r.id;
  document.getElementById('el-pattern').value=r.pattern||'';
  document.getElementById('el-dir').value=r.direction||'up';
  document.getElementById('el-sec').value=(r.affected_sectors||[]).join(',');
  document.getElementById('el-hr').value=r.hit_rate;
};
window._elSave = async function() {
  const id=document.getElementById('el-id').value.trim();
  const pat=document.getElementById('el-pattern').value.trim();
  if(!id||!pat){alert('ID 和描述為必填');return;}
  const body={id,pattern:pat,direction:document.getElementById('el-dir').value,affected_sectors:document.getElementById('el-sec').value.split(',').map(s=>s.trim()).filter(Boolean),hit_rate:parseFloat(document.getElementById('el-hr').value)||0.5,status:'active',confidence_source:'manual'};
  const ex=rulesData.find(r=>r.id===id);
  try {
    const resp=await fetch(ex?'/api/eventlogic/rules/'+encodeURIComponent(id):'/api/eventlogic/rules',{method:ex?'PUT':'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
    if(!resp.ok)throw new Error(await resp.text());
    document.getElementById('elModal').style.display='none';
    loadEventLogicData();
  }catch(e){alert('失敗: '+e.message);}
};
window._elDelete = async function(id) {
  if(!confirm('刪除 '+id+'？'))return;
  try{await fetch('/api/eventlogic/rules/'+encodeURIComponent(id),{method:'DELETE'});loadEventLogicData();}catch(e){alert('失敗: '+e.message);}
};
window._elValidate = async function(id) {
  try{await fetch('/api/eventlogic/rules/'+encodeURIComponent(id)+'/validate',{method:'POST'});}catch(e){alert('失敗: '+e.message);}
};
window._elDiscover = async function() {
  try{await fetch('/api/eventlogic/discover',{method:'POST'});loadEventLogicData();}catch(e){alert('失敗: '+e.message);}
};
