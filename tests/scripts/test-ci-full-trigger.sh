#!/usr/bin/env bash
# tests/scripts/test-ci-full-trigger.sh — scripts/ci/ci-full-trigger.sh 的契約測試。
#
# 這支腳本是「這次 diff 要不要跑 ci-full」的**唯一**判定實作（`Makefile` 的 `pre-push`
# target 與 `.githooks/pre-push` 都呼叫它）。修前兩邊各自寫一份
#     if git diff --name-only origin/main...HEAD 2>/dev/null | grep -qE '<pattern>'; then …
# 有兩個後果，本測試各用一個 scenario 釘住：
#   (a) git 失敗（range 不可解析／無 merge base／git 缺失）的 stdout 是空的，與「真的只改
#       文件」**逐字相同** ⇒ 判 docs-only ⇒ ci-full 靜默跳過（FU-20261016-21）。修後必須是
#       rc=2（fail loud），且呼叫端不得把它當 docs-only。scenario 6-9 釘住這一側，
#       scenario 11 用「舊管線形狀的突變本」自證這些斷言有鑑別力。
#   (b) 兩份 pattern 已經分歧：`|^configs/`（PR #2176）只進了 hook ⇒ `make pre-push`
#       在只改 configs/** 的 push 上仍跳過 ci-full。scenario 3 釘住 configs/，
#       scenario 12 用靜態斷言禁止 pattern 再被複製第二份。
#
# 另一個「不得退化」方向（scenario 5、10）：真的 docs-only／無關檔案仍必須是 rc=1 ——
# 修法不可以變成「一律跑 ci-full」或「一律紅」（那會讓每個 docs PR 白等 5-8 分鐘）。
set -euo pipefail

# git hook 會 export GIT_DIR 等變數；fixture 的 git 指令必須落在 fixture 自己身上
# （2026-09-23 事故：fixture commit 被寫進呼叫者的 repo，issue #1927）。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
BASH_BIN=$(command -v bash)   # scenario7 會把 PATH 換成空目錄，必須用絕對路徑叫 bash
SCRIPT="$ROOT/scripts/ci/ci-full-trigger.sh"
HOOK="$ROOT/.githooks/pre-push"
MAKEFILE="$ROOT/Makefile"

fail() { echo "FAIL: $*" >&2; exit 1; }

# new_fixture → 印出一個 repo 路徑：base commit 在 main，refs/remotes/origin/main 指著它，
# 檢出分支 fix/lane（第一個變更之後 origin/main 就用來當 diff 的 base）。
new_fixture() {
  local tmp tmp_real root
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/atlas-ci-full-trigger.XXXXXX")
  tmp_real=$(cd "$tmp" && pwd -P)
  git init -q "$tmp"
  root=$(git -C "$tmp" rev-parse --show-toplevel 2>/dev/null || echo "")
  [ "$root" = "$tmp_real" ] || {
    echo "FAIL: fixture escaped its own directory (root='$root' tmp_real='$tmp_real')" >&2
    exit 1
  }
  git -C "$tmp" config user.email test@atlas.local
  git -C "$tmp" config user.name test
  printf 'readme\n' >"$tmp/README.md"
  git -C "$tmp" add README.md
  git -C "$tmp" commit -qm "docs: base"
  git -C "$tmp" update-ref refs/remotes/origin/main HEAD
  git -C "$tmp" checkout -q -b fix/lane
  echo "$tmp"
}

# commit_paths <repo> <msg> <path...> — 寫入內容、add、commit；路徑不存在時建空檔
commit_paths() {
  local repo=$1 msg=$2
  shift 2
  local p
  for p in "$@"; do
    mkdir -p "$repo/$(dirname "$p")"
    printf 'x\n' >>"$repo/$p"
    git -C "$repo" add -- "$p"
  done
  git -C "$repo" -c commit.gpgsign=false commit -qm "$msg"
}

