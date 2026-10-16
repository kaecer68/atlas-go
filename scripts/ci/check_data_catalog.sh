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
#   bash scripts/ci/check_data_catalog.sh --warn-only  # 僅警告（限資料發現；工具錯誤仍 fail loud）
#   bash scripts/ci/check_data_catalog.sh --strict     # stale entry 升為違規（紅）
#
# 退出碼:
#   0 = 一致（或 --warn-only 下僅有資料警告）
#   1 = 有違規、或 **檢查本身跑不成**（工具／環境錯誤；此類不受 --warn-only 抑制）
#   2 = 用法錯誤（未知旗標；FU-20261016-26）
#
# 2026-10-16（FU-20261016-22）:
#   `-oP` 是 GNU 專屬 ⇒ macOS 的 BSD grep 回 rc=2／0 行，舊寫法把 stderr 導掉、
#   再加上 `| while` 與 errexit 被 `|| r3=$?` 關掉 ⇒ 掃 0 行仍印 `[PASS] 無 stale entries`。
#   現改為 POSIX ERE（-oE）、rc 顯式判定、逐行讀檔（不經管線子殼）。
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

# 旗標解析（FU-20261016-26）：`--json` 與裸字 `json` **等效**；未知旗標 ⇒ rc=2 ＋ 用法。
# 舊版 `OUTPUT_MODE="${1:-text}"` 只認裸字 `json` ⇒ `--json` 與「不帶旗標」逐位元組相同，
# 且把旗標當成 OUTPUT_MODE 的字面值（`--strict json` 這種順序會連帶丟掉 JSON 模式）。
OUTPUT_MODE="text"
WARN_ONLY=false
STRICT=false
for arg in "$@"; do
  case "$arg" in
    json|--json) OUTPUT_MODE="json" ;;
    --warn-only) WARN_ONLY=true ;;
    --strict)    STRICT=true ;;
    *)
      echo "unknown option: $arg" >&2
      echo "usage: check_data_catalog.sh [--json|json] [--warn-only] [--strict]" >&2
      exit 2
      ;;
  esac
done

VIOLATIONS=0
WARNINGS=0
# 工具／環境錯誤（= 檢查「跑不成」，與「資料有問題」區分；不受 --warn-only 抑制）
TOOL_FAILURES=0
# 這一次檢查內是否出現過 log_fail／log_tool_fail（各 check 進場時歸零）⇒
# 讓「N/5 通過」真的反映結果，而不是永遠 5/5（FU-20261016-22：舊版有違規仍印 5/5 通過）。
CHECK_FAILED=0
JSON_VIOLATIONS="[]"

if [ -t 1 ] && [ "$OUTPUT_MODE" != "json" ]; then
  RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' NC=''
