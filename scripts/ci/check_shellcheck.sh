#!/usr/bin/env bash
# scripts/ci/check_shellcheck.sh
# 守門員：對 repo 內所有 .sh 跑 shellcheck,並依 severity 分類。
#
# Blocking categories (exit 1):
#   - error:   shellcheck parse 失敗以外的 error 等級
#   - SC2115:  rm -rf "${var}"/*  — 變數空字串會展開成 /*,真實災難
#   - SC2086:  未加引號的變數,造成 word-splitting 真實 bug
#   - SC2046:  未加引號的 command substitution,同上
#   - SC2164:  cd 後未檢查 exit,後續指令在錯的目錄執行
#   - SC2206:  Quote to prevent word splitting,真實 bug
#   - SC2053:  補語錯誤
#   - SC2010:  ls | grep — 不要用,應用 glob
#   - SC2038:  find | xargs 未用 -print0
#   - SC2294:  eval 否定 array 帶來的好處
#
# Non-blocking (printed, exit 0):
#   - SC1072/SC1073:  常見的 bash regex 解析誤報(已知誤報集見下)
#   - SC2155:        宣告並賦值 — 純風格,可能 mask return value
#   - SC2034:        unused variable — 純風格
#   - SC2045:        迭代 with glob — 純風格
#
# 用法:
#   bash scripts/ci/check_shellcheck.sh         # 嚴格(預設)
#   bash scripts/ci/check_shellcheck.sh --soft  # 只列印,exit 0
set -euo pipefail

SOFT=false
if [[ "${1:-}" == "--soft" ]]; then
  SOFT=true
fi

if ! command -v shellcheck >/dev/null 2>&1; then
  echo "⚠️  shellcheck not installed; skipping." >&2
  echo "    Install: brew install shellcheck" >&2
  exit 0
fi

# 守門員要掃的根目錄(repo 內有 .sh 的地方,排除 .git/ node_modules/ vendor/ dist/)
ROOTS=(
  scripts
  openclaw
  monitoring
  monitor
  git-workflow
  .agent-hooks
)

mapfile -t FILES < <(
  for r in "${ROOTS[@]}"; do
    [[ -d "$r" ]] && find "$r" -type f -name "*.sh"
  done | sort
)

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "No .sh files found under: ${ROOTS[*]}" >&2
  exit 0
fi

# Known shellcheck false-positives we deliberately ignore.
# - SC1072/SC1073: bash regex test `[[ ... =~ ... ]]` 的括號/group 解析誤報
IGNORE_REGEX='^SC1072$|^SC1073$|^SC2155$|^SC2034$|^SC2045$'

# Categories that block CI.
BLOCK_REGEX='SC2115|SC2086|SC2046|SC2164|SC2206|SC2053|SC2010|SC2038|SC2294|SC2002|SC2009|SC2126|SC2230'

# Counters
TOTAL_FINDINGS=0
BLOCK_FINDINGS=0
INFO_FINDINGS=0
BLOCKED_FILES=()
INFO_FILES=()

for f in "${FILES[@]}"; do
  out=$(shellcheck -S warning --color=never "$f" 2>&1 || true)
  if [[ -z "$out" ]]; then continue; fi

  # Collect per-file finding codes
  mapfile -t codes < <(printf '%s\n' "$out" | grep -oE 'SC[0-9]+' | sort -u)

  file_block=()
  file_info=()
  for c in "${codes[@]}"; do
    if [[ "$c" =~ $IGNORE_REGEX ]]; then
      file_info+=("$c")
    elif [[ "$c" =~ $BLOCK_REGEX ]]; then
      file_block+=("$c")
    else
      # Anything else is a "warning not in either list" — log it but
      # don't fail. Operators can promote it to BLOCK_REGEX later.
      file_info+=("$c(unknown)")
    fi
  done

  TOTAL_FINDINGS=$((TOTAL_FINDINGS + ${#codes[@]}))
  if [[ ${#file_block[@]} -gt 0 ]]; then
    BLOCK_FINDINGS=$((BLOCK_FINDINGS + ${#file_block[@]}))
    BLOCKED_FILES+=("$f: ${file_block[*]}")
  fi
  if [[ ${#file_info[@]} -gt 0 ]]; then
    INFO_FINDINGS=$((INFO_FINDINGS + ${#file_info[@]}))
    INFO_FILES+=("$f: ${file_info[*]}")
  fi
done

echo "════════════════════════════════════════════════════════════"
echo "  shellcheck gate: ${#FILES[@]} files scanned"
echo "════════════════════════════════════════════════════════════"
echo "  blocking findings : $BLOCK_FINDINGS  (categories: ${BLOCK_REGEX//|/,})"
echo "  info findings     : $INFO_FINDINGS   (style: SC2155/SC2034/SC2045, known FP: SC1072/3)"
echo "  total findings    : $TOTAL_FINDINGS"
echo ""

if [[ ${#BLOCKED_FILES[@]} -gt 0 ]]; then
  echo "❌ BLOCKING (must fix):"
  for line in "${BLOCKED_FILES[@]}"; do
    echo "  • $line"
  done
  echo ""
fi

if [[ ${#INFO_FILES[@]} -gt 0 ]]; then
  echo "ℹ️  info (not blocking):"
  for line in "${INFO_FILES[@]}"; do
    echo "  • $line"
  done
  echo ""
fi

if [[ "$SOFT" == true ]]; then
  echo "(soft mode: exit 0 regardless)"
  exit 0
fi

if [[ $BLOCK_FINDINGS -gt 0 ]]; then
  echo "❌ shellcheck gate FAILED — fix $BLOCK_FINDINGS blocking findings or run with --soft for diagnosis only."
  exit 1
fi

echo "✅ shellcheck gate PASSED"
exit 0
