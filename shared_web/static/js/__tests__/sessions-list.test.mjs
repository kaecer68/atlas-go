import test from 'node:test';
import assert from 'node:assert/strict';
import { parseSessionsList } from '../shared/app-utils.js';

test('parseSessionsList: null payload → fetch_failed', () => {
  const r = parseSessionsList(null);
  assert.equal(r.data_status, 'fetch_failed');
  assert.deepEqual(r.sessions, []);
});

test('parseSessionsList: undefined payload → fetch_failed', () => {
  const r = parseSessionsList(undefined);
  assert.equal(r.data_status, 'fetch_failed');
  assert.deepEqual(r.sessions, []);
});

test('parseSessionsList: missing sessions field → malformed', () => {
  const r = parseSessionsList({ meta: 'x' });
  assert.equal(r.data_status, 'malformed');
  assert.deepEqual(r.sessions, []);
});

test('parseSessionsList: sessions not an array → malformed', () => {
  const r = parseSessionsList({ sessions: 'not-array' });
  assert.equal(r.data_status, 'malformed');
  assert.deepEqual(r.sessions, []);
});

test('parseSessionsList: valid payload → ok with original items', () => {
  const items = [{ id: 'session-20260614-daily' }, { id: 'session-20260613-daily' }];
  const r = parseSessionsList({ sessions: items });
  assert.equal(r.data_status, 'ok');
  assert.equal(r.sessions.length, 2);
  assert.equal(r.sessions[0].id, 'session-20260614-daily');
});

test('parseSessionsList: empty array is ok (legitimately no sessions yet)', () => {
  const r = parseSessionsList({ sessions: [] });
  assert.equal(r.data_status, 'ok');
  assert.deepEqual(r.sessions, []);
});
