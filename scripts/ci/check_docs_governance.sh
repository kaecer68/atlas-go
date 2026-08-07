#!/usr/bin/env bash
#
# check_docs_governance.sh — docs/ 文件治理硬性檢查（CI 強制）
#
# 防止 agent 在維護過程中偷跑，將 transient 內容寫入 docs/ 永久目錄。
# 這是 documentation-standard.md §docs/manifests 治理 的執行面強制。
#
# 檢查項目：
#   1. docs/manifests/ 只允許 README.md + TEMPLATE.md
#      → 個別 manifest 必須放 .omo/manifests/（gitignored，harness 私有）
#   2. docs/ 下禁止建立違規目錄：docs/plans/、docs/wave-N/、docs/superpowers/
#   3. docs/ 根目錄禁止新增未追蹤 .md 檔案（documentation-map.md 白名單）
#
# Exit 0 = 合規；Exit 1 = 發現違規

set -euo pipefail
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$REPO_ROOT"

FAILED=0
log_err() { printf 'ERROR [docs-governance] %s\n' "$1"; FAILED=1; }

echo "docs-governance: checking docs/ layout compliance..."

# ─── Check 1: docs/manifests/ 只准 README.md + TEMPLATE.md ───
if [ -d "docs/manifests" ]; then
    for f in docs/manifests/*; do
        [ -e "$f" ] || continue
        b="$(basename "$f")"
        if [ "$b" != "README.md" ] && [ "$b" != "TEMPLATE.md" ] && [ "${b:0:1}" != "." ]; then
            log_err "docs/manifests/ contains non-template file: ${b} (individual manifests belong in .omo/manifests/)"
        fi
    done
    for d in docs/manifests/*/; do
        [ -d "$d" ] || continue
        log_err "docs/manifests/ contains subdirectory: $(basename "$d") (manifests belong in .omo/manifests/)"
    done
fi

# ─── Check 2: docs/ 禁止違規目錄 ───
for banned in docs/plans docs/superpowers; do
    if [ -d "$banned" ]; then
        log_err "banned directory exists: ${banned}/ (see documentation-standard.md)"
    fi
done
for d in docs/wave-*/; do
    [ -d "$d" ] || continue
    log_err "banned directory exists: ${d} (wave content belongs in .omo/wave-N/)"
done

# ─── Check 3: docs/ 根目錄新增未追蹤 .md ───
if git rev-parse --git-dir >/dev/null 2>&1; then
    for f in docs/*.md; do
        [ -e "$f" ] || continue
        b="$(basename "$f")"
        if git ls-files --error-unmatch "$f" >/dev/null 2>&1; then
            continue
        fi
        log_err "untracked .md added at docs/ root: ${b} (root .md files require whitelist approval, see documentation-standard.md)"
    done
fi

echo ""
if [ "$FAILED" -eq 1 ]; then
    echo "docs-governance: FAILED — fix violations before push"
    exit 1
fi
echo "docs-governance: OK (docs/manifests has templates only; no banned dirs; no untracked root .md)"