# run_trigger <cwd> [args...] → TRIGGER_RC / TRIGGER_STDOUT / TRIGGER_STDERR
# （stdout 與 stderr 分開收：契約是「rc 表達判定、診斷走 stderr」，混在一起就驗不出來）
TRIGGER_RC=0
TRIGGER_STDOUT=""
TRIGGER_STDERR=""
run_trigger() {
  local dir=$1
  shift
  local out err
  [ -d "$dir" ] || fail "run_trigger: no such directory $dir"
  out=$(mktemp "${TMPDIR:-/tmp}/atlas-cft-out.XXXXXX")
  err=$(mktemp "${TMPDIR:-/tmp}/atlas-cft-err.XXXXXX")
  TRIGGER_RC=0
  ( cd "$dir" && bash "$SCRIPT" "$@" >"$out" 2>"$err" ) || TRIGGER_RC=$?
  TRIGGER_STDOUT=$(cat "$out")
  TRIGGER_STDERR=$(cat "$err")
  rm -f "$out" "$err"
}

# expect_rc <期望> <說明> — 精確相等（issue #2011：把 127/2 當成「擋下了」的舊寫法沒有鑑別力）
expect_rc() {
  local want=$1 what=$2
  [ "$TRIGGER_RC" -eq "$want" ] || \
    fail "$what: 期望 rc=${want}，實得 rc=${TRIGGER_RC}（stdout='$TRIGGER_STDOUT' stderr='$TRIGGER_STDERR'）"
}

# ── 1-5：判定本身（真 git fixture；rc 0=改碼 / 1=真的沒改碼）──────────────

scenario1_go_change_runs_ci_full() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "feat(x): go" internal/foo/foo.go
  run_trigger "$tmp" origin/main...HEAD
  expect_rc 0 "scenario1: .go 變更必須判「跑 ci-full」（rc=0）"
  [ -z "$TRIGGER_STDOUT" ] || fail "scenario1: rd=0 不該有 stdout（訊息由呼叫端負責），實得 '$TRIGGER_STDOUT'"
  echo "  ✓ scenario1: .go 變更 ⇒ rc=0（跑 ci-full）"
}

scenario2_docs_only_skips() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "docs: note" docs/note.md
  run_trigger "$tmp" origin/main...HEAD
  expect_rc 1 "scenario2: 只有 .md ⇒ 真的 docs-only（rc=1）"
  [ -z "$TRIGGER_STDOUT" ] || fail "scenario2: rc=1 不該有 stdout，實得 '$TRIGGER_STDOUT'"
  echo "  ✓ scenario2: 真的 docs-only ⇒ rc=1（可跳過 ci-full）"
}

scenario3_configs_only_runs_ci_full() {
  # ★ 分歧回歸（PR #2176 只改 hook）：`make pre-push` 用的舊 Makefile pattern 沒有
  # `^configs/`，只改 configs/parameters.json 的 push 會被它判 docs-only 而跳過 ci-full。
  # 判準依據：configs/ 全目錄都是「值／治理」檔（monitor_limits.json 等），改值＝改行為。
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "config: value" configs/parameters.json
  run_trigger "$tmp" origin/main...HEAD
  expect_rc 0 "scenario3: configs/ 值變更必須判「跑 ci-full」（rc=0）——舊 Makefile pattern 會判 1"
  echo "  ✓ scenario3: configs/** 值變更 ⇒ rc=0（PR #2176 的分歧已消除）"
}

