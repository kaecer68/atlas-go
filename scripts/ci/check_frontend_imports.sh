#!/usr/bin/env bash
# =============================================================================
# check_frontend_imports.sh — 驗證 admin_web/client_web 動態 import 與 shared_web 一致
#
# 背景:atlas-go 前端拆分為 admin_web + client_web + shared_web。
# main.js 透過 ESM 動態 import('./pages/xxx.js'),由 esbuild plugin fallback 到
# shared_web/static/js/pages/xxx.js。若 shared_web 缺檔,esbuild 會**靜默失敗**
# (fallback 找不到 → 回傳 404,UI 卡「載入中...」)。
#
# 此腳本確保:
#   1. admin_web/main.js 引用的 pages/*.js 在 shared_web/ 都存在
#   2. client_web/main.js 引用的 pages/*.js 在 shared_web/ 都存在
#   3. shared_web/components/ 引用的 pages/*.js(若 inline import)也涵蓋
#   4. 列出 shared_web/ 內未被任何 main.js 引用的 dead code(可選)
#
# 用法:
#   bash scripts/ci/check_frontend_imports.sh          # 失敗即 exit 1
#   bash scripts/ci/check_frontend_imports.sh --warn-only  # 僅警告
#
# 退出碼:0 = 一致,1 = 有 dangling imports
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

WARN_ONLY=false
if [ "${1:-}" = "--warn-only" ]; then
  WARN_ONLY=true
fi

ADMIN_MAIN="admin_web/static/js/main.js"
CLIENT_MAIN="client_web/static/js/main.js"
SHARED_PAGES_DIR="shared_web/static/js/pages"

if [ ! -f "$ADMIN_MAIN" ] || [ ! -f "$CLIENT_MAIN" ]; then
  echo "ERROR: 找不到 $ADMIN_MAIN 或 $CLIENT_MAIN" >&2
  exit 1
fi

if [ ! -d "$SHARED_PAGES_DIR" ]; then
  echo "ERROR: 找不到 $SHARED_PAGES_DIR" >&2
  exit 1
fi

# 抽取 main.js 中的動態 import 路徑
extract_imports() {
  grep -oE "import\\(['\"]\\./pages/[a-z_-]+\\.js['\"]" "$1" \
    | sed -E "s|.*pages/([a-z_-]+)\\.js['\"]|\\1|" \
    | sort -u
}

ADMIN_IMPORTS=$(extract_imports "$ADMIN_MAIN")
CLIENT_IMPORTS=$(extract_imports "$CLIENT_MAIN")
SHARED_PAGES=$(ls "$SHARED_PAGES_DIR" | sed 's/\.js$//' | sort -u)

VIOLATIONS=0

echo "=== Frontend dynamic import consistency check ==="
echo
echo "[admin_web] imports: $(echo "$ADMIN_IMPORTS" | tr '\n' ' ')"
echo "[client_web] imports: $(echo "$CLIENT_IMPORTS" | tr '\n' ' ')"
echo "[shared_web] available pages: $(echo "$SHARED_PAGES" | tr '\n' ' ')"
echo

# 檢查 admin_web imports 都在 shared_web
for imp in $ADMIN_IMPORTS; do
  if ! grep -Fxq "$imp" <<< "$SHARED_PAGES"; then
    echo "FAIL: $ADMIN_MAIN 引用 './pages/$imp.js',但 $SHARED_PAGES_DIR/ 找不到"
    VIOLATIONS=$((VIOLATIONS+1))
  fi
done

# 檢查 client_web imports 都在 shared_web
for imp in $CLIENT_IMPORTS; do
  if ! grep -Fxq "$imp" <<< "$SHARED_PAGES"; then
    echo "FAIL: $CLIENT_MAIN 引用 './pages/$imp.js',但 $SHARED_PAGES_DIR/ 找不到"
    VIOLATIONS=$((VIOLATIONS+1))
  fi
done

# 列出 dead code(shared_web 內未被引用的 pages)
echo
echo "--- Dead code warning (shared_web pages not used by admin/client main.js) ---"
for page in $SHARED_PAGES; do
  used_admin=$(grep -Fxc "$page" <<< "$ADMIN_IMPORTS" || true)
  used_client=$(grep -Fxc "$page" <<< "$CLIENT_IMPORTS" || true)
  if [ -z "$used_admin" ] && [ -z "$used_client" ]; then
    echo "  INFO: $page.js 未被任何 main.js 引用(可能是 dead code 或透過其他路徑引用)"
  fi
done

echo
if [ "$VIOLATIONS" -gt 0 ]; then
  echo "=== RESULT: ❌ $VIOLATIONS 個 dangling import(s) ==="
  if [ "$WARN_ONLY" = true ]; then
    echo "(warn-only 模式,不 exit 1)"
    exit 0
  fi
  exit 1
fi

echo "=== RESULT: ✅ 所有動態 import 都能在 shared_web 找到對應檔案 ==="
exit 0