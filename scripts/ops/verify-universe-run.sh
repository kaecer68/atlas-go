#!/usr/bin/env bash
# scripts/ops/verify-universe-run.sh - read-only truth audit + layer classifier for the
# universe pipeline. Thin wrapper only; the implementation is verify-universe-run.py
# (Python standard library only, so it runs on the production host with no install step).
#
# 這支工具**不取代告警**（見 .py 檔頭）：它不寫任何檔案、不改任何狀態，只把
# 「母體跑到哪一層、壞在哪一層」變成可重跑、可稽核的一件事。
#
# exit code（與 .py 一致）:
#   0 = 全綠（可含 WARN）／1 = 有層成立「應告警」條件／2 = 無法判定／3 = 用法・設定錯誤
#
# 最短用法（生產主機、repo checkout 內，唯讀）:
#   scripts/ops/verify-universe-run.sh
#   scripts/ops/verify-universe-run.sh --json
#   scripts/ops/verify-universe-run.sh --quiet
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON_BIN="${PYTHON:-python3}"

if ! command -v "$PYTHON_BIN" >/dev/null 2>&1; then
   echo "verify-universe-run: 找不到 python3（可用 PYTHON=<path> 指定）" >&2
   exit 3
fi

exec "$PYTHON_BIN" "$SCRIPT_DIR/verify-universe-run.py" "$@"
