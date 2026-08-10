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

# 關鍵 BTM 任務名稱（2026-08-10 code review 後收斂）：
# 只保留「binary 中唯一 literal」的檢查項 — 若該 task 名同時出現在
# 無關且恆可達的 code（log 字串 / DDL 預設值 / struct literal / 參數
# rationale），grep -F 子字串比對永遠找到 → 檢查假 PASS（false green）。
# 實證：macro_ingest 在 sqlite_core.go DDL、dashboard_api.go struct、
# main.go log 共 11 處；auto_cycle_update 在 defaults_narrative.go 參數
# rationale — 兩者註冊 block 被 DCE 移除也偵測不到。
# 新增檢查項時：確認 task 名只在註冊點出現（grep -r '<name>' 全 repo）。
CRITICAL_TASKS=(
  "template_detector_scan" # 2026-08-10 DCE 事故（:= 遮蔽）；binary 唯一 literal（實證 0→2）
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
