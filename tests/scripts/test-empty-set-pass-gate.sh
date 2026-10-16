#!/usr/bin/env bash
# tests/scripts/test-empty-set-pass-gate.sh — 「空集合 ⇒ 印通過」家族的契約測試（FU-20261016-29）
#
# 為什麼要有它
# ------------
# 三支 `scripts/ci` 腳本把「工具／環境失敗」與「真的掃到 0 筆」混為一談：`find`／`ls` 失敗時 stdout 是空的，
# 而「真的沒事」的 stdout 也是空的 ⇒ 兩者在管線輸出上逐字相同 ⇒ 一律落到「通過」分支。實測（修前）：
#   ① `check_data_naming.sh`   注入 `find` 失敗 ⇒ `[PASS] 所有 0 個目錄命名符合 snake_case (R1, R8)`＋`5/5 通過`、rc=0
#   ② `validate_json_schemas.sh` 注入 `find` 失敗 ⇒ `檢查結果: 3/3 通過 | Schema:0 驗證檔案:0`、rc=0
#   ③ `check_data_catalog.sh`  `ls -A` 失敗 ⇒ 每個目錄被當成「空目錄」跳過 ⇒ 少報違規、仍印 N/5 通過、rc=0
#
# 釘住的契約（每條都有對應 case）
#   A1 `check_data_naming.sh`：工具失敗 ⇒ rc=2 ＋ `[FAIL]`（且不得再印「所有 0 個目錄…」）
#   A2 同一 fixture 不注入失敗 ⇒ rc=0 ＋ **真的**計數（證明不是「一律紅」）
#   A3 真的有 kebab-case 目錄 ⇒ rc=1（與工具失敗的 rc=2 互斥，兩者不得混用同一個碼）
#   A4 mutation 自證：把 fail-loud 分支拿掉 ⇒ 假綠復活 ⇒ A1 的斷言有鑑別力
#   A5 合法空集合：掃描根不存在 ⇒ 仍回綠（不得在乾淨 checkout 造假紅）
#   B1 `validate_json_schemas.sh`：`find` 失敗 ⇒ rc=2 ＋ `[FAIL]`（不得再印「所有檢查通過」）
#   B2 `:211` sessions 掃描失敗 ⇒ rc=2（不得靜默跳過）
#   B3 乾淨 fixture ⇒ rc=0（不是一律紅）
#   C1 `check_data_catalog.sh`：`ls -A` 失敗 ⇒ rc=1 ＋ 指名 `ls rc=`（不得再把它當「空目錄」跳過）
#   C2 同 fixture 不注入失敗 ⇒ 真的報出缺 `_metadata.json` 的那個目錄（有鑑別力）
#   C3 乾淨 fixture ⇒ 仍印 `[PASS] 所有必要目錄都有 _metadata.json`（合法空集合沒被誤殺）
#   C4 mutation 自證：把 `ls` 的守衛拿掉 ⇒ 假綠復活
#   D  靜態：三支腳本不得再出現 `find … 2>/dev/null | filter … | wc -l` 的計數形狀
#
# 執行： bash tests/scripts/test-empty-set-pass-gate.sh
set -uo pipefail

# fixture 全部在 mktemp 內；呼叫端 repo 的基線用 git 讀 ⇒ 先 unset，避免被 hook 環境帶歪。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAMING_SH="$REPO/scripts/ci/check_data_naming.sh"
SCHEMA_SH="$REPO/scripts/ci/validate_json_schemas.sh"
CATALOG_SH="$REPO/scripts/ci/check_data_catalog.sh"
REAL_FIND="$(command -v find)"
REAL_LS="$(command -v ls)"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/empty-set-pass.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
ok()   { PASS=$((PASS + 1)); echo "  ✅ $*"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

negproof_require_file "$NAMING_SH"  "check_data_naming.sh"      || exit 1
negproof_require_file "$SCHEMA_SH"  "validate_json_schemas.sh"  || exit 1
negproof_require_file "$CATALOG_SH" "check_data_catalog.sh"     || exit 1

# ── helpers ──────────────────────────────────────────────────────────────────
# 假 find：任何呼叫都失敗（模擬工具缺失／環境壞掉）
make_find_shim() {
  local dir="$1"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\necho "find: simulated tool failure" >&2\nexit 1\n' > "$dir/find"
  chmod +x "$dir/find"
  echo "$dir"
}

# 假 find（只打 sessions 那一次掃描；其餘轉真 find）
make_find_sessions_shim() {
  local dir="$1"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\nfor a in "$@"; do\n  case "$a" in\n    *sessions*) echo "find: simulated sessions scan failure" >&2; exit 1 ;;\n  esac\ndone\nexec %s "$@"\n' "$REAL_FIND" > "$dir/find"
  chmod +x "$dir/find"
  echo "$dir"
}

