#!/usr/bin/env bash
# tests/scripts/test-dead-flag-contract.sh — 「死旗標」家族的契約測試（FU-20261016-26）
#
# 為什麼要有它
# ------------
# 「死旗標／無呼叫端」有三種判準，只看第一種（變數取了卻未引用）會得到 **0 筆**：
#   (a) 變數取了卻未引用；
#   (b) 薄殼未把 "$@" 轉發給被呼叫者；
#   (c) 旗標拼法與程式實際接受的值不符（例：只認裸字 `json`，不認文件與 CI 寫的 `--json`）。
# 本檔把 (b)(c) 兩型的修好狀態釘成可執行契約，並用突變自證斷言有鑑別力：
#   * 修前五支 check 腳本是 `OUTPUT_MODE="${1:-text}"` ⇒ `--json` 與「不帶旗標」
#     **逐位元組相同**（本檔 ①–⑤ 在修前的副本上必須變紅）。
#   * `check_jev_contract.sh` 修前沒有 `"$@"` ⇒ `--with-selftest` 被靜默丟棄（⑦）。
#   * `zero-metric-guard.sh --fail` 修前全 repo 零呼叫端 ⇒ 找到違規仍 rc=0（⑧）。
#
# 釘住的契約
#   ①–⑤ `check_constitution.sh`／`check_data_catalog.sh`／`check_data_naming.sh`／
#         `check_maturity.sh`／`check_agent_prompts.sh`：`--json` 與裸字 `json` **逐位元組相同**，
#         且兩者都必須與「不帶旗標」**不同**（＝旗標真的有作用）。
#   ⑥ 未知旗標 ⇒ rc=2 ＋ `unknown option` ＋ `usage:`（不得靜默退回預設模式）。
#   ⑦ `check_jev_contract.sh` 必須把 `"$@"` 轉發給被呼叫者（以 stub checker 觀察 argv；
#      `--with-selftest` 是連外自測，不能在 CI 真跑）。
#   ⑧ `zero-metric-guard.sh`：違規 fixture ＋ `--fail` ⇒ rc=1 且指名檔案；不帶旗標 ⇒ rc=0（報告模式）；
#      乾淨 fixture ＋ `--fail` ⇒ rc=0（不得一律紅）。
#   ⑨ 接線契約：`Makefile` 與 `.github/workflows/quality.yml` 都必須以 `--fail` 呼叫那支守門
#      （沒有呼叫端 ＝ 牙齒只在有人記得帶它時才存在）。
#   ⑩ 突變自證（把 ① 的判準餵給「旗標 arm 被拿掉」的副本 ⇒ 判準必須紅）＋
#      本測試不得改動受測腳本（跑完逐一比對 sha256）。
#
# 執行： bash tests/scripts/test-dead-flag-contract.sh
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

# 受測腳本（真檔；fixture 內是它們的複製，REPO_ROOT 由 BASH_SOURCE 推導 ⇒ 自動落在 fixture）
JSON_SCRIPTS="check_constitution.sh check_data_catalog.sh check_data_naming.sh check_maturity.sh check_agent_prompts.sh"
for s in $JSON_SCRIPTS check_jev_contract.sh zero-metric-guard.sh; do
  negproof_require_file "$REPO/scripts/ci/$s" "受測腳本 $s" || exit 1
done
negproof_require_file "$REPO/Makefile" "Makefile" || exit 1
negproof_require_file "$REPO/.github/workflows/quality.yml" "quality.yml" || exit 1

