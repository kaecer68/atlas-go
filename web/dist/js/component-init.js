import{a as S}from"../chunks/chunk-LBHFRSEK.js";var y=class{constructor(t){this.container=document.getElementById(t),this.container&&(this.statusBadge=this.container.querySelector("#cbStatusBadge"),this.statusDot=this.container.querySelector("#cbStatusDot"),this.statusText=this.container.querySelector("#cbStatusText"),this.intradayPeak=this.container.querySelector("#cbIntradayPeak"),this.consecutiveSL=this.container.querySelector("#cbConsecutiveSL"),this.cooldown=this.container.querySelector("#cbCooldown"),this.eventList=this.container.querySelector("#cbEventList"),this.resetBtn=this.container.querySelector("#cbResetBtn"),this.events=[],this.bindEvents(),this.fetchState())}bindEvents(){this.resetBtn&&this.resetBtn.addEventListener("click",()=>this.handleReset())}async fetchState(){try{let t=await fetch("/api/dashboard/circuit-breaker");if(t.ok){let e=await t.json();this.updateUI(e)}else console.warn("Circuit breaker API returned status:",t.status),this.showEmptyState()}catch(t){console.error("Failed to fetch circuit breaker state:",t),this.showEmptyState()}}showEmptyState(){this.statusText&&(this.statusText.textContent="\u672A\u9023\u7DDA"),this.statusDot&&(this.statusDot.className="cb-status-dot",this.statusDot.classList.add("unknown")),this.intradayPeak&&(this.intradayPeak.textContent="-"),this.consecutiveSL&&(this.consecutiveSL.textContent="-"),this.cooldown&&(this.cooldown.textContent="-"),this.eventList&&(this.eventList.innerHTML='<li class="cb-event-item empty text-center" style="text-align: center;">\u66AB\u7121\u4E8B\u4EF6</li>'),this.resetBtn&&(this.resetBtn.disabled=!0)}showUninitializedState(){this.statusText&&(this.statusText.textContent="\u672A\u521D\u59CB\u5316"),this.statusDot&&(this.statusDot.className="cb-status-dot",this.statusDot.classList.add("uninitialized")),this.intradayPeak&&(this.intradayPeak.textContent="\u7121\u6578\u64DA"),this.consecutiveSL&&(this.consecutiveSL.textContent="\u7121\u6578\u64DA"),this.cooldown&&(this.cooldown.textContent="\u7121\u6578\u64DA"),this.eventList&&(this.eventList.innerHTML='<li class="cb-event-item empty text-center" style="text-align: center;">\u5C1A\u7121\u5BE6\u76E4\u4EA4\u6613\u7D00\u9304</li>'),this.resetBtn&&(this.resetBtn.disabled=!0,this.resetBtn.textContent="\u672A\u555F\u7528")}updateUI(t){if(!t||typeof t!="object"){this.showEmptyState();return}if(t.initialized===!1){this.showUninitializedState();return}let e=t.state||"normal",i={normal:"\u6B63\u5E38",paused:"\u66AB\u505C",halted:"\u505C\u6B62",uninitialized:"\u672A\u521D\u59CB\u5316",unknown:"\u672A\u77E5"};if(this.statusText&&(this.statusText.textContent=i[e]||e),this.statusDot&&(this.statusDot.className="cb-status-dot",e==="normal"?this.statusDot.classList.add("normal"):e==="paused"?this.statusDot.classList.add("paused"):e==="halted"?this.statusDot.classList.add("halted"):this.statusDot.classList.add("unknown")),this.resetBtn&&(this.resetBtn.className="cb-btn-reset",e==="normal"?this.resetBtn.disabled=!0:(this.resetBtn.disabled=!1,e==="halted"&&this.resetBtn.classList.add("halted"))),this.intradayPeak)if(t.intraday_peak!==void 0&&t.day_start_value!==void 0&&t.day_start_value>0){let n=((t.intraday_peak-t.day_start_value)/t.day_start_value*100).toFixed(2);this.intradayPeak.textContent=`${n}%`}else e==="normal"?this.intradayPeak.textContent="0.00%":this.intradayPeak.textContent="-";if(this.consecutiveSL&&(t.consecutive_sl!==void 0?this.consecutiveSL.textContent=t.consecutive_sl:e==="normal"?this.consecutiveSL.textContent="0":this.consecutiveSL.textContent="-"),this.cooldown)if(t.cooldown_until){let n=new Date(t.cooldown_until);n>new Date?this.cooldown.textContent=n.toLocaleTimeString("zh-TW"):this.cooldown.textContent="\u7121"}else this.cooldown.textContent="\u7121";t.events&&Array.isArray(t.events)&&t.events.length>0?(this.events=t.events,this.renderEvents()):(this.events=[],this.eventList&&(this.eventList.innerHTML='<li class="cb-event-item empty text-center" style="text-align: center;">\u66AB\u7121\u4E8B\u4EF6</li>'))}addEvent(t){this.events.unshift(t),this.events.length>10&&this.events.pop(),this.renderEvents()}renderEvents(){if(this.eventList){if(this.eventList.innerHTML="",this.events.length===0){this.eventList.innerHTML='<li class="cb-event-item empty text-center" style="text-align: center;">\u66AB\u7121\u4E8B\u4EF6</li>';return}this.events.forEach(t=>{let e=document.createElement("li");e.className="cb-event-item";let i=new Date(t.timestamp||Date.now()).toLocaleTimeString("zh-TW"),n=document.createElement("span");n.textContent=t.reason||t.message||JSON.stringify(t);let o=document.createElement("span");o.className="cb-event-time",o.textContent=i,e.appendChild(n),e.appendChild(o),this.eventList.appendChild(e)})}}async handleReset(){let t=prompt("\u8F38\u5165\u624B\u52D5\u91CD\u7F6E\u539F\u56E0:");if(t)try{this.resetBtn&&(this.resetBtn.disabled=!0,this.resetBtn.textContent="\u91CD\u7F6E\u4E2D..."),(await fetch("/api/dashboard/circuit-breaker/reset",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({reason:t})})).ok?await this.fetchState():alert("\u91CD\u7F6E\u5931\u6557")}catch(e){console.error("Reset request failed:",e),alert("\u9023\u7DDA\u932F\u8AA4")}finally{this.resetBtn&&(this.resetBtn.textContent="\u624B\u52D5\u91CD\u7F6E (Reset)")}}handleSSE(t){t.type==="circuit_breaker_state_change"?(this.fetchState(),this.addEvent(t.data)):t.type==="circuit_breaker_event"?this.addEvent(t.data):t.type==="live_event"&&t.data&&t.data.source==="circuit_breaker"&&(this.addEvent(t.data),this.fetchState())}};function x(s){if(typeof s=="string"&&(s=document.getElementById(s)),!s){console.error("performance-report: container not found");return}s.innerHTML=`
    <div class="pr-toolbar">
      <div class="pr-period-selector" id="prPeriodSelector">
        <button class="pr-period-btn active" data-period="30d">30 \u5929</button>
        <button class="pr-period-btn" data-period="90d">90 \u5929</button>
        <button class="pr-period-btn" data-period="1y">1 \u5E74</button>
        <button class="pr-period-btn" data-period="all">\u5168\u90E8\u671F\u9593</button>
      </div>
      <button class="pr-export-btn" id="prExportBtn">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
        \u532F\u51FA Markdown
      </button>
    </div>
    
    <div class="pr-summary-header" id="prDateRange">\u8F09\u5165\u4E2D\u2026</div>
    
    <div id="prKpiGrid" class="pr-grid">
      <!-- KPI cards -->
    </div>
    
    <div class="pr-section-title">\u{1F3C6} \u6700\u4F73\u8CA2\u737B AI</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>AI \u540D\u7A31</th>
            <th>\u7E3D\u5831\u916C</th>
            <th>\u52DD\u7387</th>
            <th>\u590F\u666E\u503C</th>
          </tr>
        </thead>
        <tbody id="prAgentsBody">
          <!-- agents -->
        </tbody>
      </table>
    </div>
    
    <div class="pr-section-title">\u{1F4CA} \u5E02\u5834\u72C0\u614B\u7E3E\u6548</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>\u5E02\u5834\u72C0\u614B</th>
            <th>\u5834\u6B21\u6578</th>
            <th>\u5E73\u5747\u5831\u916C</th>
            <th>\u52DD\u7387</th>
          </tr>
        </thead>
        <tbody id="prRegimesBody">
          <!-- regimes -->
        </tbody>
      </table>
    </div>
    
    <div class="pr-section-title">\u{1F4C5} \u6708\u5EA6\u5831\u916C</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>\u6708\u4EFD</th>
            <th>\u5831\u916C</th>
            <th>\u7D2F\u7A4D</th>
          </tr>
        </thead>
        <tbody id="prMonthsBody">
          <!-- months -->
        </tbody>
      </table>
    </div>
  `;let t=s.querySelector("#prPeriodSelector");t.addEventListener("click",e=>{e.target.tagName==="BUTTON"&&(t.querySelectorAll("button").forEach(i=>i.classList.remove("active")),e.target.classList.add("active"),_(e.target.dataset.period))}),s.querySelector("#prExportBtn").addEventListener("click",()=>{let e=t.querySelector(".active").dataset.period;k("md",e)}),_("30d")}async function _(s){let t=document.getElementById("prKpiGrid");if(t){t.innerHTML='<div class="pr-loading">\u8F09\u5165\u5831\u544A\u8CC7\u6599\u4E2D\u2026</div>';try{let e=await fetch(`/api/dashboard/performance-report?period=${s}`);if(!e.ok)throw new Error("\u7121\u6CD5\u53D6\u5F97\u7E3E\u6548\u5831\u544A");let i=await e.json();B(i)}catch(e){t.innerHTML=`<div class="pr-loading" style="color:var(--down)">\u932F\u8AA4\uFF1A${e.message}</div>`}}}function k(s,t){window.open(`/api/dashboard/performance-report/export?format=${s}&period=${t}`,"_blank")}function B(s){let{fmtNTD:t,fmtPct:e,fmtFloat:i,agentNameEsm:n,regimeLabelEsm:o}=window;document.getElementById("prDateRange").textContent=`${(s.start_date||"--").slice(0,10)} \uFF5E ${(s.end_date||"--").slice(0,10)}`;var l=s.max_drawdown||0,d=l>0?-1:0;let h=[{label:"\u7E3D\u5831\u916C",value:e?e(s.total_return||0):((s.total_return||0)*100).toFixed(2)+"%",sign:s.total_return},{label:"\u5E74\u5316\u5831\u916C",value:e?e(s.annualized_return||0):((s.annualized_return||0)*100).toFixed(2)+"%",sign:s.annualized_return},{label:"\u590F\u666E\u6BD4\u7387",value:i?i(s.sharpe_ratio||0,2):(s.sharpe_ratio||0).toFixed(2)},{label:"\u6700\u5927\u56DE\u64A4",value:l>0?"-"+(e?e(l):(l*100).toFixed(2)+"%"):"0.00%",sign:d},{label:"\u7A05\u5F8C\u50F9\u503C",value:t?t(s.after_tax_value||0):(s.after_tax_value||0).toFixed(0)},{label:"\u5DF2\u7E73\u7A05\u984D",value:t?t(s.total_tax_paid||0):(s.total_tax_paid||0).toFixed(0),hint:"\u7D2F\u7A4D"},{label:"\u52DD\u7387",value:e?e(s.win_rate||0):((s.win_rate||0)*100).toFixed(1)+"%"},{label:"\u7E3D\u4EA4\u6613\u6578",value:s.total_trades||0}].map(function(a){var r="";return a.sign>0?r="positive":a.sign<0&&(r="negative"),`<div class="pr-card">
      <div class="pr-card-label">${a.label}</div>
      <div class="pr-card-value ${r}">${a.value}</div>
      ${a.hint?'<div class="pr-card-hint">'+a.hint+"</div>":""}
    </div>`}).join("");document.getElementById("prKpiGrid").innerHTML=h;let v=document.getElementById("prAgentsBody");s.top_agents&&s.top_agents.length>0?v.innerHTML=s.top_agents.map(function(a){var r=a.total_return||0,u=r>0?"positive":r<0?"negative":"";return"<tr><td>"+(n?n(a.agent_id):a.agent_id)+'</td><td style="color:'+(r>0?"var(--up)":r<0?"var(--down)":"var(--text)")+'">'+(e?e(r):(r*100).toFixed(2)+"%")+"</td><td>"+(e?e(a.win_rate||0):((a.win_rate||0)*100).toFixed(1)+"%")+"</td><td>"+(i?i(a.sharpe_like||0,2):(a.sharpe_like||0).toFixed(2))+"</td></tr>"}).join(""):v.innerHTML='<tr><td colspan="4" class="pr-loading">\u5C1A\u7121 AI \u7E3E\u6548\u8CC7\u6599</td></tr>';let b=document.getElementById("prRegimesBody");var L=s.regime_breakdown&&s.regime_breakdown.regimes?s.regime_breakdown.regimes:{},g=Object.entries(L);g.length>0?b.innerHTML=g.map(function(a){var r=a[0],u=a[1],f=u.avg_return||0,T=u.session_count||0;return"<tr><td>"+(o?o(r):r)+"</td><td>"+T+'</td><td style="color:'+(f>0?"var(--up)":f<0?"var(--down)":"var(--text)")+'">'+(e?e(f):(f*100).toFixed(2)+"%")+"</td><td>"+(e?e(u.win_rate||0):((u.win_rate||0)*100).toFixed(1)+"%")+"</td></tr>"}).join(""):b.innerHTML='<tr><td colspan="4" class="pr-loading">\u5C1A\u7121\u5E02\u5834\u72C0\u614B\u7E3E\u6548\u8CC7\u6599</td></tr>';let w=document.getElementById("prMonthsBody");s.monthly_returns&&s.monthly_returns.length>0?w.innerHTML=s.monthly_returns.map(function(a){var r=a.return||0;return"<tr><td>"+(a.month||"--")+'</td><td style="color:'+(r>0?"var(--up)":r<0?"var(--down)":"var(--text)")+'">'+(e?e(r):(r*100).toFixed(2)+"%")+"</td><td>"+(e?e(a.cumulative||0):((a.cumulative||0)*100).toFixed(2)+"%")+"</td></tr>"}).join(""):w.innerHTML='<tr><td colspan="3" class="pr-loading">\u5C1A\u7121\u6708\u5EA6\u5831\u916C\u8CC7\u6599</td></tr>'}var E={OK:{class:"badge ok",label:"OK",color:"var(--color-success)"},WARN:{class:"badge warn",label:"WARN",color:"var(--color-warning)"},FAIL:{class:"badge err",label:"FAIL",color:"var(--color-danger)"},START:{class:"badge info",label:"START",color:"var(--color-info)"}},m=class{constructor(t){if(this.container=document.getElementById(t),!this.container){console.warn(`SimHealthPanel: container #${t} not found`);return}this.traces=[],this.refreshInterval=null,this.isDestroyed=!1,this.renderSkeleton(),this.fetchTraces(),this.startAutoRefresh()}renderSkeleton(){this.container.innerHTML=`
      <div class="sim-health-panel">
        <div class="flex-between mb-sm">
          <h2 class="m-0">\u{1FA7A} \u6A21\u64EC\u5065\u5EB7\u5EA6</h2>
          <div class="control-group m-0">
            <span id="simHealthLastUpdate" class="text-muted text-sm">\u8F09\u5165\u4E2D\u2026</span>
            <button onclick="window.simHealthPanel?.fetchTraces()" title="\u624B\u52D5\u5237\u65B0">\u{1F504}</button>
          </div>
        </div>
        <div class="sim-health-summary" id="simHealthSummary">
          <div class="sim-health-stat">
            <div class="sim-health-stat__value" id="simStatTotal">-</div>
            <div class="sim-health-stat__label">\u7E3D\u6B65\u9A5F</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--ok" id="simStatOk">-</div>
            <div class="sim-health-stat__label">\u6B63\u5E38</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--warn" id="simStatWarn">-</div>
            <div class="sim-health-stat__label">\u8B66\u544A</div>
          </div>
          <div class="sim-health-stat">
            <div class="sim-health-stat__value sim-health-stat__value--fail" id="simStatFail">-</div>
            <div class="sim-health-stat__label">\u5931\u6557</div>
          </div>
        </div>
        <div class="table-wrapper mt-sm">
          <table id="simHealthTable">
            <thead>
              <tr>
                <th>\u6B65\u9A5F</th>
                <th>\u5C64\u7D1A</th>
                <th>\u72C0\u614B</th>
                <th>\u6642\u9593\u6233</th>
                <th>\u5143\u6578\u64DA</th>
              </tr>
            </thead>
            <tbody id="simHealthTableBody">
              <tr><td colspan="5" class="empty loading">\u8F09\u5165\u4E2D\u2026</td></tr>
            </tbody>
          </table>
        </div>
      </div>
    `}async fetchTraces(){if(!this.isDestroyed)try{let t=await fetch("/api/traces/sim-latest");if(t.status===404){this.showEmptyState("\u5C1A\u7121\u6A21\u64EC\u8FFD\u8E64\u8A18\u9304"),this.updateLastUpdateTime("\u7121\u8CC7\u6599");return}if(!t.ok)throw new Error(`HTTP ${t.status}`);let e=await t.json();this.traces=Array.isArray(e)?e:e.traces||[],this.updateUI(),this.updateLastUpdateTime("\u525B\u525B")}catch(t){console.error("SimHealthPanel: failed to fetch traces:",t),this.showErrorState("\u7121\u6CD5\u8F09\u5165\u6A21\u64EC\u8FFD\u8E64\u8CC7\u6599"),this.updateLastUpdateTime("\u932F\u8AA4")}}updateUI(){if(!this.container)return;let t=this.computeSummary();this.updateSummary(t),this.renderTable()}computeSummary(){let t={total:0,ok:0,warn:0,fail:0};for(let e of this.traces){t.total++;let i=(e.status||"").toUpperCase();i==="OK"?t.ok++:i==="WARN"?t.warn++:i==="FAIL"&&t.fail++}return t}updateSummary(t){let e=this.container.querySelector("#simStatTotal"),i=this.container.querySelector("#simStatOk"),n=this.container.querySelector("#simStatWarn"),o=this.container.querySelector("#simStatFail");e&&(e.textContent=t.total),i&&(i.textContent=t.ok),n&&(n.textContent=t.warn),o&&(o.textContent=t.fail)}renderTable(){let t=this.container.querySelector("#simHealthTableBody");if(!t)return;if(this.traces.length===0){t.innerHTML='<tr><td colspan="5" class="empty">\u5C1A\u7121\u8FFD\u8E64\u8A18\u9304</td></tr>';return}let e=this.traces.map(i=>{let n=(i.status||"UNKNOWN").toUpperCase(),o=E[n]||{class:"badge",label:n,color:"var(--muted)"},l=i.ts?new Date(i.ts).toLocaleString("zh-TW"):"-",d=this.formatMetadata(i.metadata);return`
        <tr class="sim-health-row sim-health-row--${n.toLowerCase()}">
          <td class="sim-health-cell sim-health-cell--step">${c(i.step||"-")}</td>
          <td class="sim-health-cell sim-health-cell--layer">${c(i.layer||"-")}</td>
          <td class="sim-health-cell sim-health-cell--status">
            <span class="${o.class}">${o.label}</span>
          </td>
          <td class="sim-health-cell sim-health-cell--time">${c(l)}</td>
          <td class="sim-health-cell sim-health-cell--meta">${d}</td>
        </tr>
      `}).join("");t.innerHTML=e}formatMetadata(t){if(!t||typeof t!="object")return'<span class="text-muted">-</span>';let e=Object.entries(t);if(e.length===0)return'<span class="text-muted">-</span>';let i=2,n=e.slice(0,i),o=e.length-i,l=n.map(([d,p])=>{let h=typeof p=="object"?JSON.stringify(p):String(p),v=h.length>30?h.substring(0,30)+"\u2026":h;return`<span class="sim-health-meta-item"><span class="sim-health-meta-key">${c(d)}:</span> <span class="sim-health-meta-val">${c(v)}</span></span>`});return o>0&&l.push(`<span class="sim-health-meta-more">+${o} \u9805</span>`),`<div class="sim-health-meta">${l.join("")}</div>`}showEmptyState(t){let e=this.container?.querySelector("#simHealthTableBody");e&&(e.innerHTML=`<tr><td colspan="5" class="empty">${c(t)}</td></tr>`),this.updateSummary({total:0,ok:0,warn:0,fail:0})}showErrorState(t){let e=this.container?.querySelector("#simHealthTableBody");e&&(e.innerHTML=`
        <tr>
          <td colspan="5">
            <div class="error-banner">
              <span>\u26A0\uFE0F ${c(t)}</span>
              <button class="retry-btn" onclick="window.simHealthPanel?.fetchTraces()">\u91CD\u8A66</button>
            </div>
          </td>
        </tr>
      `),this.updateSummary({total:0,ok:0,warn:0,fail:0})}updateLastUpdateTime(t){let e=this.container?.querySelector("#simHealthLastUpdate");e&&(e.textContent=t)}startAutoRefresh(){this.refreshInterval=setInterval(()=>{this.fetchTraces()},5e3)}stopAutoRefresh(){this.refreshInterval&&(clearInterval(this.refreshInterval),this.refreshInterval=null)}destroy(){this.isDestroyed=!0,this.stopAutoRefresh()}};function c(s){return s==null?"":String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;")}window.SimHealthPanel=m;var C=new y("circuitBreakerPanel");S.on("*",s=>C.handleSSE(s));x("performanceReportContainer");var H=document.getElementById("simHealthContainer");H&&(window.simHealthPanel=new m("simHealthContainer"));
