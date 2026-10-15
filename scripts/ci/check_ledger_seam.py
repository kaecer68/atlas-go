#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""check_ledger_seam.py — ledger store seam 守門（禁新增裸／間接 JSONL store）。

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

本檢查在 PR 上攔下**新增的**站點；既有站點由 allowlist（`scripts/ci/ledger-seam-allowlist.json`，
每筆 reason 必填）明確登記，不讓檢查永遠紅燈。

檢查兩相
--------
本檢查的掃描目標是一份**明確清單** `TARGETS`（`(import path, 建構子名)`），分兩相：

  ① **直接建構**（id `ledger.NewStore`）：`ledger.NewStore(dir)` —— JSONL-only 的 store 本身。
  ② **間接建構**（id 如 `service.NewLiveService`）：**內部會自建 store、且未注入時落到 JSONL** 的
     建構子。這一相存在的理由：呼叫端只要「建構時忘了注入」，就會安靜落到 JSONL，而檔面上
     完全看不到 `ledger.NewStore(`（實例：`internal/monitoring/api/decision/handlers.go` 與
     `api/dashboard/overview.go` 裸建 `service.NewLiveService`）。只掃 ① 會給人「已涵蓋」的
     錯覺 ⇒ ② 是必要的。

**注意（刻意）**：本檔的掃描目標是清單，不是「所有會建 store 的東西」。新增 target 的條件是
「該建構子在未注入時會**落到 `ledger.NewStore`**」；若某建構子已被本 PR 改成 backend-aware
（例：`autobacktest.NewSignalEngine(cfg)`／`NewComparator(cfg)` 走 `NewFullStore`），它**不列入**
間接清單（列入只會製造假紅燈）。

明確界線（本守門**抓不到**什麼，勿當成已涵蓋）
--------------------------------------------
  * **struct literal 建構**：`apilive.Handlers{LedgerDir: …}` 這類「用欄位填 LedgerDir、之後才
    （或忘了）注入 store」的寫法沒有建構子可掃，本守門看不到 ⇒ 由 review ＋ allowlist 追蹤。
  * **不經 `ledger.NewStore` 的 JSONL 檔案直讀**：例如自己 `os.ReadDir(LedgerDir/sessions)` 或
    `os.ReadFile(.../recommendation_outcomes.jsonl)`。那是另一個家族（file-scan），
    守門抓不到，只能靠 review（同族案例見 `internal/monitoring/api/system/handlers.go` 的舊寫法註解）。
  * **呼叫圖/別名再包裝**：把 `internal/ledger` 包成自己的 adapter 再呼叫、或 dot-import。
    （`ledger.NewFullStore(config.Config{StoreBackend: "jsonl", …})` 這類**顯式** JSONL
    呼叫不在本掃描清單裡：它把後端寫出來了，不是靠預設值；判準是「有沒有明示」。）
  * 本守門是**靜態建構站點清單**：不做呼叫圖分析，因此「有注入」不會自動放行，
    「沒注入」也不必然代表該呼叫路徑真的讀了 store —— allowlist 的 reason 必須自己講清楚。

掃描規則
--------
  * 範圍：`cmd/` 與 `internal/` 下的 `*.go`，排除 `*_test.go`（測試可以合法用 JSONL store）
    與 fixture/vendor/generated 目錄；`.worktrees/` 等 dot 目錄一律跳過。
  * 只認**程式碼**：詞法掃描把註解與字串內容換成空白（行號不變），因此
    `// ledger.NewStore(...)` 的文件註解不會誤報。
  * 認得 import alias：`ledgerpkg "…/internal/ledger"` 之後的 `ledgerpkg.NewStore(` 一樣算。
  * 比對鍵是（檔案, 呼叫原文）；同一檔案內相同原文的**數量**也要對得上 ⇒
    「新增一行跟既有站點一字不差的呼叫」也會紅（不是只檢查首次出現）。
  * 行號只是導航資訊；行號漂移（別人在上面加行）只印 warning，不紅燈。

