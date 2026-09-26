#!/usr/bin/env python3
"""check_revert_guard.py — 「落後分支造成的語意回退」靜態檢查（PR 閘門）。

為什麼要有它（issue #1993；2026-09-26 單一 session 內實證 5 個 PR / 6 次）
------------------------------------------------------------------------
PR 分支落後 main 時，`gh pr view` 顯示 `MERGEABLE`（**沒有文字衝突**），但
`git diff origin/main <PR head>` 會把 main 上**其他 PR 最近的改動顯示成「刪除」**：
分支的樹還是舊的，照現狀合併就會把別人已合併的修正**回退掉**。GitHub 的
合併性檢查（mergeable）只回答「能不能自動合併」，不回答「會不會刪掉別人的東西」。

同一個 session 的實例（全部靠人工 `git diff --numstat` + `git log origin/main -- <file>`
交叉比對才發現，再以 `gh pr update-branch` 修正）：

  #1974          monitoring/alertmanager.yml（−37）
  #1979          .github/workflows/pr-base-guard.yml（−92）、quality.yml（−66）
  #1990          internal/monitoring/metrics_bridge.go（−41）、universe_scheduler.go（−62）
  #1991 / #1994  internal/monitoring/metrics_bridge.go（−41）、metrics_bridge_test.go（−136）、
                 .claude/skills/.../SKILL.md（−41）

人力不可靠（每次都要人工跑兩條 git 指令再逐檔判讀），本檢查把它自動化。

判定規則（R1）
--------------
1. 對 `git diff <base> <head>` 的每個檔案取候選：
   * `--name-status` 是 `D`（整個檔案被刪），或
   * numstat `added == 0 && deleted > 0`（純刪除）。
     （`--no-renames`：rename 的啟發式判定會讓路徑配對不穩，改回刪＋增兩個獨立項。）
2. 候選必須是「落後」造成的：`git rev-list --no-merges <base> ^<head> -- <path>` 非空
   ⇒ main 上該檔有**本分支沒有的**改動 ⇒ 「該檔在 main 的最近一次改動不是本 PR 引進的」。
   本分支若已含 main 的該次改動（或內容等效），diff 就不會是純刪除，因此不會誤報。
3. 依檔案類別分級（severity 由**檔案是不是共用資產**決定，不是由改動大小）：
   * 共用資產（`.github/`、`monitoring/`、`configs/`、`docs/reference/`、`Makefile`、
     `docker-compose*.yml`、`scripts/ci/`）→ **FAIL**（exit 1）。
     理由：這些檔案是「唯一真相」或跨 PR／跨機器的介面，被落後分支回退等於**靜默改設定或
     改閘門**，代價遠高於一般程式碼。
   * 其餘 → **WARN**（不讓 CI 紅燈，但印出檔案、main 上該檔最後的 commit、修法）。

修法（兩條，理由都要能自我驗證）
--------------------------------
  A. `gh pr update-branch <PR>`（或本地 `git fetch origin main && git merge origin/main`）
     → 重跑本檢查；這是正解，改完 diff 就不再有假刪除。
  B. 若**本 PR 真的想刪**該檔（例如刪掉退役的設定），但 main 上它又有新改動：
     先 update-branch，再重新套用你的刪除。真的必須豁免時才登 allowlist（`reason` 必填）。

allowlist（`scripts/ci/revert-guard-allowlist.json`）
----------------------------------------------------
每筆 = `{"path": <路徑或 glob>, "reason": <必填>, "ticket": <選填>, "added": <選填 YYYY-MM-DD>}`。
`reason` 空白或以 `TODO` 開頭 ⇒ 檢查失敗（`allowlist-reason-missing`）：豁免本身必須有理由。
不再命中的條目只印警告（stale），**不讓 CI 紅燈**（跨 PR 耦合：別人不該因為你忘了刪一行而被擋）。

已知邊界（本檢查**抓不到**什麼，詳見 docs/specs/branch-revert-guard-spec.md）
--------------------------------------------------------------------------
  * 同一檔案內的**部分**回退：main 的改動若同時含新增與刪除，diff 會出現 added>0，
    因此不列入候選（本檢查只認「純刪除／整檔刪除」）。
  * 分支「已對齊 main」但內容仍與 main 矛盾（例如 cherry-pick 出不同實作）。
  * 非 git 可見的回退（production 上的檔案、config map、資料庫內容）。
  * 已**刪除**再被落後分支「復活」的檔案（那是 added>0 的方向，不在本檢查範圍）。
  * 分支自己就是 merge commit 的合成 ref（`refs/pull/<N>/merge`）：見 --head 的說明。

用法
----
    python3 scripts/ci/check_revert_guard.py                     # base=origin/main, head=HEAD
    python3 scripts/ci/check_revert_guard.py --base main --head HEAD
    python3 scripts/ci/check_revert_guard.py --json
    python3 scripts/ci/check_revert_guard.py --strict            # WARN 也視為失敗
exit code: 0 = PASS（可能含 WARN）；1 = 有 FAIL（或 allowlist 理由缺失）；2 = 用法／環境錯誤
"""
import argparse
import collections
import json
import os
import subprocess
import sys
import time

