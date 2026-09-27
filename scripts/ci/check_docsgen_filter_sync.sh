#!/usr/bin/env bash
# =============================================================================
# check_docsgen_filter_sync.sh — 內嵌 doc 清單與 CI path filter 必須一致
#
# 背景（2026-09-27，E39 缺口）：
#   cmd/atlas-mcp/docsgen 把 10 份 doc 內嵌成 Go 產物
#   cmd/atlas-mcp/server/resources_docs.gen.go。quality.yml 的 `generate` job
#   只在 `code` path filter 為 true 時執行 ⇒ 純 docs PR 會跳過 drift 檢查。
#   補救是 quality.yml 新增窄 job `generate-docs-embed`，由 `embedded_docs`
#   path filter 觸發。
#
#   但那個 path filter 是 docsgen `docFiles` 的**第二份清單**：漏列一份就等於
#   那份 doc 的 drift 又回到「機器沒擋、只有人記得」。本腳本把兩份清單釘死，
#   不相等即 FAIL。
#
# 用法：
#   bash scripts/ci/check_docsgen_filter_sync.sh
#
# 可覆寫環境變數（只為負向證明餵 fixture；CI 不設）：
#   DOCSGEN_SRC  預設 cmd/atlas-mcp/docsgen/main.go
#   WORKFLOW     預設 .github/workflows/quality.yml
#
# 退出碼：0 = 兩份清單一致；1 = 不一致或解析不到（fail-closed）
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

DOCSGEN_SRC="${DOCSGEN_SRC:-cmd/atlas-mcp/docsgen/main.go}"
WORKFLOW="${WORKFLOW:-.github/workflows/quality.yml}"

fail() {
  echo "    ❌ docsgen/filter sync: $1" >&2
  exit 1
}

[ -f "${DOCSGEN_SRC}" ] || fail "missing source file: ${DOCSGEN_SRC}"
[ -f "${WORKFLOW}" ] || fail "missing workflow file: ${WORKFLOW}"

tmp_embed="$(mktemp)"
tmp_filter="$(mktemp)"
trap 'rm -f "${tmp_embed}" "${tmp_filter}"' EXIT

# 1) docsgen docFiles 的檔案路徑（SSOT）
sed -n '/^var docFiles = /,/^}/p' "${DOCSGEN_SRC}" \
  | sed -n 's/.*{"\([^"]*\)".*/\1/p' \
  | sed '/^[[:space:]]*$/d' \
  | sort -u > "${tmp_embed}"

# 2) quality.yml `embedded_docs:` filter 的 path 清單
#    （抓 key 之後所有 `- '...'` 行，遇到第一個非 list 行即停）
awk '
  /^[[:space:]]*embedded_docs:[[:space:]]*$/ { inblk = 1; next }
  inblk {
    if ($0 ~ /^[[:space:]]*-[[:space:]]*/) { print $0; next }
    exit
  }
' "${WORKFLOW}" \
  | sed -e 's/^[[:space:]]*-[[:space:]]*//' -e "s/^'//" -e "s/'[[:space:]]*$//" \
  | sed '/^[[:space:]]*$/d' \
  | sort -u > "${tmp_filter}"

# fail-closed：解析不到任何項目時不可「空集合等於空集合」而假綠
[ -s "${tmp_embed}" ] || fail "parsed 0 paths from ${DOCSGEN_SRC} (docFiles block not found?)"
[ -s "${tmp_filter}" ] || fail "parsed 0 paths from ${WORKFLOW} (embedded_docs filter not found?)"

if ! diff -u "${tmp_embed}" "${tmp_filter}" > /dev/null; then
  echo "    ❌ docsgen embed list != embedded_docs path filter" >&2
  echo "       '<' = ${DOCSGEN_SRC} docFiles, '>' = ${WORKFLOW} embedded_docs" >&2
  echo "       a path only in '<' means edits to that doc will NOT arm generate-docs-embed;" >&2
  echo "       a path only in '>' means the filter arms a job for a doc nothing embeds." >&2
  diff -u "${tmp_embed}" "${tmp_filter}" >&2 || true
  exit 1
fi

count="$(wc -l < "${tmp_embed}" | tr -d ' ')"
echo "    ✅ docsgen docFiles == embedded_docs path filter (${count} paths)"