# 假 ls：`ls -A` 失敗（模擬目錄讀不到）；其餘轉真 ls
make_ls_shim() {
  local dir="$1"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\nfor a in "$@"; do\n  case "$a" in\n    -A) echo "ls: simulated tool failure" >&2; exit 2 ;;\n  esac\ndone\nexec %s "$@"\n' "$REAL_LS" > "$dir/ls"
  chmod +x "$dir/ls"
  echo "$dir"
}

run_with_path() { local p="$1" dir="$2"; shift 2; ( cd "$dir" && PATH="$p:$PATH" bash "$@" ); }

# ── fixture：check_data_naming.sh ─────────────────────────────────────────────
new_naming_fixture() {
  local name="$1" kebab="${2:-}"
  local dir="$TMP/$name"
  mkdir -p "$dir/scripts/ci" "$dir/data/state/kb"
  cp "$NAMING_SH" "$dir/scripts/ci/check_data_naming.sh"
  printf '{"seeded":true}\n' > "$dir/data/state/kb/seed.json"
  if [ -n "$kebab" ]; then mkdir -p "$dir/data/state/$kebab"; fi
  echo "$dir"
}

# ── fixture：validate_json_schemas.sh ────────────────────────────────────────
new_schema_fixture() {
  local name="$1" sessions="${2:-}"
  local dir="$TMP/$name"
  mkdir -p "$dir/scripts/ci" "$dir/schemas" "$dir/data"
  cp "$SCHEMA_SH" "$dir/scripts/ci/validate_json_schemas.sh"
  printf '{"type":"object","required":["a"]}\n' > "$dir/schemas/seed.schema.json"
  if [ -n "$sessions" ]; then
    mkdir -p "$dir/data/state/sessions/session-1"
    printf '{"a":1}\n' > "$dir/data/state/sessions/session-1/recommendation_outcomes.jsonl"
  fi
  echo "$dir"
}

# ── fixture：check_data_catalog.sh ───────────────────────────────────────────
# metadata=missing ⇒ data/state/kb 有檔案但沒有 _metadata.json（＝這一支**應該**報出來的東西）
new_catalog_fixture() {
  # mode: none = 全部目錄都缺 _metadata.json ／ top-only = 只有 data/state/kb 缺
  #       （top-only 是**唯一**能讓「data/state/kb 被跳過 ⇒ 印 PASS」可觀測的形狀）
  #       all = 全部都有
  local name="$1" mode="${2:-none}"
  local dir="$TMP/$name"
  mkdir -p "$dir/scripts/ci" "$dir/docs" "$dir/data/state/kb" "$dir/data/replay" "$dir/data/cache" "$dir/data/reference"
  cp "$CATALOG_SH" "$dir/scripts/ci/check_data_catalog.sh"
  printf 'seed\n' > "$dir/data/state/kb/seed.json"
  printf 'v\n' > "$dir/data/replay/VERSION"
  if [ "$mode" != "none" ]; then
    local d
    for d in state replay cache reference; do
      printf '{"seeded":true}\n' > "$dir/data/$d/_metadata.json"
    done
  fi
  if [ "$mode" = "all" ]; then
    printf '{"seeded":true}\n' > "$dir/data/state/kb/_metadata.json"
  fi
  {
    printf '# fixture catalog（hermetic；不是真 catalog）\n\n'
    printf '| 類型 | 路徑 |\n|---|---|\n'
    printf '| JSON | `data/state/kb/seed.json` |\n'
    printf '| Directory | `data/state/kb/` |\n'
    printf '| JSON | `data/replay/VERSION` |\n'
  } > "$dir/docs/data-catalog.md"
  echo "$dir"
}

