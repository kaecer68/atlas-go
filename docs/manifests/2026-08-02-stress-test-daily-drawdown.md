# Audit Manifest: stress_test_daily drawdown reporter 斷裂

> **Audit source**: SK-29(atlas-wiki 升 active 工作中發現) — `/api/dashboard/drawdown` 永遠回 `not_available`
> **Goal**: 修復 `RunDailyStressTests` 跑完後未呼叫 `drawdownReporter`,讓 dashboard drawdown 端點在 `stress_test_daily` 排程首次完成後能回傳實際 drawdown 數據。
> **Scope**:
> - In: `internal/orchestrator/system.go:RunDailyStressTests` 結尾加 reporter 呼叫
> - In: `internal/orchestrator/system_test.go` 加 unit test 驗證 reporter 被呼叫
> - Out: 改 `BuildSystem(PathStressTestDaily)` 路徑(選項 B 風險中)
> - Out: 改 `RunDailySimulation` 錯誤處理(選項 C 職責混淆)
> - Out: 改 `docker-compose.yml` / `Dockerfile` / 部署層
> **Created**: 2026-08-02
> **Status**: done

---

## 根因(已驗證)

| 證據 | 位置 | 內容 |
|---|---|---|
| Bug 點 1 | `internal/orchestrator/system.go:1159-1209` `RunDailyStressTests` | 跑完 5 個 stress scenarios 構造 `stress.Report`(含 `WorstDrawdown` / `WorstVaR`),**log `stress_test_daily_completed`** 後直接 `return nil` — 沒有任何 reporter / portfolio update / dashboard 寫入 |
| 唯一 reporter 寫入點 | `internal/orchestrator/system.go:658-668`(在 `RunDailySimulation` 內) | 只有當 `s.Sim().engine.Optimizer() != nil` 時,`SimulateDrawdownForMonitoring` → `drawdownReporter(ddResult)` 才會被呼叫 |
| A01 | `RunDailyStressTests` 跑完後未呼叫 `drawdownReporter`,dashboard drawdown 永遠 nil | `system.go:1202-1208` 結尾只 log 不送 reporter | `internal/orchestrator/system.go:1202-1208`(在 `stress_test_daily_completed` log 之後,`return nil` 之前) | 1) `system.go:1208` 前加 reporter 呼叫區塊,2) 新 unit test 驗證 reporter 收到 `DrawdownResult` 且 `MaxDrawdown == report.WorstDrawdown` / `VaR95 == report.WorstVaR`,3) 既有 `system_coverage_test.go:100` 不受影響 | done | none | TDD:RED→GREEN;commit 3feb25dd;PR #1440 |
| 為何 stress 測試路徑無 optimizer | `internal/orchestrator/composition/root.go:231-269` `BuildSystem(PathStressTestDaily)` | 路徑建構時不注入 optimizer — 所以即使 reporter 邏輯想用 `SimulateDrawdownForMonitoring`,也沒 optimizer 可用 |
| Reporter 注入點(對照用) | `cmd/atlas/main.go:1382-1386` | `system.SetDrawdownReporter(func(d portfolio.DrawdownResult) { dashboard.SetLatestDrawdown(&d) })` — 確實有注入 |
| Reporter 接收端 | `internal/monitoring/dashboard_api.go:1591-1596` | `SetLatestDrawdown` 寫入 `latestDrawdown` 欄位,`GetLatestDrawdown` 讀取 — 鏈路完整 |
| Dashboard 讀取 | `internal/monitoring/api/dashboard/drawdown.go:18-34` `HandleDrawdown` | `result == nil` 時回 `not_available`;有 result 時回 `MaxDrawdown` / `VaR95` / `WorstPath` |
| 排程狀態 | `mcp__atlas_mcp_scheduler_get_status` | `stress_test_daily` `enabled=true` `consecutive_failures=0` `last_run=2026-08-01T23:39:07Z` — 排程有跑 |
| 同型歷史 bug | `docs/archive/2026-07-20-stress-api-ledger-drift.md` A03(2026-07-20) | 更早的「排程沒跑」版本,已修;這次是「排程有跑但 reporter 沒接」延伸 |

**根因結論**:`RunDailyStressTests` 在 stress scenarios 跑完後只 log 結果,從未呼叫 `s.drawdownReporter(...)`。即使 `SetDrawdownReporter` 有在 main.go 注入、`SimulateDrawdownForMonitoring` 在另一個路徑有呼叫,dashboard drawdown 從未從 stress_test_daily 排程取得資料。

