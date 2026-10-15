#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""check_ledger_seam.py — ledger store seam 守門（禁新增裸 ledger.NewStore）。

背景（2026-10 生產缺陷，T1）
---------------------------
`internal/ledger` 的 backend-aware factory（`NewFullStore` / `NewOutcomeStore` /
`NewReportOutcomeStore` / `PGFirstOutcomeStore`）是 **opt-in**：它只保護「有改用它的呼叫端」。
factory 2026-05-09 誕生後，直建 `ledger.NewStore(dir)`（JSONL-only）的站點仍單調成長。

直建 JSONL store 的失效模式不是「讀不到」而是「**安靜地讀錯後端**」：
`internal/ledger/ledger.go` 的 `Store.LoadOutcomes()` 對不存在的檔案回 `(nil, nil)`，
於是 `internal/autobacktest` 的 signal engine 在 `ATLAS_STORE_BACKEND=postgres` 的生產機上
讀一個空的 JSONL 目錄 ⇒ `GET /api/backtest/signals` 回 **HTTP 200 全 0**、err 為 nil
⇒ `CIRCUIT_BREAKER` 的 `ForceOpenChannel` **永不觸發**（live 斷路器在生產是死的）。

本檢查在 PR 上攔下**新增的**裸 `ledger.NewStore(`；既有站點由 allowlist
（`scripts/ci/ledger-seam-allowlist.json`，每筆 reason 必填）明確登記，不讓檢查永遠紅燈。

掃描規則
--------
  * 範圍：`cmd/` 與 `internal/` 下的 `*.go`，排除 `*_test.go`（測試可以合法用 JSONL store）
    與 fixture/vendor/generated 目錄；`.worktrees/` 等 dot 目錄一律跳過。
  * 只認**程式碼**：以詞法掃描把註解與字串內容換成空白（行號不變），因此
    `// ledger.NewStore(...)` 的文件註解不會誤報。
  * 認得 import alias：`ledgerpkg "…/internal/ledger"` 之後的 `ledgerpkg.NewStore(` 一樣算。
  * 比對鍵是（檔案, 呼叫原文）；同一檔案內相同原文的**數量**也要對得上 ⇒
    「新增一行跟既有站點一字不差的直建呼叫」也會紅（不是只檢查首次出現）。
  * 行號只是導航資訊；行號漂移（別人在上面加行）只印 warning，不紅燈。

判定
----
  * 掃到、allowlist 沒有 ⇒ **FAIL**（exit 1），訊息含 `檔案:行` 與應改用的 factory。
  * 掃到的數量 > allowlist 允許的數量 ⇒ **FAIL**（exit 1）。
  * allowlist 有、掃不到 ⇒ **FAIL**（exit 1，訊息說明「站點已遷移 ⇒ 請刪掉這一筆」；
    以 `--update` 刷新）。
  * allowlist 的 reason 空白或以 `TODO` 開頭 ⇒ **FAIL**（豁免必須有理由）。
  * 行號與記錄不符 ⇒ warning。

用法
----
  python3 scripts/ci/check_ledger_seam.py [--root .] [--allowlist PATH]
                                          [--json] [--update] [--quiet]

退出碼
------
  0 = 通過；1 = 有新增裸呼叫 / stale 條目 / 缺理由；2 = 執行錯誤（掃不到檔案、allowlist 壞掉）。
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from collections import Counter, defaultdict

REPO_ROOT_DEFAULT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

SCAN_DIRS = ("cmd", "internal")
SKIP_DIRS = {
    ".git", ".worktrees", ".codegraph", ".gitnexus", ".gocache", ".build-atlas",
    "node_modules", "__pycache__", ".venv", "venv", "dist", "build", "testdata",
    "vendor", "tmp",
}
LEDGER_IMPORT = "internal/ledger"
CALL_NAME = "NewStore"
ALLOWLIST_DEFAULT = os.path.join("scripts", "ci", "ledger-seam-allowlist.json")
REASON_TODO = "TODO"

# 建議改用的 backend-aware factory（訊息與 allowlist 都用同一份）。
FACTORY_HINT = (
    "請改用 backend-aware factory：ledger.NewFullStore(cfg) / "
    "ledger.NewOutcomeStore(cfg) / ledger.NewReportOutcomeStore(cfg) / "
    "ledger.NewPGFirstOutcomeStore(pg, jsonl)"
)

