#!/usr/bin/env bash
set -euo pipefail
# check-routes.sh — 檢查 HTTP route 是否有重複註冊或概念衝突
# Exit 0 = clean, Exit 1 = 有問題

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

TMPDIR=$(mktemp -d -t routes-XXXXXX)
cleanup() { rm -rf "$TMPDIR"; }
trap cleanup EXIT

echo "=== Route Uniqueness Check ==="

# Helper: extract path from mux.Handle("METHOD /path") or mux.Handle("/path")
extract_paths() {
  local pattern="$1"
  grep -rn "$pattern" --include='*.go' cmd/atlas/ internal/ 2>/dev/null | \
    sed -nE 's/.*"([A-Z]+ )?(\/[a-zA-Z][^"]*).*/\2/p' | \
    sed 's/{[^}]*}/{id}/g' | sort -u
}

# 1. 所有 mux.Handle 路由
extract_paths 'mux\.Handle\b' > "$TMPDIR/routes_handle.txt"

# 2. 所有 mux.HandleFunc 路由
extract_paths 'mux\.HandleFunc\b' > "$TMPDIR/routes_func.txt"

# 合併
sort -u "$TMPDIR/routes_handle.txt" "$TMPDIR/routes_func.txt" > "$TMPDIR/all_routes.txt"

TOTAL=$(wc -l < "$TMPDIR/all_routes.txt" | tr -d ' ')
echo "  共 $TOTAL 條唯一路由"
echo ""
echo "=== 路由列表 ==="
while IFS= read -r r; do echo "  $r"; done < "$TMPDIR/all_routes.txt"
echo ""

ISSUES=0

# Check 1: 精確重複
echo "  [1/3] 檢查精確重複..."
comm -12 "$TMPDIR/routes_handle.txt" "$TMPDIR/routes_func.txt" 2>/dev/null > "$TMPDIR/dups.txt" || true
DUP_COUNT=0
while IFS= read -r route; do
  [ -z "$route" ] && continue
  echo "    ❌ DUPLICATE: $route"
  DUP_COUNT=$((DUP_COUNT + 1))
  ISSUES=$((ISSUES + 1))
done < "$TMPDIR/dups.txt"
[ "$DUP_COUNT" -eq 0 ] && echo "    ✓ 無精確重複"
echo ""

# Check 2: /api/dashboard/X vs /api/X 概念衝突
echo "  [2/3] 檢查概念衝突 (dashboard vs api)..."
grep '^/api/dashboard/' "$TMPDIR/all_routes.txt" | \
  sed 's|^/api/dashboard/||' | sort > "$TMPDIR/dash_stems.txt"
grep '^/api/' "$TMPDIR/all_routes.txt" | \
  grep -v '^/api/dashboard/' | grep -v '^/api/events/' | \
  grep -v '^/api/admin/' | grep -v '^/api/user/' | \
  sed 's|^/api/||' | sort > "$TMPDIR/api_stems.txt"
comm -12 "$TMPDIR/dash_stems.txt" "$TMPDIR/api_stems.txt" 2>/dev/null > "$TMPDIR/concept_dup.txt" || true
COLLISION=0
while IFS= read -r stem; do
  [ -z "$stem" ] && continue
  echo "    ❌ CONFLICT: /api/dashboard/$stem ↔ /api/$stem"
  COLLISION=$((COLLISION + 1))
  ISSUES=$((ISSUES + 1))
done < "$TMPDIR/concept_dup.txt"
[ "$COLLISION" -eq 0 ] && echo "    ✓ 無 dashboard/api 概念衝突"
echo ""

DATA_COUNT=0
grep -q '^/api/data/' "$TMPDIR/all_routes.txt" 2>/dev/null && DATA_COUNT=1 || true
echo "  [3/3] 檢查 /api/data/* 前綴..."
echo "    /api/data/* routes: $DATA_COUNT"
if [ "$DATA_COUNT" -eq 0 ]; then
  echo "    ⚠  WARNING: /api/data/* 前綴不存在"
  echo "    Consumer 若根據 MCP tool name 推測 /api/data/X 路徑會得到 401"
  echo ""
  # 找出 data_get_* 相關工具與實際路徑
  if [ -d "cmd/atlas-mcp/server" ]; then
    echo "    實際對照 (MCP tool → real HTTP path):"
    grep -rn 'Name:' --include='*.go' cmd/atlas-mcp/server/tools_*.go 2>/dev/null | \
      sed -nE 's/.*Name: *"([^"]+)".*/\1/p' | while read -r name; do
      path=$(grep -A3 "Name:.*\"$name\"" cmd/atlas-mcp/server/tools_*.go 2>/dev/null | \
        sed -nE 's|.*(/api/[a-zA-Z0-9_/{}-]+).*|\1|p' | head -1 || true)
      [ -z "$path" ] && continue
      echo "      $name → $path"
    done
  fi
fi
echo ""

if [ "$ISSUES" -gt 0 ]; then
  echo "❌ FAIL: $ISSUES route issue(s) found"
  exit 1
else
  echo "✅ PASS: 路由表無衝突"
fi
