#!/usr/bin/env bash
# test-revert-guard.sh — 「落後分支語意回退」檢查的自我測試（hermetic）
#
# 為什麼要有它：護欄本身必須有牙齒，而且必須**證明它會擋**而不是只會印 PASS。
# 本測試在 tempdir 造真 git repo（真 commit/merge，不連網、不碰本 repo、不碰生產），
# 對 check_revert_guard.py 取正反案例：
#   負向（必須 FAIL/exit≠0）：落後分支回退共用資產、落後分支回退一般檔（--strict）、
#                            整檔刪除共用資產、allowlist 理由空白或 TODO、合成 merge ref、base 解不到
#   正向（必須 PASS/exit=0）：已對齊的 PR、對齊後才刪共用資產、自己刪自己檔、
#                            部分回退（已知抓不到，刻意釘住邊界）、allowlist 有理由、stale 條目
#
# 用法：bash tests/scripts/test-revert-guard.sh
set -uo pipefail

# ⚠️ 本檔會建 throwaway git repo。Git 會把 GIT_DIR（與朋友）export 給 hook，所以
# 「被 hook 呼叫」的那一次，每個 fixture git 命令都會打到**呼叫者的 repo**：2026-09-26
# 的 pre-push（`make ci-gate`）就是這樣把 fixture 的 commit（"base（PR 分岔點）" 等）
# 直接 commit 到 main 上（與 #1927 同一個坑）。先丟掉會選 repo 的環境變數，讓每個 git
# 呼叫只認自己的 cwd；下方 mkrepo 另有 own-repo 斷言，洩漏時直接失敗、絕不 commit。
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK="$ROOT/scripts/ci/check_revert_guard.py"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

command -v git >/dev/null 2>&1 || { echo "❌ 需要 git"; exit 1; }
test -f "$CHECK" || { echo "❌ 找不到 $CHECK"; exit 1; }

# ── fixture：真 git repo ─────────────────────────────────────────────
# 共用資產與一般檔各一，讓「severity 由檔案類別決定」這件事被釘住。
# Physical（symlink 解開）路徑：macOS 的 /var 與 /private/var 是同一處，字串比對會掩蓋洩漏。
physical_path() { ( cd "$1" 2>/dev/null && pwd -P ); }

mkrepo() {
  rm -rf "${TMP}/fx"; mkdir -p "${TMP}/fx"; cd "${TMP}/fx" || exit 1
  git init -q -b main .
  # Own-repo 斷言（#1927 同款）：若 GIT_DIR 洩漏，上面這行不會建出 ${TMP}/fx/.git，
  # 而後面的 fixture commit 會落到呼叫者的分支上。寧可現在大聲失敗，也不要 commit。
  test -d "${TMP}/fx/.git" || { echo "❌ fixture repo 沒有 .git（GIT_DIR 洩漏？）" >&2; exit 1; }
  test "$(git rev-parse --show-toplevel)" = "$(physical_path "${TMP}/fx")" || {
    echo "❌ fixture repo 沒有解析到自己（GIT_DIR 洩漏？）—— 拒絕建立 fixture commit" >&2; exit 1; }
  test "$(physical_path "${TMP}/fx")" != "$(physical_path "${ROOT}")" || {
    echo "❌ fixture repo 解析到呼叫者的 checkout —— 拒絕建立 fixture commit" >&2; exit 1; }
  git config user.email ci@test.invalid; git config user.name ci
  mkdir -p monitoring .github/workflows docs/reference scripts/ci configs internal/foo
  printf 'route:\n  receiver: default\n'                > monitoring/alertmanager.yml
  printf 'name: Quality\n'                              > .github/workflows/quality.yml
  printf '| trap | module |\n|---|---|\n'               > docs/reference/traps.md
  printf '#!/bin/sh\necho ci\n'                         > scripts/ci/check_x.sh
  printf '{"version": 1}\n'                             > configs/parameters.json
  printf '# project\n'                                  > README.md
  printf 'package foo\n'                                > internal/foo/foo.go
  git add -A; git commit -q -m "base（PR 分岔點）"
}

