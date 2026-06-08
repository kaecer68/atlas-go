import{a as C}from"./chunk-DLJZ2CKR.js";import"./chunk-35S32YAS.js";async function E(s,v){let i=await v("/api/dashboard/pnl-attribution").catch(()=>null);if(!i||!i.agent_attribution){s.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted)">\u66AB\u7121\u6B78\u56E0\u8CC7\u6599</div>';return}let n=window.fmtPct||(t=>(t*100).toFixed(2)+"%"),a=window.fmtFloat||(t=>t.toFixed(4)),r=(i.agent_attribution||[]).sort((t,e)=>(e.avg_return||0)-(t.avg_return||0)).map(t=>{let p={macro:"#5b9bd5",sector:"#70ad47",style:"#ed7d31"}[t.layer]||"var(--muted)";return`<tr><td>${t.agent_name||t.agent_id}</td><td style="color:${p}">${t.layer||"-"}</td><td style="text-align:right">${n(t.avg_return||0)}</td><td style="text-align:right">${t.count||0}</td></tr>`}).join(""),h=(i.sector_attribution||[]).sort((t,e)=>(e.avg_return||0)-(t.avg_return||0)).map(t=>`<tr><td>${t.sector_label||t.sector}</td><td style="text-align:right">${n(t.avg_return||0)}</td><td style="text-align:right">${t.count||0}</td></tr>`).join(""),m=i.factor_attribution||{},c=["momentum","value","quality","agent"].map(t=>{let e=m[t]||{};return`<tr><td style="font-weight:600">${t}</td><td style="text-align:right">${a(e.avg_score||0)}</td><td style="text-align:right">${n(e.avg_return||0)}</td><td style="text-align:right">${a(e.contribution||0)}</td></tr>`}).join("");s.innerHTML=`
    <div class="panel-content">
      <div class="section-title">Agent \u8CA2\u737B</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>\u7B56\u7565\u4F86\u6E90</th><th>\u5C64\u7D1A</th><th>\u5E73\u5747\u5831\u916C</th><th>\u6B21\u6578</th></tr></thead><tbody>${r}</tbody></table></div>
      <div class="section-title" style="margin-top:16px">\u7522\u696D\u8CA2\u737B</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>\u7522\u696D</th><th>\u5E73\u5747\u5831\u916C</th><th>\u6B21\u6578</th></tr></thead><tbody>${h}</tbody></table></div>
      <div class="section-title" style="margin-top:16px">\u56E0\u5B50\u8CA2\u737B</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>\u56E0\u5B50</th><th>\u5E73\u5747\u5206\u6578</th><th>\u5E73\u5747\u5831\u916C</th><th>\u8CA2\u737B\u5EA6</th></tr></thead><tbody>${c}</tbody></table></div>
    </div>`}async function R(s,v){let i=await v("/api/dashboard/benchmark-comparison").catch(()=>null);if(!i||i.session_count<1){s.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted)">\u66AB\u7121\u57FA\u6E96\u6BD4\u8F03\u8CC7\u6599</div>';return}let n=window.fmtNTD||(e=>e.toFixed(0)),a=window.fmtPct||(e=>(e*100).toFixed(2)+"%"),r=window.fmtFloat||(e=>e.toFixed(3)),m=[{label:"\u6295\u7D44\u7D2F\u7A4D\u5831\u916C",value:a(i.portfolio_return||0)},{label:"TAIEX \u5831\u916C",value:a(i.taiex_return||0)},{label:"\u8D85\u984D\u5831\u916C",value:a(i.outperformance||0),cls:i.outperformance>0?"text-up":"text-down"},{label:"Alpha",value:a(i.alpha||0),cls:i.alpha>0?"text-up":"text-down"},{label:"Beta",value:r(i.beta||0)},{label:"Tracking Error",value:a(i.tracking_error||0)},{label:"Sharpe Ratio",value:r(i.sharpe_ratio||0)},{label:"Info Ratio",value:r(i.info_ratio||0)}].map(e=>`<div class="kpi-card"><div class="kpi-label">${e.label}</div><div class="kpi-value ${e.cls||""}">${e.value}</div></div>`).join(""),t=(i.equity_curve||[]).map(e=>{let p=(e.outperf||0)>0?"text-up":"text-down";return`<tr><td>${e.label}</td><td style="text-align:right">${a(e.portfolio||0)}</td><td style="text-align:right">${a(e.benchmark||0)}</td><td style="text-align:right" class="${p}">${e.outperf>0?"+":""}${a(e.outperf||0)}</td></tr>`}).join("");s.innerHTML=`
    <div class="panel-content">
      <div class="section-title">\u57FA\u6E96\u6BD4\u8F03\u6307\u6A19</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">${m}</div>
      <div class="section-title" style="margin-top:16px">\u6B0A\u76CA\u66F2\u7DDA\uFF1A\u6295\u7D44 vs TAIEX</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>\u65E5\u671F</th><th>\u6295\u7D44</th><th>TAIEX</th><th>\u5DEE\u984D</th></tr></thead><tbody>${t}</tbody></table></div>
    </div>`}async function B(s,v){let i=await v("/api/dashboard/portfolio-state").catch(()=>({})),n=await v("/api/dashboard/correlation-matrix").catch(()=>null),a=window.fmtPct||(c=>(c*100).toFixed(2)+"%"),r=i.current_drawdown||0,h=i.concentration_ratio||0,m="";if(n&&n.matrix&&n.matrix.length>0){let c=n.labels||n.symbols||[],t="<th></th>"+c.map(p=>`<th class="corr-header">${p}</th>`).join(""),e=n.matrix.map((p,w)=>{let f=p.map((g,y)=>{let x="inherit";return w===y?x="#666":g>.7?x="var(--down)":g>.4?x="var(--warn)":g<0&&(x="var(--up)"),`<td class="corr-cell" style="color:${x}">${g.toFixed(2)}</td>`}).join("");return`<tr><td class="corr-header">${c[w]}</td>${f}</tr>`}).join("");m=`<div class="section-title">\u76F8\u95DC\u6027\u77E9\u9663</div><div class="corr-matrix-container"><table class="corr-matrix"><thead><tr>${t}</tr></thead><tbody>${e}</tbody></table></div>`}s.innerHTML=`
    <div class="panel-content">
      <div class="section-title">\u98A8\u96AA\u6307\u6A19</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">
        <div class="kpi-card"><div class="kpi-label">\u6700\u5927\u56DE\u64A4</div><div class="kpi-value text-down">${a(r)}</div></div>
        <div class="kpi-card"><div class="kpi-label">\u6301\u5009\u96C6\u4E2D\u5EA6</div><div class="kpi-value">${a(h)}</div><div class="kpi-hint">HHI \u6307\u6578</div></div>
        <div class="kpi-card"><div class="kpi-label">\u90E8\u4F4D\u6578</div><div class="kpi-value">${i.positions_count||0}</div></div>
        <div class="kpi-card"><div class="kpi-label">\u69D3\u687F\u7387</div><div class="kpi-value">${a((i.portfolio_value||0)>0?(i.portfolio_value-i.cash)/i.cash:0)}</div></div>
      </div>
      ${m}
    </div>`}function A(s,v){s.innerHTML='<div class="loading">\u8F09\u5165\u98A8\u63A7\u72C0\u614B\u2026</div>',v("/api/dashboard/risk").then(i=>{if(!i||i.message){s.innerHTML='<div class="empty-state">\u5C1A\u7121\u98A8\u96AA\u6578\u64DA</div>';return}let n=i.var_95??"-",a=i.max_drawdown??"-",r=i.concentration_score??"-",h=i.position_count??0;s.innerHTML=`
        <div class="risk-gate-grid" style="display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:12px">
          <div class="risk-gate-metric">
            <span class="metric-label">VaR 95%</span>
            <span class="metric-value ${n<-.05?"critical":"normal"}">${typeof n=="number"?(n*100).toFixed(1)+"%":n}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">\u6700\u5927\u56DE\u64A4</span>
            <span class="metric-value ${a>.15?"critical":a>.08?"warning":"normal"}">${typeof a=="number"?(a*100).toFixed(1)+"%":a}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">\u96C6\u4E2D\u5EA6 (HHI)</span>
            <span class="metric-value ${r>.25?"warning":"normal"}">${typeof r=="number"?(r*100).toFixed(1)+"%":r}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">\u6301\u5009\u6578</span>
            <span class="metric-value">${h}</span>
          </div>
        </div>
        <div style="padding:8px 12px;font-size:12px;color:#888;border-top:1px solid #eee">
          \u98A8\u63A7\u9598\u9053\u6A21\u5F0F\uFF1A<span class="risk-gate-mode">NORMAL</span>
        </div>
      `}).catch(()=>{s.innerHTML='<div class="error">\u7121\u6CD5\u8F09\u5165\u98A8\u63A7\u6578\u64DA</div>'})}async function S(s,v){let i=document.getElementById("portfolioKPIs"),n=document.getElementById("positionsTable"),a=document.getElementById("tradeHistoryContainer");if(!(!i||!n||!a)){i.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted)">\u8CC7\u6599\u8F09\u5165\u4E2D\u2026</div>',n.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted)">\u8CC7\u6599\u8F09\u5165\u4E2D\u2026</div>',a.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted)">\u8CC7\u6599\u8F09\u5165\u4E2D\u2026</div>';try{let[r,h,m,c]=await Promise.all([s("/api/dashboard/live-status").catch(()=>({})),s("/api/dashboard/portfolio-state").catch(()=>({})),s("/api/dashboard/tax-snapshot").catch(()=>({})),s("/api/dashboard/trade-history").catch(()=>[])]),t=h||{},e=t.positions||[],p=m||{},w=Array.isArray(c)?c:[],f=p.total_tax_paid||0,g=(t.portfolio_value||0)-f,y=t.realized_pnl||0,x=t.trade_count||w.length,b=t.unrealized_pnl_total||0,_=t.concentration_ratio||0,T=t.current_drawdown||0,j={semiconductor:"\u534A\u5C0E\u9AD4",ai_supply_chain:"AI\u4F9B\u61C9\u93C8",robotics:"\u6A5F\u5668\u4EBA",financials:"\u91D1\u878D",shipping:"\u822A\u904B",energy:"\u80FD\u6E90",electronics:"\u96FB\u5B50",consumer:"\u6D88\u8CBB",industrial:"\u5DE5\u696D",other:"\u5176\u4ED6"};i.innerHTML=`
      <div class="kpi-card">
        <div class="kpi-label">\u7A05\u524D\u6DE8\u503C</div>
        <div class="kpi-value">${window.fmtNTD?window.fmtNTD(t.portfolio_value||0):(t.portfolio_value||0).toFixed(0)}</div>
        <div class="kpi-hint">\u53EF\u7528\u73FE\u91D1: ${window.fmtNTD?window.fmtNTD(t.cash||0):(t.cash||0).toFixed(0)}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u7A05\u5F8C\u6DE8\u503C</div>
        <div class="kpi-value">${window.fmtNTD?window.fmtNTD(g):g.toFixed(0)}</div>
        <div class="kpi-hint">\u5DF2\u6263\u9664\u7D2F\u7A4D\u7A05\u8CA0</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u5DF2\u5BE6\u73FE\u640D\u76CA</div>
        <div class="kpi-value ${y>0?"text-up":y<0?"text-down":""}">${window.fmtNTD?window.fmtNTD(y):y.toFixed(0)}</div>
        <div class="kpi-hint">\u6A21\u64EC\u7D2F\u7A4D\u5DF2\u5E73\u5009\u640D\u76CA</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u7D2F\u7A4D\u4EA4\u6613\u6578</div>
        <div class="kpi-value">${x}</div>
        <div class="kpi-hint">\u4EA4\u6613\u6B77\u53F2\u7E3D\u7B46\u6578</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u7D2F\u7A4D\u7A05\u8CA0</div>
        <div class="kpi-value text-down">${window.fmtNTD?window.fmtNTD(f):f.toFixed(0)}</div>
        <div class="kpi-hint">\u6301\u5009\u6A94\u6578: ${e.length} | \u66F4\u65B0: ${t.snapshot_time?new Date(t.snapshot_time).toLocaleTimeString():"-"}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u672A\u5BE6\u73FE\u640D\u76CA</div>
        <div class="kpi-value ${b>0?"text-up":b<0?"text-down":""}">${window.fmtNTD?window.fmtNTD(b):b.toFixed(0)}</div>
        <div class="kpi-hint">\u6301\u5009\u672A\u5BE6\u73FE\u7E3D\u640D\u76CA</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u6301\u5009\u96C6\u4E2D\u5EA6 (HHI)</div>
        <div class="kpi-value">${window.fmtPct?window.fmtPct(_):(_*100).toFixed(2)+"%"}</div>
        <div class="kpi-hint">0~1\uFF0C\u8D8A\u9AD8\u8D8A\u96C6\u4E2D</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">\u6700\u5927\u56DE\u64A4</div>
        <div class="kpi-value text-down">${window.fmtPct?window.fmtPct(T):(T*100).toFixed(2)+"%"}</div>
        <div class="kpi-hint">\u6B77\u53F2\u6700\u5927\u56DE\u64A4</div>
      </div>
    `;let F=t.equity_curve||[],N=F.map(o=>({label:o.label,value:o.value})),q=F.filter(o=>o.after_tax_value!==void 0).map(o=>({label:o.label,value:o.after_tax_value}));if(C(N,q),!e.length)n.innerHTML=window.emptyState?window.emptyState("\u5C1A\u7121\u6301\u5009\u8CC7\u6599",""):'<div style="padding:20px;text-align:center;color:var(--muted)">\u5C1A\u7121\u6301\u5009\u8CC7\u6599</div>';else{let o=window.fmtFloat||(l=>l.toFixed(2)),k=window.fmtInt||(l=>l.toString()),d=window.fmtPct||(l=>(l*100).toFixed(2)+"%"),$=e.map(l=>{let u=l.unrealized_pnl||0,M=l.pnl_pct||0,z=(l.average_cost||0)*(l.quantity||0),I=window.pnlColor?window.pnlColor(u):u>0?"text-up":u<0?"text-down":"";return`
          <tr>
            <td style="font-weight:600">${l.symbol}</td>
            <td>${j[l.sector]||l.sector||"\u2014"}</td>
            <td style="text-align:right">${k(l.quantity)}</td>
            <td style="text-align:right">${o(l.average_cost)}</td>
            <td style="text-align:right">${o(z)}</td>
            <td style="text-align:right">${o(l.current_price)}</td>
            <td style="text-align:right">${o(l.market_value)}</td>
            <td style="text-align:right" class="${I}">${u>0?"+":""}${o(u)}</td>
            <td style="text-align:right" class="${I}">${u>0?"+":""}${d(M)}</td>
          </tr>
        `}).join("");n.innerHTML=`
        <div class="table-wrapper">
          <table class="text-sm">
            <thead>
              <tr>
                <th style="text-align:left">\u6A19\u7684</th>
                <th style="text-align:left">\u7522\u696D\u677F\u584A</th>
                <th style="text-align:right">\u6578\u91CF (\u80A1)</th>
                <th style="text-align:right">\u5E73\u5747\u6210\u672C</th>
                <th style="text-align:right">\u6301\u5009\u6210\u672C</th>
                <th style="text-align:right">\u73FE\u50F9</th>
                <th style="text-align:right">\u5E02\u503C</th>
                <th style="text-align:right">\u672A\u5BE6\u73FE\u640D\u76CA</th>
                <th style="text-align:right">\u640D\u76CA\u7387</th>
              </tr>
            </thead>
            <tbody>${$}</tbody>
          </table>
        </div>
      `}if(!w.length)a.innerHTML='<div style="padding:20px;text-align:center;color:var(--muted)">\u5C1A\u7121\u4EA4\u6613\u6B77\u53F2</div>';else{let o=window.fmtInt||(d=>d.toString()),k=w.map(d=>{let $=d.amount??(d.quantity||0)*(d.price||0),l=d.side==="BUY"?"text-up":"text-down",u=d.side==="BUY"?"\u8CB7\u5165":"\u8CE3\u51FA";return`
          <tr>
            <td>${d.timestamp?new Date(d.timestamp).toLocaleString():"\u2014"}</td>
            <td style="font-weight:600">${d.symbol||"\u2014"}</td>
            <td class="${l}">${u}</td>
            <td style="text-align:right">${o(d.quantity||0)}</td>
            <td style="text-align:right">${window.fmtFloat?window.fmtFloat(d.price||0):(d.price||0).toFixed(2)}</td>
            <td style="text-align:right">${window.fmtNTD?window.fmtNTD($):$.toFixed(0)}</td>
            <td>${d.reason||"\u2014"}</td>
          </tr>
        `}).join("");a.innerHTML=`
        <div class="table-wrapper">
          <table class="text-sm">
            <thead>
              <tr>
                <th style="text-align:left">\u6642\u9593</th>
                <th style="text-align:left">\u6A19\u7684</th>
                <th style="text-align:left">\u65B9\u5411</th>
                <th style="text-align:right">\u6578\u91CF</th>
                <th style="text-align:right">\u6210\u4EA4\u50F9</th>
                <th style="text-align:right">\u6210\u4EA4\u91D1\u984D</th>
                <th style="text-align:left">\u539F\u56E0</th>
              </tr>
            </thead>
            <tbody>${k}</tbody>
          </table>
        </div>
      `}let H=document.getElementById("pnlAttribution");H&&E(H,s);let L=document.getElementById("benchmarkComparison");L&&R(L,s);let P=document.getElementById("riskPanel");P&&B(P,s);let D=document.getElementById("riskGatePanel");D&&A(D,s)}catch(r){console.error(r),i.innerHTML='<div style="padding:20px;text-align:center;color:var(--down)">\u8F09\u5165\u5931\u6557</div>',n.innerHTML='<div style="padding:20px;text-align:center;color:var(--down)">\u8F09\u5165\u5931\u6557</div>',a.innerHTML='<div style="padding:20px;text-align:center;color:var(--down)">\u8F09\u5165\u5931\u6557</div>'}}}export{S as loadPortfolioPage};
