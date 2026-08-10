#!/usr/bin/env bash
# =============================================================================
# check_critical_tasks.sh — 關鍵背景任務必須存在於 atlas binary
#
# 背景（2026-08-10 template_detector_scan 事故）：
#   cmd/atlas/main.go 的 `:=` 短宣告遮蔽函數級變數（detectorScanStore）
#   → Go compiler DCE 判定 `if x != nil && y != nil` 恆 false
#   → 整個 RegisterTemplateDetectorScanTasks 呼叫被編譯器移除
#   → 任務從未註冊（scheduler 85 tasks 無此任務）、模板 detector 從未掃描
#   → ci-gate / ci-full / GitHub CI 全綠（compile-only build 不驗證 binary
#     內容），監控器警告「11/14 模板未觸發」才暴露。
#
# 本檢查：build cmd/atlas → 驗證關鍵 task 名稱字串存在於 binary。
# 被 DCE 移除的 code（含其 string literal）不會出現在 binary → strings
# 找不到 → fail，提示檢查 := 遮蔽。
#
# 用法: bash scripts/ci/check_critical_tasks.sh
# 退出碼: 0 = 通過, 1 = 關鍵 task 缺失
# =============================================================================
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

# 關鍵 BTM 任務名稱（新增關鍵背景任務時，若屬「無聲故障高風險」，加入此清單）
CRITICAL_TASKS=(
  "template_detector_scan" # 2026-08-10 DCE 事故（:= 遮蔽）；24 模板 detector 掃描
  "auto_cycle_update"      # 產業 cycle 聚合（B01 CycleTracker 持久化依賴）
  "macro_ingest"           # 總經批次 ingestion 核心
)

BIN="$(mktemp)"
trap 'rm -f "$BIN"' EXIT

echo "  → building cmd/atlas for critical-task check..."
if ! go build -o "$BIN" ./cmd/atlas; then
  echo "    ❌ go build ./cmd/atlas 失敗"
  exit 1
fi

MISSING=0
for task in "${CRITICAL_TASKS[@]}"; do
  # 注意：不能用 grep -q 於管道 — grep -q 找到即提前退出 → strings 收到
  # SIGPIPE（exit 141）→ set -o pipefail 讓管道 exit 141 → 誤報 FAIL。
  # 用 grep -F + 重導向（讀完整輸入，無 SIGPIPE）。
  if ! strings "$BIN" | grep -F "$task" > /dev/null; then
    echo "    ❌ 關鍵 task '$task' 不存在於 atlas binary — 可能被 compiler DCE 移除"
    echo "       檢查 cmd/atlas/main.go 的 := 短宣告遮蔽（2026-08-10 template_detector_scan 事故）"
    echo "       驗證: strings <binary> | grep '$task'；對照: go build -gcflags='-N -l'"
    MISSING=1
  fi
done

if [ "$MISSING" -ne 0 ]; then
  echo "    ❌ 關鍵 task 缺失 — 修復後重跑"
  exit 1
fi
echo "    ✅ 所有關鍵 task（${#CRITICAL_TASKS[@]} 個）存在於 atlas binary"
