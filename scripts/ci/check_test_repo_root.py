#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""check_test_repo_root.py — 禁止測試用固定相對層數定位 repo 根（FU-20261016-20）。

背景
----
2026-10-16 一次 CI 紅是本機抓不到的：`internal/stockpicker` 的 integration 測試用
`filepath.Join(wd, "../..")` 找 repo 根，而呼叫點在 `t.Chdir(root)` **之後** ⇒ CI
（checkout 於 `/home/runner/work/atlas-go/atlas-go`）解析到 `/home/runner/work`
⇒ `read parameters.json: open /home/runner/work/configs/parameters.json: no such file`。
本機之所以綠，是因為 lane worktree 位於 `<主 clone>/.worktrees/<lane>`：從 lane 根往上兩層
**剛好是主 clone**（那裡有 `configs/parameters.json`）⇒ 同一個錯誤答案在本機看起來是對的。

`"../.."` 這個寫法把兩個隱含假設寫死在測試裡：
  ① 行程的 cwd 永遠是套件目錄（`t.Chdir` 之後就不是）；
  ② 套件永遠在 module root 下剛好兩層（`internal/x` 是，`internal/a/b` 不是，`cmd/x` 是，
     搬動套件後也不是）。
正確做法：從 cwd 往上找 `go.mod`（`internal/testutil/reporoot`），錯誤訊息要**指名解析出的路徑**。

判準（本守門）
--------------
掃「已追蹤」的 `*_test.go`，只看非註解行（行首 `//` 略過：註解可能正在**說明**為什麼不要這樣寫）：

  Tier A（硬禁止，無額度）：把「一個值當成 repo 根」算出來的形狀——
    A1 `os.Chdir("<../..>…")`（爬兩層的 chdir）
    A2 `filepath.Abs/Clean("<../..>…")`
    A3 `filepath.Abs/Clean(filepath.Join("..", "..", …))`
    A4 `filepath.Abs/Clean(filepath.Join(<變數>, "../..", …))`
    A5 `<名字含 root 的變數> = "<../..>…"`（例：`const repoRootFromCmd = "../.."`）
    A6 `<名字含 root 的變數> = filepath.Join("..", "..", …)`（例：`root := filepath.Join("..", "..")`）

  Tier B（額度制／ratchet）：其餘任何 `"../.."` 開頭的相對路徑字面值，或
    `filepath.Join("..", "..")`。repo 內既有約 140 處（多為 `"../../configs/parameters.json"`
    這種直接讀檔），全部以 `scripts/ci/test-repo-root-baseline.json` 的**每檔行數**作為額度：
      · 超過額度（新增行／新增檔）⇒ **紅燈**（新測試不得再引入固定層數）
      · 低於額度（有人改用 helper）⇒ 只**報告** stale，請跑 `--update-baseline` 下調
        （不紅燈：多條 lane 平行時，別人修一处不該讓你紅）
    理由：Tier B 的每一處都是一個獨立測試套件的行為，改它必須逐套件驗證（有些要 PG）；
    在一個 PR 裡全改是無謂風險，但不設額度就會持續增長。

用法
----
    python3 scripts/ci/check_test_repo_root.py                 # CI／本機
    python3 scripts/ci/check_test_repo_root.py --root <dir> --baseline <file>
    python3 scripts/ci/check_test_repo_root.py --update-baseline
    python3 scripts/ci/check_test_repo_root.py --json

退出碼
------
    0 = 合規；1 = 有違規（Tier A 或 Tier B 超額）；2 = 執行／用法錯誤
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from typing import Dict, List, Tuple

BASELINE_DEFAULT = os.path.join("scripts", "ci", "test-repo-root-baseline.json")
SKIP_DIRS = {".git", ".worktrees", "node_modules", "vendor"}

EXIT_OK = 0
EXIT_VIOLATION = 1
EXIT_USAGE = 2

# 兩個 pattern 原語：Tier A/B 的每一條都由它們組出來。
# 契約測試的 mutation 會把**這兩行**換成永不匹配的字串，用來證明紅燈真的來自偵測器
# （tests/scripts/test-test-repo-root-guard.sh ⑪a）。
CLIMB_LITERAL = r'"\.\./\.\."'  # 以 ../.. 開頭的相對路徑字面值
JOIN_TWO_DOTS = r'filepath\.Join\(\s*"\.\.",\s*"\.\."'  # filepath.Join("..", ".."

