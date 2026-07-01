export const cycleLegendContent = `
<h3>📊 週期羅盤 — 燈號計算說明</h3>

<div class="mb-14">
  <h4 class="h4-section-title">🟢 燈號顏色</h4>
  <div class="legend-grid">
    <div><span class="legend-dot legend-dot--success"></span>復甦 (綠)</div>
    <div><span class="legend-dot legend-dot--info"></span>擴張 (藍)</div>
    <div><span class="legend-dot legend-dot--warning"></span>成熟 (橙)</div>
    <div><span class="legend-dot legend-dot--danger"></span>衰退 (紅)</div>
  </div>
</div>

<div class="mb-14">
  <h4 class="h4-section-title">🔍 週期判定</h4>
  <p class="text-muted-md mb-6">用 <b>營收年增率</b> 和 <b>獲利年增率</b> 與產業閾值比較：</p>
  <table class="text-sm w-full">
    <thead><tr><th>判定</th><th>營收閾值</th><th>獲利閾值</th><th>範例</th></tr></thead>
    <tbody>
      <tr><td>擴張</td><td>&gt; 20%</td><td>&gt; 20%</td><td>AI供應鏈 rev 45%</td></tr>
      <tr><td>復甦</td><td>&gt; 5%</td><td>&gt; 5%</td><td>電子零組件 rev 12%</td></tr>
      <tr><td>成熟</td><td>&gt; -5%</td><td>&gt; -5%</td><td>傳產/消費 rev 3%</td></tr>
      <tr><td>衰退</td><td>其餘</td><td>其餘</td><td>航運 rev -5%</td></tr>
    </tbody>
  </table>
  <p class="text-xs-muted mt-4">半導體 / 金融 / 航運 有獨立閾值（高波動產業閾值更高）</p>
</div>

<div class="mb-14">
  <h4 class="h4-section-title">📈 信心度 — 週期判斷可靠度</h4>
  <p class="text-muted-md mb-6"><b>信心度不是「產業強弱」，而是「週期定位的可信程度」</b>（0%～100%）。<br>數值越高，代表當前產業處於該週期階段的證據越充分。</p>
  <p class="text-muted-md mt-6 mb-6"><b>信心度 = 邊界距離 × 45% + 訊號新鮮度 × 20% + 季節性契合 × 15% + 供應鏈穩定度 × 10% + 敘事事件吻合 × 10%</b></p>
  <ul class="text-small-list-muted">
    <li><b>邊界距離（45%）</b>：營收/獲利離週期閾值多遠。越遠越確定</li>
    <li><b>訊號新鮮度（20%）</b>：指標更新時間。數據越新越可靠</li>
    <li><b>季節性契合（15%）</b>：當前月份與歷史季節模式匹配程度</li>
    <li><b>供應鏈穩定度（10%）</b>：產業上下游集中度與相關性結構。連動越少、集中度越低 → 週期判斷越可靠</li>
    <li><b>敘事事件吻合（10%）</b>：宏觀敘事（如 AI 資本支出、地緣政治）與週期定位的一致性</li>
  </ul>
  <p class="text-12-muted mt-6">🎨 <b>燈號顏色深淺 = 信心度</b>。深 = 高信心（數據強烈支持）；淺 = 邊界搖擺（需謹慎解讀）。<br>💡 例：半導體顯示「擴張」但信心度僅 35%，代表雖然營收&gt;20%，但供應鏈波動大或季節性異常，實際可能處於擴張末期。</p>
</div>

<div class="mb-14">
  <h4 class="h4-section-title">🔗 進階機制</h4>
  <ul class="text-small-list-muted">
    <li><b>衰退期相關性提升</b>：當系統偵測到「衰退」時，產業間的衝擊傳導相關性自動提升 30%（Ang &amp; Chen 2002 實證），反映系統性風險主導下「同漲同跌」現象</li>
    <li><b>動態環境調變</b>：原油、美元指數、BDI 的 90 日滾動中位數基準，自動適應市場結構變化，消除靜態基準漂移</li>
    <li><b>方向性感知敘事</b>：油價暴漲 vs 暴跌對能源股的影響相反；系統依據實際 macro 數據偏差方向動態調整</li>
  </ul>
</div>

<div class="mb-14">
  <h4 class="h4-section-title">⚠️ 使用須知</h4>
  <p class="text-12-warn mb-0">週期羅盤是<b>輔助判斷工具</b>，非買賣建議。低信心度（&lt;40%）的燈號僅供參考，建議搭配產業詳情頁的供應鏈連動、季節性模式與風險指標綜合評估。<br>產業週期具有慣性，單一月份數據異常不一定代表階段轉換。</p>
</div>

<div class="control-group mt-14 justify-end">
  <button data-close-modal>關閉</button>
</div>
`;
