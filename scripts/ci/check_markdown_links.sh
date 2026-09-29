#!/usr/bin/env bash
#
# check_markdown_links.sh
#
# 驗證 **版控內** `.md` 檔案的內部連結（含 backtick 中的裸 .md 路徑）。
# 實作在 check_markdown_links.py — 此 shell 腳本僅負責蒐集檔案清單並經 stdin 傳參。
#
# ── 為什麼用 git 蒐集，而不是 find（issue #2114，2026-09-29）──────────────────────
# 舊版用 `find . -name '*.md'` 加一串 `-not -path` 排除。那份排除清單必須**手動**與
# `.gitignore` 保持同步，於是任何新加入 `.gitignore` 的目錄都會被繼續掃描：把
# PR 草稿／筆記放在那種目錄裡（例如 #2108 之後的 `session-artifacts/`），只要草稿含
# 相對連結，就能讓 required check 變紅 —— 而那些檔案永遠不會進版控（**假陽性**；
# 2026-09-28 實際發生過一次）。
#
# 實測（2026-09-29，主 clone）：find 掃到 40,444 檔，其中 39,691 檔（98%）是 gitignored 的
# 生成 prompt dump／產物／歸檔；改成 git-aware 後為 753 檔。少掉的 39,691 條**全部**
# 命中 `git check-ignore`（100%）、**被少掉的非 ignored 路徑 = 0**、**其中 tracked = 0**。
#
# ── 蒐集契約（改動前先讀完）────────────────────────────────────────────────────
#   git ls-files -c -o --exclude-standard -- '*.md'
#     · -c                ：**已版控**檔案一律檢查（即使它也命中 .gitignore ⇒ 照樣檢查）
#     · -o --exclude-standard：未版控但**未被忽略**的檔案也要檢查
#   ⇒ 三個世界被分開對待：
#       ① tracked                → 檢查（版控內容，壞連結就是壞連結）
#       ② untracked 且未 ignored → 檢查（那是**即將 commit 的草稿**；不檢查等於把紅燈
#                                  往後推到 commit 之後）
#       ③ ignored（untracked）   → **不檢查**（永不進版控；這正是本票要修的假陽性）
#   ⇒ 本檔**不再維護 .gitignore 的目錄清單**。請勿把 ignored 目錄名硬編回來：那等於
#     要求「腳本與 .gitignore 永遠同步」，下一個 ignored 目錄一定會再咬一次。
#
# ── 仍保留的排除（與舊 find 版**逐條等價**，改用 git pathspec）───────────────────
#   vendor/.gocache/.opencode/.gstack ＝ 僅頂層（對應舊 `-not -path './vendor/*'`）
#   其餘 ＝ 任何深度（對應舊 `-not -path '*/.worktrees/*'` 這種寫法）
#   ⚠️ 這些 `**/...` 樣式**必須**帶 `glob` magic（`:(exclude,glob)…`）：只用
#      `:(exclude)**/x/**` 在本機 git 2.52 **實測完全不生效**（所有排除靜默失效，
#      清單從 753 漲回 810）—— 這行不是裝飾，改壞會靜默放大掃描範圍。
#      `tests/scripts/test-check-markdown-links.sh` 有一條 case 專門釘住這件事。
#   `.git/` 不需要排除：git 本身不會列出 .git 內的檔案。
#
# 為避免 ARG_MAX (Linux: 2MB / macOS: 256KB) 限制，使用 xargs 將檔案清單分批傳遞。
#   -0：NUL 分隔（檔名含空白也安全）
#   -r：清單為空時**不呼叫** python（否則 python 收到「沒有參數」⇒ exit 2 ⇒ 假紅；
#       空 repo 或全部被排除時會走到這裡）
#
# Exit 0 = 全部有效；Exit 1 = 發現 broken 連結；Exit 2 = 不在 git work tree 內（用法/環境錯誤）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

cd "$REPO_ROOT"

# 蒐集來源是 git ⇒ 不在 work tree 內就沒有「版控檔案」可言。明確報錯優於靜默掃 0 檔
# （靜默 0 檔會回報「All internal markdown links are valid」，那是假綠）。
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "check_markdown_links: 不在 git work tree 內（本檢查以版控檔案為範圍，需要 git）" >&2
    exit 2
fi

git ls-files -z -c -o --exclude-standard -- '*.md' \
    ':(exclude,glob)vendor/**' \
    ':(exclude,glob).gocache/**' \
    ':(exclude,glob).opencode/**' \
    ':(exclude,glob)**/.worktrees/**' \
    ':(exclude,glob).gstack/**' \
    ':(exclude,glob)**/node_modules/**' \
    ':(exclude,glob)**/.omo/**' \
    ':(exclude,glob)**/docs/briefs/**' \
    ':(exclude,glob)**/docs/handoff/**' \
    ':(exclude,glob)**/docs/audit/**' \
    ':(exclude,glob)**/.claude/**' \
    ':(exclude,glob)**/.superpowers/**' \
    ':(exclude,glob)**/CHANGELOG.md' \
    | xargs -0 -r -n 200 python3 "${SCRIPT_DIR}/check_markdown_links.py"
