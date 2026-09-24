// Channel-status SSOT (frontend) — fix/20260924-channel-status-truth.
//
// ONE mapping from a channel `status` string to its label, tone and
// "is it normal?" verdict, shared by every page that renders channel health,
// so the admin data-channel page, the overview KPI, the alerts health summary
// and the home data-quality badge cannot disagree.
//
// The bug this fixes (2026-09-24): `twse_oddlot` carried a record that said
// `ok`, but its last successful fetch was 17 days old. The backend now derives
// `stale` for that (see internal/apigateway/channel_status.go) and returns
// status_text「資料過期」, yet /admin/datachannels still printed「正常」because
// the frontend only knew ok/warn/error and computed
// `正常 = total - error - warn`.
//
// Backend contract (do NOT invent values here):
//   internal/monitoring/service/session.go  StatusText()          → labels
//   internal/apigateway/channel_status.go                          → ok / warn /
//     error / degraded / inactive / stale / unknown
//   internal/monitoring/service/system.go                          → expected_delay
//
// Tones (visual; see shared_web/static/css/base/variables.css):
//   ok    green  --status-ok
//   warn  amber  --status-warn   (stale uses --status-stale, the amber token
//          crossmarket.js already uses: attention, NOT an outage)
//   err   red    --status-err
//   muted grey   --status-unknown  (inactive / unknown — never green)

// Tone → CSS class + status-light color.
export const CHANNEL_TONE = {
  ok:    { badgeClass: 'ok',    light: 'var(--status-ok)' },
  warn:  { badgeClass: 'warn',  light: 'var(--status-warn)' },
  err:   { badgeClass: 'err',   light: 'var(--status-err)' },
  muted: { badgeClass: 'muted', light: 'var(--status-unknown)' },
};

// Canonical status → label / tone. Labels mirror the backend StatusText().
export const CHANNEL_STATUS = {
  ok:             { label: '正常',     tone: 'ok' },
  expected_delay: { label: '正常延遲', tone: 'ok' },
  warn:           { label: '待更新',   tone: 'warn' },
  // Derived verdict: last successful fetch is older than the channel
  // contract's freshness window (default 48h). Amber, never red.
  stale:          { label: '資料過期', tone: 'warn', light: 'var(--status-stale)' },
  // Verdict written by the fetch path when only part of the payload arrived.
  degraded:       { label: '降級',     tone: 'warn' },
  error:          { label: '異常',     tone: 'err' },
  partial:        { label: '部分異常', tone: 'err' },
  // Operator toggle (channels.json enabled=false) — a decision, not an incident.
  inactive:       { label: '未啟用',   tone: 'muted' },
  // No health record at all.
  unknown:        { label: '未知',     tone: 'muted' },
};

// Worst-first ordering, used when a caller must pick one label for a set of
// channels (e.g. the home data-quality badge).
export const CHANNEL_SEVERITY_ORDER = [
  'error', 'partial', 'stale', 'degraded', 'warn', 'unknown',
  'expected_delay', 'ok', 'inactive',
];

const FALLBACK_STATUS = 'unknown';

// channelStatusMeta maps any status value to its display + verdict metadata.
// Unknown/empty values fall back to 未知/muted — they are never treated as
// normal, and they never become red on their own.
export function channelStatusMeta(status) {
  const raw = status == null ? '' : String(status);
  const known = Object.prototype.hasOwnProperty.call(CHANNEL_STATUS, raw);
  const key = known ? raw : FALLBACK_STATUS;
  const entry = CHANNEL_STATUS[key];
  const tone = CHANNEL_TONE[entry.tone] || CHANNEL_TONE.muted;
  return {
    status: key,
    raw,
    known,
    label: entry.label,
    tone: entry.tone,
    badgeClass: tone.badgeClass,
    light: entry.light || tone.light,
    // normal → counts as「正常」. Only the backend's ok/expected_delay may.
    normal: entry.tone === 'ok',
    // abnormal → amber/red: must be counted as「非正常」on every page.
    abnormal: entry.tone === 'warn' || entry.tone === 'err',
    // needsAttention → not normal, but not an operator-disabled channel
    // either; `unknown` belongs here so a missing verdict is surfaced (muted)
    // instead of silently passing as normal.
    needsAttention: entry.tone !== 'ok' && key !== 'inactive',
  };
}

export function isChannelNormal(status) {
  return channelStatusMeta(status).normal;
}

export function isChannelAbnormal(status) {
  return channelStatusMeta(status).abnormal;
}

// worstChannelStatus returns the most severe status among the given values.
export function worstChannelStatus(statuses) {
  let worst = FALLBACK_STATUS;
  let worstRank = CHANNEL_SEVERITY_ORDER.length;
  for (const s of (statuses || [])) {
    const meta = channelStatusMeta(s);
    const rank = CHANNEL_SEVERITY_ORDER.indexOf(meta.status);
    const normalizedRank = rank === -1 ? CHANNEL_SEVERITY_ORDER.length : rank;
    if (normalizedRank < worstRank) {
      worst = meta.status;
      worstRank = normalizedRank;
    }
  }
  return worst;
}
