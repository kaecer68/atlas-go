# AGENTS.md — scripts/openclaw

本目錄是 **OpenClaw 治理引擎**的 shell script 實作，負責實驗生命週期的自動化、安全閘門檢查與人機協作決策。

---

## OVERVIEW

OpenClaw 是 `atlas-go` 的治理層，將實驗驅動的開發流程（propose → execute → judge → promote/revert）封裝為可重複執行的腳本。所有腳本遵循 `set -euo pipefail`，確保錯誤即時中斷。

---

## 核心職責

### 1. 日常執行 (`today_start.sh`)
- 每日自動化啟動流程：`status` → `propose(auto)` → `execute(auto)` → `judge(auto)` → 決策提醒。
- 支援視窗模式（window mode）：可指定回測日期區間。
- 智慧變異選擇：若當前 mutation type 無效，自動嘗試替代方案（prompt_tightening → risk_rule_change → portfolio_constraint_revision）。

### 2. 決策輔助 (`decide.sh`)
- 輔助 promote/revert/skip 決策，提供安全檢查與互動確認。
- `--dry-run` 預覽模式：不實際執行，僅輸出預計操作。
- `--yes` 自動確認：用於 CI 或自動化 pipeline。

### 3. 變異提案 (`propose_mutation.sh`)
- 根據 agent 績效與歷史實驗結果，自動產生 mutation brief。
- 支援多種變異類型：`prompt_tightening`、`risk_rule_change`、`portfolio_constraint_revision`。

### 4. 閘門驗證
- `verify_governance_gates.sh`：驗證 G2 replay 確定性、G3 hard-guard 阻擋行為、G4 trace 持久化、M5 多場景一致性、M7 approval event 可重播性。
- `verify_operations_gate.sh`：操作層面檢查（部署就緒性）。
- `verify_parallel_scenarios.sh`：平行場景驗證。

### 5. 實驗執行與評判
- `execute_next.sh`：執行下一個待處理的 mutation brief。
- `judge_latest.sh`：自動評判最新的實驗結果。
- `run_validated_round.sh`：執行已驗證的完整實驗回合。

### 6. 狀態與管理
- `status.sh`：查詢當前實驗與 baseline 狀態。
- `onboard.sh`：新環境初始化與設定檔檢查。
- `human_approval.sh`：人工審核事件處理。

---

## CONVENTIONS

- **錯誤處理**：所有腳本開頭 `set -euo pipefail`，任何命令失敗立即中斷。
- **路徑解析**：使用 `ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"` 確保從任何位置呼叫都能正確定位專案根目錄。
- **色彩輸出**：使用標準 ANSI 色彩碼（RED/GREEN/YELLOW/BLUE/CYAN/NC）標記錯誤、警告與成功狀態。
- **日誌格式**：統一使用 `[script-name] message` 格式輸出，方便追蹤。

---

## ANTI-PATTERNS

- **不可手動修改 baseline_policy.json**：所有政策變更必須透過 `promote-baseline` 或 `revert-baseline` 命令，禁止直接編輯 JSON。
- **不可跳過閘門**：`verify_governance_gates.sh` 失敗時禁止繼續 promote，必須先修復問題。
- **不可在生產環境執行 propose**：`propose_mutation.sh` 應在開發或 staging 環境執行，避免污染生產實驗歷史。
- **不可忽略 dry-run**：`decide.sh --dry-run` 輸出應仔細審查後再執行實際操作。

---

## 常用指令

```bash
# 查看當前狀態
bash ./scripts/openclaw/status.sh

# 驗證治理閘門（開發後必跑）
bash ./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity

# 執行完整日常流程（dry-run 預覽）
bash ./scripts/openclaw/today_start.sh --dry-run

# 決策輔助（promote 預覽）
bash ./scripts/openclaw/decide.sh --promote exp-001 --reason "Improved Sharpe" --dry-run

# 初始化新環境
bash ./scripts/openclaw/onboard.sh
```

---

## 依賴

- `bash` >= 4.0
- `awk`（用於數值比較）
- `jq`（JSON 處理，若未安裝會優雅降級）
- 專案編譯後的二進位（`atlas`、`judge-experiment`、`promote-baseline` 等）
