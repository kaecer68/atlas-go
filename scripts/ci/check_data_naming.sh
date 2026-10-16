#!/usr/bin/env bash
# =============================================================================
# check_data_naming.sh — 驗證 data/ 目錄下所有檔案遵循 data-naming-convention.md
#
# 檢查項目:
#   1. 目錄命名必須是 snake_case（禁止 kebab-case）
#   2. 每日數據檔案必須是 YYYYMMDD_descriptor.json 格式
#   3. JSONL 檔案必須以 .jsonl 結尾
#   4. data/state/ 下不允許平面檔案（P3.0 遷移前的警告）
#   5. 禁止備份檔案（*.backup.*）留在主要目錄中
#
# 用法:
#   bash scripts/ci/check_data_naming.sh          # 完整檢查
#   bash scripts/ci/check_data_naming.sh --json   # JSON 輸出 (CI 整合用)
#   bash scripts/ci/check_data_naming.sh --dry    # 僅報告，不退出 (開發用)
#
# 退出碼: 0 = 通過, 1 = 違規發現, 2 = 檢查跑不成
#         （用法錯誤，或工具／環境失敗；FU-20261016-26 ＋ FU-20261016-29）。
#         `2` 與 `1` 互斥：前者＝這一支**根本沒跑成**，後者＝真的掃到違規。
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

# 旗標解析（FU-20261016-26）：`--json` 與裸字 `json` **等效**；`--dry` 保留；
# 未知旗標 ⇒ rc=2 ＋ 用法。舊版只比對 `$1`／`$2` 的字面值 ⇒ 任何拼法不符的旗標
# （含檔頭宣告的 `--json`）都被靜默丟棄 ＝ 旗標是死的。
OUTPUT_MODE="text"
DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    json|--json) OUTPUT_MODE="json" ;;
    --dry)       DRY_RUN=true ;;
    *)
      echo "unknown option: $arg" >&2
      echo "usage: check_data_naming.sh [--json|json] [--dry]" >&2
      exit 2
      ;;
  esac
done

# 工具／環境失敗（＝這一支「跑不成」）與「真的掃到 0 筆」是兩件事（FU-20261016-29）：
# 舊版把 `find` 失敗的**空集合**當成「沒有違規」⇒ 印 `[PASS] 所有 0 個目錄命名符合 snake_case`、
# `5/5 通過`、rc=0，與「真的沒違規」逐位元組不可區分。
VIOLATIONS=0
WARNINGS=0
TOOL_FAILURES=0
JSON_VIOLATIONS="[]"

if [ -t 1 ] && [ "$OUTPUT_MODE" != "json" ]; then
  RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' NC=''
fi

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; VIOLATIONS=$((VIOLATIONS + 1)); }
log_warn()  { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; WARNINGS=$((WARNINGS + 1)); }
log_info()  { printf "       %s\n" "$1"; }
# log_tool_fail — 「檢查跑不成」（工具缺失／管線 rc 非 0）：與 log_fail（資料違規）**分開計數**，
# 這樣 rc 才能把「跑不成」與「有違規」分開（FU-20261016-29）。
log_tool_fail() { printf "${RED}[FAIL]${NC} %s\n" "$1"; TOOL_FAILURES=$((TOOL_FAILURES + 1)); }

add_json_violation() {
  local check="$1" file="$2" detail="$3"
  if command -v jq >/dev/null 2>&1; then
    JSON_VIOLATIONS=$(echo "$JSON_VIOLATIONS" | jq \
      --arg check "$check" --arg file "$file" --arg detail "$detail" \
      '. + [{"check":$check,"file":$file,"detail":$detail}]')
  fi
}

# R1/R2 exceptions — directories/files that are allowed to NOT follow convention
# Format: "<path>|<reason>"
# These exceptions serve as a registry of known deviations. See data-naming-convention.md §9.