判定
----
  * 掃到、該 target 的 allowlist 沒有 ⇒ **FAIL**（exit 1），訊息含 `檔案:行`、機制與建議修法。
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
  0 = 通過；1 = 有新站點 / stale 條目 / 缺理由；2 = 執行錯誤（掃不到檔案、allowlist 壞掉）。
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
ALLOWLIST_DEFAULT = os.path.join("scripts", "ci", "ledger-seam-allowlist.json")
ALLOWLIST_VERSION = 2
REASON_TODO = "TODO"

FACTORY_HINT = (
    "請改用 backend-aware factory：ledger.NewFullStore(cfg) / "
    "ledger.NewOutcomeStore(cfg) / ledger.NewReportOutcomeStore(cfg) / "
    "ledger.NewPGFirstOutcomeStore(pg, jsonl) / ledger.NewWindowStore(cfg)"
    "（唯一例外：確實只需要 JSONL 檔案契約的呼叫端，請顯式寫出 "
    'config.Config{StoreBackend: "jsonl", …}，不要靠「忘了指定後端」的預設值）'
)

# 掃描目標（兩相）。id 形如 `pkg.Ctor`；mechanism 寫「未注入時落到哪一行」，advice 寫修法。
TARGETS = [
    {
        "id": "ledger.NewStore",
        "phase": "direct",
        "import_path": "internal/ledger",
        "name": "NewStore",
        "mechanism": (
            "JSONL-only store：LoadOutcomes() 對缺檔回 (nil, nil) ⇒ 生產（postgres）讀空目錄、"
            "回零值且 err == nil"
        ),
        "advice": FACTORY_HINT,
    },
    {
        "id": "service.NewLiveService",
        "phase": "indirect",
        "import_path": "internal/monitoring/service",
        "name": "NewLiveService",
        "mechanism": (
            "未注入 History/TradeStore 時，summariesStore()/tradeStore() 退回 "
            "ledger.NewStore(s.LedgerDir)（internal/monitoring/service/live.go:124）"
        ),
        "advice": (
            "建構後必須注入：WithHistory(service.NewSessionHistoryProvider(cfg)) 或 "
            "WithTradeStore(store)；只建構不注入 = 該 service 的 L-cold 讀取落在 JSONL"
        ),
    },
    {
        "id": "service.NewBacktestService",
        "phase": "indirect",
        "import_path": "internal/monitoring/service",
        "name": "NewBacktestService",
        "mechanism": (
            "BacktestService.Start 以 s.cfg.LedgerDir 自建 JSONL store"
            "（internal/monitoring/service/backtest.go:70）"
        ),
        "advice": (
            "此建構子目前沒有注入 API ⇒ 只能在 service 內部改走 ledger.NewFullStore(cfg)"
            "（FU-20261016-01）"
        ),
    },
    {
        "id": "risk.NewHandlers",
        "phase": "indirect",
        "import_path": "internal/monitoring/api/risk",
        "name": "NewHandlers",
        "mechanism": (
            "未注入 store 時 summariesStore() 退回 ledger.NewStore(h.LedgerDir)"
            "（internal/monitoring/api/risk/handlers.go:165）"
        ),
        "advice": "建構後必須注入：handlers.WithStore(ledger.NewReportOutcomeStore(cfg))",
    },
    {
        "id": "system.NewHandlers",
        "phase": "indirect",
        "import_path": "internal/monitoring/api/system",
        "name": "NewHandlers",
        "mechanism": (
            "未注入 History 時 capital-phase history 退回 ledger.NewStore(h.Svc.LedgerDir)"
            "（internal/monitoring/api/system/handlers.go:94）"
        ),
        "advice": "建構後必須注入：handlers.WithHistory(service.NewSessionHistoryProvider(cfg))",
    },
    {
        "id": "autobacktest.NewSignalEngineFromDir",
        "phase": "indirect",
        "import_path": "internal/autobacktest",
        "name": "NewSignalEngineFromDir",
        "mechanism": "顯式指定 JSONL 後端的 signal engine（2026-10 為相容舊呼叫端而保留）",
        "advice": "production 一律用 autobacktest.NewSignalEngine(cfg) 或 NewSignalEngineWithStore(store)",
    },
    {
        "id": "autobacktest.NewComparatorFromDir",
        "phase": "indirect",
        "import_path": "internal/autobacktest",
        "name": "NewComparatorFromDir",
        "mechanism": "顯式指定 JSONL 後端的 comparator（2026-10 為相容舊呼叫端而保留）",
        "advice": "production 一律用 autobacktest.NewComparator(cfg) 或 NewComparatorWithStore(store)",
    },
]
TARGETS_BY_ID = {target["id"]: target for target in TARGETS}

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


