import { escapeHtml } from '../shared/app-utils.js';

export function dataQualityBadge(channelMap, channelIds) {
  if (!channelMap || !Array.isArray(channelIds) || channelIds.length === 0) return '';
  const bad = [];
  for (const id of channelIds) {
    const ch = channelMap[id];
    if (ch && ch.status !== 'ok') bad.push(ch.label || ch.channel_id || id);
  }
  if (bad.length === 0) return '';
  const title = '以下資料通道異常：' + bad.join('、');
  return `<span class="data-quality-badge" title="${escapeHtml(title)}">資料異常</span>`;
}

export function buildChannelMap(dataChannels) {
  const map = {};
  if (!Array.isArray(dataChannels)) return map;
  for (const ch of dataChannels) {
    if (ch && ch.channel_id) map[ch.channel_id] = ch;
  }
  return map;
}
