/**
 * shared/retail-sentiment-panel.js
 *
 * 散戶情緒 panel 的純函式渲染器。被下列模組引用：
 *   - pages/retail_sentiment.js（新獨立頁，PR #15xx）
 *   - pages/narrative.js（向後相容，若有 legacy code 仍呼叫）
 *
 * 設計：純函式，無副作用，只回傳 HTML 字串 + attach event listener 到指定容器。
 * 容器由呼叫方傳入（避免 hardcoded DOM id 衝突）。
 *
 * 後端：GET /api/dashboard/retail-sentiment
 * 後端 package：internal/retail（RSI-tw 計算）
 */

import { fmtSafeNumber, fmtSafeSignedPct } from './format-metric.js';

const RETRY_ID = 'retail-sentiment-panel';

const PANEL_HEADING = '散戶情緒指標';
const FETCHING_HEADING = '載入散戶情緒指標中…';
const FETCHING_MESSAGE = 'API 回應較慢（4-5 秒），請稍候。';

const COMPOSITE_HELP_INTRO = 'RSI-tw 綜合散戶情緒指數（Retail Sentiment Index — Taiwan）';
const COMPOSITE_PARTS = 'Part A（40%）：融資維持率、當沖比率、融資餘額變化、VIX風險映射、週選擇權PCR、零股失衡\nPart C（25%）：散戶期貨未平倉、券商分點流向、ETF申購\nPart D：事件調整乘數（0.8-1.2）';

const READING_LABEL = { frenzy: '狂熱', neutral: '中性', fear: '恐慌' };

function buildHelpText(intro, sections, currentStr, currentCondition) {
  // sections: array of [header, [lines]]
  return [
    intro,
    '',
    ...sections.flatMap(([header, lines]) => [header, ...lines.map(l => '• ' + l)]),
    '',
    '當前數值：' + currentStr + ' — ' + currentCondition,
  ].join('\n');
}

function fmtNumberOrDash(value, opts) {
  return value == null ? '—' : fmtSafeNumber(value, opts);
}

function fmtPctOrDash(value) {
  return value == null ? '—' : fmtSafeSignedPct(value);
}

function renderEmpty(container) {
  // 沿用 narrative.js 既有 renderActionEmptyState 行為
  container.innerHTML =
    '<div class="empty">' + FETCHING_HEADING + '</div>' +
    '<div class="text-muted text-sm" style="margin-top:8px;text-align:center">' + FETCHING_MESSAGE + '</div>';
}