def _is_import_of(path, import_path):
    return path == import_path or path.endswith("/" + import_path)


def package_aliases(comment_free_src, import_path):
    """回傳這個檔案用於 import_path 的別名集合（無 alias 時為路徑最後一段）。

    comment_free_src 必須是 strip_go_source(..., strip_strings=False) 的輸出：
    註解裡的 import 範例不該算數。
    """
    default_alias = import_path.rsplit("/", 1)[-1]
    aliases = set()
    for match in SINGLE_IMPORT_RE.finditer(comment_free_src):
        if _is_import_of(match.group("path"), import_path):
            aliases.add(match.group("alias") or default_alias)
    block = IMPORT_BLOCK_RE.search(comment_free_src)
    if block:
        for match in IMPORT_LINE_RE.finditer(block.group("body")):
            if _is_import_of(match.group("path"), import_path):
                aliases.add(match.group("alias") or default_alias)
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


def scan_targets(root):
    """回傳 (found, files_scanned)。

    found: {target_id: [(rel_path, line_no, snippet)]}。
    files_scanned 用來區分「一個檔都沒掃到（掃描器壞了 ⇒ exit 2）」與
    「掃到了檔案、但沒有任何站點（全部已遷移 ⇒ 綠燈）」。
    """
    found = defaultdict(list)
    files_scanned = 0
    for path in iter_go_files(root):
        files_scanned += 1
        try:
            with open(path, encoding="utf-8") as handle:
                src = handle.read()
        except OSError as err:
            raise RuntimeError(f"cannot read {path}: {err}") from err

        # import 路徑本身是字串 ⇒ 先只去註解找別名，再用全去字串的版本比對呼叫。
        comment_free = strip_go_source(src, strip_strings=False)
        clean = strip_go_source(src)
        clean_lines = clean.split("\n")
        raw_lines = src.split("\n")
        rel = os.path.relpath(path, root)

        for target in TARGETS:
            aliases = package_aliases(comment_free, target["import_path"])
            if not aliases:
                continue
            for alias in sorted(aliases):
                needle = f"{alias}.{target['name']}("
                for index, line in enumerate(clean_lines):
                    start = 0
                    while True:
                        pos = line.find(needle, start)
                        if pos == -1:
                            break
                        # 排除 X.ledger.NewStore( 這類（前一個字元不能是識別字或點）
                        if pos > 0 and (line[pos - 1].isalnum() or line[pos - 1] in "_."):
                            start = pos + 1
                            continue
                        raw = raw_lines[index] if index < len(raw_lines) else line
                        found[target["id"]].append(
                            (rel, index + 1, normalize_snippet(raw))
                        )
                        start = pos + 1
    return found, files_scanned


def load_allowlist(path):
    if not os.path.isfile(path):
        raise RuntimeError(f"allowlist not found: {path}")
    try:
        with open(path, encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, json.JSONDecodeError) as err:
        raise RuntimeError(f"allowlist unreadable: {path}: {err}") from err
    if not isinstance(data, dict) or not isinstance(data.get("targets"), dict):
        raise RuntimeError(
            f"allowlist malformed (expected an object with a 'targets' object): {path}"
        )
    for target_id, entries in data["targets"].items():
        if target_id not in TARGETS_BY_ID:
            raise RuntimeError(
                f"allowlist lists unknown target {target_id!r} "
                f"(known: {', '.join(sorted(TARGETS_BY_ID))})"
            )
        if not isinstance(entries, list):
            raise RuntimeError(f"allowlist target {target_id!r} must be a list")
    return data


