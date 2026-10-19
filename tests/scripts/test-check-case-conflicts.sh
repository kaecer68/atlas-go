#!/usr/bin/env bash
# tests/scripts/test-check-case-conflicts.sh — check_case_conflicts.sh 的契約測試（FU-20261016-23）
#
# 為什麼要有它
# ------------
# `--strict` 這一支**唯一**多做的一件事（非 snake_case 檔名檢查）在修前只印黃字警告、
# 沒有任何 `exit 1`（全檔唯一的 `exit 1` 在 case-conflict 分支，判的是另一件事）：
#   ⇒ `--strict` 與不帶旗標的差別**只有輸出文字**，閘門語意為零。
#   ⇒ 兩個呼叫端 `quality.yml`（job `naming`）與 `scripts/ci/local-ci.sh:71` 都帶 `--strict`
#     ⇒ 這條閘門在 CI 上永遠綠。
# 這種「修好之後無人察覺」的形狀必須用 hermetic fixture 釘住，而不是靠註解。
#
# 釘住的契約（每條在下面都有對應 case）
#   ① Allowlist 校準：慣例檔（AGENTS.md／LICENSE／VERSION／Dockerfile.*…）齊備的 fixture
#      ＋ `--strict` ⇒ rc=0（擋「一律紅」的過度修正）
#   ② 有非白名單檔 ＋ `--strict` ⇒ **rc=1**，且紅燈必須**指名檔案**與**條數**
#   ③ 同一個 fixture **不帶 `--strict`** ⇒ rc=0 且不得出現任何違規輸出
#      （＝預設模式維持 WARNING-only；CI 的 ci-quick 就是走這條）
#   ④ case-conflict 對（同目錄只差大小寫）⇒ **帶與不帶 `--strict` 都 rc=1**、且訊息含
#      `CASE CONFLICT DETECTED` ⇒ 把「衝突路徑本來就有牙齒」釘成契約（F-item 敘述的更正處）
#   ⑤ 突變自證：把 `--strict` 區塊的 `exit 1` 拿掉（＝修前的 rc 行為）⇒ ② 的斷言必須紅
#   ⑥ 違規數是算出來的：2 個違規檔 ⇒ 紅燈必須寫 `2 file(s)`（不是寫死的字串）
#   ⑦ 靜態返祖守衛：`if $STRICT; then` 區塊內必須有 `exit 1`
#   ⑧ 接線契約：`quality.yml` 仍以 `--strict` 呼叫；`Makefile` 的 ci-quick 仍**不帶** `--strict`
#   ⑨ 未知旗標（含拼錯的 `--stric`）⇒ **rc=2**，不得靜默退回預設的 WARNING-only 模式
#   ⑩ 本測試不得動到呼叫者 repo（HEAD／index／受測腳本 sha256 皆不變）
#
# 執行： bash tests/scripts/test-check-case-conflicts.sh
set -uo pipefail

# fixture 自建臨時 git repo；被 git hook 呼叫時環境裡的 GIT_* 會讓 `git -C` 指錯 repo
# ⇒ 一律 unset（同 test-check-markdown-links.sh 的 2026-09-29 事故）。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_SH="$REPO/scripts/ci/check_case_conflicts.sh"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

CALLER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
CALLER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"
CALLER_SHA="$(shasum -a 256 "$SRC_SH" 2>/dev/null | awk '{print $1}')"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/caseconflict-contract.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); echo "  ✅ $*"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

negproof_require_file "$SRC_SH" "受測腳本" || exit 1

# ── helpers ──────────────────────────────────────────────────────────────────
# 本腳本以 `git ls-files`（cwd 相對）蒐集檔案 ⇒ fixture **必須是真的 git repo**，
# 且受測腳本要複製進去（不然 `git ls-files` 掃的是呼叫者 index）。
new_repo() {
  local dir="$TMP/$1"
  mkdir -p "$dir/scripts/ci"
  git -C "$dir" init -q
  git -C "$dir" config user.email fixture@example.test
  git -C "$dir" config user.name fixture
  cp "$SRC_SH" "$dir/scripts/ci/check_case_conflicts.sh"
  echo "$dir"
}

