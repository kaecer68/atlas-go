#!/usr/bin/env bash
# check_git_env_hermeticity.sh — 薄殼；實作在 .py（純標準庫、離線、不需 Go toolchain）。
# 契約測試（執行期正/負向 ＋ mutation 自證）：tests/scripts/test-git-env-hermeticity.sh
set -uo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check_git_env_hermeticity.py" "$@"
