#!/usr/bin/env bash
# tests/scripts/test-check-doc-links.sh — check_doc_links.sh 的契約測試（FU-20261016-18）
#
# 為什麼要有它
# ------------
# 舊版 `check_doc_links.sh` 在 macOS 上**永遠不可能紅**：`grep -rnP` 不被 BSD grep 支援（rc=2、
# 0 行），同一行的 `2>/dev/null` 又把錯誤吞掉 ⇒ process substitution 給 while 的是空清單 ⇒
# 迴圈跑 0 次 ⇒ 無條件印 `✅ All links OK`、exit 0。**「工具壞了」與「沒有連結」在輸出上無法區分**。
#
# 修法是把實作**轉呼叫** check_markdown_links.sh（同一棵樹上原本就可用、且是 CI required
# context `markdown-links` 跑的那一支），本檔不重寫 pattern。這種修法壞掉的方式一樣很安靜：
#   · 有人把 grep 蒐集／判定**寫回**入口（第二份實作又開始分歧）
#   · 有人把錯誤吃掉（`2>/dev/null`、`|| true`、只看輸出字串）
#   · 被呼叫的實作被改名／搬走，或呼叫端環境壞了（工具不存在）⇒ 靜默回綠
# ⇒ 契約必須用 hermetic fixture 釘住，而不是靠註解。
#
# 釘住的契約（每條都在下面有對應 case）：
#   ① docs/ 內的**相對**壞連結 ⇒ exit 1，且輸出指名檔案（牙齒）
#   ② 同一棵 fixture 把連結修好 ⇒ exit 0（證明 ① 有鑑別力，不是空集合恆紅）
#   ③ 把被呼叫端的判定換成 no-op ⇒ ① 的紅燈消失（證明紅燈**來自被呼叫端**，不是其他地方）
#   ④ 實作被搬走 ⇒ exit 2 且講明原因（不得靜默回綠）
#   ⑤ 不在 git work tree 內 ⇒ exit 2
#   ⑥ 帶了參數 ⇒ exit 2（不得靜默忽略參數）
#   ⑦ 工具不存在（PATH 內沒有 python3）⇒ 非 0（精確 127）：工具錯誤必須 fail loud
#   ⑧ 靜態契約：入口的可執行行內**不得**有 grep/find 蒐集或吞錯（實作返祖守衛）
#   ⑨ 被 hook 呼叫（GIT_DIR 指向呼叫者）時，fixture 不得污染呼叫者的 repo
#
# 結構：每個 case 都在 mktemp 內自建**臨時 git repo**，把三支真檔（入口／被呼叫端／py）
# 複製進去（三者的 REPO_ROOT 都由 BASH_SOURCE 推導 ⇒ 自然指向臨時 repo）
# ⇒ **不碰真 repo、不碰真 index**。
#
# 執行： bash tests/scripts/test-check-doc-links.sh
set -uo pipefail

# 為什麼開頭要 unset GIT_*：本檔的 fixture 用 `git -C <fixture> init/add/commit` 建臨時 repo。
# 當本檔被 git hook 呼叫時（pre-push → make ci-gate），環境帶著 GIT_DIR（指向**呼叫者**的 .git）
# ⇒ `git -C <fixture>` **不會**切到 fixture repo（GIT_DIR 優先序高於 -C 的目錄探索）
# ⇒ fixture 的 add/commit 會寫進呼叫者的 index 與分支（同 tests/scripts/test-check-markdown-links.sh
# 2026-09-29 的實測事故）⇒ 這裡 unset，並在 ⑨ 用自我重入證明。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_SH="$REPO/scripts/ci/check_doc_links.sh"
SRC_CALLEE="$REPO/scripts/ci/check_markdown_links.sh"
SRC_PY="$REPO/scripts/ci/check_markdown_links.py"
PY="${PYTHON:-python3}"

# 共用負向證明庫（精確 rc 斷言：127/2/124/126 不得被當成「擋下了」）
# shellcheck source=../../scripts/ci/negative-proof-lib.sh
. "$REPO/scripts/ci/negative-proof-lib.sh"