IMPORT_BLOCK_RE = re.compile(r"^\s*import\s*\((?P<body>.*?)\)", re.S | re.M)
SINGLE_IMPORT_RE = re.compile(r'^\s*import\s+(?P<alias>[A-Za-z_]\w*|[._])?\s*"(?P<path>[^"]+)"', re.M)
IMPORT_LINE_RE = re.compile(r'^\s*(?P<alias>[A-Za-z_]\w*|[._])?\s*"(?P<path>[^"]+)"', re.M)


def strip_go_source(src, strip_strings=True):
    """回傳與原檔等長的內容：註解（與字串內容）換成空白、換行保留（行號不變）。

    strip_strings=False 只去註解、保留字串內容，用於解析 import 路徑
    （import 路徑本身就是字串，全去字串後就讀不到了）。
    """
    n = len(src)
    out = []
    i = 0
    while i < n:
        c = src[i]
        if c == "\n":
            out.append(c)
            i += 1
            continue
        # 行註解
        if c == "/" and i + 1 < n and src[i + 1] == "/":
            j = src.find("\n", i)
            j = n if j == -1 else j
            out.append(" " * (j - i))
            i = j
            continue
        # 區塊註解
        if c == "/" and i + 1 < n and src[i + 1] == "*":
            j = src.find("*/", i + 2)
            j = n if j == -1 else j + 2
            seg = src[i:j]
            out.append("".join("\n" if ch == "\n" else " " for ch in seg))
            i = j
            continue
        if not strip_strings:
            out.append(c)
            i += 1
            continue
        # 反引號 raw string
        if c == "`":
            j = src.find("`", i + 1)
            j = n if j == -1 else j + 1
            seg = src[i:j]
            out.append("".join("\n" if ch == "\n" else " " for ch in seg))
            i = j
            continue
        # 一般字串 / rune
        if c == '"' or c == "'":
            quote = c
            j = i + 1
            while j < n:
                if src[j] == "\\":
                    j += 2
                    continue
                if src[j] == quote or src[j] == "\n":
                    j += 1
                    break
                j += 1
            seg = src[i:j]
            out.append("".join("\n" if ch == "\n" else " " for ch in seg))
            i = j
            continue
        out.append(c)
        i += 1
    return "".join(out)


def _is_ledger_import(path):
    return path == LEDGER_IMPORT or path.endswith("/" + LEDGER_IMPORT)


def ledger_aliases(clean_src):
    """回傳這個檔案裡 internal/ledger 的可用別名（預設 "ledger"）。

    clean_src 必須是 strip_go_source 的輸出：註解裡的 import 範例不該算數。
    """
    aliases = set()
    for match in SINGLE_IMPORT_RE.finditer(clean_src):
        if _is_ledger_import(match.group("path")):
            alias = match.group("alias")
            aliases.add(alias if alias else "ledger")
    block = IMPORT_BLOCK_RE.search(clean_src)
    if block:
        for match in IMPORT_LINE_RE.finditer(block.group("body")):
            if _is_ledger_import(match.group("path")):
                alias = match.group("alias")
                aliases.add(alias if alias else "ledger")
    aliases.discard("_")
    aliases.discard(".")
    return aliases


def normalize_snippet(text):
    return re.sub(r"\s+", " ", text).strip()


def iter_go_files(root):
    for scan_dir in SCAN_DIRS:
        base = os.path.join(root, scan_dir)
        if not os.path.isdir(base):
            continue
        for dirpath, dirnames, filenames in os.walk(base):
            dirnames[:] = sorted(
                d for d in dirnames if d not in SKIP_DIRS and not d.startswith(".")
            )
            for name in sorted(filenames):
                if name.endswith(".go") and not name.endswith("_test.go"):
                    yield os.path.join(dirpath, name)