DIR_EXCEPTIONS=(
  "data/state/live/state/|即時交易模組內部結構"
  "data/state-archive/|舊歸檔目錄 (P2.2 清理)"
  "data/state/experiments/archive/|實驗歸檔子目錄"
)

FILE_EXCEPTIONS=(
  "data/state/atlas.db|待 P2.1 決策處理的 SQLite 資料庫"
  "data/state/recommendation_outcomes.jsonl.backup.20260414062052|待 P2.2 清理的備份檔案"
  "data/state-archive/|歸檔目錄內所有檔案"
  "data/state/approvals/_metadata.json|目錄 metadata，非資料檔（T-303）"
  "data/state/capital_flow/_metadata.json|目錄 metadata，非資料檔（T-303）"
  "data/state/experiments/_metadata.json|目錄 metadata，非資料檔（T-303）"
  "data/state/macro/_metadata.json|目錄 metadata，非資料檔（T-303）"
  "data/state/margin/_metadata.json|目錄 metadata，非資料檔（T-303）"
  "data/state/sessions/_metadata.json|目錄 metadata，非資料檔（T-303）"
  "data/fundamentals.json|single-object JSON config（5351 行），不是 JSONL append-only（T-303）"
  "data/sector_data/sector_data.json|single-object JSON config（6 行），不是 JSONL append-only（T-303）"
)

# =============================================================================
# Helper: check if a path is in the exceptions list
# =============================================================================
is_exception() {
  local path="$1"
  local exceptions_name="$2"
  local exceptions_array

  if [ "$exceptions_name" = "dir" ]; then
    exceptions_array=("${DIR_EXCEPTIONS[@]}")
  else
    exceptions_array=("${FILE_EXCEPTIONS[@]}")
  fi

  for entry in "${exceptions_array[@]}"; do
    local prefix="${entry%%|*}"
    if [[ "$path" == "$prefix"* ]]; then
      return 0
    fi
  done
  return 1
}

# =============================================================================
# Helper: keep only paths that are not ignored by git
# Used to skip runtime/generated artifacts under data/ that are not meant to be
# committed. See data-naming-convention.md and .gitignore.
# =============================================================================
# FU-20261016-29：舊版對 `git check-ignore` 的 rc 一無所知（`2>/dev/null` ＋ 管線尾端是 `awk`）
# ⇒ git 失敗時 stdout **空**，與「全部都被 .gitignore 排除」逐字相同 ⇒ 呼叫端看到 0 筆 ⇒ 印通過。
# 現在：① 不在任何 git work tree 內 ⇒ 沒有 ignore 語意可套用 ⇒ 原樣放行（`cat`），
#        ② 在 work tree 內但 `check-ignore` 失敗 ⇒ **不導掉 stderr**，rc 由 pipefail 往上傳。
filter_not_ignored() {
  if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    # `git check-ignore` 的 rc=1 ＝「沒有任何一條路徑被 ignore」（**正常**，`--non-matching` 下
    # 仍會把每一條印出來），rc>=2 才是工具／用法／致命錯誤 ⇒ 只有後者可以往上當失敗傳。
    local rc=0
    git check-ignore --verbose --non-matching --stdin | awk -F'\t' '/^::\t/{print $2}' || rc=$?
    if [ "$rc" -gt 1 ]; then
      echo "filter_not_ignored: git check-ignore 失敗（rc=${rc}）⇒ 無法判定哪些路徑被 .gitignore 排除" >&2
      return "$rc"
    fi
    return 0
  fi
  cat
}

