#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""check_integration_filter_sync.py — integration 路徑過濾器 vs 實際 integration 測試目錄的同步守門。

背景（FU-20261015-01）
----------------------
`.github/workflows/ci-cd.yml` 的 `integration` job **只在** `changes` job 的 `dorny/paths-filter`
之 `integration:` 白名單命中時才執行；job 內跑的是 `go test -v -tags=integration -p 1 ./...`，
也就是**整個** integration 套件（不是只跑被改到的套件）。

白名單是**人工維護**的清單，而「哪些目錄有 integration 測試」是**可由 repo 推導**的事實
（檔案開頭的 `//go:build integration`）。兩者一旦脫節，症狀是**靜默**的：
某目錄的 integration 測試在本機跑得到，但只改該目錄的 PR 在 CI 會讓 `integration` job
變成 **skipping**（不是 pass）⇒「本機綠、CI 從未驗」的假安全。

實例（2026-10-16 盤查，main = c3398813b）：`internal/calibration`、`internal/apigateway`、
`internal/fubonproxy`、`internal/marketdata` 與 repo 根目錄的 `integration_test.go`
都不在白名單內（`cmd/atlas-mcp-setup` 是**刻意**例外，見下方 ③）。

判準（唯一權威）
----------------
  ① 對每個 `*.go` 掃 build constraint：出現 `integration` 這個 tag（`//go:build` 或舊式
     `// +build`）⇒ 該檔所在目錄（相對 repo 根；根目錄記為 `.`）屬於「有 integration 測試」集合。
     判準是「目錄」，不是「檔名」：`*_integration_test.go` 只是命名慣例，不能當判準
     （例：`internal/db/schema_version_pg_test.go`、`internal/repository/dual_write_alerts_test.go`
     都不叫 integration 但都帶 tag）。
  ② 該目錄必須被 `integration:` 白名單的**至少一條 glob** 涵蓋（picomatch 語意）。
  ③ 例外必須**逐條**登錄在同一段白名單內的註解：

         # integration-filter-exempt: <dir> — <理由>

     理由留空（只有 `—` 後面沒字）⇒ 紅燈。豁免必須是**活的**（該目錄仍有 integration 測試）
     ⇒ 站點消失後沒刪豁免 = 紅燈（stale）；豁免與白名單同時涵蓋同一目錄 = 紅燈（矛盾）。
     理由必填就是為了讓「為什麼可以不觸發」能被 review。目前已登錄的兩類實例：
       (a) `cmd/atlas-mcp-setup`：該套件已由 `test-cmd-atlas` job（無 paths filter、~95s）逐 PR 執行
           ⇒ 再拉整個 integration suite（實測 864s／820s）只是空跑。
       (b) `internal/marketdata`：唯一的 integration 測試在 CI **確定性 SKIP**（fixture 在
           gitignored 的 repo 根 `data/state`）⇒ 觸發買到的是零覆蓋。

反向（白名單有條目、但沒有任何 integration 測試檔）**只報告、不紅燈**：那些條目可能是
integration 測試的相依套件（例：`internal/testdb` 被 `*_integration_test.go` import），
刪掉會讓「改相依套件卻不觸發 integration job」。留或刪由人判斷，不由守門代刪。

明確界線（本守門**抓不到**什麼）
--------------------------------
  * 只認 build constraint 裡出現 `integration` 的檔；`!integration` 不算。
  * 只回答「trigger 是否可能發生」，**不**檢查測試內容、**不**執行 Go、**不**連網。
  * 不驗證白名單是否「最小」：本守門不知道 glob 是否比需要的更寬（寬 = 多跑 CI，不影響正確性）。
  * glob 語意以 picomatch（dorny/paths-filter 的實作：`picomatch(pattern, {dot: true})`，
    **無** matchBase）為準：`a/**` 涵蓋 a 之下所有層；`integration_test.go` 只匹配根目錄那一支。
    本檔內建一個 glob→regex 轉譯器並以 `--selftest-glob` 自行對拍（見下）。

掃描來源
--------
  * `<root>/.git` 存在時用 `git -C <root> ls-files -z -- '*.go'`（**只看已追蹤檔** ⇒ 自動排除
    `.worktrees/` 這類巢狀 worktree 與建置產物）；否則走檔案系統 walk（排除 `.git`／`.worktrees`／
    `node_modules`／`vendor`）。fixture（hermetic）測試走後者。

