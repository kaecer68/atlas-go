# L2.4 Observation Window — Observation Log

> **對應 runbook**：`docs/operations/l2-4-runbook.md`
> **對應 spec**：`docs/specs/l2-4-observation-spec.md`
> **對應 followup**：`docs/archive/l2-4-followup.md`
> **演化自**（已完成 cycle）：`內部觀察日誌（已刪除）`（Wave 10 L2.3 `SectorAgentLLM`）
> **範圍**：Wave 11 L2.4 — `SemiconductorLLMAgent` 啟用後 14-28 天觀察期
> **啟動日期**：（TBD，觀察期啟動時填寫）
> **觀察對象**：`SemiconductorLLMAgent.Recommend()` LLM-driven path vs 確定性 `SemiconductorExecutor` baseline

---

## 觀察指標（對齊 runbook §2.1）

每個 LLM-driven recommendation 都需從 slog `agent_loop.*` events 拉出下列欄位，彙整至此：

| 指標 | 說明 | 來源 event | 目標 |
|------|------|------------|------|
| `loop.exhausted_rate` | 跑到 `MaxLoopRounds` 才結束的比例 | `agent_loop.end` status="exhausted" | < 5% |
| `tool.success_rate` | tool call 成功比例 | `agent_loop.tool` success=true | > 95% |
| `llm.latency_p50` | LLM call 延遲中位數 | `agent_loop.end` duration_ms | < 2s |
| `llm.latency_p95` | LLM call 延遲 P95 | `agent_loop.end` duration_ms | < 8s |
| `reflect.continue_rate` | Plan → Reflect 後決定 continue 的比例 | `agent_loop.reflect` continue_decision=true | < 50% |
| `conviction.distribution` | Final conviction 0-100 直方圖 | `agent_loop.end` conviction_score | 與確定性對齊 |

---

## 觀察記錄格式（Weekly entry）

每週日（或每 N 次 recommendation）追加一筆：

```markdown
### YYYY-MM-DD — Week N

**指標彙整（本週）**
- loop.exhausted_rate: X.X% (N recs)
- tool.success_rate: X.X% (N recs)
- llm.latency_p50: Xms, p95: Xms
- reflect.continue_rate: X.X% (N recs)
- conviction.distribution: [0-20: N, 20-40: N, 40-60: N, 60-80: N, 80-100: N]

**Spot-checks（每日 3-5 筆累計）**
- 累計 spot-checks: N（目標 Day 7 ≥ 20, Day 14 ≥ 30）
- 不一致案例: ...

**LLM-driven vs 確定性對比**
- Hit rate（同一視窗）:
  - LLM-driven: X.X% (N recs)
  - Deterministic: X.X% (N recs)
- Sharpe（同公式）:
  - LLM-driven: X.X
  - Deterministic: X.X
- Token cost: $X.XX / rec, 累計 $X.XX

**異常與事件**
- ...

**決策**
- 繼續觀察 / 擴展到更多 sector / 回滾
```

---

## Week 0 — Baseline（觀察期啟動前，Pre-flight）

> **填寫時機**：flag flip 前一週，平行跑確定性 baseline
> **不可在 production 啟用**（runbook §1 step 1）

- 觀察期預計啟動日：TBD
- LLM-driven agent：**尚未啟用**（flag 仍 `false`）
- 觀察對象：確定性 `SemiconductorExecutor` baseline
- 觀察窗口：(TBD) ~ (TBD)
- 預期累計：N/A（等待 LLM agent 啟用）

### Pre-flight Checklist 驗證（對齊 runbook §1）

- [ ] 環境選擇（staging 或專用 L2.4 harness，**不可 production**）
- [ ] Feature flag `UseLLMSectorAgents` 確認為 `false`（baseline 期）
- [ ] L2.4 schedule 面板渲染確認（`/admin/#page-synergy`）
- [ ] slog 設定可見 `agent_loop.*` events（staging 環境）
- [ ] 觀察記錄檔案建檔（本檔，標註啟動日）
- [ ] 第一個 `agent_loop.start` event 驗證流程（手動觸發 1 次 recommendation 觀察 slog）

---

## Week 1 — Day 1-7（LLM-driven 啟用）

> **填寫時機**：每日 check-in 結束後（對齊 runbook §2 daily check-in 流程）
> **Acceptance gate**：6 個指標全 pass + ≥ 20 spot-checks

### Week 1 範例 entry

```markdown
### 2026-07-29 — Week 1

**指標彙整**
- loop.exhausted_rate: 3.2% (94 recs)
- tool.success_rate: 97.9% (94 recs)
- llm.latency_p50: 1.4s, p95: 4.8s
- reflect.continue_rate: 41.5% (94 recs)
- conviction.distribution: [0-20: 4, 20-40: 18, 40-60: 52, 60-80: 16, 80-100: 4]

**Spot-checks**
- 累計: 21（達 Day 7 目標 ≥ 20）✅
- 不一致案例: 2 件 recs 因 conviction 計算偏差被人工 override

**對比**
- Hit rate: LLM-driven 58.5% (94 recs) vs Deterministic 56.4% (94 recs) ✅
- Sharpe: LLM-driven 1.42 vs Deterministic 1.38
- Token cost: $0.007 / rec, 累計 $0.66

**異常與事件**
- Day 3 發生 1 次 LLM timeout → retry 成功
- Day 5 一次 `agent_loop.reflect` 連續 5 次 continue 後 exhausted（正常路徑）

**決策**
- ✅ 繼續觀察 — 6 個指標全 pass，Day 7 acceptance gate 可達標
```

