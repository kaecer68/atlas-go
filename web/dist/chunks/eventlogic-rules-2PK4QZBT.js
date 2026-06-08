import"./chunk-RBRLONW2.js";import{h as a}from"./chunk-35S32YAS.js";var r=[],s={},i="all",u=null;function k(){c()}async function c(){try{let[e,n]=await Promise.all([fetch("/api/eventlogic/rules").then(t=>t.json()).catch(()=>null),fetch("/api/eventlogic/stats").then(t=>t.json()).catch(()=>null)]);r=e&&e.rules?e.rules:[],s=n||{},v()}catch(e){console.error("Event logic load failed:",e);let n=document.getElementById("page-eventlogic-rules");n&&(n.innerHTML='<div class="empty" style="padding:40px;text-align:center;color:var(--err)">\u8F09\u5165\u5931\u6557\uFF1A'+a(e.message)+"</div>")}}function v(){let e=document.getElementById("page-eventlogic-rules");if(!e)return;let n=h();e.innerHTML=x()+f()+m(n)+y(),b()}function x(){let e=s.total_rules||r.length,n=s.active_rules||0,t=s.degraded_rules||0,o=s.expired_rules||0,l=s.average_hit_rate||0;return`<div class="kpi-grid" style="display:flex;gap:12px;margin-bottom:16px">
    <div class="kpi-card" style="flex:1"><div class="kpi-label">\u7E3D\u898F\u5247</div><div class="kpi-value">${e}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--color-success)"><div class="kpi-label">\u6D3B\u8E8D</div><div class="kpi-value">${n}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--color-warning)"><div class="kpi-label">\u964D\u7D1A</div><div class="kpi-value">${t}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--color-danger)"><div class="kpi-label">\u904E\u671F</div><div class="kpi-value">${o}</div></div>
    <div class="kpi-card" style="flex:1;border-left:3px solid var(--accent)"><div class="kpi-label">\u5E73\u5747\u547D\u4E2D\u7387</div><div class="kpi-value">${(l*100).toFixed(1)}%</div></div>
  </div>`}function f(){return`<div style="display:flex;gap:8px;margin-bottom:12px;align-items:center;flex-wrap:wrap">
    <select onchange="window._elSetFilter(this.value)" style="padding:6px 10px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
      <option value="all" ${i==="all"?"selected":""}>\u5168\u90E8</option>
      <option value="active" ${i==="active"?"selected":""}>\u6D3B\u8E8D</option>
      <option value="degraded" ${i==="degraded"?"selected":""}>\u964D\u7D1A</option>
      <option value="expired" ${i==="expired"?"selected":""}>\u904E\u671F</option>
    </select>
    <button onclick="window._elCreate()" style="padding:6px 14px;background:var(--accent);color:#fff;border:none;border-radius:6px;cursor:pointer">\uFF0B \u65B0\u589E\u898F\u5247</button>
    <button onclick="window._elDiscover()" style="padding:6px 14px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px;cursor:pointer">\u{1F504} \u89F8\u767C\u767C\u73FE</button>
  </div>`}function m(e){if(!e||e.length===0)return'<div class="empty" style="padding:40px;text-align:center;color:var(--muted)">\u5C1A\u7121\u898F\u5247</div>';let n="";for(let t of e){let o=(t.hit_rate*100).toFixed(1),l=t.hit_rate>=.7?"var(--color-success)":t.hit_rate>=.5?"var(--color-warning)":"var(--color-danger)",d=(t.affected_sectors||[]).map(g=>`<span class="badge">${a(g)}</span>`).join(" "),p=t.direction==="up"?"\u{1F4C8}":t.direction==="down"?"\u{1F4C9}":"\u2194\uFE0F";n+=`<tr>
      <td style="font-family:var(--font-mono);font-size:11px;max-width:200px;overflow:hidden;text-overflow:ellipsis">${a(t.id)}</td>
      <td>${p} ${a(t.pattern)}</td>
      <td>${d}</td>
      <td><div style="display:flex;align-items:center;gap:6px"><div style="flex:1;height:6px;background:var(--border);border-radius:3px"><div style="width:${o}%;height:100%;background:${l};border-radius:3px"></div></div><span style="font-size:12px;min-width:50px;text-align:right">${o}%</span></div></td>
      <td><span class="badge ${t.status==="active"?"badge-success":t.status==="degraded"?"badge-warning":"badge-danger"}">${t.status}</span></td>
      <td style="font-size:11px;color:var(--muted)">${t.total_hits||0} / ${t.total_tests||0}</td>
      <td style="white-space:nowrap">
        <button onclick="window._elValidate('${a(t.id)}')" title="\u9A57\u8B49" style="background:none;border:none;cursor:pointer;font-size:14px">\u2705</button>
        <button onclick="window._elEdit('${a(t.id)}')" title="\u7DE8\u8F2F" style="background:none;border:none;cursor:pointer;font-size:14px">\u270F\uFE0F</button>
        <button onclick="window._elDelete('${a(t.id)}')" title="\u522A\u9664" style="background:none;border:none;cursor:pointer;font-size:14px">\u{1F5D1}\uFE0F</button>
      </td>
    </tr>`}return`<div class="table-wrapper"><table><thead><tr><th>ID</th><th>\u898F\u5247</th><th>\u677F\u584A</th><th>\u547D\u4E2D\u7387</th><th>\u72C0\u614B</th><th>\u547D\u4E2D/\u6E2C\u8A66</th><th>\u64CD\u4F5C</th></tr></thead><tbody>${n}</tbody></table></div>`}function y(){return`<div id="elModal" class="modal" style="display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,0.5);z-index:1000;justify-content:center;align-items:center">
    <div style="background:var(--panel);border-radius:12px;padding:24px;max-width:500px;width:90%">
      <h3 id="elModalTitle" style="margin-top:0">\u65B0\u589E\u898F\u5247</h3>
      <div style="display:flex;flex-direction:column;gap:10px">
        <input id="el-id" placeholder="\u898F\u5247 ID" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
        <input id="el-pattern" placeholder="\u898F\u5247\u63CF\u8FF0" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
        <select id="el-dir" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px"><option value="up">\u{1F4C8} \u4E0A\u6F32</option><option value="down">\u{1F4C9} \u4E0B\u8DCC</option><option value="volatile">\u2194\uFE0F \u6CE2\u52D5</option></select>
        <input id="el-sec" placeholder="\u677F\u584A\uFF08\u9017\u865F\u5206\u9694\uFF09" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
        <input id="el-hr" type="number" step="0.01" min="0" max="1" value="0.5" style="padding:8px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px">
      </div>
      <div style="margin-top:16px;display:flex;gap:8px;justify-content:flex-end">
        <button id="elCancelBtn" style="padding:8px 16px;background:var(--panel-l2);border:1px solid var(--border);color:var(--text);border-radius:6px;cursor:pointer">\u53D6\u6D88</button>
        <button id="elSaveBtn" style="padding:8px 16px;background:var(--accent);color:#fff;border:none;border-radius:6px;cursor:pointer">\u5132\u5B58</button>
      </div>
    </div>
  </div>`}function b(){let e=document.getElementById("elModal");if(!e)return;e.onclick=function(o){o.target===e&&(e.style.display="none")};let n=document.getElementById("elCancelBtn");n&&(n.onclick=function(){e.style.display="none"});let t=document.getElementById("elSaveBtn");t&&(t.onclick=window._elSave)}function h(){return i==="all"?r:r.filter(e=>e.status===i)}window._elSetFilter=function(e){i=e,v()};window._elCreate=function(){u=null;let e=document.getElementById("elModal");e.style.display="flex",document.getElementById("elModalTitle").textContent="\u65B0\u589E\u898F\u5247";let n=document.getElementById("el-id");n.readOnly=!1,n.style.opacity="1",["el-id","el-pattern","el-sec"].forEach(t=>document.getElementById(t).value=""),document.getElementById("el-dir").value="up",document.getElementById("el-hr").value="0.5"};window._elEdit=function(e){let n=r.find(l=>l.id===e);if(!n)return;u=e;let t=document.getElementById("elModal");t.style.display="flex",document.getElementById("elModalTitle").textContent="\u7DE8\u8F2F: "+e;let o=document.getElementById("el-id");o.value=n.id,o.readOnly=!0,o.style.opacity="0.6",document.getElementById("el-pattern").value=n.pattern||"",document.getElementById("el-dir").value=n.direction||"up",document.getElementById("el-sec").value=(n.affected_sectors||[]).join(","),document.getElementById("el-hr").value=n.hit_rate};window._elSave=async function(){let e=document.getElementById("el-id").value.trim(),n=document.getElementById("el-pattern").value.trim();if(!e||!n){alert("ID \u548C\u63CF\u8FF0\u70BA\u5FC5\u586B");return}let t={id:e,pattern:n,direction:document.getElementById("el-dir").value,affected_sectors:document.getElementById("el-sec").value.split(",").map(l=>l.trim()).filter(Boolean),hit_rate:parseFloat(document.getElementById("el-hr").value)||.5,status:"active",confidence_source:"manual"},o=r.find(l=>l.id===e);o&&o.conditions?t.conditions=o.conditions:t.conditions=[];try{let l=await fetch(o?"/api/eventlogic/rules/"+encodeURIComponent(e):"/api/eventlogic/rules",{method:o?"PUT":"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(t)});if(!l.ok)throw new Error(await l.text());document.getElementById("elModal").style.display="none",u=null,c()}catch(l){alert("\u5931\u6557: "+l.message)}};window._elDelete=async function(e){if(confirm("\u522A\u9664 "+e+"\uFF1F"))try{await fetch("/api/eventlogic/rules/"+encodeURIComponent(e),{method:"DELETE"}),c()}catch(n){alert("\u5931\u6557: "+n.message)}};window._elValidate=async function(e){try{let n=await fetch("/api/eventlogic/rules/"+encodeURIComponent(e)+"/validate",{method:"POST"});if(!n.ok)throw new Error(await n.text());let t=await n.json(),o=r.find(p=>p.id===e),l=o&&o.status!==t.status,d=`\u2705 \u9A57\u8B49\u5B8C\u6210
`;d+="\u547D\u4E2D\u7387: "+(t.hit_rate*100).toFixed(1)+`%
`,d+="\u6E2C\u8A66\u6B21\u6578: "+t.total_tests+" (\u547D\u4E2D "+t.total_hits+`)
`,d+="\u72C0\u614B: "+t.status,l&&(d+=" (\u5DF2\u81EA\u52D5\u8ABF\u6574)"),t.message&&(d+=`
`+t.message),alert(d),c()}catch(n){alert("\u5931\u6557: "+n.message)}};window._elDiscover=async function(){try{let e=await fetch("/api/eventlogic/discover",{method:"POST"});if(!e.ok)throw new Error(await e.text());let n=await e.json(),t=`\u{1F504} \u767C\u73FE\u72C0\u614B
`;t+="\u81EA\u52D5\u767C\u73FE\u898F\u5247: "+(n.auto_discovered_count||0)+` \u689D
`,t+="\u7E3D\u898F\u5247\u6578: "+(n.total_rules||0)+` \u689D
`,n.auto_discovered_count>0&&(t+=`
\u6700\u8FD1\u767C\u73FE\u7684\u898F\u5247:
`,(n.auto_discovered_rules||[]).slice(0,5).forEach(o=>{t+="- "+o.pattern+" (\u547D\u4E2D\u7387 "+(o.hit_rate*100).toFixed(1)+`%)
`})),t+=`
`+n.message,alert(t),c()}catch(e){alert("\u5931\u6557: "+e.message)}};export{k as renderEventLogicPage};