echo "→ 「空集合 ⇒ 印通過」家族契約測試（hermetic fixtures；FU-20261016-29）"

# ═══════════════════════════ A: check_data_naming.sh ═══════════════════════
FIND_SHIM="$(make_find_shim "$TMP/shim-find")"
NAMEDIR="$(new_naming_fixture naming-tool-error)"
LOG="$TMP/A1.log"
if negproof_expect 2 "A1 find 失敗 ⇒ rc=2（工具失敗不是通過）" "$LOG" \
   -- run_with_path "$FIND_SHIM" "$NAMEDIR" scripts/ci/check_data_naming.sh; then
  ok "A1 工具失敗 fail loud（rc=2）"
else
  fail "A1 工具失敗沒有 fail loud"; negproof_show_log "$LOG"
fi
if negproof_expect_output "find/filter 管線 rc=1" "$LOG" "A1 必須報出管線 rc"; then ok "A1 指名管線 rc"; else fail "A1 沒報管線 rc"; fi
if grep -qF "所有 0 個目錄命名符合 snake_case" "$LOG"; then
  fail "A1 仍印「所有 0 個目錄命名符合 snake_case」＝假綠復活"; negproof_show_log "$LOG"
else
  ok "A1 不再印「所有 0 個目錄…」"
fi
if grep -qF "5/5 通過" "$LOG"; then fail "A1 仍印「5/5 通過」＝假綠復活"; else ok "A1 不再印「5/5 通過」"; fi

# ── A2 同一形狀但工具正常 ⇒ rc=0 ＋ 真的計數（不是一律紅）─────────────────────
NAMEDIR_OK="$(new_naming_fixture naming-clean)"
LOG="$TMP/A2.log"
if negproof_expect 0 "A2 工具正常 ⇒ rc=0" "$LOG" \
   -- run_with_path "$TMP/empty-path" "$NAMEDIR_OK" scripts/ci/check_data_naming.sh; then
  ok "A2 正常路徑回綠"
else
  fail "A2 正常路徑被誤殺（一律紅）"; negproof_show_log "$LOG"
fi
if negproof_expect_output "[PASS] 所有 3 個目錄命名符合 snake_case (R1, R8)" "$LOG" \
   "A2 綠燈必須是**真的掃過**的計數（fixture 有 data/、data/state、data/state/kb）"; then
  ok "A2 計數來自同一份清單"
else
  fail "A2 計數不是真的掃描結果"; negproof_show_log "$LOG"
fi

# ── A3 真的有違規 ⇒ rc=1（與工具失敗的 rc=2 互斥）────────────────────────────
NAMEDIR_BAD="$(new_naming_fixture naming-kebab my-kebab)"
LOG="$TMP/A3.log"
if negproof_expect 1 "A3 真的有 kebab-case 目錄 ⇒ rc=1（不是 2）" "$LOG" \
   -- run_with_path "$TMP/empty-path" "$NAMEDIR_BAD" scripts/ci/check_data_naming.sh; then
  ok "A3 資料違規仍是 rc=1"
else
  fail "A3 資料違規與工具失敗用了同一個 rc"; negproof_show_log "$LOG"
fi
if negproof_expect_output "kebab-case" "$LOG" "A3 必須指名 kebab-case"; then ok "A3 指名違規型態"; else fail "A3 沒指名違規型態"; fi

# ── A4 mutation 自證：拿掉 fail-loud 分支 ⇒ 假綠復活 ─────────────────────────
NAMEDIR_MUT="$(new_naming_fixture naming-mutated)"
MUT="$NAMEDIR_MUT/scripts/ci/check_data_naming.sh"
sed -i.bak 's/^  if \[ "\$__rc" -ne 0 \]; then$/  if false; then  # MUTATION: fail-loud 分支被拿掉/' "$MUT" && rm -f "$MUT.bak"
if cmp -s "$NAMING_SH" "$MUT"; then
  fail "A4 突變沒有生效（sed 錨點找不到 ⇒ 這條證明是空的）"