---

## Week 2 — Day 8-14（穩定期）

> **填寫時機**：每日 check-in 結束後
> **Acceptance gate**：Week 1 全 pass + LLM Sharpe ≥ 確定性 baseline + ≥ 30 spot-checks + **1× rollback 驗證**（runbook §3）

### Week 2 範例 entry

```markdown
### 2026-08-05 — Week 2

**指標彙整**
- loop.exhausted_rate: 2.1% (88 recs)
- tool.success_rate: 98.6% (88 recs)
- llm.latency_p50: 1.3s, p95: 4.2s
- reflect.continue_rate: 38.6% (88 recs)
- conviction.distribution: [0-20: 5, 20-40: 22, 40-60: 48, 60-80: 11, 80-100: 2]

**Spot-checks**
- 累計: 42（達 Day 14 目標 ≥ 30）✅
- 不一致案例: 1 件 — Day 12 rec 因 tool dispatch 結果誤判被 override

**對比**
- Hit rate: LLM-driven 59.2% (182 recs cumulative) vs Deterministic 57.1% (182 recs)
- Sharpe: LLM-driven 1.48 vs Deterministic 1.39 ✅
- Token cost: $0.007 / rec, 累計 $1.27

**Rollback 驗證（runbook §3 Day 14 要求）**
- Day 13 手動觸發 1 次 rollback 演練：`l24Mgr.SetConfig(L2_4_SCHEDULED=false)`
- 驗證下次 recommendation 走確定性 `SemiconductorExecutor`（slog 確認無 `agent_loop.*` events）
- 9 秒後恢復 L2.4，無資料損耗

**異常與事件**
- （無）

**決策**
- ✅ 通過 Day 14 acceptance gate — 進入 promotion 流程（runbook §5 + followup §3）
```

---

## Week 3-4 — Day 15-28（推廣期，可選）

> **填寫時機**：如 Week 1-2 任一 acceptance gate 未過，Week 3 改為 rollback 後的再觀察期
> **若全 pass**：進入 followup.md §3 promotion 4 步驟（Source 升級 + Default flip + LLMDriver 移除 + Version tag），此期間為 promotion 流程的觀察緩衝
> **若 gate 未過**：填寫 rollback 紀錄 + file follow-up issue（runbook §4）

---

## 已知限制（從 Wave 10 log 演化）

1. **Burn-in 期間不適用**：`MaturityBurnIn` 期間所有 auto 機制靜默，LLM-driven agent 也不應啟用。
2. **Token 成本需嚴格控制**：每次 recommendation 預算 < $0.01，月度 token 預算上限待定。
3. **Determinism 缺失**：LLM-driven agent 不可用於 backtest（replay 階段仍走確定性路徑）。
4. **失敗模式不同**：LLM 隨機性 vs 確定性，Darwinian weight 計算需適配。
5. **Conviction Clamp**：`ApplyToRecommendation` clamp 範圍 [1, 100]（`realtime/regime_adapter.go:619`），LLM-driven 結果會被這個 clamp 影響，計算 `conviction.distribution` 時須注意。
6. **Naming evolution**：本 cycle 用 `SemiconductorLLMAgent`（L2.3 PR #733 改名自 `SectorAgentLLM`）；Wave 10 log 的 `SectorAgentLLM` 引用為舊 cycle 對應名稱。

---

## 啟動 Checklist（給執行者）

對齊 runbook §1 Pre-flight Checklist + 觀察記錄本身的準備：

- [ ] Week 0 Baseline 條目填寫（本檔 §Week 0）
- [ ] Feature flag `UseLLMSectorAgents` 從 `false` 翻 `true`（同步更新 `configs/parameters.json` + env `LLM_SECTOR_AGENTS_ENABLED=true`）
- [ ] L2.4 schedule 面板確認（`/admin/#page-synergy` 渲染 + status badge）
- [ ] 第一個 LLM-driven recommendation 驗證（slog 出現 `agent_loop.start` event）
- [ ] Week 1 第一筆 entry 建立（Day 1）
- [ ] Daily check-in 排程（若手動，設定 calendar；若 auto，確認 Issue #825 auto-cron 已部署）

---

## References

- **Runbook**：`docs/operations/l2-4-runbook.md`
- **Spec**：`docs/specs/l2-4-observation-spec.md`
- **Followup**：`docs/archive/l2-4-followup.md`
- **前例（已完成 cycle）**：`內部觀察日誌（已刪除）`
- **PR #746**（metrics ship）：`fix(orchestrator): align SemiconductorLLMAgent metrics to Issue #740 spec`（commit `eff0db79`，2026-06-25）
- **PR #821**（scheduling + admin panel ship）：`feat(L2.4): observation scheduling API + admin panel for LLM-driven sector agent`（2026-06-29）