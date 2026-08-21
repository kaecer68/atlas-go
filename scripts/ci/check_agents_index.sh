#!/usr/bin/env bash
#
# check_agents_index.sh — 驗證 internal/AGENTS_INDEX.md 與實際模組一致
#
# 防止索引漂移（手動維護遺漏）：新增 internal/ 模組後若未補入 AGENTS_INDEX，
# 模型讀索引時看不到該模組 → 幻覺。本檢查強制兩者一致。
#
# 檢查項目：
#   1. AGENTS_INDEX.md 中每個 internal/ 條目對應的目錄必須存在且含 .go
#      （排除 cmd/atlas-mcp/server 特殊條目與 archived 保留條目 swarm）
#   2. 每個實際存在且含 .go 的 internal/ 頂層模組必須在 AGENTS_INDEX.md 中
#      （排除特殊條目：cmd/atlas-mcp/server）
#
# Exit 0 = 一致；Exit 1 = 漂移

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

INDEX="internal/AGENTS_INDEX.md"
FAILED=0
log_err() { printf "ERROR [agents-index] %s\n" "$1"; FAILED=1; }

echo "agents-index: checking AGENTS_INDEX.md vs actual internal/ modules..."

[[ -f "$INDEX" ]] || { echo "FATAL: $INDEX not found"; exit 1; }

# 從索引中提取 internal/ 模組條目（表格第一欄 `mod`）
indexed=$(grep -oE '^\| `[a-z_0-9]+`' "$INDEX" | sed 's/| `//; s/`//')

# 特殊條目：跨 internal/ 與 cmd/ 的條目（在索引中以 cmd/atlas-mcp/server 出現）
# archived 保留條目 swarm（目錄已刪，歷史參考）

echo "--- Check 1: indexed modules exist ---"
for m in $indexed; do
    # 跳過非 internal 條目（cmd/...）
    case "$m" in
        cmd/*) continue ;;
    esac
    if [ ! -d "internal/$m" ]; then
        # swarm 是 archived 保留條目，允許目錄不存在
        if [ "$m" != "swarm" ]; then
            log_err "indexed module missing: internal/$m (dir does not exist)"
        fi
        continue
    fi
    if ! ls internal/$m/*.go >/dev/null 2>&1; then
        log_err "indexed module has no .go files: internal/$m"
    fi
done

echo "--- Check 2: actual modules are indexed ---"
for d in internal/*/; do
    m="$(basename "$d")"
    # 只檢查含 .go 的頂層模組
    ls "$d"*.go >/dev/null 2>&1 || continue
    # NOTE: use a here-string, not `echo "$indexed" | grep -qx`, because with
    # `set -o pipefail` the early-exit of `grep -q` SIGPIPEs echo and turns a
    # successful match into a spurious pipeline failure (flaky false alarms
    # observed 2026-08-21 on unrelated diffs).
    if ! grep -Fxq "$m" <<< "$indexed"; then
        log_err "module not indexed: internal/$m (add to $INDEX)"
    fi
done

echo ""
if [ "$FAILED" -eq 1 ]; then
    echo "agents-index: FAILED — AGENTS_INDEX.md drifted from actual modules"
    exit 1
fi
echo "agents-index: OK (AGENTS_INDEX.md matches actual internal/ modules)"
