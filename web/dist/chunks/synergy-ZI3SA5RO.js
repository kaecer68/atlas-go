import{a as H}from"./chunk-DQKTUY5V.js";import"./chunk-GSJOUOZD.js";import"./chunk-LBHFRSEK.js";import"./chunk-RBRLONW2.js";import{h as d}from"./chunk-35S32YAS.js";var z={prompt_tightening:"\u7B56\u7565\u6536\u7DCA",prompt_relaxation:"\u7B56\u7565\u653E\u5BEC",constraint_tightening:"\u9650\u5236\u6536\u7DCA",constraint_relaxation:"\u9650\u5236\u653E\u5BEC",risk_rule_update:"\u98A8\u63A7\u66F4\u65B0",risk_rule_change:"\u98A8\u63A7\u8ABF\u6574",portfolio_constraint_revision:"\u6295\u7D44\u6CBB\u7406",portfolio_constraint:"\u6295\u7D44\u7D04\u675F",governance_routing:"\u6CBB\u7406\u8DEF\u7531",volume_filter:"\u6210\u4EA4\u91CF\u7BE9\u9078",conviction_adjustment:"\u4FE1\u5FF5\u8ABF\u6574",parameter_sweep:"\u53C3\u6578\u6383\u63CF",promote_spawned:"\u6649\u5347\u5019\u9078",onboarding:"\u65B0\u624B\u4E0A\u8DEF"};function A(t){return z[t]||t||""}function M(t,o){let e=H(o)||o||"";return{prompt_tightening:`${e} \u7684\u9078\u80A1\u689D\u4EF6\u5DF2\u88AB\u7CFB\u7D71\u81EA\u52D5\u6536\u7DCA\uFF0C\u4EE5\u63D0\u9AD8\u63A8\u85A6\u54C1\u8CEA\u3002`,prompt_relaxation:`${e} \u7684\u9078\u80A1\u689D\u4EF6\u5DF2\u88AB\u7CFB\u7D71\u81EA\u52D5\u653E\u5BEC\uFF0C\u4EE5\u589E\u52A0\u6A5F\u6703\u8986\u84CB\u3002`,risk_rule_change:`${e} \u7684\u98A8\u96AA\u95BE\u503C\u5DF2\u88AB\u7CFB\u7D71\u81EA\u52D5\u8ABF\u6574\uFF0C\u4EE5\u512A\u5316\u98A8\u96AA\u56DE\u5831\u3002`,risk_rule_update:`${e} \u7684\u98A8\u63A7\u898F\u5247\u5DF2\u88AB\u7CFB\u7D71\u66F4\u65B0\u3002`,portfolio_constraint_revision:`${e} \u7684\u6295\u7D44\u6CBB\u7406\u9650\u5236\u5DF2\u88AB\u91CD\u65B0\u5BE9\u8996\u3002`,portfolio_constraint:`${e} \u7684\u6295\u7D44\u90E8\u4F4D\u9650\u5236\u5DF2\u88AB\u8ABF\u6574\u3002`,governance_routing:`${e} \u7684\u57F7\u884C\u8DEF\u7531\u5DF2\u88AB\u8ABF\u6574\u3002`,volume_filter:`${e} \u7684\u6210\u4EA4\u91CF\u7BE9\u9078\u9580\u6ABB\u5DF2\u88AB\u8ABF\u6574\u3002`,conviction_adjustment:`${e} \u7684\u4FE1\u5FF5\u503C\u8A08\u7B97\u53C3\u6578\u5DF2\u88AB\u8ABF\u6574\u3002`,parameter_sweep:`${e} \u7684\u53C3\u6578\u5DF2\u88AB\u7CFB\u7D71\u6383\u63CF\u512A\u5316\u3002`,promote_spawned:`${e} \u65B0\u751F\u6210\u4EE3\u7406\u8868\u73FE\u512A\u7570\uFF0C\u5DF2\u88AB\u6649\u5347\u3002`,constraint_tightening:`${e} \u7684\u9650\u5236\u689D\u4EF6\u5DF2\u88AB\u6536\u7DCA\u3002`,constraint_relaxation:`${e} \u7684\u9650\u5236\u689D\u4EF6\u5DF2\u88AB\u653E\u5BEC\u3002`,onboarding:`${e} \u70BA\u7CFB\u7D71\u65B0\u52A0\u5165\u7684\u4EE3\u7406\uFF0C\u6B63\u5728\u9032\u884C\u9996\u6B21\u7B56\u7565\u8A55\u4F30\u3002`}[t]||`\u7CFB\u7D71\u5DF2\u5C0D ${e} \u9032\u884C\u81EA\u52D5\u512A\u5316\u8ABF\u6574\u3002`}function $(t){return t==null||isNaN(t)?"-":Math.abs(t)<.001?t.toExponential(2):t.toFixed(4)}function N(t){return t==null||isNaN(t)||t===0?"":"NT$"+Math.round(t).toLocaleString("zh-TW")}function j(t){switch(t){case"accepted":return'<span class="badge ok">\u5DF2\u63A5\u53D7</span>';case"rejected":return'<span class="badge err">\u5DF2\u62D2\u7D55</span>';case"expired":return'<span class="badge warn">\u5DF2\u904E\u671F</span>';case"running":return'<span class="badge info">\u57F7\u884C\u4E2D</span>';case"planned":return'<span class="badge info">\u5DF2\u898F\u5283</span>';default:return'<span class="badge warn">\u5F85\u8655\u7406</span>'}}function F(t,o,e){L(t,o),T(e)}function L(t,o){let e=document.getElementById("synergyLeaderboard");if(!e)return;let h=E(t);if(!h.length){e.className="empty",e.innerHTML='<div class="empty">\u5C1A\u7121 Darwinian \u6B0A\u91CD\u8CC7\u6599</div>';return}e.className="";let i={};if(o&&o.points&&o.points.length>0){let a={};for(let n of o.points)n.agent_id&&(a[n.agent_id]||(a[n.agent_id]=[]),a[n.agent_id].push(n.weight||1));for(let n in a){let s=a[n];if(s.length<2){i[n]="flat";continue}let r=s[0],c=s[1];r>c+.01?i[n]="up":r<c-.01?i[n]="down":i[n]="flat"}}let v=[...h].sort((a,n)=>n.weight-a.weight),g=t&&t.last_computed?t.last_computed:"",m=`
    ${g?`<div style="font-size:11px;color:var(--muted);margin-bottom:8px">\u6700\u5F8C\u8A08\u7B97\uFF1A${d(g)} \xB7 \u5171 ${h.length} \u500B Agent \xB7 \u6B0A\u91CD\u7BC4\u570D [0.3, 2.5]</div>`:""}
    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th width="40">\u6392\u540D</th>
            <th>Agent</th>
            <th>\u6B0A\u91CD</th>
            <th>Sharpe</th>
            <th>\u547D\u4E2D\u7387</th>
            <th>\u4FE1\u865F\u6578</th>
            <th>\u52DD/\u6557</th>
            <th>\u5747\u5831\u916C</th>
            <th>\u8DA8\u52E2</th>
            <th>\u72C0\u614B</th>
          </tr>
        </thead>
        <tbody>
  `;v.forEach((a,n)=>{let s=a.weight||1,r='<span class="badge ok">\u6B63\u5E38</span>';s>=2.5?r='<span class="badge info" title="\u5DF2\u9054\u6B0A\u91CD\u4E0A\u9650\uFF0C\u9AD8\u5F71\u97FF\u529B">\u6700\u5F37 Alpha</span>':s>2?r='<span class="badge info" title="\u9AD8\u5F71\u97FF\u529B">Alpha</span>':s<=.3?r='<span class="badge err" title="\u5DF2\u9054\u6B0A\u91CD\u4E0B\u9650\uFF0C\u9762\u81E8\u6DD8\u6C70">\u6DD8\u6C70\u908A\u7DE3</span>':s<.5&&(r='<span class="badge warn" title="\u4F4E\u5F71\u97FF\u529B\uFF0C\u53EF\u80FD\u88AB\u6DD8\u6C70">\u9AD8\u98A8\u96AA</span>');let c=i[a.agent_id]||"flat",f='<span class="text-muted">\u2192</span>';c==="up"&&(f='<span class="text-up">\u2191</span>'),c==="down"&&(f='<span class="text-down">\u2193</span>');let l=a.rolling_sharpe||0,y=a.total_signals||0,b=l>0?`<span style="color:var(--up)">${l.toFixed(2)}</span>`:l<0?`<span style="color:var(--down)">${l.toFixed(2)}</span>`:'<span class="text-muted">N/A</span>',p=a.hit_rate||0,x=p>=.6?`<span style="color:var(--up)">${fmtPct(p)}</span>`:p>=.4?fmtPct(p):`<span style="color:var(--down)">${fmtPct(p)}</span>`,u=a.avg_return||0,w=u>0?`<span style="color:var(--up)">${$(u)}</span>`:u<0?`<span style="color:var(--down)">${$(u)}</span>`:$(u);m+=`
      <tr>
        <td>#${n+1}</td>
        <td><strong>${d(H(a.agent_id))}</strong></td>
        <td>${s.toFixed(3)}</td>
        <td>${b}</td>
        <td>${x}</td>
        <td>${a.total_signals||0}</td>
        <td>${a.win_count||0}/${a.loss_count||0}</td>
        <td>${w}</td>
        <td>${f}</td>
        <td>${r}</td>
      </tr>
    `}),m+=`
        </tbody>
      </table>
    </div>
  `,e.innerHTML=m}function E(t){return!t||!t.agents?[]:Object.keys(t.agents).map(function(o){return Object.assign({agent_id:o},t.agents[o])})}function T(t){let o=document.getElementById("synergyInbox");if(!o)return;if(!t||!t.items||t.items.length===0){o.innerHTML='<div class="empty" style="grid-column:1/-1;text-align:center">\u76EE\u524D\u6C92\u6709\u65B0\u7684\u5BE6\u9A57\u5019\u9078\u8005</div>';return}let e=t.baseline_version?`<div style="font-size:11px;color:var(--muted);margin-bottom:4px">\u57FA\u7DDA\u7248\u672C\uFF1Av${t.baseline_version}</div>`:"",h='<div style="font-size:10px;color:var(--muted);margin-bottom:8px;line-height:1.4">\u7CFB\u7D71\u6BCF\u65E5\u81EA\u52D5\u9078\u51FA\u8868\u73FE\u6700\u5F31\u7684 Agent \u4F5C\u70BA\u5BE6\u9A57\u5019\u9078(planned)\u3002\u6BCF 7 \u5929\u57F7\u884C\u6E2C\u8A66\u5F8C\u66F4\u65B0\u7D50\u679C\u3002\u4E0B\u65B9\u70BA\u6BCF\u500B Agent \u7684\u6700\u65B0\u5019\u9078\u3002</div>',i=[],v=[],g=[];t.items.forEach(n=>{let s=n.status||"";s==="planned"||s==="running"?i.push(n):s==="accepted"||s==="rejected"?v.push(n):g.push(n)});let _=e+h;function m(n,s){return`<div style="grid-column:1/-1;font-weight:700;color:${s};margin-top:12px;font-size:13px;padding-bottom:4px;border-bottom:1px solid var(--border)">${n}</div>`}function a(n){let s=n.target_agent_id||"",r=n.mutation_type||"",c=n.mutation_summary||"",f=n.status||"",l=n.baseline_value,y=n.candidate_value,b="";if(f==="planned")b=`<div class="meta" style="font-size:11px">\u57FA\u7DDA SharpeLike ${$(l)} \u2192 \u5019\u9078 <span style="color:var(--warn)">\u5F85\u6E2C\u8A66</span></div>`;else if(l!=null&&y!=null&&(l!==0||y!==0)){let k=y>l;b=`<div class="meta" style="font-size:11px">\u57FA\u7DDA ${$(l)} \u2192 \u5019\u9078 <span style="color:${k?"var(--up)":"var(--down)"}">${$(y)}</span></div>`}let p=n.baseline_monetary_ntd,x=n.candidate_monetary_ntd,u="";p&&x&&(u=`<div class="meta" style="font-size:11px">\u57FA\u7DDA ${N(p)} \u2192 \u5019\u9078 ${N(x)}</div>`);let w="";f==="rejected"&&n.reject_reason&&(w=`<div style="font-size:10px;color:var(--down);margin-top:2px">\u62D2\u7D55\u539F\u56E0\uFF1A${d(n.reject_reason)}</div>`),_+=`
      <div class="inbox-card">
        <div class="title">${d(n.experiment_id)}</div>
        <div class="meta">${d(H(s))} ${j(f)}</div>
        <div class="meta"><strong>${d(A(r))}</strong></div>
        <div style="font-size:11px;color:var(--muted);margin:4px 0;line-height:1.5">${d(M(r,n.skill))}</div>
        ${b}
        ${u}
        ${c?`<div style="font-size:10px;color:var(--muted);margin-top:4px;word-break:break-all">${d(c)}</div>`:""}
        ${w}
      </div>
    `}i.length&&(_+=m(`\u{1F4CB} \u5F85\u6E2C\u8A66\uFF08${i.length}\uFF09`,"var(--warn)"),i.forEach(a)),v.length&&(_+=m(`\u2705 \u5DF2\u6E2C\u8A66\uFF08${v.length}\uFF09`,"var(--up)"),v.forEach(a)),g.length&&(_+=m(`\u{1F4C1} \u6B77\u53F2\u8A18\u9304\uFF08${g.length}\uFF09`,"var(--muted)"),g.forEach(a)),o.innerHTML=_}export{F as renderSynergyPage};
