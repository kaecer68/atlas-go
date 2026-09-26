#!/usr/bin/env python3
"""check_revert_guard.py — 「合併結果驗證 + diff 衛生」閘門（PR 閘門；issue #1993）。

本閘門回答兩個**不同**的問題（2026-09-26 重新定性；k3 設計審查 D10 之後）
-----------------------------------------------------------------------
**【FAIL】合併結果會不會真的弄丟 `main` 的東西（evil merge）？**
用 `git merge-tree --write-tree <base> <head>`（git ≥ 2.38）算出「把這個 PR 併進 base 之後
長怎樣」的樹 T，再 `git diff <base> <T>`。對「本 PR 從未以**普通 commit** 引進變更」的路徑，
合併結果必須與 base 完全相同；只要不同，這次合併就會改掉／刪掉 main 上別人的東西。
會發生這種事的唯一現實路徑就是 **evil merge**：落後分支在本地
`git merge origin/main` 解衝突時解錯（或 `-X ours/theirs` 硬吞），把 main 的修正悄悄丟掉。
此時分支「已對齊 main」、two-dot diff 看起來乾淨，但合併真的把 main 的修正退回舊版。
**修法：`gh pr update-branch <PR>`（GitHub 端三方合併，遇衝突會直接拒絕 ⇒ 不會產生 evil merge）；
本地 merge 遇衝突不要猜。**

**【WARN，不擋】diff 對讀者誠不誠實（落後分支的假刪除）？**
分支落後 `main` 時，`git diff origin/main <PR head>` 會把 main 上**其他 PR 最近的改動顯示成
「刪除」**（`added==0 && deleted>0`／整檔 `D`）：分支的樹還是分岔當時的舊版本。
⚠️ **這不是回退**：對「分支沒動過的檔」，merge commit／`git merge --squash`（＝GitHub 的 squash
按鈕）／rebase 都走三方合併（merge-base 的版本＝分支版本），會**保留** main 的版本。
2026-09-26 以 git 2.52 實測（同款實驗見 spec §7）；真正的危害是**這份 diff 會誤導 review／
agent**——本 session 7 次「看起來在刪別人東西」全是這個假象，而 main 至今仍含所有「會被回退」
的檔案。唯一真的會刪的是**把 two-dot diff 當 patch 套用**：
`git diff <base>..<head> | git apply`（或依這份 diff「手動修正」檔案）。
因此這一相只出 WARN、**不讓 CI 紅燈**：本 repo 已強制 require-up-to-date，共用資產的 FAIL 判定
只是重複既有保護，卻造成 treadmill（main 近 7 日 70 commits、54% 碰共用資產 ⇒ 對齊後數小時又 FAIL）。
`--strict` 保留給本機使用（WARN ⇒ exit 1）。

同一個 session 的實例（全部靠人工 `git diff --numstat` + `git log origin/main -- <file>`
交叉比對才發現，再以 `gh pr update-branch` 修正）：

  #1974          monitoring/alertmanager.yml（−37）
  #1979          .github/workflows/pr-base-guard.yml（−92）、quality.yml（−66）
  #1990          internal/monitoring/metrics_bridge.go（−41）、universe_scheduler.go（−62）
  #1991 / #1994  internal/monitoring/metrics_bridge.go（−41）、metrics_bridge_test.go（−136）、
                 .claude/skills/.../SKILL.md（−41）

判定規則（WARN 相）
-------------------
1. 對 `git diff <base> <head>` 的每個檔案取候選：
   * `--name-status` 是 `D`（整個檔案被刪），或
   * numstat `added == 0 && deleted > 0`（純刪除）。
     （`--no-renames`：rename 的啟發式判定會讓路徑配對不穩，改回刪＋增兩個獨立項。）
2. 候選必須是「落後」造成的：`git rev-list --no-merges <base> ^<head> -- <path>` 非空
   ⇒ main 上該檔有**本分支沒有的**改動 ⇒ 「該檔在 main 的最近一次改動不是本 PR 引進的」。
3. 依「是不是共用資產」排訊息優先序（**不是** severity）：共用資產的假刪除最容易誤導人
   （CI 閘門、監控設定、參數、規範文件、部署 compose），所以先列、標「共用資產」。
   全部都是 WARN；`--strict` 才升級成失敗。

判定規則（FAIL 相：evil merge）
-------------------------------
1. `git merge-tree --write-tree <base> <head>` → 樹 T（衝突時 git 回 rc=1，本檢查只出 WARN
   並列出衝突檔：GitHub 自己也擋衝突，不會靜默落地）。
2. `owned` = `merge-base(base,head)..head` 之間**普通 commit**（`--no-merges`）動過的路徑集合。
   merge commit 的衝突解法**不算** PR 的意圖——那正是要抓的東西。
3. 對 `git diff --name-status --no-renames <base> <T>` 的每個路徑 p：
   p ∉ owned 且 `base:p != T:p` ⇒ **FAIL**（合併會改掉 main 上本 PR 沒碰過的內容）。
   為什麼這是對的：合併的正確語意是「base ＋ 這個 PR 自己的改動」；未被本 PR 的普通 commit
   碰過的路徑，合併結果就必須逐位元等於 base。

allowlist（`scripts/ci/revert-guard-allowlist.json`）
----------------------------------------------------
每筆 = `{"path": <路徑或 glob>, "reason": <必填>, "ticket": <選填>, "added": <選填 YYYY-MM-DD>}`。
`reason` 空白或以 `TODO` 開頭 ⇒ 檢查失敗（`allowlist-reason-missing`）：豁免本身必須有理由。
不再命中的條目只印警告（stale），**不讓 CI 紅燈**（跨 PR 耦合：別人不該因為你忘了刪一行而被擋）。
**allowlist 只能豁免 WARN 相（diff 衛生）；evil merge 是正確性問題，不可豁免**（豁免它等於
允許真的回退 main）。

已知邊界（本檢查**抓不到**什麼，詳見 docs/specs/branch-revert-guard-spec.md）
--------------------------------------------------------------------------
  * **evil merge 落在本 PR 也改過的路徑上**：那條路徑屬於 `owned`，改動可能是「本 PR 真的想這樣改」，
    也可能是 merge commit 解錯——本檢查不猜，交給 review（這也是本閘門最大的盲區）。
  * 同一檔案內的**部分**回退：main 的改動若同時含新增與刪除，diff 會出現 added>0，因此 WARN 相
    不列入候選（本檢查只認「純刪除／整檔刪除」）。
  * 分支「已對齊 main」但內容仍與 main 矛盾（例如 cherry-pick 出不同實作）。
  * 非 git 可見的回退（production 上的檔案、config map、資料庫內容）。
  * 已**刪除**再被落後分支「復活」的檔案（那是 added>0 的方向，不在 WARN 相範圍）。
  * `main` 端 **rename** 共用資產時，落後分支會在**新檔名**上被判「file-deleted」（訊息易誤導，
    但修法相同：update-branch）。
  * 合成 merge ref（`refs/pull/<N>/merge`）：新鮮的會被拒絕（exit 2）；**過期的**合成 ref
    （`parents[0]` 是 base 的祖先但不是 base 本身）不會觸發拒絕，只會多一則 WARN 提示。
  * git < 2.38 沒有 `merge-tree --write-tree` ⇒ 本檢查**fail-closed**（exit 2，不回報通過）。

用法
----
    python3 scripts/ci/check_revert_guard.py                     # base=origin/main, head=HEAD
    python3 scripts/ci/check_revert_guard.py --base main --head HEAD
    python3 scripts/ci/check_revert_guard.py --json
    python3 scripts/ci/check_revert_guard.py --strict            # WARN 也視為失敗（本機用）
exit code: 0 = PASS（可能含 WARN）；1 = 有 FAIL（evil merge 或 allowlist 理由缺失）；2 = 用法／環境錯誤
"""
import argparse
import collections
import json
import os
import re
import subprocess
import sys
import time

