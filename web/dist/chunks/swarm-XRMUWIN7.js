import{h as o}from"./chunk-35S32YAS.js";async function v(){let[t,a,i,n,e]=await Promise.all([fetch("/api/dashboard/swarm-status").then(r=>r.json()).catch(()=>null),fetch("/api/dashboard/swarm-consensus").then(r=>r.json()).catch(()=>null),fetch("/api/dashboard/swarm-anomalies").then(r=>r.json()).catch(()=>null),fetch("/api/dashboard/swarm-scenarios").then(r=>r.json()).catch(()=>null),fetch("/api/dashboard/swarm-strategies").then(r=>r.json()).catch(()=>null)]);p(t),h(a),u(i),m(n),y(e)}function p(t){let a=document.getElementById("swarm-status");if(!a)return;if(!t||t.error){a.innerHTML='<div class="empty" style="padding:24px;text-align:center;background:var(--panel-l2);border-radius:8px"><div style="font-size:32px;margin-bottom:8px">\u{1F41F}</div><div style="color:var(--text);font-weight:600;margin-bottom:4px">\u7B49\u5F85 Swarm \u6A21\u64EC\u8CC7\u6599</div><div style="color:var(--muted);font-size:13px">MiroFish Swarm \u6BCF 30 \u5206\u9418\u81EA\u52D5\u57F7\u884C\u4E00\u6B21\u80CC\u666F\u6A21\u64EC\u3002<br>\u9996\u6B21\u555F\u52D5\u5F8C\u8ACB\u7A0D\u5F85\uFF0C\u6216\u624B\u52D5\u89F8\u767C\u6A21\u64EC\u3002</div></div>';return}let i=t.total_fish!=null?t.total_fish:"\u2014",n=t.consensus_confidence!=null?(t.consensus_confidence*100).toFixed(1)+"%":"\u2014",e=t.top_accuracy!=null?(t.top_accuracy*100).toFixed(1)+"%":"\u2014",r=t.anomaly_count||0,s=r===0?"var(--color-success)":r<=3?"var(--color-warning)":"var(--color-danger)",d=t.recorded_at?new Date(t.recorded_at).toLocaleString():"\u2014",l=t.generations_evolved!=null?t.generations_evolved:"\u2014",c=t.training_scenarios!=null?t.training_scenarios:"\u2014";a.innerHTML=`
    <div class="kpi-card"><div class="kpi-label">\u9B5A\u7FA4\u6578\u91CF</div><div class="kpi-value">${i}</div></div>
    <div class="kpi-card"><div class="kpi-label">\u5171\u8B58\u4FE1\u5FC3\u5EA6</div><div class="kpi-value">${n}</div></div>
    <div class="kpi-card"><div class="kpi-label">\u6700\u4F73\u9B5A\u6E96\u78BA\u7387</div><div class="kpi-value">${e}</div></div>
    <div class="kpi-card" style="border-left:3px solid ${s}"><div class="kpi-label">\u7570\u5E38\u5075\u6E2C</div><div class="kpi-value" style="color:${s}">${r}</div></div>
    <div class="kpi-card"><div class="kpi-label">\u6700\u8FD1\u57F7\u884C</div><div class="kpi-value" style="font-size:14px">${o(d)}</div></div>
    <div class="kpi-card"><div class="kpi-label">\u6F14\u5316\u4E16\u4EE3</div><div class="kpi-value">${l}</div></div>
    <div class="kpi-card"><div class="kpi-label">\u8A13\u7DF4\u8CC7\u6599\u7B46\u6578</div><div class="kpi-value">${c}</div></div>
  `}function h(t){let a=document.getElementById("swarm-consensus");if(!a)return;if(!t||!Array.isArray(t)||t.length===0){a.innerHTML='<div class="empty" style="padding:20px;text-align:center;color:var(--muted)">\u5C1A\u7121\u5171\u8B58\u8CC7\u6599</div>';return}let i="";for(let n of t){let e=(n.consensus_direction||"neutral").toLowerCase(),r=e==="bullish"?"\u{1F4C8}":e==="bearish"?"\u{1F4C9}":"\u2194\uFE0F",s=e==="bullish"?"var(--color-success)":e==="bearish"?"var(--color-danger)":"var(--muted)",d=e==="bullish"?"\u770B\u591A":e==="bearish"?"\u770B\u7A7A":"\u4E2D\u7ACB",l=n.average_confidence!=null?(n.average_confidence*100).toFixed(1)+"%":"\u2014";i+=`<tr>
      <td style="font-family:var(--font-mono)">${o(n.symbol||"")}</td>
      <td><span style="color:${s};font-weight:600">${r} ${d}</span></td>
      <td>${l}</td>
      <td>${n.bullish_count||0}</td>
      <td>${n.bearish_count||0}</td>
      <td>${n.neutral_count||0}</td>
    </tr>`}a.innerHTML=`<div class="table-wrapper"><table><thead><tr><th>\u6A19\u7684</th><th>\u5171\u8B58\u65B9\u5411</th><th>\u4FE1\u5FC3\u5EA6</th><th>\u770B\u591A</th><th>\u770B\u7A7A</th><th>\u4E2D\u7ACB</th></tr></thead><tbody>${i}</tbody></table></div>`}function u(t){let a=document.getElementById("swarm-anomalies");if(!a)return;if(!t||!Array.isArray(t)||t.length===0){a.innerHTML='<div class="empty" style="padding:20px;text-align:center;color:var(--color-success)">\u2705 \u7121\u7570\u5E38\u5075\u6E2C</div>';return}let i="";for(let n of t){let e=n.severity||0,r=e>.7?"var(--color-danger)":e>.4?"var(--color-warning)":"var(--muted)",s=e>.7?"\u9AD8":e>.4?"\u4E2D":"\u4F4E";i+=`<div style="padding:10px 14px;border-left:3px solid ${r};margin-bottom:8px;background:var(--panel-l2);border-radius:6px">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="font-weight:600;color:${r}">${o(n.type||"Unknown")}</span>
        <span style="font-size:11px;color:var(--muted)">${s}</span>
      </div>
      <div style="margin-top:4px;font-size:13px;color:var(--text)">${o(n.description||"")}</div>
      <div style="margin-top:2px;font-size:11px;color:var(--muted)">\u5F71\u97FF: ${(n.symbols||[]).join(", ")||"\u2014"}</div>
    </div>`}a.innerHTML=i}function m(t){let a=document.getElementById("swarm-scenarios");if(!a)return;if(!t||!Array.isArray(t)||t.length===0){a.innerHTML='<div class="empty" style="padding:20px;text-align:center;color:var(--muted)">\u5C1A\u7121\u60C5\u5883\u8CC7\u6599</div>';return}let i={risk_on:"\u98A8\u96AA\u504F\u597D",risk_off:"\u98A8\u96AA\u8FF4\u907F",crisis:"\u5371\u6A5F",complacent:"\u81EA\u6EFF",transition:"\u8F49\u63DB"},n="";for(let e of t){let r=i[e.regime]||e.regime||"\u2014";n+=`<tr>
      <td style="font-weight:600">${o(e.name||"")}</td>
      <td><span class="badge">${o(r)}</span></td>
      <td>${e.volatility!=null?e.volatility.toFixed(4):"\u2014"}</td>
      <td>${e.trend!=null?e.trend.toFixed(6):"\u2014"}</td>
    </tr>`}a.innerHTML=`<div class="table-wrapper"><table><thead><tr><th>\u60C5\u5883</th><th>\u76E4\u52E2</th><th>\u6CE2\u52D5\u7387</th><th>\u8DA8\u52E2</th></tr></thead><tbody>${n}</tbody></table></div>`}function y(t){let a=document.getElementById("swarm-strategies");if(!a)return;if(!t||!Array.isArray(t)||t.length===0){a.innerHTML='<div class="empty" style="padding:20px;text-align:center;color:var(--muted)">\u5C1A\u7121\u7B56\u7565\u63A8\u85A6\u8CC7\u6599</div>';return}let i="";for(let n of t){let e=n.performance||{},r=e.success_rate!=null?(e.success_rate*100).toFixed(1)+"%":"\u2014",s=e.avg_improvement!=null?e.avg_improvement.toFixed(4):"\u2014",d=e.convergence_rate!=null?(e.convergence_rate*100).toFixed(1)+"%":"\u2014";i+=`<tr>
      <td style="font-weight:600">${o(n.name||"")}</td>
      <td><span class="badge">${o(n.type||"\u2014")}</span></td>
      <td>${n.score!=null?n.score.toFixed(4):"\u2014"}</td>
      <td>${r}</td>
      <td>${s}</td>
      <td>${d}</td>
    </tr>`}a.innerHTML=`<div class="table-wrapper"><table><thead><tr><th>\u7B56\u7565\u540D\u7A31</th><th>\u985E\u578B</th><th>\u5206\u6578</th><th>\u6210\u529F\u7387</th><th>\u5E73\u5747\u6539\u5584</th><th>\u6536\u6582\u7387</th></tr></thead><tbody>${i}</tbody></table></div>`}window.loadSwarmData=v;export{v as loadSwarmData};
