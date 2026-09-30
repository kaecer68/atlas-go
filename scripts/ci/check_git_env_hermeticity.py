#!/usr/bin/env python3
"""check_git_env_hermeticity.py — 「建 throwaway repo／做真 git 寫入」的 GIT_* 環境隔離結構斷言。

【為什麼有這個檢查】
ambient 環境變數（GIT_DIR／GIT_WORK_TREE／GIT_INDEX_FILE／GIT_COMMON_DIR／GIT_PREFIX／
GIT_OBJECT_DIRECTORY／GIT_ALTERNATE_OBJECT_DIRECTORIES）會**污染**子程序內的 git 指令：
在 throwaway repo 內執行的 `git -C "$VAR" ...` 若沒先 unset，git 會改去操作**呼叫者**的 repo
（本 session 實證：a2a-dev #152 `hermes-update-safe.sh` 誤讀呼叫者 HEAD；atlas-go #2141 GIT_DIR
hook 污染）。靜態檢查無法執行別人的腳本，所以這裡只斷言**結構**；執行期的正/負向證明在
`tests/scripts/test-git-env-hermeticity.sh`（(c) 絆線亦是執行期自我測試的一部分）。

【掃描邊界（明確宣告，避免誤以為全 repo 都掃了）】
- 掃：`tests/**/*.sh`、`scripts/**/*.sh` 中**含 `git init`（建 throwaway repo）**者 → 逐檔要求
      (a) 7 個 GIT_* 變數的 unset 行存在；(b) 不得有裸 `git -C "$VAR"`
- 只記錄不要求：同樣檔案集合中含 `git init` 以外的 git 寫入者（揭露用，見 --report-all）
- **不掃**（各自理由）：
  * `Makefile` 食譜與 `*.mk`：repo 內無此型「建 throwaway repo」用法（無 `git init`）
  * `.github/workflows/*.yml`：CI 內無 `git init`；且屬平台層，改動面不同
  * Go 程式內 shell-out（`os/exec` 呼叫 git）：目前 repo 內無此型呼叫（未涵蓋 ⚠️，若未來出現需擴充）
- 豁免（allowlist）＝顯式具名＋理由，**不靜默跳過**。

用法：
  python3 scripts/ci/check_git_env_hermeticity.py [--root DIR] [--paths DIR ...] [--report-all]
退出碼：0＝合規；1＝有未防護檔（allowlist 之外）
"""
from __future__ import annotations

import argparse
import os
import re
import sys
import time

GIT_VARS = (
    "GIT_DIR",
    "GIT_WORK_TREE",
    "GIT_INDEX_FILE",
    "GIT_COMMON_DIR",
    "GIT_PREFIX",
    "GIT_OBJECT_DIRECTORY",
    "GIT_ALTERNATE_OBJECT_DIRECTORIES",
)

# 顯式豁免：檔案 → 具名理由（不得靜默跳過）
# 精煉 (b) 之後，本 repo 的 5 個 throwaway-repo 建立者**全部合規**（皆具 7 變數 unset ＋ 顯式定址），
# 因此 allowlist 目前為空。留空的意義：一旦未來新增不合規檔案，檢查會直接 FAIL（不允許靜默跳過）；
# 真的需要豁免時，必須在此顯式加入「檔名 → 具名理由」。
ALLOWLIST: dict[str, str] = {}

BARE_GIT_C = re.compile(r'git\s+-C\s+"?\$\{?([A-Za-z_][A-Za-z0-9_]*)')
GIT_INIT = re.compile(r'\bgit\s+init\b')


def unset_covers_all(text: str) -> bool:
    """回報 unset 陳述（允許 `\\` 續行、允許拆成多個 unset）是否涵蓋全部 7 個變數。

    實作要點：先去掉反斜線續行，再蒐集所有 `unset ...` 陳述，最後要求**聯集**涵蓋 7 個變數。
    （單行判定會誤報 `unset A B \\<newline> C D` 這種常見寫法 —— atlas-go 的
    tests/scripts/test-revert-guard.sh 即為此形，第一版掃描器曾對它假紅。）
    """
    joined = text.replace("\\\n", " ")
    covered: set[str] = set()
    for line in joined.splitlines():
        s = line.strip()
        if not s.startswith("unset "):
            continue
        for v in GIT_VARS:
            if re.search(rf"\b{v}\b", s):
                covered.add(v)
    return covered == set(GIT_VARS)