def scan_sites(root):
    """回傳 (sites, files_scanned)；sites 是 [(rel_path, line_no, snippet)]。

    files_scanned 用來區分「一個都沒掃到（掃描器壞了 ⇒ exit 2）」與
    「掃到了檔案、但沒有任何裸呼叫（全部已遷移 ⇒ 綠燈）」。
    """
    sites = []
    files_scanned = 0
    for path in iter_go_files(root):
        files_scanned += 1
        try:
            with open(path, encoding="utf-8") as handle:
                src = handle.read()
        except OSError as err:
            raise RuntimeError(f"cannot read {path}: {err}") from err
        # import 路徑本身是字串 ⇒ 先只去註解找別名，再用全去字串的版本比對呼叫。
        aliases = ledger_aliases(strip_go_source(src, strip_strings=False))
        if not aliases:
            continue
        clean = strip_go_source(src)
        lines = clean.split("\n")
        raw_lines = src.split("\n")
        for alias in sorted(aliases):
            needle = f"{alias}.{CALL_NAME}("
            for index, line in enumerate(lines):
                start = 0
                while True:
                    pos = line.find(needle, start)
                    if pos == -1:
                        break
                    # 排除 X.ledger.NewStore( 這類（前一個字元不能是識別字或點）
                    if pos > 0 and (line[pos - 1].isalnum() or line[pos - 1] in "_."):
                        start = pos + 1
                        continue
                    rel = os.path.relpath(path, root)
                    raw = raw_lines[index] if index < len(raw_lines) else line
                    sites.append((rel, index + 1, normalize_snippet(raw)))
                    start = pos + 1
    return sites, files_scanned


def load_allowlist(path):
    if not os.path.isfile(path):
        raise RuntimeError(f"allowlist not found: {path}")
    try:
        with open(path, encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, json.JSONDecodeError) as err:
        raise RuntimeError(f"allowlist unreadable: {path}: {err}") from err
    if not isinstance(data, dict) or not isinstance(data.get("sites"), list):
        raise RuntimeError(f"allowlist malformed (expected an object with a 'sites' list): {path}")
    return data


def evaluate(sites, allowlist):
    """回傳 (violations, warnings)；violations 內每項是 dict。"""
    violations = []
    warnings = []

    found = Counter()
    found_lines = defaultdict(list)
    for rel, line, snippet in sites:
        found[(rel, snippet)] += 1
        found_lines[(rel, snippet)].append(line)

    allowed = Counter()
    allowed_lines = defaultdict(list)
    for entry in allowlist["sites"]:
        rel = entry.get("file", "")
        snippet = normalize_snippet(entry.get("snippet", ""))
        allowed[(rel, snippet)] += 1
        allowed_lines[(rel, snippet)].append((entry.get("line"), entry.get("reason", "")))

    for key, count in sorted(found.items()):
        rel, snippet = key
        if allowed.get(key, 0) < count:
            lines = ",".join(str(x) for x in sorted(found_lines[key]))
            reason = (
                "新增的裸 ledger.NewStore 呼叫"
                if allowed.get(key, 0) == 0
                else f"裸 ledger.NewStore 呼叫數量超出 allowlist（掃到 {count}，允許 {allowed[key]}）"
            )
            violations.append({
                "kind": "new-direct-call",
                "file": rel,
                "line": lines,
                "snippet": snippet,
                "detail": f"{reason}；{FACTORY_HINT}",
            })

    for key, count in sorted(allowed.items()):
        rel, snippet = key
        if found.get(key, 0) < count:
            recorded = ",".join(str(x) for x, _ in sorted(
                allowed_lines[key], key=lambda item: (item[0] is None, item[0])
            ))
            violations.append({
                "kind": "stale-allowlist",
                "file": rel,
                "line": recorded,
                "snippet": snippet,
                "detail": (
                    "allowlist 條目已無對應的裸呼叫（站點可能已遷移 ⇒ 請刪掉這一筆，"
                    "或跑 --update 刷新行號）"
                ),
            })
        for _, reason in allowed_lines[key]:
            if not reason.strip() or reason.strip().startswith(REASON_TODO):
                violations.append({
                    "kind": "allowlist-reason-missing",
                    "file": rel,
                    "line": ",".join(str(x) for x, _ in allowed_lines[key]),
                    "snippet": snippet,
                    "detail": "allowlist 每筆必須填寫 reason（空白或 TODO 開頭視為違規）",
                })
                break

    for key, lines in found_lines.items():
        recorded = [entry.get("line") for entry in allowlist["sites"]
                    if entry.get("file") == key[0]
                    and normalize_snippet(entry.get("snippet", "")) == key[1]]
        current = sorted(lines)
        if sorted(x for x in recorded if isinstance(x, int)) != current:
            warnings.append({
                "kind": "line-drift",
                "file": key[0],
                "line": ",".join(str(x) for x in current),
                "snippet": key[1],
                "detail": f"記錄行號 {recorded} 與目前 {current} 不符（只影響導航，請以 --update 刷新）",
            })

    return violations, warnings


