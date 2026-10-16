#!/usr/bin/env bash
# tests/scripts/test-check-data-catalog.sh — check_data_catalog.sh 的契約測試（FU-20261016-22）
#
# 為什麼要有它
# ------------
# 舊版 `check_data_catalog.sh` 的「stale entry」檢查在 macOS 上**永遠不可能報告 stale**：
#   ① `grep -oP` 是 GNU 專屬：BSD grep 回 `grep: invalid option -- P`、rc=2、0 行輸出；
#   ② 同一行的 `2>/dev/null` 把 stderr 吞掉；`grep … | sed … | sort -u | while …` 的結束碼是
#      **最後一段（while）**的 rc（=0），而呼叫端的 `check_stale_entries || r3=$?` 又讓 errexit 失效；
#   ③ `| while` 讓迴圈跑在**管線子殼**內 ⇒ 就算找到 stale，`found_any=1` 也回不到父殼。
#   ⇒ 三者相乘 = 掃 0 行仍印 `[PASS] catalog 中無 stale entries` ＋ `5/5 通過` ＋ rc=0。
#   實測（main clone，2026-10-16）：同一 pattern 換 `-E` 得 45 條、其中 5 條 stale，舊版仍印 PASS。
#
# 釘住的契約（每條都在下面有對應 case）
#   ① fixture 有 stale ⇒ **不得**再印 `[PASS] catalog 中無 stale entries`，且必須指名那條路徑與掃描計數
#   ② 同一棵 fixture 加 `--strict` ⇒ exit 1（紅燈），且紅燈必須指名路徑
#   ③ 同一棵 fixture 把該檔補上 ⇒ exit 0 且印回 PASS（證明 ① 有鑑別力，不是恆紅）
#   ④ `--strict` 對乾淨的 catalog 仍 exit 0（證明不是「一律紅」的過度修正）
#   ⑤ 把 `found_any` 在判定處強制歸零（模擬子殼回不來）⇒ ① 的斷言必須紅（測試自己有牙齒）
#   ⑥ 抽取工具壞掉（PATH shim：`grep -oE` 回 rc=2）⇒ exit 1 且講明 `grep rc=2`（工具錯誤不得被吞）
#   ⑦ 同上但 `--warn-only` ⇒ 仍然 exit 1（warn-only 只能壓抑資料發現，壓不掉「檢查跑不成」）
#   ⑧ catalog 內 0 條 data/ 路徑 ⇒ exit 1（堵「0 行 ⇒ 判通過」這條路）
#   ⑨ data/state/ 讀不到（`find` rc≠0）⇒ exit 1（工具／環境錯誤不得被吞）
#   ⑩ 靜態返祖守衛：可執行行內不得再出現 GNU 專屬的 `-P`
#   ⑪ 把 `-oE` 突變回 `-oP`：BSD grep ⇒ fail loud（rc=1、`grep rc=2`）；GNU grep ⇒ 仍不得印 PASS
#   ⑫ 本測試不得動到呼叫者 repo（fixture 全部在 mktemp 內，腳本本身不碰 git）
#
# 執行： bash tests/scripts/test-check-data-catalog.sh
set -uo pipefail

# fixture 不建 git repo，但呼叫端 repo 的基線要用 git 讀 ⇒ 一律 unset，避免被 hook 環境帶歪。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_SH="$REPO/scripts/ci/check_data_catalog.sh"
REAL_GREP="$(command -v grep)"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

CALLER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
CALLER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/datacatalog-contract.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

STALE_PATH="data/state/ghost-entry.json"
PASS_MARKER="catalog 中無 stale entries"

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); echo "  ✅ $*"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

negproof_require_file "$SRC_SH" "受測腳本" || exit 1

# ── helpers ──────────────────────────────────────────────────────────────────
# new_fixture <名稱> [stale 路徑] — 建 hermetic fixture（腳本副本 ＋ docs/data-catalog.md ＋ 少量 data/）；
# 腳本用 BASH_SOURCE 推導 REPO_ROOT ⇒ 自然指向 fixture，**不碰真 repo**。
new_fixture() {
  local name="$1" stale="${2:-}"
  local dir="$TMP/$name"
  mkdir -p "$dir/scripts/ci" "$dir/docs" "$dir/data/state/kb" "$dir/data/replay"
  cp "$SRC_SH" "$dir/scripts/ci/check_data_catalog.sh"
  printf 'seed\n' > "$dir/data/fundamentals.json"
  printf '{"seeded":true}\n' > "$dir/data/state/kb/_metadata.json"
  printf 'v\n' > "$dir/data/replay/VERSION"
  {
    printf '# fixture catalog（hermetic；不是真 catalog）\n\n'
    printf '| 類型 | 路徑 |\n|---|---|\n'
    printf '| JSON | `data/fundamentals.json` |\n'
    printf '| Directory | `data/state/kb/` |\n'
    printf '| JSON | `data/replay/VERSION` |\n'
    if [ -n "$stale" ]; then printf '| JSON | `%s` |\n' "$stale"; fi
    printf '| 說明 | `data/state/not a real path` |\n'
    printf '| 模板 | `data/state/{name}/` |\n'
  } > "$dir/docs/data-catalog.md"
  echo "$dir"
}

