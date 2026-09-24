import { escapeHtml } from '../shared/app-utils.js';
import { channelStatusMeta, worstChannelStatus } from '../shared/channel-status.js';

export function dataQualityBadge(channelMap, channelIds) {
  if (!channelMap || !Array.isArray(channelIds) || channelIds.length === 0) return '';
  const flagged = [];
  for (const id of channelIds) {
    const ch = channelMap[id];
    if (!ch) continue;
    const meta = channelStatusMeta(ch.status);
    // Flag everything that is not plainly normal, EXCEPT operator-disabled
    // channels (inactive = a decision, not a data-quality problem). `unknown` is
    // flagged on purpose: a missing verdict must not read as「正常」.
    if (!meta.needsAttention) continue;
    flagged.push({ name: ch.label || ch.channel_id || id, meta });
  }
  if (flagged.length === 0) return '';
  // Badge text = the worst status's own label (異常 / 資料過期 / 降級 / 待更新),
  // never a blanket「資料異常」that paints an amber state as a red failure.
  const worst = channelStatusMeta(worstChannelStatus(flagged.map(f => f.meta.status)));
  const title = '以下資料通道狀態需注意：' + flagged.map(f => `${f.name}（${f.meta.label}）`).join('、');
  return `<span class="data-quality-badge" title="${escapeHtml(title)}">${escapeHtml(worst.label)}</span>`;
}

export function buildChannelMap(dataChannels) {
  const map = {};
  if (!Array.isArray(dataChannels)) return map;
  for (const ch of dataChannels) {
    if (ch && ch.channel_id) map[ch.channel_id] = ch;
  }
  return map;
}