# =============================================================================
# Helper: collect_tree <變數名> <掃描根> <描述> <find 參數…>
#   把「find → filter_not_ignored → sort」的結果收成變數，並把兩種「沒有輸出」**分開**：
#     回傳 0 = 掃描成功（`<變數名>` 為清單；可能是**合法的**空清單）
#     回傳 1 = **掃描根不存在** ⇒ 真的沒有東西可檢查（合法空集合；沿用既有語意印通過／跳過）
#     回傳 2 = **工具／環境失敗**（管線 rc 非 0）⇒ 已印 [FAIL]；呼叫端必須 fail loud
#   `2>/dev/null` 一律不加：工具壞掉要看得到原因（與 FU-20261016-27 同紀律）。
# =============================================================================
collect_tree() {
  local __var="$1" __root="$2" __desc="$3"
  shift 3

  if [ ! -d "$__root" ]; then
    log_info "$__root 不存在 —「${__desc}」沒有東西可檢查（合法空集合，非工具失敗）"
    printf -v "$__var" '%s' ""
    return 1
  fi

  local __out __rc=0
  __out=$(find "$__root" "$@" | filter_not_ignored | sort) || __rc=$?
  if [ "$__rc" -ne 0 ]; then
    log_tool_fail "列出「${__desc}」失敗（find/filter 管線 rc=${__rc}）⇒ 檢查無效，不得視為通過 (FU-20261016-29)"
    return 2
  fi

  printf -v "$__var" '%s' "$__out"
  return 0
}

# =============================================================================
# 檢查 1: 目錄命名必須是 snake_case（禁止 kebab-case）
# R1: snake_case, R8: 禁止 kebab-case. See data-naming-convention.md §2.
# =============================================================================
check_dir_naming() {
  printf "\n═══ 檢查 1/5: 目錄命名 (R1 + R8) ═══\n"
  local found_any=0

  local dirs rc=0
  collect_tree dirs data/ "data/ 目錄" -type d || rc=$?
  [ "$rc" -eq 2 ] && return 1   # 工具失敗 ⇒ fail loud（rc=2），不得與「真的 0 筆」混淆

  while IFS= read -r dir; do
    [ -n "$dir" ] || continue
    local dirname
    dirname=$(basename "$dir")

    # Skip dot-directories (hidden)
    [[ "$dirname" == .* ]] && continue

    # Skip exceptions
    if is_exception "$dir" "dir"; then
      log_info "例外: $dir"
      continue
    fi

    # R8: kebab-case violation
    if echo "$dirname" | grep -q '-'; then
      log_fail "$dir — 目錄名稱包含連字號 (kebab-case)，應使用 snake_case (R8)"
      add_json_violation "kebab_case_dir" "$dir" "directory name '$dirname' contains hyphen, use snake_case"
      found_any=1
    fi

    # R9: no kebab-case period
  done <<< "$dirs"

  if [ "$found_any" -eq 0 ]; then
    # 計數取自**同一次掃描**的清單（不再第二次 find）：舊版第二次 find 也會失敗，
    # 於是印出「所有 0 個目錄」——與工具壞掉的空集合逐字相同。
    local count=0
    if [ -n "$dirs" ]; then
      count=$(printf '%s\n' "$dirs" | wc -l | tr -d ' ')
    fi
    log_pass "所有 ${count} 個目錄命名符合 snake_case (R1, R8)"
  fi
  return $found_any
}

