#!/usr/bin/env bash
# =============================================================================
# validate_json_schemas.sh — 驗證 JSONL 檔案是否符合其 JSON Schema
#
# 檢查項目:
#   1. schemas/ 目錄存在且包含 *.schema.json 檔案
#   2. 每個 schema 有對應的 JSONL 資料檔案（反之則警告）
#   3. 使用 Python jsonschema 逐行驗證 JSONL（streaming，支援大檔案）
#   4. 預設僅驗證前 1000 行（CI 快速模式），--full 可驗證全部
#
# 依賴: python3 + jsonschema 套件 (pip install jsonschema)
#       若無則使用 fallback jq-based basic 檢查
#
# 用法:
#   bash scripts/ci/validate_json_schemas.sh          # 快速模式 (前 1000 行)
#   bash scripts/ci/validate_json_schemas.sh --full   # 完整模式 (所有行)
#   bash scripts/ci/validate_json_schemas.sh --json   # JSON 輸出
#   bash scripts/ci/validate_json_schemas.sh --dry    # 僅檢查 schema 存在性
#
# 退出碼: 0 = 通過, 1 = 驗證失敗, 2 = 缺少依賴
# =============================================================================

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

OUTPUT_MODE="${1:-text}"
FULL_MODE=false
DRY_MODE=false
MAX_LINES=1000

for arg in "$@"; do
  case "$arg" in
    --full) FULL_MODE=true ;;
    --dry) DRY_MODE=true ;;
    --json) OUTPUT_MODE="json" ;;
  esac
done

if $FULL_MODE; then
  MAX_LINES=999999  # Effectively all lines
fi

VIOLATIONS=0
WARNINGS=0
SCHEMA_COUNT=0
VALIDATED_COUNT=0
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

SCHEMAS_DIR="schemas"
HAS_JSONSCHEMA=false

# =============================================================================
# 檢查 0: 依賴可用性
# =============================================================================
check_dependencies() {
  if python3 -c "import jsonschema" 2>/dev/null; then
    HAS_JSONSCHEMA=true
    return 0
  fi

  if command -v jq >/dev/null 2>&1; then
    log_warn "Python jsonschema 未安裝 — 使用 jq fallback (僅檢查 JSON 格式有效性)"
    log_info "完整驗證請安裝: pip install jsonschema"
    return 0
  fi

  log_warn "無 JSON 驗證工具 (python3/jsonschema 或 jq) — 僅檢查 schema 存在性"
  return 0
}

# =============================================================================
# 檢查 1: schemas/ 目錄存在且有 schema 檔案
# =============================================================================
check_schemas_exist() {
  printf "\n═══ 檢查 1/3: Schema 檔案存在性 ═══\n"

  if [ ! -d "$SCHEMAS_DIR" ]; then
    log_warn "$SCHEMAS_DIR/ 目錄不存在 — 尚未建立任何 JSON Schema"
    log_info "參考: docs/JSON_SCHEMA_STANDARD.md"
    return 0
  fi

  while IFS= read -r schema; do
    SCHEMA_COUNT=$((SCHEMA_COUNT + 1))
    local name
    name=$(basename "$schema" .schema.json)
    log_info "Schema: $name → $schema"
  done < <(find "$SCHEMAS_DIR" -maxdepth 1 -name "*.schema.json" 2>/dev/null | sort)

  if [ "$SCHEMA_COUNT" -eq 0 ]; then
    log_warn "$SCHEMAS_DIR/ 存在但無 .schema.json 檔案"
    return 0
  fi

  log_pass "找到 ${SCHEMA_COUNT} 個 JSON Schema"
  return 0
}

# =============================================================================
# Helper: find JSONL files matching a schema name
# =============================================================================
find_data_files() {
  local schema_name="$1"
  # Search for matching JSONL files
  find data/ -type f -name "${schema_name}.jsonl" 2>/dev/null
}

# =============================================================================
# Helper: validate a single JSON line against schema using Python jsonschema
# =============================================================================
validate_line_python() {
  local schema_file="$1" line_num="$2"

  # JSON line via stdin, file paths via sys.argv — avoids shell injection
  python3 -c "
import json, sys
from jsonschema import validate, ValidationError

schema_file = sys.argv[1]
line_num = int(sys.argv[2])

try:
    instance = json.loads(sys.stdin.read())
    with open(schema_file) as f:
        schema = json.load(f)
    validate(instance=instance, schema=schema)
    sys.exit(0)
except ValidationError as e:
    print(f'Line {line_num}: Schema validation failed — {e.message}', file=sys.stderr)
    sys.exit(1)
except json.JSONDecodeError as e:
    print(f'Line {line_num}: Invalid JSON — {e}', file=sys.stderr)
    sys.exit(2)
except Exception as e:
    print(f'Line {line_num}: Error — {e}', file=sys.stderr)
    sys.exit(3)
" "$schema_file" "$line_num" 2>&1
}