fi

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; VIOLATIONS=$((VIOLATIONS + 1)); CHECK_FAILED=1; }
log_warn()  { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; WARNINGS=$((WARNINGS + 1)); }
log_info()  { printf "       %s\n" "$1"; }
# log_tool_fail — 「檢查跑不成」：工具缺失／rc 非 0,1／掃到 0 筆。與 log_fail（資料違規）分開計數，
# 這樣 --warn-only 只能壓抑資料發現，壓不掉工具錯誤（FU-20261016-22）。
log_tool_fail() { printf "${RED}[FAIL]${NC} %s\n" "$1"; TOOL_FAILURES=$((TOOL_FAILURES + 1)); CHECK_FAILED=1; }

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
  CHECK_FAILED=0

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
  CHECK_FAILED=0
  local found_any=0

  # Priority data files to check
  local priority_paths=(
    "data/replay/"
    "data/fundamentals.json"
    "data/test_returns.json"
    "data/sector_data/"
  )

  # Check that each priority path is mentioned in the catalog
  # grep 的 rc 要分辨「查無（1）」與「工具壞掉（>=2）」：後者既不可當成「未記錄」也不可當成「已記錄」。
  for path in "${priority_paths[@]}"; do
    if [ -e "$path" ]; then
      local grep_rc=0
      grep -qF -- "$path" "$CATALOG_MD" || grep_rc=$?
      case "$grep_rc" in
        0) ;;  # 已記錄
        1)
          log_warn "$path — 存在於檔案系統但未在 $CATALOG_MD 中記錄"
          add_json_violation "undocumented_file" "$path" "exists on disk but not in catalog"
          found_any=1
          ;;
        *)
          log_tool_fail "掃描 $CATALOG_MD 失敗（grep rc=${grep_rc}）⇒ $path 是否已記錄無法判定（不得視為通過）"
          found_any=1
          ;;
      esac
    fi
  done

  # Check key state/ subdirectories are in catalog
  # find 的 rc 也要接：目錄讀不到 ⇒ 0 個子目錄 ⇒ 迴圈空跑 ⇒ 全數「通過」（同型假綠）。
  local state_dirs find_rc=0
  state_dirs=$(find data/state/ -maxdepth 1 -type d ! -name "sessions" 2>&1 | sort) || find_rc=$?
  if [ "$find_rc" -ne 0 ]; then
    log_tool_fail "列出 data/state/ 子目錄失敗（find rc=${find_rc}）⇒ 未記錄檔案檢查無效（不得視為通過）"
    return 1
  fi
  while IFS= read -r dir; do
    [ -n "$dir" ] || continue

    # Skip sessions/ individual dirs (too many)
    [[ "$dir" == data/state/sessions/session-* ]] && continue
    # Skip empty dirs
    if [ -z "$(ls -A "$dir" 2>/dev/null)" ]; then
      continue
    fi

    local dir_grep_rc=0
    grep -qF -- "$dir" "$CATALOG_MD" || dir_grep_rc=$?
    case "$dir_grep_rc" in
      0) ;;
      1)
        log_warn "$dir — 存在於檔案系統但未在 catalog 中記錄"
        add_json_violation "undocumented_dir" "$dir" "exists on disk but not in catalog"
        found_any=1
        ;;
      *)
        log_tool_fail "掃描 $CATALOG_MD 失敗（grep rc=${dir_grep_rc}）⇒ $dir 是否已記錄無法判定（不得視為通過）"
        found_any=1
        ;;
    esac
  done <<< "$state_dirs"

  if [ "$found_any" -eq 0 ]; then
    log_pass "主要資料資產均已記錄在 catalog 中"
  fi
  [ "$CHECK_FAILED" -eq 0 ] || return 1  # 工具錯誤 ⇒ 這一支不算通過
  return 0                                # 資料發現（warn）不算違規
}

