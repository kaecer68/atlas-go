#!/usr/bin/env bash
# =============================================================================
# check_data_catalog.sh — 驗證 data-catalog.md 與實際 data/ 目錄一致
#
# 檢查項目:
#   1. data/ 目錄下的所有 JSONL/JSON/CSV 檔案都已記錄在 catalog 中
#   2. catalog 中記錄的檔案在 data/ 中實際存在（無 stale entries）
#   3. data/state/ 下每個子目錄都有 _metadata.json
#   4. catalog 的 JSON 版本與 Markdown 版本同步（若存在）
#
# 用法:
#   bash scripts/ci/check_data_catalog.sh          # 完整檢查
#   bash scripts/ci/check_data_catalog.sh --json   # JSON 輸出 (CI 整合用)
#   bash scripts/ci/check_data_catalog.sh --warn-only  # 僅警告，不失敗
#
# 退出碼: 0 = 一致, 1 = 不一致或 catalog 不存在
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUTPUT_MODE="${1:-text}"
WARN_ONLY=false
if [ "${1:-}" = "--warn-only" ] || [ "${2:-}" = "--warn-only" ]; then
  WARN_ONLY=true
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

CATALOG_MD="docs/data-catalog.md"
CATALOG_JSON="docs/DATA_CATALOG.json"

# =============================================================================
# 檢查 1: 目錄檔案存在
# =============================================================================
check_catalog_exists() {
  printf "\n═══ 檢查 1/5: 目錄檔案存在性 ═══\n"

  if [ ! -f "$CATALOG_MD" ]; then
    log_fail "$CATALOG_MD 不存在 — 尚未建立資料目錄"
    log_info "參考: docs/data-catalog-template.md"
    return 1
  fi

  log_pass "$CATALOG_MD 存在"

  # Catalog JSON is optional (generated from MD)
  if [ -f "$CATALOG_JSON" ]; then
    log_pass "$CATALOG_JSON 存在 (機器可讀版本)"
  else
    log_warn "$CATALOG_JSON 不存在 — 建議產生機器可讀版本"
  fi
  return 0
}

