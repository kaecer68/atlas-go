#!/usr/bin/env bash
# scripts/ci/ci-full-trigger.sh — 「這次 diff 要不要跑 ci-full」的**唯一**判定實作。
#
# 用法: bash scripts/ci/ci-full-trigger.sh [<diff-range>]      (預設 origin/main...HEAD)
#
# 退出碼契約（三態；呼叫端必須分別處理，不得把 2 併進 1）:
#   0  diff 含程式碼／值變更   ⇒ 呼叫端**必須**跑 ci-full
#   1  **而且只有**「真的沒有程式碼變更」（docs-only）⇒ 呼叫端可跳過 ci-full
#   2  無法判定（git 失敗／工具缺失／range 不可解析）⇒ 呼叫端**必須 fail loud**
#
# 【為什麼要有這支】（FU-20261016-21；與 E4／FU-20261016-19 同族）
#   修前 Makefile:46 與 .githooks/pre-push:180 各自寫一份：
#       if git diff --name-only origin/main...HEAD 2>/dev/null | grep -qE '<pattern>'; then …
#   `git diff` 失敗時 stdout 是**空字串**（`2>/dev/null` 連 `fatal:` 都吞掉），而「真的只改文件」
#   同樣是空 stdout ⇒ 兩者在管線輸出上**逐字相同**，一律落到 else 分支 ⇒ 判為 docs-only
#   ⇒ ci-full（lint/staticcheck/test/race/coverage）**靜默跳過**。實測（2026-10-16）：
#     * range 不可解析（`origin/does-not-exist...HEAD`）⇒ `git diff` rc=128、管線 rc=1（＝docs-only）
#     * 不在 git work tree ⇒ rc=129、管線 rc=1
#     * shallow clone（base ref 在、但無 merge base）⇒ `fatal: … no merge base` rc=128、管線 rc=1
#   ⇒ 這支把「工具失敗」與「真的沒改碼」分開：前者 rc=2（fail loud），後者才是 rc=1。
#
#   同一份判準也只有這裡一份。修前兩份**已經分歧**：`|^configs/`（PR #2176，2026-09-30 只改
#   `.githooks/pre-push`）沒有同步進 `Makefile:46` ⇒ `make pre-push`（Makefile:40 明示「同 hook 邏輯」）
#   在**只改 configs/** 的 push 上仍判 docs-only 而跳過 ci-full。收斂成一支即消滅這個分歧。
#
# 判準（CODE_PATTERN）:「改了會影響行為」的檔案 —— 程式碼、建置／CI／hook 設定，以及 configs/
#   全目錄（值／治理檔：parameters*.json、monitor_limits.json、stress_index_weights.json、
#   sector_symbols.json、etf_metadata.json、portfolio_allocation.json、agents.json、llm_router.yaml、
#   methodology_rules.yaml、realtime.yaml、parameters/ 目錄、constitution 讀的 allowed_env_vars.md）。
#   configs/ 以**整個目錄**為準而非只納 `parameters*`：只納前者會漏掉同為值變更的 monitor_limits.json 等。
#   ⚠️ 不得為了省時間把這條 pattern 縮窄（縮窄＝把值變更降級成 docs-only）。
#
# 本檔**不**吞錯（no `2>/dev/null`、no `|| true`、no `grep -c`）：git 的 `fatal:` 必須讓操作者看到。
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
用法: bash scripts/ci/ci-full-trigger.sh [<diff-range>]

  <diff-range>  交給 `git diff --name-only` 的 range（預設 origin/main...HEAD）

退出碼: 0=含程式碼變更（跑 ci-full）／1=真的 docs-only（可跳過 ci-full）／2=無法判定（fail loud）
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac
[ "$#" -le 1 ] || { echo "❌ ci-full-trigger: 只接受 1 個參數（收到 $# 個）" >&2; usage; exit 2; }

RANGE="${1:-origin/main...HEAD}"

# 「改了會影響行為」的檔案（單一真相；見檔頭註解）
CODE_PATTERN='\.(go|ts|tsx|js|jsx|vue|py|sh|c|cpp|h)$|(^|/)Makefile$|Dockerfile|docker-compose|\.github/|\.githooks/|^configs/'

# git 的返回碼必須**顯式**接住：不導掉 stderr（診斷要靠 git 自己的 `fatal:`），
# 也不把非 0 當成「沒有變更」。
diff_rc=0
changed=$(git diff --name-only "$RANGE") || diff_rc=$?
if [ "$diff_rc" -ne 0 ]; then
  echo "" >&2
  echo "❌ ci-full-trigger: 無法用 git 解析 diff range '$RANGE'（git rc=${diff_rc}）" >&2
  echo "   「取不到 diff」不等於「只改文件」——不以 docs-only 放行（fail loud）。" >&2
  echo "   修復: 讓 range 可解析（例: git fetch origin main && git rev-parse origin/main），" >&2
  echo "         或在真的不想跑 ci-full 時明示覆寫（PRE_PUSH_FULL=never），不要靠工具失敗靜默跳過。" >&2
  echo "" >&2
  exit 2
fi

if printf '%s\n' "$changed" | grep -qE "$CODE_PATTERN"; then
  exit 0
fi
exit 1
