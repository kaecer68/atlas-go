#!/usr/bin/env bash
#
# check_agents_md_drift.sh
#
# 驗證所有 internal/*/AGENTS.md + cmd/atlas-mcp/server/AGENTS.md 中提到的
# `<file>.go:NN` / `<file>.go:NN-MM` line references 是否仍對應到真實檔案 + 真實行號。
#
# 實作在 check_agents_md_drift.py — 此 shell 腳本僅負責 find + 透過 stdin 傳參。
#
# 為避免 ARG_MAX (Linux: 2MB / macOS: 256KB) 限制，使用 xargs 將檔案清單分批傳遞。
#
# Exit 0 = 全部有效；Exit 1 = 發現 drift（file missing / line out of range / range endpoint out of range）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

# 用 stdin + xargs 將檔案清單分批傳遞給 Python (避免 ARG_MAX overflow)
# -n 200: 每批 200 個檔案 (Linux ARG_MAX 2MB, 保守取較小值)
# -0: null-separated 處理檔名含空格的特殊情況
find . \
    \( -path './internal/*/AGENTS.md' -o -path './cmd/atlas-mcp/server/AGENTS.md' \) \
    -not -path '*/node_modules/*' \
    -not -path '*/.git/*' \
    -not -path '*/.omo/*' \
    -print0 |
    xargs -0 -n 200 python3 "${SCRIPT_DIR}/check_agents_md_drift.py"