TIER_A: List[Tuple[str, "re.Pattern[str]"]] = [
    ("A1 os.Chdir 爬兩層（cwd 假設）", re.compile(r'os\.Chdir\(\s*' + CLIMB_LITERAL)),
    ("A2 filepath.Abs/Clean 吃固定層數字面值", re.compile(r'filepath\.(?:Abs|Clean)\(\s*' + CLIMB_LITERAL)),
    ("A3 filepath.Abs/Clean(Join(\"..\", \"..\"))", re.compile(r'filepath\.(?:Abs|Clean)\(\s*' + JOIN_TWO_DOTS)),
    ("A4 filepath.Abs/Clean(Join(<dir>, ../..))", re.compile(r'filepath\.(?:Abs|Clean)\(\s*filepath\.Join\(\s*[A-Za-z_][\w.]*\s*,\s*' + CLIMB_LITERAL)),
    ("A5 root 變數 = 固定層數字面值", re.compile(r'\b(?:[A-Za-z_]\w*)?[Rr]oot\w*\s*(?::=|=(?!=))\s*' + CLIMB_LITERAL)),
    ("A6 root 變數 = filepath.Join(\"..\", \"..\")", re.compile(
        r'\b(?:[A-Za-z_]\w*)?[Rr]oot\w*\s*(?::=|=(?!=))\s*' + JOIN_TWO_DOTS)),
]

TIER_B = re.compile(CLIMB_LITERAL + r'|' + JOIN_TWO_DOTS)

def is_comment(line: str) -> bool:
    return line.lstrip().startswith("//")


def tracked_test_files(root: str) -> List[str]:
    """已追蹤的 `*_test.go`（相對 root）。git 不可用時退回路徑 walk。"""
    try:
        out = subprocess.run(
            ["git", "-C", root, "ls-files", "-z", "--", "*_test.go"],
            check=True, capture_output=True,
        ).stdout
        files = [p.decode("utf-8", "replace") for p in out.split(b"\0") if p]
        return sorted(files)
    except (OSError, subprocess.CalledProcessError):
        files = []
        for dirpath, dirnames, filenames in os.walk(root):
            dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
            for name in filenames:
                if name.endswith("_test.go"):
                    files.append(os.path.relpath(os.path.join(dirpath, name), root))
        return sorted(files)


def scan(root: str, files: List[str]) -> Tuple[List[str], Dict[str, List[Tuple[int, str]]]]:
    tier_a: List[str] = []
    tier_b: Dict[str, List[Tuple[int, str]]] = {}
    for rel in files:
        path = os.path.join(root, rel)
        try:
            with open(path, encoding="utf-8", errors="replace") as fh:
                lines = fh.read().splitlines()
        except OSError as exc:
            raise SystemExit(f"read {rel}: {exc}")
        for lineno, line in enumerate(lines, 1):
            if is_comment(line):
                continue
            hit = None
            for label, pattern in TIER_A:
                if pattern.search(line):
                    hit = label
                    break
            if hit:
                tier_a.append(f"{rel}:{lineno}: [{hit}] {line.strip()}")
                continue
            if TIER_B.search(line):
                tier_b.setdefault(rel, []).append((lineno, line.strip()))
    return tier_a, tier_b


def load_baseline(path: str) -> Dict[str, int]:
    if not os.path.exists(path):
        return {}
    with open(path, encoding="utf-8") as fh:
        raw = json.load(fh)
    files = raw.get("files", raw)
    if not isinstance(files, dict):
        raise SystemExit(f"baseline {path}: expected an object with a 'files' mapping")
    return {str(k): int(v) for k, v in files.items()}


