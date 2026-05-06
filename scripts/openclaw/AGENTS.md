# AGENTS.md — scripts/openclaw

本目錄是 **OpenClaw 治理 / 審核 / promote-revert 操作層**。這不是一般 helper scripts 集合，而是一套有流程約束的 operator interface。

---

## OVERVIEW

OpenClaw 腳本負責把 mutation proposal、execute、judge、approve/reject、replay audit event、governance gate、operations gate 串成可追蹤流程。

核心特性：

- 人工決策可審計（`data/state/approvals/*.json`）
- promote / revert 必須附 `--reason`
- 多數危險操作支援 `--dry-run`
- G2/G3/G4/M5/M7/M8 驗證有對應腳本，不靠口頭流程

---

## WHERE TO LOOK

| 任務 | 腳本 | 備註 |
|------|------|------|
| 查看當前狀態 | `status.sh` | 最先跑；不知道下一步時先看這裡 |
| 生成 mutation 建議 | `propose-mutation.sh` | 可互動、可 `--auto` |
| 執行下一個實驗 | `execute-next.sh` | 也可指定 `--brief` |
| 評判最新結果 | `judge-latest.sh` | 支援 `--json` |
| promote / revert 決策 | `decide.sh` | 直接決策入口 |
| 人工審核入口 | `human-approval.sh` | 產生 approval event，推薦用這個 |
| 重播審核事件 | `replay-approval-event.sh` | 先 `--dry-run` |
| 治理 gate 驗證 | `verify-governance-gates.sh` | G2/G3/G4 + M5 + M7 |
| operations gate 驗證 | `verify-operations-gate.sh` | runbook / monitoring / rollback drill |
| branch protection 設定 | `setup-branch-protection.sh` | 預設 dry-run |
| 每日啟動 / 完整 round | `today-start.sh`, `run-validated-round.sh` | operator convenience entrypoint |

---

## CONVENTIONS

- 預設 workflow：`status.sh` → `propose-mutation.sh` → `execute-next.sh` → `judge-latest.sh` → `human-approval.sh` / `decide.sh`。
- `human-approval.sh` 比直接 `decide.sh` 更可稽核，因為會先落 approval event。
- Gate 驗證不是可選裝飾：`verify-governance-gates.sh` / `verify-operations-gate.sh` 是流程契約的一部分。
- rollback / replay / approval contract 都透過 shell + `jq` 驗證，腳本本身就是 runbook 的 executable form。

---

## ANTI-PATTERNS

- **不要在沒有 `--reason` 的情況 promote / revert**：這違反本目錄的稽核假設。
- **不要略過 `--dry-run` 就直接 replay / rollback**：尤其是 approval event replay 與 branch protection restore。
- **不要把 gate 訊號當錯誤**：`futility guard`、`auto-pivot`、`skip` 常是控制信號，不代表腳本壞掉。
- **不要手動改 approval event JSON 來模擬流程**：應透過 `human-approval.sh` 產生事件，再用 `replay-approval-event.sh` 重播。
- **不要把 OpenClaw 腳本與一般 scripts 混用**：這裡的腳本有治理語義與狀態假設，跟 `coverage.sh`、`darwinian-adjust.sh` 不同。
- **不要忽略 `verify-governance-gates.sh --require-scenario-diversity`**：strict mode 是檢查 scenario 是否真的區分得開。

---

## FILES & STATE

| 路徑 | 用途 |
|------|------|
| `data/state/baseline_policy.json` | 當前 baseline policy |
| `data/state/experiments.jsonl` | 實驗紀錄索引 |
| `data/state/experiments/` | 實驗結果明細 |
| `data/state/approvals/` | promote / revert / approve / reject 稽核事件 |
| `configs/briefs/` | mutation brief 範本 |
| `docs/operations-playbook.md` | operator runbook，腳本契約會檢查這份文件 |

---

## NOTES

- `QUICK_REFERENCE.md` 是最快的 operator cheat sheet；新增腳本時記得同步更新。
- 若腳本需要驗證文件存在、JSON schema、replayability，延續現有 `jq` + explicit failure message 風格。
- 本目錄偏 shell orchestration；真正的業務邏輯應回到 `internal/experiment/`、`internal/baseline/`、`internal/monitoring/`、`internal/orchestrator/`。