# =============================================================================
# 檢查 3: catalog 中記錄的檔案實際存在（反向檢查 — 無 stale entries）
#
# 【2026-10-16 FU-20261016-22：這一支原本在 macOS 上永遠不可能抓到 stale】
#   ① `grep -oP` 是 GNU 專屬：BSD grep（macOS）回 `grep: invalid option -- P`、rc=2、0 行輸出。
#   ② 同一行的 `2>/dev/null` 把 stderr 吞掉；`grep … | sed … | sort -u | while …` 的結束碼是
#      **最後一段（while）** 的 rc（=0），而呼叫端的 `check_stale_entries || r3=$?` 又讓 errexit
#      失效 ⇒ rc=2 全程被吞。
#   ③ `| while` 讓迴圈跑在**管線子殼**內 ⇒ 就算真的找到 stale，`found_any=1` 也回不到父殼
#      ⇒ 無條件印 `[PASS] catalog 中無 stale entries`。
#   ⇒ 實測（main clone，2026-10-16）：同一 pattern 換成 `-E` 得 45 條、其中 5 條 stale，
#      舊版在 macOS 仍印 `[PASS] 無 stale entries`／`5/5 通過` rc=0。
#
# 【為什麼 stale 預設仍是 WARN，不直接 FAIL】
#   `[ ! -e ]` 判的是「**這棵工作樹**裡有沒有」，不是「catalog 是否過期」：
#   乾淨 checkout 實測 45 條中 **37 條**不存在（`data/state/**` 是 runtime 產物，已被 .gitignore
#   排除），而 dev clone 只有 5 條 ⇒ 直接把 stale 當違規會在 CI 造出 30+ 條假紅，並讓
#   `make ci-gate`（pre-push）在資料修好前永遠紅。⇒ 預設 WARN（**計數 + 不得再印 PASS**），
#   需要硬紅時用 `--strict`；catalog 完成「tracked vs runtime」分類後才適合把 CI 升上去。
# =============================================================================
check_stale_entries() {
  printf "\n═══ 檢查 3/5: Stale 目錄條目 ═══\n"
  CHECK_FAILED=0
  local found_any=0
  local scan_file scanned=0

  scan_file="$(mktemp "${TMPDIR:-/tmp}/atlas-data-catalog.XXXXXX")" || {
    log_tool_fail "無法建立暫存檔（mktemp 失敗）⇒ 無法掃描 ${CATALOG_MD}（不得視為通過）"
    return 1
  }

  # Extract file/directory paths from catalog markdown
  # Pattern: `data/...` in backticks
  # 用 POSIX ERE（-oE；BSD/GNU 皆支援）取代 GNU 專屬的 -P，且**不導掉 stderr**
  # —— 工具壞掉必須看得見，rc 由下一段判定。
  local scan_rc=0
  grep -oE '`data/[^`]+`' "$CATALOG_MD" >"$scan_file" || scan_rc=$?
  if [ "$scan_rc" -gt 1 ]; then
    log_tool_fail "抽取 $CATALOG_MD 的 data/ 路徑失敗（grep rc=${scan_rc}）⇒ 本次掃描沒有結果，不得視為通過"
    rm -f "$scan_file"
    return 1
  fi
  # 去重（維持原本 `sort -u` 的語意：同一條路徑只判一次）。sort 的 rc 一樣要接。
  local sort_rc=0
  sort -u -o "$scan_file" "$scan_file" || sort_rc=$?
  if [ "$sort_rc" -ne 0 ]; then
    log_tool_fail "排序／去重掃描結果失敗（sort rc=${sort_rc}）⇒ 檢查無效，不得視為通過"
    rm -f "$scan_file"
    return 1
  fi

  # 逐行讀檔（**不是** `| while`）：計數與 found_any 必須留在同一個殼內。
  local raw catpath
  while IFS= read -r raw || [ -n "$raw" ]; do
    [ -n "$raw" ] || continue
    catpath="${raw#\`}"       # 去掉開頭反引號
    catpath="${catpath%\`}"   # 去掉結尾反引號
    # Skip paths that are descriptions, not actual paths (contain spaces or description text)
    # Skip pattern/template paths
    case "$catpath" in
      *' '*|*'{'*) continue ;;
    esac
    scanned=$((scanned + 1))

    if [ ! -e "$catpath" ]; then
      log_warn "$catpath — catalog 中記錄但檔案不存在 (stale entry)"
      add_json_violation "stale_entry" "$catpath" "referenced in catalog but file/dir does not exist"
      found_any=$((found_any + 1))
    fi
  done <"$scan_file"
  rm -f "$scan_file"

  # 「掃到 0 條」過去等於「通過」（grep rc=2 時走的就是這條路）⇒ 現在必須大聲失敗。
  if [ "$scanned" -eq 0 ]; then
    log_tool_fail "在 $CATALOG_MD 內掃不到任何 data/ 路徑引用（0 條）⇒ 檢查無效，不得視為通過"
    return 1
  fi

  if [ "$found_any" -eq 0 ]; then
    log_pass "catalog 中無 stale entries（已掃描 $scanned 條 data/ 路徑）"
  elif [ "$STRICT" = true ]; then
    log_fail "發現 $found_any 條 stale entry（已掃描 $scanned 條）— --strict 模式視為違規"
  else
    log_warn "發現 $found_any 條 stale entry（已掃描 $scanned 條）— 預設僅警告；要硬紅請用 --strict"
  fi
  [ "$CHECK_FAILED" -eq 0 ] || return 1  # 工具錯誤／--strict 下的 stale ⇒ 不算通過
  return 0
}