scenario4_build_and_ci_inputs_run_ci_full() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  local p
  local -a paths=(Makefile Dockerfile docker-compose.yml .github/workflows/x.yml \
                  .githooks/pre-push scripts/x.sh internal/x/x.py bin/x.c lib/x.cpp inc/x.h \
                  internal/x/y.ts admin_web/z.jsx)
  for p in "${paths[@]}"; do
    rm -rf "${tmp:?}/internal" "${tmp:?}/.github" "${tmp:?}/.githooks" "${tmp:?}/scripts" \
           "${tmp:?}/bin" "${tmp:?}/lib" "${tmp:?}/inc" "${tmp:?}/admin_web"
    git -C "$tmp" checkout -q -- . 2>/dev/null || true
    commit_paths "$tmp" "chore: $p" "$p"
    run_trigger "$tmp" origin/main...HEAD
    expect_rc 0 "scenario4: $p 必須判「跑 ci-full」（rc=0）"
    git -C "$tmp" -c commit.gpgsign=false commit -q --allow-empty -m "chore: reset $p"
    git -C "$tmp" update-ref refs/remotes/origin/main HEAD
  done
  echo "  ✓ scenario4: Makefile/Dockerfile/compose/.github/.githooks/*.sh/*.py/*.js/*.ts ⇒ rc=0"
}

scenario5_non_code_paths_still_skip() {
  # 不得退化方向（1/2）：修法不可以變成「一律跑 ci-full」。非程式碼、非值變更的路徑
  # 仍走輕量路徑（行為與修前一致），否則每個 docs PR 都要多等 5-8 分鐘。
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "assets: png" assets/logo.png
  run_trigger "$tmp" origin/main...HEAD
  expect_rc 1 "scenario5: 非程式碼路徑（assets/logo.png）仍應跳過 ci-full（rc=1）"
  echo "  ✓ scenario5: 非程式碼路徑仍 rc=1（沒有被改成一律跑 ci-full）"
}

# ── 6-9：git 失敗（＝本票的根因形狀）必須 rc=2，不得是 1 ──────────────────

scenario6_unresolvable_range_is_not_docs_only() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "feat(x): go" internal/foo/foo.go
  # 前提斷言：這個 range 真的不可解析，否則本 scenario 會靜默退化成 scenario1。
  git -C "$tmp" rev-parse --verify -q origin/does-not-exist >/dev/null && \
    fail "scenario6: fixture 竟然解析得到 origin/does-not-exist — range 不是不可解析"
  run_trigger "$tmp" origin/does-not-exist...HEAD
  expect_rc 2 "scenario6: range 不可解析必須 rc=2（fail loud），不得是 1（docs-only）"
  printf '%s\n' "$TRIGGER_STDERR" | grep -qF "origin/does-not-exist...HEAD" || \
    fail "scenario6: 失敗訊息必須指名 range，實得 '$TRIGGER_STDERR'"
  printf '%s\n' "$TRIGGER_STDERR" | grep -qF "git rc=128" || \
    fail "scenario6: 失敗訊息必須回報 git 的 rc=128，實得 '$TRIGGER_STDERR'"
  echo "  ✓ scenario6: range 不可解析 ⇒ rc=2（並指名 range 與 git rc）"
}

scenario7_git_missing_is_not_docs_only() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "feat(x): go" internal/foo/foo.go
  local fake="$tmp/emptybin"
  mkdir -p "$fake"
  TRIGGER_RC=0
  local out err
  out=$(mktemp "${TMPDIR:-/tmp}/atlas-cft-out.XXXXXX"); err=$(mktemp "${TMPDIR:-/tmp}/atlas-cft-err.XXXXXX")
  ( cd "$tmp" && PATH="$fake" "$BASH_BIN" "$SCRIPT" main~1...HEAD >"$out" 2>"$err" ) || TRIGGER_RC=$?
  TRIGGER_STDOUT=$(cat "$out"); TRIGGER_STDERR=$(cat "$err"); rm -f "$out" "$err"
  # 127（找不到 git）必須被映射成契約值 2 ⇒ 呼叫端單靠 2 就能 fail loud。
  expect_rc 2 "scenario7: git 不存在必須 rc=2（不是 127、不是 1）"
  printf '%s\n' "$TRIGGER_STDERR" | grep -qF "git rc=127" || \
    fail "scenario7: 訊息必須回報 git 執行失敗的原始 rc=127，實得 '$TRIGGER_STDERR'"
  echo "  ✓ scenario7: git 缺失 ⇒ rc=2（原始 rc=127 有被回報）"
}