run_check()      { ( cd "$1" && bash scripts/ci/check_data_catalog.sh ); }
run_check_args() { local dir="$1"; shift; ( cd "$dir" && bash scripts/ci/check_data_catalog.sh "$@" ); }
run_check_path() { local dir="$1" p="$2"; shift 2; ( cd "$dir" && PATH="$p:$PATH" bash scripts/ci/check_data_catalog.sh "$@" ); }

# 假綠偵測器：輸出內出現「catalog 中無 stale entries」＝這條路徑又開始說謊。
no_false_pass() { ! grep -qF "$PASS_MARKER" "$1"; }

# 去掉**整行註解**後的可執行行（靜態契約用）
code_lines() { grep -v '^[[:space:]]*#' "$1"; }

# 模擬「`| while` 子殼回不來」：在判定處把 found_any 強制歸零。
mutate_force_found_any() {
  awk '{ if ($0 ~ /^  if \[ "\$scanned" -eq 0 \]; then$/) { print "  found_any=0  # MUTATION：模擬子殼回傳不回來" } print }' "$1" > "$2"
}

# shim：讓「抽取路徑」的工具壞掉（只針對 -oE 呼叫回 rc=2，其餘轉真 grep）。
make_grep_shim() {
  local dir="$1"
  mkdir -p "$dir"
  printf '#!/usr/bin/env bash\nfor a in "$@"; do\n  case "$a" in\n    *-oE*) echo "grep: simulated tool failure" >&2; exit 2 ;;\n  esac\ndone\nexec %s "$@"\n' "$REAL_GREP" > "$dir/grep"
  chmod +x "$dir/grep"
  echo "$dir"
}

echo "→ check_data_catalog 契約測試（hermetic fixtures；FU-20261016-22）"

# ── ① fixture 有 1 條 stale ⇒ 不得印 PASS，必須指名該路徑與掃描計數 ──────────────
DIR="$(new_fixture stale "$STALE_PATH")"
LOG="$TMP/01-stale.log"
if negproof_expect 0 "fixture 有 stale（預設模式＝資料發現僅警告）⇒ exit 0" "$LOG" -- run_check "$DIR"; then
  ok "① 預設模式仍回 0（政策未放寬成硬紅）"
else
  fail "① 預設模式的結束碼不是 0"; negproof_show_log "$LOG"
fi
if no_false_pass "$LOG"; then ok "① 不再印「${PASS_MARKER}」（假綠擋住）"; else fail "① 仍印「${PASS_MARKER}」＝假綠復活"; negproof_show_log "$LOG"; fi
negproof_expect_output "[WARN] $STALE_PATH — catalog 中記錄但檔案不存在 (stale entry)" "$LOG" \
  "① 必須指名是哪一條 stale（否則與「沒掃到」無法區分）" || fail "① 沒有指名 stale 路徑"
negproof_expect_output "發現 1 條 stale entry（已掃描 4 條" "$LOG" "① 必須報出計數（掃了幾條／命中幾條）" \
  || fail "① 沒有報出計數"

# ── ② 同一棵 fixture 加 --strict ⇒ exit 1（紅燈）──────────────────────────────
LOG="$TMP/02-strict.log"
if negproof_expect 1 "--strict ⇒ 發現 stale 必須紅（exit 1）" "$LOG" -- run_check_args "$DIR" --strict; then
  ok "② --strict 紅燈"
else
  fail "② --strict 沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "[FAIL] 發現 1 條 stale entry（已掃描 4 條）— --strict 模式視為違規" "$LOG" \
  "② 紅燈必須指名規則與條數" || fail "② 紅燈訊息不含預期的原文"
negproof_expect_output "$STALE_PATH" "$LOG" "② 紅燈必須指名 stale 路徑" || fail "② 紅燈沒指名路徑"
if no_false_pass "$LOG"; then ok "② --strict 下也沒有 PASS 假綠"; else fail "② --strict 下仍印 PASS"; fi

# ── ③ 把該檔補上 ⇒ exit 0 且 PASS 回來（證明 ① 有鑑別力）──────────────────────
DIR2="$(new_fixture clean)"
LOG="$TMP/03-clean.log"
if negproof_expect 0 "把 stale 補上（同一棵 fixture 形狀）⇒ exit 0" "$LOG" -- run_check "$DIR2"; then
  ok "③ 乾淨 fixture 回綠"
