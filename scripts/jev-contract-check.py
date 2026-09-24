#!/usr/bin/env python3
"""jev-contract-check — 對 repo 內所有 Jev 呼叫者強制執行 docs/jev/JEV-USAGE-CONTRACT.md。

檢查項（AST 靜態分析，不需網路）：
  C1 每個呼叫 Jev 的 .py 都必須設定 User-Agent（否則 Cloudflare 以 403 擋掉）
  C2 URL 開啟必須帶 timeout
  C3 請求必須包在 try/except（fail-open）
  C4 choice 題的 criteria 不得是 list 字面值（TypeSafe 回 422）
  C5 scripts/jevkit.py 自測（**warn-only 2026-09-24**：含實際 API 呼叫，依賴外部網路，
     與本檢查「不需網路」的前提矛盾；網路/限流造成失敗只警告不阻擋。
     需要嚴格模式時設 `JEV_CONTRACT_STRICT_SELFTEST=1`。jevkit.py **不存在**仍 blocking。）
  C6 呼叫 Jev 的 .py 必須有 `# jev-docs: <官方 URL>` 標註（≥1 條）
     —— 2026-09-23 新增：過去多次「先自行推論、才回頭對照官方文件」造成違規實作
     （atlas-wiki 閘門違反官方 ≥5 條指引；照官方問法改寫後同一批資料 F1 0.33→0.78）。
     沒有出處等於沒有依據。行內豁免：`# jev-contract-allow C6`
  C7 repo 內 Jev 呼叫者**不得使用 alias `jev-latest`**（2026-09-24 新增）
     —— 官方（https://docs.typesafe.ai/models）：已校準門檻的 caller 必須 pin 版本 ID，
     alias 會隨官方更新漂移、讓已校準門檻失效。生產一律 `jev-1.13.0`。
     只檢查「程式碼字串字面量」（`'jev-latest'` / `"jev-latest"`）；`#` 註解行與
     純敘述文件（.md）不檢查。行內豁免：`# jev-contract-allow C7`

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
    # C6 官方文件出處標註（2026-09-23 新增）— 必須放在 early return 之前，否則使用 jevkit 的檔案會被跳過
    caller = ("jevkit" in src or "api.typesafe" in src or "typesafe.ai" in src) and "import" in src
    if caller and PRAGMA + " C6" not in src:
        if not re.search(r"#\s*jev-docs:\s*\S+", src):
            problems.append(
                f"{os.path.relpath(path)}: C6 缺少 `# jev-docs:` 官方文件出處標註 → 先讀官方文件"
                "（docs.typesafe.ai）再設計；格式例：`# jev-docs: https://docs.typesafe.ai/primitives`"
            )
    # C7 生產 caller 禁用 alias jev-latest（2026-09-24 新增）— 只查程式碼字串字面量；
    # `#` 註解行不查（文件/教學文字可提及 alias），行內豁免：`# jev-contract-allow C7`
    if caller and PRAGMA + " C7" not in src:
        for lineno, line in enumerate(src.split("\n"), 1):
            stripped = line.lstrip()
            if stripped.startswith("#"):
                continue
            if PRAGMA + " C7" in line:
                continue
            if '"' + 'jev-latest' in line or "'" + 'jev-latest' in line:
                problems.append(
                    f"{os.path.relpath(path)}: C7 使用 alias `jev-latest`（line {lineno}）→ 生產必須 pin "
                    "版本 ID `jev-1.13.0`（https://docs.typesafe.ai/models）；"
                    "行內豁免：`# jev-contract-allow C7`"
                )
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
    warnings: List[str] = []
    scanned = 0
    for p in iter_py(root):
        scanned += 1
        problems.extend(check_file(p))

    # C5 jevkit 自測
    #
    # 2026-09-24 修正（fleet-wide，a2a-dev + atlas-go 同步）：
    #   jevkit --selftest 含**實際 API 呼叫**，依賴外部網路（TypeSafe）。
    #   這與本檢查宣稱的「AST 靜態分析，不需網路」自相矛盾，且 API 限流
    #   （實測：同 session 反覆呼叫後被限流）會讓 pre-push gate 偽失敗、
    #   阻擋正常推送。故預設改為 **warn-only**：
    #     - jevkit.py **不存在** → 仍 blocking（靜態要求，規範 §0）
    #     - selftest **回非 0** → warn-only（可能只是網路/限流）
    #   需要嚴格模式（例：CI 專用 runner，網路穩定）時設：
    #     JEV_CONTRACT_STRICT_SELFTEST=1
    kit = os.path.join(root, "scripts", "jevkit.py")
    if os.path.exists(kit):
        r = subprocess.run([sys.executable, kit, "--selftest"], capture_output=True, text=True)
        if r.returncode != 0:
            msg = f"scripts/jevkit.py: C5 自測失敗（可能為網路/限流，非靜態違規）\n{r.stdout}{r.stderr}"
            if os.environ.get("JEV_CONTRACT_STRICT_SELFTEST") == "1":
                problems.append(msg)
            else:
                warnings.append(msg)
        else:
            print(f"  ✓ jevkit 自測通過（{r.stdout.strip().splitlines()[-1][:70]}）")
    else:
        problems.append("scripts/jevkit.py 不存在（規範 §0 要求所有呼叫者使用它）")

    print(f"  scanned {scanned} python files")
    if warnings:
        print("\n⚠️  JEV 契約警告（warn-only，不阻擋）：")
        for w in warnings:
            print("   -", w.splitlines()[0])
        print("   （如需強制：JEV_CONTRACT_STRICT_SELFTEST=1）")
    if problems:
        print("\n❌ JEV 契約違規：")
        for x in problems:
            print("   -", x)
        print("\n  規範：docs/jev/JEV-USAGE-CONTRACT.md")
        return 1
    print("  ✅ Jev 契約檢查通過" + ("（含警告）" if warnings else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main())
