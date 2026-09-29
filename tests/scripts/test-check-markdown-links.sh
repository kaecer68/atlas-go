#!/usr/bin/env bash
# tests/scripts/test-check-markdown-links.sh — check_markdown_links.sh 的契約測試（issue #2114）
#
# 為什麼要有它
# ------------
# 本票的修法是「蒐集改成 git-aware（只檢查版控內檔案）」。這種修法**壞掉的方式很安靜**：
# 把蒐集改回 `find`、或把 pathspec 的 `glob` magic 拿掉，都不會讓任何人看到紅燈 ——
# 只會多掃幾萬個 ignored 檔（實測 40,444 檔中有 39,691 檔是 ignored），偶爾再咬一次假陽性。
# 所以契約必須用 hermetic fixture 釘住，而不是靠註解。
#
# 釘住的契約（四個世界）：
#   ① tracked                → 檢查（壞連結 ⇒ exit 1）
#   ② untracked 且未 ignored → 檢查（那是**即將 commit 的草稿**；不檢查等於把紅燈往後推）
#   ③ ignored（untracked）   → **不檢查**（壞連結 ⇒ 仍 exit 0）  ← 本票的假陽性本體
#   ④ 保留的排除清單必須**真的生效**（`:(exclude,glob)…` 的 glob magic 不能掉）
#   ＋ 空 repo（0 個 .md）⇒ exit 0（`xargs -r` 契約：空清單不得呼叫 python）
#   ＋ 不在 git work tree 內 ⇒ exit 2（明確報錯，不得靜默回報「全部有效」）
#
# 結構：每個 case 都在 mktemp 內自建**臨時 git repo**，並把真實腳本與 py **複製**進去
# （腳本的 REPO_ROOT 由 BASH_SOURCE 推導 ⇒ 自然指向臨時 repo）⇒ **不碰真 repo、不碰真 index**。
#
# 執行： bash tests/scripts/test-check-markdown-links.sh
set -uo pipefail

# ── 為什麼開頭要 unset GIT_*（2026-09-29 我實際踩到的坑）──────────────────────────
# 本檔的 fixture 用 `git -C <fixture> add/commit` 建立臨時 repo。當本檔被 **git hook**
# 呼叫時（例如 pre-push → make ci-gate），環境裡帶著 GIT_DIR（指向**呼叫者**的 .git）
# ⇒ `git -C <fixture>` **不會**切到 fixture repo（GIT_DIR 的優先序高於 -C 的目錄探索）
# ⇒ 那些 add/commit 會寫進**呼叫者的 index 與分支**。實測後果：本票的 pre-push 一次執行
# 就把 **6 個 "fixture" commit 塞進 worktree 分支**，並把 fixture 檔（docs/ok.md…）寫進
# 它的 index（事後以 `git reset --mixed <本分支真正的 commit>` 復原）。
# ⇒ 契約：本測試必須對「被 hook 呼叫」免疫。做法＝這裡 unset，並在 ⑧ 用自我重入證明。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_CEILING_DIRECTORIES

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC_SH="$REPO/scripts/ci/check_markdown_links.sh"
SRC_PY="$REPO/scripts/ci/check_markdown_links.py"
PY="${PYTHON:-python3}"

# 呼叫者 repo 的基線（⑧ 會比對：本測試不得動到呼叫者）。**必須在 unset 之後、REPO 定義之後**。
CALLER_HEAD="$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)"
CALLER_TRACKED="$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')"

TMP="$(mktemp -d)"
if [ "${KEEP_FIXTURES:-0}" = "1" ]; then
  echo "ℹ️  KEEP_FIXTURES=1 ⇒ fixture 保留在 $TMP"
else
  trap 'rm -rf "$TMP"' EXIT
fi

PASS=0
FAIL=0
fail() { echo "    FAIL: $*"; FAIL=$((FAIL + 1)); }
ok() { PASS=$((PASS + 1)); }

if [ ! -f "$SRC_SH" ] || [ ! -f "$SRC_PY" ]; then
  echo "❌ 找不到 scripts/ci/check_markdown_links.{sh,py}"
  exit 1
fi

# ── helpers ──────────────────────────────────────────────────────────────────

# 建一個臨時 repo（含真實腳本與 py 的副本）。echo 出目錄路徑。
new_repo() {
  local dir="$TMP/$1"
  mkdir -p "$dir/scripts/ci"
  git -C "$dir" init -q
  git -C "$dir" config user.email fixture@example.test
  git -C "$dir" config user.name fixture
  cp "$SRC_SH" "$dir/scripts/ci/check_markdown_links.sh"
  cp "$SRC_PY" "$dir/scripts/ci/check_markdown_links.py"
  printf 'ignored-notes/\n' > "$dir/.gitignore"
  echo "$dir"
}

