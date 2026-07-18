#!/usr/bin/env bash
#
# check_markdown_links.sh
#
# 驗證所有 .md 檔案中的內部連結（含 backtick 中的裸 .md 路徑）。
# 實作在 check_markdown_links.py — 此 shell 腳本僅負責 find + 透過 stdin 傳參。
#
# 為避免 ARG_MAX (Linux: 2MB / macOS: 256KB) 限制，使用 xargs 將檔案清單分批傳遞。
#
# Exit 0 = 全部有效；Exit 1 = 發現 broken 連結

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

# 用 stdin + xargs 將檔案清單分批傳遞給 Python (避免 ARG_MAX overflow)
# -n 200: 每批 200 個檔案 (Linux ARG_MAX 2MB, 保守取較小值)
# -0: null-separated 處理檔名含空格的特殊情況
find . \
    -name '*.md' \
    -not -path './vendor/*' \
    -not -path './.gocache/*' \
    -not -path './.opencode/*' \
    -not -path './.gstack/*' \
    -not -path '*/node_modules/*' \
    -not -path '*/.git/*' \
    -not -path '*/.omo/*' \
    -not -name 'CHANGELOG.md' \
    -not -path '*/docs/briefs/*' \
    -not -path '*/docs/archive/*' \
    -not -path '*/docs/handoff/*' \
    -not -path '*/docs/investigations/*' \
    -not -path '*/docs/audit/*' \
    -not -path '*/.claude/*' \
    -not -path '*/.superpowers/*' \
    -print0 |
    xargs -0 -n 200 python3 "${SCRIPT_DIR}/check_markdown_links.py"