def _check_reason(entries):
    for entry in entries:
        reason = str(entry.get("reason", ""))
        if not reason.strip() or reason.strip().startswith(REASON_TODO):
            return True
    return False


def evaluate(found, allowlist):
    """回傳 (violations, warnings)。"""
    violations = []
    warnings = []

    for target in TARGETS:
        target_id = target["id"]
        sites = found.get(target_id, [])
        entries = allowlist["targets"].get(target_id, [])

        found_counter = Counter()
        found_lines = defaultdict(list)
        for rel, line, snippet in sites:
            found_counter[(rel, snippet)] += 1
            found_lines[(rel, snippet)].append(line)

        allowed_counter = Counter()
        allowed_lines = defaultdict(list)
        for entry in entries:
            snippet = normalize_snippet(str(entry.get("snippet", "")))
            allowed_counter[(entry.get("file", ""), snippet)] += 1
            allowed_lines[(entry.get("file", ""), snippet)].append(
                (entry.get("line"), str(entry.get("reason", "")))
            )

        phase_label = "直接" if target["phase"] == "direct" else "間接"
        for key, count in sorted(found_counter.items()):
            rel, snippet = key
            allowed = allowed_counter.get(key, 0)
            if allowed >= count:
                continue
            lines = ",".join(str(x) for x in sorted(found_lines[key]))
            if allowed == 0:
                what = f"新增的{phase_label} JSONL store 站點（{target_id}）"
            else:
                what = (
                    f"{phase_label}站點數量超出 allowlist：掃到 {count}、允許 {allowed}"
                    f"（{target_id}）"
                )
            violations.append({
                "kind": "new-direct-call" if target["phase"] == "direct"
                        else "new-indirect-construction",
                "target": target_id,
                "file": rel,
                "line": lines,
                "snippet": snippet,
                "detail": f"{what}；機制：{target['mechanism']}；{target['advice']}",
            })

        for key, count in sorted(allowed_counter.items()):
            rel, snippet = key
            if found_counter.get(key, 0) < count:
                recorded = ",".join(str(x) for x, _ in allowed_lines[key])
                violations.append({
                    "kind": "stale-allowlist",
                    "target": target_id,
                    "file": rel,
                    "line": recorded,
                    "snippet": snippet,
                    "detail": (
                        "allowlist 條目已無對應站點（站點可能已遷移 ⇒ 請刪掉這一筆，"
                        "或跑 --update 刷新行號）"
                    ),
                })
        if _check_reason(entries):
            violations.append({
                "kind": "allowlist-reason-missing",
                "target": target_id,
                "file": ",".join(sorted({entry.get("file", "") for entry in entries})),
                "line": ",".join(str(entry.get("line")) for entry in entries),
                "snippet": "",
                "detail": "allowlist 每筆必須填寫 reason（空白或 TODO 開頭視為違規）",
            })

        for key, lines in found_lines.items():
            recorded = [
                entry.get("line")
                for entry in entries
                if entry.get("file") == key[0]
                and normalize_snippet(str(entry.get("snippet", ""))) == key[1]
            ]
            current = sorted(lines)
            if sorted(x for x in recorded if isinstance(x, int)) != current:
                warnings.append({
                    "kind": "line-drift",
                    "target": target_id,
                    "file": key[0],
                    "line": ",".join(str(x) for x in current),
                    "snippet": key[1],
                    "detail": (
                        f"記錄行號 {recorded} 與目前 {current} 不符"
                        f"（只影響導航，請以 --update 刷新）"
                    ),
                })

    return violations, warnings