export function renderRetailSentimentPanel(container, retailSentiment) {
  if (!container) return;
  container.classList.remove('loading');

  if (!retailSentiment) {
    renderEmpty(container);
    return;
  }

  const hasValidData = typeof retailSentiment.margin_balance === 'number' && retailSentiment.margin_balance > 0;
  const readingClass = retailSentiment.extreme_reading === 'frenzy' ? 'err' :
                       retailSentiment.extreme_reading === 'fear' ? 'warn' : 'ok';

  const sentimentScoreRaw = retailSentiment.sentiment_score;
  const marginChangeRaw = retailSentiment.margin_change_pct;
  const marginBalanceRaw = retailSentiment.margin_balance;
  const dayTradingRatioRaw = retailSentiment.day_trading_ratio;
  const marginPercentileRaw = retailSentiment.margin_percentile;
  const shortBalanceRaw = retailSentiment.short_balance;
  const shortChangeRaw = retailSentiment.short_change_pct;
  const compositeScoreRaw = retailSentiment.composite_sentiment;

  const score = fmtNumberOrDash(sentimentScoreRaw, { decimals: 2, useGrouping: true });
  const changeStr = fmtPctOrDash(marginChangeRaw);
  const changeClass = marginChangeRaw != null && marginChangeRaw >= 0 ? 'up' : 'down';
  const dataStatusBadge = hasValidData
    ? '<span class="badge ok">🟢 資料正常</span>'
    : '<span class="badge">🟡 資料待更新</span>';

  const marginPercentileValue = marginPercentileRaw != null ? marginPercentileRaw * 100 : null;
  const marginPercentileStr = fmtNumberOrDash(marginPercentileValue, { decimals: 0 });
  const marginBalanceStr = fmtNumberOrDash(marginBalanceRaw, { decimals: 0 });
  const dayTradingRatioValue = dayTradingRatioRaw != null ? dayTradingRatioRaw * 100 : null;
  const dayTradingRatioStr = fmtNumberOrDash(dayTradingRatioValue, { decimals: 1, suffix: '%' });
  const shortBalanceStr = fmtNumberOrDash(shortBalanceRaw, { decimals: 0 });
  const shortChangeStr = fmtPctOrDash(shortChangeRaw);
  const shortChangeClass = shortChangeRaw != null && shortChangeRaw >= 0 ? 'up' : 'down';
  const compositeScore = fmtNumberOrDash(compositeScoreRaw, { decimals: 2, useGrouping: true });
  const compositeClass = compositeScoreRaw != null && compositeScoreRaw > 0 ? 'up' :
                         compositeScoreRaw != null && compositeScoreRaw < 0 ? 'down' : 'warn';
  const compositeLabel = compositeScoreRaw != null && compositeScoreRaw > 0.5 ? '極度樂觀' :
                         compositeScoreRaw != null && compositeScoreRaw > 0 ? '偏多' :
                         compositeScoreRaw != null && compositeScoreRaw > -0.5 ? '偏空' : '極度恐慌';

  const sentimentHelp = '綜合融資餘額變化、當沖比率、散戶交易行為等指標計算出的散戶市場情緒指標。\n\n分數範圍：-1.0 ~ +1.0\n• ＞+0.5（狂熱）：散戶過度樂觀，融資大增、當沖猖獗，市場可能接近短期頂部\n• 0.0 ~ +0.5（偏多）：散戶積極參與，市場熱絡但尚未過熱\n• -0.5 ~ 0.0（偏空）：散戶趨於保守，融資減少，市場觀望氣氛濃厚\n• ＜-0.5（恐慌）：散戶極度悲觀，恐慌砍倉，歷史上常是階段性底部訊號\n\n當前數值：' + score + ' — ' + (sentimentScoreRaw > 0.5 ? '市場狂熱，建議減碼' : sentimentScoreRaw > 0 ? '散戶偏多' : sentimentScoreRaw > -0.5 ? '散戶偏空觀望' : '市場恐慌，可能接近底部');

  const marginChangeHelp = '融資餘額相對前一交易日的變化百分比。融資是散戶向券商借錢買股票的行為，是觀察散戶槓桿程度的重要指標。\n\n解讀標準：\n• ＞+5%：散戶瘋狂加碼，槓桿急速攀升，系統性風險劇增\n• +2% ~ +5%：散戶積極加槓桿，市場過熱跡象浮現\n• -2% ~ +2%：正常波動區間，散戶情緒平穩\n• -5% ~ -2%：散戶開始去槓桿，市場降溫\n• ＜-5%：散戶恐慌砍倉，融資大減，常伴隨市場急跌，但也可能是底部訊號\n\n當前數值：' + changeStr + ' — ' + (Math.abs(marginChangeRaw * 100) > 5 ? '散戶情緒劇烈波動' : Math.abs(marginChangeRaw * 100) > 2 ? '散戶情緒明顯變化' : '正常波動範圍');

  const marginBalanceHelp = '全市場散戶向券商融資買股票的總金額（單位：億元）。融資餘額越高代表散戶槓桿越大，市場風險越高。\n\n歷史百分位解讀：\n• ＞90th：極高水位，散戶槓桿處於歷史高檔，系統性回調風險極高\n• 70th ~ 90th：偏高水位，市場過熱，建議逐步降低持股\n• 30th ~ 70th：正常區間，風險可控\n• 10th ~ 30th：偏低水位，市場冷清，但可能是佈局時機\n• ＜10th：極低水位，散戶幾乎離場，歷史上常是長期底部區域\n\n當前數值：' + marginBalanceStr + ' 億（歷史 ' + marginPercentileStr + 'th 百分位）\n' + (marginPercentileValue > 90 ? '⚠️ 融資處於歷史極高水位，系統性風險極高，建議大幅減碼' : marginPercentileValue > 70 ? '⚡ 融資偏高，市場過熱，建議逐步獲利了結' : marginPercentileValue > 30 ? '✅ 融資水位正常，風險可控' : marginPercentileValue > 10 ? '💡 融資偏低，市場冷清，可關注佈局機會' : '📉 融資極低，散戶幾乎離場，可能是長期底部');

  const dayTradingHelp = '當日沖銷（Day Trading）成交量占總成交量的比例。當沖是散戶在同一天內買進又賣出的交易行為，是觀察市場投機程度的重要指標。\n\n解讀標準：\n• ＞40%：市場極度投機，散戶狂熱當沖，類似2021年航運股狂潮，短期崩盤風險極高\n• 30% ~ 40%：當沖比率偏高，市場投機氣氛濃厚，注意追高空單風險\n• 20% ~ 30%：正常偏高的當沖活動，市場熱絡但尚屬健康\n• 15% ~ 20%：當沖比率正常，市場交易穩定\n• ＜15%：當沖冷清，市場缺乏投機動能，散戶參與度低\n\n當前數值：' + dayTradingRatioStr + ' — ' + (dayTradingRatioValue > 40 ? '市場極度投機，高風險警戒！' : dayTradingRatioValue > 30 ? '當沖比率偏高，注意風險' : dayTradingRatioValue > 20 ? '當沖活躍，市場熱絡' : dayTradingRatioValue > 15 ? '當沖比率正常' : '當沖冷清，市場觀望');

  const shortBalanceHelp = '全市場散戶向券商融券賣股票的總金額（單位：億元）。融券餘額越高代表散戶看空力道越強，是觀察市場空方情緒的重要指標。\n\n解讀標準：\n• 融券餘額大幅上升：散戶積極做空，市場看空情緒濃厚\n• 融券餘額大幅下降：散戶回補空單，空方力道減弱，可能出現軋空行情\n• 融資/融券比率異常：若融資高但融券也高，代表市場分歧加大\n\n當前數值：' + shortBalanceStr + ' 億（變化 ' + shortChangeStr + '）\n' + (shortChangeRaw > 0.05 ? '⚠️ 融券大幅增加，散戶積極做空' : shortChangeRaw < -0.05 ? '📈 融券大幅減少，空方回補，注意軋空風險' : '✅ 融券變化正常');

  const compositeHelp = COMPOSITE_HELP_INTRO + '\n\n' + COMPOSITE_PARTS + '\n\n分數範圍：-1.0 ~ +1.0\n• ＞+0.5：散戶狂熱，市場接近短期頂部\n• +0.2 ~ +0.5：散戶偏多\n• -0.2 ~ +0.2：中性\n• -0.5 ~ -0.2：散戶偏空\n• ＜-0.5：散戶恐慌，可能是底部訊號\n\n當前數值：' + compositeScore + ' — ' + compositeLabel;

  const hasSubIndicators = retailSentiment.sentiment_sub_indicators &&
      (retailSentiment.sentiment_sub_indicators.category_a || retailSentiment.sentiment_sub_indicators.category_c);

  let subIndicatorHTML = '';
  if (hasSubIndicators) {
    const si = retailSentiment.sentiment_sub_indicators;
    const ca = si.category_a || {};
    const cc = si.category_c || {};
    const cd = si.category_d || {};

    // Audit A01 (2026-08-12): fallback 子指標顯示「資料缺失」標記，
    // 不再把 fallback 值（0.5/0.0）冒充為真實數值。
    const fallbackBadge = (raw, fallbackFlag) => {
      if (fallbackFlag) {
        return ' <span class="badge" style="font-size:10px;padding:1px 6px" title="資料缺失，此數值為預設值">資料缺失</span>';
      }
      return '';
    };

    const aIndicatorRows = [
      ['融資餘額百分位', ca.margin_maintenance_z, ca.is_fallback], // Audit A11: 原名「維持率 Z-score」誤導（實為融資餘額百分位映射）
      ['當沖比率', ca.day_trading_z, ca.is_fallback],              // Audit A12: 原名「當沖 Z-score」（實為原始比率）
      ['融資餘額 Z-score', ca.margin_balance_z, ca.is_fallback],
      ['VIX 風險分數', ca.vix_risk_score, ca.is_fallback],
      ['週選擇權 PCR', ca.weekly_pcr, ca.is_fallback],
      ['零股交易失衡', ca.odd_lot_imbalance, ca.is_fallback]
    ].map(r => {
      const raw = r[1];
      const v = fmtSafeNumber(raw, { decimals: 2, useGrouping: true });
      const cls = raw != null && raw > 0.5 ? 'up' : raw != null && raw < -0.5 ? 'down' : '';
      return '<tr><td style="font-size:12px;padding:3px 8px">' + r[0] + '</td><td style="font-size:12px;text-align:right;padding:3px 8px" class="' + cls + '">' + v + fallbackBadge(r[1], r[2]) + '</td></tr>';
    }).join('');

    const cIndicatorRows = [
      ['散戶期貨 OI', cc.futures_retail_oi, cc.is_fallback],
      ['外資+投信淨買超', cc.broker_flow_score, cc.is_fallback], // Audit A07: 原名「券商分點流向」與實際（外資+投信）不符
      ['ETF 申購分數', cc.etf_subscription_score, cc.is_fallback]
    ].map(r => {
      const raw = r[1];
      const v = fmtSafeNumber(raw, { decimals: 2, useGrouping: true });
      const cls = raw != null && raw > 0.5 ? 'up' : raw != null && raw < -0.5 ? 'down' : '';
      return '<tr><td style="font-size:12px;padding:3px 8px">' + r[0] + '</td><td style="font-size:12px;text-align:right;padding:3px 8px" class="' + cls + '">' + v + fallbackBadge(r[1], r[2]) + '</td></tr>';
    }).join('');

    const dEvents = (cd.active_events && cd.active_events.length > 0) ? cd.active_events.join('、') : '無觸發事件';
    const dAdj = cd.adjustment_factor || cd.d_multiplier || 1.0;
    const dAdjClass = dAdj < 0.95 ? 'warn' : dAdj > 1.05 ? 'up' : '';

    subIndicatorHTML =
      '<div class="mt-sm" style="border:1px solid var(--border);border-radius:6px;overflow:hidden">' +
        '<div id="subIndicatorToggle" style="display:flex;align-items:center;justify-content:space-between;padding:8px 12px;cursor:pointer;background:var(--bg);user-select:none" data-toggle="subIndicators">' +
          '<span style="font-size:12px;font-weight:600;color:var(--accent)">📊 子指標明細</span>' +
          '<span id="subIndicatorArrow" style="font-size:11px;transition:transform 0.2s">▲</span>' +
        '</div>' +
        '<div id="subIndicatorBody" style="display:block;padding:10px 12px;border-top:1px solid var(--border)">' +
          '<div style="margin-bottom:10px">' +
            '<div style="font-size:12px;font-weight:600;margin-bottom:6px;color:var(--accent)">Part A（40%）— 散戶情緒 <span style="font-weight:400;font-size:11px;color:var(--text-muted)">A Score: ' + fmtSafeNumber(ca.a_score, { decimals: 3 }) + '</span></div>' +
            '<table style="width:100%;border-collapse:collapse">' + aIndicatorRows + '</table>' +
          '</div>' +
          '<div style="margin-bottom:10px">' +
            '<div style="font-size:12px;font-weight:600;margin-bottom:6px;color:var(--accent)">Part C（25%）— 機構/衍生品流向 <span style="font-weight:400;font-size:11px;color:var(--text-muted)">C Score: ' + fmtSafeNumber(cc.c_score, { decimals: 3 }) + '</span></div>' +
            '<table style="width:100%;border-collapse:collapse">' + cIndicatorRows + '</table>' +
          '</div>' +
          '<div>' +
            '<div style="font-size:12px;font-weight:600;margin-bottom:6px;color:var(--accent)">Part D — 事件調整 <span style="font-weight:400;font-size:11px;color:var(--text-muted)">乘數: <span class="' + dAdjClass + '">' + fmtSafeNumber(dAdj, { decimals: 3 }) + '</span></span></div>' +
            '<div style="font-size:11px;color:var(--text)">' + dEvents + '</div>' +
          '</div>' +
        '</div>' +
      '</div>';
  }

  container.innerHTML =
    '<div style="display:flex;align-items:center;gap:10px;margin-bottom:10px">' +
      '<span class="text-muted text-sm">' + PANEL_HEADING + '</span>' +
      '<span class="badge ' + readingClass + '">' + (READING_LABEL[retailSentiment.extreme_reading] || retailSentiment.extreme_reading) + '</span>' +
      dataStatusBadge +
    '</div>' +
    '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:10px">' +
      '<div class="kpi-card" style="cursor:pointer;" data-help="' + compositeHelp.replace(/"/g, '&quot;') + '" data-title="RSI-tw 綜合指數說明">' +
        '<div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">RSI-tw 綜合 ℹ️</div>' +
        '<div class="kpi-value ' + compositeClass + ' text-lg">' + compositeScore + '</div>' +
      '</div>' +
      '<div class="kpi-card" style="cursor:pointer;" data-help="' + sentimentHelp.replace(/"/g, '&quot;') + '" data-title="情緒分數說明">' +
        '<div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">情緒分數 ℹ️</div>' +
        '<div class="kpi-value text-lg">' + score + '</div>' +
      '</div>' +
      '<div class="kpi-card" style="cursor:pointer;" data-help="' + marginChangeHelp.replace(/"/g, '&quot;') + '" data-title="融資變化說明">' +
        '<div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">融資變化 ℹ️</div>' +
        '<div class="kpi-value ' + changeClass + ' text-lg">' + changeStr + '</div>' +
      '</div>' +
      '<div class="kpi-card" style="cursor:pointer;" data-help="' + marginBalanceHelp.replace(/"/g, '&quot;') + '" data-title="融資餘額說明">' +
        '<div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">融資餘額 ℹ️</div>' +
        '<div class="kpi-value text-lg">' + marginBalanceStr + ' 億</div>' +
      '</div>' +
      '<div class="kpi-card" style="cursor:pointer;" data-help="' + dayTradingHelp.replace(/"/g, '&quot;') + '" data-title="當沖比率說明">' +
        '<div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">當沖比率 ℹ️</div>' +
        '<div class="kpi-value text-lg">' + dayTradingRatioStr + '</div>' +
      '</div>' +
      '<div class="kpi-card" style="cursor:pointer;" data-help="' + shortBalanceHelp.replace(/"/g, '&quot;') + '" data-title="融券餘額說明">' +
        '<div class="kpi-label" style="color:var(--accent);text-decoration:underline dotted;">融券餘額 ℹ️</div>' +
        '<div class="kpi-value ' + shortChangeClass + ' text-lg">' + shortBalanceStr + ' 億</div>' +
      '</div>' +
    '</div>' +
    '<div class="mt-sm text-muted text-sm">歷史百分位: ' + marginPercentileStr + 'th</div>' +
    subIndicatorHTML;

  // attach event listeners
  container.querySelectorAll('.kpi-card[data-help]').forEach(card => {
    card.addEventListener('click', function() {
      const title = this.getAttribute('data-title');
      const helpText = this.getAttribute('data-help');
      const htmlContent = '<p>' + helpText.replace(/\\n\\n/g, '</p><p>').replace(/\\n/g, '<br>') + '</p>';
      if (typeof window.openInfoHelp === 'function') {
        window.openInfoHelp(title, htmlContent);
      } else if (typeof openInfoHelp === 'function') {
        openInfoHelp(title, htmlContent);
      }
    });
  });

  // sub-indicator accordion (no inline onclick; uses data-toggle)
  const toggle = container.querySelector('[data-toggle="subIndicators"]');
  if (toggle) {
    toggle.addEventListener('click', function() {
      const body = container.querySelector('#subIndicatorBody');
      const arrow = container.querySelector('#subIndicatorArrow');
      if (!body || !arrow) return;
      const isHidden = body.style.display === 'none' || body.style.display === '';
      body.style.display = isHidden ? 'block' : 'none';
      arrow.textContent = isHidden ? '▲' : '▼';
      arrow.style.transform = isHidden ? 'rotate(180deg)' : 'rotate(0deg)';
    });
  }
}

export const RETRY_ID_CONST = RETRY_ID;