# =============================================================================
# Helper: validate a single JSON line using jq fallback
# =============================================================================
validate_line_jq() {
  local line="$1" line_num="$2" data_file="$3"
  if echo "$line" | jq empty 2>/dev/null; then
    return 0
  else
    echo "Line ${line_num}: Invalid JSON" >&2
    return 1
  fi
}

# =============================================================================
# 檢查 2: JSONL 檔案對應的 schema 存在（反向檢查）
# =============================================================================
check_data_without_schema() {
  printf "\n═══ 檢查 2/3: 無 Schema 的 JSONL 檔案 ═══\n"
  local found_any=0

  # Priority JSONL files that should have schemas
  local priority_files=(
    "data/state/recommendation_outcomes.jsonl"
    "data/state/experiments.jsonl"
    "data/state/human_interventions.jsonl"
    "data/state/darwinian_history.jsonl"
    "data/state/metrics.jsonl"
    "data/state/clamping_events.jsonl"
  )

  for datafile in "${priority_files[@]}"; do
    if [ ! -f "$datafile" ]; then
      continue
    fi

    local basename_noext
    basename_noext=$(basename "$datafile" .jsonl)
    local expected_schema="$SCHEMAS_DIR/${basename_noext}.schema.json"

    if [ ! -f "$expected_schema" ]; then
      log_warn "$datafile — 無對應 Schema: $expected_schema (P1.2 待建立)"
      add_json_violation "schema_missing" "$datafile" "no schema at $expected_schema"
      found_any=1
    fi
  done

  # Also check per-session outcomes
  if [ -d "data/state/sessions" ]; then
    local session_outcome_count
    session_outcome_count=$(find data/state/sessions/ -name "recommendation_outcomes.jsonl" 2>/dev/null | wc -l | tr -d ' ')
    if [ "$session_outcome_count" -gt 0 ]; then
      local session_schema="$SCHEMAS_DIR/recommendation_outcomes.schema.json"
      if [ ! -f "$session_schema" ]; then
        log_warn "data/state/sessions/*/recommendation_outcomes.jsonl (${session_outcome_count} dirs) — 無對應 Schema"
      fi
    fi
  fi

  if [ "$found_any" -eq 0 ]; then
    log_pass "所有優先級 JSONL 檔案有對應 Schema（或尚無檔案需檢查）"
  fi
  return 0  # Warnings only
}

# =============================================================================
# 檢查 3: Schema 驗證（若 Python jsonschema 可用）
# =============================================================================
check_schema_validation() {
  printf "\n═══ 檢查 3/3: Schema 驗證 ═══\n"

  if [ "$SCHEMA_COUNT" -eq 0 ]; then
    log_warn "無 Schema 可驗證 — 跳過"
    return 0
  fi

  if [ "$DRY_MODE" = true ]; then
    log_info "DRY MODE — 跳過驗證"
    return 0
  fi

  local total_checked=0
  local total_failed=0

  while IFS= read -r schema; do
    local schema_name
    schema_name=$(basename "$schema" .schema.json)
    local data_files
    data_files=$(find_data_files "$schema_name")

    if [ -z "$data_files" ]; then
      log_warn "$schema_name — Schema 存在但無對應 JSONL 檔案"
      continue
    fi

    for datafile in $data_files; do
      VALIDATED_COUNT=$((VALIDATED_COUNT + 1))
      local lines
      lines=$(wc -l < "$datafile" 2>/dev/null | tr -d ' ')
      local check_lines=$MAX_LINES
      if [ "$lines" -lt "$MAX_LINES" ]; then
        check_lines=$lines
      fi

      if [ "$FULL_MODE" != true ] && [ "$lines" -gt "$MAX_LINES" ]; then
        log_info "驗證 $datafile (前 $check_lines / 共 $lines 行)"
      else
        log_info "驗證 $datafile ($lines 行)"
      fi

      local line_num=0
      local file_failed=0

      while IFS= read -r line; do
        line_num=$((line_num + 1))
        [ "$line_num" -gt "$check_lines" ] && break
        [ -z "$line" ] && continue  # Skip blank lines

        local result
        if $HAS_JSONSCHEMA; then
          result=$(echo "$line" | validate_line_python "$schema" "$line_num" 2>&1) || {
            log_fail "$datafile:$line_num — $result"
            add_json_violation "validation_failed" "$datafile" "line $line_num: $result"
            file_failed=$((file_failed + 1))
            if [ "$file_failed" -ge 10 ]; then
              log_warn "$datafile — 已達 10 個錯誤，跳過剩餘行數"
              break
            fi
          }
        else
          validate_line_jq "$line" "$line_num" "$datafile" || {
            log_fail "$datafile:$line_num — Invalid JSON"
            add_json_violation "invalid_json" "$datafile" "line $line_num: not valid JSON"
            file_failed=$((file_failed + 1))
            if [ "$file_failed" -ge 10 ]; then
              log_warn "$datafile — 已達 10 個錯誤，跳過剩餘行數"
              break
            fi
          }
        fi
      done < "$datafile"

      total_checked=$((total_checked + check_lines))
      total_failed=$((total_failed + file_failed))

      if [ "$file_failed" -eq 0 ]; then
        log_pass "$datafile — 驗證通過 ($check_lines 行)"
      fi
    done
  done < <(find "$SCHEMAS_DIR" -maxdepth 1 -name "*.schema.json" 2>/dev/null | sort)

  if [ "$total_failed" -eq 0 ] && [ "$VALIDATED_COUNT" -gt 0 ]; then
    log_pass "所有 Schema 驗證通過 (${total_checked} 行, ${VALIDATED_COUNT} 檔案)"
  elif [ "$VALIDATED_COUNT" -eq 0 ]; then
    log_warn "沒有 JSONL 檔案需要驗證"
  fi

  printf "      驗證統計: %d 行檢查, %d 失敗, %d 檔案\n" "$total_checked" "$total_failed" "$VALIDATED_COUNT"
  return 0  # Non-blocking until schemas are comprehensive
}

