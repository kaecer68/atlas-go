#!/usr/bin/env bash
# check_sa12_negative_evidence.sh — SA12.A negative evidence close-out
#
# Checks confirming that legacy problems (duplicate weights, fake applied,
# nil current, synthetic ranking, etc.) are resolved.
#
# Usage: bash scripts/ci/check_sa12_negative_evidence.sh
# Exit: 0 = clean, 1 = evidence remains
#
# ── 2026-09-26（E9 / FU-20260926-19）：這支腳本原本沒有接進任何 gate ──────
# 舊檔名 `sa12-negative-evidence.sh` 不符 `make ci` 的 `scripts/ci/check_*.sh`
# glob，也不在任何 workflow 內 ⇒ 2026-07-19 建立後**沒有人跑過**，兩條
# 「命中檔案數 == N」的斷言因此在無人察覺下腐化（main 上固定 2 條 FAIL）：
#
#   08 'CapitalFlowActionRiskOn' 期望 2 檔：2026-07-27 (#1372) 起
#      internal/orchestrator/strategy_evolver.go 開始**消費** canonical 的
#      sectorallocation.CapitalFlowActionRiskOn（這正是 SA-INV-09 要的方向）
#      ⇒ 命中變 3 檔。真正的風險（第三份 taxonomy）不存在。
#   09 '"synthetic"' 期望 1 檔：2026-07-24 (2282122b) 之後陸續有檔案合法標註
#      「synthetic（OHLCV 回填列）」或**排除** synthetic 列（F06：只使用
#      non-synthetic outcome；見 internal/portfolio/darwinian_period_matrix.go）
#      ⇒ 命中變 5 檔。真正的風險（synthetic 餵進 ranking）不存在。
#
# 兩條「數檔案」斷言測不到它們宣稱的不變式（會被合法的消費者/標註撐破），
# 因此改為**直接量測不變式**：
#   08  → CapitalFlowAction 的定義點必須恰好兩份（canonical + 凍結的 deprecated 副本）
#   08b → deprecated 的 capitalflow.* action enum 必須零 production 消費者
#   09  → '"synthetic"' 不得與 rank/score/weight 同現
#   09b → F06 的 synthetic 排除守門必須存在（正向證據，>=1）
#
# 接線方式（**不改 Makefile／quality.yml**——該兩檔由其他 lane 佔用）：
# 檔名改成 `check_*` 後由 `make ci` 的既有 glob 自動納入
# （Makefile: `for script in scripts/ci/check_*.sh`）；`make ci` 由 `make ci-full`
# 呼叫，而 `make ci-full` 是 pre-push hook 與 PR lifecycle 的 MUST gate。
# ⚠️ 已知限制：GitHub Actions 端沒有任何 workflow 呼叫 `make ci`（實測
# `grep -rn "make ci" .github/workflows/` 無命中）⇒ 這是 local/pre-PR gate，
# 不是 GitHub required check。要變成 GH 端紅燈需動 quality.yml（已被佔用）。
# ⚠️ 通用教訓：「命中檔案數 == N」型斷言會隨合法程式碼成長而腐化；本檔 05/12
# 仍是此型（現為綠燈）。任何新增同型斷言都必須能被 gate 定期跑到才可靠。

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "$REPO_ROOT" || exit 1

passed=0
failed=0

neg() {
    # neg: expect exactly $3 (default 0) matches of the pattern in .go (non-test) files
    local desc="$1" pat="$2" expect="${3:-0}"
    local count
    count=$(grep -rl "$pat" internal/ --include="*.go" 2>/dev/null | grep -v '_test.go' | wc -l | tr -d ' ')
    if [[ "$count" -eq "$expect" ]]; then
        printf "PASS  %s (found=%d)\n" "$desc" "$count"
        passed=$((passed + 1))
    else
        printf "FAIL  %s (found=%d, expected=%d)\n" "$desc" "$count" "$expect"
        failed=$((failed + 1))
    fi
}

pos() {
    # pos: expect exactly N matches
    local desc="$1" pat="$2" expect="$3"
    local count
    count=$(grep -rl "$pat" internal/ --include="*.go" 2>/dev/null | grep -v '_test.go' | wc -l | tr -d ' ')
    if [[ "$count" -eq "$expect" ]]; then
        printf "PASS  %s (found=%d)\n" "$desc" "$count"
        passed=$((passed + 1))
    else
        printf "FAIL  %s (found=%d, expected=%d)\n" "$desc" "$count" "$expect"
        failed=$((failed + 1))
    fi
}

atleast() {
    # atleast: expect >= $3 matches（正向證據：守門存在即可，允許未來合法增加）
    local desc="$1" pat="$2" expect="$3"
    local count
    count=$(grep -rl "$pat" internal/ --include="*.go" 2>/dev/null | grep -v '_test.go' | wc -l | tr -d ' ')
    if [[ "${count:-0}" -ge "$expect" ]]; then
        printf "PASS  %s (found=%d, want>=%d)\n" "$desc" "$count" "$expect"
        passed=$((passed + 1))
    else
        printf "FAIL  %s (found=%d, want>=%d)\n" "$desc" "$count" "$expect"
        failed=$((failed + 1))
    fi
}

