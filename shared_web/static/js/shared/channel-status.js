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
  // Retired BY DESIGN (backend: ChannelContract.Retirement) — the upstream is
  // permanently gone and a replacement input already serves the consumer. Not
  // the same state as `inactive` (switched off right now, reversible): this one
  // can never come back, so it is never an attention case. `retired: true`
  // removes it from needsAttention below (2026-10-05 三分類).
  retired:        { label: '已退役',   tone: 'muted', retired: true },
  // No health record at all.
  unknown:        { label: '未知',     tone: 'muted' },
};

// 需關注的三種成因（SSOT for the frontend; the backend classifies —
// internal/monitoring/service/channel_attention.go — and these entries only
// carry the display text/tone, exactly like CHANNEL_STATUS above).
//
// The point of the split (2026-10-05):「系統錯誤」是我們的問題、「已知上游限制」
// 是上游的問題、「預期等待」是時間還沒到。混成一張清單會讓管理者無法判斷該不該
// 動手。
export const ATTENTION_CATEGORY = {
  system_error:   { key: 'system_error',   title: '系統錯誤（我們的問題）', hint: 'atlas 這一側的問題，需要有人處理',        tone: 'err' },
  upstream_limit: { key: 'upstream_limit', title: '已知上游限制（配額／tier）', hint: '已登錄的上游限制，等上游恢復或配額重置', tone: 'warn' },
  expected_wait:  { key: 'expected_wait',  title: '預期等待（日曆未到）', hint: '上游在該期間沒有發布機會，現在不必處理',   tone: 'muted' },
};

// Render order: our problem first (it is the only one that needs action).
export const ATTENTION_CATEGORY_ORDER = ['system_error', 'upstream_limit', 'expected_wait'];

// attentionCategoryMeta maps a category to its display metadata. An unknown or
// missing category falls back to 系統錯誤 — the backend's own default — so an
// unclassified row is surfaced as our problem instead of being silently hidden
// or excused as an upstream matter.
// FALLBACK_ATTENTION_CATEGORY is the backend's own default
// (internal/monitoring/service/channel_attention.go: an unattributable condition
// is reported against atlas). Kept as a named constant — and read through a
// bracket, not a dot — because these are WIRE VALUES sent by the backend
// (`category`), not API field names: a dotted read of this map would look to
// scripts/ci/check_field_contract.sh like a backend field access that no Go json
// tag declares.
const FALLBACK_ATTENTION_CATEGORY = 'system_error';

export function attentionCategoryMeta(category) {
  const raw = category == null ? '' : String(category);
  return Object.prototype.hasOwnProperty.call(ATTENTION_CATEGORY, raw)
    ? ATTENTION_CATEGORY[raw]
    : ATTENTION_CATEGORY[FALLBACK_ATTENTION_CATEGORY];
}

// isKnownAttentionCategory tells a classified row from a fallback. The page
// renders both under 系統錯誤 (fail-safe: surface it, never hide it), but only a
// KNOWN category may drive the block's tone — an unclassified row keeps the
// historical status-derived tone so a stale-only set is not framed as an outage.
export function isKnownAttentionCategory(category) {
  const raw = category == null ? '' : String(category);
  return Object.prototype.hasOwnProperty.call(ATTENTION_CATEGORY, raw);
}

// Worst-first ordering, used when a caller must pick one label for a set of
// channels (e.g. the home data-quality badge).
export const CHANNEL_SEVERITY_ORDER = [
  'error', 'partial', 'stale', 'degraded', 'warn', 'unknown',
  'expected_delay', 'ok', 'inactive',
  // Least severe: a retired channel must never dominate a set's verdict (the
  // home data-quality badge reads worstChannelStatus).
  'retired',
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
    // retired → off BY DESIGN (upstream gone, replacement wired). Exposed so
    // callers can report it separately from 未啟用 instead of guessing.
    retired: entry.retired === true,
    // needsAttention → not normal, but not a decision either: an
    // operator-disabled channel (`inactive`) and a retired-by-design one
    // (`retired`) are both off on purpose, so neither belongs in「需關注」.
    // `unknown` DOES belong here so a missing verdict is surfaced (muted)
    // instead of silently passing as normal.
    needsAttention: entry.tone !== 'ok' && key !== 'inactive' && !entry.retired,
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