DEFAULT_ALLOWLIST = "scripts/ci/revert-guard-allowlist.json"

# 共用資產：**只用來排訊息優先序**（哪些假刪除最容易誤導人），不再決定 severity。
# 刻意用「目錄前綴 + 具名檔 + basename glob」明列，不用 `**` 遞迴 glob：
# 清單要能被人工一眼看完，要擴大範圍必須改這個常數（並在 spec 說明）。
SHARED_DIR_PREFIXES = (
    ".github/",
    "monitoring/",
    "configs/",
    "docs/reference/",
    "scripts/ci/",
)
SHARED_FILES = ("Makefile",)
SHARED_BASENAME_GLOBS = ("docker-compose*.yml", "docker-compose*.yaml")

TODO_MARK = "TODO"
CHECK_ID = "revert-guard"
HYGIENE_ID = "stale-branch-diff-hygiene"
EVIL_ID = "evil-merge"


class GitError(RuntimeError):
    pass


class MergeTreeUnsupported(GitError):
    """本機 git 太舊（< 2.38 沒有 merge-tree --write-tree）⇒ 無法判定 evil merge。"""


def git_full(args, cwd=None):
    """執行 git，回傳 (rc, stdout, stderr)；永不拋錯。"""
    proc = subprocess.run(
        ["git", "-c", "core.quotepath=false"] + list(args),
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    return (proc.returncode,
            proc.stdout.decode("utf-8", "replace"),
            proc.stderr.decode("utf-8", "replace"))


def git(args, cwd=None, check=True):
    """執行 git；回傳 (rc, stdout)。check=True 時非 0 直接拋 GitError。"""
    rc, out, err = git_full(args, cwd=cwd)
    if check and rc != 0:
        raise GitError("git %s 失敗（rc=%d）：%s" % (" ".join(args), rc, err.strip()))
    return rc, out


def try_rev(ref, cwd=None):
    """把 ref 解成 commit sha；解不到回 None（不拋錯）。"""
    rc, out = git(["rev-parse", "--verify", "--quiet", ref + "^{commit}"], cwd=cwd, check=False)
    return out.strip() if rc == 0 and out.strip() else None


def is_shared_asset(path):
    """共用資產（訊息優先序用，不是 severity）。"""
    if path in SHARED_FILES:
        return True
    if any(path.startswith(p) for p in SHARED_DIR_PREFIXES):
        return True
    base = os.path.basename(path)
    for pat in SHARED_BASENAME_GLOBS:
        # basename 比對：`~/.github/workflows/` 之外的 compose 檔（例：
        # docs/operations/docker-compose.prod.yml）也算共用資產。
        if base.startswith(pat.split("*")[0]) and base.endswith(pat.split("*")[-1]):
            return True
    return False


# ── WARN 相：落後分支的假刪除（diff 衛生）───────────────────────────────

def changed_entries(base, head, cwd=None):
    """回傳 {path: {"status": ch, "added": int|None, "deleted": int|None}}。

    --no-renames：rename 判定是啟發式，會讓「路徑」在 diff 兩側對不起來；
    本檢查要逐路徑判讀，寧可看到「A 刪、B 增」兩個獨立項。
    """
    _, ns = git(["diff", "--name-status", "--no-renames", base, head], cwd=cwd)
    entries = collections.OrderedDict()
    for line in ns.splitlines():
        if not line.strip():
            continue
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        ch = parts[0][:1]
        entries[parts[-1]] = {"status": ch, "added": None, "deleted": None}

    _, numstat = git(["diff", "--numstat", "--no-renames", base, head], cwd=cwd)
    for line in numstat.splitlines():
        parts = line.split("\t")
        if len(parts) < 3:
            continue
        path = parts[-1]
        if path not in entries:
            entries[path] = {"status": "M", "added": None, "deleted": None}
        try:
            entries[path]["added"] = int(parts[0])
            entries[path]["deleted"] = int(parts[1])
        except ValueError:
            # 二進位檔：numstat 是 `-\t-\tpath`，無法判「純刪除」；
            # 只有 status=D（整檔刪除）才算候選。
            entries[path]["added"] = None
            entries[path]["deleted"] = None
    return entries


def deletion_candidates(entries):
    """取「純刪除」或「整檔刪除」的候選路徑，並記錄種類。

    刻意不用 `--diff-filter=D` 就交差：純刪除（檔案還在、內容被砍光淨行數）也是同一類
    症狀；反之 `added>0`（分支自己也加了行）會讓判定不確定，寧可放掉（見 spec §5）。
    """
    out = collections.OrderedDict()
    for path, e in entries.items():
        if e["status"] == "D":
            out[path] = "file-deleted"
        elif e["added"] == 0 and (e["deleted"] or 0) > 0:
            out[path] = "pure-deletion"
    return out


def newer_main_commit(base, head, path, cwd=None):
    """main（base）上該檔的最近一次改動中，**本分支沒有**的那一筆。沒有則回 ('', '')。

    `base ^head` = 可由 base 抵達、但 head 沒有的 commit。「該檔在 main 的最近一次改動不是
    本 PR 引進」的機械等價條件就是這裡非空：分支若已含該次改動，diff 就不會是純刪除
    （先含內容，再看見淨刪除＝真的動手刪），因此不會互相矛盾。
    先取 `--no-merges`（回報真正的內容 commit，不報 merge 包裝）；都沒有才放寬。
    """
    for extra in (["--no-merges"], []):
        _, out = git(["rev-list", "-n", "1"] + extra + [base, "^" + head, "--", path],
                     cwd=cwd, check=False)
        sha = out.strip()
        if sha:
            return sha
    return ""


def commit_info(sha, cwd=None):
    """回傳 (short_sha, iso_date, subject)。"""
    _, out = git(["show", "-s", "--format=%h%x00%ad%x00%s", "--date=short", sha],
                 cwd=cwd, check=False)
    parts = out.strip("\n").split("\x00")
    while len(parts) < 3:
        parts.append("")
    return parts[0], parts[1], parts[2]


def unchanged_since_fork(mb, head, path, cwd=None):
    """該路徑在本分支上是否**自 merge-base 以來沒被動過**（blob 相同）。

    main 端 rename 的偵測用：rename 後，落後分支還留著**舊檔名**（未動過），而舊檔名在
    base 上不存在 ⇒ diff 會把它算成「新增」。只有「自 fork 以來沒動過」的舊檔名才算證據；
    本 PR 自己新增的檔案（merge-base 上不存在）不算。
    """
    if not mb:
        return False
    _, a = git(["rev-parse", "--verify", "--quiet", mb + ":" + path], cwd=cwd, check=False)
    _, b = git(["rev-parse", "--verify", "--quiet", head + ":" + path], cwd=cwd, check=False)
    return bool(a.strip()) and a.strip() == b.strip()


def branch_touched_file(base, head, path, cwd=None):
    """本分支是否動過該檔（以 merge-base 的 blob 對照 head 的 blob）。

    這是**訊息用的信心訊號**，不影響 severity：
      False ⇒ 分支根本沒碰這個檔，diff 裡的刪除 100% 是 main 上別人近期的改動（經典案例）。
      True  ⇒ 分支動過（例如真的刪掉該檔、或改同一個檔別的段落）⇒ 提示「可能是刻意的刪除」。
    """
    _, mb = git(["merge-base", base, head], cwd=cwd, check=False)
    mb = mb.strip()
    if not mb:
        return True
    _, a = git(["rev-parse", "--verify", "--quiet", mb + ":" + path], cwd=cwd, check=False)
    _, b = git(["rev-parse", "--verify", "--quiet", head + ":" + path], cwd=cwd, check=False)
    return a.strip() != b.strip()


# ── FAIL 相：evil merge（合併結果驗證）─────────────────────────────────

def ordinary_paths(base, head, cwd=None):
    """回傳 (owned, mb)：`mb..head` 之間**普通 commit**（--no-merges）動過的路徑集合。

    這是本閘門的意圖判準：普通 commit 才算本 PR 的意圖；merge commit 的衝突解法不算
    ——那正是「解衝突解錯」會留下痕跡的地方。
    """
    _, mb = git(["merge-base", base, head], cwd=cwd, check=False)
    mb = mb.strip()
    if not mb:
        return set(), ""
    _, out = git(["log", "--no-merges", "--pretty=format:", "--name-only",
                  "%s..%s" % (mb, head)], cwd=cwd, check=False)
    owned = set()
    for line in out.splitlines():
        line = line.strip()
        if line:
            owned.add(line)
    return owned, mb


CONFLICT_LINE = re.compile(r"^[0-7]{6} [0-9a-f]{40} [123]\t(.+)$")
UNSUPPORTED_HINTS = ("usage: git merge-tree", "unknown option", "unrecognized option")


def merge_tree(base, head, cwd=None):
    """跑 `git merge-tree --write-tree <base> <head>`。

    回傳 dict：tree（sha 或 ""）、conflicts（衝突路徑清單）、unsupported（bool）。
    git < 2.38 沒有 --write-tree ⇒ 拋 MergeTreeUnsupported（呼叫端 fail-closed）。
    附註：merge-tree 會把結果樹寫進 object DB（loose object），但**不動任何 ref、不動工作樹**。
    """
    rc, out, err = git_full(["merge-tree", "--write-tree", base, head], cwd=cwd)
    if rc != 0 and any(h in err for h in UNSUPPORTED_HINTS):
        raise MergeTreeUnsupported(err.strip())
    lines = out.splitlines()
    tree = lines[0].strip() if lines and re.fullmatch(r"[0-9a-f]{40}", lines[0].strip()) else ""
    conflicts = []
    for line in lines:
        m = CONFLICT_LINE.match(line)
        if m:
            conflicts.append(m.group(1))
    if rc != 0 and not tree:
        raise GitError("git merge-tree --write-tree 沒有回傳樹（rc=%d）：%s" % (rc, err.strip()))
    return {"tree": tree, "conflicts": sorted(set(conflicts))}


def evaluate_evil_merge(base, head, cwd=None):
    """回傳 (fails, warns, info)：fails = 合併會改掉 main 上本 PR 沒碰過的內容。

    info 內含 merge-tree 的樹 sha 與 owned 路徑數，供 JSON／訊息當證據。
    """
    info = {"tree": "", "conflicts": [], "owned_paths": 0, "merge_base": "",
            "checked": False, "skipped_reason": ""}
    fails, warns = [], []

    try:
        mr = merge_tree(base, head, cwd=cwd)
    except MergeTreeUnsupported as exc:
        raise MergeTreeUnsupported(str(exc))
    info["tree"] = mr["tree"]
    info["conflicts"] = mr["conflicts"]

    if mr["conflicts"]:
        # 衝突 ⇒ GitHub 自己就會擋，不會靜默落地。這裡只出 WARN（並說明為什麼不算 FAIL）。
        info["skipped_reason"] = "merge-conflict"
        warns.append({
            "check": HYGIENE_ID,
            "kind": "merge-conflict",
            "path": ", ".join(mr["conflicts"]),
            "severity": "WARN",
            "shared_asset": any(is_shared_asset(p) for p in mr["conflicts"]),
            "merge_tree": mr["tree"],
            "message": "把本 PR 併進 base 會產生衝突 ⇒ GitHub 自己會擋（不會靜默落地）；"
                       "請用 gh pr update-branch（遇衝突會拒絕）或本地解衝突後再確認",
        })
        return fails, warns, info

    owned, mb = ordinary_paths(base, head, cwd=cwd)
    info["owned_paths"] = len(owned)
    info["merge_base"] = mb
    info["checked"] = True

    _, ns = git(["diff", "--name-status", "--no-renames", base, mr["tree"]], cwd=cwd, check=False)
    _, numstat = git(["diff", "--numstat", "--no-renames", base, mr["tree"]], cwd=cwd, check=False)
    stats = {}
    for line in numstat.splitlines():
        parts = line.split("\t")
        if len(parts) >= 3:
            stats[parts[-1]] = (parts[0], parts[1])

    for line in ns.splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        status, path = parts[0][:1], parts[-1]
        if path in owned:
            continue                      # 本 PR 的普通 commit 動過 ⇒ 那是它的意圖（見 spec §5 盲區）
        kind = {"D": "merge-deletes", "A": "merge-adds", "M": "merge-modifies"}.get(status, "merge-modifies")
        added, deleted = stats.get(path, ("?", "?"))
        fails.append({
            "check": EVIL_ID,
            "path": path,
            "kind": kind,
            "severity": "FAIL",
            "shared_asset": is_shared_asset(path),
            "added": added,
            "deleted": deleted,
            "merge_tree": mr["tree"],
            "merge_base": mb,
            "message": "合併結果（merge-tree %s）與 base 在該路徑不同，但本 PR 的普通 commit "
                       "從未碰過它 ⇒ 這個改動只可能來自 merge commit 的衝突解法（evil merge）"
                       % mr["tree"],
        })
    return fails, warns, info


# ── allowlist ─────────────────────────────────────────────────────────

def load_allowlist(path):
    """回傳 (entries, error)：entries = [{"path","reason","ticket","added"}]。"""
    if not os.path.exists(path):
        return [], None
    try:
        with open(path, "r", encoding="utf-8") as fh:
            raw = fh.read()
    except OSError as exc:
        return [], "allowlist 讀取失敗（%s）：%s" % (path, exc)
    if not raw.strip():
        return [], None                    # 空檔 = 沒有豁免（例：`--allowlist /dev/null`）
    try:
        data = json.loads(raw)
    except ValueError as exc:
        return [], "allowlist 解析失敗（%s）：%s" % (path, exc)
    entries = data.get("entries")
    if not isinstance(entries, list):
        return [], "allowlist 格式錯誤：%s 的 `entries` 必須是陣列" % path
    for item in entries:
        if not isinstance(item, dict) or "path" not in item:
            return [], "allowlist 格式錯誤：每筆條目必須是含 `path` 的物件（%s）" % path
    return entries, None


def allowlist_match(entries, path):
    import fnmatch
    for e in entries:
        pattern = str(e.get("path", ""))
        if not pattern:
            continue
        if pattern == path or fnmatch.fnmatch(path, pattern):
            return e
    return None


def reason_is_missing(entry):
    reason = str(entry.get("reason", "")).strip()
    return (not reason) or reason.upper().startswith(TODO_MARK)


# ── 評估 + 輸出 ────────────────────────────────────────────────────────

def evaluate_hygiene(base, head, cwd, entries):
    """WARN 相：落後分支造成的假刪除。回傳 (warns, exempted, stale)。"""
    changed = changed_entries(base, head, cwd=cwd)
    candidates = deletion_candidates(changed)
    # main 端 rename 的偵測：rename 後，落後分支在**新檔名**上會看到整檔刪除，同時在
    # 舊檔名上看到「新增」。那不是真的刪東西，訊息必須講清楚（否則讀者會被新檔名誤導）。
    added_paths = sorted(p for p, e in changed.items() if e["status"] == "A")
    _, mb = git(["merge-base", base, head], cwd=cwd, check=False)
    mb = mb.strip()
    warns, exempted, used = [], [], set()

    for path, kind in candidates.items():
        main_commit = newer_main_commit(base, head, path, cwd=cwd)
        if not main_commit:
            continue                       # 分支已含 main 對該檔的最近改動 ⇒ 是真刪除
        entry = allowlist_match(entries, path)
        if entry is not None:
            used.add(id(entry))
            exempted.append((path, entry))
            continue
        short, date, subject = commit_info(main_commit, cwd=cwd)
        touched = branch_touched_file(base, head, path, cwd=cwd)
        warns.append({
            "check": HYGIENE_ID,
            "path": path,
            "kind": kind,
            "severity": "WARN",            # 落後分支的假刪除**不再 FAIL**（見檔頭與 spec §1）
            "possible_rename_from": ([q for q in added_paths[:20]
                                      if unchanged_since_fork(mb, head, q, cwd=cwd)]
                                     if kind == "file-deleted" else []),
            "shared_asset": is_shared_asset(path),
            "added": changed[path]["added"],
            "deleted": changed[path]["deleted"],
            "main_commit": short,
            "main_commit_full": main_commit,
            "main_commit_date": date,
            "main_commit_subject": subject,
            "branch_touched_file": touched,
        })
    # 共用資產先列（最容易誤導人），其餘照路徑排序，輸出穩定。
    warns.sort(key=lambda v: (not v["shared_asset"], v["path"]))
    stale = [e for e in entries if id(e) not in used]
    return warns, exempted, stale


def _delta(v):
    added = "?" if v.get("added") is None else v.get("added")
    deleted = "?" if v.get("deleted") is None else v.get("deleted")
    return "added=%s deleted=%s" % (added, deleted)


def render_text(out, base, head, fails, warns, exempted, stale, notices, info, elapsed):
    out("revert-guard: base=%s head=%s" % (base, head))
    out("  FAIL 判準：evil merge — 合併結果（git merge-tree --write-tree）改掉 main 上"
        "本 PR 普通 commit 從未碰過的內容")
    out("  WARN 判準：落後分支造成的假刪除（diff 不誠實，**合併本身不會回退**；"
        "共用資產優先列出）")

    for v in fails:
        out("  ❌ [evil merge] %s（%s，%s）" % (v["path"], v["kind"], _delta(v)))
        out("      合併結果樹：%s（merge-base %s）與 base 在該路徑不同"
            % (v["merge_tree"], v["merge_base"]))
        out("      本 PR 的普通 commit（merge-base..head，--no-merges）從未動過該檔 ⇒ "
            "這個改動只可能來自 merge commit 的衝突解法。")
        out("      這是唯一會真的回退 main 的路徑。修法：gh pr update-branch <PR>"
            "（遇衝突會拒絕）；本地 merge 遇衝突不要猜、不要用 -X ours/theirs。")
    for v in warns:
        if v["kind"] == "merge-conflict":
            out("  ⚠️  [合併衝突] %s：%s" % (v["path"], v.get("message", "")))
            continue
        label = "共用資產" if v["shared_asset"] else "其他檔案"
        out("  ⚠️  [落後造成的假刪除／%s] %s（%s，%s 行）"
            % (label, v["path"], v["kind"], _delta(v)))
        out("      main 上該檔最近的改動：%s %s %s"
            % (v["main_commit"], v["main_commit_date"], v["main_commit_subject"]))
        if v["branch_touched_file"]:
            out("      本分支動過該檔 ⇒ 可能是刻意的刪除；對齊後再確認一次。")
        else:
            out("      本分支沒有動過該檔 ⇒ 這個「刪除」是落後造成的 diff 假象，"
                "合併會保留 main 的版本（不是回退）。")
        if v.get("possible_rename_from"):
            out("      注意：本分支還留著自 fork 以來沒動過的舊路徑 %s ⇒ "
                "main 可能是把該檔 rename（這裡報的是新檔名，語意上沒有東西被刪）。"
                % ", ".join(v["possible_rename_from"][:5]))
    for path, entry in exempted:
        out("  ↷  [allowlist] %s — %s" % (path, str(entry.get("reason", "")).strip()))
    for entry in stale:
        out("  ℹ️  [allowlist stale] %s 已不再命中（可刪；不讓 CI 紅燈）"
            % entry.get("path"))
    for n in notices:
        out("  ℹ️  %s" % n)

    if fails:
        out("")
        out("為什麼這是 FAIL：合併的正確語意是「base ＋ 這個 PR 自己的改動」。未被本 PR 的普通 commit"
            " 碰過的路徑，合併結果必須與 base 完全相同；不同就是有人把 main 的內容弄丟了。")
        out("怎麼修（依序試）：")
        out("  1. gh pr update-branch <PR>（GitHub 端三方合併；遇衝突會直接拒絕 ⇒ 不會產生 evil merge）")
        out("  2. 本地等效：git fetch origin main && git merge origin/main")
        out("     ⚠️ 解衝突時不要猜、不要用 -X ours/theirs 硬吞：解錯＝evil merge，"
            "main 的修正會被悄悄丟掉。")
        out("  3. evil merge 不可用 allowlist 豁免（那是正確性問題，不是 diff 衛生）。")
        out("  規格：docs/specs/branch-revert-guard-spec.md")
    elif warns:
        out("")
        out("這些是「diff 會誤導 review／agent」的假刪除，**不是**回退（合併會保留 main 的版本）。")
        out("仍然建議對齊，讓 diff 誠實：")
        out("  1. gh pr update-branch <PR>")
        out("     本地等效：git fetch origin main && git merge origin/main（遇衝突不要猜）")
        out("  2. 若本 PR 真的要刪除該檔：先 update-branch，再把刪除重新套用一次。")
        out("  3. 例外才登 %s（reason 必填；只豁免 WARN 相）。" % DEFAULT_ALLOWLIST)
        out("  規格：docs/specs/branch-revert-guard-spec.md")
    out("")
    if fails:
        out("❌ revert-guard FAIL（evil merge %d 筆 / 假刪除 WARN %d 筆；耗時 %.2fs）"
            % (len(fails), len(warns), elapsed))
    elif warns:
        out("⚠️  revert-guard PASS（含 %d 筆 WARN；未偵測到 evil merge；耗時 %.2fs）"
            % (len(warns), elapsed))
    else:
        out("✅ revert-guard PASS（無 evil merge、無落後假刪除；耗時 %.2fs）" % elapsed)


def build_parser():
    ap = argparse.ArgumentParser(description="合併結果驗證 + diff 衛生閘門（issue #1993）")
    ap.add_argument("--base", default="origin/main",
                    help="基準 ref（預設 origin/main；CI 傳 base 分支）")
    ap.add_argument("--head", default="HEAD",
                    help="PR head ref（預設 HEAD）。⚠️ 不要傳 GitHub 的合成 merge ref "
                         "refs/pull/<N>/merge：那個 commit 已含 main，會讓檢查永遠 PASS（假綠）")
    ap.add_argument("--allowlist", default="",
                    help="allowlist JSON（預設 <repo>/scripts/ci/revert-guard-allowlist.json）")
    ap.add_argument("--strict", action="store_true",
                    help="WARN（落後假刪除）也視為失敗（exit 1）——本機用；CI 不用")
    ap.add_argument("--allow-missing-base", action="store_true",
                    help="base ref 解不到時只警告並 exit 0（給無 remote 的環境；CI 不使用）")
    ap.add_argument("--json", action="store_true", help="以 JSON 輸出（機器可讀）")
    ap.add_argument("--quiet", action="store_true", help="只印總結一行")
    return ap


def json_dump(payload):
    print(json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True))


