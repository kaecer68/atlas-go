#!/usr/bin/env bash
# check_shell_var_nonascii.sh — 薄殼；實作在 .py（只用標準庫、離線可跑、不需要 Go toolchain）。
#
# 為什麼要有 .sh：`make ci` 以 `bash scripts/ci/check_*.sh` 逐支跑（glob），`make ci-quick`／
# `make ci-gate` 與 GitHub Actions 走同一支；維持與其他 check_*.sh 一致的呼叫面。
#
# 規則、實測依據（平台/locale 相依）、修法與已知限制見 check_shell_var_nonascii.py 檔頭。
# baseline（只放別人的 PR 正在修的項，每項必填理由）：scripts/ci/shell-var-nonascii-baseline.txt
# 自我測試：tests/scripts/test-shell-var-nonascii.sh（hermetic fixture，斷言精確 exit code）
set -uo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check_shell_var_nonascii.py" "$@"
