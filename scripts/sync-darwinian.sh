#!/bin/bash
#
# scripts/sync-darwinian.sh — Merge MacBook(dev) Darwinian state into iMac(prod).
#
# Background (2026-08-27):
#   darwinian_history.jsonl is an append-only evolution asset, but data/state/
#   is NOT git-tracked. The first iMac deploy (2026-08-15) started a brand-new
#   history file on iMac; the 94-day MacBook history (2026-05-21 → 2026-08-14)
#   was never migrated. See DARWINIAN_DUAL_MACHINE_DIVERGENCE.md.
#
# This script performs a UNION merge (dedupe by line, stable sort by timestamp),
# so it is monotonic: merged line count >= max(source counts). It never drops
# lines that exist on either machine, so the "rsync direction flipped" failure
# mode (94 days wiped to 12) is structurally impossible.
#
# Requirements:
#   - target atlas must be STOPPED before running (docker compose down) to
#     avoid torn lines from an in-flight O_APPEND.
#   - ssh access to the peer (default kk@kimac).
#
# Usage:
#   scripts/sync-darwinian.sh [--dry-run] [--force] [--local-only] [--peer HOST:PATH]
#
# Defaults:
#   --peer      kk@kimac:/Users/kk/workspace/atlas/data/state
#   local       /Users/kaecer/workspace/atlas/data/state  (MacBook dev)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

LOCAL_DIR="${PROJECT_ROOT}/data/state"
PEER="kk@kimac"
PEER_DIR="/Users/kk/workspace/atlas/data/state"
DRY_RUN=false
FORCE=false
LOCAL_ONLY=false

usage() {
  sed -n '2,24p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=true; shift ;;
    --force) FORCE=true; shift ;;
    --local-only) LOCAL_ONLY=true; shift ;;
    --peer) PEER="${2%%:*}"; PEER_DIR="${2#*:}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1"; usage; exit 2 ;;
  esac
done

LOCAL_HIST="${LOCAL_DIR}/darwinian_history.jsonl"
LOCAL_W="${LOCAL_DIR}/darwinian_weights.json"

TS="$(date +%F_%H%M)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

echo "== sync-darwinian =="
echo "local : ${LOCAL_DIR}"
if $LOCAL_ONLY; then
  echo "peer  : (local-only)"
else
  echo "peer  : ${PEER}:${PEER_DIR}"
fi

# --- 1. Collect source files -------------------------------------------------
# local always participates; peer participates unless --local-only
if $LOCAL_ONLY; then
  echo "[1/5] local files only"
else
  echo "[1/5] pull peer files"
  scp -q "${PEER}:${PEER_DIR}/darwinian_history.jsonl" "${WORK}/peer_history.jsonl"
  scp -q "${PEER}:${PEER_DIR}/darwinian_weights.json" "${WORK}/peer_weights.json"
fi

# --- 2. Line-count guard -----------------------------------------------------
# A merge must never reduce total lines. If --force is passed we skip the
# sanity abort but the union merge still guarantees monotonicity.
LOCAL_LINES=$(wc -l < "${LOCAL_HIST}" | tr -d ' ')
echo "  local history lines: ${LOCAL_LINES}"
if ! $LOCAL_ONLY; then
  PEER_LINES=$(wc -l < "${WORK}/peer_history.jsonl" | tr -d ' ')
  echo "  peer  history lines: ${PEER_LINES}"
  if [[ "${PEER_LINES}" -lt 10 && "${LOCAL_LINES}" -gt "${PEER_LINES}" ]]; then
    if ! $FORCE; then
      echo "ERROR: peer history looks truncated (${PEER_LINES} lines < local ${LOCAL_LINES})."
      echo "       Refusing to merge. Use --force if you are sure."
      exit 1
    fi
    echo "WARN: peer history truncated but --force given; proceeding."
  fi
fi

# --- 3. Backup peer target ---------------------------------------------------
if ! $LOCAL_ONLY && ! $DRY_RUN; then
  echo "[2/5] backup peer target"
  ssh "${PEER}" "cd ${PEER_DIR} && cp darwinian_history.jsonl darwinian_history.jsonl.bak-${TS} && cp darwinian_weights.json darwinian_weights.json.bak-${TS} && ls darwinian_history.jsonl.bak-* | tail -n +4 | xargs -r rm --"
  echo "  backed up (keeping latest 3)"
else
  echo "[2/5] (backup skipped: dry-run or local-only)"
fi

# --- 4. Merge ----------------------------------------------------------------
echo "[3/5] merge history (union, dedupe, sort)"
if $LOCAL_ONLY; then
  echo "  local-only: nothing to merge (single source)."
else
  python3 "${SCRIPT_DIR}/merge_darwinian.py" \
    "${LOCAL_HIST}" "${WORK}/peer_history.jsonl" "${WORK}/merged_history.jsonl"
fi

# --- 5. Push + verify --------------------------------------------------------
if $DRY_RUN; then
  echo "[4/5] DRY-RUN — not pushing"
  echo "[5/5] done (dry-run)"
  exit 0
fi

if ! $LOCAL_ONLY; then
  echo "[4/5] push merged history to peer"
  scp -q "${WORK}/merged_history.jsonl" "${PEER}:${PEER_DIR}/darwinian_history.jsonl"
  # weights.json: peer is prod truth — keep peer's, do not overwrite.
  echo "  (weights.json untouched — peer is prod truth)"

  echo "[5/5] verify"
  ssh "${PEER}" "wc -l ${PEER_DIR}/darwinian_history.jsonl && tail -1 ${PEER_DIR}/darwinian_history.jsonl | python3 -c 'import sys,json; print(\"last ts:\", json.loads(sys.stdin.readline())[\"timestamp\"])'"
else
  echo "[5/5] local-only: nothing pushed"
fi

echo "== done =="