用法
----
    python3 scripts/ci/check_integration_filter_sync.py                     # CI／本機
    python3 scripts/ci/check_integration_filter_sync.py --json              # 機器可讀
    python3 scripts/ci/check_integration_filter_sync.py --root <dir> --workflow <file>
    python3 scripts/ci/check_integration_filter_sync.py --selftest-glob      # 只跑 glob 自我對拍

退出碼
------
    0 = 同步（例外皆已登錄且有理由；反向清單只是報告）
    1 = 不同步（有目錄未被涵蓋／豁免沒理由／豁免 stale／豁免與白名單矛盾）
    2 = 執行錯誤（root 不存在／workflow 不存在／解析不到 `integration:` 白名單／glob 樣式不支援）
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from typing import Dict, List, Tuple

# ---------------------------------------------------------------------------
# 常數
# ---------------------------------------------------------------------------

WORKFLOW_DEFAULT = ".github/workflows/ci-cd.yml"
FILTER_KEY_DEFAULT = "integration"
EXEMPT_RE = re.compile(
    r"^\s*#\s*integration-filter-exempt\s*:\s*(?P<path>\S+)\s*(?P<sep>—|--|::)\s*(?P<reason>\S.*)?$"
)
BUILD_TAG_RE = re.compile(r"^\s*//\s*(?:go:build|\+build)\s+(?P<expr>.+?)\s*$")
SKIP_DIRS = {".git", ".worktrees", "node_modules", "vendor"}
SCAN_HEAD_BYTES = 4096

EXIT_OK = 0
EXIT_MISMATCH = 1
EXIT_USAGE = 2


# ---------------------------------------------------------------------------
# glob（picomatch 子集）→ regex
# ---------------------------------------------------------------------------

def glob_to_regex(pattern: str) -> str:
    """把 picomatch 樣式轉成 regex 原文（不含 ^…$，由呼叫端 fullmatch）。

    支援 `**`／`*`／`?`／`[...]`；`*` 不跨 `/`（picomatch 預設，無 matchBase）。
    遇到不支援的樣式（`{a,b}` 大括號展開、`!` 負樣式、`(a|b)` 群組）**丟例外**，
    以免「看不懂的樣式」被安靜地當成「不匹配」而給出假綠。
    """
    if pattern.startswith("!"):
        raise ValueError("負樣式（!…）不支援：%s" % pattern)
    for unsupported in ("{", "}", "(", ")"):
        if unsupported in pattern:
            raise ValueError("不支援的 glob 語法 '%s'：%s" % (unsupported, pattern))

    out: List[str] = []
    i = 0
    n = len(pattern)
    while i < n:
        ch = pattern[i]
        if ch == "*":
            if pattern.startswith("**", i):
                # `**/` ⇒ 零或多層目錄；`/**` 在結尾 ⇒ 該目錄本身或之下任何路徑
                if pattern.startswith("**/", i):
                    out.append("(?:[^/]+/)*")
                    i += 3
                    continue
                if i + 2 == n and i >= 1 and pattern[i - 1] == "/":
                    # picomatch：`a/**` 同時匹配 `a` 本身與 `a` 之下任何路徑
                    # （已 emit 的 `/` 要收回，否則 `a` 本身匹配不到）
                    out.pop()
                    out.append("(?:/.*)?")
                    i += 2
                    continue
                out.append(".*")
                i += 2
                continue
            out.append("[^/]*")
            i += 1
            continue
        if ch == "?":
            out.append("[^/]")
            i += 1
            continue
        if ch == "[":
            j = i + 1
            if j < n and pattern[j] in ("!", "^"):
                j += 1
            if j < n and pattern[j] == "]":
                j += 1
            while j < n and pattern[j] != "]":
                j += 1
            if j >= n:
                raise ValueError("未閉合的 '['：%s" % pattern)
            body = pattern[i + 1:j]
            if body.startswith("!"):
                body = "^" + body[1:]
            out.append("[%s]" % body)
            i = j + 1
            continue
        out.append(re.escape(ch))
        i += 1
    return "".join(out)


