#!/usr/bin/env bash
# =============================================================================
# check_design_tokens.sh — 防止 undefined design token 回歸 (issue #1733)
#
# 背景:shared_web/static/css/pages/stock-quote.css 曾使用 variables.css 未定義的
# legacy token (--text-secondary / --border-color / --spacing-* / --radius-sm /
# --bg-tertiary)。#1733 已全數改用 variables.css 既有 token:
#   --text-secondary → --muted
#   --border-color   → --border
#   --spacing-*      → --space-* (2xs/xs/sm/md/lg/xl/2xl)
#   --radius-sm      → --editorial-radius-sm
#   --bg-tertiary    → --panel (subtle fill) / --panel-l2 (elevated surface)
#
# 此腳本 grep source(css/js/ts/html)中這五族 legacy token 用法;重新出現代表回歸,
# CI 會 fail。檢查範圍限 shared_web/admin_web/client_web,排除 node_modules/dist
# 與 binary(compiled 資產會在其他 freshness 檢查處理)。
#
# 用法:
#   bash scripts/ci/check_design_tokens.sh
#
# 退出碼:0 = 無 legacy token 用法,1 = 發現回歸
# =============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

PATTERN='var\(--text-secondary\)|var\(--border-color\)|var\(--spacing-[0-9]+\)|var\(--bg-tertiary\)|var\(--radius-sm\)'

echo "🎨 Design token check (issue #1733 legacy undefined tokens)..."

# -I: skip binary files(compiled assets); --exclude-dir: skip build output/deps
MATCHES="$(grep -rn -I -E "$PATTERN" shared_web admin_web client_web \
  --include='*.css' --include='*.js' --include='*.ts' --include='*.html' \
  --exclude-dir=node_modules --exclude-dir=dist 2>/dev/null || true)"

if [ -n "$MATCHES" ]; then
  echo "❌ Found legacy undefined design token usage (map to variables.css tokens):"
  echo "$MATCHES"
  exit 1
fi

echo "✅ No legacy undefined design tokens found"
exit 0