scenario8_git_itself_fails_is_not_docs_only() {
  # seam（與 FU-20261016-19 scenario12 同法）：range 合法，但 git 本身失敗（任何原因）。
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "feat(x): go" internal/foo/foo.go
  local fake="$tmp/fakebin"
  mkdir -p "$fake"
  printf '#!/usr/bin/env bash\necho "fatal: injected git failure (test seam)" >&2\nexit 128\n' >"$fake/git"
  chmod +x "$fake/git"
  TRIGGER_RC=0
  local out err
  out=$(mktemp "${TMPDIR:-/tmp}/atlas-cft-out.XXXXXX"); err=$(mktemp "${TMPDIR:-/tmp}/atlas-cft-err.XXXXXX")
  ( cd "$tmp" && PATH="$fake:$PATH" bash "$SCRIPT" main~1...HEAD >"$out" 2>"$err" ) || TRIGGER_RC=$?
  TRIGGER_STDOUT=$(cat "$out"); TRIGGER_STDERR=$(cat "$err"); rm -f "$out" "$err"
  expect_rc 2 "scenario8: git 失敗必須 rc=2"
  printf '%s\n' "$TRIGGER_STDERR" | grep -qF "injected git failure" || \
    fail "scenario8: git 自己的 stderr 不得被吞掉，實得 '$TRIGGER_STDERR'"
  echo "  ✓ scenario8: git 失敗（range 合法）⇒ rc=2，且保留 git 的 fatal: 訊息"
}

scenario9_outside_work_tree_is_not_docs_only() {
  local tmp notgit; tmp=$(mktemp -d "${TMPDIR:-/tmp}/atlas-cft-nongit.XXXXXX")
  trap 'rm -rf "$tmp"' RETURN
  notgit="$tmp/plain"; mkdir -p "$notgit"
  run_trigger "$notgit" origin/main...HEAD
  expect_rc 2 "scenario9: 不在 git work tree 內必須 rc=2（不得判 docs-only）"
  echo "  ✓ scenario9: 不在 git work tree ⇒ rc=2"
}

# ── 10：預設 range 與參數處理 ────────────────────────────────────────────

scenario10_default_range_and_usage() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "docs: note" docs/note.md
  run_trigger "$tmp"                      # 無參數 ⇒ 預設 origin/main...HEAD
  expect_rc 1 "scenario10: 無參數時預設 origin/main...HEAD（docs-only ⇒ rc=1）"
  commit_paths "$tmp" "feat(x): go" internal/bar/bar.go
  run_trigger "$tmp"
  expect_rc 0 "scenario10: 無參數時預設 origin/main...HEAD（含 .go ⇒ rc=0）"
  run_trigger "$tmp" a b
  expect_rc 2 "scenario10: 多餘參數必須 rc=2（不得靜默忽略）"
  run_trigger "$tmp" --help
  expect_rc 0 "scenario10: --help ⇒ rc=0"
  echo "  ✓ scenario10: 預設 range / 多參數 rc=2 / --help rc=0"
}

# ── 11：突變自證 —— 舊管線形狀在同一 fixture 上必須是 rc=1（＝本票的假綠）────