BEFORE_SHA="$(
  for s in $JSON_SCRIPTS check_jev_contract.sh zero-metric-guard.sh; do
    shasum -a 256 "$REPO/scripts/ci/$s" | awk '{print $1}'
  done | shasum -a 256 | awk '{print $1}'
)"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/fu26-deadflag.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0 FAIL=0
ok()   { PASS=$((PASS + 1)); echo "  ✅ $*"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

# ── fixture ──────────────────────────────────────────────────────────────────
# 這幾支檢查靠 `REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"` 定位 ⇒ 把**真檔**
# 複製進 <tmp>/scripts/ci/ 就會以 <tmp> 當 repo root（零連外、零副作用）。
# 補上最小必要輸入：configs/agents.json（否則 check_agent_prompts.sh 會**提前** exit，
# 根本走不到輸出模式分支 ⇒ 斷言會變成在測別的東西）。
build_fixture() {  # <目的目錄>
  local dir="$1"
  mkdir -p "$dir/scripts/ci" "$dir/configs" \
           "$dir/shared_web/static/js/pages" "$dir/shared_web/static/js/components"
  printf '{"agents":[]}\n' > "$dir/configs/agents.json"
  local s
  for s in $JSON_SCRIPTS check_jev_contract.sh zero-metric-guard.sh; do
    cp "$REPO/scripts/ci/$s" "$dir/scripts/ci/$s"
  done
}

# 一律在 fixture 內跑（受測腳本以自身路徑推導 REPO_ROOT）
run_script() {  # <fixture> <script 名> [args…]
  local dir="$1" s="$2"; shift 2
  ( cd "$dir" && bash "scripts/ci/$s" "$@" )
}

# ── 判準：`--json` 與裸字 `json` 等效，且都與不帶旗標不同 ────────────────────────
# 回傳 0 = 契約成立；1 = 契約不成立（並在 stdout 說明是哪一半不成立）
json_flag_contract() {  # <fixture> <script> <log 前綴>
  local dir="$1" s="$2" p="$3"
  ( cd "$dir" && bash "scripts/ci/$s" )        >"$p-none.log"   2>&1
  ( cd "$dir" && bash "scripts/ci/$s" json )   >"$p-bare.log"   2>&1
  ( cd "$dir" && bash "scripts/ci/$s" --json ) >"$p-dash.log"   2>&1
  # 先驗「死旗標」那一半（修前的症狀就是這一句），再驗「兩種拼法等效」。
  if cmp -s "$p-none.log" "$p-dash.log"; then
    echo "--json 與不帶旗標輸出逐位元組相同（＝旗標沒作用）"
    return 1
  fi
  if ! cmp -s "$p-bare.log" "$p-dash.log"; then
    echo "--json 與裸字 json 輸出不同（兩者應等效）"
    return 1
  fi
  return 0
}

echo "→ 死旗標家族契約測試（hermetic fixtures；FU-20261016-26）"

FIX="$TMP/repo"
build_fixture "$FIX"

# ── ①–⑤ `--json` 真的有作用 ──────────────────────────────────────────────────
n=0
for s in $JSON_SCRIPTS; do
  n=$((n + 1))
  tag="$(printf '%02d' "$n")"
  if msg="$(json_flag_contract "$FIX" "$s" "$TMP/$tag")"; then
    ok "${tag} ${s}：--json ≡ json（逐位元組）且 ≠ 不帶旗標"
  else
    fail "${tag} ${s}：${msg}"
    sed 's/^/       /' "$TMP/$tag-dash.log" | head -5
  fi
done

# ── ⑥ 未知旗標 ⇒ rc=2 ＋ unknown option ＋ usage ─────────────────────────────
for s in $JSON_SCRIPTS zero-metric-guard.sh; do
  LOG="$TMP/unknown-$s.log"
  if negproof_expect 2 "⑥ ${s} --bogus ⇒ rc=2（不得靜默退回預設模式）" "$LOG" \
       -- run_script "$FIX" "$s" --bogus; then
    if negproof_expect_output "unknown option" "$LOG" "⑥ ${s} 必須說明未知旗標" \
       && negproof_expect_output "usage:" "$LOG" "⑥ ${s} 必須印用法"; then
      ok "⑥ ${s}：未知旗標 fail loud（rc=2 ＋ usage）"
    else
      fail "⑥ ${s}：rc=2 但訊息不完整"; negproof_show_log "$LOG"
    fi
  else
    fail "⑥ ${s}：未知旗標被靜默接受"; negproof_show_log "$LOG"
  fi
done

# ── ⑦ jev 薄殼必須轉發 "$@" ─────────────────────────────────────────────────
# 用 stub checker 觀察 argv（真 checker 的 `--with-selftest` 會做一次真 API 呼叫，不可在 CI 跑）。
STUB="$FIX/scripts/jev-contract-check.py"
printf '#!/usr/bin/env python3\nimport sys\nprint("STUB-ARGS: " + " ".join(sys.argv[1:]))\n' > "$STUB"
LOG="$TMP/jev-forward.log"
if ( cd "$FIX" && bash scripts/ci/check_jev_contract.sh --with-selftest ) >"$LOG" 2>&1; then
  if negproof_expect_output "--with-selftest" "$LOG" "⑦ 薄殼必須把旗標轉發給 checker"; then
    ok "⑦ check_jev_contract.sh 有轉發 \"\$@\""
  else
    fail "⑦ 旗標没有到達 checker（薄殼把它吞了）"; negproof_show_log "$LOG"
  fi
else
  fail "⑦ fixture 內跑不動 check_jev_contract.sh"; negproof_show_log "$LOG"
fi
rm -f "$STUB"

# ── ⑧ zero-metric-guard 的牙齒與報告模式 ─────────────────────────────────────
printf 'const a = fmtCurrency(v, { decimals: 0 });\n' > "$FIX/shared_web/static/js/pages/has-violation.js"
LOG="$TMP/zmg-fail.log"
if negproof_expect 1 "⑧ 違規 fixture ＋ --fail ⇒ rc=1" "$LOG" \
     -- run_script "$FIX" zero-metric-guard.sh --fail; then
  if negproof_expect_output "has-violation.js" "$LOG" "⑧ 紅燈必須指名違規檔"; then
    ok "⑧ zero-metric-guard --fail 有牙齒"
  else
    fail "⑧ 紅燈沒有指名檔案"; negproof_show_log "$LOG"
  fi
else
  fail "⑧ zero-metric-guard --fail 沒有紅 ⇒ 這顆牙齒是假的"; negproof_show_log "$LOG"
fi

LOG="$TMP/zmg-default.log"
if negproof_expect 0 "⑧ 同 fixture 不帶旗標 ⇒ rc=0（報告模式維持）" "$LOG" \
     -- run_script "$FIX" zero-metric-guard.sh; then
  ok "⑧ 報告模式（預設 rc=0）沒有被改成硬紅"
else
  fail "⑧ 預設模式變成硬紅（＝改了既有預設行為）"; negproof_show_log "$LOG"
fi

rm -f "$FIX/shared_web/static/js/pages/has-violation.js"
printf 'const b = fmtSafeCurrency(v, { decimals: 0 });\n' > "$FIX/shared_web/static/js/pages/clean.js"
LOG="$TMP/zmg-clean.log"
if negproof_expect 0 "⑧ 乾淨 fixture ＋ --fail ⇒ rc=0（不是一律紅）" "$LOG" \
     -- run_script "$FIX" zero-metric-guard.sh --fail; then
  ok "⑧ 乾淨頁面不會被誤紅"
else
  fail "⑧ 乾淨頁面被誤紅（過度修正）"; negproof_show_log "$LOG"
fi

# ── ⑨ 接線契約：守門必須真的有呼叫端 ─────────────────────────────────────────
for f in Makefile .github/workflows/quality.yml; do
  if grep -qF -- "zero-metric-guard.sh --fail" "$REPO/$f"; then
    ok "⑨ ${f} 以 --fail 呼叫 zero-metric-guard.sh"
  else
    fail "⑨ ${f} 沒有以 --fail 呼叫 zero-metric-guard.sh ⇒ 牙齒只在有人記得帶它時才存在"
  fi
done

# ── ⑩ 突變自證 ＋ 不得改動受測腳本 ────────────────────────────────────────────
# 突變要打在**行為行**上，而且必須還原成「修前的形狀」（不是隨手改壞）：
#   ① `json|--json)` → `json)`（旗標拼法不再被接受）
#   ② `*)` 的 `exit 2` 拿掉（未知旗標回到靜默忽略）
# 兩者合起來正是修前的 `OUTPUT_MODE="${1:-text}"`：`--json` 既不生效、也不報錯
# ⇒ ① 的判準必須以「與不帶旗標逐位元組相同」變紅。
MUT="$TMP/mutant"
build_fixture "$MUT"
python3 - "$MUT/scripts/ci/check_constitution.sh" <<'PYEOF'
import re
import sys

path = sys.argv[1]
src = open(path, encoding="utf-8").read()
src, n1 = re.subn(r'^\s*json\|--json\) OUTPUT_MODE="json" ;;$',
                  '    json) OUTPUT_MODE="json" ;;', src, flags=re.M)
src, n2 = re.subn(r'(?ms)^\s*\*\)\n.*?exit 2\n\s*;;\n', '    *) ;;\n', src, count=1)
if (n1, n2) != (1, 1):
    sys.exit("mutation: 取代次數不如預期（arm=%d, unknown=%d）" % (n1, n2))
open(path, "w", encoding="utf-8").write(src)
PYEOF
if msg="$(json_flag_contract "$MUT" check_constitution.sh "$TMP/mut")"; then
  fail "⑩ 突變沒被偵測到（判準沒鑑別力）: ${msg}"
else
  ok "⑩ 突變自證：還原成修前形狀 ⇒ 判準如預期變紅（${msg}）"
fi

AFTER_SHA="$(
  for s in $JSON_SCRIPTS check_jev_contract.sh zero-metric-guard.sh; do
    shasum -a 256 "$REPO/scripts/ci/$s" | awk '{print $1}'
  done | shasum -a 256 | awk '{print $1}'
)"
if [ "$BEFORE_SHA" = "$AFTER_SHA" ]; then
  ok "⑩ 受測腳本 sha256 不變（測試沒有改到真 repo 的檔）"
else
  fail "⑩ 受測腳本在測試過程中被改動"
fi

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ dead-flag 契約測試失敗：${FAIL} 項（通過 ${PASS} 項）"
  exit 1
fi
echo "✅ dead-flag 契約測試全數通過（${PASS} 項）"
