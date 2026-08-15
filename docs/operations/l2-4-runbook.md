# L2.4 Observation Window — Operations Runbook

> **Status**: ⛔ **2026-08-06 — 軌道收尾**。PR #821 merged 2026-06-29 (commit `f69b3551`);Issue #825 / #826 已關閉 (依 `.omo/manifests/2026-08-06-l2-4-issue-alignment-audit.md`（harness 私有，不入 repo）)。本 runbook 保留供未來若重啟 L2.4 觀察期使用;`LLMDriver` deprecated alias 已於 2026-08-06 移除 (§5 step 3 已不適用)。
> **對象**: ops / on-call engineering
> **範圍**: Wave 11 L2.4 — `UseLLMSectorAgents` 啟用後 7-14 天觀察期
> **Issue**: [#742](https://github.com/kaecer68/atlas-go/issues/742)
> **觀察指標權威**: [`../specs/l2-4-observation-spec.md`](../specs/l2-4-observation-spec.md-spec) §Metrics
> **Log 範本**: `.omo/evidence/l2-4-observation-log.md`
> **Flag 函式**: `config.GetUseLLMSectorAgents()`（`internal/config/parameters.go`）+ `config.GetL2_4Schedule()` (新加,L2.4 scheduling)
> **Metric 來源**: PR #821 (commits `1d82c8a5` + `78fd2b4b` + `1491ab93` + `f2c37c61`) — `SemiconductorLLMAgent.Recommend()` 內 6 個 `slog.Info` events
> **未來工作**: [`.omo/manifests/l2-4-followup.md`](.omo/manifests/l2-4-followup.md) — auto-cron / CLI flag / promotion procedure

## 1. Pre-flight Checklist

啟用 L2.4 觀察期前,逐項確認:

- [ ] **環境選擇**: 使用 staging 或專用 L2.4 harness,**不可在 production 啟用**。
- [ ] **基線對照組**: 平行跑一輪 deterministic `SemiconductorExecutor`(同一 symbol、同一視窗)做為對比組。
- [ ] **Flag flip**: 編輯 `configs/parameters.json`,把 `orchestrator.use_llm_sector_agents.value` 由 `false` 改為 `true`,`source` 仍保持 `experimental`。
- [ ] **Env var 設定**: 除 JSON 旗標外,還須設 `LLM_SECTOR_AGENTS_ENABLED=true`(由 `config/config.go` 讀取,傳遞鏈:`LLMSectorAgentsEnabled` → `factory.go` → `WithLLMSectorAgents(driver)` → 註冊 `llmSectorAgentsPlugin`)。缺此 env var plugin 不會註冊,driver 不會注入。
- [ ] **重啟服務**: flag 在啟動時讀取,無 hot-reload。執行 `docker compose restart atlas` 確認新行程載入配置。
- [ ] **Health 端點**: `curl -fsS http://localhost:18080/api/llm/health` 回 `router_version: v2.1`,三個 Provider 至少 Primary 為 healthy。
- [ ] **slog 設定確認**: `recommendation.symbol` 必須出現在 log output(JSON 格式),`agent_loop.start` event 可見。
- [ ] **Log 檔案建檔**: 見 `.omo/evidence/l2-4-observation-log.md` (Week 0 Baseline)
- [ ] **L2.4 schedule 面板確認**: 開 `http://<host>/admin/#page-synergy`,確認 L2.4 排程面板有渲染(status badge + 預設值 + 4 按鈕)。若「載入中…」持續顯示,代表 `l24Mgr.SetConfig` 沒在 boot 跑成功(見 PR #821 commit `f2c37c61`)。

## 2. Daily Check-in 流程

每日(或每 N 次 recommendation)執行:

### 2.1 指標收集

從 slog JSON output 拉出下列欄位,彙整至觀察記錄(見 `.omo/evidence/l2-4-observation-log.md`):

| 指標 | 來源 event | 計算 |
|------|------------|------|
| `loop.exhausted_rate` | `agent_loop.end.exhausted=true` | 該次數 / 總 recommendation 數 |
| `tool.success_rate` | `agent_loop.tool` (success=true) | 成功次數 / 總 tool 次數 |
| `llm.latency_p50` / `p95` | `agent_loop.plan` 的 `latency_ms` | 百分位數(rollup);reflect **不**量測延遲(Issue #740 設計) |
| `reflect.continue_rate` | `agent_loop.reflect` (continue=true) | 比例應 < 50% |
| `conviction.distribution` | `agent_loop.reflect.continue=false` 的 `conviction` | 直方圖 |

詳細 schema 見 [`../specs/l2-4-observation-spec.md`](../specs/l2-4-observation-spec.md-spec) §Metrics。

### 2.2 Spot-check(每日 3-5 筆)

抽 3-5 個 LLM-driven recommendation,對比 deterministic baseline:

- **Conviction 差異**: 兩者差距 |Δ| 是否在 ±15 內?
- **Reasoning 品質**: `agent_loop.reflect` 前的 plan 步驟是否引用 tool output?
- **異常標記**: latency spike(> 8s)、tool error、`agent_loop.exhausted` 觸發。

### 2.3 Log 範本

```markdown
### YYYY-MM-DD — Day N
- Recommendations: N
- loop.exhausted_rate: X.X%
- tool.success_rate: X.X%
- llm.latency_p50/p95: Xms / Xms
- reflect.continue_rate: X.X%
- spot-check: 3-5 recs,Δconviction=|X-X|
- 異常: ...
- 決策: 繼續觀察 / 進入 promotion 評估 / 觸發 rollback
```

## 3. Acceptance Criteria

### Day 7 checkpoint

| 條件 | 閾值 | 動作 |
|------|------|------|
| `loop.exhausted_rate` | < 5% | 超標 → 排查 plan prompt 或 `MaxIter` |
| `tool.success_rate` | > 95% | 超標 → 排查 tool dispatch 與 LLM tool schema |
| `llm.latency_p95` | < 8s | 超標 → 評估換 provider / 縮短 prompt |
| 0 unhandled panic | 必須 | 觸發 → 立即 rollback |
| Spot-check ≥ 20 recs | 必須 | 不足 → 延長觀察至 day 14 |

### Day 14 promotion gate

在 Day 7 條件全部通過後,加驗:

- **LLM Sharpe ≥ deterministic baseline**(per symbol 平均)。計算方式: 對 LLM 與 deterministic 各跑一次相同視窗(7-14 天),比較 Sharpe ratio。
- **Reasoning 連貫性**: 累計 spot-check ≥ 20 筆,reasoning 必須引用具體 tool output(不可純編造)。
- **Roll-back 驗證**: 至少一次手動測試翻 flag 為 `false` 並重啟,確認下一個 recommendation cycle 回到 deterministic 路徑(1 cycle 內完成切換)。

任一條件未過 → 觸發 §4 Rollback,並 file follow-up issue 紀錄根因。

## 4. Rollback Procedure

當 acceptance criteria 任一未達標,或觀察期內出現 panic、latency 暴增等異常:

1. **編輯配置**: `configs/parameters.json` 將 `orchestrator.use_llm_sector_agents.value` 由 `true` 改回 `false`。
2. **重啟服務**: `docker compose restart atlas`。無熱載入,必須重啟。
3. **驗證切換**: 觀察下一個 recommendation cycle 確認 `Supports()` 走 deterministic `SemiconductorExecutor`(`slog` 不應再出現 `agent_loop.*` events)。可用 synergy 頁的 L2.4 排程面板按「停止觀察期」快速驗證。
4. **記錄異常**: 在觀察記錄(見 `.omo/evidence/l2-4-observation-log.md`)標註 rollback 時間與觸發條件。
5. **File follow-up issue**: 根因分析(LLM model?prompt?tool dispatch?state machine?),**未解決前不可重啟 L2.4**。

Rollback 後 deterministic 路徑立即恢復(gate mechanism 保證),不會造成服務中斷。

## 5. Promotion Procedure

Day 14 acceptance 全部通過後,依序執行(每步獨立 PR):

1. **Source 升級**: 在 `configs/parameters.json` 把 `orchestrator.use_llm_sector_agents.source` 從 `experimental` 改為 `empirical`(`value` 暫不動)。
2. **翻 default 為 true(獨立 PR)**: 將 `use_llm_sector_agents.value` 改為 `true`,並同步新增 `use_llm_sector_agents_deprecated` 旗標供暫時 opt-out。**這是獨立 PR**,不在本次 runbook PR 範圍。
3. **移除 deprecated alias**: ~~刪除 `internal/orchestrator/sector_agent_llm.go` 中的 `LLMDriver` 別名(向後相容已無必要)~~ — **✅ 已於 2026-08-06 完成**(Issue #826 關閉時清理)。
4. **Tag 版本**: 上述變更合併後,標記 `v0.0.0.22`(具體版本號依當時累積變更決定,參考 `CHANGELOG.md`)。

> 設計保持簡單:promotion 流程不引入新 CLI flag,僅翻 default + 刪 alias。

完整 4 步工作報告見 [`.omo/manifests/l2-4-followup.md`](.omo/manifests/l2-4-followup.md) §3。

## 6. Failure Modes & Escalation

| 失敗模式 | 偵測 | 處置 | 升級對象 |
|----------|------|------|----------|
| **Metric 收集失敗(slog 未輸出)** | health 端點正常但 log 無 `agent_loop.*` events | 確認 `Metrics *slog.Logger` 注入正確;若為 stdlib `slog.Default` 設為 JSON handler;`SemiconductorLLMAgent.Metrics` 不得為 nil | engineering on-call |
| **Flag flip 導致 crash** | 重啟後 `atlas` 容器反覆 restart | 立即翻 flag 回 `false` + 重啟,回到 §4 Rollback | engineering on-call + issue owner |
| **L2.4 排程面板空白** | synergy 頁顯示「載入中…」 | 檢查 `l24Mgr.SetConfig` boot log;若 failed → 看 `l24_seed_failed` 警告;立即 rollback flag + 翻 env var | engineering on-call |
| **Health 端點失敗** | `curl /api/llm/health` timeout 或 5xx | 檢查 LLM Provider 狀態與 circuit breaker;**不影響 L2.4 flag 本身** | LLM platform on-call |
| **Deterministic baseline 缺席** | 對比組無資料 | 確認平行 baseline run 排程;若長期缺資料 → 暫停 L2.4 觀察 | product owner |
| **觀察期 ownership 衝突** | 多 team 對 promotion gate 意見分歧 | 以觀察記錄(見 `.omo/evidence/l2-4-observation-log.md`)數據為準;由 issue owner 仲裁 | Kaecer(product owner) |
| **Communication 缺口** | Day 7 / Day 14 checkpoint 漏跑 | Calendar reminder + 在 PR 留言 thread 公告 | issue owner |

### 溝通管道

- 觀察進度更新: 在本 PR 留言 thread 每日摘要(僅 spot-check 結果,非逐筆 log)。
- 緊急異常(panic、>10s latency、>10% exhausted rate): Slack `#atlas-ops` + 開 incident issue,標籤 `incident`。
- 觀察期結束決策: 在觀察記錄(見 `.omo/evidence/l2-4-observation-log.md`)末段總結,並 link 到後續 promotion / rollback PR。

## 7. References

- Issue: [#742](https://github.com/kaecer68/atlas-go/issues/742)
- 指標定義: [`../specs/l2-4-observation-spec.md`](../specs/l2-4-observation-spec.md-spec) §Metrics
- L2.3 架構: [`../specs/llm-sector-agent-spec.md`](../specs/llm-sector-agent-spec.md-spec)
- L2.4 follow-up plan: [`.omo/manifests/l2-4-followup.md`](.omo/manifests/l2-4-followup.md) — auto-cron / CLI flag / promotion procedure
- Flag 函式: `config.GetUseLLMSectorAgents()` + `config.GetL2_4Schedule()` (`internal/config/parameters.go`)
- Metric 實作: PR #821 — `internal/orchestrator/semiconductor_llm_agent.go` + `internal/monitoring/api/pipeline/l2_4_*.go`
- Log 範本: `.omo/evidence/l2-4-observation-log.md`
- Plan: [Issue #711](https://github.com/kaecer68/atlas-go/issues/711) §L2.4
