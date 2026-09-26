#!/usr/bin/env bash
# test-revert-guard.sh — 「合併結果驗證 + diff 衛生」閘門的自我測試（hermetic）
#
# 為什麼要有它：護欄本身必須有牙齒，而且必須**證明它會擋**而不是只會印 PASS。
# 本測試在 tempdir 造真 git repo（真 commit/merge，不連網、不碰本 repo、不碰生產），
# 對 check_revert_guard.py 取正反案例：
#   FAIL（必須 exit 1）：**evil merge** — 合併結果丟掉 main 上「本 PR 沒以普通 commit 引進」的內容
#   WARN 不擋（exit 0）：落後分支的假刪除（diff 不誠實，但合併本身不會回退）★ 2026-09-26 重新定性
#   環境錯（exit 2）：base／head 解不到、**新鮮的**合成 merge ref（第一 parent == base tip）
#   正向（exit 0）：已對齊、對齊後刻意刪、刪自己的檔、main 純修改共用資產（R1）、
#                   main rename 共用資產（R4）、過期合成 merge ref（R3）、部分回退（已知抓不到）、
#                   allowlist 有理由、stale 條目、落後分支 + --strict（本機升級成失敗）
#
# 用法：bash tests/scripts/test-revert-guard.sh
set -uo pipefail

# ⚠️ 本檔會建 throwaway git repo。Git 會把 GIT_DIR（與朋友）export 給 hook，所以
# 「被 hook 呼叫」的那一次，每個 fixture git 命令都會打到**呼叫者的 repo**：2026-09-26
# 的 pre-push（`make ci-gate`）就是這樣把 fixture 的 commit（"base（PR 分岔點）" 等）
# 直接 commit 到 main 上（與 #1927 同一個坑）。先丟掉會選 repo 的環境變數，讓每個 git
# 呼叫只認自己的 cwd；下方 mkrepo 另有 own-repo 斷言，洩漏時直接失敗、絕不 commit。
# （GIT_NAMESPACE 是防禦縱深：本 repo 沒用 namespace，但同族的「選 repo」變數。）
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_COMMON_DIR \
      GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_PREFIX GIT_NAMESPACE

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK="$ROOT/scripts/ci/check_revert_guard.py"
TMP="$(mktemp -d)"; trap 'rm -rf "${TMP}"' EXIT
pass=0; fail=0
ok()  { echo "  ✅ $1"; pass=$((pass+1)); }
bad() { echo "  ❌ $1"; fail=$((fail+1)); }

command -v git >/dev/null 2>&1 || { echo "❌ 需要 git"; exit 1; }
test -f "$CHECK" || { echo "❌ 找不到 $CHECK"; exit 1; }
git merge-tree --write-tree HEAD HEAD >/dev/null 2>&1 || {
    echo "⚠️  本機 git 不支援 merge-tree --write-tree（< 2.38）⇒ 檢查會 fail-closed；本測試需要它"; }

# ── fixture：真 git repo ─────────────────────────────────────────────
# 共用資產與一般檔各一，讓「共用資產只在訊息優先序上有意義」這件事被釘住。
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

# PR 自己的工作（分支只動 internal/foo）。
pr_own_work() {
  git checkout -q -b feat
  printf 'package foo\n\nfunc Feature() {}\n' > internal/foo/foo.go
  git add -A; git commit -q -m "feat: PR 自己的工作"
}

