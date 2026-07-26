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
# 退出碼: 0 = 通過, 1 = 違規發現
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUTPUT_MODE="${1:-text}"
DRY_RUN=false
if [ "${1:-}" = "--dry" ] || [ "${2:-}" = "--dry" ]; then
  DRY_RUN=true
fi

VIOLATIONS=0
WARNINGS=0
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
filter_not_ignored() {
  if command -v git >/dev/null 2>&1; then
    git check-ignore --verbose --non-matching --stdin 2>/dev/null | awk -F'\t' '/^::\t/{print $2}'
  else
    cat
  fi
}

# =============================================================================
# 檢查 1: 目錄命名必須是 snake_case（禁止 kebab-case）
# R1: snake_case, R8: 禁止 kebab-case. See data-naming-convention.md §2.
# =============================================================================
check_dir_naming() {
  printf "\n═══ 檢查 1/5: 目錄命名 (R1 + R8) ═══\n"
  local found_any=0

  while IFS= read -r dir; do
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
  done < <(find data/ -type d 2>/dev/null | filter_not_ignored | sort)

  if [ "$found_any" -eq 0 ]; then
    local count
    count=$(find data/ -type d 2>/dev/null | filter_not_ignored | wc -l | tr -d ' ')
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

    while IFS= read -r file; do
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
    done < <(find "$dir" -maxdepth 1 -type f \( -name "*.json" -o -name "*.jsonl" \) 2>/dev/null | filter_not_ignored | sort)
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
  while IFS= read -r file; do
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
  done < <(find data/ \
      \( -path 'data/state/sessions' -o \
         -path 'data/state/mutation-briefs' -o \
         -path 'data/state/parameter-snapshots' -o \
         -path 'data/state/sector_index' -o \
         -path 'data/state/baseline_reports' -o \
         -path 'data/state/backtest_results' -o \
         -path 'data/state/live/state' -o \
         -path 'data/state/experiments/archive' -o \
         -path 'data/state-archive' \) -prune -o \
      -type f \( -name "*.json" -o -name "*.jsonl" \) -print 2>/dev/null | filter_not_ignored | sort)

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

  while IFS= read -r file; do
    local fname
    fname=$(basename "$file")

    if is_exception "$file" "file"; then
      log_info "例外: $file"
      continue
    fi

    log_warn "$file — data/state/ 下的平面檔案，應遷移至子目錄 (§3.2 R1)"
    add_json_violation "state_flat_file" "$file" "flat file in data/state/, should be in subdirectory (P3.0)"
    found_any=1
  done < <(find data/state/ -maxdepth 1 -type f 2>/dev/null | filter_not_ignored | sort)

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

  while IFS= read -r file; do
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
  done < <(find data/ -type f -name "*.backup.*" 2>/dev/null | filter_not_ignored | sort)

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
        --argjson total_warnings "$WARNINGS" \
        --argjson checks_passed "$checks_passed" \
        '{status:(if $total_violations > 0 then "violations_found" elif $total_warnings > 0 then "warnings" else "passed" end),total_violations:$total_violations,total_warnings:$total_warnings,checks_passed:$checks_passed,checks_total:5}'
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
    if [ "$VIOLATIONS" -gt 0 ]; then
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