scenario11_old_pipeline_shape_is_the_bug() {
  local tmp; tmp=$(new_fixture); trap 'rm -rf "$tmp"' RETURN
  commit_paths "$tmp" "feat(x): go" internal/foo/foo.go
  # 受測的「舊形狀」＝修前兩個呼叫端的逐字形狀（Makefile:46 / .githooks/pre-push:180）：
  #   if git diff --name-only origin/main...HEAD 2>/dev/null | grep -qE '<pattern>'; then …
  # 這裡把 range 換成不可解析的那個，並用本檔從真腳本抽出的 pattern（避免 pattern 漂移）。
  local pattern mut="$tmp/old-pipeline.sh"
  pattern=$(grep -m1 '^CODE_PATTERN=' "$SCRIPT" | sed "s/^CODE_PATTERN=//; s/^'//; s/'$//")
  [ -n "$pattern" ] || fail "scenario11: 抽不到 CODE_PATTERN（腳本結構已變）"
  printf '#!/usr/bin/env bash\nset -euo pipefail\nRANGE="$1"\nCODE_PATTERN=%s\nif git diff --name-only "$RANGE" 2>/dev/null | grep -qE "$CODE_PATTERN"; then\n  exit 0\nfi\nexit 1\n' \
    "'$pattern'" >"$mut"
  local old_rc=0
  ( cd "$tmp" && bash "$mut" "origin/does-not-exist...HEAD" >/dev/null 2>&1 ) || old_rc=$?
  [ "$old_rc" -eq 1 ] || fail "scenario11: 舊管線形狀在同一 fixture 上應為 rc=1（docs-only 假綠），實得 $old_rc"
  run_trigger "$tmp" origin/does-not-exist...HEAD
  expect_rc 2 "scenario11: 同一 fixture 上真腳本必須 rc=2 ⇒ 與舊形狀（rc=1）有鑑別力"
  echo "  ✓ scenario11: 舊管線形狀 rc=1（假綠再現），真腳本 rc=2 ⇒ 斷言有鑑別力"
}

# ── 12：靜態接線守衛 —— 單一實作、不得再長出第二份 pattern ────────────────

scenario12_wiring_and_no_duplicated_pattern() {
  grep -Fq 'scripts/ci/ci-full-trigger.sh' "$MAKEFILE" || \
    fail "scenario12: Makefile 的 pre-push 沒有呼叫唯一判定實作"
  grep -Fq 'scripts/ci/ci-full-trigger.sh' "$HOOK" || \
    fail "scenario12: .githooks/pre-push 沒有呼叫唯一判定實作"
  # 返祖守衛：兩個呼叫端都不得再出現 pattern 或舊管線（否則又會各自漂移）。
  local f
  for f in "$MAKEFILE" "$HOOK"; do
    # 只看可執行行（註解會逐字引用舊形狀來說明歷史，那不是缺陷）。
    if grep -vE '^[[:space:]]*#' "$f" | grep -qF 'cpp|h)$'; then
      fail "scenario12: $f 的可執行行又內嵌了「程式碼變更」pattern（必須只有 ci-full-trigger.sh 一份）"
    fi
    if grep -vE '^[[:space:]]*#' "$f" | grep -qF '2>/dev/null | grep -qE'; then
      fail "scenario12: $f 的可執行行又出現吞錯的舊管線形狀"
    fi
    grep -qF 'origin/main...HEAD' "$f" || fail "scenario12: $f 不再對 origin/main...HEAD 判定（結構已變，請更新本測試）"
  done
  # 呼叫端必須分別處理三態：hook 對「非 0 非 1」要 fail loud。
  grep -qF 'push refused (fail-closed)' "$HOOK" || \
    fail "scenario12: hook 沒有對「無法判定」的 fail-closed 分支"
  echo "  ✓ scenario12: 兩端都呼叫同一支；無重複 pattern／無吞錯管線；hook 有 fail-closed 分支"
}

echo "=== test-ci-full-trigger.sh ==="
scenario1_go_change_runs_ci_full
scenario2_docs_only_skips
scenario3_configs_only_runs_ci_full
scenario4_build_and_ci_inputs_run_ci_full
scenario5_non_code_paths_still_skip
scenario6_unresolvable_range_is_not_docs_only
scenario7_git_missing_is_not_docs_only
scenario8_git_itself_fails_is_not_docs_only
scenario9_outside_work_tree_is_not_docs_only
scenario10_default_range_and_usage
scenario11_old_pipeline_shape_is_the_bug
scenario12_wiring_and_no_duplicated_pattern
echo "✅ test-ci-full-trigger.sh PASSED (12/12)"
