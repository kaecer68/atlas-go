#!/usr/bin/env bash
# tests/scripts/test-generate-drift-coverage.sh
# 生成物 drift 覆蓋面的 **hermetic** 靜態契約測試（E39, 2026-09-27）。
#
# 為什麼存在：`quality.yml` 的 `generate` job 只跑 `go generate .`（root package =
#   只有 cmd/gentags directive），所以 `cmd/atlas-mcp/server/resources.go`（docsgen ->
#   resources_docs.gen.go）與 `tools.go`（descgen -> auto-desc.gen.*）的 drift **不會**
#   被它擋下。實測（fixture commit：只改 docs/reference/traps.md 不重生 gen 檔）：
#   `go generate .` 後 git status --porcelain 為空（綠），`go generate ./cmd/atlas-mcp/...`
#   後則出現 ` M cmd/atlas-mcp/server/resources_docs.gen.go`（紅）。
#
# 本測試只看 workflow 文字，不執行 Go、不連網、不動任何檔案。
# 契約（失敗即 exit 1）：
#   ① quality.yml 的 generate job 必須涵蓋 cmd/atlas-mcp 的生成器（`go generate ./...`
#      或顯式 `go generate ./cmd/atlas-mcp/...`），且保留 porcelain 式 drift 檢查。
#   ② quality.yml 的 generate-docs-embed job（E39 的純 docs 缺口）必須存在且跑
#      `go generate ./cmd/atlas-mcp/...`。
#   ③ ci-cd.yml 既有的較廣覆蓋不得被移除（`go generate ./...` + `./cmd/atlas-mcp/...`）。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
QUALITY="${ROOT}/.github/workflows/quality.yml"
CICD="${ROOT}/.github/workflows/ci-cd.yml"
FAIL=0

ok()  { echo "  ✅ $1"; }
bad() { echo "  ❌ $1"; FAIL=1; }

# 取出某個 job 的 YAML 區塊（job 之間以 column-0 以下、2-space 的 key 為界）。
job_block() {  # <file> <job id>
  awk -v job="$2" '
    $0 == "  " job ":" { inside = 1; print; next }
    inside && /^  [A-Za-z0-9_.-]+:[[:space:]]*$/ { exit }
    inside { print }
  ' "$1"
}

has_mcp_generate() {  # <job block on stdin>
  grep -qE 'go generate (\./\.\.\.|\./cmd/atlas-mcp/\.\.\.)' || return 1
}

for f in "${QUALITY}" "${CICD}"; do
  [ -f "${f}" ] || { echo "❌ 找不到 ${f}"; exit 1; }
done

# ① quality.yml generate job
qgen="$(job_block "${QUALITY}" generate)"
if [ -z "${qgen}" ]; then
  bad "① quality.yml 找不到 generate job（被改名或移除？）"
else
  if has_mcp_generate <<< "${qgen}"; then
    ok "① quality.yml generate job 涵蓋 cmd/atlas-mcp 生成器（docsgen/descgen）"
  else
    bad "① quality.yml generate job 只涵蓋 root package ⇒ docsgen/descgen drift 擋不到"
  fi
  if grep -q 'git status --porcelain' <<< "${qgen}"; then
    ok "① quality.yml generate job 保留 porcelain 式 drift 檢查"
  else
    bad "① quality.yml generate job 的 drift 檢查不是 porcelain 式（漏 untracked gen 檔）"
  fi
fi

# ② quality.yml generate-docs-embed（E39：純 docs PR 的缺口）
qembed="$(job_block "${QUALITY}" generate-docs-embed)"
if [ -z "${qembed}" ]; then
  bad "② quality.yml 缺少 generate-docs-embed job（純 docs PR 的 drift 又會漏）"
else
  if grep -qE 'go generate \./cmd/atlas-mcp/\.\.\.' <<< "${qembed}"; then
    ok "② generate-docs-embed 跑 go generate ./cmd/atlas-mcp/..."
  else
    bad "② generate-docs-embed 未跑 go generate ./cmd/atlas-mcp/..."
  fi
fi

# ③ ci-cd.yml 既有覆蓋不得被移除
cgen="$(job_block "${CICD}" generate)"
if [ -z "${cgen}" ]; then
  bad "③ ci-cd.yml 找不到 generate job"
else
  if grep -qE 'go generate \./\.\.\.' <<< "${cgen}"; then
    ok "③ ci-cd.yml 保留 go generate ./...（現行唯一較廣的覆蓋）"
  else
    bad "③ ci-cd.yml 的 go generate ./... 不見了（覆蓋被移除）"
  fi
  if grep -qE 'go generate \./cmd/atlas-mcp/\.\.\.' <<< "${cgen}"; then
    ok "③ ci-cd.yml 保留 go generate ./cmd/atlas-mcp/..."
  else
    bad "③ ci-cd.yml 的 go generate ./cmd/atlas-mcp/... 不見了"
  fi
fi

if [ "${FAIL}" -eq 0 ]; then
  echo "✅ test-generate-drift-coverage PASS"
  exit 0
fi
echo "❌ test-generate-drift-coverage FAIL"
exit 1