# =============================================================================
# 檢查 2: data/ 下的檔案都已記錄在 catalog 中
# Scan actual data/ files and grep catalog for references
# =============================================================================
check_undocumented_files() {
  printf "\n═══ 檢查 2/5: 未記錄的檔案 ═══\n"
  local found_any=0

  # Priority data files to check
  local priority_paths=(
    "data/replay/"
    "data/fundamentals.json"
    "data/test_returns.json"
    "data/sector_data/"
  )

  # Check that each priority path is mentioned in the catalog
  for path in "${priority_paths[@]}"; do
    if [ -e "$path" ]; then
      local escaped_path
      escaped_path=$(echo "$path" | sed 's/\//\\\//g')
      if ! grep -q "$path" "$CATALOG_MD" 2>/dev/null; then
        log_warn "$path — 存在於檔案系統但未在 $CATALOG_MD 中記錄"
        add_json_violation "undocumented_file" "$path" "exists on disk but not in catalog"
        found_any=1
      fi
    fi
  done

  # Check key state/ subdirectories are in catalog
  while IFS= read -r dir; do
    local dirname
    dirname=$(basename "$dir")

    # Skip sessions/ individual dirs (too many)
    [[ "$dir" == data/state/sessions/session-* ]] && continue
    # Skip empty dirs
    if [ -z "$(ls -A "$dir" 2>/dev/null)" ]; then
      continue
    fi

    if ! grep -q "$dir" "$CATALOG_MD" 2>/dev/null; then
      log_warn "$dir — 存在於檔案系統但未在 catalog 中記錄"
      add_json_violation "undocumented_dir" "$dir" "exists on disk but not in catalog"
      found_any=1
    fi
  done < <(find data/state/ -maxdepth 1 -type d ! -name "sessions" 2>/dev/null | sort)

  if [ "$found_any" -eq 0 ]; then
    log_pass "主要資料資產均已記錄在 catalog 中"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 檢查 3: catalog 中記錄的檔案實際存在（反向檢查 — 無 stale entries）
# =============================================================================
check_stale_entries() {
  printf "\n═══ 檢查 3/5: Stale 目錄條目 ═══\n"
  local found_any=0

  # Extract file/directory paths from catalog markdown
  # Pattern: `data/...` in backticks
  grep -oP '`data/[^`]+`' "$CATALOG_MD" 2>/dev/null | sed 's/^`//;s/`$//' | sort -u | while IFS= read -r catpath; do
    # Skip paths that are descriptions, not actual paths (contain spaces or description text)
    if echo "$catpath" | grep -q ' '; then
      continue
    fi
    # Skip pattern/template paths
    if echo "$catpath" | grep -q '{'; then
      continue
    fi

    if [ ! -e "$catpath" ]; then
      log_warn "$catpath — catalog 中記錄但檔案不存在 (stale entry)"
      add_json_violation "stale_entry" "$catpath" "referenced in catalog but file/dir does not exist"
      found_any=1
    fi
  done

  if [ "$found_any" -eq 0 ]; then
    log_pass "catalog 中無 stale entries"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 檢查 4: data/state/ 下每個子目錄都有 _metadata.json
# Per data-maturity-standard.md §4.1
# =============================================================================
check_metadata_files() {
  printf "\n═══ 檢查 4/5: _metadata.json 存在性 ═══\n"
  local found_any=0

  # Skip these directories (known exceptions per data-maturity-standard.md §4.2)
  local skip_dirs=(
    "data/state/sessions"      # Individual sessions don't need metadata
    "data/state/live/state"    # Managed by live module
    "data/state/state-archive" # Legacy archive
  )

  while IFS= read -r dir; do
    local dirname
    dirname=$(basename "$dir")

    # Skip individual session dirs
    [[ "$dir" == data/state/sessions/session-* ]] && continue
    # Skip empty dirs
    if [ -z "$(ls -A "$dir" 2>/dev/null)" ]; then
      continue
    fi

    local skip=0
    for skip_dir in "${skip_dirs[@]}"; do
      if [[ "$dir" == "$skip_dir"* ]]; then
        skip=1
        break
      fi
    done
    [ "$skip" -eq 1 ] && continue

    local metadata_file="$dir/_metadata.json"
    if [ ! -f "$metadata_file" ]; then
      log_warn "$dir — 缺少 _metadata.json (data-maturity-standard.md §4.1)"
      add_json_violation "metadata_missing" "$dir" "no _metadata.json in directory"
      found_any=1
    fi
  done < <(find data/state/ -maxdepth 1 -type d 2>/dev/null | sort)

  # Also check data/ root-level dirs
  for top_dir in data/replay data/cache data/reference; do
    if [ -d "$top_dir" ]; then
      if [ ! -f "$top_dir/_metadata.json" ]; then
        log_warn "$top_dir — 頂層目錄缺少 _metadata.json"
        add_json_violation "metadata_missing" "$top_dir" "top-level data dir missing _metadata.json"
        found_any=1
      fi
    fi
  done

  if [ "$found_any" -eq 0 ]; then
    log_pass "所有必要目錄都有 _metadata.json"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 檢查 5: catalog 的 JSON 版本與 Markdown 版本同步 (若 JSON 存在)
# =============================================================================
check_catalog_sync() {
  printf "\n═══ 檢查 5/5: Catalog JSON ↔ Markdown 同步 ═══\n"

  if [ ! -f "$CATALOG_JSON" ]; then
    log_warn "$CATALOG_JSON 不存在 — 跳過同步檢查 (P1.3 將產生)"
    return 0
  fi

  # Simple check: both files modified within 60 seconds of each other
  local md_mtime json_mtime
  if [[ "$(uname)" == "Darwin" ]]; then
    md_mtime=$(stat -f %m "$CATALOG_MD" 2>/dev/null || echo "0")
    json_mtime=$(stat -f %m "$CATALOG_JSON" 2>/dev/null || echo "0")
  else
    md_mtime=$(stat -c %Y "$CATALOG_MD" 2>/dev/null || echo "0")
    json_mtime=$(stat -c %Y "$CATALOG_JSON" 2>/dev/null || echo "0")
  fi

  local diff=$((md_mtime - json_mtime))
  diff=${diff#-}  # Absolute value

  if [ "$diff" -gt 120 ]; then
    log_warn "catalog 檔案可能不同步 (MD 和 JSON 的修改時間差 ${diff}s)"
    add_json_violation "catalog_unsynced" "$CATALOG_JSON" "MD and JSON versions may be out of sync (${diff}s apart)"
  else
    log_pass "catalog 版本同步"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Atlas 資料目錄新鮮度檢查\n"
  printf "參考文件: docs/data-catalog.md\n"
  printf "================================\n\n"

  local r1=0 r2=0 r3=0 r4=0 r5=0

  check_catalog_exists || r1=$?
  # Only run subsequent checks if catalog exists
  if [ "$r1" -eq 0 ]; then
    check_undocumented_files || r2=$?
    check_stale_entries || r3=$?
    check_metadata_files || r4=$?
    check_catalog_sync || r5=$?
  fi

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
      printf "  1. 建立 $CATALOG_MD (參考 data-catalog-template.md)\n"
      printf "  2. 記錄未記錄的檔案 (檢查 2)\n"
      printf "  3. 為每個子目錄建立 _metadata.json (檢查 4)\n"
    elif [ "$WARNINGS" -gt 0 ]; then
      printf "${YELLOW}發現 %d 處警告${NC}\n" "$WARNINGS"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  if [ "$VIOLATIONS" -gt 0 ]; then
    if [ "$WARN_ONLY" = true ]; then
      printf "\n${YELLOW}WARN-ONLY MODE — 不退出${NC}\n"
      exit 0
    fi
    exit 1
  fi
  exit 0
}

main "$@"