DEFAULT_ALLOWLIST = "scripts/ci/revert-guard-allowlist.json"

# 共用資產：落後分支回退這些檔案 ⇒ FAIL。
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
CHECK_ID = "stale-branch-revert"


class GitError(RuntimeError):
    pass


def git(args, cwd=None, check=True):
    """執行 git；回傳 (rc, stdout)。check=True 時非 0 直接拋 GitError。"""
    proc = subprocess.run(
        ["git", "-c", "core.quotepath=false"] + list(args),
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    out = proc.stdout.decode("utf-8", "replace")
    if check and proc.returncode != 0:
        raise GitError("git %s 失敗（rc=%d）：%s"
                       % (" ".join(args), proc.returncode,
                          proc.stderr.decode("utf-8", "replace").strip()))
    return proc.returncode, out


def try_rev(ref, cwd=None):
    """把 ref 解成 commit sha；解不到回 None（不拋錯）。"""
    rc, out = git(["rev-parse", "--verify", "--quiet", ref + "^{commit}"], cwd=cwd, check=False)
    return out.strip() if rc == 0 and out.strip() else None


def is_shared_asset(path):
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
    症狀；反之 `added>0`（分支自己也加了行）會讓判定不確定，寧可放掉（見 spec §邊界）。
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


def branch_touched_file(base, head, path, cwd=None):
    """本分支是否動過該檔（以 merge-base 的 blob 對照 head 的 blob）。

    這是**訊息用的信心訊號**，不影響 severity：
      False ⇒ 分支根本沒碰這個檔，diff 裡的刪除 100% 是 main 上別人近期的改動（經典案例）。
      True  ⇒ 分支動過（例如真的刪掉該檔、或改同一個檔別的段落）⇒ 建議先 update-branch 再確認。
    """
    _, mb = git(["merge-base", base, head], cwd=cwd, check=False)
    mb = mb.strip()
    if not mb:
        return True
    _, a = git(["rev-parse", "--verify", "--quiet", mb + ":" + path], cwd=cwd, check=False)
    _, b = git(["rev-parse", "--verify", "--quiet", head + ":" + path], cwd=cwd, check=False)
    return a.strip() != b.strip()


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


def evaluate_violations(base, head, cwd, entries):
    """回傳 (violations, exempted, stale)。violations 每筆含 severity／main commit 資訊。"""
    changed = changed_entries(base, head, cwd=cwd)
    candidates = deletion_candidates(changed)
    violations, exempted, used = [], [], set()

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
        violations.append({
            "check": CHECK_ID,
            "path": path,
            "kind": kind,
            "severity": "FAIL" if is_shared_asset(path) else "WARN",
            "added": changed[path]["added"],
            "deleted": changed[path]["deleted"],
            "main_commit": short,
            "main_commit_full": main_commit,
            "main_commit_date": date,
            "main_commit_subject": subject,
            "branch_touched_file": touched,
        })
    stale = [e for e in entries if id(e) not in used]
    return violations, exempted, stale


def render_text(out, base, head, violations, exempted, stale, elapsed):
    fails = [v for v in violations if v["severity"] == "FAIL"]
    warns = [v for v in violations if v["severity"] == "WARN"]
    out("revert-guard: base=%s head=%s" % (base, head))
    out("  共用資產（回退即 FAIL）：%s + %s + %s"
        % (", ".join(SHARED_DIR_PREFIXES), ", ".join(SHARED_FILES),
           ", ".join(SHARED_BASENAME_GLOBS)))
    for v in fails:
        out("  ❌ [共用資產] %s（%s，%s 行）" % (v["path"], v["kind"], _delta(v)))
        _detail(out, v)
    for v in warns:
        out("  ⚠️  [其他檔案] %s（%s，%s 行）" % (v["path"], v["kind"], _delta(v)))
        _detail(out, v)
    for path, entry in exempted:
        out("  ↷  [allowlist] %s — %s" % (path, str(entry.get("reason", "")).strip()))
    for entry in stale:
        out("  ℹ️  [allowlist stale] %s 已不再命中（可刪；不讓 CI 紅燈）"
            % entry.get("path"))
    if violations:
        out("")
        out("為什麼這是問題：GitHub 的 MERGEABLE 只保證「能自動合併」，不保證「不刪掉別人的東西」。")
        out("  分支落後 main 時，diff 把 main 上其他 PR 的改動顯示成刪除 ⇒ 照現狀合併就是回退。")
        out("怎麼修（依序試）：")
        out("  1. PR 分支：gh pr update-branch <PR>")
        out("     本地等效：git fetch origin main && git merge origin/main（或 git pull --ff-only）")
        out("     然後重跑本檢查；改完就不會再有假刪除。")
        out("  2. 若本 PR 真的要刪除該檔：先 update-branch，再把刪除重新套用一次。")
        out("  3. 例外才登 %s（reason 必填，寫到別人能自行驗證）。" % DEFAULT_ALLOWLIST)
        out("  規格：docs/specs/branch-revert-guard-spec.md")
    out("")
    summary = "❌ revert-guard FAIL（共用資產 %d 筆 / 其他 %d 筆；耗時 %.2fs）" % (
        len(fails), len(warns), elapsed) if fails else \
        ("⚠️  revert-guard PASS（含 %d 筆 WARN；耗時 %.2fs）" % (len(warns), elapsed)
         if warns else "✅ revert-guard PASS（無落後分支回退；耗時 %.2fs）" % elapsed)
    out(summary)


def _delta(v):
    added = "?" if v["added"] is None else v["added"]
    deleted = "?" if v["deleted"] is None else v["deleted"]
    return "added=%s deleted=%s" % (added, deleted)


def _detail(out, v):
    out("      main 上該檔最近的改動：%s %s %s" % (v["main_commit"], v["main_commit_date"],
                                                  v["main_commit_subject"]))
    if v["branch_touched_file"]:
        out("      本分支動過該檔 ⇒ 可能是刻意的刪除；仍必須先 update-branch（否則回退 main 的改動）")
    else:
        out("      本分支沒有動過該檔 ⇒ diff 的刪除純粹來自落後（他人已合併的修正會被回退）")


def build_parser():
    ap = argparse.ArgumentParser(description="落後分支語意回退靜態檢查（issue #1993）")
    ap.add_argument("--base", default="origin/main",
                    help="基準 ref（預設 origin/main；CI 傳 base 分支）")
    ap.add_argument("--head", default="HEAD",
                    help="PR head ref（預設 HEAD）。⚠️ 不要傳 GitHub 的合成 merge ref "
                         "refs/pull/<N>/merge：那個 commit 已含 main，會讓檢查永遠 PASS（假綠）")
    ap.add_argument("--allowlist", default="",
                    help="allowlist JSON（預設 <repo>/scripts/ci/revert-guard-allowlist.json）")
    ap.add_argument("--strict", action="store_true", help="WARN 也視為失敗（exit 1）")
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
    _, parents = git(["rev-list", "--parents", "-n", "1", head_sha], check=False)
    toks = parents.split()
    if len(toks) > 2 and toks[1] == base_sha:
        print("❌ revert-guard：--head 指向合成 merge commit（第一 parent == base tip）。")
        print("   這個 commit 已經含 main ⇒ 檢查會永遠 PASS（假綠），因此拒絕執行。")
        print("   請傳真正的 PR head（CI：`git fetch origin +refs/pull/<N>/head:refs/remotes/origin/pr-head`）。")
        return 2

    allowlist_path = args.allowlist or os.path.join(repo_root, DEFAULT_ALLOWLIST)
    entries, err = load_allowlist(allowlist_path)
    if err:
        print("❌ revert-guard：%s" % err)
        return 2

    bad_reason = [e for e in entries if reason_is_missing(e)]
    violations, exempted, stale = evaluate_violations(base, head, None, entries)
    fails = [v for v in violations if v["severity"] == "FAIL"]
    warns = [v for v in violations if v["severity"] == "WARN"]
    elapsed = time.time() - t0

    if args.json:
        json_dump({
            "check": CHECK_ID,
            "base": {"ref": base, "sha": base_sha},
            "head": {"ref": head, "sha": head_sha},
            "allowlist": allowlist_path,
            "fail": fails,
            "warn": warns,
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
            print("revert-guard: FAIL=%d WARN=%d allowlisted=%d elapsed=%.2fs"
                  % (len(fails), len(warns), len(exempted), elapsed))
        else:
            render_text(print, base, head, violations, exempted, stale, elapsed)

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