else
  fail "③ 乾淨 fixture 仍紅（① 沒有鑑別力）"; negproof_show_log "$LOG"
fi
negproof_expect_output "[PASS] catalog 中無 stale entries（已掃描 3 條 data/ 路徑）" "$LOG" \
  "③ 綠燈必須是「真的掃過」的那一版在說的" || fail "③ 綠燈缺掃描計數"

# ── ④ --strict 對乾淨 catalog 仍 exit 0（擋「一律紅」的過度修正）────────────────
LOG="$TMP/04-clean-strict.log"
if negproof_expect 0 "--strict ＋ 乾淨 catalog ⇒ 仍 exit 0" "$LOG" -- run_check_args "$DIR2" --strict; then
  ok "④ --strict 不是一律紅"
else
  fail "④ --strict 對乾淨 catalog 也紅（過度修正）"; negproof_show_log "$LOG"
fi

# ── ⑤ 突變：把 found_any 在判定處強制歸零 ⇒ ① 的斷言必須紅 ─────────────────────
DIRM="$TMP/mutated"
cp -R "$DIR" "$DIRM"
mutate_force_found_any "$SRC_SH" "$DIRM/scripts/ci/check_data_catalog.sh"
if cmp -s "$SRC_SH" "$DIRM/scripts/ci/check_data_catalog.sh"; then
  fail "⑤ 突變沒有生效（mutate_force_found_any 的錨點找不到 ⇒ 這條證明是空的）"
else
  LOGM="$TMP/05-mutation.log"
  run_check "$DIRM" >"$LOGM" 2>&1 || true
  if no_false_pass "$LOGM"; then
    fail "⑤ 突變後仍未印 PASS ⇒ ① 的斷言沒有鑑別力（抓不到子殼回不來這個形狀）"
    negproof_show_log "$LOGM"
  else
    ok "⑤ 突變 ⇒ 假綠回來 ⇒ ① 的斷言確實會紅"
  fi
fi

# ── ⑥ 抽取工具壞掉（rc=2）⇒ exit 1、講明 rc（工具錯誤不得被吞）──────────────────
SHIM="$(make_grep_shim "$TMP/shim-grep")"
DIR3="$(new_fixture tool-error "$STALE_PATH")"
LOG="$TMP/06-tool-error.log"
if negproof_expect 1 "grep 回 rc=2 ⇒ exit 1（不得靜默回綠、也不得靜默回黃）" "$LOG" \
   -- run_check_path "$DIR3" "$SHIM"; then
  ok "⑥ 工具錯誤 fail loud"
else
  fail "⑥ 工具錯誤沒有 fail loud"; negproof_show_log "$LOG"
fi
negproof_expect_output "grep rc=2" "$LOG" "⑥ 必須報出工具的 rc" || fail "⑥ 沒有報出工具 rc"
negproof_expect_output "不得視為通過" "$LOG" "⑥ 必須講明後果（不得視為通過）" || fail "⑥ 訊息沒有講明後果"
if no_false_pass "$LOG"; then ok "⑥ 工具錯誤時也沒有 PASS 假綠"; else fail "⑥ 工具錯誤時仍印 PASS"; fi

# ── ⑦ 工具壞掉 ＋ --warn-only ⇒ 仍然 exit 1（warn-only 壓不掉「跑不成」）─────────
LOG="$TMP/07-tool-error-warnonly.log"
if negproof_expect 1 "grep rc=2 ＋ --warn-only ⇒ 仍 exit 1" "$LOG" \
   -- run_check_path "$DIR3" "$SHIM" --warn-only; then
  ok "⑦ --warn-only 不會掩蓋工具錯誤"
else
  fail "⑦ --warn-only 把工具錯誤吞掉了"; negproof_show_log "$LOG"
fi
negproof_expect_output "不因 --warn-only 而放行" "$LOG" "⑦ 必須講明 warn-only 不適用於工具錯誤" \
  || fail "⑦ 訊息沒有講明 warn-only 的邊界"

# ── ⑧ catalog 內 0 條 data/ 路徑 ⇒ exit 1（堵「0 行 ⇒ 判通過」）──────────────────
DIR4="$(new_fixture zero-paths)"
printf '# fixture catalog\n\n這份 catalog 完全沒有反引號包起來的 data/ 路徑\n' > "$DIR4/docs/data-catalog.md"
LOG="$TMP/08-zero-paths.log"
if negproof_expect 1 "掃到 0 條 data/ 路徑 ⇒ exit 1（不得視為通過）" "$LOG" -- run_check "$DIR4"; then
  ok "⑧ 0 條路徑 fail loud"
