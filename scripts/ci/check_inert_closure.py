#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""check_inert_closure.py — inert（宣告生效、實際未生效）閉環靜態檢查（#1944 建議 2）。

背景
----
atlas-go 反覆出現同一家族缺陷：**有寫入、無消費**的靜默失效 —— config 有宣告但全 repo 沒有
reader（例如 `industry.max_daily_weight_change` 這類「假風控」）、writer 有 setter 但沒有
production consumer、狀態欄位硬寫 `applied=true` 卻沒有消費證據。issue #1944 建議 2 要求建立
自動檢查，避免同類再生。

本檢查在 PR 上攔下**新增的** inert 閉環；既有歷史項由 baseline 明確登記（每項必填理由），
不以「歷史遺留」讓檢查永遠紅燈。

四類檢查（check id）
--------------------
  config-inert        configs/parameters.json 的葉參數沒有任何 reader：非測試程式碼沒有讀取對應
                      Go 欄位、沒有讀該欄位的 accessor 被呼叫、不在 param_table（parameters API
                      的 get/set 表）、也沒有以字串字面量被引用。
  writer-no-consumer  exported writer（Set/Register/Emit/Record/Publish… 開頭）沒有任何非測試
                      呼叫端。tier=`dead`（連測試都沒呼叫）或 `test-only`（只有測試呼叫）。
  claim-not-derived   「已生效」語意欄位（applied/calibrated/…）由字面值 `true` 硬寫，
                      而不是由消費證據推導。
  claim-not-consumed  同上欄位有 producer（被寫入）但沒有 consumer（沒有讀取點，也沒有以
                      JSON key 字面量被消費）。

低誤報機制（理由必填）
--------------------
  1. 行內豁免：`// inert-ok[<check-id>]: <理由>`，放在宣告行或前 3 行；省略 `[<check-id>]`
     代表適用全部檢查。理由空白時，豁免本身即為違規（`inert-ok-missing-reason`）。
  2. baseline allowlist：`scripts/ci/inert-baseline.json`，每項必填 `reason`；`reason` 空白或
     以 `TODO` 開頭同樣視為違規（`baseline-reason-missing`）。
  3. config 參數（JSON 不支援註解）只能走 baseline，並在 `reason` 寫明處置與依據。

誠實聲明（本檢查抓不到什麼）
--------------------------
  純靜態：抓不到只有 runtime 才看得出來的 inert（consumer 存在但資料恆空、旗標翻了但行為
  不變、consumer 在別的服務）。因此 baseline 只保證「已登記」，不保證「已修」。詳見
  docs/specs/inert-static-check-spec.md。

用法
----
  python3 scripts/ci/check_inert_closure.py [--root .] [--baseline PATH]
                                            [--json] [--update-baseline] [--quiet]
退出碼
------
  0 = 通過；1 = 有新增違規或缺理由；2 = 執行錯誤。
