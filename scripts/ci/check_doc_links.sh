#!/usr/bin/env bash
#
# check_doc_links.sh — 內部連結（斷鏈）檢查的**入口**；實作唯一在 check_markdown_links.sh
#
# ── 舊版為什麼「永遠不會紅」（FU-20261016-18；2026-10-16 本機實測）───────────────
# 舊版本檔內自己用 grep 找連結，兩個缺陷疊在一起 ⇒ 在 macOS 上**從來沒有檢查過任何一條連結**，
# 卻在 `make ci-gate` 印 `✅ All links OK`：
#
#   ① GNU-only flag：`grep -rnP '…'` —— macOS 的 BSD grep 不支援 `-P`：
#        $ grep -rnP '\[.*?\]\([^http#].*?\.md[#\)]' docs/ --include="*.md"
#        grep: invalid option -- P     ⇒ rc=2、stdout 0 行（實測 BSD grep 2.6.0-FreeBSD）
#   ② 同一行的 `2>/dev/null` 把 ① 的錯誤**吞掉** ⇒ process substitution 餵進 while 的是**空清單**
#      ⇒ 迴圈跑 0 次 ⇒ `FAILED` 恆為 0 ⇒ 尾端無條件印 `✅ All links OK`、exit 0。
#      在那份輸出裡，「工具壞了」與「沒有連結」**完全無法區分**。
#
# ── 修法：轉呼叫既有實作，不寫第二份 pattern ─────────────────────────────────────
# 同一棵樹已有一支**可用**的斷鏈檢查，而且就是 GitHub CI required context `markdown-links`
# 實際跑的那一支：`scripts/ci/check_markdown_links.sh`（蒐集 git-aware；判定在
# `check_markdown_links.py`；契約測試 tests/scripts/test-check-markdown-links.sh）。
# 因此本檔只做轉呼叫 —— 這是有意的設計選擇，不是「少寫幾行」：
#   · **一份實作** ⇒ 不會再產生「兩份 pattern 靜默分歧」的第二個同型缺陷；本票的根因正是
#     兩份實作之一悄悄失效，而沒有任何機制會發現。
#   · 本地 `make ci-gate` 跑的**就是** CI 那一支 ⇒ 本地閘門與 CI 不再各說各話。
#   · 覆蓋面是舊版的**超集**（實測，非推論）：舊版只掃 `docs/**/*.md` 的 `[]()` 相對連結；
#     新版把**同一批**檔案全部含進蒐集清單（`git ls-files` 實測 docs 內 208/208 個 `.md`，
#     零遺漏），並額外檢查全 repo `.md` 的內部連結與 backtick 裸路徑。三條 docs 排除目錄
#     （`docs/briefs`／`docs/handoff`／`docs/audit`）在本 repo**不存在**（實測 0 檔）
#     ⇒ 沒有任何檔案因此少檢查。
#
# ── 契約（fail loud：工具錯誤不得被當成「沒有連結」）──────────────────────────────
#   exit 0 = 真的檢查過，且全部有效
#   exit 1 = 找到壞連結
#   exit 2 = 環境／用法錯誤（帶了不接受的參數、找不到實作、不在 git work tree 內）
#   · 本檔**沒有** `2>/dev/null`、`|| true`、`grep -c` 當判定；被呼叫端的工具失敗
#     （git 不在 work tree 內、python3 不存在、`git ls-files` 失敗…）一律以非 0 收場 ⇒ 紅燈。
#   · 舊版的 `2>/dev/null` 就是「工具錯誤被吞掉」的來源 ⇒ 已移除，勿再引入。
#
# 契約測試：tests/scripts/test-check-doc-links.sh（hermetic fixture：壞連結必紅、實作缺失／
# 非 git 目錄必 fail loud、突變回舊版 grep 形態必紅、被 hook 呼叫不得污染呼叫者 repo）。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CALLEE="${SCRIPT_DIR}/check_markdown_links.sh"

# 帶參數 ⇒ 明確用法錯誤（exit 2）。刻意**不**轉傳給被呼叫端：它不吃參數，靜默忽略參數會變成
# 另一種「看起來做了、其實沒做」的形狀。
if [ "$#" -gt 0 ]; then
    echo "check_doc_links: 用法錯誤：本檔不接受參數（收到 $# 個：$*）—— 它只是 check_markdown_links.sh 的入口" >&2
    exit 2
fi

# 實作不存在 ⇒ 明確報錯（exit 2）。**不得**靜默當成「沒有連結可檢查」—— 那正是本檔修掉的形狀。
if [ ! -f "$CALLEE" ]; then
    echo "check_doc_links: 找不到實作 ${CALLEE} ⇒ 無法檢查（fail loud，不回報綠燈）" >&2
    exit 2
fi

echo "🔗 Scanning markdown links（docs/ + 全 repo 內部連結；實作：scripts/ci/check_markdown_links.sh）..."
exec bash "$CALLEE"
