// Experiments and human intervention controls
import { agentName, sectorName } from '../names.js';
import { getJSON, notify, formatDate } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

export async function loadOverrides() {
  const container = document.getElementById('overrideBadges');
  if (!container) return;
  try {
    const data = await getJSON('/api/control/active-overrides');
    const paused = data.paused_agents || [];
    const banned = data.banned_sectors || [];
    const pausedBadges = paused.map(a => `<span class="badge err">⏸ ${a}</span>`).join('');
    const bannedBadges = banned.map(s => `<span class="badge warn">🚫 ${s}</span>`).join('');
    container.innerHTML = pausedBadges + bannedBadges || '<span class="text-muted text-sm">目前無生效覆寫</span>';
  } catch (e) { console.error(e); }
}

export async function loadAuditLog() {
  const el = document.getElementById('auditLog');
  if (!el) return;
  try {
    const data = await getJSON('/api/control/audit-log');
    el.classList.remove('loading');
    const items = (data.interventions || []).slice(0, 20);
    if (!items.length) { el.innerHTML = renderEmptyState('無紀錄', ''); return; }
    const actionMap = {
      'pause_agent': '暫停 Agent',
      'resume_agent': '恢復 Agent',
      'set_model_weight': '設定模型權重',
      'sector_ban': '封鎖產業',
      'sector_unban': '解除封鎖',
      'approve_rec': '批准推薦',
      'reject_rec': '拒絕推薦'
    };
    el.innerHTML = `<table><thead><tr><th>時間</th><th>操作者</th><th>動作</th><th>對象</th><th>原因</th></tr></thead><tbody>
      ${items.map(it => `<tr><td>${formatDate(it.recorded_at)}</td><td>${it.operator ? escapeHtml(it.operator) : '-'}</td><td>${actionMap[it.type] || escapeHtml(it.type)}</td><td>${escapeHtml(agentName(it.target_agent_id)) || escapeHtml(sectorName(it.target_sector)) || (it.target_symbol ? escapeHtml(it.target_symbol) : '') || (it.target_model_id ? escapeHtml(it.target_model_id) : '') || '-'}</td><td>${it.reason ? escapeHtml(it.reason) : '-'}</td></tr>`).join('')}
    </tbody></table>`;
  } catch (e) { el.innerHTML = '<div class="empty">載入失敗</div>'; }
}

export async function loadExperimentHistory() {
  const el = document.getElementById('experimentHistory');
  if (!el) return;
  try {
    const data = await getJSON('/api/experiment/history');
    el.classList.remove('loading');
    const items = (data.history || []).slice(0, 20);
    if (!items.length) { el.innerHTML = renderEmptyState('無紀錄', ''); return; }
    el.innerHTML = `<table><thead><tr><th>時間</th><th>版本</th><th>實驗</th><th>AI</th><th>狀態</th></tr></thead><tbody>
      ${items.map(it => `<tr><td>${formatDate(it.promoted_at)}</td><td>v${it.version_after || '-'}</td><td>${escapeHtml(it.experiment_id) || ''}</td><td>${escapeHtml(agentName(it.target_agent_id)) || (it.target_agent_id ? escapeHtml(it.target_agent_id) : '')}</td><td><span class="badge ${it.status==='accepted'?'ok':(it.status==='rejected'?'err':'warn')}">${it.status==='accepted'?'已接受':(it.status==='rejected'?'已拒絕':escapeHtml(it.status))}</span></td></tr>`).join('')}
    </tbody></table>`;
    const revertSel = document.getElementById('revertSelect');
    revertSel.innerHTML = '<option value="">-- 選擇要回滾的版本 --</option>' + items.map((it, i) => `<option value="${escapeHtml(it.experiment_id)}">v${it.version_after || i} - ${escapeHtml(it.experiment_id)}</option>`).join('');
  } catch (e) { el.innerHTML = '<div class="empty">載入失敗</div>'; }
}

// --- Actions ---
export async function judgeExperiment(id) {
  try {
    const res = await postJSON('/api/experiment/judge', { experiment_id: id });
    notify(`評判完成：${id} → ${res.status==='accepted'?'已接受':res.status}`, res.status==='accepted'?'ok':'info');
    loadAll();
  } catch (e) { notify('評判失敗：' + e.message, 'err'); }
}