else
  LOGM="$TMP/A4.log"
  run_with_path "$FIND_SHIM" "$NAMEDIR_MUT" scripts/ci/check_data_naming.sh >"$LOGM" 2>&1 || true
  if grep -qF "所有 0 個目錄命名符合 snake_case" "$LOGM"; then
    ok "A4 突變 ⇒ 假綠復活 ⇒ A1 的斷言有鑑別力"
  else
    fail "A4 突變後仍未印假綠 ⇒ A1 的斷言可能有別的原因在擋（要釐清）"
    negproof_show_log "$LOGM"
  fi
fi

# ── A5 合法空集合：掃描根不存在 ⇒ 仍回綠（不得造假紅）────────────────────────
NAMEDIR_NODATA="$TMP/naming-nodata"
mkdir -p "$NAMEDIR_NODATA/scripts/ci"
cp "$NAMING_SH" "$NAMEDIR_NODATA/scripts/ci/check_data_naming.sh"
LOG="$TMP/A5.log"
if negproof_expect 0 "A5 沒有 data/（＝真的沒有東西可檢查）⇒ rc=0" "$LOG" \
   -- run_with_path "$TMP/empty-path" "$NAMEDIR_NODATA" scripts/ci/check_data_naming.sh; then
  ok "A5 合法空集合沒有被誤判成工具失敗"
else
  fail "A5 合法空集合被誤殺（對乾淨 checkout 造假紅）"; negproof_show_log "$LOG"
fi
if negproof_expect_output "沒有東西可檢查（合法空集合，非工具失敗）" "$LOG" \
   "A5 必須說明「這是合法空集合」而不是沉默"; then
  ok "A5 合法空集合有明確說明"
else
  fail "A5 沒有把「合法空集合」講清楚（與工具失敗不可辨）"
fi

# ═══════════════════════ B: validate_json_schemas.sh ═══════════════════════
SCHEMADIR="$(new_schema_fixture schema-tool-error sessions)"
LOG="$TMP/B1.log"
if negproof_expect 2 "B1 find 失敗 ⇒ rc=2" "$LOG" \
   -- run_with_path "$FIND_SHIM" "$SCHEMADIR" scripts/ci/validate_json_schemas.sh; then
  ok "B1 工具失敗 fail loud（rc=2）"
else
  fail "B1 工具失敗沒有 fail loud"; negproof_show_log "$LOG"
fi
if grep -qF "所有檢查通過" "$LOG"; then
  fail "B1 仍印「所有檢查通過」＝假綠復活"; negproof_show_log "$LOG"
else
  ok "B1 不再印「所有檢查通過」"
fi
if grep -qF "3/3 通過" "$LOG"; then fail "B1 仍印「3/3 通過」＝假綠復活"; else ok "B1 不再印「3/3 通過」"; fi

# ── B2 :211 的 sessions 掃描失敗 ⇒ 不得靜默跳過 ──────────────────────────────
SESS_SHIM="$(make_find_sessions_shim "$TMP/shim-find-sessions")"
LOG="$TMP/B2.log"
if negproof_expect 2 "B2 sessions 掃描失敗 ⇒ rc=2（不得靜默跳過）" "$LOG" \
   -- run_with_path "$SESS_SHIM" "$SCHEMADIR" scripts/ci/validate_json_schemas.sh; then
  ok "B2 sessions 掃描失敗 fail loud"
else
  fail "B2 sessions 掃描失敗被靜默吞掉"; negproof_show_log "$LOG"
fi
if negproof_expect_output "data/state/sessions/ 的 recommendation_outcomes.jsonl 失敗" "$LOG" \
   "B2 必須指名是哪一支掃描壞了"; then
  ok "B2 指名 sessions 掃描"
else
  fail "B2 沒有指名 sessions 掃描"; negproof_show_log "$LOG"
fi

