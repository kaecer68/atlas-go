import{a as u}from"./chunk-RBRLONW2.js";import{h as i}from"./chunk-35S32YAS.js";var p=null,m="--:--";async function j(){let t=document.getElementById("decisionChain");if(t){t.classList.add("loading"),t.innerHTML='<div class="loading-placeholder" style="padding:20px;text-align:center;color:var(--muted)">\u8F09\u5165\u6C7A\u7B56\u93C8\u8CC7\u6599\u4E2D\u2026</div>';try{p=await u("/api/dashboard/decision-chain"),m=new Date().toLocaleTimeString("zh-TW"),b(p)}catch(n){console.error("Decision chain load failed:",n),t.innerHTML='<div class="error-box" style="padding:20px;color:var(--err);text-align:center">\u8F09\u5165\u5931\u6557\uFF1A'+i(String(n.message||n))+"</div>",t.classList.remove("loading")}}}function b(t){let n=document.getElementById("decisionChain");n&&(n.classList.remove("loading"),n.innerHTML=""+l("\u23F1 \u5373\u6642\u4E8B\u4EF6\u96F7\u9054","events-radar",g(t),m)+l("\u{1F9E0} \u4E8B\u4EF6\u908F\u8F2F\u5EAB","logic-rules",h(t),"W4 \u81EA\u6211\u7CBE\u9032\u4E2D")+l("\u{1F525} \u7522\u696D\u71B1\u529B\u5716","sector-heatmap",v(t))+l("\u{1F4CB} \u63A8\u85A6\u6A19\u7684","recommendations",x(t))+l("\u{1F514} \u51FA\u5834\u63D0\u9192","exit-alerts",$(t)))}function l(t,n,o,e){return`<div class="dc-panel" id="dc-${n}">
    <div class="dc-panel-header" onclick="document.getElementById('dc-${n}').classList.toggle('collapsed')">
      <span class="dc-arrow">\u25BC</span>
      <span class="dc-title">${t}</span>
      <span class="dc-sub">${e||""}</span>
    </div>
    <div class="dc-panel-body">${o}</div>
  </div>`}function d(t){return`<div class="dc-empty" style="padding:16px;color:var(--muted);text-align:center">${t||"\u5C1A\u7121\u8CC7\u6599"}</div>`}function g(t){let n=t&&t.events;if(!n)return d("\u5C1A\u7121\u4E8B\u4EF6\u8CC7\u6599");let o="",e=n.premarket;if(e){let s=[];if(e.us_market&&e.us_market.sox_pct!==void 0){let a=e.us_market.sox_pct>=0?"color:var(--color-success);font-weight:600":"color:var(--color-danger);font-weight:600";s.push(`<span>SOX <span style="${a}">${f(e.us_market.sox_pct)}</span></span>`)}if(e.fx&&e.fx.usd_twd){let a=e.fx.change_pct>=0?"color:var(--color-danger)":"color:var(--color-success)";s.push(`<span>USD/TWD ${e.fx.usd_twd.toFixed(2)} <span style="${a}">${f(e.fx.change_pct)}</span></span>`)}if(e.foreign_flow&&e.foreign_flow.net_buy_twd!==void 0&&s.push(`<span>\u5916\u8CC7\u6DE8\u8CB7\u8D85 ${y(e.foreign_flow.net_buy_twd)}</span>`),e.bdi&&e.bdi.value){let a=e.bdi.deviation_pct>=0?"color:var(--color-success);font-weight:600":"color:var(--color-danger);font-weight:600";s.push(`<span>BDI ${e.bdi.value} <span style="${a}">${f(e.bdi.deviation_pct)}</span></span>`)}s.length&&(o=`<div class="dc-badge-row" style="margin-bottom:10px">${s.map(a=>`<span class="badge info" style="font-size:11px">${a}</span>`).join(" ")}</div>`)}let r=(n.today||[]).map(s=>{let a=s.severity||"low";return`<div class="dc-event-row">
      <span>${a==="critical"?"\u{1F534}":a==="high"?"\u{1F7E0}":a==="medium"?"\u{1F7E1}":"\u{1F7E2}"} <strong>${i(s.theme)}</strong></span>
      <span class="text-muted" style="font-size:11px">Conf ${(s.confidence*100).toFixed(0)}% \xB7 Hit ${(s.hit_rate*100).toFixed(0)}% \xB7 ${i(s.status||"active")}</span>
    </div>`}).join(""),c=(n.recent||[]).slice(0,5).map(s=>`<div class="dc-event-row" style="opacity:0.7">
      <span>\u{1F4CC} <strong>${i(s.theme)}</strong></span>
      <span class="text-muted" style="font-size:11px">${w(s.timestamp)} \xB7 Conf ${(s.confidence*100).toFixed(0)}%</span>
    </div>`).join("");return`<div class="dc-section">
    ${o}
    ${n.today&&n.today.length?'<div class="dc-section-title">\u{1F4E1} \u4ECA\u65E5\u4E8B\u4EF6 ('+n.today.length+")</div>"+r:d("\u4ECA\u65E5\u66AB\u7121\u4E8B\u4EF6")}
    ${c?'<div class="dc-section-title" style="margin-top:8px">\u{1F4C6} \u8FD1\u671F\u4E8B\u4EF6</div>'+c:""}
  </div>`}function h(t){let n=t&&t.logic_rules;return!n||!n.length?d("\u5C1A\u7121\u4E8B\u4EF6\u908F\u8F2F\u898F\u5247\uFF08\u7B49\u5F85 W4 \u7A2E\u5B50\u898F\u5247\u8F09\u5165\uFF09"):`<div class="dc-section">${n.map(e=>{let r=Math.round(e.hit_rate*100),c=r>=70?"var(--status-ok)":r>=50?"var(--status-warn)":"var(--status-err)";return`<div class="dc-rule-row">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px">
        <span class="dc-rule-id" title="${i(e.id)}">#${i(e.id).slice(0,30)}</span>
        <span class="badge ${e.status==="active"?"ok":"warn"}">${i(e.status)}</span>
      </div>
      <div class="dc-rule-pattern">${i(e.pattern)}</div>
      <div style="display:flex;align-items:center;gap:8px;margin-top:4px">
        <div class="dc-hitbar" style="flex:1;height:6px;background:var(--border);border-radius:3px">
          <div style="width:${r}%;height:100%;background:${c};border-radius:3px;transition:width 0.3s"></div>
        </div>
        <span style="font-size:11px;font-weight:600;min-width:36px;text-align:right">${r}%</span>
      </div>
      <div style="margin-top:2px;font-size:10px;color:var(--muted)">
        ${(e.affected_sectors||[]).map(s=>`<span class="badge muted">${i(s)}</span>`).join(" ")}
        <span class="badge ${e.direction==="up"?"ok":e.direction==="down"?"err":"warn"}">${i(e.direction)}</span>
      </div>
    </div>`}).join("")}</div>`}function v(t){let n=t&&t.sector_heatmap;return!n||!n.length?d("\u5C1A\u7121\u7522\u696D\u6578\u64DA"):`<div class="dc-heat-grid" style="display:flex;flex-wrap:wrap;gap:8px">${n.map(e=>{let r=e.confidence==="high"?"rgba(34,197,94,0.15)":e.confidence==="medium"?"rgba(251,191,36,0.15)":"rgba(156,163,175,0.1)",c=e.confidence==="high"?"var(--color-success)":e.confidence==="medium"?"var(--color-warning)":"var(--muted)",s=e.confidence==="high"?"\u{1F525}":e.confidence==="medium"?"\u{1F7E1}":"\u26AA";return`<div class="dc-heat-badge" style="background:${r};border:1px solid ${c};border-radius:8px;padding:8px 10px;display:flex;align-items:center;gap:8px">
      <span style="font-size:14px">${s}</span>
      <div style="flex:1">
        <div style="font-size:12px;font-weight:600">${i(e.sector)}</div>
        <div style="font-size:10px;color:var(--muted)">${(e.reasons||[]).map(i).join(" \xB7 ")}</div>
      </div>
      <span class="badge ${e.confidence==="high"?"ok":e.confidence==="medium"?"warn":"muted"}">${e.confidence}</span>
    </div>`}).join("")}</div>`}function x(t){let n=t&&t.recommendations;return!n||!n.length?d("\u66AB\u7121\u63A8\u85A6\u6A19\u7684"):`<div class="dc-table-wrap"><table class="dc-table">
    <thead><tr><th>\u4EE3\u865F</th><th>\u540D\u7A31</th><th>\u65B9\u5411</th><th>\u80A1\u6578</th><th>\u7F6E\u4FE1\u5EA6</th><th>\u539F\u56E0</th></tr></thead>
    <tbody>${n.map(e=>{let r=Math.round(e.confidence*100),c=r>=80?"var(--color-success)":r>=60?"var(--color-warning)":"var(--muted)";return`<tr>
      <td><span class="badge muted">${i(e.symbol)}</span></td>
      <td>${i(e.name||e.symbol)}</td>
      <td><span class="badge ${e.action==="buy"?"ok":e.action==="sell"?"err":"warn"}">${i(e.action)}</span></td>
      <td>${e.shares||"-"}</td>
      <td><span style="color:${c};font-weight:600">${r}%</span></td>
      <td style="font-size:11px;color:var(--muted)">${(e.reasons||[]).map(i).join(", ")}</td>
    </tr>`}).join("")}</tbody>
  </table></div>`}function $(t){let n=t&&t.exit_alerts;return!n||!n.length?d("\u76EE\u524D\u6C92\u6709\u9700\u8981\u51FA\u5834\u63D0\u9192\u7684\u6301\u5009"):`<div class="dc-section">${n.map(e=>{let r=e.pnl_pct>=0?"+":"";return`<div class="dc-exit-row">
      <span>\u{1F514} <strong>${i(e.symbol)}</strong> ${i(e.name!==e.symbol?e.name:"")}</span>
      <span class="badge ${e.pnl_pct>=10?"ok":e.pnl_pct<=-5?"err":"warn"}">
        ${r}${e.pnl_pct.toFixed(1)}%
      </span>
      <span style="font-size:11px;color:var(--muted);flex:1;text-align:right">${i(e.suggestion)}</span>
    </div>`}).join("")||d("\u76EE\u524D\u6C92\u6709\u9700\u8981\u51FA\u5834\u63D0\u9192\u7684\u6301\u5009")}</div>`}function f(t){return t==null||isNaN(t)?"-":(t>=0?"+":"")+t.toFixed(1)+"%"}function y(t){if(t==null)return"-";let n=Math.abs(t);return n>=1e8?(t/1e8).toFixed(1)+"\u5104":n>=1e4?(t/1e4).toFixed(1)+"\u842C":t.toFixed(0)}function w(t){if(!t)return"";let n=Date.now()-new Date(t).getTime(),o=Math.floor(n/6e4);if(o<60)return o+"\u5206\u9418\u524D";let e=Math.floor(o/60);return e<24?e+"\u5C0F\u6642\u524D":Math.floor(e/24)+"\u5929\u524D"}async function M(t){try{p=await u("/api/dashboard/decision-chain"),m=new Date().toLocaleTimeString("zh-TW"),_(t,p)}catch(n){console.error("Panel refresh failed:",n)}}function _(t,n){let o=document.querySelector("#dc-"+t+" .dc-panel-body");if(o)switch(t){case"events-radar":o.innerHTML=g(n);break;case"logic-rules":o.innerHTML=h(n);break;case"sector-heatmap":o.innerHTML=v(n);break;case"recommendations":o.innerHTML=x(n);break;case"exit-alerts":o.innerHTML=$(n);break}}export{j as loadDecisionChain,M as refreshPanel};
