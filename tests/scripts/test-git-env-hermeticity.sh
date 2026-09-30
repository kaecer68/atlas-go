#!/usr/bin/env bash
# test-git-env-hermeticity.sh — 「建 throwaway repo 的 GIT_* 隔離」契約測試（hermetic，不連網）
#
# 【為什麼有這支測試】靜態檢查只能斷言結構；這個陷阱的**危害**必須被實際證明一次：
# ambient `GIT_DIR` 會讓在 throwaway repo 內執行的 git 指令打到**呼叫者**的 repo。
# 本測試同時提供：
#   C 正向：已防護的 fixture 在**被劫持的環境**下跑 ⇒ rc=0 且被保護 repo 完全不變
#   B 負向：**移除 unset** 的 fixture 在同樣環境下跑 ⇒ 被保護 repo 確實被改（HEAD 變／
#           core.bare false→true／commit +1）⇒ 證明這個護欄有牙齒
#   mutation 自證（A）：拿掉檢查器對 unset 的偵測 ⇒ A 判準必 FAIL（下方以 fixture 反證）
#
# 用法：bash tests/scripts/test-git-env-hermeticity.sh
set -uo pipefail

# (a) 本檔自己也會建 throwaway repo ⇒ 必須先丟掉會「選 repo」的 ambient 變數（否則被 hook 呼叫時
# 每個 fixture git 指令都會打到呼叫者的 repo；本 session 已實證此坑）。
# (b) 之後所有 fixture 呼叫一律顯式定址（`git -C "$VAR"` 或 `--git-dir`），不再有裸呼叫。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_PREFIX \
      GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
CHECK="$ROOT/scripts/ci/check_git_env_hermeticity.py"
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

echo "── git 環境隔離（GIT_* hermeticity）契約測試 ──"
[ -f "$CHECK" ] || { echo "❌ 找不到檢查器 $CHECK"; exit 1; }

# ── 被保護 repo（模擬「呼叫者的 repo」）──────────────────────────────
PROT="$TMP/protected"; mkdir -p "$PROT"
git -c init.defaultBranch=main init -q "$PROT"
git -C "$PROT" config user.email t@t; git -C "$PROT" config user.name t
echo one > "$PROT/f"; git -C "$PROT" add f; git -C "$PROT" commit -qm base
head_before="$(git -C "$PROT" rev-parse HEAD)"
bare_before="$(git -C "$PROT" config --bool core.bare 2>/dev/null || echo unset)"
count_before="$(git -C "$PROT" rev-list --count HEAD)"

# ── fixture：建 throwaway repo 的腳本（$1=guarded|unguarded）──────────
write_fixture() {
  local mode="$1" out="$2"
  {
    echo '#!/usr/bin/env bash'
    echo 'set -uo pipefail'
    echo 'WORK="$(mktemp -d)"'
    if [ "$mode" = guarded ]; then
      echo '# (a) 丟掉會選 repo 的 ambient 變數（c) 絆線：進入記、結束比對'
      echo 'unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_COMMON_DIR GIT_PREFIX \'
      echo '      GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES'
    fi
    echo 'git init -q "$WORK/repo"'
    echo 'git -C "$WORK/repo" config user.email t@t; git -C "$WORK/repo" config user.name t'
    echo 'echo x > "$WORK/repo/x"; git -C "$WORK/repo" add x; git -C "$WORK/repo" commit -qm fixture'
    echo 'test -f "$WORK/repo/x" || exit 3'
  } > "$out"
  chmod +x "$out"
}

protected_state() { printf '%s|%s|%s' "$(git -C "$PROT" rev-parse HEAD 2>/dev/null)" "$(git -C "$PROT" config --bool core.bare 2>/dev/null || echo unset)" "$(git -C "$PROT" rev-list --count HEAD 2>/dev/null)"; }
before="$(protected_state)"

# ── C 正向 ───────────────────────────────────────────────────────────
write_fixture guarded "$TMP/guarded.sh"
GIT_DIR="$PROT/.git" bash "$TMP/guarded.sh" >/dev/null 2>&1; rc=$?
after="$(protected_state)"
[ "$rc" -eq 0 ] && ok "C 正向 rc=0（已防護 fixture 在劫持環境下正常完成）" || bad "C 正向 rc=${rc}（期望 0）"
[ "$after" = "$before" ] && ok "C 正向：被保護 repo 不變（HEAD／core.bare／commit 數皆相同）" || bad "C 正向：被保護 repo 竟被改動（before=$before after=${after}）"

# ── B 負向（移除 unset ⇒ 必須被劫持）────────────────────────────────
write_fixture unguarded "$TMP/unguarded.sh"
GIT_DIR="$PROT/.git" bash "$TMP/unguarded.sh" >/dev/null 2>&1; rc2=$?
hij="$(protected_state)"
[ "$hij" != "$before" ] && ok "B 負向：確實被劫持（before=$before → after=${hij}）" || bad "B 負向：未被劫持，負向對照失效"
echo "     （B 的具體變動：rc=${rc2}，HEAD=$(git -C "$PROT" rev-parse --short HEAD 2>/dev/null)，bare=$(git -C "$PROT" config --bool core.bare 2>/dev/null || echo unset)，commits=$(git -C "$PROT" rev-list --count HEAD 2>/dev/null) 起始 HEAD=${head_before:0:7} bare=$bare_before commits=${count_before}）"

# ── A/mutation 自證：檢查器對 fixture 的鑑別力 ───────────────────────
FIX="$TMP/fixroot"; mkdir -p "$FIX/scripts"
cp "$TMP/unguarded.sh" "$FIX/scripts/fixture.sh"
python3 "$CHECK" --root "$FIX" --paths scripts >/dev/null 2>&1; rc_bad=$?
cp "$TMP/guarded.sh" "$FIX/scripts/fixture.sh"
python3 "$CHECK" --root "$FIX" --paths scripts >/dev/null 2>&1; rc_good=$?
[ "$rc_bad" -eq 1 ] && ok "A 判準有牙齒：缺 unset 的 fixture ⇒ 檢查器 exit 1" || bad "A：缺 unset 竟未 FAIL（exit=${rc_bad}）⇒ mutation 自證失敗"
[ "$rc_good" -eq 0 ] && ok "A 判準不誤報：已防護 fixture ⇒ 檢查器 exit 0" || bad "A：已防護 fixture 被誤報（exit=${rc_good}）"

# ── repo 現況（全掃必須為 0 未防護）─────────────────────────────────
python3 "$CHECK" --root "$ROOT" >/dev/null 2>&1; rc_repo=$?
[ "$rc_repo" -eq 0 ] && ok "repo 全掃：未防護數 = 0（allowlist 皆具名理由）" || bad "repo 全掃：存在未防護檔（exit=${rc_repo}）"

echo "── 結果：pass=$pass fail=$fail ──"
[ "$fail" -eq 0 ] && { echo "✅ PASS"; exit 0; } || { echo "❌ FAIL"; exit 1; }