# ── B3 乾淨 fixture ⇒ rc=0（不是一律紅）──────────────────────────────────────
SCHEMADIR_OK="$(new_schema_fixture schema-clean)"
LOG="$TMP/B3.log"
if negproof_expect 0 "B3 無注入失敗 ⇒ rc=0" "$LOG" \
   -- run_with_path "$TMP/empty-path" "$SCHEMADIR_OK" scripts/ci/validate_json_schemas.sh; then
  ok "B3 正常路徑回綠"
else
  fail "B3 正常路徑被誤殺"; negproof_show_log "$LOG"
fi
if negproof_expect_output "[PASS] 找到 1 個 JSON Schema" "$LOG" "B3 綠燈必須真的數到 schema"; then
  ok "B3 真的數到 schema"
else
  fail "B3 沒有真的數到 schema"; negproof_show_log "$LOG"
fi

# ═════════════════════ check_data_catalog.sh ═════════════════════
LS_SHIM="$(make_ls_shim "$TMP/shim-ls")"
CATDIR="$(new_catalog_fixture catalog-missing-metadata top-only)"
LOG="$TMP/C2.log"
if negproof_expect 0 "C2 無注入失敗 ⇒ rc=0（資料發現僅警告）" "$LOG" \
   -- run_with_path "$TMP/empty-path" "$CATDIR" scripts/ci/check_data_catalog.sh; then
  ok "C2 正常路徑回綠"
else
  fail "C2 正常路徑結束碼不是 0"; negproof_show_log "$LOG"
fi
if negproof_expect_output "[WARN] data/state/kb — 缺少 _metadata.json" "$LOG" \
   "C2 必須真的報出缺 _metadata.json 的目錄（＝C1 的鑑別力來源）"; then
  ok "C2 真的報出缺 metadata 的目錄"
else
  fail "C2 沒有報出缺 metadata 的目錄"; negproof_show_log "$LOG"
fi
if grep -qF "[PASS] 所有必要目錄都有 _metadata.json" "$LOG"; then
  fail "C2 在有違規的情況下仍印 PASS（受測腳本語意可疑）"; negproof_show_log "$LOG"
else
  ok "C2 有違規時不印 PASS"
fi

# ── C1 ls -A 失敗 ⇒ rc=1 ＋ 指名 ls rc（不得當成「空目錄」跳過）───────────────
LOG="$TMP/C1.log"
if negproof_expect 1 "C1 ls -A 失敗 ⇒ rc=1（工具錯誤不得被吞）" "$LOG" \
   -- run_with_path "$LS_SHIM" "$CATDIR" scripts/ci/check_data_catalog.sh; then
  ok "C1 ls 失敗 fail loud（rc=1，與同檔已落地的工具錯誤契約一致）"
else
  fail "C1 ls 失敗沒有 fail loud"; negproof_show_log "$LOG"
fi
if negproof_expect_output "ls rc=2" "$LOG" "C1 必須報出 ls 的 rc"; then ok "C1 指名 ls rc"; else fail "C1 沒報 ls rc"; fi
if negproof_expect_output "不得視為通過" "$LOG" "C1 必須講明後果"; then ok "C1 講明「不得視為通過」"; else fail "C1 沒講明後果"; fi
if grep -qF "[PASS] 所有必要目錄都有 _metadata.json" "$LOG"; then
  fail "C1 仍印「所有必要目錄都有 _metadata.json」＝假綠復活"; negproof_show_log "$LOG"
else
  ok "C1 不再印 metadata PASS 假綠"
fi

# ── C3 乾淨 fixture（每支目錄都有 _metadata.json）⇒ 仍印 PASS ─────────────────
CATDIR_OK="$(new_catalog_fixture catalog-clean all)"
LOG="$TMP/C3.log"
if negproof_expect 0 "C3 乾淨 fixture ⇒ rc=0" "$LOG" \
   -- run_with_path "$TMP/empty-path" "$CATDIR_OK" scripts/ci/check_data_catalog.sh; then
  ok "C3 乾淨 fixture 回綠"
else
  fail "C3 乾淨 fixture 被誤殺"; negproof_show_log "$LOG"
