#!/usr/bin/env bash
# =============================================================================
# check_maturity.sh — internal/ 模組成熟度標記一致性檢查
#
# 檢查項目:
#   1. 每個 internal/*/ Go package (非 testdata) 都有 doc.go
#   2. 每個 doc.go 有恰好一個 Maturity: 標記
#   3. Maturity: 值為合法值 (stable/evolving/experimental/utility)
#   4. MATURITY.md 存在且列出的模組與 doc.go 一致
#   5. doc.go 和 MATURITY.md 的層級分類互相匹配
#
# 用法:
#   bash scripts/ci/check_maturity.sh          # 完整檢查
#   bash scripts/ci/check_maturity.sh --json   # JSON 輸出 (CI 整合用)
#
# 退出碼: 0 = 通過, 1 = 違規發現
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUTPUT_MODE="${1:-text}"
VIOLATIONS=0
JSON_VIOLATIONS="[]"

if [ -t 1 ] && [ "$OUTPUT_MODE" != "json" ]; then
  RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[1;33m' NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' NC=''
fi

log_pass()  { printf "${GREEN}[PASS]${NC} %s\n" "$1"; }
log_fail()  { printf "${RED}[FAIL]${NC} %s\n" "$1"; }
log_warn()  { printf "${YELLOW}[WARN]${NC} %s\n" "$1"; }
log_info()  { printf "       %s\n" "$1"; }

add_json_violation() {
  local check="$1" file="$2" detail="$3"
  VIOLATIONS=$((VIOLATIONS + 1))
  if command -v jq >/dev/null 2>&1; then
    JSON_VIOLATIONS=$(echo "$JSON_VIOLATIONS" | jq \
      --arg check "$check" --arg file "$file" --arg detail "$detail" \
      '. + [{"check":$check,"file":$file,"detail":$detail}]')
  fi
}

VALID_TIERS="stable evolving experimental utility archived"

# =============================================================================
# 輔助: 取得所有 internal/*/ Go package 目錄 (排除 testdata)
# =============================================================================
get_go_packages() {
  for dir in internal/*/; do
    pkg=$(basename "$dir")
    [ "$pkg" = "testdata" ] && continue
    # 只有包含 .go 檔案的才是 Go package
    if ls "$dir"*.go >/dev/null 2>&1; then
      echo "$pkg"
    fi
  done
}

# =============================================================================
# 檢查 1: 每個 Go package 都有 doc.go
# =============================================================================
check_docgo_exists() {
  printf "\n═══ 檢查 1/4: doc.go 存在性 ═══\n"
  local found_any=0

  while IFS= read -r pkg; do
    if [ ! -f "internal/$pkg/doc.go" ]; then
      log_fail "internal/$pkg/ — 缺少 doc.go"
      add_json_violation "docgo_missing" "internal/$pkg/" "no doc.go found"
      found_any=1
    fi
  done < <(get_go_packages)

  if [ "$found_any" -eq 0 ]; then
    local count
    count=$(get_go_packages | wc -l | tr -d ' ')
    log_pass "所有 ${count} 個 Go package 都有 doc.go"
  fi
  return $found_any
}

# =============================================================================
# 檢查 2: 每個 doc.go 有合法的 Maturity: 標記
# =============================================================================
check_maturity_tags() {
  printf "\n═══ 檢查 2/4: Maturity 標記合法性 ═══\n"
  local found_any=0

  while IFS= read -r pkg; do
    local docfile="internal/$pkg/doc.go"
    local count=0
    count=$(grep -c "Maturity:" "$docfile" 2>/dev/null) || true

    if [ "$count" -eq 0 ]; then
      log_fail "$docfile — 缺少 Maturity: 標記"
      add_json_violation "maturity_missing" "$docfile" "no Maturity: tag"
      found_any=1
      continue
    fi

    if [ "$count" -gt 1 ]; then
      log_fail "$docfile — 有多個 Maturity: 標記 (${count})，應只有一個"
      add_json_violation "maturity_multiple" "$docfile" "$count Maturity: tags found, expected 1"
      found_any=1
      continue
    fi

    local value
    value=$(grep "Maturity:" "$docfile" | sed 's/.*Maturity: *//' | tr -d '[:space:]')

    local valid=0
    for tier in $VALID_TIERS; do
      [ "$value" = "$tier" ] && { valid=1; break; }
    done

    if [ "$valid" -eq 0 ]; then
      log_fail "$docfile — 非法的 Maturity 值: '$value' (合法值: $VALID_TIERS)"
      add_json_violation "maturity_invalid" "$docfile" "invalid Maturity value: '$value'"
      found_any=1
    fi
  done < <(get_go_packages)

  if [ "$found_any" -eq 0 ]; then
    log_pass "所有 Maturity: 標記均合法"
  fi
  return $found_any
}

# =============================================================================
# 檢查 3: MATURITY.md 存在且結構完整
# =============================================================================
check_maturity_md() {
  printf "\n═══ 檢查 3/4: MATURITY.md 存在性 ═══\n"
  local maturity_file="internal/MATURITY.md"

  if [ ! -f "$maturity_file" ]; then
    log_fail "MATURITY.md 不存在: $maturity_file"
    add_json_violation "maturity_md_missing" "$maturity_file" "reference file not found"
    return 1
  fi

  log_pass "MATURITY.md 存在"

  # Verify all five tier sections exist
  for tier_label in "S · Stable" "E · Evolving" "X · Experimental" "U · Utility" "A · Archived"; do
    if ! grep -q "$tier_label" "$maturity_file"; then
      log_fail "MATURITY.md — 缺少 '$tier_label' 章節"
      add_json_violation "maturity_md_structure" "$maturity_file" "missing section: $tier_label"
      return 1
    fi
  done

  log_pass "MATURITY.md 五個層級章節完整"
  return 0
}

# =============================================================================
# 檢查 4: doc.go 與 MATURITY.md 交叉比對
# =============================================================================
check_cross_consistency() {
  printf "\n═══ 檢查 4/4: doc.go ↔ MATURITY.md 一致性 ═══\n"

  local maturity_file="internal/MATURITY.md"
  if [ ! -f "$maturity_file" ]; then
    log_warn "跳過交叉比對 (MATURITY.md 不存在)"
    return 0
  fi

  local found_any=0

  # 從 doc.go 建立 tier map
  declare -A doc_tiers
  while IFS= read -r pkg; do
    local docfile="internal/$pkg/doc.go"
    local value
    value=$(grep "Maturity:" "$docfile" | sed 's/.*Maturity: *//' | tr -d '[:space:]')
    doc_tiers["$pkg"]="$value"
  done < <(get_go_packages)

  # 從 MATURITY.md 解析預期的 tier
  declare -A md_tiers
  local current_tier=""
  while IFS= read -r line; do
    # Detect tier sections
    if echo "$line" | grep -q "S · Stable"; then
      current_tier="stable"
    elif echo "$line" | grep -q "E · Evolving"; then
      current_tier="evolving"
    elif echo "$line" | grep -q "X · Experimental"; then
      current_tier="experimental"
    elif echo "$line" | grep -q "U · Utility"; then
      current_tier="utility"
    elif echo "$line" | grep -q "A · Archived"; then
      current_tier="archived"
    elif echo "$line" | grep -q "非 Package"; then
      current_tier=""
    fi

    # Extract package name from table rows: | `pkgname` | ...
    if [ -n "$current_tier" ]; then
      local pkg_name
      pkg_name=$(echo "$line" | sed -n 's/^| *`\([^`]*\)` *|.*/\1/p')
      if [ -n "$pkg_name" ]; then
        # Skip directory entries (contain /) — those are non-package references
        case "$pkg_name" in
          */*) continue ;;
        esac
        md_tiers["$pkg_name"]="$current_tier"
      fi
    fi
  done < "$maturity_file"

  # Compare: for each doc.go package, check MATURITY.md has same tier
  for pkg in "${!doc_tiers[@]}"; do
    local doc_tier="${doc_tiers[$pkg]}"
    local md_tier="${md_tiers[$pkg]:-}"

    if [ -z "$md_tier" ]; then
      log_fail "internal/$pkg/ — doc.go 標記為 '$doc_tier'，但 MATURITY.md 未列出此模組"
      add_json_violation "cross_missing_md" "internal/$pkg/" "doc.go tier=$doc_tier, not found in MATURITY.md"
      found_any=1
    elif [ "$doc_tier" != "$md_tier" ]; then
      log_fail "internal/$pkg/ — doc.go: $doc_tier ≠ MATURITY.md: $md_tier"
      add_json_violation "cross_mismatch" "internal/$pkg/" "doc.go=$doc_tier, MATURITY.md=$md_tier"
      found_any=1
    fi
  done

  # Reverse: for each MATURITY.md package, check doc.go has it
  for pkg in "${!md_tiers[@]}"; do
    if [ -z "${doc_tiers[$pkg]:-}" ]; then
      log_fail "MATURITY.md 列出 '$pkg' (${md_tiers[$pkg]})，但 internal/$pkg/ 不是 Go package 或無 doc.go"
      add_json_violation "cross_orphan_md" "internal/$pkg/" "listed in MATURITY.md as ${md_tiers[$pkg]}, but no doc.go"
      found_any=1
    fi
  done

  if [ "$found_any" -eq 0 ]; then
    log_pass "doc.go 與 MATURITY.md 完全一致"
  fi
  return $found_any
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Atlas 模組成熟度標記檢查\n========================\n\n"

  local r1=0 r2=0 r3=0 r4=0

  check_docgo_exists || r1=$?
  check_maturity_tags || r2=$?
  check_maturity_md || r3=$?
  check_cross_consistency || r4=$?

  printf "\n═══════════════════════════════════════\n"

  if [ "$OUTPUT_MODE" = "json" ]; then
    if command -v jq >/dev/null 2>&1; then
      local checks_passed=0
      [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r4" -eq 0 ] && checks_passed=$((checks_passed + 1))

      jq -n \
        --argjson violations "${JSON_VIOLATIONS:-[]}" \
        --argjson total "$VIOLATIONS" \
        --argjson checks_passed "$checks_passed" \
        '{status:(if $total > 0 then "violations_found" else "passed" end),total_violations:$total,checks_passed:$checks_passed,checks_total:4}'
    else
      printf '{"status":"ok","note":"jq not available for detailed output"}\n'
    fi
  else
    local checks_passed=0
    [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r4" -eq 0 ] && checks_passed=$((checks_passed + 1))

    printf "檢查結果: %d/4 通過\n" "$checks_passed"
    if [ "$VIOLATIONS" -gt 0 ]; then
      printf "${RED}發現 %d 處違規${NC}\n\n" "$VIOLATIONS"
      printf "修復建議:\n"
      printf "  1. 缺少 doc.go → 建立 internal/<pkg>/doc.go 並加上 Maturity: <tier>\n"
      printf "  2. Maturity 值不合法 → 使用 stable/evolving/experimental/utility\n"
      printf "  3. 不一致 → 更新 MATURITY.md 或 doc.go 使兩者一致\n"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  if [ "$r1" -ne 0 ] || [ "$r2" -ne 0 ] || [ "$r3" -ne 0 ] || [ "$r4" -ne 0 ]; then
    exit 1
  fi
  exit 0
}

main "$@"