commit_all() {
  git -C "$1" add -A >/dev/null 2>&1
  git -C "$1" commit -qm fixture >/dev/null 2>&1
}

# 跑檢查器；結果放進 RC / OUT（不讓 set -e 提前中止）
RC=0
OUT_FILE=""
run_case() {
  OUT_FILE="$TMP/out-$(basename "$1").txt"
  ( cd "$1" && bash scripts/ci/check_markdown_links.sh ) > "$OUT_FILE" 2>&1
  RC=$?
}
out() { cat "$OUT_FILE"; }

expect_rc() {
  if [ "${RC}" = "$2" ]; then
    ok
  else
    fail "$1: exit=${RC}，期望 $2"
    out | head -8 | sed 's/^/      /'
  fi
}

expect_out_contains() {
  if grep -qF -- "$2" "$OUT_FILE"; then
    ok
  else
    fail "$1: 輸出缺少「$2」"
    out | head -8 | sed 's/^/      /'
  fi
}

# mutation ①：把 git-aware 蒐集整段換回舊的 find（含被移除的那些排除）
# 用 python 切字串（不靠 sed/轉義）：從 `git ls-files` 起，到 `| xargs` 前一行止。
mutate_to_find() {
  "$PY" - "$1" <<'PY'
import sys
path = sys.argv[1]
src = open(path, encoding="utf-8").read()
start = src.index("git ls-files -z -c -o --exclude-standard -- '*.md'")
end = src.index("| xargs -0 -r -n 200")
block = src[start:end]
if src.count(block) != 1:
    print("MUTATION TARGET NOT UNIQUE")
    sys.exit(2)
new = "find . -name '*.md' -not -path './.git/*' -print0 "
open(path, "w", encoding="utf-8").write(src.replace(block, new))
PY
}

# mutation ②：刪掉單一條排除（用來證明「綠」是排除造成的，而不是「沒掃到」）
mutate_drop_exclusion() {
  # $1=腳本路徑 $2=要刪掉的那一行所含的子字串
  "$PY" - "$1" "$2" <<'PY'
import sys
path, needle = sys.argv[1], sys.argv[2]
lines = open(path, encoding="utf-8").read().splitlines(keepends=True)
kept = [ln for ln in lines if needle not in ln]
removed = len(lines) - len(kept)
if removed != 1:
    print("MUTATION TARGET NOT UNIQUE (%d)" % removed)
    sys.exit(2)
open(path, "w", encoding="utf-8").write("".join(kept))
PY
}

echo "→ check_markdown_links 契約測試（git-aware 蒐集；hermetic fixtures）"

# ── ① tracked 壞連結 ⇒ exit 1（牙齒）─────────────────────────────────────────
DIR="$(new_repo tracked-broken)"
mkdir -p "$DIR/docs"
printf '# a\n\n[x](missing.md)\n' > "$DIR/docs/a.md"
commit_all "$DIR"
run_case "$DIR"
expect_rc "tracked 壞連結" 1
expect_out_contains "tracked 壞連結要指名檔案" 'docs/a.md:3'

# ── ② untracked 且未 ignored 的壞連結 ⇒ exit 1（即將 commit 的草稿）───────────
DIR="$(new_repo untracked-broken)"
mkdir -p "$DIR/docs"
printf '# a\n\n[x](missing.md)\n' > "$DIR/docs/draft.md"
run_case "$DIR"
expect_rc "untracked 未 ignored 壞連結" 1
expect_out_contains "untracked 壞連結要指名檔案" 'docs/draft.md:3'

# ── ③ ignored 的壞連結 ⇒ exit 0（本票的假陽性本體）───────────────────────────
DIR="$(new_repo ignored-draft)"
mkdir -p "$DIR/docs" "$DIR/ignored-notes"
printf '# ok\n\n見 [other](other.md)。\n' > "$DIR/docs/ok.md"
printf '# other\n' > "$DIR/docs/other.md"
printf '# draft\n\n[x](docs/nope.md)\n' > "$DIR/ignored-notes/draft.md"
commit_all "$DIR"
# 前置事實檢查：那兩個檔在 fixture 裡真的「一個版控內、一個 ignored」
if git -C "$DIR" ls-files --error-unmatch docs/ok.md >/dev/null 2>&1; then ok; else fail "fixture: docs/ok.md 應為 tracked"; fi
if git -C "$DIR" check-ignore -q ignored-notes/draft.md; then ok; else fail "fixture: ignored-notes/draft.md 應被 .gitignore 忽略"; fi
run_case "$DIR"
expect_rc "ignored 草稿的壞連結不得讓檢查變紅" 0

