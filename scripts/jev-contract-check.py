#!/usr/bin/env python3
"""jev-contract-check — 對 repo 內所有 Jev 呼叫者強制執行 docs/jev/JEV-USAGE-CONTRACT.md。

檢查項（AST 靜態分析，不需網路）：
  C1 每個呼叫 Jev 的 .py 都必須設定 User-Agent（否則 Cloudflare 以 403 擋掉）
  C2 URL 開啟必須帶 timeout
  C3 請求必須包在 try/except（fail-open）
  C4 choice 題的 criteria 不得是 list 字面值（TypeSafe 回 422）
  C5 scripts/jevkit.py 自測必須通過（選用，無 API key 時自動略過實際呼叫）

用法：python3 scripts/jev-contract-check.py [--root .]
退出碼：0 通過 / 1 違規
"""
from __future__ import annotations

import argparse
import ast
import re
import os
import subprocess
import sys
from typing import List, Tuple

API_MARK = "api.typesafe.ai"
SKIP_DIRS = {".git", "node_modules", "__pycache__", ".venv", "venv", "dist", "build"}
# 契約文件與檢查器本身不呼叫 API；實驗文件含範例片段，不納入靜態檢查
SKIP_FILES = {"jev-contract-check.py", "verify_semantic_find.py"}


def iter_py(root: str) -> List[str]:
    out = []
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        for fn in filenames:
            if fn in SKIP_FILES:
                continue
            full = os.path.join(dirpath, fn)
            if fn.endswith(".py"):
                out.append(full)
                continue
            # 無副檔名的 Python CLI（例如 scripts/jev-find）：看 shebang
            if "." not in fn or fn.startswith("jev-"):
                try:
                    head = open(full, encoding="utf-8", errors="replace").readline()
                except OSError:
                    continue
                if "python" in head:
                    out.append(full)
    return out


def parse(path: str):
    try:
        return ast.parse(open(path, encoding="utf-8", errors="replace").read()), open(path, encoding="utf-8", errors="replace").read()
    except SyntaxError:
        return None, ""


def enclosing_try(tree: ast.AST) -> bool:
    return any(isinstance(n, ast.Try) for n in ast.walk(tree))


PRAGMA = "jev-contract-allow"
IGNORES = {ln.split(":", 1)[0] for ln in []}


def check_file(path: str) -> List[str]:
    problems: List[str] = []
    tree, src = parse(path)
    # 行內豁免：同一行出現 `# jev-contract-allow C4` 時跳過該行的 C4（供「故意違規」的驗證程式碼使用）
    allow_lines = {i + 1 for i, line in enumerate(src.split("\n")) if PRAGMA in line}
    if tree is None or API_MARK not in src:
        return problems
    rel = os.path.relpath(path)
    # 使用 jevkit 的檔案：UA/timeout/retry/fail-open 皆由 jevkit 提供 → 只需確認真的有 import
    if re.search(r"\bfrom\s+jevkit\s+import|\bimport\s+jevkit\b", src):
        if "jevkit" not in src:
            problems.append(f"{rel}: 宣稱使用 jevkit 但未見 import")
        return problems
    # C1 User-Agent
    if "User-Agent" not in src:
        problems.append(f"{rel}: C1 缺少 User-Agent（TypeSafe 對 Python-urllib 預設 UA 回 403）")
    for node in ast.walk(tree):
        # C2 timeout on urlopen / urlretrieve
        if isinstance(node, ast.Call):
            fn = node.func
            name = getattr(fn, "attr", "") or getattr(fn, "id", "")
            if name in ("urlopen", "urlretrieve") and not any(k.arg == "timeout" for k in node.keywords):
                problems.append(f"{rel}: C2 {name}() 未帶 timeout（line {node.lineno}）")
        # C4 choice criteria 不得是 list 字面值（獨立於 Call 之外檢查！）
        if isinstance(node, ast.Dict) and node.lineno not in allow_lines:
            kv = {}
            for k, v in zip(node.keys, node.values):
                if isinstance(k, ast.Constant) and isinstance(k.value, str):
                    kv[k.value] = v
            tv = kv.get("type")
            if isinstance(tv, ast.Constant) and tv.value == "choice":
                crit = kv.get("criteria")
                if isinstance(crit, ast.List):
                    problems.append(
                        f"{rel}: C4 choice 的 criteria 是 list 字面值（line {node.lineno}）→ 必須是 dict，否則 422")
    # C3 fail-open：呼叫 API 的檔案應有 try/except
    if not enclosing_try(tree):
        problems.append(f"{rel}: C3 未見 try/except（違反 fail-open：Jev 故障不得中斷呼叫端）")
    return problems


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    args = ap.parse_args()
    root = args.root

    problems: List[str] = []
    scanned = 0
    for p in iter_py(root):
        scanned += 1
        problems.extend(check_file(p))

    # C5 jevkit 自測
    kit = os.path.join(root, "scripts", "jevkit.py")
    if os.path.exists(kit):
        r = subprocess.run([sys.executable, kit, "--selftest"], capture_output=True, text=True)
        if r.returncode != 0:
            problems.append(f"scripts/jevkit.py: C5 自測失敗\n{r.stdout}{r.stderr}")
        else:
            print(f"  ✓ jevkit 自測通過（{r.stdout.strip().splitlines()[-1][:70]}）")
    else:
        problems.append("scripts/jevkit.py 不存在（規範 §0 要求所有呼叫者使用它）")

    print(f"  scanned {scanned} python files")
    if problems:
        print("\n❌ JEV 契約違規：")
        for x in problems:
            print("   -", x)
        print("\n  規範：docs/jev/JEV-USAGE-CONTRACT.md")
        return 1
    print("  ✅ Jev 契約檢查通過")
    return 0


if __name__ == "__main__":
    sys.exit(main())
