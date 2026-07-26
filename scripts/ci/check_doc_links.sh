#!/usr/bin/env bash
# check_doc_links.sh — 驗證 docs/ 內跨檔案引用無斷鏈
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"
echo "🔗 Scanning docs/ for broken links..."
FAILED=0
# 找出所有 .md 檔案中 []() 格式的相對連結（排除外部 URL）
while IFS=':' read -r file line content; do
    target=$(echo "$content" | grep -oP '\([.a-zA-Z0-9/_-]+\.md[)#]' | sed 's/^[()]//; s/[)#]$//' | head -1)
    [[ -z "$target" ]] && continue
    resolved="$REPO_ROOT/$file"
    base_dir=$(dirname "$resolved")
    target_path="$base_dir/$target"
    target_real=$(cd "$base_dir" 2>/dev/null && echo "$target" | sed 's|\.\/||' | xargs -I{} realpath {} 2>/dev/null || echo "")
    if [ ! -f "$target_path" ] && [ ! -f "$target_real" ]; then
        echo "  ❌ BROKEN: $file → $target"
        FAILED=1
    fi
done < <(grep -rnP '\[.*?\]\([^http#].*?\.md[#\)]' docs/ --include="*.md" 2>/dev/null | grep -v ".git/")
if [ "$FAILED" -eq 1 ]; then
    echo "❌ Broken links found"
    exit 1
fi
echo "  ✅ All links OK"