**為何不選選項 B(注入 optimizer)**:stress_test_daily 路徑本就不需要跑完整 daily simulation 排程,選項 A 直接用已計算的 `stress.Report.WorstDrawdown` / `WorstVaR` 構造 `portfolio.DrawdownResult` 即可滿足 dashboard 需求,改動最小。

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| A01 | `RunDailyStressTests` 跑完後未呼叫 `drawdownReporter`,dashboard drawdown 永遠 nil | `system.go:1202-1208` 結尾只 log 不送 reporter | `internal/orchestrator/system.go:1202-1208`(在 `stress_test_daily_completed` log 之後,`return nil` 之前) | 1) `system.go:1208` 前加 reporter 呼叫區塊,2) 新 unit test 驗證 reporter 收到 `DrawdownResult` 且 `MaxDrawdown == report.WorstDrawdown` / `VaR95 == report.WorstVaR`,3) 既有 `system_coverage_test.go:100` 不受影響 | pending | none | TDD:RED→GREEN |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 確認 `RunDailyStressTests` 完整原始碼 | - | done | `internal/orchestrator/system.go:1156-1209` |
| 確認 `RunDailySimulation` reporter 模式 | - | done | `internal/orchestrator/system.go:658-668` |
| 確認 `DrawdownResult` 結構 | - | done | `internal/portfolio/optimizer_drawdown.go:18-23` |
| 確認 `stress.Report` 結構 | - | done | `internal/stress/runner.go:32-39` |
| 確認 dashboard drawdown handler 讀取鏈 | - | done | `internal/monitoring/api/dashboard/drawdown.go:18-34` + `internal/monitoring/dashboard_api.go:1591-1669` |
| 確認 stress_test_daily 排程掛載 | - | done | `cmd/atlas/main.go:1367-1399` |
| 確認 composition PathStressTestDaily 建構 | - | done(不修改) | `internal/orchestrator/composition/root.go:231-269`(確認不注入 optimizer 屬預期行為) |
| 確認既有 reporter setter test | - | done | `internal/orchestrator/system_coverage_test.go:91-114` |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 確定修復策略 = 選項 A | A01 | done | 最小改動,純複製貼上 reporter 模式 |
| 設計測試案例 | A01 | done | 1) 注入 spy reporter,2) 呼叫 `RunDailyStressTests`,3) 驗證 reporter 收到非零 `DrawdownResult` |
| 評估 blast radius | A01 | done | 改 `system.go` 一個函式,加一個 unit test,風險 LOW |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 加 RED failing test | A01 | pending | `internal/orchestrator/system_test.go` |
| 確認測試 fail | A01 | pending | `go test ./internal/orchestrator/ -run TestRunDailyStressTests_CallsReporter` |
| 實作 reporter 呼叫 | A01 | pending | `internal/orchestrator/system.go:1208` 之前 |
| 確認測試 pass | A01 | pending | `go test ./internal/orchestrator/ -run TestRunDailyStressTests_CallsReporter` |
| 跑 system_coverage_test.go 回歸 | A01 | pending | `go test ./internal/orchestrator/ -run TestSystem_Setters` |
| 跑 ci-gate | A01 | pending | `make ci-gate` |
| 跑 ci-full | A01 | pending | `make ci-full` |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| 更新 manifest 狀態 | A01 | pending | - |
| git commit | A01 | pending | `fix(manifest): #A01 call drawdownReporter from RunDailyStressTests` |
| push branch + gh pr create | A01 | pending | PR title: `fix(stress_test_daily): update dashboard latestDrawdown after stress scenarios complete` |
| git cleanup-tools 收尾 | A01 | pending | - |

---

## Backlog

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|----------------|----------------|
| - | (無新發現) | - | - |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- One commit per ID
- PR body 必含: Summary / Root Cause(指向 `system.go:1156-1209`)/ Verification

---

## Session-End State

- **Done this session**: A01
- **Remaining**: -
- **Next action**: 等 PR #1440 CI 過 + kaecer merge
- **Uncommitted code**: no
- **Branch / PR**: `fix/20260802-stress-test-daily-drawdown-reporter` / #1440
- **Paused because**: -

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-08-02 | 1.0 | Initial manifest | assistant |
| 2026-08-02 | 1.1 | A01 implementation done, PR #1440 opened | assistant |
