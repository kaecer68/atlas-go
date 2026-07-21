#!/usr/bin/env bash
# =============================================================================
# update_doc_crossrefs.sh — 自動更新 docs 移動後的 cross-references
#
# 2026-07-10: PR-3 docs architecture 重組.
# 5 個文件從 docs/ 移到 docs/reference/:
#   - CONSTITUTION.md          (46 refs)
#   - traps.md                 (22 refs)
#   - iteration-gate.md        ( 4 refs)
#   - guidelines-index.md      ( 9 refs)
#   - parameter-system.md      (15 refs)
# 合計 96 cross-references 需更新
#
# 用法:
#   bash scripts/ci/update_doc_crossrefs.sh          # 執行更新
#   bash scripts/ci/update_doc_crossrefs.sh --dry-run # 只印將改的檔案
#   bash scripts/ci/update_doc_crossrefs.sh --check   # CI 模式: 有未更新就 exit 1
#
# 退出碼: 0 = 全部 OK, 1 = --check 發現需更新
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

DRY_RUN=false
CHECK_MODE=false
if [ "${1:-}" = "--dry-run" ]; then DRY_RUN=true; fi
if [ "${1:-}" = "--check" ]; then CHECK_MODE=true; fi

# 移動映射 (from -> to)
declare -A MOVES=(
    ["docs/reference/constitution.md"]="docs/reference/constitution.md"
    ["docs/reference/traps.md"]="docs/reference/traps.md"
    ["docs/reference/iteration-gate.md"]="docs/reference/iteration-gate.md"
    ["docs/reference/guidelines-index.md"]="docs/reference/guidelines-index.md"
    ["docs/reference/parameter-system.md"]="docs/reference/parameter-system.md"
)

# 要掃的副檔名
EXTS=("*.md" "*.yml" "*.yaml" "*.sh" "Makefile" "CLAUDE.md" "*.txt")

# 排除路徑 (這些檔案不能被改)
EXCLUDE_PATHS=("/.git/" "/.omo/" "/node_modules/" "/.worktrees/")

# 統計
TOTAL_MATCHES=0
TOTAL_FILES=0

echo "🔄 docs cross-reference updater (PR-3, 2026-07-10)"
echo "  5 個文件要從 docs/ 移到 docs/reference/"
echo "  預期 96 個 cross-references 更新"
echo ""

if $CHECK_MODE; then
    echo "  [mode: CHECK] 只檢查, 不修改"
fi
if $DRY_RUN; then
    echo "  [mode: DRY-RUN] 只印將改的內容, 不實際修改"
fi
echo ""

# 1. 先用 grep 找到所有需要更新的位置
for from in "${!MOVES[@]}"; do
    to="${MOVES[$from]}"
    echo "  掃描 $from →"

    for ext in "${EXTS[@]}"; do
        # 找引用 $from 的所有檔案 (排除 excluded paths)
        # SC2038: use -print0 + xargs -0 to handle non-alphanumeric filenames.
        matches=$(find . -type f -name "$ext" \
            -not -path "./.git/*" \
            -not -path "./.omo/*" \
            -not -path "./node_modules/*" \
            -not -path "./.worktrees/*" \
            -print0 2>/dev/null | xargs -0 grep -l "$from" 2>/dev/null || true)

        if [ -z "$matches" ]; then
            continue
        fi

        for file in $matches; do
            # 排除 REFERENCE 目錄內的新檔案
            if [[ "$file" == *"/docs/reference/"* ]]; then
                continue
            fi

            # 計算匹配數
            count=$(grep -c "$from" "$file" 2>/dev/null || echo 0)
            TOTAL_MATCHES=$((TOTAL_MATCHES + count))
            TOTAL_FILES=$((TOTAL_FILES + 1))

            if $DRY_RUN || $CHECK_MODE; then
                echo "    $file: $count refs"
            else
                # 實際更新: 替換 docs/X.md 為 docs/reference/X.md
                # 但要避免重複更新 (如 docs/reference/constitution.md 內)
                # 簡化: 用 sed 全文替換 (因為 docs/X.md 字串不會在 reference/ 內的 X.md 出現)
                sed -i '' "s|$from|$to|g" "$file"
                echo "    ✓ $file: $count refs updated"
            fi
        done
    done
done

echo ""
echo "================================"
echo "  統計:"
echo "    需要更新: $TOTAL_MATCHES cross-references"
echo "    影響檔案: $TOTAL_FILES"
echo ""

if $CHECK_MODE; then
    if [ "$TOTAL_MATCHES" -gt 0 ]; then
        echo "  ❌ $TOTAL_MATCHES cross-references 需更新 (跑 update_doc_crossrefs.sh 修)"
        exit 1
    else
        echo "  ✅ 所有 cross-references 已更新"
        exit 0
    fi
fi

if $DRY_RUN; then
    echo "  [DRY-RUN] 跑 update_doc_crossrefs.sh (無 --dry-run) 實際更新"
else
    echo "  ✅ 更新完成"
    echo "  建議: git diff 確認, 然後 commit + push"
fi
