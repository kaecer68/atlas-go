// shared_web/static/js/__tests__/scheduler-format.test.mjs
//
// scheduler.js display fixes (2026-09-03):
//   (a) interval column showed raw Go nanoseconds (86400000000000) — must
//       render humanized (24h / 6h / 5m / …)
//   (b) next_run fell back to a Go zero time ("0001-01-01T00:00:00Z") which
//       rendered as a bogus date — must show "—" (or the liveness next_run).
//
// Run: cd admin_web && npm test (node --test ../shared_web/static/js/__tests__/*.mjs)
// NOTE: shared_web has no package.json (type=module), so plain .js imports
// resolve as CommonJS under Node 22. scheduler.js has no internal imports, so
// we load it as an explicit ESM data: URL to keep this test runnable anywhere.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const schedulerSrc = readFileSync(
  fileURLToPath(new URL('../pages/scheduler.js', import.meta.url)),
  'utf8'
);
const mod = await import('data:text/javascript;base64,' + Buffer.from(schedulerSrc).toString('base64'));
const { formatDuration, formatTime, mergeLivenessAndStatus } = mod;

// ─── (a) interval humanization ───────────────────────────────────────────────

test('formatDuration: raw Go ns intervals render as 24h / 6h', () => {
  assert.equal(formatDuration(86400000000000), '24h'); // 24 * 3600 * 1e9
  assert.equal(formatDuration(21600000000000), '6h');
});

test('formatDuration: sub-day units', () => {
  assert.equal(formatDuration(300000000000), '5m'); // 5 * 60 * 1e9
  assert.equal(formatDuration(1800000000000), '30m');
  assert.equal(formatDuration(60000000000), '1m');
  assert.equal(formatDuration(30000000000), '30s');
  assert.equal(formatDuration(500000000), '500ms'); // 0.5s
  assert.equal(formatDuration(604800000000000), '7d'); // 7 * 86400 * 1e9
});

test('formatDuration: Go duration strings stay readable', () => {
  assert.equal(formatDuration('1h0m0s'), '1h');
  assert.equal(formatDuration('5m0s'), '5m');
  assert.equal(formatDuration('45s'), '45s');
});

test('formatDuration: empty / invalid input', () => {
  assert.equal(formatDuration(null), '—');
  assert.equal(formatDuration(undefined), '—');
  assert.equal(formatDuration(''), '—');
});

// ─── (b) next_run zero-value handling ─────────────────────────────────────────

test('formatTime: Go zero time renders as —', () => {
  assert.equal(formatTime('0001-01-01T00:00:00Z'), '—');
  assert.equal(formatTime('0001-01-01T08:06:00+08:00'), '—');
  assert.equal(formatTime(null), '—');
  assert.equal(formatTime(undefined), '—');
});

test('formatTime: real timestamps still format', () => {
  const out = formatTime('2026-09-03T03:28:00Z');
  assert.notEqual(out, '—');
  assert.ok(!String(out).startsWith('0001-01-01'), 'real ts must not be treated as zero');
});

test('mergeLivenessAndStatus: zero runtime next_run falls back to liveness, else —', () => {
  // BTM task that has never run since process start: runtime next_run is the
  // Go zero value (serialized); the merge must not forward it to the renderer.
  const statuses = [
    { name: 'auto_cycle_update', channel_id: 'x', enabled: true, interval: 21600000000000, next_run: '0001-01-01T00:00:00Z', last_run: '', consecutive_failures: 0, last_error: '' },
  ];
  const merged = mergeLivenessAndStatus([], statuses);
  assert.equal(merged[0].next_run_at, null, 'zero runtime next_run must not be used');

  // With a liveness row carrying next_run_at, that value wins.
  const liveness = [
    { name: 'auto_cycle_update', interval: '6h0m0s', last_run_at: '2026-09-03T03:28:00Z', last_success_at: '2026-09-03T03:28:00Z', next_run_at: '2026-09-04T03:28:00Z', consecutive_failures: 0 },
  ];
  const merged2 = mergeLivenessAndStatus(liveness, statuses);
  assert.equal(merged2[0].next_run_at, '2026-09-04T03:28:00Z');

  // Liveness without next_run_at → null → the 下次執行 cell renders "—".
  const livenessNoNext = [
    { name: 'auto_cycle_update', interval: '6h0m0s', last_run_at: '2026-09-03T03:28:00Z', last_success_at: '2026-09-03T03:28:00Z', consecutive_failures: 0 },
  ];
  const merged3 = mergeLivenessAndStatus(livenessNoNext, statuses);
  assert.equal(merged3[0].next_run_at, null);
});

test('mergeLivenessAndStatus: non-zero runtime next_run is preserved', () => {
  const statuses = [
    { name: 'prism_auto_balancer', enabled: true, interval: 300000000000, next_run: '2026-09-03T08:05:00Z', last_run: '2026-09-03T08:00:00Z', consecutive_failures: 0 },
  ];
  const merged = mergeLivenessAndStatus([], statuses);
  assert.equal(merged[0].next_run_at, '2026-09-03T08:05:00Z');
});