# =============================================================================
# 檢查 4: data/state/ 下每個子目錄都有 _metadata.json
# Per data-maturity-standard.md §4.1
# =============================================================================
check_metadata_files() {
  printf "\n═══ 檢查 4/5: _metadata.json 存在性 ═══\n"
  CHECK_FAILED=0
  local found_any=0

  # Skip these directories (known exceptions per data-maturity-standard.md §4.2)
  local skip_dirs=(
    "data/state/sessions"      # Individual sessions don't need metadata
    "data/state/live/state"    # Managed by live module
    "data/state/state-archive" # Legacy archive
  )

  # find 的 rc 要接（同檢查 2）：讀不到目錄 ⇒ 0 個子目錄 ⇒ 「所有必要目錄都有 _metadata.json」假綠。
  local meta_dirs find_rc=0
  meta_dirs=$(find data/state/ -maxdepth 1 -type d 2>&1 | sort) || find_rc=$?
  if [ "$find_rc" -ne 0 ]; then
    log_tool_fail "列出 data/state/ 子目錄失敗（find rc=${find_rc}）⇒ _metadata.json 檢查無效（不得視為通過）"
    return 1
  fi

  while IFS= read -r dir; do
    [ -n "$dir" ] || continue

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
  done <<< "$meta_dirs"

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
  [ "$CHECK_FAILED" -eq 0 ] || return 1  # 工具錯誤 ⇒ 這一支不算通過
  return 0
}

# =============================================================================
# 檢查 5: catalog 的 JSON 版本與 Markdown 版本同步 (若 JSON 存在)
# =============================================================================
check_catalog_sync() {
  printf "\n═══ 檢查 5/5: Catalog JSON ↔ Markdown 同步 ═══\n"
  CHECK_FAILED=0

  if [ ! -f "$CATALOG_JSON" ]; then
    log_warn "$CATALOG_JSON 不存在 — 跳過同步檢查 (P1.3 將產生)"
    return 0
  fi

  # Simple check: both files modified within 60 seconds of each other
  # stat 失敗時**不得**填 0：兩個 mtime 都變 0 ⇒ diff=0 ⇒ 印「catalog 版本同步」假綠。
  local md_mtime json_mtime stat_rc=0
  if [[ "$(uname)" == "Darwin" ]]; then
    md_mtime=$(stat -f %m "$CATALOG_MD") || stat_rc=$?
    json_mtime=$(stat -f %m "$CATALOG_JSON") || stat_rc=$?
  else
    md_mtime=$(stat -c %Y "$CATALOG_MD") || stat_rc=$?
    json_mtime=$(stat -c %Y "$CATALOG_JSON") || stat_rc=$?
  fi
  if [ "$stat_rc" -ne 0 ]; then
    log_tool_fail "讀取 catalog 修改時間失敗（stat rc=${stat_rc}）⇒ 同步檢查無效（不得視為通過）"
    return 1
  fi

  local diff=$((md_mtime - json_mtime))
  diff=${diff#-}  # Absolute value

  if [ "$diff" -gt 120 ]; then
    log_warn "catalog 檔案可能不同步 (MD 和 JSON 的修改時間差 ${diff}s)"
    add_json_violation "catalog_unsynced" "$CATALOG_JSON" "MD and JSON versions may be out of sync (${diff}s apart)"
  else
    log_pass "catalog 版本同步"
  fi
  [ "$CHECK_FAILED" -eq 0 ] || return 1
  return 0
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
    if [ "$VIOLATIONS" -gt 0 ]; then
      printf "${RED}發現 %d 處違規${NC}\n\n" "$VIOLATIONS"
      printf "修復建議:\n"
      printf "  1. 建立 $CATALOG_MD (參考 data-catalog-template.md)\n"
      printf "  2. 記錄未記錄的檔案 (檢查 2)\n"
      printf "  3. 為每個子目錄建立 _metadata.json (檢查 4)\n"
      printf "  4. 修正 catalog 中已不存在的條目，或補上該檔案 (檢查 3；--strict 視為違規)\n"
    elif [ "$WARNINGS" -gt 0 ]; then
      printf "${YELLOW}發現 %d 處警告${NC}\n" "$WARNINGS"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  # 工具／環境錯誤優先且不受 --warn-only 抑制：「檢查跑不成」不是「通過」，也不是可壓抑的警告。
  if [ "$TOOL_FAILURES" -gt 0 ]; then
    printf "${RED}檢查本身無法執行（%d 處工具／環境錯誤）⇒ 不因 --warn-only 而放行${NC}\n" "$TOOL_FAILURES"
    exit 1
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