# 只在 **index** 加一個「同檔名、不同大小寫」的檔案：APFS 是大小寫不敏感，
# 工作樹放不下兩個同檔名檔 ⇒ 用 update-index 直接寫 index entry（hermetic、跨平台一致）。
add_case_variant() {
  local dir="$1" src="$2" dst="$3"
  local blob
  blob="$(git -C "$dir" ls-files -s -- "$src" | awk '{print $2}')"
  git -C "$dir" update-index --add --cacheinfo "100644,$blob,$dst"
}

# 慣例檔：與腳本 ALLOWLIST 一對一（工具強制的命名，不是個別豁免）
CONVENTION_FILES="README.md LICENSE NOTICE CHANGELOG.md CONTRIBUTING.md SECURITY.md \
AGENTS.md CLAUDE.md Makefile VERSION Dockerfile Dockerfile.cron Dockerfile.atlas.local \
Dockerfile.cron.local"

run_script() { local dir="$1" script="$2"; shift 2; ( cd "$dir" && bash "$script" "$@" ); }
run_fixture() { local dir="$1"; shift; run_script "$dir" scripts/ci/check_case_conflicts.sh "$@"; }

# 這支腳本的「假綠標記」：出現它就代表檢查說「全部合規」。
GREEN_MARKER="All files follow snake_case convention"
no_false_pass() { ! grep -qF "$GREEN_MARKER" "$1"; }

# 建一棵含全部慣例檔的 fixture（＋可選的額外檔）
new_convention_repo() {
  local name="$1"; shift
  local dir; dir="$(new_repo "$name")"
  local f
  for f in $CONVENTION_FILES; do printf 'x\n' > "$dir/$f"; done
  local extra
  for extra in "$@"; do printf 'x\n' > "$dir/$extra"; done
  git -C "$dir" add -A >/dev/null 2>&1
  echo "$dir"
}

echo "→ check_case_conflicts 契約測試（hermetic fixtures；FU-20261016-23）"

# ── ① Allowlist 校準：慣例檔齊備 ⇒ `--strict` 必須回綠 ──────────────────────────
DIR_A="$(new_convention_repo convention)"
LOG="$TMP/01-convention-strict.log"
if negproof_expect 0 "① 慣例檔齊備 ＋ --strict ⇒ rc=0（不是一律紅）" "$LOG" -- run_fixture "$DIR_A" --strict; then
  ok "① allowlist 校準正確"
else
  fail "① 乾淨 fixture 被 --strict 判紅（＝把慣例檔當違規，過度修正）"; negproof_show_log "$LOG"
fi
negproof_expect_output "$GREEN_MARKER" "$LOG" "① 回綠必須是「掃過且合規」的那一版在說的" \
  || fail "① 綠燈缺少合規宣告"

# ── ② 非白名單檔 ＋ `--strict` ⇒ rc=1、指名檔案與條數 ──────────────────────────
DIR_B="$(new_convention_repo violation FooBar.md)"
LOG="$TMP/02-violation-strict.log"
if negproof_expect 1 "② 有非白名單檔 ＋ --strict ⇒ 必須紅（exit 1）" "$LOG" -- run_fixture "$DIR_B" --strict; then
  ok "② --strict 有牙齒（修前的假綠已堵住）"
else
  fail "② --strict 沒有紅 ⇒ 本票缺陷復活"; negproof_show_log "$LOG"
fi
negproof_expect_output "FooBar.md" "$LOG" "② 紅燈必須指名是哪個檔（否則無從修起）" \
  || fail "② 紅燈沒有指名檔案"
negproof_expect_output "1 file(s) above are not in the ALLOWLIST" "$LOG" \
  "② 紅燈必須報出條數與規則來源" || fail "② 紅燈沒有報出條數"
if no_false_pass "$LOG"; then ok "② 有違規時不再印「全部合規」（假綠擋住）"; else fail "② 有違規仍印合規宣告"; fi

