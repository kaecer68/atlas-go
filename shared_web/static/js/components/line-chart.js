// line-chart.js — 共用雙線折線圖 canvas helper（SSOT Phase 2, P2-4）
// 抽自 sparkline.js 的 renderDualEquityCurve：淨值趨勢（稅前/稅後）與
// 基準比較（投組 vs TAIEX）等「雙線 + 時間軸」圖表共用同一繪圖邏輯。
//
// 用法：
//   import { renderDualLineChart } from './line-chart.js';
//   renderDualLineChart({
//     canvas,
//     yFormat: fmtNTD,                      // Y 軸刻度格式（預設原值）
//     series: [                             // 至少 2 點才畫該線
//       { label: '稅前淨值', color: '#4fc1ff', points: [{ label: '2026-08-01', value: 123 }] },
//       { label: '稅後淨值', color: '#f59e0b', points: [...] },
//     ],
//   });
//
// 回傳 boolean：true = 有畫出圖（至少一條線 ≥2 點）；false = 資料不足，呼叫端自行隱藏面板。
import { getThemeColor } from '../shared/utils.js';
import { hexToRgba } from '../shared/color-tokens.js';

function isFiniteNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

/**
 * 通用雙線折線圖（canvas，DPR-aware）。雙軸以全部 series 數值縮放，
 * 支援稀疏 X 標籤、漸層填色、圖例與圓點。
 */
export function renderDualLineChart(opts) {
  if (!opts || !opts.canvas) return false;
  const canvas = opts.canvas;
  const series = Array.isArray(opts.series) ? opts.series : [];

  // 每條線只保留有限數值點；少於 2 點的線不畫
  const usable = series
    .map(function (s) {
      const points = (s && Array.isArray(s.points) ? s.points : [])
        .filter(function (p) { return p && isFiniteNumber(p.value); });
      return { label: s.label || '', color: s.color || 'var(--accent)', points: points };
    })
    .filter(function (s) { return s.points.length >= 2; });

  if (usable.length === 0) return false;

  const allValues = [];
  usable.forEach(function (s) { s.points.forEach(function (p) { allValues.push(p.value); }); });
  if (allValues.length === 0) return false;

  const minV = Math.min.apply(null, allValues);
  const maxV = Math.max.apply(null, allValues);
  const range = maxV - minV || 1;

  const yFormat = (typeof opts.yFormat === 'function') ? opts.yFormat : function (v) { return String(v); };
  const H = opts.height || 220;

  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.parentElement.getBoundingClientRect();
  const W = Math.max(120, rect.width - 40);
  canvas.width = W * dpr; canvas.height = H * dpr;
  canvas.style.width = W + 'px'; canvas.style.height = H + 'px';
  ctx.scale(dpr, dpr);

  const pad = { top: 20, right: 20, bottom: 28, left: 80 }; // 左側預留幣別標籤
  const chartW = W - pad.left - pad.right;
  const chartH = H - pad.top - pad.bottom;

  ctx.clearRect(0, 0, W, H);

  // 面板底色
  ctx.fillStyle = hexToRgba(getThemeColor('--panel') || '#13161c', 0.6);
  ctx.beginPath();
  if (ctx.roundRect) ctx.roundRect(pad.left, pad.top, chartW, chartH, 6);
  else ctx.rect(pad.left, pad.top, chartW, chartH);
  ctx.fill();

  // 水平格線
  ctx.strokeStyle = hexToRgba(getThemeColor('--text') || '#f0f4f8', 0.05);
  ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + chartW, y); ctx.stroke();
  }

  // Y 軸刻度
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.6);
  ctx.font = '10px system-ui';
  ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    const val = maxV - (range / 4) * i;
    ctx.fillText(yFormat(val), pad.left - 8, y + 3);
  }

  function xFor(points, i) {
    return pad.left + (points.length <= 1 ? 0 : (i / (points.length - 1)) * chartW);
  }
  function yFor(v) {
    return pad.top + (1 - (v - minV) / range) * chartH;
  }

  usable.forEach(function (s) {
    const colorHex = s.color;
    // 漸層填色
    const gradient = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
    gradient.addColorStop(0, hexToRgba(colorHex, 0.25));
    gradient.addColorStop(1, hexToRgba(colorHex, 0.02));
    ctx.beginPath();
    ctx.moveTo(xFor(s.points, 0), pad.top + chartH);
    for (let i = 0; i < s.points.length; i++) ctx.lineTo(xFor(s.points, i), yFor(s.points[i].value));
    ctx.lineTo(xFor(s.points, s.points.length - 1), pad.top + chartH);
    ctx.closePath();
    ctx.fillStyle = gradient;
    ctx.fill();

    // 主線
    ctx.save();
    ctx.shadowColor = hexToRgba(colorHex, 0.4);
    ctx.shadowBlur = 6;
    ctx.strokeStyle = colorHex;
    ctx.lineWidth = 2.2;
    ctx.lineJoin = 'round';
    ctx.beginPath();
    s.points.forEach(function (p, i) {
      if (i === 0) ctx.moveTo(xFor(s.points, i), yFor(p.value));
      else ctx.lineTo(xFor(s.points, i), yFor(p.value));
    });
    ctx.stroke();
    ctx.restore();

    // 少量點才畫圓點（避免 186 點塞滿）
    if (s.points.length <= 30) {
      ctx.fillStyle = colorHex;
      s.points.forEach(function (p) {
        ctx.beginPath();
        ctx.arc(xFor(s.points, s.points.indexOf(p)), yFor(p.value), 2.5, 0, Math.PI * 2);
        ctx.fill();
      });
    }
  });

  // X 軸標籤（以第一條線為基準，稀疏顯示）
  const labelSource = usable[0].points;
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.5);
  ctx.font = '9px system-ui';
  ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(labelSource.length / 6));
  labelSource.forEach(function (p, i) {
    if (i % step === 0 || i === labelSource.length - 1) {
      ctx.fillText(p.label || '', xFor(labelSource, i), pad.top + chartH + 18);
    }
  });

  // 圖例（第一條在左，其餘依序向右）
  if (usable.length > 1 || opts.alwaysLegend) {
    ctx.font = '10px system-ui';
    ctx.textAlign = 'left';
    let legendX = pad.left + 10;
    usable.forEach(function (s) {
      ctx.fillStyle = s.color;
      ctx.fillRect(legendX, pad.top + 10, 10, 10);
      ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.8);
      ctx.fillText(s.label || '', legendX + 15, pad.top + 19);
      legendX += 15 + ctx.measureText(s.label || '').width + 24;
    });
  }
  return true;
}