def write_allowlist(path, sites, previous):
    """以目前掃描結果重寫 allowlist；保留 (file, snippet) 相同的既有 reason。"""
    kept = {}
    for entry in previous.get("sites", []):
        key = (entry.get("file", ""), normalize_snippet(entry.get("snippet", "")))
        kept.setdefault(key, entry.get("reason", ""))

    entries = []
    for rel, line, snippet in sites:
        key = (rel, snippet)
        reason = kept.pop(key, "")
        if not reason.strip() or reason.strip().startswith(REASON_TODO):
            reason = (
                f"{REASON_TODO}: explain why this site keeps the direct JSONL store "
                f"({FACTORY_HINT})"
            )
        entries.append({"file": rel, "line": line, "snippet": snippet, "reason": reason})

    payload = {
        "_doc": [
            "ledger store seam 豁免清單（scripts/ci/check_ledger_seam.py）。",
            "每筆格式：{file, line, snippet, reason}；line 只是導航用，比對鍵是 (file, snippet) 與其數量。",
            "reason 空白或以 TODO 開頭 ⇒ 檢查失敗（豁免本身必須有理由）。",
            "新站點一律先紅燈：要用裸 JSONL store 就必須在這裡登錄並寫明理由；",
            "只是為了讓 CI 變綠而登錄（reason 寫「歷史遺留」）不算理由。",
            "遷移掉一個站點 ⇒ 刪掉對應這一筆（stale 條目會讓檢查紅燈，這是刻意的）。",
            "規格：scripts/ci/check_ledger_seam.py 的 docstring。",
        ],
        "version": previous.get("version", 1),
        "sites": entries,
    }
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(payload, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    return len(entries)


def main(argv=None):
    parser = argparse.ArgumentParser(description="ledger store seam guard")
    parser.add_argument("--root", default=REPO_ROOT_DEFAULT)
    parser.add_argument("--allowlist", default=None)
    parser.add_argument("--json", action="store_true")
    parser.add_argument("--update", action="store_true")
    parser.add_argument("--quiet", action="store_true")
    args = parser.parse_args(argv)

    root = os.path.abspath(args.root)
    if not os.path.isdir(root):
        print(f"❌ 掃描根目錄不存在：{root}", file=sys.stderr)
        return 2
    allowlist_path = args.allowlist or os.path.join(root, ALLOWLIST_DEFAULT)

    try:
        sites, files_scanned = scan_sites(root)
        allowlist = load_allowlist(allowlist_path)
    except RuntimeError as err:
        print(f"❌ {err}", file=sys.stderr)
        return 2

    if files_scanned == 0:
        print(
            f"❌ 掃描 {SCAN_DIRS} 找不到任何 Go 檔案 —— 掃描器壞了或目錄不對：{root}",
            file=sys.stderr,
        )
        return 2

    if args.update:
        count = write_allowlist(allowlist_path, sites, allowlist)
        print(f"📝 已更新 {allowlist_path}（{count} 筆）")
        print(f"   → 請確認每筆 reason 不是 {REASON_TODO} 開頭，否則下一次檢查會紅燈。")
        return 0

    violations, warnings = evaluate(sites, allowlist)

    if args.json:
        print(json.dumps({
            "files_scanned": files_scanned,
            "sites": len(sites),
            "allowlisted": len(allowlist["sites"]),
            "violations": violations,
            "warnings": warnings,
        }, ensure_ascii=False, indent=2))
        return 1 if violations else 0

    if not args.quiet:
        for warn in warnings:
            print(f"⚠️  {warn['file']}:{warn['line']} {warn['detail']}")

    if violations:
        for item in violations:
            print(
                f"::error file={item['file']},line={str(item['line']).split(',')[0]}::"
                f"ledger store seam：{item['detail']} ｜ {item['file']}:{item['line']}"
                f" ｜ {item['snippet']}"
            )
        print(f"❌ ledger seam 檢查失敗：{len(violations)} 項（掃到 {len(sites)} 個裸呼叫）")
        print(f"   → 若這是刻意的 JSONL 站點，請在 {allowlist_path} 登錄並寫理由；")
        print(f"   → 若是既有站點搬了行，跑 `python3 scripts/ci/check_ledger_seam.py --update`。")
        return 1

    if not args.quiet:
        print(
            f"✅ ledger seam OK：掃描 {files_scanned} 個 Go 檔，{len(sites)} 個裸 ledger.NewStore "
            f"全部已登錄（allowlist {len(allowlist['sites'])} 筆）"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