export async function viewDiff(id) {
  try {
    const data = await getJSON('/api/experiment/diff?experiment_id=' + encodeURIComponent(id));
    document.getElementById('diffBaseline').textContent = data.baseline_prompt || '(empty)';
    document.getElementById('diffCandidate').textContent = data.candidate_prompt || '(empty)';
    document.getElementById('diffModal').classList.add('show');
  } catch (e) { notify('載入差異比對失敗：' + e.message, 'err'); }
}
export function closeModal() { document.getElementById('diffModal').classList.remove('show'); }

export let pendingPromotePath = '';
export function openPromote(path) {
  pendingPromotePath = path;
  document.getElementById('promotePreview').innerHTML = `<code>${path}</code>`;
  document.getElementById('promoteModal').classList.add('show');
}
export function closePromoteModal() { document.getElementById('promoteModal').classList.remove('show'); pendingPromotePath = ''; }

export function openKpiHelp(key) {
  const titleMap = {
    narrative: '敘事脈絡',
    regime: '市場狀態',
    weakest: '待改進 AI 策略',
    experiment: '實驗狀態',
    crowding: '擁擠標的',
    data_time: '資料時間說明'
  };
  const contentMap = {
    narrative: `<p><strong>這是什麼？</strong><br>顯示當前回測窗口中，由總經數據（利率、匯率、資金流向等）驅動的最重要敘事主題，以及外資出逃指數。它回答了「現在市場的主要故事是什麼」。</p>
<p><strong>為什麼重要？</strong><br>敘事脈絡決定了倉位規模與產業配置傾向。若出現「AI 資本支出激增」或「地緣政治緊張」等事件，會直接影響下游 Agent 的推薦權重與控制層的過濾條件。</p>
<p><strong>該注意什麼？</strong><br>點擊卡片上的「開啟宏觀敘事 →」可前往完整 6 大面板。若外資出逃指數處於「橙燈」（50-69分）或「紅燈」（70分以上），代表外資正在明顯撤離，建議同步檢視【相對趨勢】頁的總經雷達，並考慮降低整體曝險。</p>`,
    regime: `<p><strong>這是什麼？</strong><br>基於最新回測資料計算出的市場體制（RISK_ON／NEUTRAL／RISK_OFF）。這是 Context Layer 對當前環境的綜合判斷。<br><br>
<strong>請注意名稱含義：</strong>
<ul style="margin:6px 0;padding-left:18px">
<li><code>RISK_ON</code> = <span class="text-up">風險偏好（積極）</span>：市場願意承擔風險，通常是加倉機會。</li>
<li><code>NEUTRAL</code> = <span class="text-warn">中性</span>：方向不明，系統會自動縮減單一倉位上限至 85%。</li>
<li><code>RISK_OFF</code> = <span class="text-down">風險趨避（保守）</span>：市場傾向避險，這才是需要特別警惕的狀態。</li>
</ul></p>
<p><strong>為什麼重要？</strong><br>體制決定了整個投資組合的基調。RISK_ON 時可積極參與成長與動能策略；NEUTRAL 時應控制單一標的比重並提高篩選標準；RISK_OFF 時應優先考慮降低曝險、轉向防禦性配置或提高現金比重。</p>
<p><strong>該注意什麼？</strong><br>當 regime 從 RISK_ON 快速轉向 RISK_OFF，通常伴隨外資出逃指數飆升（紅燈）。此時請立即前往【相對趨勢】頁檢查總經雷達，並檢視【投資管線】是否有過多推薦被控制層阻擋——這是市場風險情緒惡化的早期信號。</p>`,
    weakest: `<p><strong>這是什麼？</strong><br>演化系統根據策略層級（Sector/Style 優於 Control）、歷史觀察數量（越少越需補足）與 Sharpe-like 風險調整回報，綜合選出最需要改進的 Agent。它就是下一輪突變實驗（Mutation）的目標對象。</p>
<p><strong>為什麼重要？</strong><br>Atlas-Go 的演化循環（Evolution Loop）持續識別當前最需要迭代的策略，並自動生成 prompt、規則或約束條件的突變改進提案。這不是「應該避開該策略的選股建議」，而是「這個策略的統計表現有改進空間，值得投入實驗資源」的演化訊號。</p>
<p><strong>該注意什麼？</strong><br>在提出突變前，建議先進入【AI 觀測台】頁檢查該 Agent 的觀察數（observations）是否充足、失效是否集中在特定 regime。若回測資料不足（如最新交易日尚無隔日數據），Sharpe-like 可能基於估算值計算，此時應優先參考歷史窗口的穩定數據。</p>`,
    experiment: `<p><strong>這是什麼？</strong><br>顯示當前待評判（judge）和待晉升（promote）的實驗數量。它是整個 mutation → judge → promote → revert 閉環的可視化入口。</p>
<p><strong>為什麼重要？</strong><br>若存在「待評判」實驗，表示已有突變運行完成但尚未決定接受或拒絕；若存在「待晉升」實驗，表示已有實驗通過門檻但尚未寫入基線政策（baseline_policy.json）。</p>
<p><strong>該注意什麼？</strong><br>停滯的實驗會阻塞下一輪改進循環。建議定期進入【模擬交易】頁進行評判與晉升。若晉升後發現績效反轉，可透過【控制與稽核】頁回滾基線版本。</p>`,
    crowding: `<p><strong>這是什麼？</strong><br>當同一標的同時被 ≥3 個 Agent 推薦，或 Style Layer 的標的池重疊過高時，CIO 層會觸發擁擠懲罰（conviction × 0.7）。此卡片列出當前被多重疊覆蓋的標的。</p>
<p><strong>為什麼重要？</strong><br>擁擠是風格趨同的信號，往往預示短期波動放大或回調風險。這幫助操作者識別「大家都愛」的熱門標的是否已過度集中。</p>
<p><strong>該注意什麼？</strong><br>高重疊不一定立刻危險，但如果疊加外資出逃指數紅燈（>70分）或 NEUTRAL regime，應特別警惕。可考慮在【投資管線】頁手動拒絕部分高擁擠標的，或進一步降低該風格 Agent 的 Darwinian 權重。</p>`,
    data_time: `<p><strong>卡片上的 3 個時間分別代表什麼？</strong></p>
<table style="width:100%;font-size:13px;margin:8px 0">
<thead><tr><th>#</th><th>時間</th><th>來源</th><th>語意</th></tr></thead>
<tbody>
<tr><td>①</td><td><strong>主值</strong>（較大數字）</td><td>TWSE 回放 CSV 的最後交易日</td><td>系統擁有的最「新回放資料日期」。<br>這是歷史股價數據的最新一天。</td></tr>
<tr><td>②</td><td><strong>「最後模擬」ID</strong>（如 <code>window-20260413</code>）</td><td>回測窗口 JSON 檔名</td><td>最近一次模擬運行的窗口編號。<br>編碼了該次模擬的交易日。</td></tr>
<tr><td>③</td><td><strong>「最後模擬」時間</strong></td><td>窗口檔案的修改時間</td><td>最近一次模擬實際執行的時間點。</td></tr>
</tbody>
</table>
<p><strong>為什麼這 3 個時間會不一致？</strong><br><strong>這是正常的。</strong>它們量測的是完全不同的維度：</p>
<ul style="margin:4px 0 8px;padding-left:18px;line-height:1.8">
<li>① 取決於 TWSE 市場交易日與 backfill 排程 — 週末/假日不會有新資料</li>
<li>② 取決於模擬排程器（每天執行一次）— 窗口 ID 對應排程日而非資料日</li>
<li>③ 取決於模擬任務的實際執行時間 — 若背景佇列塞車可能延遲數小時</li>
</ul>
<p><strong>舉例：</strong>回放資料最新到 4/10（週五），排程器在 4/13（週一）執行模擬，但因為資料量大任務排到 4/14 凌晨才跑完。此時①=4/10、②=<code>window-20260413</code>、③=4/14 02:30。</p>
<p><strong>需要留意的異常信號：</strong></p>
<table style="width:100%;font-size:13px">
<thead><tr><th>信號</th><th>可能含義</th><th>建議行動</th></tr></thead>
<tbody>
<tr><td>① 超過 5 天未更新</td><td>TWSE backfill 排程未執行</td><td>檢查 <code>cmd/import-replay</code> 是否正常運作</td></tr>
<tr><td>② 與 ① 差距超過 7 天</td><td>模擬排程器未定期執行</td><td>檢查 cron / systemd 排程服務狀態</td></tr>
<tr><td>③ 與 ② 差距過大（>1 天）</td><td>模擬任務阻塞或佇列積壓</td><td>檢查 taskexec 管理器是否有卡住任務</td></tr>
</tbody>
</table>`,
     cm_spx: `<p><strong>這是什麼？</strong><br>S&P 500 指數，覆蓋美國 500 家大型企業的市值加權指數，被視為美國整體股市表現的基準。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝當日漲幅 ≥ 0、<span class="text-success">綠色</span>＝當日跌幅。數值旁顯示的是即時點位（last price）。<br>（本系統遵循台股紅漲綠跌慣例。）</p>
<p><strong>該注意什麼？</strong><br>S&P 500 與台股加權指數（TWSE）有 0.4–0.6 的滾動相關性。若 SPX 單日跌幅 &gt; 1.5%，隔日台股開盤常見 0.5–1% 的補跌壓力。請同步檢視【動態相關性】表格的 ρ 值，若 ρ &gt; 0.7 警示傳導效應已放大。</p>`,
     cm_ndx: `<p><strong>這是什麼？</strong><br>Nasdaq 指數（NASDAQ-100），由納斯達克交易所上市的 100 家最大非金融類公司組成，科技股權重高。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝當日上漲、<span class="text-success">綠色</span>＝當日下跌。對台股的傳導集中在半導體與 AI 概念股。</p>
<p><strong>該注意什麼？</strong><br>NDX 對台股的影響主要透過台積電 ADR（TSM）。若 NDX 與 TSM ADR 同步下跌超過 1%，預期隔日台積電現貨有顯著負面反應。注意與 SOX 指數的同向性，作為「半導體週期」領先指標。</p>`,
     cm_dji: `<p><strong>這是什麼？</strong><br>道瓊工業指數（Dow Jones Industrial Average），由 30 家美國藍籌股組成，價格加權指數。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝當日上漲、<span class="text-success">綠色</span>＝當日下跌。DJI 偏重傳產、金融、工業，對台股塑膠、紡織、航運等傳產股有間接傳導。</p>
<p><strong>該注意什麼？</strong><br>DJI 對台股的傳導較 SPX/NDX 弱（相關性通常 0.3–0.5），但若 DJI 大跌常反映總體景氣轉弱訊號，應同步檢視 DXY 與 US 10Y 殖利率變化以判斷是否為「risk-off」環境。</p>`,
     cm_sox: `<p><strong>這是什麼？</strong><br>費城半導體指數（PHLX Semiconductor Index, SOX），追蹤 30 家半導體設計、製造、設備公司的股價，被視為全球半導體景氣的領先指標。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝半導體類股上漲、<span class="text-success">綠色</span>＝下跌。SOX 對台股的傳導最強（台股加權指數約 65% 與半導體相關）。</p>
<p><strong>該注意什麼？</strong><br>SOX 與台股加權的滾動相關性常達 0.7 以上，是判斷台股半導體族群強弱的最重要外部參考。若 SOX 單週跌幅 &gt; 3%，且 NDX、TSM ADR 同步走弱，建議降低半導體持倉比重並啟動更嚴格的個股風控。</p>`,
     cm_nvda: `<p><strong>這是什麼？</strong><br>NVIDIA Corporation 股價。GPU 龍頭，AI 晶片需求核心受惠者，也是費城半導體指數最大權值股之一。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝上漲、<span class="text-success">綠色</span>＝下跌。對台股的傳導鏈：NVDA → 供應鏈（台積電、ABF 載板、CoWoS 設備）→ 台股加權指數。</p>
<p><strong>該注意什麼？</strong><br>NVDA 對台積電的營收與股價有 1–2 個月的領先性。當 NVDA 月線跌破 -10%，常預示台積電下季展望下修風險。注意 NVDA 與 TSM ADR 的價差/比值變化，可作為套利與避險參考。</p>`,
     cm_aapl: `<p><strong>這是什麼？</strong><br>Apple Inc. 股價。全球最大市值公司之一，產品線涵蓋 iPhone、Mac、服務。對台股的傳導集中在 PCB、組裝、相機模組供應鏈。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝上漲、<span class="text-success">綠色</span>＝下跌。AAPL 對台股的傳導鏈：AAPL 銷售 → 鴻海/和碩/大立光/玉晶光 → 台股消費電子族群。</p>
<p><strong>該注意什麼？</strong><br>AAPL 財報週（每年 1/4/7/10 月）前後 5 個交易日，台股蘋果供應鏈波動放大。當 AAPL 跌破 200 日均線且月線連 2 黑，需重新評估台股蘋概股持倉。</p>`,
     cm_msft: `<p><strong>這是什麼？</strong><br>Microsoft Corporation 股價。雲端（Azure）與企業軟體龍頭。對台股的傳導主要在 AI 伺服器供應鏈（緯創、廣達、英業達）。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝上漲、<span class="text-success">綠色</span>＝下跌。MSFT 資本支出指引是台股 AI 伺服器族群最重要的領先指標。</p>
<p><strong>該注意什麼？</strong><br>MSFT 季報中 Azure 營收增速若 &lt; 30% YoY，常引發 AI 伺服器供應鏈評價修正。當 MSFT 與 NVDA 同步走弱，是 AI 敘事退潮的早期訊號，建議啟動 AI 概念股風控（Darwinian 權重下修）。</p>`,
     cm_tsm: `<p><strong>這是什麼？</strong><br>台積電 ADR（TSM）在紐約交易所的價格，代表台積電在美國市場的估值。1 ADR = 5 股台股現貨。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝ADR 上漲、<span class="text-success">綠色</span>＝下跌。TSM ADR 對台股加權指數的傳導力最高（單一個股權重約 25-30%）。</p>
<p><strong>該注意什麼？</strong><br>TSM ADR 與台股 2330 現貨之間有 T+1 套利關係，但匯率與手續費會吃掉 0.3% 左右價差。當 TSM ADR 隔夜大跌 &gt; 2%，台積電現貨幾乎必定開低；建議在重大事件（如法說、Blackwell 量產）前降低 TSM 曝險。</p>`,
     cm_vix: `<p><strong>這是什麼？</strong><br>CBOE 波動率指數（VIX），又稱「恐慌指數」，衡量 S&P 500 未來 30 天隱含波動率。市場預期波動越大，VIX 越高。</p>
<p><strong>顏色意義：</strong><br>VIX 本身沒有漲跌顏色標記（中性指標）。但 VIX &lt; 15 視為<span class="text-success">市場冷靜</span>、&gt; 25 為<span class="text-warn">警戒</span>、&gt; 35 觸發<span class="text-danger">危機模式</span>。</p>
<p><strong>該注意什麼？</strong><br>VIX ≥ 35 時，系統自動觸發【危機模式】：電路熔斷器強制開啟、最佳化器協方差矩陣對角膨脹 1.5x、最大持倉比例減半。VIX 從 12 飆升到 25 以上通常預示單週內有重大事件，建議檢視避險部位並降低曝險。</p>`,
     cm_dxy: `<p><strong>這是什麼？</strong><br>美元指數（DXY），衡量美元兌一籃子六種主要貨幣（EUR、JPY、GBP、CAD、SEK、CHF）的強弱。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝美元走強、<span class="text-success">綠色</span>＝美元走弱。對台股的影響：強美元 → 新興市場資金外流 → 台股壓力。</p>
<p><strong>該注意什麼？</strong><br>DXY 突破 105 視為「強勢美元」訊號，常伴隨美債殖利率走高與新興市場下修。當 DXY 與 US 10Y 同向走強，是全球資金回流美國的訊號，建議降低新興市場（含台股）曝險。DXY 與 USD/TWD 高度正相關，可交叉驗證。</p>`,
     cm_usd_twd: `<p><strong>這是什麼？</strong><br>美元兌新台幣匯率（USD/TWD）。直接影響台股上市櫃公司（特別是外銷股）的匯兌損益與競爭力。</p>
<p><strong>顏色意義：</strong><br><span class="text-success">綠色</span>＝台幣升值（數字變小）、<span class="text-danger">紅色</span>＝台幣貶值（數字變大）。</p>
<p><strong>該注意什麼？</strong><br>USD/TWD 快速突破 32.0 視為台幣貶值加速，外銷股（台積電、鴻海、出口導向電子）短期受惠但長期不利。央行通常會在 32.5 以上進場調節。當 USD/TWD 與 DXY 走勢分歧（亞幣集體走弱但台幣獨強），常預示央行干預。</p>`,
     cm_us10y: `<p><strong>這是什麼？</strong><br>美國 10 年期公債殖利率，被視為「無風險利率」基準。影響全球資產估值（DCF 模型折現率）、股票本益比、與資金流向。</p>
<p><strong>顏色意義：</strong><br><span class="text-danger">紅色</span>＝殖利率上漲（債價下跌）、<span class="text-success">綠色</span>＝殖利率下跌（債價上漲）。對股市而言，殖利率上漲 = 估值壓力。</p>
<p><strong>該注意什麼？</strong><br>US 10Y 突破 4.5% 通常對全球股市（尤其成長股）形成估值壓力。當 US 10Y 與 DXY 同步走強，是「risk-off」訊號；當 US 10Y 下跌但 DXY 走強（stagflation 訊號），成長股受壓最重。建議與 DXY 交叉解讀。</p>`,
     cm_crisis: `<p><strong>這是什麼？</strong><br>危機信號是基於 VIX 指數閾值的全系統風險開關：</p>
<ul style="margin:6px 0;padding-left:18px;line-height:1.8">
<li><span class="text-success">綠燈</span>（正常模式）：VIX &lt; 25，系統以標準參數運作</li>
<li><span class="text-warn">黃燈</span>（警戒模式）：25 ≤ VIX &lt; 35，系統自動降低最大持倉比例</li>
<li><span class="text-danger">紅燈</span>（危機模式）：VIX ≥ 35，電路熔斷器強制開啟、協方差矩陣對角膨脹 1.5x、最大持倉比例減半</li>
</ul>
<p><strong>為什麼重要？</strong><br>危機模式是「系統最後防線」。即使所有 AI Agent 與控制層都同意某個高曝險建議，危機模式仍會以硬規則強制降倉，防止模型失效（model failure）導致災難性虧損。</p>
<p><strong>該注意什麼？</strong><br>從綠燈直接跳到紅燈通常預示市場發生 shock event（央行緊急決策、地緣衝突、流動性危機）。VIX 從 35 快速回落至 25 以下後，需觀察 3 個交易日確認危機解除，避免過早解除風控。危機模式下，所有新增推薦都會被自動降低 conviction 30%。</p>`
  };
  document.getElementById('infoTitle').textContent = titleMap[key] || '說明';
  document.getElementById('infoContent').innerHTML = contentMap[key] || '';
  document.getElementById('infoModal').classList.add('show');
}
export function closeInfoModal() { document.getElementById('infoModal').classList.remove('show'); }
export function openInfoHelp(title, htmlContent) {
  document.getElementById('infoTitle').textContent = title || '說明';
  document.getElementById('infoContent').innerHTML = htmlContent || '';
  document.getElementById('infoModal').classList.add('show');
}

