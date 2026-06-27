#!/usr/bin/env bash
#
# check_markdown_links.sh
#
# 驗證所有 .md 檔案中的內部連結（含 backtick 中的裸 .md 路徑）。
# 實作在 check_markdown_links.py — 此 shell 腳本僅負責 find + 傳參。
#
# Exit 0 = 全部有效；Exit 1 = 發現 broken 連結

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

mapfile -t MD_FILES < <(find . \
    -name '*.md' \
    -not -path './vendor/*' \
    -not -path './.gocache/*' \
    -not -path './.opencode/*' \
    -not -path '*/node_modules/*' \
    -not -path '*/.git/*' \
    -not -path './web/static/css/*' \
    -not -path './web/static/js/*' \
    -not -path '*/.omo/*' \
    -not -name 'CHANGELOG.md' \
    -not -path '*/docs/briefs/*' \
    -not -path '*/docs/archive/*' \
    -not -path '*/docs/handoff/*' \
    -not -path '*/docs/investigations/*' \
    -not -path '*/docs/audit/*' \
    -not -path '*/docs/wave-11/*' \
    -not -path '*/.claude/*' \
    -not -name 'agents-md-audit.md' \
    -print)

if [ "${#MD_FILES[@]}" -eq 0 ]; then
    echo "No markdown files found"
    exit 0
fi

exec python3 "${SCRIPT_DIR}/check_markdown_links.py" "${MD_FILES[@]}"