def write_baseline(path: str, counts: Dict[str, int], scanner: str) -> None:
    payload = {
        "_comment": (
            "Tier B 額度（FU-20261016-20）：*_test.go 內以 \"../..\" 開頭的固定層數相對路徑，"
            "依檔案記行數。新增/超額 ⇒ scripts/ci/check_test_repo_root.py 紅燈。"
            "改用 internal/testutil/reporoot 之後跑 --update-baseline 下調。"
        ),
        "_scanner": scanner,
        "files": dict(sorted(counts.items())),
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
        fh.write("\n")


def main(argv: List[str]) -> int:
    parser = argparse.ArgumentParser(add_help=True)
    parser.add_argument("--root", default=None)
    parser.add_argument("--baseline", default=None)
    parser.add_argument("--update-baseline", action="store_true")
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--selftest-glob", action="store_true", help="保留給 CI 硬參數相容；本檢查不用 glob")
    args = parser.parse_args(argv)

    root = args.root or os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    if not os.path.isdir(root):
        print(f"❌ root 不存在：{root}", file=sys.stderr)
        return EXIT_USAGE
    baseline_path = args.baseline or os.path.join(root, BASELINE_DEFAULT)

    files = tracked_test_files(root)
    if not files:
        # 掃不到檔案 ≠ 沒有違規：這是最典型的假綠形狀。
        print(f"❌ 在 {root} 找不到任何 *_test.go（工具／路徑錯誤，不得當成通過）", file=sys.stderr)
        return EXIT_USAGE

    tier_a, tier_b = scan(root, files)
    counts = {rel: len(hits) for rel, hits in tier_b.items()}

    if args.update_baseline:
        write_baseline(baseline_path, counts, "scripts/ci/check_test_repo_root.py")
        print(f"✅ baseline 已更新：{baseline_path}（{len(counts)} 個檔案、{sum(counts.values())} 行）")
        return EXIT_OK

    baseline = load_baseline(baseline_path)

    growth: List[str] = []
    stale: List[str] = []
    for rel, n in sorted(counts.items()):
        allowed = baseline.get(rel, 0)
        if n > allowed:
            growth.append(f"{rel}: {n} 行，額度 {allowed}（+{n - allowed}）")
        elif n < allowed:
            stale.append(f"{rel}: {n} 行，額度 {allowed}（-{allowed - n}）")
    for rel, allowed in sorted(baseline.items()):
        if allowed and rel not in counts:
            stale.append(f"{rel}: 0 行，額度 {allowed}（-{allowed}）")

    if args.json:
        print(json.dumps({
            "tier_a": tier_a,
            "tier_b_growth": growth,
            "tier_b_stale": stale,
            "tier_b": {rel: [ln for ln, _ in hits] for rel, hits in counts.items()},
        }, ensure_ascii=False, indent=2))
    else:
        print("🔎 測試不得用固定相對層數定位 repo 根（FU-20261016-20）")
        print(f"   掃描：{len(files)} 個 *_test.go（已追蹤）；baseline：{baseline_path}")
        print(f"   Tier A（硬禁止）：{len(tier_a)} 處；Tier B（額度）：{sum(counts.values())} 行 / "
              f"{len(counts)} 檔（額度 {sum(baseline.values())} 行 / {len(baseline)} 檔）")
        print("")
        if tier_a:
            print("   ❌ Tier A 命中（改成 internal/testutil/reporoot，錯誤訊息要指名解析出的路徑）：")
            for line in tier_a:
                print(f"      {line}")
        if growth:
            print("   ❌ Tier B 超額（新測試不得再引入固定層數）：")
            for line in growth:
                print(f"      {line}")
            for rel in sorted({g.split(':', 1)[0] for g in growth}):
                for lineno, text in tier_b[rel]:
                    print(f"        {rel}:{lineno}: {text}")
        if stale:
            print("   ℹ️  baseline 過期（有人減少了用量）⇒ 跑 --update-baseline 下調：")
            for line in stale:
                print(f"      {line}")
        if tier_a or growth:
            print("")
            print("   ❌ 測試用固定相對層數定位 repo 根：lane 本機可能假綠、CI 紅（見 FU-20261016-20）")
            return EXIT_VIOLATION
        print("")
        print("   ✅ 無 Tier A、Tier B 未超額")
    return EXIT_VIOLATION if (tier_a or growth) else EXIT_OK


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