export async function confirmPromote() {
  if (!pendingPromotePath) return;
  try {
    const res = await postJSON('/api/experiment/promote', { result_path: pendingPromotePath });
    notify(`晉升成功：基線 v${res.version}`, 'ok');
    closePromoteModal();
    loadAll();
  } catch (e) { notify('晉升失敗：' + e.message, 'err'); }
}

export async function promoteExperiment() {
  const path = document.getElementById('promoteSelect').value;
  if (!path) { notify('請先選擇一個實驗', 'warn'); return; }
  openPromote(path);
}

export async function revertExperiment() {
  const id = document.getElementById('revertSelect').value;
  const reason = document.getElementById('revertReason').value.trim();
  if (!id) { notify('請選擇要回滾的版本', 'warn'); return; }
  if (!confirm('確定要回滾到 ' + id + ' 嗎？')) return;
  try {
    await postJSON('/api/experiment/revert', { type: 'experiment', experiment_id: id, reason: reason || '儀表板回滾' });
    notify('回滾成功', 'ok');
    loadAll();
  } catch (e) { notify('回滾失敗: ' + e.message, 'err'); }
}


export async function approveRec(btn, symbol, agentID) {
  const cell = btn.parentElement;
  cell.querySelectorAll('button').forEach(b => b.disabled = true);
  btn.textContent = '…';
  try {
    await postJSON('/api/control/approve-recommendation', { symbol, agent_id: agentID, reason: '儀表板人工批准', operator: 'human' });
    btn.textContent = '✓';
    notify(`已批准 ${symbol}（${agentID}）`, 'ok');
    loadAll();
  } catch (e) {
    cell.querySelectorAll('button').forEach(b => b.disabled = false);
    btn.textContent = '✓';
    notify('批准失敗：' + e.message, 'err');
  }
}
export async function rejectRec(btn, symbol, agentID) {
  const cell = btn.parentElement;
  cell.querySelectorAll('button').forEach(b => b.disabled = true);
  btn.textContent = '…';
  try {
    await postJSON('/api/control/reject-recommendation', { symbol, agent_id: agentID, reason: '儀表板人工拒絕', operator: 'human' });
    btn.textContent = '✕';
    notify(`已拒絕 ${symbol}（${agentID}）`, 'info');
    loadAll();
  } catch (e) {
    cell.querySelectorAll('button').forEach(b => b.disabled = false);
    btn.textContent = '✕';
    notify('拒絕失敗：' + e.message, 'err');
  }
}
export async function pauseAgent() {
  const agent_id = document.getElementById('agentSelect').value;
  if (!agent_id) return;
  try {
    await postJSON('/api/control/pause-agent', { agent_id, reason: '儀表板人工暫停', operator: 'human' });
    loadOverrides(); notify('已暫停 Agent');
  } catch (e) {
    notify('暫停失敗：' + e.message, 'err');
  }
}
export async function resumeAgent() {
  const agent_id = document.getElementById('agentSelect').value;
  if (!agent_id) return;
  try {
    await postJSON('/api/control/resume-agent', { agent_id, reason: '儀表板人工恢復', operator: 'human' });
    loadOverrides(); notify('已恢復 Agent');
  } catch (e) {
    notify('恢復失敗：' + e.message, 'err');
  }
}
export async function banSector() {
  const sector = document.getElementById('sectorSelect').value;
  try {
    await postJSON('/api/control/sector-ban', { sector, banned: true, reason: '儀表板人工封鎖', operator: 'human' });
    loadOverrides(); notify('已封鎖產業');
  } catch (e) {
    notify('封鎖失敗：' + e.message, 'err');
  }
}
export async function unbanSector() {
  const sector = document.getElementById('sectorSelect').value;
  try {
    await postJSON('/api/control/sector-ban', { sector, banned: false, reason: '儀表板人工解除封鎖', operator: 'human' });
    loadOverrides(); notify('已解除產業封鎖');
  } catch (e) {
    notify('解除封鎖失敗：' + e.message, 'err');
  }
}