# 落後分支：從 base 開分支 → 只在 internal/foo 加東西 → main 再動別人的檔。
stale_branch() {
  git checkout -q -b feat
  printf 'package foo\n\nfunc Feature() {}\n' > internal/foo/foo.go
  git add -A; git commit -q -m "feat: PR 自己的工作"
  git checkout -q main
  printf 'route:\n  receiver: default\n  group_by: [alertname]\n  repeat_interval: 4h\n' \
      > monitoring/alertmanager.yml
  printf 'name: Quality\non: [pull_request]\njobs:\n  x: {}\n' > .github/workflows/quality.yml
  printf '| trap | module |\n|---|---|\n| new trap | internal/foo |\n' > docs/reference/traps.md
  printf '#!/bin/sh\necho ci-2\necho guard\n'           > scripts/ci/check_x.sh
  printf '{\n  "version": 1,\n  "updated_at": "2026-09-26"\n}\n' > configs/parameters.json
  printf '# project\n\n其他 PR 的說明\n'                > README.md
  git add -A; git commit -q -m "m2: 其他 PR 已合併的改動"
}

run_case() {   # name expected_rc [check args...]
  local name=$1 exp=$2; shift 2
  python3 "$CHECK" "$@" > "${TMP}/out.txt" 2>&1
  local rc=$?
  if [ "${rc}" = "${exp}" ]; then ok "${name}（exit=${rc}）"
  else bad "${name}（exit=${rc}，期望 ${exp}）"; sed 's/^/       /' "${TMP}/out.txt" | head -10; fi
}
expect_out() { # name 'grep pattern'
  if grep -qE "$2" "${TMP}/out.txt"; then ok "$1"
  else bad "$1（輸出找不到：$2）"; sed 's/^/       /' "${TMP}/out.txt" | head -10; fi
}

echo "── 落後分支語意回退檢查自我測試 ──"

# ── 負向：落後分支把他人改動顯示成刪除 ─────────────────────────────
mkrepo; stale_branch
run_case "N1 落後分支回退共用資產 → FAIL" 1 --base main --head feat --quiet
run_case "N1b --json 也 FAIL" 1 --base main --head feat --json
python3 "$CHECK" --base main --head feat --quiet >/dev/null 2>&1
expect_out "N1c 指出監控設定被回退" 'monitoring/alertmanager.yml'
python3 "$CHECK" --base main --head feat --allowlist /dev/null > "${TMP}/out.txt" 2>&1
expect_out "N1d 共用資產標 FAIL" '❌ \[共用資產\] .*alertmanager.yml'
expect_out "N1e 一般檔標 WARN 並列 main 上該檔的 commit" '⚠️  \[其他檔案\] README.md'
expect_out "N1f 給出 update-branch 修法" 'gh pr update-branch'
run_case "N1g --strict 讓 WARN 也紅" 1 --base main --head feat --allowlist /dev/null --strict

# 落後分支刪掉整個共用資產檔（branch_touched_file=True 的分支）
mkrepo; stale_branch; git checkout -q feat; git rm -q scripts/ci/check_x.sh
git add -A; git commit -q -m "feat: 刪掉舊 CI 腳本"
run_case "N2 落後分支刪整個共用資產檔 → FAIL" 1 --base main --head feat --quiet
python3 "$CHECK" --base main --head feat --allowlist /dev/null > "${TMP}/out.txt" 2>&1
expect_out "N2b 指出整檔刪除且本分支動過它" 'file-deleted'

# ── 正向：不該誤報 ───────────────────────────────────────────────
mkrepo; stale_branch
run_case "P1 未對齊但有 allowlist → PASS" 0 --base main --head feat --quiet \
  --allowlist "$ROOT/tests/fixtures/revert-guard/allowlist-valid.json"
python3 "$CHECK" --base main --head feat \
  --allowlist "$ROOT/tests/fixtures/revert-guard/allowlist-valid.json" > "${TMP}/out.txt" 2>&1
expect_out "P1b 印出豁免與理由" 'allowlist\] monitoring/alertmanager.yml'

mkrepo; stale_branch
git checkout -q feat; git merge -q --no-edit main >/dev/null
run_case "P2 已 update-branch → PASS" 0 --base main --head feat --quiet

mkrepo; stale_branch
# 對齊後才刪共用資產（刻意的刪除，不是落後造成的）
git checkout -q feat; git merge -q --no-edit main >/dev/null
rm -f scripts/ci/check_x.sh; git add -A; git commit -q -m "feat: 刪除退役的 CI 腳本"
run_case "P3 對齊後刻意刪共用資產 → PASS（不誤報）" 0 --base main --head feat --quiet

mkrepo
# 分支自己刪自己開的檔（main 沒動過它）
git checkout -q -b feat
rm -f README.md; git add -A; git commit -q -m "feat: 刪掉自己的檔"
run_case "P4 刪自己動過的檔 → PASS" 0 --base main --head feat --quiet

