# Wave 10 L2.4 — LLM-Driven Sector Agent Observation Log

> **觀察期啟動日期**: 2026-06-25
> **目的**: 追蹤 LLM-driven sector agent（透過 `SectorAgentLLM` + `PlanReflectRunner`）在 production-like 環境的行為，與確定性 sector agent 對比。
> **必備前置**: L2.3 function calling PoC 完成、LLM 真的接入 `SectorAgentLLM.LLM` driver。

---

## 觀察指標

每個 LLM-driven agent 每次 recommendation 都需記錄：

| 指標 | 說明 | 目標 |
|------|------|------|
| Loop rounds | Plan → reflect 重複次數 | 1-3 |
| Tool calls / rec | 平均每次 recommendation 呼叫的 tool 次數 | 1-2 |
| LLM latency (p50/p95) | 從 PlanStep 到 Final 的端到端延遲 | < 2s / < 5s |
| Conviction distribution | Final conviction 0-100 分佈 | 與確定性 agent 對齊 |
| Hit rate (vs deterministic) | LLM-driven vs 確定性 agent 在同一視窗的勝率 | ≥ 確定性 |
| Sharpe | 同 Sharpe 公式對比 | ≥ 確定性 |
| Token cost / rec | 每次 recommendation 的 token 用量 | < $0.01 |

---

## 觀察記錄格式

每週（或每 N 次 recommendation）追加一筆：

```markdown
### YYYY-MM-DD — Week N
- Loop rounds: avg=X.X, max=N
- Tool calls / rec: avg=X.X
- LLM latency: p50=Xms, p95=Xms
- Conviction distribution: [0-20: N, 20-40: N, 40-60: N, 60-80: N, 80-100: N]
- LLM-driven hit rate: X.X% (N recs)
- Deterministic hit rate: X.X% (N recs, same window)
- Token cost: $X.XX (N recs)
- 異常: ...
- 決策: 繼續觀察 / 擴展到更多 sector / 回滾
```

---

## Week 0 — Baseline（觀察期啟動前）

- 觀察期啟動日: 2026-06-25
- LLM-driven agent: **尚未啟用**（L2.3 PoC 與 L2.4 整合未完成）
- 觀察對象: 確定性 sector agent（SemiconductorExecutor 等 13 個）
- 觀察窗口: 2026-06-25 ~ 2026-07-02
- 預期累計: N/A（等待 LLM agent 啟用）

---

## 啟動 Checklist（給執行者）

- [ ] L2.3 function calling PoC 完成（adapter payload + `query_sector_momentum` tool 實作）
- [ ] L2.4 `SectorAgentLLM.LLM` 接入真的 LLM driver（不是 stub）
- [ ] Feature flag `UseLLMSectorAgents` 加上（預設 `false`，opt-in 啟用）
- [ ] 同一個 sector 同時跑 LLM-driven 與確定性版本，輸出到 `data/state/observation/{sector}/` 對比
- [ ] 觀察期 4 週後由 oracle audit 決定是否擴展到所有 sector agents

---

## 已知限制

1. **Burn-in 期間不適用**：MaturityBurnIn 期間所有 auto 機制靜默，LLM-driven agent 也不應啟用。
2. **Token 成本需嚴格控制**：每次 recommendation 預算 < $0.01，月度 token 預算上限待定。
3. **Determinism 缺失**：LLM-driven agent 不可用於 backtest（replay 階段仍走確定性路徑）。
4. **失敗模式不同**：LLM 隨機性 vs 確定性，Darwinian weight 計算需適配。
