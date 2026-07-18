#!/usr/bin/env bash
# =============================================================================
# check_atlas_mcp_docs_consistency.sh — 驗證 MCP 文件中無錯誤 env var
#
# 背景:2026-07-10 hermes agent 反映早期 docs/mcp-integration-guide.md
# 使用了錯誤的 env var (ATLAS_WORK_DIR/ATLAS_DATABASE_URL/
# ATLAS_REDIS_URL/ATLAS_API_TOKEN), 浪費 30+ 分鐘摸索。實際 main.go
# 讀的是 ATLAS_BASE_URL/ATLAS_API_KEY/ATLAS_MCP_TOKEN。
#
# 本腳本確保:
#   1. 權威文件 (cmd/atlas-mcp/README.md + AGENT_QUICKSTART.md + 根 README +
#      AGENTS.md) 沒有出現錯誤的 env var 名
#   2. AGENTS.md 行數 ≤ 155 (避免 160 行 reject)
#   3. 工具數應為 110 (grep "110 tool" 在至少 3 個檔案; 2026-07-15 Round 2
#   audit fixes 將 phantom registerTemplateDetectorTools 重複呼叫移除,
#   registerTools delta 從 108 變 106, +audit 4 = 110 踏 [110,112] 下界)
#
# 排除:
#   - docs/environment.md / docs/specs/security-audit.md (backend 用, 非 atlas-mcp)
#   - CHANGELOG.md (歷史紀錄, 不可改)
#
# 用法:
#   bash scripts/ci/check_atlas_mcp_docs_consistency.sh          # 失敗即 exit 1
#   bash scripts/ci/check_atlas_mcp_docs_consistency.sh --warn-only  # 僅警告
#
# 退出碼:0 = 一致,1 = 發現錯誤 env var 或違規
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

WARN_ONLY=false
if [ "${1:-}" = "--warn-only" ]; then
    WARN_ONLY=true
fi

# 錯誤的 env var (這 4 個是舊版文件曾誤用的,實際 main.go 不讀)
WRONG_VARS=(
    "ATLAS_WORK_DIR"        # 不存在 (backend 才有,不是 atlas-mcp)
    "ATLAS_DATABASE_URL"    # 應為 DATABASE_URL (libpq 標準)
    "ATLAS_REDIS_URL"       # 不存在
    "ATLAS_API_TOKEN"       # 應為 ATLAS_API_KEY (X-API-Key header)
)

# 權威文件清單 (這些檔案不能有錯誤 env var)
AUTHORITATIVE_FILES=(
    "cmd/atlas-mcp/README.md"
    ".claude/skills/atlas-mcp-integration/AGENT_QUICKSTART.md"
    "README.md"
    "AGENTS.md"
    "CLAUDE.md"
)

# 額外檢查:AGENTS.md 行數 ≤ 155 (避免 160 行 reject)
AGENTS_MAX_LINES=155

# 額外檢查:108 tool 必須在 3+ 個檔案出現 (防止下次又有人寫 80+)
TOOL_COUNT_MIN=3

FAIL_COUNT=0
WARN_COUNT=0

echo "🔍 Checking atlas-mcp docs consistency..."
echo ""

# 1. 檢查每個權威檔案不含錯誤 env var
echo "  → Scanning authoritative files for wrong env vars..."
for file in "${AUTHORITATIVE_FILES[@]}"; do
    if [ ! -f "$file" ]; then
        echo "    ⚠️  File not found: $file (skipping)"
        WARN_COUNT=$((WARN_COUNT + 1))
        continue
    fi
    for var in "${WRONG_VARS[@]}"; do
        # 用 word boundary grep 避免誤判 (e.g. ATLAS_BASE_URL 不會匹配 ATLAS_WORK_DIR 因為 _ 分隔)
        # 用 grep -v 過濾掉「明確標示為已廢棄 / 不要再用 / deprecated / 對照」的行
        # 這些行是刻意保留的對照說明, 不算違規
        forbidden_lines=$(grep -nE "\\b${var}\\b" "$file" 2>/dev/null \
            | grep -vE "DEPRECATED|deprecated|已廢棄|已棄用|過時|不要再用|不要使用|取代|考古" || true)
        if [ -n "$forbidden_lines" ]; then
            echo "    ❌ $file contains forbidden env var (NOT in deprecation context): $var"
            echo "$forbidden_lines" | head -3 | sed 's/^/        /'
            FAIL_COUNT=$((FAIL_COUNT + 1))
        fi
    done
done

# 2. 檢查 AGENTS.md 行數
echo ""
echo "  → Checking AGENTS.md line count (must be ≤ $AGENTS_MAX_LINES)..."
if [ -f "AGENTS.md" ]; then
    line_count=$(wc -l < AGENTS.md | tr -d ' ')
    if [ "$line_count" -gt "$AGENTS_MAX_LINES" ]; then
        echo "    ❌ AGENTS.md is $line_count lines (max: $AGENTS_MAX_LINES)"
        FAIL_COUNT=$((FAIL_COUNT + 1))
    else
        echo "    ✓ AGENTS.md is $line_count lines (≤ $AGENTS_MAX_LINES)"
    fi
fi

# 3. 檢查 110 tool 散佈在至少 3 個檔案(2026-07-15 Round 2 dedup 後由 108 變 110)
echo ""
echo "  → Checking tool count 111 propagates to ≥ $TOOL_COUNT_MIN files..."
files_with_110=0
for file in "${AUTHORITATIVE_FILES[@]}"; do
    if [ -f "$file" ] && grep -qE "111 (個 tool|tools|tool |個 tool| tool)" "$file"; then
        files_with_110=$((files_with_110 + 1))
    fi
done
if [ "$files_with_110" -lt "$TOOL_COUNT_MIN" ]; then
    echo "    ❌ '111 tools' only appears in $files_with_110 files (need ≥ $TOOL_COUNT_MIN)"
    FAIL_COUNT=$((FAIL_COUNT + 1))
else
    echo "    ✓ '111 tools' appears in $files_with_110 files (≥ $TOOL_COUNT_MIN)"
fi

# 總結
echo ""
echo "================================"
if [ "$FAIL_COUNT" -eq 0 ] && [ "$WARN_COUNT" -eq 0 ]; then
    echo "✅ All consistency checks passed"
    exit 0
elif [ "$FAIL_COUNT" -eq 0 ]; then
    echo "⚠️  $WARN_COUNT warnings, 0 failures"
    if [ "$WARN_ONLY" = "true" ]; then
        exit 0
    else
        echo "   (use --warn-only to allow warnings)"
        exit 1
    fi
else
    echo "❌ $FAIL_COUNT failures, $WARN_COUNT warnings"
    if [ "$WARN_ONLY" = "true" ]; then
        echo "   (--warn-only: not exiting despite failures)"
        exit 0
    else
        exit 1
    fi
fi
