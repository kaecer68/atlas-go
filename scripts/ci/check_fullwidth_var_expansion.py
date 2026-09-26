#!/usr/bin/env python3
r"""check_fullwidth_var_expansion.py — F28 護欄：掃出「$VAR 緊鄰多位元組字元」的陷阱。
（atlas-go 移植自 a2a-dev 同名工具；tokenizer 原樣沿用，擴充「檔案類型」與「CI 接法」見下方說明。）

【陷阱是什麼】
    bash 在 UTF-8 LC_CTYPE 下解析 `$VAR` 時，會把緊接在後的非 ASCII 位元組吃進變數名
    （`$V` + 全形括號 → 變數名 `V<0xEF>`）→ 該名字不存在：
      · `set -u`：`unbound variable` **直接中止**（即使 V 已定義）
      · `set +u`：**靜默展開為空**，尾隨位元組原樣輸出（輸出變成非法 UTF-8）
    只在「會展開」的語境成立；單引號字面、`#` 註解、引號 heredoc 主體、`\\$` 逃脫都不展開。

【為什麼不是一行 regex 就好】（2026-09-25 由獨立審查以 19 個對抗案例實證）
    天真 regex 會同時**偽陰性**（`echo a#$Z`＋全形括號 的 `#` 在字中間不是註解、
    `${P##*/}` 的 `#` 是參數運算子、`\\`+LF 行接續、跨行雙引號）與**偽陽性**
    （`<<'EOF'` 不展開的主體、跨行單引號字面、`$Z\\x80` 的 0x80 是接續位元組）。
    因此這裡做一個**小型 bash tokenizer**（只覆蓋本陷阱需要的語境）：逐位元組、跨行狀態機、
    heredoc 佇列、`${...}` 參數展開巢狀、`$()`/反引號巢狀、ANSI-C `$'...'`、`\\` 逃脫與行接續。

【已處理的對抗案例】（皆由獨立審查提供，並 regression 進 tests/fullwidth-var-expansion-selftest.sh）
    真陷阱必命中：`echo a#$Z`＋全形括號（字中間的 `#` 不是註解）、`${P##*/} "$Z`＋全形括號（參數運算子）、
      `\`+LF 行接續、跨行雙引號、未加引號 heredoc 主體（含 `#` 字面）、here-string `<<< "…"`、
      `$()`／反引號內、**雙引號內再巢狀 `$( … '字面' … )`**、巢狀 heredoc（含加引號的內層）、
      非 UTF-8 檔、`eval '…'`／`trap '…' EXIT`／`bash -c '…'`（遞延再解析）。
    不展開則不誤報：單引號字面（單行/跨行）、`#` 註解、`\$` 逃脫、`<<'EOF'`／`<<"EOF"`／`<<\EOF`
      主體、`$(cat <<'EOF' … EOF)`、巢狀引號 heredoc、`$Z` 後接**接續位元組**（如 `0x80`）、
      `${VAR}`、特殊參數 `$1`/`$@`。

【已知限制】（兩種方向都算缺陷，以下為尚未覆蓋者 —— 不是「靜默放過」而是明確列出）
    - 遞延再解析的**間接**形式：`xargs sh -c '…'`、`source <(…)`、把字串存進變數後 `eval "$v"`
      （白名單只涵蓋「指令緊鄰單引號參數」的直接形式）
    - heredoc 內再宣告 heredoc 的部分組合；`<<-` 只做 tab 剝除的基本處理
    - ANSI-C `$'…'` 的巢狀引號、`${VAR}` 內再巢狀 `${…}`、極端混用引號或超長行
    - `$( … )` 內用**未加引號**的裸 `)` 於非子殼語境（如 `case` 的 `)` 模式）→ 可能提早結束該語境
    上述若造成誤判，後果是 **掃描器 exit 1 ⇒ `ci-static` 直接變紅**（不是「印出可疑供人工複核」）——
    所以遇到時請以本檔為準修正掃描器或把該檔案加進已知清單，**不要**關掉這個步驟 ✗。
    掃描輸出永遠附「檔:行 + 變數名 + 首byte」供快速判斷。

用法（atlas-go）:
    python3 scripts/ci/check_fullwidth_var_expansion.py                # 掃 repo（見下方「檔案類型」）
    python3 scripts/ci/check_fullwidth_var_expansion.py --root DIR     # 掃指定目錄（測試用）
    python3 scripts/ci/check_fullwidth_var_expansion.py --quiet

exit code: 0 = 0 命中；1 = 有命中；2 = 用法錯誤

【atlas-go 移植與擴充（2026-09-26）】
    本檔**移植自 a2a-dev `scripts/check-fullwidth-var-expansion.py`（F28 護欄）**，
    tokenizer 逐行列為原樣沿用（不重寫第二套；a2a-dev 是這個陷阱的原生守門）。
    atlas-go 端擴充**兩件事**（原版只掃 *.sh）：
    ① **檔案類型**：`*.sh`、`Makefile`/`makefile`/`GNUmakefile`、`*.mk`、`*.py`、
       以及 `*.yml`/`*.yaml` 的 **`run:` 區塊**（只掃區塊，不掃整個 YAML：
       YAML 的 `name:`/`description:` 是資料不是 shell，而且整檔掃會被 YAML 的引號/反引號
       污染 tokenizer 狀態 → 偽陽性；實測 a2a-dev tokenizer 直接吃整份 quality.yml 會誤報
       步驟名稱那行）。
       · `Makefile` 掃整檔是刻意的：`$$VAR` 經 make 展開後就是 shell 的 `$VAR`，同一個 bug。
       · `*.py` 掃整檔：Python 檔案裡的 shell 片段（`subprocess` 字串、fixture 產生器）同樣會踩。
         因此**本檔與它的自我測試都必須自我乾淨**（自我測試用八進位轉義生成樣本，見
         `tests/scripts/test-fullwidth-var-expansion.sh` 檔頭）。
    ② **與 CI 的接法**：`.github/workflows/quality.yml` 的 blocking job `fullwidth-var-expansion`
       ＋ `make ci-gate`／`make ci`（`scripts/ci/check_*.sh` glob 自動納入）；負向證明**斷言精確
       exit code**（注入 fixture 必須 rc=1、移除必須 rc=0；127/126/2 都不算「擋下了」）。
"""
import argparse
import os
import re
import sys

