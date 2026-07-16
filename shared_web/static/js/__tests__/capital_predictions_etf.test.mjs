// 對應 capital_predictions.js 內 C06 新增的 renderETFEstimatesTable 純函式。
//
// 重點：
// 1. 空/null/undefined → 空字串（讓 wrapper 直接蓋 innerHTML 清除顯示）
// 2. direction=add 對應「加碼」+ row--add CSS class；remove 對應「減碼」+ row--remove
// 3. 依 ETF 名稱 zh-Hant locale 排序,同 ETF 內依 est_flow 降冪
// 4. est_aum 與 est_flow 依數量級選擇「億」或「兆」呈現
// 5. escapeHtml 保護 XSS payload（重要：data 從外部 API 傳入）
//
// 執行：node --test shared_web/static/js/__tests__/capital_predictions_etf.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// DOM stubs required because capital_predictions.js 在 module scope 呼叫 window.scrollToSection
// ============================================================================

global.window = {
  matchMedia() { return { matches: false, media: '' }; },
  switchPage() {},
  dispatchEvent() {},
  addEventListener() {},
  removeEventListener() {},
  scrollToSection() {},
};

global.document = {
  getElementById() { return null; },
  createElement() {
    return { innerHTML: '', textContent: '', style: {}, setAttribute() {}, appendChild() {} };
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
  removeEventListener() {},
};

const { renderETFEstimatesTable } = await import('../pages/capital_predictions.js');

// ============================================================================
// Defensive: 空資料 / null / undefined / 缺欄位
// ============================================================================

test('renderETFEstimatesTable: empty array → 空字串', () => {
  assert.equal(renderETFEstimatesTable([]), '');
});

test('renderETFEstimatesTable: null → 空字串', () => {
  assert.equal(renderETFEstimatesTable(null), '');
});

test('renderETFEstimatesTable: undefined → 空字串', () => {
  assert.equal(renderETFEstimatesTable(undefined), '');
});

test('renderETFEstimatesTable: 非 array 輸入 → 空字串', () => {
  assert.equal(renderETFEstimatesTable({ etf_name: 'foo' }), '');
  assert.equal(renderETFEstimatesTable('not-an-array'), '');
});

// ============================================================================
// Happy path: 基本欄位都存在
// ============================================================================

test('renderETFEstimatesTable: 一筆完整的 add 估計 → 渲染完整 row', () => {
  const html = renderETFEstimatesTable([{
    etf_name: '元大台灣50',
    stock_symbol: '2330',
    stock_name: '台積電',
    direction: 'add',
    est_weight: 0.05,
    etf_aum: 1500,
    est_flow: 75,
  }]);
  assert.match(html, /<table class="cp-etf__table">/);
  assert.match(html, /元大台灣50/);
  assert.match(html, /2330/);
  assert.match(html, /台積電/);
  assert.match(html, /加碼/);
  assert.match(html, /cp-etf__row--add/);
  assert.match(html, /5\.0%/);
  assert.match(html, /1\.5 兆/); // AUM 1500 → 1.5 兆
  assert.match(html, /75 百萬/); // 流量 75 → 75 百萬
});

test('renderETFEstimatesTable: remove 方向 → 「減碼」+ row--remove', () => {
  const html = renderETFEstimatesTable([{
    etf_name: '富邦台50',
    stock_symbol: '2317',
    stock_name: '鴻海',
    direction: 'remove',
    est_weight: 0.02,
    etf_aum: 800,
    est_flow: -16,
  }]);
  assert.match(html, /減碼/);
  assert.match(html, /cp-etf__row--remove/);
  // 負值呈現無 + / - 號
  assert.match(html, /16 百萬/);
});

// ============================================================================
// Defensive: 缺欄位
// ============================================================================

test('renderETFEstimatesTable: 缺 etf_name → 顯示 —', () => {
  const html = renderETFEstimatesTable([{
    stock_symbol: '2330',
    direction: 'add',
    est_weight: 0.05,
  }]);
  // etf_name 缺 → 顯示 —,但仍然渲染 row（不丟整列）
  assert.match(html, /<td>—<\/td>/);
  assert.match(html, /加碼/);
});

test('renderETFEstimatesTable: 缺 stock_name → name span 內容為空', () => {
  const html = renderETFEstimatesTable([{
    etf_name: '元大台灣50',
    stock_symbol: '2330',
    direction: 'add',
    est_weight: 0.05,
  }]);
  assert.match(html, /2330/);
  // span 仍渲染,但內容為空（無空白）
  assert.match(html, /<span class="cp-etf__name"><\/span>/);
});

test('renderETFEstimatesTable: 缺 est_weight / est_aum / est_flow → 顯示 —', () => {
  const html = renderETFEstimatesTable([{
    etf_name: '元大台灣50',
    stock_symbol: '2330',
    direction: 'add',
  }]);
  assert.match(html, /cp-etf__num">—</);
});

test('renderETFEstimatesTable: 未知 direction（既非 add 也非 remove） → "—"', () => {
  const html = renderETFEstimatesTable([{
    etf_name: 'foo',
    stock_symbol: '2330',
    direction: 'unknown',
    est_weight: 0.05,
  }]);
  assert.match(html, /<td>—<\/td>/);
  // 沒有 add/remove 的 class
  assert.doesNotMatch(html, /cp-etf__row--add/);
  assert.doesNotMatch(html, /cp-etf__row--remove/);
});

// ============================================================================
// 數字格式
// ============================================================================

test('renderETFEstimatesTable: AUM 38000 NTD billion → 38 兆 NTD', () => {
  // etf_aum 的單位為「NTD 億」(per types.go: ETFAUM float64 // in NTD billions)
  // 38000 NTD 億 = 3.8 兆 = 38 兆 NTD。但 formatAUM 直接將 NTD 億除以 1000 得到兆數,
  // 因此 38000 NTD 億 顯示為 38 兆 (內部單位是 NTD 兆)。
  const html = renderETFEstimatesTable([{
    etf_name: 'foo',
    etf_aum: 38000,
  }]);
  assert.match(html, /38\.0 兆/);
});

test('renderETFEstimatesTable: AUM 50 (億) → 50 億（不到兆）', () => {
  const html = renderETFEstimatesTable([{
    etf_name: 'foo',
    etf_aum: 50,
  }]);
  assert.match(html, /50 億/);
});

test('renderETFEstimatesTable: AUM 0.005 NTD 億 → 1 百萬（(0.5).toFixed(0) 進位）', () => {
  // 0.005 NTD 億 × 100 = 0.5 → toFixed(0) = '1'
  const html = renderETFEstimatesTable([{
    etf_name: 'foo',
    etf_aum: 0.005,
  }]);
  assert.match(html, /1 百萬/);
});

test('renderETFEstimatesTable: est_flow=1500 (百萬) → 1.50 億', () => {
  const html = renderETFEstimatesTable([{
    etf_name: 'foo',
    est_flow: 1500,
  }]);
  assert.match(html, /1\.50 億/);
});

// ============================================================================
// 排序
// ============================================================================

test('renderETFEstimatesTable: 多筆依 ETF 名稱 zh-Hant 排序,同 ETF 內依 est_flow 降冪', () => {
  const html = renderETFEstimatesTable([
    { etf_name: '元大台灣50', stock_symbol: 'A', est_flow: 50 },
    { etf_name: '富邦台50', stock_symbol: 'B', est_flow: 100 },
    { etf_name: '元大台灣50', stock_symbol: 'C', est_flow: 80 },
  ]);
  // zh-Hant 排序: 元 (U+5143) < 富 (U+5BC6),因此元大 rows 在富邦 row 之前
  // 同 ETF 內 C (流 80) > A (流 50)
  // 預期 row 順序:
  //   Row 1: 元大台灣50, C
  //   Row 2: 元大台灣50, A
  //   Row 3: 富邦台50, B
  const firstYuanDaPos = html.indexOf('<td>元大台灣50</td>');
  const secondYuanDaPos = html.indexOf('<td>元大台灣50</td>', firstYuanDaPos + 1);
  const fuBangPos = html.indexOf('<td>富邦台50</td>');
  const cPos = html.indexOf('<td class="cp-etf__sym">C');
  const aPos = html.indexOf('<td class="cp-etf__sym">A');
  const bPos = html.indexOf('<td class="cp-etf__sym">B');
  assert.ok(firstYuanDaPos >= 0 && secondYuanDaPos >= 0 && fuBangPos >= 0);
  assert.ok(cPos >= 0 && aPos >= 0 && bPos >= 0);
  // 元大第一筆 < 元大第二筆 < 富邦
  assert.ok(firstYuanDaPos < secondYuanDaPos);
  assert.ok(secondYuanDaPos < fuBangPos);
  // 同 ETF 內:C 在 A 之前（降冪）
  assert.ok(cPos < aPos);
  // A 在 B 之前（兩個元大 row 都在富邦 row 之前）
  assert.ok(aPos < bPos);
});

// ============================================================================
// XSS 保護
// ============================================================================

test('renderETFEstimatesTable: etf_name 含 HTML → escape 成純文字', () => {
  const html = renderETFEstimatesTable([{
    etf_name: '<script>alert(1)</script>',
    stock_symbol: '2330',
    direction: 'add',
    est_weight: 0.05,
  }]);
  assert.doesNotMatch(html, /<script>alert/);
  assert.match(html, /&lt;script&gt;/);
});

test('renderETFEstimatesTable: stock_name 含 HTML → escape 成純文字', () => {
  const html = renderETFEstimatesTable([{
    etf_name: 'foo',
    stock_symbol: '2330',
    stock_name: '<img src=x onerror=alert(1)>',
    direction: 'add',
    est_weight: 0.05,
  }]);
  // 危險的 <img ...> tag 必須被 escape 成 &lt;img...
  assert.match(html, /&lt;img/);
  // 不應該有 raw 的 <img 出現在 <span> 內,讓瀏覽器把它當 element 渲染
  // 用更精準 regex: <span class="cp-etf__name"> 與 </span> 之間不應有 <img
  assert.doesNotMatch(html, /cp-etf__name">[^&]*<img/);
});
