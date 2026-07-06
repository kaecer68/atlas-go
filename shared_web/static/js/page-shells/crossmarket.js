export const template = `
  <div class="page-header">
    <p class="page-desc">美股對台股傳導通道即時觀測 — 指數、科技股、波動率、相關性、危機信號</p>
  </div>
  <div class="section">
    <h3>🚨 危機信號</h3>
    <div class="panel" id="cm-crisis"></div>
  </div>
  <div class="section mt-16">
    <h3>📊 美國主要指數</h3>
    <div class="kpi-grid" id="cm-us-indices"></div>
  </div>
  <div class="section mt-16">
    <h3>💻 半導體與科技股</h3>
    <div class="kpi-grid" id="cm-tech-stocks"></div>
  </div>
  <div class="section mt-16">
    <h3>🌐 跨市場宏觀指標</h3>
    <div class="kpi-grid" id="cm-macro"></div>
  </div>
  <div class="section mt-16">
    <h3>📈 動態相關性</h3>
    <div class="panel" id="cm-correlation"></div>
  </div>
  <div class="section mt-16">
    <h3>🔄 產業相關性矩陣</h3>
    <div class="panel" id="cm-correlation-matrix"></div>
  </div>
`;