# C locale（Latin-1 ctype）視為 alnum 的位元組範圍 → 只有落在 UTF-8 lead byte 的部分才可能觸發
# （0xAA/0xB5/0xBA 不是合法 UTF-8 lead byte；0xD7 是希伯來文 lead byte，C isalnum 不含）
_LEAD_BYTES = frozenset(list(range(0xC2, 0xD7)) + list(range(0xD8, 0xF5)))

_NAME = re.compile(rb"[A-Za-z_][A-Za-z0-9_]*")
_SKIP_DIRS = {".git", "node_modules", ".venv", "__pycache__", ".pytest_cache", ".mypy_cache"}
_WORD_BOUNDARY = b" \t\n;&|(){}<>"          # `#` 只有在字首（前面是空白/運算子/行首）才算註解


def _splice_continuations(data: bytes):
    """把 `\\` + LF 的行接續接起來（回傳 logical bytes 與「每個 logical 位元組的實際行號」）。

    為什麼要先 splice：`echo "pre $Z\\` + LF + `（post"` 這種寫法在 bash 眼中是**同一行**，
    陷阱就成立（2026-09-25 由獨立審查實證為天真逐行掃描的偽陰性）。
    """
    logical = bytearray()
    linenos = []
    line = 1
    i = 0
    n = len(data)
    while i < n:
        if data[i:i + 1] == b"\\" and data[i + 1:i + 2] == b"\n":
            i += 2
            line += 1
            continue
        logical.append(data[i])
        linenos.append(line)
        if data[i:i + 1] == b"\n":
            line += 1
        i += 1
    return bytes(logical), linenos


