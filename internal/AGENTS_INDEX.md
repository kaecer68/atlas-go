# AGENTS_INDEX.md — 模組 AGENTS.md 索引

> 進入 `internal/<mod>/` 工作前，先讀該目錄下的 `AGENTS.md`（或 `CONSTITUTION.md`）。模組特有陷阱寫在裡面，跳過會踩坑。

## 索引

| 模組 | 成熟度 | 關鍵主題 |
|------|--------|---------|
| `orchestrator` | stable | 三層 executor 路由、GuardOutcomes 對齊、PluginHost |
| `sim` | stable | 模擬引擎、部位狀態轉換、JSONL replay |
| `experiment` | evolving | 實驗生命週期、Mutation → execute → judge → promote |
| `baseline` | stable | Policy 升降級、版本控制、A/B 測試 |
| `ledger` | stable | JSONL append-only、OutcomeCount 計算 |
| `portfolio` | evolving | Darwinian 權重、FactorEngine、FactorType 變更流程 |
| `marketdata` | stable | Provider 抽象、TWSE / FinMind / Fugle |
| `live` | experimental | 交易安全旗標、`-allow-live-broker` |
| `prism` | evolving | Regime-specific 訓練佇列 |
| `janus` | evolving | 跨 cohort regime 偵測、PRISM 權重 |
| `narrative` | evolving | 宏觀敘事、因果鏈、台灣壓力指數 |
| `risk` | evolving | RiskManager、VaR、宏觀回撤、自主校準 |
| `industry` | evolving | 產業輪動、供給需求、季節性、週期 |
| `monitoring` | stable | Dashboard API、監控、人工干預入口 |
| `eventbus` | stable | 事件匯流排 |
| `spawning` | stable | Agent 生成管理 |
| `tax` | stable | 台灣稅務計算 |
| `realtime` | stable | 即時資料轉接器 |
| `apigateway` | stable | **CONSTITUTION.md** — API Gateway、BackgroundTaskManager、架構憲法 |

## 成熟度定義

- `stable` — 介面與行為穩定，可安全使用
- `evolving` — 活躍開發中，修改前請讀 AGENTS.md
- `experimental` — 功能未完全穩定，謹慎使用

## 無 AGENTS.md 的模組

以下為共享基礎設施，直接讀碼即可：

`screener`、`config`、`db`、`repository`
