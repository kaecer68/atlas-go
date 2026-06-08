import{f as o}from"./chunk-RBRLONW2.js";import{a as m}from"./chunk-OU7I3ZGZ.js";import{h as n}from"./chunk-35S32YAS.js";function f(i){let r=document.getElementById("experimentInbox");if(!i){r.innerHTML=o("\u5C1A\u7121\u5BE6\u9A57\u8CC7\u6599","\u57F7\u884C\u300Cgo run ./cmd/run-experiment -brief &lt;file&gt;\u300D\u5F8C\u5C07\u81EA\u52D5\u986F\u793A"),r.classList.remove("loading");return}r.classList.remove("loading");let c=i.pending_judges||[],s=i.pending_promotes||[],d=i.recent_history||[],l=(e,t)=>`
    <div class="inbox-card">
      <div class="title">${e.experiment_id}</div>
      <div class="meta">${m(e.target_agent_id)} \xB7 ${e.mutation_type} \xB7 \u57FA\u7DDA ${fmt(e.baseline_value)} / \u5019\u9078 ${fmt(e.candidate_value)}</div>
      ${e.mutation_summary?`<div style="margin:3px 0;font-size:11px;color:var(--muted)">${e.mutation_summary}</div>`:""}
      ${t?`<div style="margin:4px 0;font-size:11px;color:var(--muted)">${t}</div>`:""}
      <div class="actions">${e._actions||""}</div>
    </div>
  `,p=e=>`
    <button onclick="judgeExperiment('${e}')">\u8A55\u5224</button>
    <button onclick="viewDiff('${e}')">\u5DEE\u7570</button>
  `,u=e=>`
    <button class="primary" onclick="openPromote('data/state/experiments/${e}.json')">\u6649\u5347</button>
    <button onclick="viewDiff('${e}')">\u5DEE\u7570</button>
  `,$=(e,t)=>`<span class="badge ${e==="accepted"?"ok":e==="rejected"?"err":"warn"}">${e==="accepted"?"\u5DF2\u63A5\u53D7":e==="rejected"?"\u5DF2\u62D2\u7D55":e}</span>${t?` <span title="${t.replace(/"/g,"&quot;")}" style="cursor:help;border-bottom:1px dotted var(--muted)">\u2139\uFE0F</span>`:""}`;r.innerHTML=`
    <div class="inbox-col">
      <h3>\u5F85\u8A55\u5224 (${c.length})</h3>
      ${c.length?c.map(e=>l(e).replace("${item._actions || ''}",p(e.experiment_id))).join(""):o("\u7121\u5F85\u8A55\u5224\u5BE6\u9A57","\u57F7\u884C\u5BE6\u9A57\u5F8C\u5C07\u81EA\u52D5\u986F\u793A")}
    </div>
    <div class="inbox-col">
      <h3>\u5F85\u6649\u5347 (${s.length})</h3>
      ${s.length?s.map(e=>l(e).replace("${item._actions || ''}",u(e.experiment_id))).join(""):o("\u7121\u5F85\u6649\u5347\u5BE6\u9A57","\u8A55\u5224\u901A\u904E\u5F8C\u5C07\u81EA\u52D5\u986F\u793A")}
    </div>
    <div class="inbox-col">
      <h3>\u8FD1\u671F\u6B77\u53F2 (${d.length})</h3>
      ${d.length?d.map(e=>{let t=e.status==="rejected"&&e.reject_reason?`\u539F\u56E0: ${n(e.reject_reason)}`:"";return l(e,t).replace("${item._actions || ''}",$(e.status,e.reject_reason))}).join(""):o("\u7121\u6B77\u53F2\u7D00\u9304","")}
    </div>
  `;let a=document.getElementById("promoteSelect");a.innerHTML='<option value="">-- \u9078\u64C7\u5DF2\u63A5\u53D7\u7684\u5BE6\u9A57 --</option>'+s.map(e=>`<option value="data/state/experiments/${n(e.experiment_id)}.json">${n(e.experiment_id)} (${n(m(e.target_agent_id))})</option>`).join(""),a.options.length>1&&a.selectedIndex===0&&(a.selectedIndex=1)}export{f as renderInbox};