def _parse_heredoc_decls(data: bytes, start: int):
    r"""從 `<<` 起點解析 heredoc 宣告（可能同時多個）→ ([(delim, expanded)], 結束位移)。

    支援：`<<EOF`、`<<"EOF"`、`<<'EOF'`、`<<\EOF`、`<<-EOF`（tab 剝除）。引號分隔 = 主體不展開。
    """
    decls = []
    i = start
    n = len(data)
    while i < n and data[i:i + 2] == b"<<":
        if data[i + 2:i + 3] == b"<":       # here-string `<<<`
            break
        k = i + 2
        if data[k:k + 1] == b"-":
            k += 1
        while data[k:k + 1] in (b" ", b"\t"):
            k += 1
        q = data[k:k + 1]
        if q in (b"'", b'"'):
            end = data.find(q, k + 1)
            if end == -1:
                break
            decls.append((data[k + 1:end], False))
            i = end + 1
        elif q == b"\\":
            m = _NAME.match(data, k + 1) or re.match(rb"[^\s;|&<>()\n]+", data[k + 1:])
            if not m:
                break
            decls.append((m.group(0), False))
            i = k + 1 + len(m.group(0))
        else:
            m = _NAME.match(data, k) or re.match(rb"[^\s;|&<>()\n]+", data[k:])
            if not m:
                break
            decls.append((m.group(0), True))
            i = k + len(m.group(0))
        while data[i:i + 1] in (b" ", b"\t"):
            i += 1
    return decls, i


def _heredoc_decls(line: bytes):
    """在某一行裡找 heredoc 宣告（供 heredoc 主體內巢狀使用）。"""
    k = line.find(b"<<")
    if k == -1:
        return []
    if line[k + 2:k + 3] == b"<":
        return []
    decls, _ = _parse_heredoc_decls(line, k)
    return decls


