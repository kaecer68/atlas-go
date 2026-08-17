#!/usr/bin/env bash
# Test scripts/cron-entrypoint.sh cron field matching.
# Regression test for the P0-E' leading-zero bug: date(1) zero-pads
# %M/%H/%d/%m ("08"/"00") while CRON_SCHEDULE fields are usually not
# ("8"/"0"); the old string comparisons never matched, silently disabling
# every cron container whose schedule contained a single-digit field.
#
# Drives the production entrypoint through its CRON_MATCH_TEST hook (same
# code path as the live 60s loop) with simulated instants.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
ENTRYPOINT="$ROOT/scripts/cron-entrypoint.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# Run the entrypoint matcher for a schedule at a simulated instant.
# CRON_MATCH_TEST format: MIN:HOUR:DAY:MONTH:WDAY (as date(1) would emit,
# zero-padded).
run_matcher() {
  local sched=$1 now=$2
  CRON_SCHEDULE="$sched" CRON_COMMAND="true" CRON_MATCH_TEST="$now" sh "$ENTRYPOINT" >/dev/null 2>&1
}

assert_match() {
  local sched=$1 now=$2
  if ! run_matcher "$sched" "$now"; then
    fail "expected MATCH for schedule '$sched' at '$now'"
  fi
}

assert_nomatch() {
  local sched=$1 now=$2
  if run_matcher "$sched" "$now"; then
    fail "expected NO MATCH for schedule '$sched' at '$now'"
  fi
}

# ── P0-E' regression: zero-padded now vs non-padded schedule field ──
# 08:00 against "0 8 * * *" must match (old code: "08" != "8" -> never ran)
assert_match    "0 8 * * *"   "00:08:17:08:0"
# 08:00 against "0 9 * * *" must NOT match
assert_nomatch  "0 9 * * *"   "00:08:17:08:0"
# 15:30 against "30 15 * * *" must match (replay-sync, was the only survivor)
assert_match    "30 15 * * *" "30:15:17:08:0"
# darwinian "0 9 * * *" at 09:00 — old code failed even on "00" vs "0"
assert_match    "0 9 * * *"   "00:09:17:08:0"
# midnight "0 0 * * *" — "00" vs "0"
assert_match    "0 0 * * *"   "00:00:17:08:0"
# boundary: same hour, wrong minute / same minute, wrong hour
assert_nomatch  "0 9 * * *"   "30:09:17:08:0"
assert_nomatch  "0 9 * * *"   "00:10:17:08:0"

# ── zero-padded day/month fields ──
assert_match    "0 9 5 * *"   "00:09:05:08:0"
assert_nomatch  "0 9 6 * *"   "00:09:05:08:0"
assert_match    "0 9 * 3 *"   "00:09:17:03:0"
assert_nomatch  "0 9 * 4 *"   "00:09:17:03:0"

# ── weekday list / range via crontab_field_match (numeric path) ──
assert_match    "0 9 * * 0,6"   "00:09:17:08:6"
assert_nomatch  "0 9 * * 0,6"   "00:09:17:08:3"
assert_match    "30 15 * * 1-5" "30:15:17:08:3"
assert_nomatch  "30 15 * * 1-5" "30:15:17:08:6"

# ── star fields match any value ──
assert_match    "0 * * * *"   "00:11:17:08:3"
assert_nomatch  "0 * * * *"   "01:11:17:08:3"
assert_match    "* * * * *"   "59:23:31:12:6"

# ── syntax sanity ──
sh -n "$ENTRYPOINT" || fail "cron-entrypoint.sh failed sh -n"
sh -n "${BASH_SOURCE[0]}" || fail "test script failed sh -n"

echo "PASS: cron-entrypoint schedule matching tests"