# ── ④ mutation：把蒐集改回 find ⇒ ③ 必須變紅（證明 ③ 有牙齒）───────────────
DIR="$(new_repo mutation-find)"
mkdir -p "$DIR/docs" "$DIR/ignored-notes"
printf '# ok\n' > "$DIR/docs/ok.md"
printf '# draft\n\n[x](docs/nope.md)\n' > "$DIR/ignored-notes/draft.md"
commit_all "$DIR"
if mutate_to_find "$DIR/scripts/ci/check_markdown_links.sh" > "$TMP/mut1.txt" 2>&1
then
  run_case "$DIR"
  expect_rc "mutation（蒐集改回 find）⇒ ignored 草稿必須變紅" 1
else
  fail "mutation/find: 改不動蒐集那一行（$(cat "$TMP/mut1.txt")）"
fi

# ── ⑤ 保留的排除必須真的生效（:(exclude,glob) 的 glob magic）─────────────────
DIR="$(new_repo exclusion-magic)"
mkdir -p "$DIR/.claude" "$DIR/docs"
printf '# claude\n\n[x](missing.md)\n' > "$DIR/.claude/notes.md"
printf '# ok\n' > "$DIR/docs/ok.md"
commit_all "$DIR"
run_case "$DIR"
expect_rc "**/.claude/** 是保留排除（即使 tracked 也不檢查）" 0
# 反證：把那一條排除拿掉 ⇒ 同一個 fixture 必須變紅（否則上一條只是「沒掃到」的假綠）
if mutate_drop_exclusion "$DIR/scripts/ci/check_markdown_links.sh" ":(exclude,glob)**/.claude/**" > "$TMP/mut2.txt" 2>&1
then
  run_case "$DIR"
  expect_rc "移除 .claude 排除 ⇒ 必須變紅（證明排除是它生效的原因）" 1
else
  fail "mutation/exclusion: 改不動 .claude 那一條排除（$(cat "$TMP/mut2.txt")）"
fi

# ── ⑥ 空 repo（0 個 .md）⇒ exit 0（xargs -r 契約）────────────────────────────
DIR="$(new_repo empty-repo)"
run_case "$DIR"
expect_rc "空 repo（0 個 .md）⇒ exit 0，不得是 exit 2" 0

# ── ⑦ 不在 git work tree 內 ⇒ exit 2（明確報錯，不得靜默假綠）─────────────────
DIR="$TMP/not-a-repo"
mkdir -p "$DIR/scripts/ci"
cp "$SRC_SH" "$DIR/scripts/ci/check_markdown_links.sh"
cp "$SRC_PY" "$DIR/scripts/ci/check_markdown_links.py"
run_case "$DIR"
expect_rc "不在 git work tree ⇒ exit 2" 2
expect_out_contains "不在 git work tree 要講清楚" '不在 git work tree'

# ── ⑧ 自我重入：在「hook 形狀」的環境（GIT_DIR 指向呼叫者）再跑一次本腳本 ──────────
# 這一條把「被 hook 呼叫也不得動到呼叫者」變成可測的契約：
#   · 子行程繼承 GIT_DIR（模擬 pre-push）⇒ 若開頭的 unset 被移除，子行程的 fixture 就會
#     把檔案寫進呼叫者的 index／分支 ⇒ 下面的 HEAD / tracked 比對會 FAIL。
#   · 子行程用 SELF_REENTRY=1 自我抑制，不會無限遞迴。
if [ "${SELF_REENTRY:-0}" != "1" ]; then
  REENTRY_LOG="$TMP/reentry.log"
  if GIT_DIR="$REPO/.git" SELF_REENTRY=1 bash "$0" > "$REENTRY_LOG" 2>&1; then
    ok
  else
    fail "⑧ 自我重入（hook 形狀：GIT_DIR 指向呼叫者）失敗"
    sed -n '1,12p' "$REENTRY_LOG" | sed 's/^/      /'
  fi
  if [ "$(git -C "$REPO" rev-parse HEAD 2>/dev/null || echo none)" = "$CALLER_HEAD" ] \
     && [ "$(git -C "$REPO" ls-files 2>/dev/null | wc -l | tr -d ' ')" = "$CALLER_TRACKED" ]; then
    ok
  else
    fail "⑧ 本測試動到了呼叫者的 repo（HEAD 或 index 變了）—— fixture 不 hermetic"
  fi
fi

echo ""
if [ "$FAIL" -gt 0 ]; then
  echo "❌ test-check-markdown-links: $PASS passed, $FAIL failed"
  exit 1
fi
echo "✅ test-check-markdown-links: $PASS passed（tracked/untracked/ignored/exclusion ×2 mutation/空 repo/非 git/hook-GIT_DIR 重入＋呼叫者不變）"