def scan_bytes(data: bytes):
    """回傳 [(line, name_bytes, byte_value)]；line 為 1-based 的**實際行號**。"""
    hits = []
    data, linenos = _splice_continuations(data)
    i = 0
    n = len(data)

    def record(offset, name, byte):
        hits.append((linenos[offset] if offset < len(linenos) else linenos[-1], name, byte))

    # 狀態
    state = "normal"          # normal | single | double | ansi
    brace_depth = 0           # ${ ... } 巢狀（>0 = 在參數展開內，`#` 不算註解）
    backtick = False
    # 「遞延再解析」白名單：這些指令的**單引號參數**之後會被 bash 重新解析
    #（eval '…' / bash -c '…' / trap '…' EXIT / ssh host '…'），所以內容仍會展開 →
    # 不能當成單引號字面遮罩掉（2026-09-25 由獨立審查以 eval/trap/bash -c 三例實證為偽陰性）。
    REPARSE_WORDS = {b"eval", b"-c", b"-lc", b"-ic", b"trap", b"source", b".",
                     b"ssh", b"sh", b"bash", b"zsh", b"sudo", b"env", b"xargs", b"command"}
    # `$(` 的語境堆疊：[外層 state, 外層 brace_depth, 本層的普通括號深度]。
    # 為什麼需要「本層深度」：`"$(cd "$(dirname "$x")" )"` 這種巢狀裡，若用**全域**括號計數，
    # 內層 `$(` 會讓外層雙引號永遠無法關閉 → 之後整份檔案都被當成「雙引號內」→
    # 註解不遮罩、單引號字面被當展開 → 偽陽性/偽陰性同時出現（2026-09-25 由獨立審查實證）。
    subst_stack = []
    # 「引號語境回復堆疊」（**移植後修正**，2026-09-26）：a2a-dev 版的 `reparse`（eval/trap/bash -c
    # 的單引號參數會再解析）在區間內出現雙引號時，收尾一律寫回 "normal" ⇒ 該 reparse 的收尾單引號
    # 之後被當成「開啟單引號字面」⇒ **整份檔案後半被遮罩** ⇒ 偽陰性。
    # atlas-go 實測：`tests/scripts/test-install-webhook.sh`（3 處）、
    # `tests/scripts/test-binary-freshness-guard.sh`（2 處）因此被漏掉（同檔內先有 `bash "…" …`）。
    # 修法：把「進入雙引號／ANSI-C 前的語境」推上堆疊，收尾時回復（不是一律寫回 normal）。
    restore = []
    last_word = b""            # 上一個「已完成」的字（空白/運算子為界）
    cur_word = b""             # 目前累積中的字
    pending_heredocs = []     # [(delimiter: bytes, expanded: bool)]
    heredoc = None            # (delimiter, expanded)
    prev = b""

    def expanding():
        return state in ("normal", "double") and heredoc is None

    while i < n:
        c = data[i:i + 1]

                # ── heredoc 主體（優先於其他狀態）──────────────────────────────
        #     主體內**可以再宣告 heredoc**（巢狀）。語意與外層相同：分隔符有引號＝主體不展開（遮罩），
        #     無引號＝展開（逐行掃描，且可在其中再巢狀）。2026-09-25 由獨立審查實證：
        #     天真實作會在「未加引號外層 + 加引號內層」時誤報（內層內容其實是字面）。
        if heredoc is not None:
            delim, expanded = heredoc
            j2 = data.find(b"\n", i)          # 主體的一行（logical data，行接續已 splice）
            if j2 == -1:
                j2 = n
            line_raw = data[i:j2]
            if line_raw.lstrip(b"\t") == delim or line_raw == delim:
                heredoc = None
                i = j2 + 1 if j2 < n else n
                prev = b"\n"
                continue
            if expanded:
                # 未加引號的 heredoc：主體會展開 → 一般語境（引號/`#` 在主體內都只是字面）
                inner = _heredoc_decls(line_raw)
                if inner:
                    # 本行宣告了內層 heredoc → 其主體從下一行開始（依宣告順序）
                    pending_heredocs[:0] = inner
                    i = j2 + 1 if j2 < n else n
                    if i < n and pending_heredocs:
                        heredoc = pending_heredocs.pop(0)
                    prev = b"\n"
                    continue
                m = 0
                while m < len(line_raw):
                    k = line_raw.find(b"$", m)
                    if k == -1:
                        break
                    nm = _NAME.match(line_raw, k + 1)
                    if nm:
                        nxt = line_raw[nm.end():nm.end() + 1]
                        if nxt and nxt[0] in _LEAD_BYTES:
                            record(i + k, nm.group(0), nxt[0])
                        m = nm.end()
                    else:
                        m = k + 1
            i = j2 + 1 if j2 < n else n
            prev = b"\n"
            continue

        # ── 換行：logical 行結束 → 若前面有 heredoc 宣告，主體從下一行開始 ──
        if c == b"\n":
            i += 1
            prev = c
            last_word = b""; cur_word = b""
            if state in ("normal", "reparse") and pending_heredocs:
                heredoc = pending_heredocs.pop(0)
            continue

        # 追蹤字（空白/運算子為界；保留「上一個完成的字」）→ 供 eval/trap/-c 的遞延再解析判定
        if state in ("normal", "reparse"):
            if c in b" \t;&|(){}\'\"":
                if cur_word:
                    last_word = cur_word
                    cur_word = b""
            else:
                cur_word = (cur_word + c)[-24:]

        # ── 引號狀態 ───────────────────────────────────────────────────
        if state == "single":
            if c == b"'":
                state = "normal"
            i += 1
            prev = c
            continue
        if state == "reparse":
            if c == b"'":
                state = "normal"
                i += 1
                prev = c
                continue
            # 其餘落回下面的 normal 語意（展開、`#` 註解、巢狀引號由 bash 在解析時決定）
        if state == "ansi":
            if c == b"\\":
                i += 2
                prev = c
                continue
            if c == b"'":
                state = restore.pop() if restore else "normal"
            i += 1
            prev = c
            continue
        if state == "double":
            if c == b"\\":
                i += 2
                prev = c
                continue
            if c == b'"' and brace_depth == 0:
                # 不需要看括號深度：進入 `$(` 時外層引號狀態已被推上 subst_stack，
                # 這裡看到的 `"` 一定屬於**當前**語境（bash 的引號語境是巢狀的）。
                state = restore.pop() if restore else "normal"   # 回復（可能是 reparse）
                i += 1
                prev = c
                continue
            # 雙引號內仍會展開 → 繼續往下做 $VAR 偵測（見檔尾統一處理）

        # ── 註解：只有字首的 `#`（且不在 ${...} 內）─────────────────────
        if state in ("normal", "reparse") and brace_depth == 0 and c == b"#" and (prev == b"" or prev in _WORD_BOUNDARY or prev in b"\n"):
            k = data.find(b"\n", i)
            i = n if k == -1 else k
            continue

        # ── heredoc 起點 ───────────────────────────────────────────────
        if state in ("normal", "reparse") and c == b"<" and data[i:i + 2] == b"<<":
            k = i + 2
            if data[k:k + 1] == b"<":      # `<<<`（here-string）不是 heredoc → 整段跳過
                i = k + 1                  # 跳過三個 `<`，後續參數照一般語境掃描
                prev = b"<"
                continue
            decls, end_off = _parse_heredoc_decls(data, i)
            if decls:
                pending_heredocs.extend(decls)
                i = end_off
                prev = b">"
                continue