echo "=== SA12.A Negative Evidence Checks ==="

# ⚠️ fail-closed 前置（issue #2011）:這支腳本有多條「**必須 0 命中**」的負面證據檢查，
# 而所有計數都走 `grep -rl "$pat" internal/ ... | wc -l`。若 `internal/` 被搬走／改名，
# 或 grep 因為任何理由回空 ⇒ 每條都數到 0 ⇒ **全部 PASS**（「找不到」被當成「不存在」）。
# 所以先證明「掃描面」真的存在且有內容，再開始判定。
[ -d internal ] || { echo "❌ 找不到 internal/（cwd=$(pwd)）⇒ 0 命中不是『沒有殘留』，是『沒掃到東西』"; exit 1; }
scanned=$(grep -rl "" internal/ --include="*.go" 2>/dev/null | wc -l | tr -d ' ')
if [ "${scanned:-0}" -eq 0 ]; then
  echo "❌ internal/ 底下找不到任何 *.go（grep 失效或被改名）⇒ 後續 0 命中沒有意義"; exit 1
fi
echo "（掃描面：internal/**/*.go 共 ${scanned} 檔）"

neg "01 legacy fake ApplySectorRotation"        'Sector rotation applied for'
neg "02 duplicate _sectorWeights map (SA02)"     '_sectorWeights'
neg "03 unused sectorWeight function (SA02)"      'func sectorWeight'
neg "04 nil-provider partial WeightEngine"        'NewDefaultEngine.*nil, nil, nil, nil, nil, nil'
neg "05 second normalizeAllocations"              'normalizeAllocations' 1   # rotator's own normalization (not duplicate projection)
neg "06 direct base_weights merge in rotator"     'BaseWeights\[.*BaseAllocations'
neg "07 string-as-receipt"                        'receipt.*=.*"applied"'
# 08：SA-INV-09「單一 typed action enum」的正確測量是**定義點**，不是「提到該
# 識別字的檔案數」。2026-07-27 起 orchestrator 合法消費 canonical 常數（見檔頭）
# ⇒ 舊的 `'CapitalFlowActionRiskOn' 2` 必然假紅。定義點必須恰好兩份：
#   internal/sectorallocation/projector.go（canonical，production 使用）
#   internal/capitalflow/action_mapper.go（deprecated 凍結副本，待刪）
neg "08 no third CapitalFlowAction taxonomy (definition sites)" \
    'CapitalFlowActionRiskOn[[:space:]]*CapitalFlowAction[[:space:]]*=' 2
# 08b：那份凍結副本不得長出 production 消費者（一被使用，SA-INV-09 就破了）。
# 任何新檔案寫出 `capitalflow.CapitalFlowAction*`（含 *Mapper）⇒ 紅燈。
neg "08b deprecated capitalflow.* action enum has no production consumer" \
    'capitalflow\.CapitalFlowAction' 0
# 09：F06「無 synthetic ranking」的正確測量是「synthetic 是否被當成 ranking 輸入」，
# 不是「提到 synthetic 的檔案數」（合法標註／排除都會增加檔案數 ⇒ 舊的
# `'"synthetic"' 1` 必然假紅）。排除守門（MarketPeriodSource == "synthetic"）
# 本身是合規行為，不與 rank/score/weight 同現，因此不命中。
neg "09 synthetic never feeds rank/score/weight" \
    '"synthetic".*\(rank\|score\|weight\)\|\(rank\|score\|weight\).*"synthetic"' 0
# 09b：同時要求反向證據——排除 synthetic 列的守門必須真的存在，否則 09 的 0 命中
# 只代表「沒有人處理過 synthetic」。>=1 允許未來在更多 ranking 路徑補守門。
atleast "09b F06 synthetic-row exclusion guard present" \
    'MarketPeriodSource == "synthetic"\|is_synthetic = 0\|is_synthetic=0' 1
neg "10 non-canonical L1 base_allocations"        'base_allocations.*semiconductor'
neg "11 live sector mutation path"                'live.*ApplySectorRotation\|ApplySectorRotation.*live'

# positive check: ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED should appear
# exactly once (in cmd/atlas/main.go)
count=$(grep -rl 'ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED' cmd/ --include="*.go" 2>/dev/null | wc -l | tr -d ' ')
if [[ "$count" -eq 2 ]]; then
    printf "PASS  12 ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED in cmd/ (found=%d)\n" "$count"
    passed=$((passed + 1))
else
    printf "FAIL  12 ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED in cmd/ (found=%d, expected=2)\n" "$count"
    failed=$((failed + 1))
fi

total=$((passed + failed))
echo
echo "Passed: $passed / $total  Failed: $failed / $total"

if [[ "$failed" -eq 0 ]]; then
    echo "All negative evidence checks pass."
    exit 0
else
    echo "Negative evidence remains — SA12.A not yet complete."
    exit 1
fi