# 呼叫者 repo 的基線（⑨ 會比對：本測試不得動到呼叫者）。**必須在 unset 之後**。
CALLER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
CALLER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/doclinks-contract.XXXXXX")"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
ok() { PASS=$((PASS + 1)); echo "  ✅ $*"; }
fail() { FAIL=$((FAIL + 1)); echo "  ❌ $*"; }

for f in "$SRC_SH" "$SRC_CALLEE" "$SRC_PY"; do
  negproof_require_file "$f" "受測檔案（入口／被呼叫端／py 實作）" || exit 1
done

# ── helpers ──────────────────────────────────────────────────────────────────

# new_repo <名稱> — 建臨時 repo，內含三支真檔的副本；echo 出目錄路徑
new_repo() {
  local dir="$TMP/$1"
  mkdir -p "$dir/scripts/ci"
  git -C "$dir" init -q
  git -C "$dir" config user.email fixture@example.test
  git -C "$dir" config user.name fixture
  cp "$SRC_SH" "$dir/scripts/ci/check_doc_links.sh"
  cp "$SRC_CALLEE" "$dir/scripts/ci/check_markdown_links.sh"
  cp "$SRC_PY" "$dir/scripts/ci/check_markdown_links.py"
  echo "$dir"
}

commit_all() { git -C "$1" add -A >/dev/null 2>&1; git -C "$1" commit -qm fixture >/dev/null 2>&1; }

# 受測命令（在同一 shell 內，供 negproof_expect 呼叫）：在 fixture 目錄內跑真檔入口。
run_doc_links() { ( cd "$1" && bash scripts/ci/check_doc_links.sh ); }
# 受測命令變體：自訂 PATH（用於「工具不存在」的 case）
run_doc_links_path() { ( cd "$1" && PATH="$2" bash scripts/ci/check_doc_links.sh ); }
# 受測命令變體：帶參數（用於「參數必須被明確拒絕」的 case）
run_doc_links_args() {
  local dir="$1"; shift
  ( cd "$dir" && bash scripts/ci/check_doc_links.sh "$@" )
}

# 去掉**整行註解**後的可執行行（靜態契約用；本檔的說明文字裡本來就會提到 2>/dev/null）
code_lines() { grep -v '^[[:space:]]*#' "$1"; }

echo "→ check_doc_links 契約測試（轉呼叫 check_markdown_links.sh；hermetic fixtures）"

# ── ① docs/ 內的相對壞連結 ⇒ exit 1（牙齒）────────────────────────────────────
DIR="$(new_repo docs-broken)"
mkdir -p "$DIR/docs"
printf '# ok\n\n[good](target.md)\n\n[bad](missing-doclinks-probe.md)\n' > "$DIR/docs/a.md"
printf '# target\n' > "$DIR/docs/target.md"
commit_all "$DIR"
LOG="$TMP/01-broken.log"
if negproof_expect 1 "docs/ 內相對壞連結 ⇒ exit 1（舊版在這裡回 0）" "$LOG" -- run_doc_links "$DIR"; then ok "① 壞連結必紅"; else fail "① 壞連結沒有讓入口變紅"; negproof_show_log "$LOG"; fi
negproof_expect_output 'docs/a.md' "$LOG" "① 紅燈要指名是哪個檔" || fail "① 輸出沒指名檔案"
negproof_expect_output 'missing-doclinks-probe.md' "$LOG" "① 紅燈要指出是哪條連結" || fail "① 輸出沒指出壞連結"

# ── ② 同一棵 fixture 修好 ⇒ exit 0（證明 ① 有鑑別力）──────────────────────────
printf '# ok\n\n[good](target.md)\n' > "$DIR/docs/a.md"
commit_all "$DIR"
LOG="$TMP/02-fixed.log"
if negproof_expect 0 "修好同一條連結 ⇒ exit 0" "$LOG" -- run_doc_links "$DIR"; then ok "② 修好後回綠"; else fail "② 修好後仍紅（① 沒有鑑別力）"; negproof_show_log "$LOG"; fi
negproof_expect_output 'All internal markdown links are valid' "$LOG" "② 綠燈必須是「真的檢查過」的那一支在說的" \
  || fail "② 綠燈不是被呼叫端印的"

