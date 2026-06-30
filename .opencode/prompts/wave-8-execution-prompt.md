# Wave 8 Execution Prompt — 給新 CLI 工作區的裁決後指令

> **用途**：當新 Wave 8 CLI 找到 `docs/archive/wave-8-plan.md` 的錯誤並修正後，由使用者把這份 prompt 貼到那個 CLI 的下一則 user message。
> **建立日期**：2026-06-20
> **核心原則**：plan 是輕量文件，code 在 main 上的獨立 PR

> **⏪ 歷史狀態**：Wave 8 已於 v0.0.0.7（PRs #619-#631）收尾。本檔案保留為歷史 audit 記錄，**不應被新 CLI 工作區自動載入**。

---

## 分支策略（重要）

**不要把所有 Wave 8 程式碼塞在 docs/wave-8-plan 分支**。改用以下結構：

```
main
├── docs/wave-8-plan        ← 只放計劃文件（修正後合併到 main）
├── feat/wave-8-risk-gate-rejected
├── feat/wave-8-risk-gate-override
├── feat/wave-8-industry-calendar-event
└── ... (9 RED + 5 YELLOW = 14 個 PR，全部基於 origin/main)
```

## 第一步：先把 docs/archive/wave-8-plan.md 的修正 commit + push

```bash
git status
git diff docs/archive/wave-8-plan.md
git add docs/archive/wave-8-plan.md
git commit -m "docs: correct Wave 8 plan based on code verification"
git push origin docs/wave-8-plan
```

合併到 main 的時機：等 `atlas-feature-llm-phase1-core-plumbing` worktree 釋放 main 後，由這個 CLI 做 admin-squash merge：

```bash
gh pr create --base main --head docs/wave-8-plan
gh pr merge --squash --admin --delete-branch
```

## 第二步：每個事件開新分支 + PR 到 main

禁止在 docs/wave-8-plan 分支上寫程式碼。每個事件開新分支，從 `origin/main` 切出：

```bash
git fetch origin main
git switch -c feat/wave-8-risk-gate-rejected origin/main

# 實作事件（嚴格白名單）
# - internal/monitoring/api/events/
# - internal/monitoring/service/
# - internal/monitoring/handlers.go
# - shared_web/static/js/services/event-source.js, admin_web/static/js/event-listeners.js
# - monitoring/rules/

git add internal/monitoring/api/events/risk_gate_rejected.go
git commit -m "feat(events): add RiskGateRejected event type + SSE producer"
git push -u origin feat/wave-8-risk-gate-rejected

gh pr create --base main --head feat/wave-8-risk-gate-rejected
# 等 CI 全綠
gh pr merge --squash --admin --delete-branch
```

## 第三步：每個 PR 的 description 必須包含

1. **觸發點檔案路徑**：用 `gitnexus_context` / `gitnexus_query` 找到的
2. **Payload schema 最終版本**：從實際 source code 推導
3. **Schema version**：沿用 PD-1，第一個事件預設 v1
4. **JSONL 審計軌跡**：沿用 PD-2，或明確說明不需要
5. **效能影響評估**：沿用 PD-3，或說明低頻
6. **Frontend component**：是否需要新 component 或擴展既有
7. **atlas-pre-change-protocol 執行記錄**：跨 ≥3 檔必跑

## 第四步：錯誤復原機制

如果中途發現某個事件的設計需要大改：
1. 在當前 PR branch 上 commit 修正
2. 不要合併未通過 review 的版本
3. 若錯誤跨多個 PR 已合併：在 `docs/archive/wave-8-plan.md` 加「修訂紀錄」段落，開新的 fix PR

## 第一個事件開工順序（建議）

```
Wave 8.0: 基礎設施（schema_version 框架、EventStore 介面、throttle framework）
Wave 8.1: RiskGateRejected
Wave 8.2: RiskGateOverride
Wave 8.3: IndustryCalendarEvent
Wave 8.4: TradeSlippage
Wave 8.5: LLMAnnotatorCircuitOpen
Wave 8.6: LLMAnnotatorFallbackUsed
Wave 8.7: LLMAnnotatorQuotaExceeded
Wave 8.8: BacktestCompleted
Wave 8.9: CalibrationCompleted
Wave 8.10: frontend 整合測試 + docs 收尾
```

第 1 個事件 `RiskGateRejected` 優先：觸發點最近、`internal/risk/gate.go` 已存在、refactor 風險最低。

## 你現在要做的具體動作

```bash
# 1. 確認目前狀態
git status
git log --oneline -5
git diff docs/archive/wave-8-plan.md | head -100

# 2. 照「第一步」commit + push docs/archive/wave-8-plan 修正
# 3. 然後：
git fetch origin main
git switch -c feat/wave-8-risk-gate-rejected origin/main
# 4. 開始實作第一個事件
```

## 不要做的事

- ❌ 不要把 9 個事件的程式碼塞同一個 PR
- ❌ 不要基於 docs/wave-8-plan 分支開新分支（要從 origin/main）
- ❌ 不要修改 `docs/archive/wave-8-plan.md` 之外的 docs/ 檔案
- ❌ 不要重新生成整套 Wave 8 plan
- ❌ 不要動 internal/llm/、internal/llm_annotator/、internal/narrative/、internal/spawning/、internal/orchestrator/
- ❌ 不要動 cmd/atlas/main.go 的 provider 區段
- ❌ 不要修改 capability enum 名稱
- ❌ 不要引入 React/Vue

## 觸發點驗證清單（每個事件實作前必跑）

```bash
gitnexus_context --task "find where RiskGateRejected would fire"
gitnexus_explore --query "RiskGate reject path in internal/risk/"
grep -n "type RiskGate" internal/risk/gate.go
```

如果發現 trigger 點或 schema 與 plan 假設不同，**先 commit plan 修正再繼續**，不要默默調整程式碼。

## 等你回報

請輸出：
1. `git status` 結果
2. `docs/archive/wave-8-plan.md` 的 diff 摘要
3. 你建議的第一個事件開工順序
4. 是否有額外的 plan 修正需要在合併前處理
