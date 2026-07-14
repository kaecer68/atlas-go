/**
 * capital-board.js — 錢潮看板
 *
 * 顯示目前 atlas 啟用模型（含 favored_sectors / avoided_sectors 圖示）。
 * Source: /api/narrative/models
 */

import { silentGetJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { financialColor } from '../shared/color-tokens.js';

const SECTOR_ALIAS = {
  '半導體業': '半導體',
  '半導體產業': '半導體',
  'AI伺服器產業': 'AI',
  'AI伺服器': 'AI',
  '電子零組件業': '電子零組件',
  '金融業': '金融',
  '金融股': '金融',
  '外銷產業': '外銷',
  '傳產股': '傳產',
  '傳統產業': '傳產',
};

function canonicalSector(name) {
  return SECTOR_ALIAS[name] || name;
}

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  錢潮看板顯示 atlas narrative engine 目前啟用的模型，每個模型依 weight
  比例列出「看好」與「看壞」的板塊。weight 越高代表該模型對近期方向的影響
  越強。各模型看好/看壞的板塊可能有重疊（信心強）或分散（信心弱）。
</details>
<section id="cb-grid" class="cb-board" aria-live="polite">載入中…section>
`;

function renderWeights(models) {
  return models.map(function (m) {
    const w = typeof m.weight === 'number' ? m.weight : 0;
    const pct = Math.round(w * 100);
    const reason = m.rationale ? String(m.rationale) : '';
    return (
      '<div class="cb-model">' +
      '<div class="cb-model__head">' +
      '<span class="cb-model__name">' + escapeHtml(m.name || m.id || '未命名') + '</span>' +
      '<span class="cb-model__weight">權重 ' + pct + '%</span>' +
      '</div>' +
      '<div class="cb-model__bar" aria-hidden="true">' +
      '<div class="cb-model__bar-fill" style="width:' + pct + '%;background:' + financialColor(w >= 1 ? 1 : 0, 'flow') + '"></div>' +
      '</div>' +
      (reason ? '<div class="cb-model__rationale text-muted">' + escapeHtml(reason) + '</div>' : '') +
      '</div>'
    );
  }).join('');
}

function renderSectors(models) {
  const allSectors = [];
  models.forEach(function (m) {
    const favored = Array.isArray(m.favored_sectors) ? m.favored_sectors : [];
    const avoided = Array.isArray(m.avoided_sectors) ? m.avoided_sectors : [];
    favored.forEach(function (s) { allSectors.push({ name: canonicalSector(s), vote: 'favored', weight: m.weight || 0 }); });
    avoided.forEach(function (s) { allSectors.push({ name: canonicalSector(s), vote: 'avoided', weight: m.weight || 0 }); });
  });

  if (!allSectors.length) {
    return '<div class="cb-board__sectors text-muted">目前模型未提供 sector 偏好資料（顯示原始理由）</div>';
  }

  const grouped = {};
  allSectors.forEach(function (entry) {
    if (!grouped[entry.name]) grouped[entry.name] = { favored: 0, avoided: 0, total: 0 };
    grouped[entry.name][entry.vote] += entry.weight;
    grouped[entry.name].total += entry.weight;
  });

  const entries = Object.keys(grouped).map(function (name) {
    const g = grouped[name];
    const net = g.favored - g.avoided;
    return { name: name, favored: g.favored, avoided: g.avoided, total: g.total, net: net };
  }).sort(function (a, b) { return Math.abs(b.net) - Math.abs(a.net); });

  return (
    '<div class="cb-board__sectors">' +
    entries.map(function (e) {
      const verdict = e.net > 0.05 ? 'favored' : e.net < -0.05 ? 'avoided' : 'neutral';
      const verdictLabel = verdict === 'favored' ? '看好' : verdict === 'avoided' ? '看壞' : '中性';
      const verdictColor = financialColor(verdict === 'favored' ? 1 : verdict === 'avoided' ? -1 : 0, 'flow');
      return (
        '<div class="cb-sector-row" data-verdict="' + verdict + '">' +
        '<span class="cb-sector-row__name">' + escapeHtml(e.name) + '</span>' +
        '<span class="cb-sector-row__bar" aria-hidden="true">' +
        (e.favored > 0 ? '<span style="width:' + (e.favored / e.total * 100).toFixed(1) + '%;background:rgba(34,139,34,0.45)"></span>' : '') +
        (e.avoided > 0 ? '<span style="width:' + (e.avoided / e.total * 100).toFixed(1) + '%;background:rgba(178,34,34,0.45)"></span>' : '') +
        '</span>' +
        '<span class="cb-sector-row__verdict" style="color:' + verdictColor + '">' + verdictLabel + '</span>' +
        '</div>'
      );
    }).join('') +
    '</div>'
  );
}

async function loadModels() {
  const data = await silentGetJSON('/api/narrative/models');
  const models = (data && Array.isArray(data.models)) ? data.models : [];
  const grid = document.getElementById('cb-grid');

  if (!models.length) {
    grid.innerHTML = '<div class="empty">目前無啟用中的模型</div>';
    return;
  }

  grid.innerHTML = (
    '<div class="cb-board__weights">' + renderWeights(models) + '</div>' +
    '<h3 class="cb-board__title">板塊看好 / 看壞彙總</h3>' + renderSectors(models)
  );
}

export async function init() {
  await loadModels();
}