# ── ③ 同 fixture 不帶 `--strict` ⇒ 預設行為不變（rc=0、無違規輸出）────────────
LOG="$TMP/03-violation-default.log"
if negproof_expect 0 "③ 同 fixture（違規）不帶 --strict ⇒ rc=0（預設仍為 WARNING-only）" "$LOG" \
   -- run_fixture "$DIR_B"; then
  ok "③ 預設模式沒有被改成硬紅"
else
  fail "③ 預設模式變成硬紅（＝改動了既有預設行為）"; negproof_show_log "$LOG"
fi
if grep -qF -- "--strict:" "$LOG"; then
  fail "③ 預設模式印出了 --strict 的紅燈訊息"
else
  ok "③ 預設模式沒有 --strict 紅燈訊息"
fi
if grep -qF "Non-snake_case files found" "$LOG"; then
  fail "③ 預設模式印出了非 snake_case 清單（--strict 專屬輸出漏出來了）"
else
  ok "③ 預設模式不做非 snake_case 檢查"
fi

# ── ④ case-conflict 對 ⇒ 帶與不帶 `--strict` 都 rc=1（衝突路徑本來就有牙齒）─────
DIR_C="$(new_convention_repo conflict-pair Folder.md)"
add_case_variant "$DIR_C" Folder.md folder.md || fail "④ 無法把大小寫變體寫進 index"
if [ "$(git -C "$DIR_C" ls-files | grep -ci '^folder\.md$')" -eq 2 ]; then
  ok "④ fixture 真的含一對只差大小寫的 index entry"
else
  fail "④ fixture 沒有造出大小寫對（$(git -C "$DIR_C" ls-files | tr '\n' ' ')）⇒ 後續斷言是空的"
fi
LOG="$TMP/04-conflict-strict.log"
if negproof_expect 1 "④ case-conflict ＋ --strict ⇒ rc=1" "$LOG" -- run_fixture "$DIR_C" --strict; then
  ok "④ --strict 對衝突紅燈"
else
  fail "④ --strict 對衝突沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "CASE CONFLICT DETECTED" "$LOG" "④ 紅燈必須是衝突那條路徑（不是檔名風格）" \
  || fail "④ 紅燈不是 CASE CONFLICT"
LOG="$TMP/04-conflict-default.log"
if negproof_expect 1 "④ case-conflict 不帶 --strict ⇒ 也 rc=1" "$LOG" -- run_fixture "$DIR_C"; then
  ok "④ 衝突路徑與 --strict 無關（本票更正 F-item 敘述的依據）"
else
  fail "④ 不帶 --strict 對衝突沒有紅"; negproof_show_log "$LOG"
fi

# ── ⑤ 突變自證：拿掉 `--strict` 區塊的 `exit 1` ⇒ ② 的斷言必須紅 ────────────────
DIR_M="$TMP/mutated"
cp -R "$DIR_B" "$DIR_M"
grep -v '^        exit 1$' "$DIR_B/scripts/ci/check_case_conflicts.sh" \
  > "$DIR_M/scripts/ci/check_case_conflicts.sh"
if cmp -s "$DIR_B/scripts/ci/check_case_conflicts.sh" "$DIR_M/scripts/ci/check_case_conflicts.sh"; then
  fail "⑤ 突變沒有生效（錨點 '^        exit 1$' 找不到 ⇒ 這條證明是空的）"
else
  LOGM="$TMP/05-mutation.log"
  # This step proves the OPPOSITE direction from the steps above, so it must NOT be
  # written with `negproof_expect 1` (that helper asserts "the command was blocked").
  # What we want here is: with `exit 1` removed, the guard must NOT block any more
  # (rc=0 = the false-green this ticket closed), and THAT is what makes ②'s
  # `--strict ⇒ rc=1` assertion meaningful. Before 2026-10-19 this step called
  # `negproof_expect 1`, so the expected outcome (rc=0) was reported as an
  # `::error::` annotation on every run while the shell still printed ✅ and exited 0
  # — a self-proof that reddened CI and asserted nothing.
  rc_mut=0
  run_fixture "$DIR_M" --strict >"$LOGM" 2>&1 || rc_mut=$?
  if [ "$rc_mut" -eq 0 ]; then
    ok "⑤ 突變（拿掉 exit 1）⇒ 守門不再擋（rc=0 ＝ 修前假綠原文重現）⇒ ② 的斷言確有鑑別力"
  else
    fail "⑤ 突變後仍 rc=${rc_mut}（≠0）⇒ ② 沒有鑑別力：守門還有別處在擋，拿掉 exit 1 動不了它"
    negproof_show_log "$LOGM"
  fi