# ── ③ 被呼叫端的判定換成 no-op ⇒ ① 的紅燈必須消失（紅燈來自被呼叫端）───────────
DIR="$(new_repo callee-stub)"
mkdir -p "$DIR/docs"
printf '# ok\n\n[bad](missing-doclinks-probe.md)\n' > "$DIR/docs/a.md"
commit_all "$DIR"
printf '#!/usr/bin/env bash\nexit 0\n' > "$DIR/scripts/ci/check_markdown_links.sh"
LOG="$TMP/03-stub.log"
if negproof_expect 0 "把被呼叫端換成 no-op ⇒ 紅燈消失（證明紅燈來自被呼叫端，不是別處）" "$LOG" -- run_doc_links "$DIR"; then
  ok "③ 判定確實來自被呼叫端"
else
  fail "③ 被呼叫端 no-op 後仍紅 ⇒ 紅燈不是來自被呼叫端（測試沒有鑑別力）"
  negproof_show_log "$LOG"
fi

# ── ④ 實作被搬走 ⇒ exit 2 且講明原因（不得靜默回綠）────────────────────────────
DIR="$(new_repo callee-missing)"
mkdir -p "$DIR/docs"
printf '# ok\n\n[bad](missing-doclinks-probe.md)\n' > "$DIR/docs/a.md"
commit_all "$DIR"
rm -f "$DIR/scripts/ci/check_markdown_links.sh"
LOG="$TMP/04-missing.log"
if negproof_expect 2 "實作不存在 ⇒ exit 2（fail loud，不得回綠）" "$LOG" -- run_doc_links "$DIR"; then
  ok "④ 實作缺失 fail loud"
else
  fail "④ 實作缺失沒有 fail loud（靜默回綠＝本票的缺陷形狀）"
  negproof_show_log "$LOG"
fi
negproof_expect_output '找不到實作' "$LOG" "④ 要講清楚是實作缺失" || fail "④ 訊息沒講清楚原因"

# ── ⑤ 不在 git work tree 內 ⇒ exit 2（明確報錯，不得靜默假綠）─────────────────
DIR="$TMP/not-a-repo"
mkdir -p "$DIR/scripts/ci"
cp "$SRC_SH" "$DIR/scripts/ci/check_doc_links.sh"
cp "$SRC_CALLEE" "$DIR/scripts/ci/check_markdown_links.sh"
cp "$SRC_PY" "$DIR/scripts/ci/check_markdown_links.py"
LOG="$TMP/05-notgit.log"
if negproof_expect 2 "不在 git work tree 內 ⇒ exit 2" "$LOG" -- run_doc_links "$DIR"; then
  ok "⑤ 非 git 目錄 fail loud"
else
  fail "⑤ 非 git 目錄沒有 fail loud"
  negproof_show_log "$LOG"
fi
negproof_expect_output '不在 git work tree' "$LOG" "⑤ 要講清楚是環境問題" || fail "⑤ 訊息沒講清楚原因"

# ── ⑥ 帶參數 ⇒ exit 2（不得靜默忽略參數）─────────────────────────────────────
DIR="$(new_repo args)"
mkdir -p "$DIR/docs"
printf '# ok\n\n[good](target.md)\n' > "$DIR/docs/a.md"
printf '# target\n' > "$DIR/docs/target.md"
commit_all "$DIR"
LOG="$TMP/06-args.log"
if negproof_expect 2 "帶參數 ⇒ 用法錯誤 exit 2（被呼叫端不吃參數，靜默忽略＝另一種假裝有做事）" "$LOG" \
   -- run_doc_links_args "$DIR" --paths docs/; then
  ok "⑥ 參數被明確拒絕"
