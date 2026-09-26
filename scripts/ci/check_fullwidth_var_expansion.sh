#!/usr/bin/env bash
# check_fullwidth_var_expansion.sh — 薄殼；實作在 .py（純標準庫、離線、不需 Go toolchain）。
#
# 為什麼要有 .sh：
#   `make ci` 以 `bash scripts/ci/check_*.sh` 逐支跑（glob）、`make ci-gate` 逐支明列、
#   GitHub Actions 也走同一支 ⇒ 與其他 check_*.sh 一致的呼叫面。
#
# 規則/移植說明（含 atlas-go 擴充的檔案類型：*.sh / Makefile / *.mk / *.py / YAML 的 run: 區塊）
# 見 check_fullwidth_var_expansion.py 檔頭（移植自 a2a-dev 同名工具，tokenizer 原樣沿用）。
# 自我測試：tests/scripts/test-fullwidth-var-expansion.sh
set -uo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check_fullwidth_var_expansion.py" "$@"