fi

# ── ⑥ 條數是算出來的：2 個違規檔 ⇒ 紅燈寫 2 ─────────────────────────────────
DIR_D="$(new_convention_repo two-violations FooBar.md ZedQuux.md)"
LOG="$TMP/06-two-violations.log"
if negproof_expect 1 "⑥ 2 個違規檔 ＋ --strict ⇒ rc=1" "$LOG" -- run_fixture "$DIR_D" --strict; then
  ok "⑥ 多違規仍紅"
else
  fail "⑥ 多違規沒有紅"; negproof_show_log "$LOG"
fi
negproof_expect_output "2 file(s) above are not in the ALLOWLIST" "$LOG" \
  "⑥ 條數必須是量出來的（2 個）" || fail "⑥ 條數不正確（寫死？）"
negproof_expect_output "FooBar.md" "$LOG" "⑥ 必須指名第 1 個違規檔" || fail "⑥ 沒有指名 FooBar.md"
negproof_expect_output "ZedQuux.md" "$LOG" "⑥ 必須指名第 2 個違規檔" || fail "⑥ 沒有指名 ZedQuux.md"

# ── ⑦ 靜態返祖守衛：`if $STRICT; then` 區塊內必須有 `exit 1` ─────────────────────
STRICT_BLOCK="$TMP/strict-block.txt"
awk '/^if [$]STRICT; then$/,0' "$SRC_SH" > "$STRICT_BLOCK"
if [ ! -s "$STRICT_BLOCK" ]; then
  fail "⑦ 找不到 'if \$STRICT; then' 區塊（實作被換掉？）"
elif grep -qE '^[[:space:]]*exit 1$' "$STRICT_BLOCK"; then
  ok "⑦ --strict 區塊內有 exit 1"
else
  fail "⑦ --strict 區塊內沒有 exit 1（本票缺陷復活）"
fi

PARSE_BLOCK="$TMP/parse-block.txt"
awk '/^case "[$][{]1:-[}]" in$/,/^esac$/' "$SRC_SH" > "$PARSE_BLOCK"
if [ ! -s "$PARSE_BLOCK" ]; then
  fail "⑦ 找不到旗標解析的 case 區塊（實作被換掉？）"
elif grep -qE '^[[:space:]]*exit 2$' "$PARSE_BLOCK"; then
  ok "⑦ 旗標解析區塊內有 exit 2（未知旗標 fail loud，不是靜默降級）"
else
  fail "⑦ 旗標解析區塊內沒有 exit 2（拼錯的旗標會靜默退回 WARNING-only）"
fi

# ── ⑧ 接線契約：CI 帶 --strict；ci-quick 不帶 ──────────────────────────────────
if grep -qF 'bash scripts/ci/check_case_conflicts.sh --strict' "$REPO/.github/workflows/quality.yml"; then
  ok "⑧ quality.yml（job naming）仍以 --strict 呼叫（修法語意與呼叫端一致）"
else
  fail "⑧ quality.yml 不再以 --strict 呼叫（本票的修法對象消失）"
fi
# 只看**呼叫行**：`@echo`／`echo` 這類文件行也會提到腳本名（本票自己就加了一行），
# 把它們算進來會得到「突變打在註解行」的假陽性 —— 2026-10-16 本測試第一次跑就紅在這裡。
MAKE_CALLS="$(awk '/check_case_conflicts\.sh/ { l=$0; sub(/^[[:space:]]*@?/, "", l); if (l !~ /^echo[[:space:]]/) print NR": "$0 }' "$REPO/Makefile")"
if [ -z "$MAKE_CALLS" ]; then
  fail "⑧ Makefile 找不到任何 check_case_conflicts.sh 的呼叫行（探針是空的 ⇒ 這條證明沒有鑑別力）"