// --- Forecast vs Reality summary (admin_web experiments page) ---
export function renderForecastVsRealitySummary(data) {
  const el = document.getElementById('forecastVsRealitySummary');
  if (!el) return;
  el.classList.remove('loading');

  const predictions = data && Array.isArray(data.symbol_predictions) ? data.symbol_predictions : [];
  if (!predictions.length) {
    el.innerHTML = renderEmptyState('尚無預測命中資料', '');
    return;
  }

  const withHit = predictions.filter(p => p.hit === true || p.hit === false);
  const hits = withHit.filter(p => p.hit === true).length;
  const total = withHit.length;
  const hitRate = total > 0 ? (hits / total * 100).toFixed(1) + '%' : '—';

  const passed = predictions.filter(p => p.passed_guards === true);
  const passedHits = passed.filter(p => p.hit === true).length;
  const passedRate = passed.length > 0 ? (passedHits / passed.length * 100).toFixed(1) + '%' : '—';

  el.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px">
      <div class="panel" style="text-align:center">
        <div class="kpi-label">預測總數</div>
        <div class="kpi-value" style="font-size:20px">${predictions.length}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">整體命中率</div>
        <div class="kpi-value" style="color:var(--up);font-size:20px">${hitRate}</div>
        <div class="kpi-hint">${hits} / ${total}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">控制層放行命中率</div>
        <div class="kpi-value" style="color:var(--color-success);font-size:20px">${passedRate}</div>
        <div class="kpi-hint">${passedHits} / ${passed.length}</div>
      </div>
    </div>
  `;
}

// --- Boot ---
export function populateAgentSelect() {
  const select = document.getElementById('agentSelect');
  if (!select) return;
}

if (typeof window !== "undefined") Object.assign(window, {
  closeModal, closeInfoModal, closePromoteModal, openKpiHelp, openInfoHelp,
  confirmPromote, promoteExperiment, revertExperiment,
  pauseAgent, resumeAgent, banSector, unbanSector,
  judgeExperiment, viewDiff, approveRec, rejectRec
});