mkrepo
# 部分回退（main 的改動同時新增與刪除 → diff 出現 added>0）：本檢查**已知抓不到**。
# 刻意釘住這條邊界：若哪天規則放寬，這個測試會紅，逼人更新 spec §邊界。
git checkout -q -b feat
printf 'route:\n  receiver: old\n' > monitoring/alertmanager.yml   # 分支也動同一個檔
git add -A; git commit -q -m "feat: 改 receiver"
git checkout -q main
printf 'route:\n  receiver: new\n  group_by: [alertname]\n' > monitoring/alertmanager.yml
git add -A; git commit -q -m "m2: 其他 PR 改 receiver 並加欄位"
run_case "P5 部分回退（混合 hunk）→ 已知抓不到 ⇒ PASS" 0 --base main --head feat --quiet

mkrepo; stale_branch
printf '{\n  "version": 1,\n  "entries": [{"path": "monitoring/*"}]}' > "${TMP}/al-todo.json"
run_case "N3 allowlist 空白理由 → FAIL" 1 --base main --head feat --quiet --allowlist "${TMP}/al-todo.json"
printf '{\n  "version": 1,\n  "entries": [{"path": "monitoring/*", "reason": "TODO: 之後補"}]}' > "${TMP}/al-todo2.json"
run_case "N3b allowlist reason=TODO → FAIL" 1 --base main --head feat --quiet --allowlist "${TMP}/al-todo2.json"

mkrepo; stale_branch
printf '{\n  "version": 1,\n  "entries": [{"path": "no/such/file.md", "reason": "已人工複核：與本 PR 無關"}]}' > "${TMP}/al-stale.json"
git checkout -q feat; git merge -q --no-edit main >/dev/null    # 對齊 → 無違規，只剩 stale 條目
run_case "P6 stale allowlist 條目只警告 → PASS" 0 --base main --head feat --quiet \
  --allowlist "${TMP}/al-stale.json"
python3 "$CHECK" --base main --head feat --allowlist "${TMP}/al-stale.json" > "${TMP}/out.txt" 2>&1
expect_out "P6b 印出 stale 警告" 'allowlist stale'
run_case "P6c 有違規時 stale 條目不會放水 → FAIL" 1 --base main --head feat^ --quiet \
  --allowlist "${TMP}/al-stale.json"

# ── 環境／用法錯誤（fail-closed）─────────────────────────────────
mkrepo; stale_branch
run_case "N4 base ref 解不到 → exit 2（fail-closed）" 2 --base no-such-ref --head feat --quiet
run_case "N4b --allow-missing-base → exit 0 但明說不是通過" 0 --base no-such-ref --head feat --allow-missing-base
python3 "$CHECK" --base no-such-ref --head feat --allow-missing-base > "${TMP}/out.txt" 2>&1
expect_out "N4c 說清楚「這不是通過」" '這不是「通過」'
run_case "N4d head ref 解不到 → exit 2" 2 --base main --head no-such-head --quiet

# 合成 merge ref（refs/pull/<N>/merge 等價）：第一 parent == base tip ⇒ 必須拒絕執行（假綠防線）
mkrepo; stale_branch
git checkout -q --detach main; git merge -q --no-ff -m "synthetic PR merge ref" feat >/dev/null
SYNTH="$(git rev-parse HEAD)"
run_case "N5 合成 merge ref → exit 2（拒絕假綠）" 2 --base main --head "${SYNTH}"
python3 "$CHECK" --base main --head "${SYNTH}" > "${TMP}/out.txt" 2>&1
expect_out "N5b 說明為何拒絕" '假綠'
git checkout -q main

# 真 repo 上的參照案例：把本 session 的實例形態（monitoring/** 純刪除）用 fixture 重演一次
mkrepo; stale_branch
python3 "$CHECK" --base main --head feat --json > "${TMP}/out.json" 2>&1
if python3 - "${TMP}/out.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
paths = {v["path"] for v in d["fail"]}
assert "monitoring/alertmanager.yml" in paths, paths
assert ".github/workflows/quality.yml" in paths, paths
assert {v["path"] for v in d["warn"]} == {"README.md"}, d["warn"]
for v in d["fail"]:
    assert v["main_commit"] and v["main_commit_subject"], v
    assert v["branch_touched_file"] is False, v
PY
then ok "N6 --json 結構完整（fail/warn/path/commit 資訊）"
else bad "N6 --json 結構不完整"; sed 's/^/       /' "${TMP}/out.json" | head -20; fi

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-revert-guard PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-revert-guard FAIL（pass=${pass} fail=${fail}）"
exit 1