def glob_matches(path: str, pattern: str) -> bool:
    return re.fullmatch(glob_to_regex(pattern), path) is not None


# ---------------------------------------------------------------------------
# 蒐集 integration 檔
# ---------------------------------------------------------------------------

def _has_integration_tag(head: str) -> bool:
    """檔案開頭的 build constraint 是否**要求** integration tag（`!integration` 不算）。"""
    for line in head.splitlines():
        m = BUILD_TAG_RE.match(line)
        if not m:
            if line.strip() and not line.lstrip().startswith("//") and not line.startswith("#"):
                break  # 進到程式碼，build constraint 區已結束
            continue
        expr = m.group("expr")
        for token in re.split(r"[\s,&|()!]+", expr):
            if token == "integration":
                return True
    return False


def _iter_go_files_fs(root: str) -> List[str]:
    found: List[str] = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS)
        for name in sorted(filenames):
            if name.endswith(".go"):
                found.append(os.path.relpath(os.path.join(dirpath, name), root))
    return sorted(found)


def _iter_go_files_git(root: str) -> List[str]:
    out = subprocess.run(
        ["git", "-C", root, "ls-files", "-z", "--", "*.go"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if out.returncode != 0:
        raise RuntimeError(
            "git ls-files 失敗（rc=%d）：%s"
            % (out.returncode, out.stderr.decode("utf-8", "replace").strip())
        )
    return sorted(p for p in out.stdout.decode("utf-8", "replace").split("\0") if p)


def collect_integration_dirs(root: str) -> Dict[str, List[str]]:
    """回傳 {目錄相對路徑（根目錄為 '.'）: [帶 integration tag 的檔名…]}，鍵已排序。"""
    # `.git` 可能是目錄（一般 clone）或檔案（linked worktree / submodule）⇒ 用 exists 而非 isdir
    if os.path.exists(os.path.join(root, ".git")):
        paths = _iter_go_files_git(root)
    else:
        paths = _iter_go_files_fs(root)

    result: Dict[str, List[str]] = {}
    for rel in paths:
        full = os.path.join(root, rel)
        try:
            with open(full, "rb") as fh:
                head = fh.read(SCAN_HEAD_BYTES).decode("utf-8", "replace")
        except OSError as exc:
            raise RuntimeError("無法讀取 %s：%s" % (rel, exc))
        if not _has_integration_tag(head):
            continue
        d = os.path.dirname(rel).replace(os.sep, "/")
        result.setdefault(d if d else ".", []).append(os.path.basename(rel))
    return {k: sorted(v) for k, v in sorted(result.items())}


# ---------------------------------------------------------------------------
# 解析 workflow 的 integration 白名單
# ---------------------------------------------------------------------------

def parse_filter_block(workflow_text: str, filter_key: str) -> Tuple[List[str], Dict[str, str], List[str], int]:
    """回傳 (glob 清單, {豁免目錄: 理由}, 沒有理由的豁免行, 白名單起始行號)。"""
    lines = workflow_text.splitlines()
    start = None
    for idx, line in enumerate(lines):
        m = re.match(r"^(\s+)" + re.escape(filter_key) + r":\s*$", line)
        if m:
            # 只認 `filters: |` 內的那一段：`filters: |` 在同檔有兩份（quality.yml 那些檔案不會走這裡）
            start = idx
            indent = len(m.group(1))
            break
    if start is None:
        raise ValueError("在 workflow 找不到 `%s:` 白名單區塊" % filter_key)

    block: List[str] = []
    for line in lines[start + 1:]:
        if not line.strip():
            block.append(line)
            continue
        if len(line) - len(line.lstrip()) <= indent:
            break
        block.append(line)

    patterns: List[str] = []
    exempts: Dict[str, str] = {}
    no_reason: List[str] = []
    for line in block:
        stripped = line.strip()
        if stripped.startswith("#"):
            m = EXEMPT_RE.match(line)
            if m:
                reason = (m.group("reason") or "").strip()
                if reason:
                    exempts[m.group("path").rstrip("/") or "."] = reason
                else:
                    no_reason.append(stripped)
            continue
        m = re.match(r"^\s*-\s*'(?P<pat>[^']+)'\s*$", line)
        if not m:
            m = re.match(r'^\s*-\s*"(?P<pat>[^"]+)"\s*$', line)
        if m:
            patterns.append(m.group("pat"))
    if not patterns:
        raise ValueError("`%s:` 白名單解析不到任何 glob（格式是否改變？）" % filter_key)
    return patterns, exempts, no_reason, start + 1


# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------

def run(root: str, workflow: str, filter_key: str, as_json: bool) -> int:
    if not os.path.isdir(root):
        print("❌ 掃描根不存在：%s" % root, file=sys.stderr)
        return EXIT_USAGE
    if not os.path.isfile(workflow):
        print("❌ workflow 不存在：%s" % workflow, file=sys.stderr)
        return EXIT_USAGE

    try:
        dirs = collect_integration_dirs(root)
    except (RuntimeError, ValueError) as exc:
        print("❌ %s" % exc, file=sys.stderr)
        return EXIT_USAGE

    with open(workflow, "r", encoding="utf-8") as fh:
        wf_text = fh.read()
    try:
        patterns, exempts, no_reason, filter_line = parse_filter_block(wf_text, filter_key)
    except ValueError as exc:
        print("❌ %s" % exc, file=sys.stderr)
        return EXIT_USAGE

    try:
        for pat in patterns:
            glob_to_regex(pat)
    except ValueError as exc:
        print("❌ 白名單有本守門看不懂的 glob 樣式（拒絕假綠）：%s" % exc, file=sys.stderr)
        return EXIT_USAGE

    covered: List[Tuple[str, List[str], str]] = []
    uncovered: List[Tuple[str, List[str]]] = []
    exempt_used: Dict[str, str] = {}
    for d, files in dirs.items():
        probe = files[0] if d == "." else "%s/%s" % (d, files[0])
        hit = next((p for p in patterns if glob_matches(probe, p)), None)
        if hit:
            covered.append((d, files, hit))
            continue
        if d in exempts:
            exempt_used[d] = exempts[d]
            continue
        uncovered.append((d, files))

    # 反向：白名單條目沒有任何 integration 測試檔（只報告）
    unused_patterns: List[str] = []
    for pat in patterns:
        if not any(glob_matches(files[0] if d == "." else "%s/%s" % (d, files[0]), pat)
                   for d, files in dirs.items()):
            unused_patterns.append(pat)

    stale_exempts = sorted(set(exempts) - set(exempt_used) - {d for d, _, _ in covered})
    conflicting = sorted(set(exempts) & {d for d, _, _ in covered})

    ok = not uncovered and not no_reason and not stale_exempts and not conflicting

    if as_json:
        print(json.dumps({
            "ok": ok,
            "filter_line": filter_line,
            "integration_dirs": dirs,
            "covered": [{"dir": d, "files": f, "pattern": p} for d, f, p in covered],
            "uncovered": [{"dir": d, "files": f} for d, f in uncovered],
            "exempt": exempt_used,
            "exempt_without_reason": no_reason,
            "stale_exempt": stale_exempts,
            "conflicting_exempt": conflicting,
            "filter_entries_without_integration_tests": unused_patterns,
        }, ensure_ascii=False, indent=2))
        return EXIT_OK if ok else EXIT_MISMATCH

    print("🔎 integration 路徑過濾器同步檢查（FU-20261015-01）")
    print("   workflow      : %s（白名單起點 line %d）" % (workflow, filter_line))
    print("   帶 integration tag 的目錄：%d" % len(dirs))
    print("   白名單 glob        ：%d" % len(patterns))
    print("   白名單內已登錄豁免 ：%d" % len(exempts))
    print("")

    for d, files, p in covered:
        print("   ✅ %-42s ← %-32s (%s)" % (d, p, ", ".join(files)))
    for d, reason in sorted(exempt_used.items()):
        print("   ⏭️  %-42s ← exempt：%s" % (d, reason))
    print("")

    if unused_patterns:
        print("   ℹ️  以下白名單條目**沒有**任何 integration 測試檔（報告用，不紅燈）：")
        for p in unused_patterns:
            print("        - '%s'" % p)
        print("      → 這些可能是 integration 測試的相依套件（改了要一起觸發）。留或刪由人判斷。")
        print("")

    sys.stdout.flush()  # 讓 stdout/stderr 混流（CI log、`> file 2>&1`）保持列印順序

    if not ok:
        if uncovered:
            print("   ❌ 有 integration 測試、但**沒有任何白名單條目**涵蓋的目錄：", file=sys.stderr)
            for d, files in uncovered:
                if d == ".":
                    # repo 根：picomatch 無 matchBase ⇒ 裸樣式只匹配根目錄那一支（不是任何同名檔）
                    sug = "、".join("- '%s'" % f for f in files)
                else:
                    sug = "- '%s/**'" % d
                print("        - %s（%s）→ 建議補：%s" % (d, ", ".join(files), sug), file=sys.stderr)
            print("      症狀：只改這些目錄的 PR ⇒ integration job = skipping ⇒ 這些測試在 CI 從未被觸發。",
                  file=sys.stderr)
        for raw in no_reason:
            print("   ❌ 豁免沒寫理由（理由必填）：%s" % raw, file=sys.stderr)
        for d in stale_exempts:
            print("   ❌ 豁免已 stale（該目錄已無 integration 測試）：%s" % d, file=sys.stderr)
        for d in conflicting:
            print("   ❌ 豁免與白名單同時涵蓋同一目錄（矛盾，請刪豁免）：%s" % d, file=sys.stderr)
        print("", file=sys.stderr)
        print("❌ integration 過濾器與實際 integration 目錄不同步", file=sys.stderr)
        return EXIT_MISMATCH

    print("✅ integration 過濾器與實際 integration 目錄同步（%d 目錄：%d 涵蓋 + %d 豁免）"
          % (len(dirs), len(covered), len(exempt_used)))
    return EXIT_OK


def selftest_glob() -> int:
    """glob 轉譯器自我對拍（預期值以 node + picomatch 實測為準，見註解）。"""
    cases = [
        # (pattern, path, expected) —— 以 picomatch({dot:true}) 實測
        ("integration_test.go", "integration_test.go", True),
        ("integration_test.go", "cmd/atlas-mcp-setup/integration_test.go", False),
        ("internal/calibration/**", "internal/calibration/pg_reader_integration_test.go", True),
        ("internal/calibration/**", "internal/calibration/nested/x_test.go", True),
        ("internal/calibration/**", "internal/calibration_extra/x_test.go", False),
        ("cmd/atlas-mcp/**", "cmd/atlas-mcp/server/tools_stock_winrate_pg_integration_test.go", True),
        ("cmd/atlas-mcp/**", "cmd/atlas-mcp-setup/integration_test.go", False),
        ("**/*.sql", "migrations/a.sql", True),
        ("**/*.sql", "a.sql", True),
        ("internal/monitoring/**", "internal/monitoring/api/live/outcomes_store_integration_test.go", True),
    ]
    bad = 0
    for pat, path, want in cases:
        got = glob_matches(path, pat)
        flag = "ok " if got == want else "FAIL"
        if got != want:
            bad += 1
        print("  %s %-28s %-62s got=%s want=%s" % (flag, pat, path, got, want))
    print("glob 自我對拍：%d/%d 通過" % (len(cases) - bad, len(cases)))
    return EXIT_OK if bad == 0 else EXIT_MISMATCH


def main(argv: List[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description="integration 路徑過濾器同步守門（FU-20261015-01）")
    ap.add_argument("--root", default=".", help="repo 根（預設 .）")
    ap.add_argument("--workflow", default=None, help="workflow 路徑（預設 <root>/%s）" % WORKFLOW_DEFAULT)
    ap.add_argument("--filter-key", default=FILTER_KEY_DEFAULT, help="白名單 key（預設 integration）")
    ap.add_argument("--json", action="store_true", help="機器可讀輸出")
    ap.add_argument("--selftest-glob", action="store_true", help="只跑 glob 轉譯器自我對拍")
    args = ap.parse_args(argv)

    if args.selftest_glob:
        return selftest_glob()

    root = os.path.abspath(args.root)
    workflow = args.workflow or os.path.join(root, WORKFLOW_DEFAULT)
    if args.workflow and not os.path.isabs(args.workflow):
        workflow = os.path.abspath(args.workflow)
    return run(root, workflow, args.filter_key, args.json)


if __name__ == "__main__":
    sys.exit(main())