fi
if negproof_expect_output "[PASS] 所有必要目錄都有 _metadata.json" "$LOG" \
   "C3 合法空集合（真的都沒缺）仍必須說 PASS"; then
  ok "C3 合法空集合仍印 PASS"
else
  fail "C3 合法空集合被誤殺（PASS 不見了）"; negproof_show_log "$LOG"
fi

# ── C4 mutation 自證：把 ls 守衛拿掉 ⇒ 目錄又被當成「空目錄」跳過 ──────────────
CATDIR_MUT="$(new_catalog_fixture catalog-mutated top-only)"
MUTC="$CATDIR_MUT/scripts/ci/check_data_catalog.sh"
sed -i.bak 's/^    if \[ "\$ls_rc" -ne 0 \]; then$/    if false; then  # MUTATION: ls 守衛被拿掉/' "$MUTC" && rm -f "$MUTC.bak"
if cmp -s "$CATALOG_SH" "$MUTC"; then
  fail "C4 突變沒有生效（sed 錨點找不到 ⇒ 這條證明是空的）"
else
  LOGM="$TMP/C4.log"
  run_with_path "$LS_SHIM" "$CATDIR_MUT" scripts/ci/check_data_catalog.sh >"$LOGM" 2>&1 || true
  if grep -qF "所有必要目錄都有 _metadata.json" "$LOGM"; then
    ok "C4 突變 ⇒ 目錄又被當成「空目錄」跳過 ⇒ C1 的斷言有鑑別力"
  else
    fail "C4 突變後仍未出現假綠 ⇒ C1 的斷言可能有別的原因在擋（要釐清）"
    negproof_show_log "$LOGM"
  fi
fi

# ── B4 mutation 自證：把 sessions 掃描的守衛拿掉 ⇒ 又回到靜默跳過（rc=0 ＋ 3/3 通過）──
SCHEMADIR_MUT="$(new_schema_fixture schema-mutated sessions)"
MUTS="$SCHEMADIR_MUT/scripts/ci/validate_json_schemas.sh"
sed -i.bak 's/^    if \[ "\$session_rc" -ne 0 \]; then$/    if false; then  # MUTATION: sessions 守衛被拿掉/' "$MUTS" && rm -f "$MUTS.bak"
if cmp -s "$SCHEMA_SH" "$MUTS"; then
  fail "B4 突變沒有生效（sed 錨點找不到 ⇒ 這條證明是空的）"
else
  LOGM="$TMP/B4.log"
  run_with_path "$SESS_SHIM" "$SCHEMADIR_MUT" scripts/ci/validate_json_schemas.sh >"$LOGM" 2>&1 || true
  if grep -qF "3/3 通過" "$LOGM" && ! grep -qF "sessions/ 的 recommendation_outcomes.jsonl 失敗" "$LOGM"; then
    ok "B4 突變 ⇒ sessions 掃描失敗又被靜默跳過 ⇒ B2 的斷言有鑑別力"
  else
    fail "B4 突變後仍未回到「靜默跳過」⇒ B2 的斷言可能有別的原因在擋（要釐清）"
    negproof_show_log "$LOGM"
  fi
fi

# ═══════════════════════════ D: 靜態返祖守衛 ═══════════════════════════
echo "→ D 靜態：禁「find … 2>/dev/null | … | wc -l」計數形狀（三支腳本）"
for f in "$NAMING_SH" "$SCHEMA_SH" "$CATALOG_SH"; do
  # 只看可執行行（整行註解不算；註解裡引用舊形狀是刻意的）
  if grep -v '^[[:space:]]*#' "$f" | grep -nE 'find .*2>/dev/null.*\|.*wc -l' >/dev/null 2>&1; then
    fail "D $(basename "$f") 仍存在「find … 2>/dev/null … | wc -l」形狀（工具失敗會退化成 0 筆）"
    grep -v '^[[:space:]]*#' "$f" | grep -nE 'find .*2>/dev/null.*\|.*wc -l' | sed 's/^/       /'
  else
    ok "D $(basename "$f") 無該形狀"
  fi
done

echo ""
echo "結果： ${PASS} passed, ${FAIL} failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