# ── 引號／參數展開／命令替換的開頭 ─────────────────────────────
        if state in ("normal", "double", "reparse"):
            if c == b"'" and state in ("normal", "reparse"):
                _w = cur_word or last_word
                state = "reparse" if _w in REPARSE_WORDS else "single"
                i += 1
                prev = c
                continue
            if c == b'"' and brace_depth == 0:
                restore.append(state)
                state = "double"
                i += 1
                prev = c
                continue
            if c == b"$" and data[i + 1:i + 2] == b"'":
                restore.append(state)
                state = "ansi"
                i += 2
                prev = c
                continue
            if c == b"$" and data[i + 1:i + 2] == b"{":
                brace_depth += 1
                i += 2
                prev = c
                continue
            if c == b"}" and brace_depth > 0:
                brace_depth -= 1
                i += 1
                prev = c
                continue
            if c == b"$" and data[i + 1:i + 2] == b"(":
                # `$( … )` 內部是**新的引號語境**（`"$(echo 'lit')"` 的單引號是字面）
                subst_stack.append([state, brace_depth, 0, list(restore)])
                brace_depth = 0
                state = "normal"
                i += 2
                prev = c
                continue
            if c == b"(" and subst_stack:
                subst_stack[-1][2] += 1          # 本層的普通子殼（`$( ( cmd ) )`）
                i += 1
                prev = c
                continue
            if c == b")" and subst_stack and state in ("normal", "reparse"):
                if subst_stack[-1][2] > 0:
                    subst_stack[-1][2] -= 1
                else:
                    state, brace_depth, _, restore = subst_stack.pop()
                    restore = list(restore)
                i += 1
                prev = c
                continue
            if c == b"`":
                backtick = not backtick
                i += 1
                prev = c
                continue
            if c == b"\\" and state in ("normal", "reparse"):
                i += 2
                prev = c
                continue

        # ── 本陷阱的偵測：expanding 語境下的 `$NAME` + lead byte ───────
        if c == b"$" and (state in ("normal", "double", "reparse")):
            m = _NAME.match(data, i + 1)
            if m:
                nxt = data[m.end():m.end() + 1]
                if nxt and nxt[0] in _LEAD_BYTES:
                    record(i, m.group(0), nxt[0])
                i = m.end()
                prev = data[m.end() - 1:m.end()]
                continue

        i += 1
        prev = c
    return hits

# ── atlas-go 擴充：要掃的檔案類型 ────────────────────────────────────────────
# 為什麼不只是 *.sh（2026-09-26）：
#   · 本 repo 實際命中的 7 處裡有 2 處在 `.github/workflows/quality.yml` 的 `run:` 區塊內
#     ⇒ YAML 一定得納入，否則這類 bug 會從 CI 腳本自己長出來。
#   · `Makefile` 的 `$$VAR` 經 make 展開後就是 shell 的 `$VAR`（同一個 bug）。
#   · `*.py` 內的 shell 片段（subprocess 字串、契約測試的 fixture 產生器）同樣會踩。
_SHELL_SUFFIXES = (".sh", ".mk")
_PY_SUFFIXES = (".py",)
_YAML_SUFFIXES = (".yml", ".yaml")
_MAKEFILE_NAMES = ("Makefile", "makefile", "GNUmakefile")
_SKIP_DIRS = _SKIP_DIRS | {"vendor", "dist", ".gocache", ".opencode", ".idea", ".vscode"}

# GitHub Actions / YAML 的 `run:`（含 `- run:`）。`|`/`>` 是區塊純量，其餘為 inline 指令。
_RUN_RE = re.compile(r"^(\s*)(?:-[ \t]+)*run:[ \t]*(.*)$")