else
  fail "⑥ 帶參數時沒有明確用法錯誤"
  negproof_show_log "$LOG"
fi

# ── ⑦ 工具不存在（PATH 內沒有 python3）⇒ 精確 127（工具錯誤不得被吞）──────────
# 注意：這一條測的是**整條鏈**（入口 → 被呼叫端 → xargs → python3）。工具壞掉時必須非 0；
# 舊版的形狀是「工具壞掉 ⇒ 回 0」。
DIR="$(new_repo tool-missing)"
mkdir -p "$DIR/docs"
printf '# ok\n\n[good](target.md)\n' > "$DIR/docs/target.md"
printf '# target\n' > "$DIR/docs/target.md"
commit_all "$DIR"
SHIM="$TMP/path-shim"
mkdir -p "$SHIM"
for t in bash git xargs dirname cat sort tr wc sed grep env sh; do
  p="$(command -v "$t" 2>/dev/null || true)"
  [ -n "$p" ] && ln -sf "$p" "$SHIM/$t"
done
if [ -e "$SHIM/python3" ]; then
  fail "⑦ fixture 準備失敗：shim 內竟然有 python3"
else
  LOG="$TMP/07-tool-missing.log"
  if negproof_expect 127 "python3 不存在 ⇒ 精確 127（工具錯誤 fail loud，不是回綠）" "$LOG" \
     -- run_doc_links_path "$DIR" "$SHIM"; then
    ok "⑦ 工具缺失 fail loud（rc=127）"
  else
    fail "⑦ 工具缺失時的結束碼不是 127 ⇒ 工具錯誤沒有被明確傳遞"
    negproof_show_log "$LOG"
  fi
fi

# ── ⑧ 靜態契約：入口不得自己蒐集檔案／不得吞錯（實作返祖守衛）────────────────────
CODE="$TMP/entry-code.txt"
code_lines "$SRC_SH" > "$CODE"
if grep -qF -- 'exec bash "$CALLEE"' "$CODE"; then
  ok "⑧ 入口以 exec 轉呼叫被呼叫端"
else
  fail "⑧ 入口不再轉呼叫被呼叫端（實作被寫回入口 ⇒ 會再出現第二份分歧的實作）"
fi
for pat in 'grep ' 'find ' '2>/dev/null' '|| true'; do
  if grep -qF -- "$pat" "$CODE"; then
    fail "⑧ 入口的可執行行內出現 '$pat'（蒐集/判定寫回入口，或錯誤被吞）"
  else
    ok "⑧ 入口沒有 '$pat'"
  fi
done

# ── ⑨ 自我重入：在「hook 形狀」的環境（GIT_DIR 指向呼叫者）再跑一次本腳本 ────────
if [ "${SELF_REENTRY:-0}" != "1" ]; then
  REENTRY_LOG="$TMP/reentry.log"
  if GIT_DIR="$REPO/.git" SELF_REENTRY=1 bash "$0" > "$REENTRY_LOG" 2>&1; then
    ok "⑨ 自我重入（hook 形狀：GIT_DIR 指向呼叫者）通過"
  else
    fail "⑨ 自我重入失敗（fixture 不 hermetic 或契約不成立）"
    sed -n '1,12p' "$REENTRY_LOG" | sed 's/^/      /'
  fi
  if [ "$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)" = "$CALLER_HEAD" ] \
     && [ "$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')" = "$CALLER_TRACKED" ]; then
    ok "⑨ 本測試沒有動到呼叫者 repo"
  else
    fail "⑨ 本測試動到了呼叫者的 repo（HEAD 或 index 變了）—— fixture 不 hermetic"
  fi
fi

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ test-check-doc-links: $PASS passed, $FAIL failed"
  exit 1
fi
echo "✅ test-check-doc-links: $PASS passed（壞連結必紅／修好回綠／no-op 對照／實作缺失／非 git／參數／工具缺失 127／靜態返祖守衛／hook 重入＋呼叫者不變）"