# =============================================================================
# 主流程
# =============================================================================
main() {
  printf "Atlas JSON Schema 驗證\n"
  printf "參考文件: docs/JSON_SCHEMA_STANDARD.md\n"
  printf "模式: %s (max %s 行)\n" "$([ "$FULL_MODE" = true ] && echo '完整' || echo '快速')" "$([ "$FULL_MODE" = true ] && echo '全部' || echo "$MAX_LINES")"
  printf "================================\n\n"

  check_dependencies

  local r1=0 r2=0 r3=0

  check_schemas_exist || r1=$?
  check_data_without_schema || r2=$?
  check_schema_validation || r3=$?

  printf "\n═══════════════════════════════════════\n"

  if [ "$OUTPUT_MODE" = "json" ]; then
    if command -v jq >/dev/null 2>&1; then
      local checks_passed=0
      [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
      [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))

      jq -n \
        --argjson violations "${JSON_VIOLATIONS:-[]}" \
        --argjson total_violations "$VIOLATIONS" \
        --argjson total_warnings "$WARNINGS" \
        --argjson schema_count "$SCHEMA_COUNT" \
        --argjson validated_count "$VALIDATED_COUNT" \
        --argjson checks_passed "$checks_passed" \
        '{status:(if $total_violations > 0 then "violations_found" elif $total_warnings > 0 then "warnings" else "passed" end),total_violations:$total_violations,total_warnings:$total_warnings,schema_count:$schema_count,validated_files:$validated_count,checks_passed:$checks_passed,checks_total:3}'
    else
      printf '{"status":"ok","note":"jq not available for detailed output"}\n'
    fi
  else
    local checks_passed=0
    [ "$r1" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r2" -eq 0 ] && checks_passed=$((checks_passed + 1))
    [ "$r3" -eq 0 ] && checks_passed=$((checks_passed + 1))

    printf "檢查結果: %d/3 通過 | Schema:%d 驗證檔案:%d\n" "$checks_passed" "$SCHEMA_COUNT" "$VALIDATED_COUNT"
    if [ "$VIOLATIONS" -gt 0 ]; then
      printf "${RED}發現 %d 處錯誤${NC}\n\n" "$VIOLATIONS"
      printf "修復建議:\n"
      printf "  1. 修正 JSONL 中不符合 Schema 的欄位\n"
      printf "  2. 若 Schema 定義錯誤 → 更新 schemas/*.schema.json\n"
    elif [ "$WARNINGS" -gt 0 ]; then
      printf "${YELLOW}發現 %d 處警告${NC}\n" "$WARNINGS"
    else
      printf "${GREEN}所有檢查通過${NC}\n"
    fi
  fi

  if [ "$VIOLATIONS" -gt 0 ]; then
    exit 1
  fi
  exit 0
}

main "$@"