def write_allowlist(path, found, previous):
    """以目前掃描結果重寫 allowlist；保留 (file, snippet) 相同的既有 reason。"""
    targets = {}
    total = 0
    for target in TARGETS:
        target_id = target["id"]
        kept = {}
        for entry in previous.get("targets", {}).get(target_id, []):
            key = (entry.get("file", ""), normalize_snippet(str(entry.get("snippet", ""))))
            kept.setdefault(key, str(entry.get("reason", "")))

        entries = []
        for rel, line, snippet in sorted(found.get(target_id, [])):
            reason = kept.pop((rel, snippet), "")
            if not reason.strip() or reason.strip().startswith(REASON_TODO):
                reason = f"{REASON_TODO}: {target['mechanism']} ⇒ {target['advice']}"
            entries.append({
                "file": rel, "line": line, "snippet": snippet, "reason": reason,
            })
        if entries or target_id in previous.get("targets", {}):
            targets[target_id] = entries
            total += len(entries)

    payload = {
        "_doc": [
            "ledger store seam 豁免清單（scripts/ci/check_ledger_seam.py）。",
            "結構：{\"targets\": {<target id>: [{file, line, snippet, reason}, …]}}。",
            "line 只是導航用；比對鍵是 (file, snippet) 及其數量（同檔重複也會紅）。",
            "reason 空白或以 TODO 開頭 ⇒ 檢查失敗（豁免本身必須有理由）。",
            "新站點一律先紅燈：要用裸／未注入的 JSONL store 就必須在這裡登錄並寫明理由；",
            "只是為了讓 CI 變綠而登錄（reason 寫「歷史遺留」）不算理由。",
            "遷移掉一個站點 ⇒ 刪掉對應這一筆（stale 條目會讓檢查紅燈，這是刻意的）。",
            "ledger.NewStore = 直接建構；其餘 target = 未注入時會落到 JSONL 的間接建構子。",
            "規格：scripts/ci/check_ledger_seam.py 的 docstring（含明確界線：struct literal、",
            "檔案直讀、呼叫圖包裝都不在本守門範圍）。",
        ],
        "version": ALLOWLIST_VERSION,
        "targets": targets,
    }
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(payload, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    return total


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
        found, files_scanned = scan_targets(root)
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
        count = write_allowlist(allowlist_path, found, allowlist)
        print(f"📝 已更新 {allowlist_path}（{count} 筆）")
        print(f"   → 請確認每筆 reason 不是 {REASON_TODO} 開頭，否則下一次檢查會紅燈。")
        return 0

    violations, warnings = evaluate(found, allowlist)
    direct_sites = len(found.get("ledger.NewStore", []))
    indirect_sites = sum(len(v) for k, v in found.items() if k != "ledger.NewStore")
    allow_total = sum(len(v) for v in allowlist.get("targets", {}).values())

    if args.json:
        print(json.dumps({
            "files_scanned": files_scanned,
            "direct_sites": direct_sites,
            "indirect_sites": indirect_sites,
            "per_target": {k: len(v) for k, v in sorted(found.items())},
            "allowlisted": allow_total,
            "violations": violations,
            "warnings": warnings,
        }, ensure_ascii=False, indent=2))
        return 1 if violations else 0

    if not args.quiet:
        for warn in warnings:
            print(f"⚠️  {warn['file']}:{warn['line']} {warn['detail']}")

    if violations:
        for item in violations:
            first_line = str(item["line"]).split(",")[0] or "1"
            print(
                f"::error file={item['file']},line={first_line}::"
                f"ledger store seam：{item['detail']} ｜ {item['file']}:{item['line']}"
                f" ｜ {item['snippet']}"
            )
        print(
            f"❌ ledger seam 檢查失敗：{len(violations)} 項"
            f"（直接站點 {direct_sites}、間接站點 {indirect_sites}）"
        )
        print(f"   → 若這是刻意的站點，請在 {allowlist_path} 登錄並寫理由；")
        print("   → 若是既有站點搬了行，跑 `python3 scripts/ci/check_ledger_seam.py --update`。")
        return 1

    if not args.quiet:
        print(
            f"✅ ledger seam OK：掃描 {files_scanned} 個 Go 檔，"
            f"直接站點 {direct_sites} 個、間接站點 {indirect_sites} 個全部已登錄"
            f"（allowlist {allow_total} 筆）"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