# 落後分支：main 再動別人的檔（含共用資產），分支沒有對齊。
stale_branch() {
  pr_own_work
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

# R2：evil merge — 分支在**本地對齊 main**，但解衝突時把 main 的修正丟掉。
# 結果：分支「已對齊」（main 是 feat 的祖先、two-dot diff 看不出落後），但合併它會真的
# 把 main 的修正退回舊版。這是本閘門唯一該 FAIL 的情況，也是舊版閘門的盲區。
evil_merge_branch() {
  pr_own_work
  git checkout -q main
  printf 'route:\n  receiver: default\n  group_by: [alertname]\n  repeat_interval: 4h\n' \
      > monitoring/alertmanager.yml
  git add -A; git commit -q -m "m2: 其他 PR 的監控修正"
  git checkout -q feat
  git merge -q --no-edit main >/dev/null
  # 解衝突解錯的等價物：把 main 的修正從 merge commit 的結果裡拿掉（amend 保留 merge parent）。
  printf 'route:\n  receiver: default\n' > monitoring/alertmanager.yml
  git add -A; git commit -q --amend --no-edit
}

run_case() {   # name expected_rc [check args...]
  local name=$1 exp=$2; shift 2
  python3 "$CHECK" "$@" > "${TMP}/out.txt" 2>&1
  local rc=$?
  if [ "${rc}" = "${exp}" ]; then ok "${name}（exit=${rc}）"
  else bad "${name}（exit=${rc}，期望 ${exp}）"; sed 's/^/       /' "${TMP}/out.txt" | head -12; fi
}
expect_out() { # name 'grep pattern'
  if grep -qE "$2" "${TMP}/out.txt"; then ok "$1"
  else bad "$1（輸出找不到：$2）"; sed 's/^/       /' "${TMP}/out.txt" | head -12; fi
}
expect_no_out() { # name 'grep pattern'
  if grep -qE "$2" "${TMP}/out.txt"; then
    bad "$1（輸出**不該**出現：$2）"; sed 's/^/       /' "${TMP}/out.txt" | head -12
  else ok "$1"; fi
}

echo "── revert-guard 自我測試（合併結果驗證 + diff 衛生）──"

# ── FAIL 相：evil merge（唯一會真的回退 main 的路徑）─────────────────
mkrepo; evil_merge_branch
if git merge-base --is-ancestor main feat; then ok "E0 evil fixture 的 head 已對齊 main（舊閘門會 PASS 的形狀）"
else bad "E0 evil fixture 沒有對齊 main（fixture 壞了）"; fi
run_case "E1 evil-merge head → FAIL" 1 --base main --head feat --quiet
python3 "$CHECK" --base main --head feat > "${TMP}/out.txt" 2>&1
expect_out "E1b 標成 evil merge" '❌ \[evil merge\] monitoring/alertmanager.yml'
expect_out "E1c 附 merge-tree 證據（樹 sha）" '合併結果樹：[0-9a-f]{40}'
expect_out "E1d 指出本 PR 的普通 commit 沒碰過該檔" '普通 commit.*從未動過該檔'
expect_out "E1e 修法指向 gh pr update-branch" 'gh pr update-branch'
run_case "E1f --strict 也 FAIL（本來就 FAIL）" 1 --base main --head feat --strict --quiet
# 這段正是 k3 造出的形狀：對齊後 two-dot diff 仍看得見「刪除」，但舊規則因為
# `rev-list base ^head -- <path>` 為空而放行（唯一真回退的盲區）。釘住 merge-tree 判定。
python3 "$CHECK" --base main --head feat --json > "${TMP}/out.json" 2>&1
if python3 - "${TMP}/out.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
ev = [v for v in d["fail"] if v["check"] == "evil-merge"]
assert len(ev) == 1, d["fail"]
assert ev[0]["path"] == "monitoring/alertmanager.yml", ev[0]
assert len(ev[0]["merge_tree"]) == 40 and ev[0]["merge_base"], ev[0]
assert d["merge"]["checked"] is True and d["merge"]["owned_paths"] >= 1, d["merge"]
assert d["warn"] == [], d["warn"]
PY
then ok "E2 --json：evil-merge 判定與 merge-tree 證據完整"
else bad "E2 --json 結構不完整"; sed 's/^/       /' "${TMP}/out.json" | head -20; fi
# 對照組：同一個 fixture 去掉「解衝突解錯」這一步（正常對齊）⇒ 不得 FAIL。
mkrepo; pr_own_work
git checkout -q main; printf 'route:\n  receiver: default\n  group_by: [x]\n' > monitoring/alertmanager.yml
git add -A; git commit -q -m "m2: 其他 PR 的監控修正"
git checkout -q feat; git merge -q --no-edit main >/dev/null
run_case "E3 正常對齊（同樣的 merge，但沒有解錯衝突）→ PASS" 0 --base main --head feat --quiet

# ── WARN 相：落後分支的假刪除（不擋）────────────────────────────────
mkrepo; stale_branch
run_case "L1 落後分支的假刪除 → WARN 且 exit 0（不擋）" 0 --base main --head feat --quiet
python3 "$CHECK" --base main --head feat > "${TMP}/out.txt" 2>&1
expect_out "L1b 指出監控設定有假刪除" 'monitoring/alertmanager.yml'
expect_out "L1c 共用資產標「共用資產」（訊息優先序）" '假刪除／共用資產\] .*alertmanager.yml'
expect_out "L1d 一般檔標 WARN 並列 main 上該檔的 commit" '假刪除／其他檔案\] README.md'
expect_out "L1e 說清楚這不是回退" '不是.*回退|合併會保留 main 的版本'
expect_out "L1f 給出 update-branch 修法" 'gh pr update-branch'
run_case "L1g --strict 讓 WARN 也紅（本機用）" 1 --base main --head feat --allowlist /dev/null --strict
python3 "$CHECK" --base main --head feat --json > "${TMP}/out.json" 2>&1
if python3 - "${TMP}/out.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
assert d["fail"] == [], d["fail"]
paths = {v["path"]: v for v in d["warn"]}
assert "monitoring/alertmanager.yml" in paths and ".github/workflows/quality.yml" in paths, paths
assert paths["monitoring/alertmanager.yml"]["check"] == "stale-branch-diff-hygiene", paths
assert paths["monitoring/alertmanager.yml"]["shared_asset"] is True, paths
assert paths["README.md"]["shared_asset"] is False, paths
for v in d["warn"]:
    assert v["main_commit"] and v["main_commit_subject"], v
    assert v["branch_touched_file"] is False, v
assert d["merge"]["checked"] is True, d["merge"]
PY
then ok "L1h --json 結構完整（fail 空 / warn 分類 / commit 資訊）"
else bad "L1h --json 結構不完整"; sed 's/^/       /' "${TMP}/out.json" | head -20; fi

# 落後分支刪掉整個共用資產檔（branch_touched_file=True 的分支）⇒ 仍是 WARN，不擋。
mkrepo; stale_branch; git checkout -q feat; git rm -q scripts/ci/check_x.sh
git add -A; git commit -q -m "feat: 刪掉舊 CI 腳本"
run_case "L2 落後分支刪整個共用資產檔 → WARN 且 exit 0" 0 --base main --head feat --quiet
run_case "L2b 同上加 --strict → FAIL（本機嚴格模式）" 1 --base main --head feat --strict --quiet
python3 "$CHECK" --base main --head feat > "${TMP}/out.txt" 2>&1
expect_out "L2c 指出整檔刪除且本分支動過它" 'file-deleted'

# ── R1：main 對共用資產「純修改」（added>0）⇒ 不觸發（釘住邊界）──────
mkrepo; pr_own_work
git checkout -q main
printf 'route:\n  receiver: renamed-default\n' > monitoring/alertmanager.yml   # 1 刪 1 增
git add -A; git commit -q -m "m2: main 改共用資產一行（純修改）"
run_case "R1 main 純修改共用資產（added>0）→ PASS（不進候選）" 0 --base main --head feat --quiet
python3 "$CHECK" --base main --head feat --json > "${TMP}/out.json" 2>&1
if python3 - "${TMP}/out.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1], encoding="utf-8"))
assert d["fail"] == [] and d["warn"] == [], (d["fail"], d["warn"])
# 純修改在 two-dot diff 裡是 added=1 deleted=1（不是純刪除）⇒ 形狀依賴，刻意釘住。
PY
then ok "R1b 落後但 main 只做純修改 ⇒ 完全無 finding（形狀依賴，已釘住）"
else bad "R1b 出現 finding（覆蓋率漂移了）"; sed 's/^/       /' "${TMP}/out.json" | head -20; fi

# ── R3：過期的合成 merge ref（parents[0] 是 base 的祖先但 ≠ base）─────
mkrepo; stale_branch
git checkout -q --detach main~1
git merge -q --no-ff -m "stale synthetic PR merge ref" feat >/dev/null
STALE_SYNTH="$(git rev-parse HEAD)"
git checkout -q feat
run_case "R3 過期合成 merge ref → 不拒絕（exit 0），只加提示" 0 --base main --head "${STALE_SYNTH}" --quiet
python3 "$CHECK" --base main --head "${STALE_SYNTH}" > "${TMP}/out.txt" 2>&1
expect_out "R3b 提示可能是過期的合成 merge ref" '過期的.*合成 merge ref|可能是\*\*過期的\*\*合成'

# ── R4：main 端 rename 共用資產 ⇒ 只 WARN（訊息提示 rename）──────────
mkrepo; stale_branch
git checkout -q main
git mv scripts/ci/check_x.sh scripts/ci/check_guard.sh
git commit -q -m "m2: rename CI 腳本"
run_case "R4 main rename 共用資產 → WARN 且 exit 0（新檔名上報）" 0 --base main --head feat --quiet
python3 "$CHECK" --base main --head feat > "${TMP}/out.txt" 2>&1
expect_out "R4b 在新檔名上報假刪除" 'scripts/ci/check_guard\.sh'
expect_out "R4c 訊息明確提示 main 端 rename" 'main 可能是把該檔 rename'

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
python3 "$CHECK" --base main --head feat > "${TMP}/out.txt" 2>&1
expect_out "P2b 完全沒有 finding" '無 evil merge、無落後假刪除'

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
# 刻意釘住這條邊界：若哪天規則放寬，這個測試會紅，逼人更新 spec §5。
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
git checkout -q feat; git merge -q --no-edit main >/dev/null    # 對齊 → 無 finding，只剩 stale 條目
run_case "P6 stale allowlist 條目只警告 → PASS" 0 --base main --head feat --quiet \
  --allowlist "${TMP}/al-stale.json"
python3 "$CHECK" --base main --head feat --allowlist "${TMP}/al-stale.json" > "${TMP}/out.txt" 2>&1
expect_out "P6b 印出 stale 警告" 'allowlist stale'

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

echo ""
if [ "${fail}" -eq 0 ]; then
  echo "✅ test-revert-guard PASS（${pass} 項）"
  exit 0
fi
echo "❌ test-revert-guard FAIL（pass=${pass} fail=${fail}）"
exit 1