elif printf '%s\n' "$MAKE_CALLS" | grep -q -- '--strict'; then
  fail "⑧ Makefile 的呼叫行改成帶 --strict ⇒ 預設閘門語意被改（不在本票範圍）"
else
  ok "⑧ Makefile（ci-quick）仍以預設模式呼叫（WARNING-only 不變）"
fi
if grep -qF 'bash scripts/ci/check_case_conflicts.sh --strict' "$REPO/scripts/ci/local-ci.sh"; then
  ok "⑧ local-ci.sh 仍以 --strict 呼叫（本地閘門與 CI 同語意）"
else
  fail "⑧ local-ci.sh 的 --strict 呼叫不見了"
fi

# ── ⑨ 未知旗標 ⇒ rc=2（旗標拼錯不得靜默退回預設 WARNING-only）──────────────────
LOG="$TMP/09-unknown-flag.log"
if negproof_expect 2 "⑨ --bogus-flag ⇒ exit 2（用法錯誤，不是靜默忽略）" "$LOG" \
   -- run_fixture "$DIR_A" --bogus-flag; then
  ok "⑨ 未知旗標被拒"
else
  fail "⑨ 未知旗標被靜默忽略"; negproof_show_log "$LOG"
fi
negproof_expect_output "unknown option: --bogus-flag" "$LOG" "⑨ 必須指名是哪個旗標" || fail "⑨ 沒有指名旗標"
negproof_expect_output "usage: check_case_conflicts.sh [--strict]" "$LOG" "⑨ 必須印出正確用法" || fail "⑨ 沒有用法訊息"

# 拼錯的 --strict 是**本票缺陷的孿生形狀**：舊版會靜默降級成 WARNING-only，
# 而這棵 fixture（DIR_B）**真的有違規** ⇒ 假綠。用「真的有違規」的 fixture 測才不是空的。
LOG="$TMP/09b-typo.log"
if negproof_expect 2 "⑨ --stric（打錯字）⇒ exit 2，不得降級成假綠" "$LOG" -- run_fixture "$DIR_B" --stric; then
  ok "⑨ 拼錯的旗標不會讓閘門無聲失去牙齒"
else
  fail "⑨ 拼錯的 --strict 被靜默忽略 ⇒ 與本票同一種失效"; negproof_show_log "$LOG"
fi
if grep -qF "$GREEN_MARKER" "$LOG"; then
  fail "⑨ 未知旗標下竟印出合規宣告"
else
  ok "⑨ 未知旗標下不會有任何掃描結論（沒有假綠）"
fi

# ── ⑩ 本測試不得動到呼叫者 repo ───────────────────────────────────────────────
if [ "$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)" = "$CALLER_HEAD" ] \
   && [ "$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')" = "$CALLER_TRACKED" ]; then
  ok "⑩ 本測試沒有動到呼叫者 repo（HEAD 與 index 不變）"
else
  fail "⑩ 本測試動到了呼叫者的 repo"
fi
if [ "$(shasum -a 256 "$SRC_SH" 2>/dev/null | awk '{print $1}')" = "$CALLER_SHA" ]; then
  ok "⑩ 受測腳本沒有被本測試改寫（sha256 不變）"
else
  fail "⑩ 受測腳本被本測試改寫了"
fi
for leftover in Folder.md folder.md FooBar.md ZedQuux.md; do
  if [ -e "$REPO/$leftover" ]; then
    fail "⑩ fixture 檔漏進了真 repo：$leftover"
  fi
done
ok "⑩ fixture 檔沒有漏進真 repo"

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ test-check-case-conflicts: $PASS passed, $FAIL failed"
  exit 1
fi
echo "✅ test-check-case-conflicts: $PASS passed（--strict 有牙齒／指名檔案與條數／預設仍 WARNING／衝突路徑不受 --strict 影響／未知旗標 rc=2／突變自證／條數可量／靜態返祖／接線契約／呼叫者不變）"