def collect(root: str, paths: list[str]) -> list[str]:
    out: list[str] = []
    for base in paths:
        base_abs = os.path.join(root, base)
        for dirpath, _dirnames, filenames in os.walk(base_abs):
            for name in filenames:
                if name.endswith(".sh"):
                    out.append(os.path.relpath(os.path.join(dirpath, name), root))
    return sorted(out)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
    ap.add_argument("--paths", nargs="*", default=["tests", "scripts"])
    ap.add_argument("--report-all", action="store_true", help="連未建 throwaway repo 的 git 使用者一併列出（揭露用）")
    args = ap.parse_args()

    started = time.perf_counter()
    files = collect(args.root, args.paths)

    in_scope: list[str] = []
    git_users_out_of_scope: list[str] = []
    violations: list[tuple[str, str, bool]] = []  # (file, reason, allowlisted)

    for rel in files:
        try:
            text = open(os.path.join(args.root, rel), encoding="utf-8", errors="replace").read()
        except OSError:
            continue
        is_throwaway = bool(GIT_INIT.search(text))
        if not is_throwaway:
            if re.search(r'\bgit\s+(?:-C|commit|worktree|rev-parse)\b', text):
                git_users_out_of_scope.append(rel)
            continue
        in_scope.append(rel)
        reasons = []
        hermetic_env = unset_covers_all(text)
        bare = BARE_GIT_C.findall(text)
        if not hermetic_env:
            reasons.append("缺少涵蓋 7 個 GIT_* 變數的 unset")
            # (b) 只有在缺乏 (a) 時才成立：有了 unset，`git -C "$VAR"` 即是「在已隔離環境內的
            # 顯式定址」，屬合規寫法；沒有 unset 才會被 ambient GIT_DIR 劫持。
            if bare:
                reasons.append(f"裸 git -C \"$VAR\" × {len(bare)}（{', '.join(sorted(set(bare))[:3])}）")
        elif not bare:
            pass
        if reasons:
            violations.append((rel, "；".join(reasons), rel in ALLOWLIST))

    elapsed_ms = (time.perf_counter() - started) * 1000.0

    print("── git 環境隔離結構斷言（GIT_* hermeticity）──")
    print(f"掃描：{len(files)} 個 *.sh（現行範圍 tests/ + scripts/），耗時 {elapsed_ms:.0f} ms")
    print(f"在範圍內（建 throwaway repo，含 git init）：{len(in_scope)} 檔")
    for rel in in_scope:
        status = "✅ 合規" if rel not in {v[0] for v in violations} else ("⚠️ 豁免" if rel in ALLOWLIST else "❌ 未防護")
        print(f"  {status}  {rel}")

    if args.report_all and git_users_out_of_scope:
        print(f"揭露（有 git 使用但未建 throwaway repo ⇒ 不在 (a)(b) 要求範圍）：{len(git_users_out_of_scope)} 檔")
        for rel in git_users_out_of_scope:
            print(f"  · {rel}")

    hard_fail = [v for v in violations if not v[2]]
    print()
    for rel, why, _allow in violations:
        tag = "豁免" if rel in ALLOWLIST else "未防護"
        print(f"  [{tag}] {rel} — {why}")
        if rel in ALLOWLIST:
            print(f"          理由：{ALLOWLIST[rel]}")
    if hard_fail:
        print(f"❌ 未防護數 = {len(hard_fail)}（allowlist 之外，必須為 0）")
        return 1
    print(f"✅ A 結構判準通過：未防護數 = 0（allowlist {len(ALLOWLIST)} 檔，皆具名理由）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