# =============================================================================
# 檢查 2: 每日數據檔案格式 (R2)
# R2: 每日快照必須使用 YYYYMMDD_descriptor.json 格式
# 應用範圍: macro/, margin/, capital_flow/
# =============================================================================
check_daily_file_format() {
  printf "\n═══ 檢查 2/5: 每日數據檔案格式 (R2) ═══\n"
  local found_any=0

  local daily_dirs=("data/state/macro" "data/state/margin" "data/state/capital_flow")

  for dir in "${daily_dirs[@]}"; do
    if [ ! -d "$dir" ]; then
      continue
    fi

    local files rc=0
    collect_tree files "$dir" "$dir 的每日數據檔" \
        -maxdepth 1 -type f \( -name "*.json" -o -name "*.jsonl" \) || rc=$?
    [ "$rc" -eq 2 ] && return 1   # 工具失敗 ⇒ fail loud（rc=2）

    while IFS= read -r file; do
      [ -n "$file" ] || continue
      local fname
      fname=$(basename "$file")

      # Skip metadata and README files
      [[ "$fname" == "_metadata.json" ]] && continue
      [[ "$fname" == "README.md" ]] && continue
      [[ "$fname" == "latest.json" ]] && continue

      # R2: YYYYMMDD_descriptor.json (8 digits + underscore + descriptor + .json)
      # Accept both YYYYMMDD_descriptor.json AND YYYYMMDD.json (legacy)
      # Report YYYYMMDD.json as a warning (needs descriptor per convention)
      if echo "$fname" | grep -qE '^[0-9]{8}\.json$'; then
        log_warn "$file — 日期後缺少描述符，格式應為 YYYYMMDD_descriptor.json (R2)"
        add_json_violation "daily_no_descriptor" "$file" "date-only filename, should be YYYYMMDD_descriptor.json"
        found_any=1
      elif echo "$fname" | grep -qE '^[0-9]{8}_[a-z][a-z_]*\.json$'; then
        : # Valid format, pass
      else
        log_warn "$file — 不標準的每日數據命名: '$fname' (R2)"
        add_json_violation "daily_nonstandard" "$file" "non-standard daily file name: '$fname'"
        found_any=1
      fi
    done <<< "$files"
  done

  if [ "$found_any" -eq 0 ]; then
    log_pass "每日數據檔案命名符合 R2 規範"
  fi
  return 0  # Warnings only, never block
}