else
  fail "⑧ 0 條路徑仍然回綠（＝舊版的假綠路徑）"; negproof_show_log "$LOG"
fi
negproof_expect_output "掃不到任何 data/ 路徑引用" "$LOG" "⑧ 必須講明是掃不到而不是沒有 stale" \
  || fail "⑧ 訊息沒有講明原因"
if no_false_pass "$LOG"; then ok "⑧ 0 條路徑時也沒有 PASS 假綠"; else fail "⑧ 0 條路徑仍印 PASS"; fi

# ── ⑨ data/state/ 讀不到（find rc≠0）⇒ exit 1（工具／環境錯誤不得被吞）────────────────
DIR5="$(new_fixture missing-state)"
rm -rf "$DIR5/data/state"
LOG="$TMP/09-missing-state.log"
if negproof_expect 1 "data/state/ 不存在 ⇒ find rc≠0 ⇒ exit 1" "$LOG" -- run_check "$DIR5"; then
  ok "⑨ find 失敗 fail loud"
else
  fail "⑨ find 失敗仍回綠"; negproof_show_log "$LOG"
fi
negproof_expect_output "列出 data/state/ 子目錄失敗" "$LOG" "⑨ 必須講明是蒐集失敗而不是通過" || fail "⑨ 訊息沒有講明原因"

# ── ⑩ 靜態返祖守衛：可執行行內不得有 GNU 專屬的 -P ────────────────────────────
CODE="$TMP/entry-code.txt"
code_lines "$SRC_SH" > "$CODE"
if grep -qE 'grep +-[a-zA-Z]*P' "$CODE"; then
  fail "⑩ 可執行行內出現 grep -P（macOS 上會 rc=2 ⇒ 又是同一種假綠）"
else
  ok "⑩ 沒有 grep -P"
fi
if grep -qF -- "grep -oE" "$CODE"; then
  ok "⑩ 抽取用 POSIX ERE（-oE）"
else
  fail "⑩ 找不到 -oE 抽取（實作被換掉？）"
fi

# ── ⑪ 把 -oE 突變回 -oP：BSD grep ⇒ fail loud；GNU grep ⇒ 也不得印 PASS ─────────
DIR6="$(new_fixture mutation-op "$STALE_PATH")"
sed 's/grep -oE /grep -oP /' "$SRC_SH" > "$DIR6/scripts/ci/check_data_catalog.sh"
if cmp -s "$SRC_SH" "$DIR6/scripts/ci/check_data_catalog.sh"; then
  fail "⑪ 突變沒有生效（找不到 -oE 錨點 ⇒ 這條證明是空的）"
else
  LOG="$TMP/11-mutation-op.log"
  rc=0; run_check "$DIR6" >"$LOG" 2>&1 || rc=$?
  if printf '%s\n' x | grep -oP x >/dev/null 2>&1; then
    # GNU grep：-P 可用 ⇒ 檢查會跑，但 stale 存在時仍不得印 PASS
    if no_false_pass "$LOG"; then ok "⑪（GNU）突變後仍不得印 PASS"; else fail "⑪（GNU）突變後印了 PASS"; negproof_show_log "$LOG"; fi
  else
    # BSD grep：-P 不被支援 ⇒ 必須 fail loud（rc=1、報 grep rc=2），而不是靜默回綠
    if [ "$rc" -eq 1 ] && grep -qF 'grep rc=2' "$LOG"; then
      ok "⑪（BSD）-P 不被支援 ⇒ fail loud（rc=1、報了 grep rc=2）"
    else
      fail "⑪（BSD）突變後 rc=$rc 且沒有報 grep rc=2 ⇒ 工具錯誤被吞（＝本票的缺陷形狀）"
      negproof_show_log "$LOG"
    fi
  fi
fi

# ── ⑫ 本測試不得動到呼叫者 repo ───────────────────────────────────────────────
if [ "$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)" = "$CALLER_HEAD" ] \
   && [ "$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')" = "$CALLER_TRACKED" ]; then
  ok "⑫ 本測試沒有動到呼叫者 repo（HEAD 與 index 不變）"
else
  fail "⑫ 本測試動到了呼叫者的 repo"
fi
if [ -e "$REPO/$STALE_PATH" ]; then
  fail "⑫ fixture 的路徑漏進了真 repo：$STALE_PATH"
else
  ok "⑫ fixture 路徑沒有漏進真 repo"
fi

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ test-check-data-catalog: $PASS passed, $FAIL failed"
  exit 1
fi
echo "✅ test-check-data-catalog: $PASS passed（stale 必指名＋不假綠／--strict 紅燈／乾淨回綠／突變自證／工具 rc=2 fail loud／warn-only 不掩蓋／0 條路徑／data-state 讀不到／靜態返祖／-oP 突變／呼叫者不變）"
