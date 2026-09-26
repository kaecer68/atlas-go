#!/usr/bin/env python3
"""check_shell_var_nonascii.py — `$VAR` 緊接非 ASCII 字元的靜態守門員（CI 閘門）

【為什麼需要】（2026-09-26 實證；atlas-go scripts/ci/check_finmind_quota.sh:65）
  在 UTF-8 locale 下，bash 會把非 ASCII 字元的**第一個位元組**當成變數名的一部分 ⇒
      bash -c 'set -u; V=1; echo "$V（"'      # bash: V\xef: unbound variable（實測 rc=127）
      bash -c 'set -u; V=1; echo "${V}（"'    # 正常（實測 rc=0）
  也就是說：**這不是「變數沒設」** ✗，是**展開語法**錯了 ⇒ 那一行**永遠**壞掉（與值無關）。
  `set -u`（本 repo 的 .sh 幾乎都有）⇒ 直接崩潰；沒開 `set -u` ⇒ **靜默**吞掉變數值（更難發現）。
  ⚠️ 這是**平台 + locale 相依**的：`LC_ALL=C`（單一位元組 locale）下不會發生 ⇒
     只看執行期永遠抓不到 ⇒ 必須靠本靜態檢查。

  實測表（2026-09-26，本檢查的存在理由就是這張表無法被執行期測試取代）：
  | 環境                                                | `echo "$V（"`          |
  |-----------------------------------------------------|------------------------|
  | macOS bash 5.3.9, `LC_ALL=C.UTF-8` / `en_US.UTF-8`  | rc=127 unbound variable |
  | macOS bash 5.3.9, `LC_ALL=C`                        | rc=0 `A[1（]B`          |
  | linux/glibc bash 5.2.15（debian bookworm，charmap=UTF-8）| rc=0 `A[1（]B`      |
  | linux/musl bash 5.3.20（alpine container）          | rc=0 `A[1（]B`          |
  機制（對照實測，不是推論）：macOS libc 的 `isalnum(0xEF)` 在 UTF-8 locale 回 1、在 `C` locale 回 0
  ——bash 的變數名判定走 ctype ⇒ 多吃了那一個位元組（所以名字是 `V\\xef`，只多吃 1 byte）。

【規則】`$NAME`（NAME 由 `[A-Za-z_]` 起頭）後**緊接**非 ASCII（≥ U+0080）⇒ 失敗。
  只有「真的會展開」的位置才算：未加引號、雙引號內、`$( … )` / `` ` … ` `` / `${ … }` 內、
  以及**未加引號 delimiter 的 heredoc body**。
【不算違規】**註解（詞首 `#` 之後）、單引號內、`$'…'`（ANSI-C）、`${NAME}`（已加括號）、
  `\\$NAME`（已轉義）、`$1`/`$?`/`$@`/`$#`/`$*`/`$!`/`$$`/`$-`/`$1 0`（單一位元組參數，實測在字元邊界停止）。

【修法】`$NAME` → `${NAME}`（**只加括號**；不要改成半形括號 ✗ —— 那會改動訊息內容/語意）。

【已知限制】（刻意保守，避免誤判；詳見檔尾 `# ── 實作說明`）
  ① `case` 分支裡的裸 `)` 會讓 `$( … )` 巢狀深度少算（提前關閉）⇒ 該區塊之後的引號狀態可能失準。
  ② heredoc body 內的 `$( … )` 跨行不追蹤。
  兩者都只影響「同一段文字裡的後續幾行」，且失敗方向是**可能漏報**，不會誤報乾淨的行。

【用法】
  bash scripts/ci/check_shell_var_nonascii.sh                 # 掃 tracked + untracked（未忽略）.sh
  bash scripts/ci/check_shell_var_nonascii.sh --json          # 機器可讀
  bash scripts/ci/check_shell_var_nonascii.sh FILE...         # 只掃指定檔（自我測試用；不必是 git repo）
  bash scripts/ci/check_shell_var_nonascii.sh --selftest-map  # 印「違規/安全」判定表
  bash scripts/ci/check_shell_var_nonascii.sh --no-baseline   # 忽略 baseline（要一次看全部就加這個）

【退出碼】0 = 乾淨（含 baseline 內已知項）；1 = 發現**新**違規（或掃到 0 個檔案 ⇒ fail-closed）；2 = 用法錯誤。

【baseline】scripts/ci/shell-var-nonascii-baseline.txt —— 只放「別人的 PR 正在修、本 PR 不得碰」的項，
  每項必填理由；比對成功會印 ⚠️ 提醒（不會靜默消失），`#2020 併入後刪除`的項若已失效會印 stale 警告。

【測試】tests/scripts/test-shell-var-nonascii.sh（hermetic fixture；斷言**精確 exit code**）
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys

NAME_RE = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
# heredoc delimiter 只認「像識別字」的形式（`<<EOF` / `<<'EOF'` / `<<-EOF`）⇒
# 不會把算術位移 `$(( x << 2 ))` 誤判成 heredoc（delimiter 是數字 ⇒ 不當 heredoc）。
HEREDOC_WORD_END = set(" \t\n;&|<>()")
BASELINE_NAME = "shell-var-nonascii-baseline.txt"


class Frame:
    """巢狀展開的一層引號狀態。

    kind: top（整個腳本）/ cmd（`$( … )`、`<( … )`）/ arith（`$(( … ))`）/
          param（`${ … }`）/ backtick（`` ` … ` ``）
    """

    __slots__ = ("kind", "state", "depth")

    def __init__(self, kind: str, depth: int = 0) -> None:
        self.kind = kind
        self.state = "none"  # none | single | double
        self.depth = depth


class Heredoc:
    __slots__ = ("delim", "expand", "strip_tabs")

    def __init__(self, delim: str, expand: bool, strip_tabs: bool) -> None:
        self.delim = delim
        self.expand = expand
        self.strip_tabs = strip_tabs


def _nonascii(ch: str) -> bool:
    return ord(ch) > 0x7F


def _parse_heredoc_operator(line: str, i: int):
    """line[i:i+2] == '<<' ⇒ (Heredoc|None, next_index)。`<<<` 與非識別字 delimiter 回 None。"""
    n = len(line)
    j = i + 2
    if j < n and line[j] == "<":  # here-string
        return None, i + 2
    strip_tabs = False
    if j < n and line[j] == "-":
        strip_tabs = True
        j += 1
    while j < n and line[j] in " \t":
        j += 1
    quote = None
    expand = True
    word = []
    while j < n:
        c = line[j]
        if quote:
            if c == quote:
                quote = None
                expand = False
                j += 1
                continue
            word.append(c)
            j += 1
            continue
        if c in "'\"":
            quote = c
            expand = False
            j += 1
            continue
        if c == "\\":
            expand = False
            if j + 1 < n:
                word.append(line[j + 1])
                j += 2
            else:
                j += 1
            continue
        if c in HEREDOC_WORD_END:
            break
        word.append(c)
        j += 1
    delim = "".join(word)
    if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", delim or ""):
        return None, j
    return Heredoc(delim, expand, strip_tabs), j


def _check_dollar(line: str, i: int):
    """line[i] == '$' ⇒ (違規結束索引|None, 下一個索引)。

    違規結束索引非 None ⇒ 這裡出現 `$name` 且緊接非 ASCII。
    """
    n = len(line)
    j = i + 1
    if j >= n:
        return None, i + 1
    c = line[j]
    if c in "{('\"":
        return None, j  # ${ / $( / $' / $" —— 由呼叫端處理（引號/巢狀）
    m = NAME_RE.match(line, j)
    if not m:
        # `$1` `$?` `$@` `$*` `$#` `$!` `$$` `$-` 等單一位元組參數：不吃後續位元組（實測）
        return None, j + 1
    end = m.end()
    if end < n and _nonascii(line[end]):
        return end, end
    return None, end


def scan_text(text: str):
    """掃 shell 文字 ⇒ [(lineno, col, snippet, bad_char)]。"""
    out = []
    frames = [Frame("top")]
    pending: list[Heredoc] = []
    in_body = False
    body: Heredoc | None = None

    for lineno, line in enumerate(text.split("\n"), 1):
        # ── heredoc body ────────────────────────────────────────────────
        if not in_body and pending:
            in_body = True
            body = pending.pop(0)
        if in_body and body is not None:
            probe = line.lstrip("\t") if body.strip_tabs else line
            if probe.rstrip("\r") == body.delim:
                in_body = False
                body = None
                continue
            if not body.expand:
                continue
            # heredoc body：`'`/`"` 是字面；只認 `$` 展開與 `\` 轉義
            i = 0
            while i < len(line):
                c = line[i]
                if c == "\\":
                    i += 2
                    continue
                if c == "$":
                    end, nxt = _check_dollar(line, i)
                    if end is not None:
                        out.append((lineno, i + 1, line.strip(), line[end]))
                    i = nxt
                    continue
                i += 1
            continue

        # ── 一般（可展開）文字 ──────────────────────────────────────────
        i = 0
        n = len(line)
        while i < n:
            fr = frames[-1]
            c = line[i]

            if c == "\\":
                # 單引號內反斜線是字面字元；其他情境跳過下一個字元（能正確處理 \$ / \\$）
                i += 1 if fr.state == "single" else 2
                continue

            if fr.state == "single":
                if c == "'":
                    fr.state = "none"
                i += 1
                continue

            if fr.state == "double":
                if c == '"':
                    fr.state = "none"
                    i += 1
                    continue
                if c == "`":
                    frames.append(Frame("backtick"))
                    i += 1
                    continue
                if c == "$":
                    if line.startswith("$(", i):
                        frames.append(Frame("arith" if line.startswith("$((", i) else "cmd", 1))
                        i += 3 if line.startswith("$((", i) else 2
                        continue
                    if line.startswith("${", i):
                        frames.append(Frame("param", 1))
                        i += 2
                        continue
                    end, nxt = _check_dollar(line, i)
                    if end is not None:
                        out.append((lineno, i + 1, line.strip(), line[end]))
                    i = nxt
                    continue
                i += 1
                continue

            # state == none
            if c == "'":
                fr.state = "single"
                i += 1
                continue
            if c == '"':
                fr.state = "double"
                i += 1
                continue
            if c == "`":
                frames.append(Frame("backtick"))
                i += 1
                continue
            if c == "#" and (i == 0 or line[i - 1] in " \t;|&()<>" ):
                break  # 註解（不展開）⇒ 該行剩餘部分略過
            if c == "<" and line.startswith("<<", i):
                hd, nxt = _parse_heredoc_operator(line, i)
                if hd is not None:
                    pending.append(hd)
                    i = nxt
                    continue
            if (c == "<" or c == ">") and line.startswith("(", i + 1):
                frames.append(Frame("cmd", 1))  # process substitution <( ) / >( )
                i += 2
                continue
            if c == "$":
                if line.startswith("$(", i):
                    frames.append(Frame("arith" if line.startswith("$((", i) else "cmd", 1))
                    i += 3 if line.startswith("$((", i) else 2
                    continue
                if line.startswith("${", i):
                    frames.append(Frame("param", 1))
                    i += 2
                    continue
                if line.startswith("$'", i) or line.startswith('$"', i):
                    fr.state = "single" if line[i + 1] == "'" else "double"
                    i += 2
                    continue
                end, nxt = _check_dollar(line, i)
                if end is not None:
                    out.append((lineno, i + 1, line.strip(), line[end]))
                i = nxt
                continue

            if fr.kind in ("cmd", "arith", "param"):
                opener, closer = ("{", "}") if fr.kind == "param" else ("(", ")")
                if c == opener:
                    fr.depth += 1
                    i += 1
                    continue
                if c == closer:
                    fr.depth -= 1
                    if fr.depth <= 0:
                        frames.pop()
                    i += 1
                    continue

            if fr.kind == "backtick" and c == "`":
                frames.pop()
                i += 1
                continue

            i += 1
    return out


def _display(ch: str) -> str:
    return "%s（U+%04X）" % (ch, ord(ch))


def find_shell_files(root: str):
    """git 為優先來源（tracked + untracked 未忽略）；非 git 環境回 None。"""
    try:
        p = subprocess.run(
            ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.sh"],
            cwd=root, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=False,
        )
    except OSError:
        return None
    if p.returncode != 0:
        return None
    return [f for f in p.stdout.decode("utf-8", "surrogateescape").split("\0") if f]


def collect(paths, root: str):
    found = []
    for p in paths:
        full = p if os.path.isabs(p) else os.path.join(root, p)
        if os.path.isdir(full):
            for dirpath, dirnames, filenames in os.walk(full):
                dirnames[:] = [d for d in dirnames if d not in (".git", "node_modules", "vendor")]
                for fn in filenames:
                    if fn.endswith(".sh"):
                        found.append(os.path.relpath(os.path.join(dirpath, fn), root))
        else:
            found.append(os.path.relpath(full, root))
    return sorted(set(found))


def load_baseline(root: str):
    """TSV：<path>\\t<offending 行內的片段>\\t<理由>；`#` 開頭為註解。"""
    path = os.path.join(root, "scripts", "ci", BASELINE_NAME)
    entries = []
    if not os.path.isfile(path):
        return entries
    with open(path, encoding="utf-8") as fh:
        for raw in fh:
            line = raw.rstrip("\n")
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) < 3:
                continue
            entries.append({"path": parts[0].strip(), "needle": parts[1], "reason": parts[2].strip()})
    return entries


SELFTEST_MAP = [
    ('echo "$V（"', "違規", "name 後緊接非 ASCII ⇒ bash 把 0xEF 吃進變數名"),
    ('echo $V（', "違規", "未加引號也一樣（展開與否跟引號無關）"),
    ('echo "$V（"  # 尾巴註解 $V（', "違規", "`#` 之後才算註解；`$V（` 在註解之前"),
    ('case "$TOKEN" in *"x"*) echo "（$V）" ;; esac', "違規", "`$( … )` 之外的巢狀引號不會讓狀態失準"),
    ('echo "${V}（"', "安全", "已加括號"),
    ('echo "$1（"', "安全", "單一位元組位置參數"),
    ('echo "$?（"', "安全", "單一位元組特殊參數"),
    ('echo "$@（"', "安全", "同上"),
    ('echo "$10（"', "安全", "`$10` = `$1` + 字面 `0`"),
    ('echo "$_（"', "違規", "`_` 是 name 字元 ⇒ 一樣被吃進去"),
    ("echo '$V（'", "安全", "單引號：不展開"),
    ("echo $'$V（'", "安全", "$'…'（ANSI-C）：不展開"),
    ('echo "\\$V（"', "安全", "已轉義的 $"),
    ("# echo $V（", "安全", "整行是註解"),
    ('X="$(sed -n \'s/a/b/p\' "$F" | tr -d "\'\\"[:space:]")"', "安全", "`$()` 內的多層引號不會讓外層狀態失準"),
    ('echo "（$(echo "$V（")）"', "違規", "`$()` 內真的違規"),
    ("echo `echo $V（`", "違規", "反引號內會展開"),
    ('echo "${V:-$W（}"', "違規", "`${ … }` 內會展開"),
    ('cat <<EOF\n$V（\nEOF', "違規", "未加引號 delimiter 的 heredoc body 會展開"),
    ("cat <<'EOF'\n$V（\nEOF", "安全", "引號 delimiter 的 heredoc body 不展開"),
    ("x=$(( 1 << 2 ))\necho \"$V（\"", "違規", "`<<` 位移不會被誤判成 heredoc（否則後面的違規會被吃掉）"),
]


def _selftest_map() -> int:
    print("判定對照表（檢查器對每個片段的判定；標『違規』者在 UTF-8 locale 下真的會壞）")
    bad = 0
    for snippet, want, why in SELFTEST_MAP:
        got = "違規" if scan_text(snippet) else "安全"
        if got != want:
            bad += 1
        print("  %s %-52r 期望=%-4s 實得=%-4s — %s" % ("✅" if got == want else "❌", snippet, want, got, why))
    return 1 if bad else 0


def main(argv) -> int:
    ap = argparse.ArgumentParser(add_help=True)
    ap.add_argument("paths", nargs="*", help="要掃的檔案/目錄（預設：git 內所有 .sh）")
    ap.add_argument("--root", default=None, help="repo 根（預設：本檔的 ../..）")
    ap.add_argument("--json", action="store_true", help="機器可讀輸出")
    ap.add_argument("--no-baseline", action="store_true", help="忽略 baseline（全部當新違規）")
    ap.add_argument("--selftest-map", action="store_true", help="印判定對照表後結束")
    args = ap.parse_args(argv)

    if args.selftest_map:
        return _selftest_map()

    root = os.path.abspath(args.root or os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))

    if args.paths:
        files = collect(args.paths, root)
    else:
        files = find_shell_files(root)
        if files is None:
            print("❌ shell-var-nonascii：找不到 git（請在 repo 內執行，或明確給檔案清單）", file=sys.stderr)
            return 2

    if not files:
        print("❌ shell-var-nonascii：掃到 0 個 .sh 檔（root=%s）" % root)
        print("   ⇒ 通常是**檔案發現邏輯**壞了（路徑/glob 改動）⇒ 不得靜默退化成「永遠 PASS 的空檢查」。")
        return 1

    baseline = [] if args.no_baseline else load_baseline(root)
    matched = [False] * len(baseline)
    violations = []
    baselined = []
    unreadable = []

    for rel in files:
        try:
            with open(os.path.join(root, rel), "rb") as fh:
                text = fh.read().decode("utf-8", "replace")
        except OSError as exc:
            unreadable.append((rel, str(exc)))
            continue
        for lineno, col, snippet, bad_char in scan_text(text):
            hit = None
            for k, ent in enumerate(baseline):
                if rel == ent["path"] and ent["needle"] in snippet:
                    hit = k
                    break
            item = {"file": rel, "line": lineno, "col": col, "snippet": snippet, "char": bad_char}
            if hit is None:
                violations.append(item)
            else:
                matched[hit] = True
                item["reason"] = baseline[hit]["reason"]
                baselined.append(item)

    stale = [ent for k, ent in enumerate(baseline) if not matched[k]]

    if args.json:
        print(json.dumps({"scanned": len(files), "violations": violations,
                          "baselined": baselined, "stale_baseline": stale,
                          "unreadable": [u[0] for u in unreadable]}, ensure_ascii=False, indent=2))
        return 1 if (violations or unreadable) else 0

    for rel, err in unreadable:
        print("❌ shell-var-nonascii：讀不到 %s（%s）" % (rel, err))

    for ent in stale:
        print("⚠️  baseline 已失效（不再是違規）⇒ 請刪掉 scripts/ci/%s 的這一行：" % BASELINE_NAME)
        print("     %s\t%s\t%s" % (ent["path"], ent["needle"], ent["reason"]))

    for it in baselined:
        print("⚠️  baseline 略過（已知項）：%s:%d  %s" % (it["file"], it["line"], it["snippet"]))
        print("    理由：%s" % it["reason"])

    if violations:
        print("❌ shell-var-nonascii FAIL（%d 筆；掃 %d 個 .sh）" % (len(violations), len(files)))
        for it in violations:
            print("   %s:%d:%d: `$…` 緊接非 ASCII %s" % (it["file"], it["line"], it["col"], _display(it["char"])))
            print("      原文：%s" % it["snippet"])
        print("")
        print("   為什麼是 bug（不是「變數沒設」）：在 UTF-8 locale 下 bash 會把「%s」的第一個位元組"
              % _display(violations[0]["char"]))
        print("   吃進變數名 ⇒ 展開成 `$NAME\\x…`（未定義）。`set -u` 腳本必然 unbound variable 崩潰；")
        print("   沒開 `set -u` 則**靜默**吞掉該變數的值（更難發現）。`LC_ALL=C` 下不發作 ⇒ 只有靜態檢查擋得住。")
        print("   修法：`$NAME` → `${NAME}(…)`（**只加括號**；不要改成半形括號，那會改動訊息內容）。")
        if unreadable:
            return 1
        return 1

    if unreadable:
        return 1

    extra = "" if not baselined else "，另略過 %d 筆 baseline 已知項" % len(baselined)
    print("✅ shell-var-nonascii PASS（掃 %d 個 .sh%s）" % (len(files), extra))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))

# ── 實作說明（為什麼要巢狀 frame stack）──────────────────────────────────────
# 第一版（只追單一 state）在 `TOKEN="$(sed -n 's/x/y/p' "$F" | tr -d "'\"[:space:]")"` 這種行上**失準**：
# 外層 `"` 開了之後，`$()` 內的新引號讓狀態被誤關／誤開 ⇒ 檔案後半全部被當成字面 ⇒ **漏報**
# （實測：install-webhook.sh:96 的真 bug 被吃掉）。
# 現在改成 frame stack：`$(` / `$((` / `${` / `` ` `` 各推一層，關閉時彈回**外層原本的引號狀態**。
# 巢狀深度用 depth 記；`<<` 只在 delimiter 像識別字時才當 heredoc（避免 `$(( x << 2 ))` 誤判）。
