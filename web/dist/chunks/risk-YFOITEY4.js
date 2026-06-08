import{m as M}from"./chunk-OU7I3ZGZ.js";import{h as b}from"./chunk-35S32YAS.js";function R(a){let n=document.getElementById("liveStatus");if(n.classList.remove("loading"),!a||!a.circuit_breaker){n.innerHTML='<div class="empty">\u5373\u6642\u72C0\u614B\u66AB\u7121\u8CC7\u6599</div>';return}let s=a.circuit_breaker,i=a.portfolio||{},o=i.unrealized_pnl||0,r=i.day_pnl||0,y=o>=0?"up":"down",t=r>=0?"up":"down",x=s.state==="tripped"?"\u5DF2\u89F8\u767C":"\u6B63\u5E38",c=s.state==="tripped"?"err":"ok",e="";if(s.cooldown_until&&s.cooldown_until!=="0001-01-01T00:00:00Z"){let g=new Date(s.cooldown_until),p=new Date;g>p&&(e=`<div class="metric"><div class="label">\u51B7\u537B\u4E2D</div><div class="value warn">${Math.ceil((g-p)/6e4)} \u5206\u9418</div></div>`)}let d="";s.consecutive_sl>0&&(d=`<div class="metric"><div class="label">\u9023\u7E8C\u6B62\u640D</div><div class="value ${s.consecutive_sl>=3?"err":"warn"}">${s.consecutive_sl} \u6B21</div></div>`),n.innerHTML=`
    <div class="metric"><div class="label">\u7194\u65B7\u6A5F\u5236</div><div class="value ${c}">${x}</div></div>
    <div class="metric"><div class="label">\u73FE\u91D1</div><div class="value">${(i.cash||0).toLocaleString()}</div></div>
    <div class="metric"><div class="label">\u6301\u5009\u5E02\u503C</div><div class="value">${(i.total_exposure||0).toLocaleString()}</div></div>
    <div class="metric"><div class="label">\u6301\u5009\u6578</div><div class="value">${i.positions_count||0}</div></div>
    <div class="metric"><div class="label">\u672A\u5BE6\u73FE\u640D\u76CA</div><div class="value ${y}">${o.toLocaleString()}</div></div>
    <div class="metric"><div class="label">\u7576\u65E5\u640D\u76CA</div><div class="value ${t}">${r.toLocaleString()}</div></div>
    ${d}
    ${e}
  `}function W(a,n,s){let i=document.getElementById("riskCards"),o=document.getElementById("riskCardsPanel"),r=document.getElementById("riskPositionConcentration"),y=document.getElementById("riskSectorDistribution");if(!i||!a){o&&(o.style.display="none");return}o.style.display="",o.classList.remove("loading"),i.classList.remove("loading");let t=a,x=t.insufficient_data||t.data_points<30,c=l=>typeof l=="number"&&!isNaN(l)?(l*100).toFixed(1)+"%":"\u2014",e=s||{},g={advance:"\u{1F680} \u63A8\u9032",reduce:"\u{1F53B} \u7E2E\u6E1B",standby:"\u23F8\uFE0F \u89C0\u671B"}[e.phase]||e.phase||"\u2014",p=e.rolling_sharpe!=null&&!isNaN(e.rolling_sharpe)?e.rolling_sharpe.toFixed(2):null,$=e.consecutive_losses||0,C=e.days_in_phase||0,I=e.can_advance,k="",m=t.concentration||[];if(m.length>0){let l=m.reduce((v,h)=>v+(h.weight||0),0),_=m.length>0&&m[0].weight||0,u=m.slice(0,3).reduce((v,h)=>v+(h.weight||0),0),f=m.map((v,h)=>{let S=((v.weight||0)*100).toFixed(1);return`<tr><td style="padding:3px 8px;font-size:12px">${h+1}</td><td style="padding:3px 8px;font-size:12px">${b(v.symbol)}</td><td style="padding:3px 8px;font-size:12px;text-align:right">${S}%</td><td style="padding:3px 8px;font-size:12px;text-align:right">${(v.market_value||0).toLocaleString()}</td></tr>`}).join("");k=`
      <div style="display:flex;gap:16px;flex-wrap:wrap;margin-top:12px">
        <div style="flex:1;min-width:180px">
          <div style="font-size:12px;color:var(--muted);margin-bottom:6px">\u6301\u5009\u96C6\u4E2D\u5EA6\uFF08\u5E02\u503C\uFF09</div>
          <div style="font-size:20px;font-weight:700;color:${l>.6?"var(--down)":l>.4?"var(--warn)":"var(--up)"}">${(l*100).toFixed(1)}%</div>
          <div style="font-size:11px;color:var(--muted);margin-top:4px">\u524D 3 \u5927 ${(u*100).toFixed(1)}% \xB7 \u6700\u5927 ${(_*100).toFixed(1)}%</div>
        </div>
        <div style="flex:2;min-width:300px">
          <table style="width:100%;font-size:12px;border-collapse:collapse">
            <thead><tr style="border-bottom:1px solid var(--border)"><th style="text-align:left;padding:4px 8px">#</th><th style="text-align:left;padding:4px 8px">\u6A19\u7684</th><th style="text-align:right;padding:4px 8px">\u6B0A\u91CD</th><th style="text-align:right;padding:4px 8px">\u5E02\u503C</th></tr></thead>
            <tbody>${f}</tbody>
          </table>
        </div>
      </div>
    `}else k='<div style="font-size:12px;color:var(--muted);margin-top:12px">\u66AB\u7121\u6301\u5009\u8CC7\u6599</div>';let z="",L=(t.sector_exposure||[]).filter(l=>l.weight>0).sort((l,_)=>(_.weight||0)-(l.weight||0));if(L.length>0){let l=Math.max(...L.map(u=>u.weight||0),.01);z=`
      <div style="margin-top:16px">
        <div style="font-size:13px;font-weight:700;margin-bottom:8px">\u677F\u584A\u66DD\u96AA\u5206\u5E03\uFF08\u5E02\u503C\u6B0A\u91CD\uFF09</div>
        ${L.map(u=>{let f=u.weight||0,v=(f*100).toFixed(1),h=(f/l*100).toFixed(1),S=f>.3?"var(--accent)":f>.15?"var(--warn)":"var(--muted)";return`
        <div style="margin:4px 0">
          <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">
            <span>${b(M(u.sector)||u.sector)}</span>
            <span>${v}%</span>
          </div>
          <div style="width:100%;height:6px;background:var(--bg);border-radius:3px;overflow:hidden">
            <div style="width:${h}%;height:100%;background:${S};border-radius:3px;transition:width 0.3s"></div>
          </div>
        </div>
      `}).join("")}
      </div>
    `}else z='<div style="font-size:12px;color:var(--muted);margin-top:16px">\u66AB\u7121\u677F\u584A\u66DD\u96AA\u8CC7\u6599</div>';let F=t.cash_ratio!=null?(t.cash_ratio*100).toFixed(1):null,T=t.portfolio_value?t.portfolio_value.toLocaleString():"\u2014",B=e.deployed_capital||0,w=e.total_capital||0,H=w>0?(B/w*100).toFixed(1):null;i.innerHTML=`
    <div class="panel" style="text-align:center">
      <div class="kpi-label">VaR 95%</div>
      <div class="kpi-value" style="color:var(--down)">${x?"\u8CC7\u6599\u4E0D\u8DB3":c(t.var_95)}</div>
      <div class="kpi-hint">\u65E5\u983B \xB7 95% \u4FE1\u8CF4\u6C34\u6E96</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">VaR 99%</div>
      <div class="kpi-value" style="color:var(--down)">${x?"\u8CC7\u6599\u4E0D\u8DB3":c(t.var_99)}</div>
      <div class="kpi-hint">\u65E5\u983B \xB7 \u6975\u7AEF\u4E8B\u4EF6\u58D3\u529B</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">CVaR 95%</div>
      <div class="kpi-value" style="color:var(--down)">${x?"\u8CC7\u6599\u4E0D\u8DB3":c(t.cvar_95)}</div>
      <div class="kpi-hint">95% \u689D\u4EF6\u671F\u671B\u8667\u640D</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">\u6700\u5927\u56DE\u64A4</div>
      <div class="kpi-value" style="color:var(--warn)">${x?"\u8CC7\u6599\u4E0D\u8DB3":c(t.max_drawdown_pct)}</div>
      <div class="kpi-hint">\u6B77\u53F2\u5CF0\u503C\u56DE\u64A4\u5E45\u5EA6</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">Rolling Sharpe</div>
      <div class="kpi-value" style="color:${p!==null?p>.5?"var(--up)":p<0?"var(--down)":"var(--warn)":"var(--muted)"}">${p!==null?p:"\u2014"}</div>
      <div class="kpi-hint">${p!==null?"\u98A8\u96AA\u8ABF\u6574\u5F8C\u6536\u76CA":"\u5C1A\u7121\u8CC7\u91D1\u968E\u6BB5\u8CC7\u6599"}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">\u6295\u7D44\u6DE8\u503C</div>
      <div class="kpi-value">${T}</div>
      <div class="kpi-hint">${F!==null?"\u73FE\u91D1 "+F+"%":""}${H!==null?" \xB7 \u66DD\u96AA "+H+"%":""}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">\u8CC7\u91D1\u968E\u6BB5</div>
      <div class="kpi-value" style="font-size:16px">${g}</div>
      <div class="kpi-hint">${C>0?"\u6301\u7E8C "+C+" \u5929":""}${$>0?" \xB7 \u9023\u7E8C\u8667\u640D "+$+" \u6B21":""}${I?" \xB7 \u53EF\u63A8\u9032":""}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">\u6301\u5009\u6578</div>
      <div class="kpi-value">${t.position_count||0}</div>
      <div class="kpi-hint">${t.data_points>=30?"\u8CC7\u6599\u9EDE "+t.data_points+" \xB7 \u53EF\u4FE1":"\u8CC7\u6599\u9EDE "+(t.data_points||0)+" \xB7 \u7D71\u8A08\u4E0D\u8DB3"}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">\u4FDD\u7559\u73FE\u91D1</div>
      <div class="kpi-value">${e.reserve_cash?e.reserve_cash.toLocaleString():"\u2014"}</div>
      <div class="kpi-hint">\u7E3D\u8CC7\u672C ${w?w.toLocaleString():"\u2014"}</div>
    </div>
  `,r&&(r.innerHTML=`
    <div style="border-top:1px solid var(--border);padding-top:12px">
      <div style="font-size:13px;font-weight:700;margin-bottom:6px">\u5009\u4F4D\u96C6\u4E2D\u5EA6\u5206\u6790</div>
      ${k}
    </div>
  `),y&&(y.innerHTML=z)}function N(a){var n=document.getElementById("riskCalibration"),s=document.getElementById("riskCalibrationPanel");if(!(!n||!s)){if(!a||a.status==="not_available"||!a.report){s.style.display="",n.classList.remove("loading"),n.innerHTML='<div class="empty">\u5C1A\u7121\u6821\u6E96\u5831\u544A</div>';return}s.style.display="",n.classList.remove("loading");var i=a.report,o=a.generated||"",r=i.verdict==="calibrated",y=r?"\u{1F535}":"\u{1F7E2}",t=r?"\u5DF2\u6821\u6E96":"\u95BE\u503C\u7A69\u5B9A",x=r?"#3b82f6":"#10b981",c="";if(i.changes&&i.changes.length>0){var e=i.changes.map(function(d){var g=d.confidence==="high"?"var(--up)":d.confidence==="medium"?"var(--warn)":"var(--muted)";return'<tr><td style="padding:4px 8px;font-size:12px;font-family:monospace">'+b(d.name)+'</td><td style="padding:4px 8px;font-size:12px;text-align:right">'+d.before.toFixed(4)+'</td><td style="padding:4px 8px;font-size:12px;text-align:right;color:var(--up)">'+d.after.toFixed(4)+'</td><td style="padding:4px 8px;font-size:12px;color:var(--muted)">'+b(d.rationale)+'</td><td style="padding:4px 8px;font-size:12px;text-align:center"><span style="padding:1px 6px;border-radius:3px;font-size:11px;background:'+g+"22;color:"+g+'">'+d.confidence+"</span></td></tr>"}).join("");c='<div style="margin-top:12px"><div style="font-size:13px;font-weight:700;margin-bottom:6px">\u53C3\u6578\u8ABF\u6574</div><table style="width:100%;font-size:12px;border-collapse:collapse"><thead><tr style="border-bottom:1px solid var(--border)"><th style="text-align:left;padding:4px 8px">\u53C3\u6578</th><th style="text-align:right;padding:4px 8px">\u8ABF\u6574\u524D</th><th style="text-align:right;padding:4px 8px">\u8ABF\u6574\u5F8C</th><th style="text-align:left;padding:4px 8px">\u539F\u56E0</th><th style="text-align:center;padding:4px 8px">\u4FE1\u5FC3</th></tr></thead><tbody>'+e+"</tbody></table></div>"}n.innerHTML='<div style="display:flex;gap:12px;align-items:center;flex-wrap:wrap"><span style="font-size:28px">'+y+'</span><div><div style="font-size:14px;font-weight:700;color:'+x+'">'+t+'</div><div style="font-size:11px;color:var(--muted)">'+(o?"\u6821\u6E96\u6642\u9593 "+new Date(o).toLocaleString("zh-TW"):"")+'</div></div><div style="margin-left:auto;text-align:right;font-size:12px;color:var(--muted)"><div>\u8A55\u4F30 '+(i.orders_evaluated||0)+" \u7B46\u8A02\u55AE</div><div>\u5340\u9593 "+(i.session_span||"\u2014")+'</div></div></div><div style="margin-top:10px;font-size:13px;padding:8px 12px;background:var(--panel-l2);border-radius:6px;color:var(--text);line-height:1.5">'+b(i.summary||"")+"</div>"+c}}function j(a,n){return{semiconductor:"semiconductor",ai_supply_chain:"ai_supply_chain",financials:"financials",shipping:"shipping",value_yield:"high_dividend",etf_rotation:"etf_rotation",technical_breakout:"small_cap",growth_momentum:"small_cap",macro:"TAIEX",cro:"control",cio:"control"}[a]||(n==="sector"?a:null)}export{j as inferSectorFromAgent,R as renderLiveStatus,N as renderRiskCalibration,W as renderRiskCards};