# =============================================================================
# 檢查 3: JSONL 檔案副檔名正確 (R4)
# R4: append-only 日誌必須使用 .jsonl，不可使用 .json
# =============================================================================
check_jsonl_extension() {
  printf "\n═══ 檢查 3/5: JSONL 副檔名 (R4) ═══\n"
  local found_any=0

  # Check: all JSONL content uses .jsonl extension
  # T-401 perf: prune high-volume subdirs (mutation briefs accumulate hundreds/day)
  local files rc=0
  collect_tree files data/ "data/ 下的 JSON／JSONL 檔" \
      \( -path 'data/state/sessions' -o \
         -path 'data/state/mutation-briefs' -o \
         -path 'data/state/parameter-snapshots' -o \
         -path 'data/state/sector_index' -o \
         -path 'data/state/baseline_reports' -o \
         -path 'data/state/backtest_results' -o \
         -path 'data/state/live/state' -o \
         -path 'data/state/experiments/archive' -o \
         -path 'data/state-archive' \) -prune -o \
      -type f \( -name "*.json" -o -name "*.jsonl" \) -print || rc=$?
  [ "$rc" -eq 2 ] && return 1   # 工具失敗 ⇒ fail loud（rc=2）

  while IFS= read -r file; do
    [ -n "$file" ] || continue
    local fname
    fname=$(basename "$file")

    if is_exception "$file" "file"; then
      continue
    fi

    # Check if file extension is .jsonl
    if [[ "$fname" != *.jsonl ]]; then
      # Quick check: does it look like JSONL (multiple JSON objects)?
      # If first char is '{' and file has multiple lines with '{', flag it
      if [ -r "$file" ] && head -c1 "$file" 2>/dev/null | grep -q '{'; then
        local line_count
        line_count=$(wc -l < "$file" 2>/dev/null | tr -d ' ')
        if [ "$line_count" -gt 1 ]; then
          log_warn "$file — 可能是 JSONL 內容但使用非 .jsonl 副檔名 (R4)"
          add_json_violation "jsonl_extension" "$file" "multi-line JSON file, should use .jsonl extension"
          found_any=1
        fi
      fi
    fi
  done <<< "$files"

  if [ "$found_any" -eq 0 ]; then
    log_pass "所有 JSONL 檔案使用正確的 .jsonl 副檔名"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 檢查 4: data/state/ 下不允許平面檔案 (data-directory-standard.md §3.2 R1)
# 警告級別 — P3.0 遷移後才變為錯誤
# =============================================================================
check_state_flat_files() {
  printf "\n═══ 檢查 4/5: data/state/ 平面檔案 (R1) ═══\n"
  local found_any=0

  local files rc=0
  collect_tree files data/state/ "data/state/ 下的平面檔" -maxdepth 1 -type f || rc=$?
  [ "$rc" -eq 2 ] && return 1   # 工具失敗 ⇒ fail loud（rc=2）

  while IFS= read -r file; do
    [ -n "$file" ] || continue
    local fname
    fname=$(basename "$file")

    if is_exception "$file" "file"; then
      log_info "例外: $file"
      continue
    fi

    log_warn "$file — data/state/ 下的平面檔案，應遷移至子目錄 (§3.2 R1)"
    add_json_violation "state_flat_file" "$file" "flat file in data/state/, should be in subdirectory (P3.0)"
    found_any=1
  done <<< "$files"

  if [ "$found_any" -eq 0 ]; then
    log_pass "data/state/ 下沒有平面檔案"
  fi
  return 0  # Warnings only until P3.0
}

# =============================================================================
# 檢查 5: 禁止備份和臨時檔案 (R6)
# R6: 備份檔案格式為 .backup.YYYYMMDDHHMMSS
# 備份不應留在主要目錄中 — 應移至 data/archive/
# =============================================================================
check_backup_files() {
  printf "\n═══ 檢查 5/5: 備份與臨時檔案 (R6) ═══\n"
  local found_any=0

  local files rc=0
  collect_tree files data/ "data/ 下的備份檔" -type f -name "*.backup.*" || rc=$?
  [ "$rc" -eq 2 ] && return 1   # 工具失敗 ⇒ fail loud（rc=2）

  while IFS= read -r file; do
    [ -n "$file" ] || continue
    local fname
    fname=$(basename "$file")

    if is_exception "$file" "file"; then
      continue
    fi

    # R6: backup files
    if echo "$fname" | grep -qE '\.backup\.'; then
      log_warn "$file — 備份檔案不應留在主要目錄，請移至 data/archive/ (R6)"
      add_json_violation "backup_in_main" "$file" "backup file should be in data/archive/, not main directories"
      found_any=1
    fi
  done <<< "$files"

  if [ "$found_any" -eq 0 ]; then
    log_pass "主目錄中沒有備份檔案"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Atlas 資料檔案命名規範檢查\n"
  printf "參考文件: docs/data-naming-convention.md\n"
  printf "================================\n\n"

  local r1=0 r2=0 r3=0 r4=0 r5=0

  check_dir_naming || r1=$?
  check_daily_file_format || r2=$?
  check_jsonl_extension || r3=$?
  check_state_flat_files || r4=$?
  check_backup_files || r5=$?

  printf "\n═══════════════════════════════════════\n"

  if [ "$OUTPUT_MODE" = "json" ]; then
    if command -v jq >/dev/null 2>&1; then
      local checks_passed=0
      [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r4" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r5" -eq 0 ] && checks_passed=$((checks_passed + 1))

      jq -n \
        --argjson violations "${JSON_VIOLATIONS:-[]}" \
        --argjson total_violations "$VIOLATIONS" \
        --argjson total_tool_failures "$TOOL_FAILURES" \
        --argjson total_warnings "$WARNINGS" \
        --argjson checks_passed "$checks_passed" \
        '{status:(if $total_tool_failures > 0 then "check_unusable" elif $total_violations > 0 then "violations_found" elif $total_warnings > 0 then "warnings" else "passed" end),total_violations:$total_violations,total_tool_failures:$total_tool_failures,total_warnings:$total_warnings,checks_passed:$checks_passed,checks_total:5}'
    else
      printf '{"status":"ok","note":"jq not available for detailed output"}\n'
    fi
  else
    local checks_passed=0
    [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r4" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r5" -eq 0 ] && checks_passed=$((checks_passed + 1))

    printf "檢查結果: %d/5 通過\n" "$checks_passed"
    if [ "$TOOL_FAILURES" -gt 0 ]; then
      # 不得在工具失敗時印「所有檢查通過」（FU-20261016-29）
      printf "${RED}檢查未完成：%d 處因工具／環境失敗而無法執行${NC}\n" "$TOOL_FAILURES"
    elif [ "$VIOLATIONS" -gt 0 ]; then
      printf "${RED}發現 %d 處違規${NC}\n\n" "$VIOLATIONS"
      printf "修復建議:\n"
      printf "  1. 目錄命名 → 使用 snake_case (參考 data-naming-convention.md §2)\n"
      printf "  2. 每日檔案 → 使用 YYYYMMDD_descriptor.json 格式\n"
      printf "  3. JSONL 檔案 → 使用 .jsonl 副檔名\n"
    elif [ "$WARNINGS" -gt 0 ]; then
      printf "${YELLOW}發現 %d 處警告${NC} (暫不阻擋 CI)\n" "$WARNINGS"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  # T-402: per-check breakdown so the user sees the unique-violation count
  # (matching the JSON_VIOLATIONS array length) instead of a single rolled-up
  # number. The JSON output above already exposes the per-file detail; this
  # gives the text output the same fidelity.
  if [ "$OUTPUT_MODE" != "json" ] && [ -n "${JSON_VIOLATIONS:-}" ]; then
    if command -v jq >/dev/null 2>&1; then
      local r1_count r2_count r4_count r6_count r8_count
      r1_count=$(echo "$JSON_VIOLATIONS" | jq '[.[] | select(.check=="state_flat_file" or .check=="kebab_case_dir")] | length' 2>/dev/null || echo 0)
      r2_count=$(echo "$JSON_VIOLATIONS" | jq '[.[] | select(.check=="daily_no_descriptor" or .check=="daily_nonstandard")] | length' 2>/dev/null || echo 0)
      r4_count=$(echo "$JSON_VIOLATIONS" | jq '[.[] | select(.check=="jsonl_extension")] | length' 2>/dev/null || echo 0)
      r6_count=$(echo "$JSON_VIOLATIONS" | jq '[.[] | select(.check=="backup_in_main")] | length' 2>/dev/null || echo 0)
      r8_count=$(echo "$JSON_VIOLATIONS" | jq '[.[] | select(.check=="kebab_case_dir")] | length' 2>/dev/null || echo 0)
      printf "\n各檢查違規統計（unique files）:\n"
      printf "  R1 平面檔遷移: %s\n" "$r1_count"
      printf "  R2 每日檔格式:  %s\n" "$r2_count"
      printf "  R4 JSONL 副檔名: %s\n" "$r4_count"
      printf "  R6 備份檔位置:  %s\n" "$r6_count"
      printf "  R8 目錄 kebab-case: %s\n" "$r8_count"
    fi
  fi

  # 工具／環境失敗優先：這一支「跑不成」不是「通過」，也不與「真的掃到違規」共用 rc（FU-20261016-29）。
  if [ "$TOOL_FAILURES" -gt 0 ]; then
    if [ "$OUTPUT_MODE" != "json" ]; then
      printf "${RED}⇒ 不得視為通過（FU-20261016-29）${NC}\n"
    else
      # JSON 模式的 stdout 必須留給 JSON 本身 ⇒ 診斷訊息走 stderr
      printf "check unusable: %d tool/environment failure(s) — not a pass (FU-20261016-29)\n" \
        "$TOOL_FAILURES" >&2
    fi
    exit 2
  fi

  # Only exit non-zero for actual violations (not warnings)
  if [ "$VIOLATIONS" -gt 0 ]; then
    if [ "$DRY_RUN" = true ]; then
      printf "\n${YELLOW}DRY RUN — 不退出${NC}\n"
      exit 0
    fi
    exit 1
  fi
  exit 0
}

main "$@"