def _yaml_run_blocks(text: str):
    """抽出 YAML 內所有 `run:` 區塊 → [(block_text, line_map)]。

    line_map[i] = block_text 第 (i+1) 個邏輯行在**原檔**的 1-based 行號（供回報正確行號）。

    為什麼只掃 `run:` 區塊而不是整檔（實測，不是推測）：
      把整份 `.github/workflows/quality.yml` 丟進本 tokenizer ⇒ 會誤報 `name:` 裡的字面樣式
      （YAML 的 `name:` 是資料；而且 `- name: … `$VAR（`…` 的反引號會讓 tokenizer 進入
      命令替換語境），並且 YAML 的引號/反引號會污染跨檔狀態。抽出區塊並**各自從乾淨狀態**掃描，
      兩個問題同時消失。
    """
    lines = text.split("\n")
    blocks = []
    i = 0
    total = len(lines)
    while i < total:
        m = _RUN_RE.match(lines[i])
        if not m:
            i += 1
            continue
        indent = len(m.group(1))
        rest = m.group(2).strip()
        if rest[:1] in ("|", ">"):          # 區塊純量 → 取後續更縮排的行
            body = []
            j = i + 1
            while j < total:
                ln = lines[j]
                if ln.strip() == "":
                    body.append((j + 1, ln))
                    j += 1
                    continue
                if (len(ln) - len(ln.lstrip(" \t"))) <= indent:
                    break
                body.append((j + 1, ln))
                j += 1
            while body and body[-1][1].strip() == "":
                body.pop()
            if body:
                base = min(len(ln) - len(ln.lstrip(" \t")) for _, ln in body)
                stripped = [ln[base:] if len(ln) >= base else ln for _, ln in body]
                blocks.append(("\n".join(stripped) + "\n", [n for n, _ in body]))
            i = j
            continue
        if rest:                             # `run: bash foo.sh`（inline）
            blocks.append((rest + "\n", [i + 1]))
        i += 1
    return blocks


def _iter_targets(root):
    """走訪要掃的檔案（略過 _SKIP_DIRS）。"""
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = sorted(d for d in dirnames if d not in _SKIP_DIRS)
        for fn in sorted(filenames):
            if fn in _MAKEFILE_NAMES or fn.endswith(
                    _SHELL_SUFFIXES + _PY_SUFFIXES + _YAML_SUFFIXES):
                yield os.path.join(dirpath, fn)


def main() -> int:
    ap = argparse.ArgumentParser(add_help=True)
    # 本檔在 scripts/ci/ ⇒ 往上三層才是 repo 根（a2a-dev 版放在 scripts/ ⇒ 兩層）
    ap.add_argument("--root", default=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
    ap.add_argument("--quiet", action="store_true")
    args = ap.parse_args()
    root = args.root
    if not os.path.isdir(root):
        print("check_fullwidth_var_expansion: --root 不存在: %s" % root, file=sys.stderr)
        return 2

    def report(rel, ln, name, byte, kind):
        print("%s:%d: $%s%s  ← 變數名後緊接 0x%02X（多位元組字元首byte）%s"
              % (rel, ln, name.decode("ascii", "replace"),
                 bytes([byte]).decode("latin-1"), byte, kind))

    scanned = 0
    total = 0
    for p in _iter_targets(root):
        try:
            data = open(p, "rb").read()
        except OSError:
            continue
        rel = os.path.relpath(p, root)
        if p.endswith(_YAML_SUFFIXES):
            scanned += 1
            for block, line_map in _yaml_run_blocks(data.decode("utf-8", "replace")):
                for lno, name, byte in scan_bytes(block.encode("utf-8", "replace")):
                    total += 1
                    if not args.quiet:
                        orig = line_map[lno - 1] if 0 <= lno - 1 < len(line_map) else line_map[-1]
                        report(rel, orig, name, byte, "（YAML run: 區塊）")
            continue
        scanned += 1
        for lno, name, byte in scan_bytes(data):
            total += 1
            if not args.quiet:
                report(rel, lno, name, byte, "")

    print("SCANNED=%d HITS=%d" % (scanned, total))
    return 1 if total else 0


if __name__ == "__main__":
    sys.exit(main())