"""

from __future__ import annotations

import argparse
import collections
import json
import os
import re
import sys
import time

# ---------------------------------------------------------------------------
# 掃描範圍與規則常數
# ---------------------------------------------------------------------------
SKIP_DIRS = {
    ".git", "node_modules", "__pycache__", ".venv", "venv", "dist", "build",
    "testdata", ".codegraph", ".gitnexus", "vendor",
}
SCAN_DIRS = ("internal", "cmd")          # production Go 掃描範圍（見 spec：其他目錄不視為 production）
CONFIG_PKG_DIR = "internal/config/"
PARAM_TABLE_FILE = "internal/config/param_table.go"

WRITER_PREFIXES = (
    "Set", "Store", "Save", "Write", "Persist", "Record", "Apply", "Mark",
    "Enable", "Disable", "Sync", "Publish", "Emit", "Register", "Install",
    "Wire", "Attach", "Mount", "Bind", "Update", "Add", "Put", "Upsert",
    "Insert", "Create", "Remove", "Delete", "Clear", "Reset", "Append",
    "Commit", "Flush", "Recalculate", "Promote", "Toggle", "Freeze", "Unfreeze",
)
CLAIM_KEY_RE = re.compile(
    r"^(applied|calibrated|wired|enforced|consumed|effective"
    r"|[a-z0-9_]+_(applied|calibrated|wired|enforced|consumed|effective))$"
)
ANNOT_RE = re.compile(r"//\s*inert-ok(?:\[([A-Za-z0-9_\-]+)\])?\s*:\s*(.*)$")
ANNOT_LOOKBACK = 3
# parameters.json 的最上層信封欄位（不是可調參數，不列入 reader 檢查）
CONFIG_META_KEYS = {"version", "updated_at", "$schema"}
CHECK_IDS = ("config-inert", "writer-no-consumer", "claim-not-derived", "claim-not-consumed")
BASELINE_TODO = "TODO"
IDENT_TOKEN_RE = re.compile(r"[A-Za-z_]\w*")
CALL_RE = re.compile(r"\b([A-Za-z_]\w*)\s*\(")
CONFIG_ROOT_TYPE = "ParametersConfig"      # configs/parameters.json 的根結構
FUNC_DECL_RE = re.compile(
    r"^func\s+(?:\(([^)]*)\)\s*)?([A-Z][A-Za-z0-9_]*)\s*\(([^)]*)\)([^{\n]*)", re.M
)
STRUCT_TAG_RE = re.compile(r"^\s*([A-Z][A-Za-z0-9_]*)\s+\S[^`\n]*`([^`]*)`", re.M)
# json tag 可能帶選項（`,omitempty`）→ 不可要求捕捉群組後面緊接 `"`
JSON_TAG_RE = re.compile(r'json:"([^",]+)')


def _brace_block(src, open_idx):
    """回傳 src[open_idx] 開始的大括號區塊內容（不含最外層大括號）。"""
    depth = 0
    i = open_idx
    while i < len(src):
        if src[i] == "{":
            depth += 1
        elif src[i] == "}":
            depth -= 1
            if depth == 0:
                return src[open_idx + 1:i]
        i += 1
    return src[open_idx + 1:]


def _base_type(tok):
    """把 `*Foo`、`[]Foo`、`map[string]Foo`、`Foo[T]` 收斂成 `Foo`。"""
    tok = tok.strip().lstrip("*").lstrip("[]")
    if tok.startswith("map[") and "]" in tok:
        tok = tok[tok.index("]") + 1:]
    if "[" in tok:
        tok = tok[:tok.index("[")]
    return tok.lstrip("*")


class Violation:
    __slots__ = ("check", "key", "detail", "evidence", "tier")

    def __init__(self, check, key, detail="", evidence="", tier=""):
        self.check = check
        self.key = key
        self.detail = detail
        self.evidence = evidence
        self.tier = tier

    def as_dict(self):
        out = {"check": self.check, "key": self.key, "detail": self.detail,
               "evidence": self.evidence}
        if self.tier:
            out["tier"] = self.tier
        return out

    def __str__(self):
        tier = f" [{self.tier}]" if self.tier else ""
        ev = f"  ({self.evidence})" if self.evidence else ""
        return f"{self.key}{tier}: {self.detail}{ev}"


# ---------------------------------------------------------------------------
# Go 原始碼詞法處理
# ---------------------------------------------------------------------------
def strip_go(src):
    """回傳 (clean, strings)。

    clean 與原檔等長：註解與字串「內容」以空白取代、換行保留，因此行號不變。
    strings = [(字面量內容, 行號)]，用來偵測以字串（動態 key / 依名稱註冊）做的引用。
    """
    n = len(src)
    out = []
    strings = []
    i = 0
    line = 1
    while i < n:
        c = src[i]
        if c == "\n":
            out.append(c)
            line += 1
            i += 1
            continue
        if c == "/" and i + 1 < n and src[i + 1] == "/":
            j = src.find("\n", i)
            j = n if j == -1 else j
            out.append(" " * (j - i))
            i = j
            continue
        if c == "/" and i + 1 < n and src[i + 1] == "*":
            j = src.find("*/", i + 2)
            j = n if j == -1 else j + 2
            seg = src[i:j]
            out.append("".join("\n" if ch == "\n" else " " for ch in seg))
            line += seg.count("\n")
            i = j
            continue
        if c == '"':
            j = i + 1
            buf = []
            while j < n and src[j] != "\n":
                if src[j] == "\\" and j + 1 < n:
                    buf.append(src[j:j + 2])
                    j += 2
                    continue
                if src[j] == '"':
                    break
                buf.append(src[j])
                j += 1
            if j < n and src[j] == '"':
                strings.append(("".join(buf), line))
                out.append(" " * (j + 1 - i))
                i = j + 1
                continue
            out.append(" " * (j - i))
            i = j
            continue
        if c == "`":
            j = src.find("`", i + 1)
            j = n - 1 if j == -1 else j
            strings.append((src[i + 1:j], line))
            seg = src[i:j + 1]
            out.append("".join("\n" if ch == "\n" else " " for ch in seg))
            line += seg.count("\n")
            i = j + 1
            continue
        if c == "'":
            j = i + 1
            while j < n and src[j] not in ("'", "\n"):
                if src[j] == "\\" and j + 1 < n:
                    j += 2
                    continue
                j += 1
            out.append(" " * (j + 1 - i))
            i = j + 1
            continue
        out.append(c)
        i += 1
    return "".join(out), strings


# ---------------------------------------------------------------------------
# RepoIndex：兩趟掃描建立所有查詢索引（單趟 regex 掃全 repo，維持 CI < 30s）
# ---------------------------------------------------------------------------
class RepoIndex:
    def __init__(self, root, config_path=None):
        self.root = os.path.abspath(root)
        self.config_path = config_path or os.path.join(self.root, "configs", "parameters.json")
        self.files = {}
        self.go_files = []
        self.scan_go = []
        self.funcs = []
        self.annotations = []
        self.tags = {}
        self.param_table_keys = set()
        self.accessor_fields = {}
        self.struct_types = {}           # 型別名 -> {json key: (欄位, 型別, 行, 檔)}
        self.ident_files = collections.defaultdict(list)   # 名稱 -> 非測試檔案（僅候選名稱）
        self.call_files = collections.defaultdict(list)    # 被呼叫名稱 -> 非測試檔案
        self.test_call_counts = collections.Counter()
        self.literal_files = {}          # 字面量 -> 第一個非測試檔案（不含 internal/config plumbing）
        self.literal_outside_config = {}  # 同上，但排除 internal/config（唯一可當 reader 證據者）
        self.claim_sites = collections.defaultdict(lambda: {"hard": [], "writes": [], "reads": 0})
        self.t0 = time.time()
        self._pass1()
        self._pass2()

    # -- 第一趟：讀檔、抽 struct tag / func 宣告 / 註解 -------------------
    def _pass1(self):
        for dirpath, dirnames, filenames in os.walk(self.root):
            dirnames[:] = sorted(d for d in dirnames if d not in SKIP_DIRS)
            for fn in sorted(filenames):
                if not fn.endswith(".go"):
                    continue
                full = os.path.join(dirpath, fn)
                rel = os.path.relpath(full, self.root)
                try:
                    src = open(full, encoding="utf-8", errors="replace").read()
                except OSError:
                    continue
                clean, strs = strip_go(src)
                self.files[rel] = {"clean": clean, "strings": strs,
                                   "is_test": fn.endswith("_test.go"),
                                   # 只在真的有豁免註解時保留原文行（省記憶體）
                                   "raw": src.split("\n") if "inert-ok" in src else None,
                                   # struct tag 位於反引號內，strip_go 會抹掉 → 保留原檔供抽 tag
                                   "src": src}
                self.go_files.append(rel)
        self.go_files.sort()
        self.scan_go = [r for r in self.go_files
                        if r.startswith(tuple(d + "/" for d in SCAN_DIRS))
                        and not self.files[r]["is_test"]]
        for rel in self.go_files:
            c = self.files[rel]
            if c["is_test"]:
                continue
            if rel.startswith(tuple(d + "/" for d in SCAN_DIRS)):
                # struct tag 在反引號字串內，必須對原檔抽（clean 已把字串內容抹成空白）
                src = c["src"]
                for m in STRUCT_TAG_RE.finditer(src):
                    jm = JSON_TAG_RE.search(m.group(2))
                    if jm:
                        self.tags.setdefault(jm.group(1), []).append(
                            (m.group(1), rel, src[:m.start()].count("\n") + 1))
            for m in FUNC_DECL_RE.finditer(c["clean"]):
                self.funcs.append({
                    "rel": rel, "line": c["clean"][:m.start()].count("\n") + 1,
                    "name": m.group(2), "recv": m.group(1) or "",
                    "params": m.group(3).strip(), "rets": m.group(4).strip(),
                })
        for c in self.files.values():
            c.pop("src", None)      # 抽完 struct tag 即釋放原檔字串（省記憶體）
        self._index_annotations()
        self._index_struct_types()
        self._index_param_table()
        self._index_accessors()

    def _index_annotations(self):
        for rel in self.go_files:
            if not self.in_scope(rel):
                continue
            raw = self.files[rel].get("raw")
            if not raw:
                continue
            for i, line in enumerate(raw, 1):
                m = ANNOT_RE.search(line)
                if m:
                    self.annotations.append({"rel": rel, "line": i,
                                             "scope": (m.group(1) or "all").strip(),
                                             "reason": (m.group(2) or "").strip()})

    # -- struct 巢狀解析（config 路徑 -> Go 欄位；解決同 json tag 歧義）---------
    FIELD_LINE_RE = re.compile(r"^\s*([A-Z][A-Za-z0-9_]*)\s+(\S+)\s+`([^`]*)`", re.M)

    def _index_struct_types(self):
        """建立 {型別名: {json key: (欄位名, 欄位型別, 行號)}}。

        為什麼需要：`sector_constraints_carry_trade_unwind` 這類 json tag 在 Risk 與 Drawdown
        兩個 struct 都有（Go 欄位名還不同），扁平比對會把 config 路徑配到錯的欄位，造成誤報
        （`drawdown.*` 被當成沒有 reader，因為它被配到 Risk 的欄位）。改走
        `ParametersConfig` → 巢狀型別 的路徑解析即可唯一決定。
        """
        src_all = {}
        for rel in self.go_files:
            if self.files[rel]["is_test"] or not rel.startswith(CONFIG_PKG_DIR):
                continue
            src = self._read(rel)
            if src:
                src_all[rel] = src
        for rel, src in src_all.items():
            for m in re.finditer(r"^type\s+(\w+)\s+struct\s*\{", src, re.M):
                body = _brace_block(src, m.end() - 1)
                fields = {}
                for fm in self.FIELD_LINE_RE.finditer(body):
                    jm = JSON_TAG_RE.search(fm.group(3))
                    if not jm:
                        continue
                    raw_type = fm.group(2)
                    line = src[:m.end()].count("\n") + 1 + body[:fm.start()].count("\n") + 1
                    fields.setdefault(jm.group(1),
                                      (fm.group(1), _base_type(raw_type), line, rel, raw_type))
                if fields:
                    self.struct_types.setdefault(m.group(1), fields)

    def _read(self, rel):
        try:
            return open(os.path.join(self.root, rel), encoding="utf-8", errors="replace").read()
        except OSError:
            return ""

    def go_field_map(self):
        """扁平備援索引：json key -> (欄位名, rel, 行號)（僅 internal/config 的宣告）。

        巢狀解析失敗時使用；同 tag 多結構時取第一個（較不精確，見 resolve_config_path）。
        """
        out = {}
        for key, entries in self.tags.items():
            for fld, rel, line in entries:
                if rel.startswith(CONFIG_PKG_DIR):
                    out.setdefault(key, (fld, rel, line))
        return out

    def resolve_config_path(self, path):
        """以 struct 巢狀解析 config 路徑 -> (欄位名, rel, 行號)。

        找不到時退回「扁平 json tag 比對」（較不精確，但避免整個檢查失效）。
        """
        parts = path.split(".")
        cur_type = CONFIG_ROOT_TYPE
        cur = None
        resolved = 0
        for seg in parts:
            fields = self.struct_types.get(cur_type)
            if not fields:
                break
            f = fields.get(seg)
            if not f:
                break
            cur = f
            cur_type = f[1]
            resolved += 1
        if cur is not None and resolved > 0:
            # (欄位, 檔, 行, 已解析段數, 總段數, 原始型別)
            return cur[0], cur[3], cur[2], resolved, len(parts), cur[4]
        flat = self.go_field_map().get(parts[-1])
        if flat:
            return flat[0], flat[1], flat[2], len(parts), len(parts), ""
        for i in range(len(parts), 0, -1):
            flat = self.go_field_map().get(parts[i - 1])
            if flat:
                return flat[0], flat[1], flat[2], i, len(parts), ""
        return None

    def _index_param_table(self):
        """param_table.go = parameters API 的 by-name get/set 表（真正的 reader 介面）。

        注意：key 是字串字面量，clean 版本已被抹成空白 → 必須讀原檔。
        """
        path = os.path.join(self.root, PARAM_TABLE_FILE)
        if not os.path.exists(path):
            return
        try:
            src = open(path, encoding="utf-8", errors="replace").read()
        except OSError:
            return
        self.param_table_keys = set(
            re.findall(r'^\t"([A-Za-z0-9_]+)":\s*\{', src, re.M))

    def _body(self, rel, line):
        c = self.files[rel]["clean"]
        idx, cur = 0, 1
        while cur < line:
            nl = c.find("\n", idx)
            if nl == -1:
                return ""
            idx = nl + 1
            cur += 1
        start = c.find("{", idx)
        if start == -1:
            return ""
        depth, i = 0, start
        while i < len(c):
            if c[i] == "{":
                depth += 1
            elif c[i] == "}":
                depth -= 1
                if depth == 0:
                    break
            i += 1
        return c[start:i + 1]

    def _index_accessors(self):
        for f in self.funcs:
            if not f["rel"].startswith(CONFIG_PKG_DIR):
                continue
            if not (f["name"].startswith("Get") or f["name"].startswith("Is")):
                continue
            body = self._body(f["rel"], f["line"])
            fields = set(re.findall(r"\.([A-Z][A-Za-z0-9_]*)(?:\.Value|\b)", body))
            fields.discard("Value")
            if fields:
                self.accessor_fields.setdefault(f["name"], set()).update(fields)

    # -- 第二趟：識別字 / 呼叫點 / 字面量 / claim 站點 ----------------------
    def _pass2(self):
        candidates = set()
        for key, entries in self.tags.items():
            for fld, _, _ in entries:
                candidates.add(fld)
        candidates.update(self.accessor_fields)
        claim_fields = self.claim_fields()
        claim_re = None
        if claim_fields:
            field_names = sorted({v[0] for v in claim_fields.values()}, key=len, reverse=True)
            alt = "|".join(re.escape(f) for f in field_names)
            keys = sorted({k for _, k, _, _ in claim_fields.values()})
            key_alt = "|".join(re.escape(k) for k in keys)
            claim_re = re.compile(
                r"(?:\.\s*(?P<sel>" + alt + r")\b(?P<op>\s*(?:\+\+|--|[-+*/]?=))?)"
                r"|(?:\b(?P<lit>" + alt + r")\s*:\s*true\b)"
                r"|(?:\b(?P<set>" + alt + r")\s*:)"
                r"|(?:\"(?P<key>" + key_alt + r")\"\s*:)")
        for rel in self.go_files:
            c = self.files[rel]
            clean = c["clean"]
            is_test = c["is_test"]
            in_scope = self.in_scope(rel)
            if in_scope:
                ids = set(IDENT_TOKEN_RE.findall(clean))
                if not is_test:
                    for name in ids & candidates:
                        self.ident_files[name].append(rel)
                for name in set(CALL_RE.findall(clean)):
                    if is_test:
                        self.test_call_counts[name] += 1
                    else:
                        self.call_files[name].append(rel)
            if not is_test and in_scope:
                outside_cfg = not rel.startswith(CONFIG_PKG_DIR)
                for s, _ in c["strings"]:
                    self.literal_files.setdefault(s, rel)
                    if outside_cfg:
                        self.literal_outside_config.setdefault(s, rel)
            if claim_re is not None and not is_test and in_scope:
                self._collect_claim_sites(rel, clean, claim_re, claim_fields)

    def claim_fields(self):
        """回傳 {ident: (field, key, rel, line)}；claim = 「已生效」語意欄位。"""
        out = {}
        for key, entries in self.tags.items():
            if not CLAIM_KEY_RE.match(key):
                continue
            for fld, rel, line in entries:
                out[f"{key}@{fld}"] = (fld, key, rel, line)
        return out

    def _collect_claim_sites(self, rel, clean, claim_re, claim_fields):
        key_to_field = {}
        for _, (fld, k, _, _) in claim_fields.items():
            key_to_field.setdefault(k, fld)
        for m in claim_re.finditer(clean):
            ln = clean[:m.start()].count("\n") + 1
            site = f"{rel}:{ln}"
            for gname in ("sel", "lit", "set", "key"):
                g = m.group(gname)
                if not g:
                    continue
                field = key_to_field.get(g, g) if gname == "key" else g
                bucket = self.claim_sites[field]
                if gname == "sel":
                    if m.group("op"):
                        bucket["writes"].append(site)
                    else:
                        bucket["reads"] += 1
                elif gname == "lit":
                    bucket["hard"].append(site)
                else:            # set / key 皆為寫入（複合字面值或 JSON map literal）
                    bucket["writes"].append(site)

    # -- 查詢工具 ---------------------------------------------------------
    def in_scope(self, rel):
        """只有 internal/ 與 cmd/ 視為 production（排除 tests/、fixtures、前端 embed 等）。"""
        return rel.startswith(tuple(d + "/" for d in SCAN_DIRS))

    def non_test_files(self, outside_config_only=False, scan_dirs_only=False):
        for rel in self.go_files:
            c = self.files[rel]
            if c["is_test"]:
                continue
            if outside_config_only and rel.startswith(CONFIG_PKG_DIR):
                continue
            if scan_dirs_only and not rel.startswith(tuple(d + "/" for d in SCAN_DIRS)):
                continue
            yield rel, c

    def refs_outside_config(self, name):
        return [r for r in self.ident_files.get(name, ()) if not r.startswith(CONFIG_PKG_DIR)]

    def has_non_test_consumer(self, name, decl_rel, decl_line):
        files = self.call_files.get(name) or []
        if any(r != decl_rel for r in files):
            return True
        if decl_rel in files:
            pat = re.compile(r"\b" + re.escape(name) + r"\s*\(")
            for m in pat.finditer(self.files[decl_rel]["clean"]):
                if self.files[decl_rel]["clean"][:m.start()].count("\n") + 1 != decl_line:
                    return True
        return False

    def find_annot(self, rel, line, check):
        for a in self.annotations:
            if a["rel"] != rel or not (line - ANNOT_LOOKBACK <= a["line"] <= line):
                continue
            if a["scope"] in ("all", "", check):
                return a
        return None


# ---------------------------------------------------------------------------
# 檢查 1：config 旗標有 reader
# ---------------------------------------------------------------------------
def flatten_params(node, path=""):
    """把 parameters.json 展平為葉節點路徑（`{"value": ...}` 的節點視為葉）。"""
    if isinstance(node, dict):
        if "value" in node:
            return [path]
        leaves = []
        for k, v in node.items():
            leaves.extend(flatten_params(v, f"{path}.{k}" if path else k))
        return leaves
    return [path]


def check_config_inert(idx):
    violations = []
    if not os.path.exists(idx.config_path):
        print(f"❌ 找不到 config：{idx.config_path}", file=sys.stderr)
        return violations
    try:
        cfg = json.load(open(idx.config_path, encoding="utf-8"))
    except (OSError, ValueError) as exc:
        print(f"❌ 無法解析 {idx.config_path}: {exc}", file=sys.stderr)
        sys.exit(2)

    leaves = sorted({p for p in flatten_params(cfg) if p and p not in CONFIG_META_KEYS})
    seen = set()
    for path in leaves:
        hit = idx.resolve_config_path(path)
        decl = "" if hit is None else f"{hit[1]}:{hit[2]}"
        if hit is None:
            violations.append(Violation(
                "config-inert", path,
                "config 葉參數在 internal/config 找不到對應 Go 欄位（沒有宣告型別可讀）",
                idx.config_path))
            continue
        field, resolved_n, total, raw_type = hit[0], hit[3], hit[4], hit[5]
        if (path, field) in seen:
            continue
        seen.add((path, field))

        flat_key = "_".join(path.split("."))
        # 路徑深於 Go 巢狀結構（例如 JSON 的 metadata 子區塊、或以 map 為容器的子設定）：
        #   - 容器是 map → 讀容器就是讀所有項目，容器證據有效；
        #   - 容器是 struct/純量 → 容器「被讀」不代表這一葉被讀，不可拿來當 reader 證據。
        container_is_map = raw_type.startswith("map[")
        partial = resolved_n < total
        if not partial or container_is_map:
            if idx.refs_outside_config(field):
                continue
            if any(idx.call_files.get(a) for a, flds in idx.accessor_fields.items()
                   if field in flds):
                continue
        if flat_key in idx.param_table_keys:
            continue
        # 以「完整路徑字串」被讀取（例：參數覆寫 API 以 dotted path 取值）。
        # 刻意不採用單段 key（`"surge_boost"`）：同一字串常被當成顯示標籤，
        # 無法證明是 config reader（見 spec「已知誤報來源」）。
        if flat_key in idx.literal_outside_config:
            continue
        detail = ("config 有宣告但沒有任何 reader（非測試程式碼未讀取該欄位、無 accessor 呼叫者、"
                  "不在 param_table、也無字串引用）")
        if partial and not container_is_map:
            detail = (f"config 路徑深於 Go 巢狀結構（最近容器欄位 {field} @ {decl} 不算 reader）："
                      "Go 型別沒有對應這一葉的欄位")
        violations.append(Violation("config-inert", path, detail, decl))
    return violations


# ---------------------------------------------------------------------------
# 檢查 2：writer 有 consumer
# ---------------------------------------------------------------------------
def check_writer_no_consumer(idx):
    violations = []
    for f in idx.funcs:
        rel, name, line = f["rel"], f["name"], f["line"]
        if not rel.startswith(tuple(d + "/" for d in SCAN_DIRS)):
            continue
        if not name.startswith(WRITER_PREFIXES):
            continue
        if f["params"] == "":
            continue          # 零參數的 Set*/Register* 是 reader/accessor，不是 writer
        if idx.has_non_test_consumer(name, rel, line):
            continue
        test_calls = idx.test_call_counts.get(name, 0)
        tier = "test-only" if test_calls else "dead"
        detail = ("writer 只有測試呼叫端（production 無 consumer）" if tier == "test-only"
                  else "writer 沒有任何呼叫端（含測試），疑似死碼")
        violations.append(Violation(
            "writer-no-consumer", f"{rel}:{name}", detail,
            f"{rel}:{line}  test-files={test_calls}", tier=tier))
    return violations


# ---------------------------------------------------------------------------
# 檢查 3/4：狀態欄位 producer / consumer 配對
# ---------------------------------------------------------------------------
def check_claims(idx):
    violations = []
    fields = idx.claim_fields()
    for ident, (field, key, rel, line) in sorted(fields.items()):
        sites = idx.claim_sites.get(field)
        if not sites:
            continue
        decl = f"{rel}:{line}"
        hard = [s for s in sorted(set(sites["hard"]))]
        for site in hard:
            violations.append(Violation(
                "claim-not-derived", ident,
                f"「已生效」欄位 {field} 以字面值 true 硬寫（未由消費證據推導）", site))
        writes = [s for s in sites["writes"] if s != decl]
        reads = sites["reads"]
        if writes and reads == 0 and key not in idx.literal_files:
            violations.append(Violation(
                "claim-not-consumed", ident,
                f"「已生效」欄位 {field} 有 producer（被寫入）但沒有 consumer"
                "（非測試程式碼無讀取點、無 JSON key 字面量消費）",
                f"{decl}  writes={len(writes)}"))
    return violations


# ---------------------------------------------------------------------------
# 行內豁免過濾
# ---------------------------------------------------------------------------
GO_EVIDENCE_RE = re.compile(r"^(.+\.go):(\d+)")


def filter_annotations(idx, violations):
    kept = []
    reported_missing = set()
    for v in violations:
        m = GO_EVIDENCE_RE.match(v.evidence or "")
        if m:
            rel, line = m.group(1), int(m.group(2))
            a = idx.find_annot(rel, line, v.check)
            if a is not None:
                if a["reason"]:
                    continue
                key = f"{a['rel']}:{a['line']}"
                if key not in reported_missing:
                    reported_missing.add(key)
                    kept.append(Violation(
                        "inert-ok-missing-reason", key,
                        "`inert-ok` 豁免註解沒有填理由（理由必填，空理由不算豁免）", key))
                continue
        kept.append(v)
    for a in idx.annotations:
        if a["reason"]:
            continue
        key = f"{a['rel']}:{a['line']}"
        if key in reported_missing:
            continue
        reported_missing.add(key)
        kept.append(Violation(
            "inert-ok-missing-reason", key,
            "`inert-ok` 豁免註解沒有填理由（理由必填，空理由不算豁免）", key))
    uniq = {}
    for v in kept:
        uniq[(v.check, v.key)] = v
    return sorted(uniq.values(), key=lambda v: (v.check, v.key))


# ---------------------------------------------------------------------------
# baseline allowlist
# ---------------------------------------------------------------------------
def load_baseline(path):
    if not os.path.exists(path):
        return {"version": 1, "entries": {}}
    try:
        data = json.load(open(path, encoding="utf-8"))
    except (OSError, ValueError) as exc:
        print(f"❌ baseline 無法解析（{path}）: {exc}", file=sys.stderr)
        sys.exit(2)
    data.setdefault("entries", {})
    return data


def baseline_entries(baseline):
    out = {}
    for check, items in baseline.get("entries", {}).items():
        if isinstance(items, list):
            out[check] = {k: {} for k in items}
        else:
            out[check] = dict(items)
    return out


def apply_baseline(violations, baseline):
    entries = baseline_entries(baseline)
    known = {f"{c}\x00{k}" for c, items in entries.items() for k in items}
    new, used = [], set()
    for v in violations:
        k = f"{v.check}\x00{v.key}"
        if k in known:
            used.add(k)
            continue
        new.append(v)
    stale = sorted(k for k in known if k not in used)
    missing = sorted(
        f"{c}: {k}" for c, items in entries.items() for k, e in items.items()
        if not (e or {}).get("reason", "").strip()
        or (e or {}).get("reason", "").strip().upper().startswith(BASELINE_TODO))
    return new, stale, missing


def write_baseline(path, violations, baseline):
    entries = baseline.setdefault("entries", {})
    added = 0
    for v in violations:
        bucket = entries.setdefault(v.check, {})
        if v.key in bucket:
            continue
        bucket[v.key] = {
            "reason": f"{BASELINE_TODO}: 請填寫理由（測試 seam／legacy 待 #1944 稽核／已人工複核）",
            "evidence": v.evidence,
        }
        if v.tier:
            bucket[v.key]["tier"] = v.tier
        added += 1
    for check in list(entries):
        entries[check] = dict(sorted(entries[check].items()))
    baseline["version"] = baseline.get("version", 1)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(baseline, fh, ensure_ascii=False, indent=2)
        fh.write("\n")
    return added


LABELS = {
    "config-inert": "config 旗標有 reader",
    "writer-no-consumer": "writer 有 consumer",
    "claim-not-derived": "狀態欄位由證據推導",
    "claim-not-consumed": "狀態欄位有 consumer",
    "inert-ok-missing-reason": "豁免註解理由必填",
}


def main(argv=None):
    ap = argparse.ArgumentParser(description="inert 閉環靜態檢查（#1944 建議 2）")
    ap.add_argument("--root", default=".", help="repo 根目錄（預設 .）")
    ap.add_argument("--baseline", default="",
                    help="baseline JSON 路徑（預設 <root>/scripts/ci/inert-baseline.json）")
    ap.add_argument("--json", action="store_true", help="以 JSON 輸出（機器可讀）")
    ap.add_argument("--update-baseline", action="store_true",
                    help="把新違規寫入 baseline（reason 留 TODO，由人工填寫）")
    ap.add_argument("--quiet", action="store_true", help="只印總結")
    args = ap.parse_args(argv)

    root = os.path.abspath(args.root)
    baseline_path = args.baseline or os.path.join(root, "scripts", "ci", "inert-baseline.json")
    idx = RepoIndex(root)
    violations = filter_annotations(idx, run_checks(idx))
    baseline = load_baseline(baseline_path)

    if args.update_baseline:
        added = write_baseline(baseline_path, violations, baseline)
        print(f"📝 baseline 更新：新增 {added} 項 -> {os.path.relpath(baseline_path, root)}")
        print(f"   ⚠️  新項目 reason 為 {BASELINE_TODO}；未填理由會讓檢查變紅（理由必填）。")
        return 0

    new, stale, missing = apply_baseline(violations, baseline)
    by_check = collections.Counter(v.check for v in violations)
    if args.json:
        print(json.dumps({
            "root": root,
            "elapsed_sec": round(time.time() - idx.t0, 2),
            "go_files": len(idx.go_files),
            "total_violations": len(violations),
            "by_check": dict(by_check),
            "new": [v.as_dict() for v in new],
            "stale_baseline": [s.replace("\x00", " ") for s in stale],
            "baseline_reason_missing": missing,
        }, ensure_ascii=False, indent=2))
        return 1 if (new or missing) else 0

    if not args.quiet:
        print("🔍 inert 閉環靜態檢查（#1944 建議 2）")
        print(f"   repo     : {root}")
        print(f"   Go 檔案   : {len(idx.go_files)}（production 掃描範圍 {', '.join(SCAN_DIRS)}）")
        print(f"   baseline : {os.path.relpath(baseline_path, root)}")
        print("")
        for check in CHECK_IDS:
            n = by_check.get(check, 0)
            newn = sum(1 for v in new if v.check == check)
            print(f"   {check:22s} {LABELS.get(check, check):18s} 命中 {n:4d}  新增 {newn:3d}")
        print("")

    if missing:
        print(f"❌ baseline 有 {len(missing)} 項沒有填理由（理由必填）：")
        for r in missing[:20]:
            print(f"     {r}")
        if len(missing) > 20:
            print(f"     ... 其餘 {len(missing) - 20} 項")
        print("")

    if stale and not args.quiet:
        print(f"⚠️  baseline 有 {len(stale)} 項已不再命中（stale；僅警告，不影響 CI 結果）：")
        for s in stale[:20]:
            print(f"     {s.replace(chr(0), ' ')}")
        if len(stale) > 20:
            print(f"     ... 其餘 {len(stale) - 20} 項")
        print("   → 已接線或已移除者請人工複核後從 baseline 刪除。")
        print("")

    if new:
        print(f"❌ 發現 {len(new)} 筆**新增** inert 閉環（未被 baseline 覆蓋）：")
        cur = None
        for v in new:
            if v.check != cur:
                cur = v.check
                print(f"  [{cur}] {LABELS.get(cur, cur)}")
            print(f"    • {v}")
        print("")
        print("怎麼修（三選一，理由必填）：")
        print("  1. 接線：讓 writer 有 production consumer／參數有 reader／狀態由消費證據推導。")
        print("  2. 行內豁免：在宣告處（或前 3 行）加 `// inert-ok[<check-id>]: <理由>`。")
        print("  3. baseline：`make inert-check-update` 後逐項填寫 reason（config 參數只能走這條）。")
        print("  規格：docs/specs/inert-static-check-spec.md")
        return 1

    if missing:
        print("❌ baseline 有未填理由的項目 → 檢查失敗（理由必填；TODO 佔位不算理由）。")
        print("   請編輯 scripts/ci/inert-baseline.json 逐項填寫 reason 後重跑。")
        return 1

    print(f"✅ inert 閉環檢查通過（{len(violations)} 筆既有項由 baseline 覆蓋；"
          f"耗時 {round(time.time() - idx.t0, 2)}s）")
    return 0


def run_checks(idx):
    return (check_config_inert(idx) + check_writer_no_consumer(idx) + check_claims(idx))


if __name__ == "__main__":
    sys.exit(main())