def main(argv=None):
    args = build_parser().parse_args(argv)
    t0 = time.time()
    repo_root = os.path.abspath(os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                             "..", ".."))

    rc, _ = git(["rev-parse", "--git-dir"], check=False)
    if rc != 0:
        print("❌ revert-guard：這裡不是 git 工作樹（本檢查需要 git 歷史）")
        return 2

    base = args.base
    head = args.head
    base_sha = try_rev(base)
    if base_sha is None:
        if args.allow_missing_base:
            print("⚠️  revert-guard SKIPPED：base ref 解不到（%s）—— --allow-missing-base 生效" % base)
            print("   ⚠️  這不是「通過」：沒有基準就無法證明分支沒有回退他人改動。")
            return 0
        print("❌ revert-guard：base ref 解不到（%s）。" % base)
        print("   fail-closed：無法決定基準時**不**回報通過（靜默跳過＝假綠）。")
        print("   CI：先 `git fetch origin <base_branch>`；無 remote 的環境可用 --allow-missing-base。")
        return 2
    head_sha = try_rev(head)
    if head_sha is None:
        print("❌ revert-guard：head ref 解不到（%s）。" % head)
        print("   CI：checkout 後傳真正的 PR head（refs/remotes/origin/pr-head），不要傳合成 merge ref。")
        return 2

    # 合成 merge ref 的假綠防線：refs/pull/<N>/merge 的第一個 parent 就是 base tip，
    # 它的樹已經含 main ⇒ diff 永不含「落後的假刪除」⇒ 檢查會永遠 PASS。
    notices = []
    _, parents = git(["rev-list", "--parents", "-n", "1", head_sha], check=False)
    toks = parents.split()
    if len(toks) > 2 and toks[1] == base_sha:
        print("❌ revert-guard：--head 指向合成 merge commit（第一 parent == base tip）。")
        print("   這個 commit 已經含 main ⇒ 檢查會永遠 PASS（假綠），因此拒絕執行。")
        print("   請傳真正的 PR head（CI：`git fetch origin +refs/pull/<N>/head:refs/remotes/origin/pr-head`）。")
        return 2
    if len(toks) > 2:
        # 過期的合成 merge ref：parents[0] 是 base 的祖先但 != base（GitHub 的 merge ref 會隨
        # base 前進而變舊）。不是假綠（樹裡沒有**現在**的 base），但值得提醒餵錯 ref。
        rc, _ = git(["merge-base", "--is-ancestor", toks[1], base_sha], check=False)
        if rc == 0:
            notices.append(
                "--head 的第一個 parent 是 base 的祖先但不是 base 本身（%s）⇒ 可能是**過期的**"
                "合成 merge ref（refs/pull/<N>/merge 會隨 base 前進而變舊）。"
                "CI 請傳真正的 PR head。" % toks[1][:8])

    allowlist_path = args.allowlist or os.path.join(repo_root, DEFAULT_ALLOWLIST)
    entries, err = load_allowlist(allowlist_path)
    if err:
        print("❌ revert-guard：%s" % err)
        return 2

    bad_reason = [e for e in entries if reason_is_missing(e)]
    warns, exempted, stale = evaluate_hygiene(base, head, None, entries)
    try:
        fails, phase_warns, info = evaluate_evil_merge(base, head, None)
    except MergeTreeUnsupported as exc:
        print("❌ revert-guard：本機 git 不支援 `merge-tree --write-tree`（需要 git ≥ 2.38）。")
        print("   %s" % str(exc).splitlines()[0])
        print("   fail-closed：無法判定 evil merge 時**不**回報通過（這個判定是唯一的 FAIL 相）。")
        return 2
    warns.extend(phase_warns)
    elapsed = time.time() - t0

    if args.json:
        json_dump({
            "check": CHECK_ID,
            "base": {"ref": base, "sha": base_sha},
            "head": {"ref": head, "sha": head_sha},
            "allowlist": allowlist_path,
            "fail": fails,
            "warn": warns,
            "notices": notices,
            "merge": info,
            "allowlisted": [{"path": p, "reason": str(e.get("reason", ""))} for p, e in exempted],
            "stale_allowlist": [e.get("path") for e in stale],
            "allowlist_reason_missing": [e.get("path") for e in bad_reason],
            "shared_asset_dirs": list(SHARED_DIR_PREFIXES),
            "shared_asset_files": list(SHARED_FILES),
            "shared_asset_globs": list(SHARED_BASENAME_GLOBS),
            "elapsed_sec": round(elapsed, 2),
        })
    else:
        if args.quiet:
            print("revert-guard: FAIL=%d WARN=%d allowlisted=%d evil_merge_checked=%s elapsed=%.2fs"
                  % (len(fails), len(warns), len(exempted), info["checked"], elapsed))
        else:
            render_text(print, base, head, fails, warns, exempted, stale, notices, info, elapsed)

    if bad_reason:
        print("❌ revert-guard：allowlist 有未填理由的條目（理由必填；TODO 佔位不算理由）：")
        for e in bad_reason:
            print("   • %s" % e.get("path"))
        print("   請在 %s 逐筆寫上 reason（要寫到別人能自行驗證）。" % DEFAULT_ALLOWLIST)
        return 1
    if fails:
        return 1
    if warns and args.strict:
        print("❌ revert-guard --strict：WARN 視為失敗（%d 筆）" % len(warns))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
