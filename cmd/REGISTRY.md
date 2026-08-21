# cmd/ Binary Registry

> 本文件為 `cmd/` 目录下所有 binary 的索引。
> 資料來源：`audit-cmd-registry/cmd-registry-audit.md`（2026-07-24）
>
> **維護原則**：
> 1. 新增 binary 必須同步更新本表
> 2. DEPRECATED binary 列入「建議移除」段落
> 3. Spike binary 須註明預計移除版本
> 4. 每個 binary 至少要有 doc comment 說明用途

---

## 格式

每行：`| binary | 用途 | 主要 internal 依賴 | 狀態 |`

狀態標記：
- ✅ Stable — 生產穩定，介面已固化
- ✅ Evolving / ✅ Utility / ✅ E — 生產使用但仍在演進
- ⚠️ Experimental — 實驗中，不保證穩定
- 🔴 Deprecated — 建議移除

---

## 1. Orchestrator（生產主程序）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/atlas` | 主程式 — 三層 executor 路由、Stage 3/4、分析引擎、HTTP API、Dashboard | orchestrator, strategy_techniques, risk, narrative, macroflow, capitalflow, subscription, recommender, dailyreport, eventdriven, llm | ✅ Stable |
| `cmd/atlas-mcp` | MCP server — 117 tools、stdio/SSE/HTTP transports、multi-tenant auth、audit | `cmd/atlas-mcp/server`, db, constants | ✅ Stable |
| `cmd/atlas-mcp-setup` | 互動式 wizard — 偵測 MCP 客戶端並寫入設定檔 | portprobe, server.NewHTTPClient | ✅ Utility |
| `cmd/atlas-stage4-loader` | Stage 4 PR#2 — 將 4 個 staging JSONL 寫入 SQLite | ledger, constants | ✅ E |
| `cmd/atlas-stage4-backfill` | Stage 4 PR#1 — 從 ledger 擷取 regime/event/stress/prediction 90 天歷史 | —（stdlib only） | ✅ E |
| `cmd/daily-replay-sync` | cron 同步 TWSE CSV 到 JSONL/SQLite 雙後端 | db, marketdata, monitoring, orchestrator | ✅ Evolving |
| `cmd/cron-quote-backfill` | cron 每日增量拉取 FinMind quotes | marketdata, config, domain, ledger | ✅ Utility |

---

## 2. Backtest（回測引擎）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/backtest-window` | 單視窗回測，支援 param-override | backtest, config, ledger, monitoring | ✅ Evolving |
| `cmd/backtest-pipeline` | 滾動 OOS 回測 + ML 模型訓練（OLS/PCR/PLS/ElasticNet/GLM/RF） | backtest, domain, feature, ml, replay | ✅ Evolving |
| `cmd/backtest-event-flow` | Stage 4 PR#3 — 預測 backtest，評估事件驅動資金流預測準確率 | capitalflow, eventdriven, industry, ledger | ✅ E |
| `cmd/janus-backtest` | Janus regime weighting 引擎回測 | janus, backtest, prism, config, ledger | ⚠️ Experimental |

---

## 3. Calibrate（校準工具）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/calibrate-parameters` | 執行 calibration cycle（garch/var/darwinian/factor/all） | calibration | ✅ Utility |
| `cmd/calibrate-baselines` | 一次性 bootstrap stress index hybrid baselines.json | narrative | ✅ Utility |
| `cmd/calibrate-seasonal` | 產業季節性 pattern 校準，支援真實 replay 或合成數據 | industry, config, domain, replay | ✅ Evolving |
| `cmd/calibrate-rsi-tw` | RSI-TW 散戶情緒指數校準（VIX/Margin） | config, marketdata | ✅ Utility |
| `cmd/calibrate-stress-index` | Taiwan stress index 權重校準（252 日歷史窗口） | narrative | ✅ Evolving |
| `cmd/calibrate-thresholds` | 產業 revenue threshold 校準 | industry | ✅ Utility |
| `cmd/calibration-validate` | 校準結果有效性驗證（48h stale gate） | config | ✅ Utility |

---

## 4. Backfill（數據回填）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/backfill-quotes` | TWSE quotes 增量回填（JSONL + SQLite 雙寫） | replay, domain, ledger | ✅ Utility |
| `cmd/backfill-replay` | TWSE FinMind quotes 歷史回填（**已移除 2026-07-25**；binary / Dockerfile 引用已清理，FinMind API 402） | constants, db, monitoring | ❌ Removed |
| `cmd/backfill-taifex-oi` | TAIFEX 外資期貨未平倉歷史（FinMind 版，2026-07-25 #1338 移除，FinMind 402） | marketdata, constants, monitoring | ❌ Removed |
| `cmd/backfill-taifex-oi-v2` | TAIFEX 外資期貨未平倉歷史回填（官網 CSV 逐日 → macro snapshot `foreign_futures_oi_net`，macrobackfill 模式） | —（stdlib + x/text） | ✅ Utility |
| `cmd/backfill-institutional-investors` | 三大法人日成交明細回填 | marketdata, constants, monitoring | ✅ Utility |
| `cmd/backfill-month-revenue` | 月營收回填（FinMind） | marketdata, constants, monitoring | ✅ Utility |
| `cmd/backfill-financial-statements` | 財務報表回填 | marketdata, constants, monitoring | ✅ Utility |
| `cmd/backfill-industry-tree` | 產業分類權重每日動態計算 | config, marketdata | ✅ Utility |
| `cmd/backfill-fundamentals-ps-sector` | 修補 fundamentals.json 缺少的 PS/Sector 欄位 | —（stdlib only） | ✅ Utility |
| `cmd/backfill-summaries` | 補建 orphan session 的 summary.json | backfill, config | ✅ Utility |
| `cmd/reconcile-sessions` | PG vs JSONL session summary 對帳 — 對稱差報告 + `-apply` 回填單邊缺口 (B6) | reconcile, ledger, repository, config | ✅ Utility |
| `cmd/backfill-var-returns` | 將 session summary 聚合進 VaR return 計算 | —（stdlib only） | ⚠️ Utility |
| `cmd/backfill-margin-history` | TWSE 融資餘額歷史回填（2024-07 起，data/state/margin/*_margin.json，one-shot） | narrative, marketdata, constants | ✅ Utility |

---

## 5. Experiment（實驗管理）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/run-experiment` | 執行 mutation experiment 回放 | experiment, baseline, config, ledger | ✅ Evolving |
| `cmd/judge-experiment` | 統計打分（Welch t-test、Sharpe、Ddwn）並寫入 DB | experiment, config, db, ledger, monitoring, repository | ✅ Evolving |
| `cmd/promote-baseline` | 將 accepted experiment 升級為 baseline policy | baseline, config, experiment | ✅ Stable |
| `cmd/revert-baseline` | 回滾 baseline 到指定版本 | baseline, config | ✅ Stable |

---

## 6. Fetch / Ingest（數據攝入）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/fetch-historical` | TWSE historical bars 批量抓取（MI_INDEX） | config, constants, marketdata/twse | ✅ Utility |
| `cmd/fetch-historical-capital-flow` | TWSE T86 三大法人歷史回填 | constants | ✅ Utility |
| `cmd/macro-ingest` | Macro snapshot + geopolitical ingest | db, marketdata, monitoring, narrative | ✅ Evolving |
| `cmd/geo-ingest` | 地緣政治分數攝入（RSS + GDELT） | db, monitoring, narrative | ✅ Utility |
| `cmd/extend-replay-etf` | TWSE ETF 歷史日棒抓取 | constants, marketdata/twse, apigateway/httpclient | ✅ Utility |
| `cmd/import-replay` | CSV → JSONL 格式轉換 | importer | ✅ Utility |
| `cmd/merge-replay` | 多個 JSONL 合併為 replay CSV | —（stdlib only） | ✅ Utility |

---

## 7. Convert / Migration（格式轉換與遷移）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/convert-baseline-policy` | baseline_policy.json 格式轉換 | baseline | ✅ Utility |
| `cmd/convert-experiment-results` | 實驗結果格式轉換 | domain | ✅ Utility |
| `cmd/convert-experiments-jsonl` | experiments JSONL 格式轉換 | domain | ✅ Utility |
| `cmd/convert-recommendation-outcomes` | recommendation_outcomes 格式轉換 | domain | ✅ Utility |
| `cmd/migrate-data` | JSONL → PostgreSQL 全量遷移（metrics/alerts/outcomes/screening/...） | db, config, domain, marketdata | ✅ Utility |
| `cmd/migrate-jsonl-to-sqlite` | JSONL ledger → SQLite 遷移 | domain, ledger | ✅ Utility |

---

## 8. Check / Health（健康檢查）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/check-maturity` | CI 檢查所有 internal doc.go 有 Maturity tag | —（stdlib only） | ✅ Stable |
| `cmd/check-data-health` | Replay CSV 健康檢查（日期範圍、延遲天數） | config, replay | ✅ Utility |
| `cmd/check-persistence-format` | 掃描 data/state 持久性格式分類 | domain | ✅ Utility |
| `cmd/cleanup-channel-health` | 清理過期 alerts | domain, monitoring | ✅ Utility |
| `cmd/parameter-health-check` | 參數品質報告（citation/todo/calibrated/evidence 等級） | —（stdlib only） | ✅ Utility |
| `cmd/validate-parameters` | 參數 JSON schema 驗證 | config, constants | ✅ Utility |
| `cmd/realtime-quote` | 即時報價串流（Fugle → Redis pub/sub） | realtime, domain | ⚠️ Utility |
| `cmd/fubon-dma` | 富邦 DMA 券商介面 CLI（login/submit/cancel/query/logout） | domain, live | ⚠️ Utility |
| `cmd/stress-test` | 壓力測試情境執行（market_crash/sector_rotation/liquidity_crisis） | stress-test/internal/risktest | ⚠️ Experimental |

---

## 9. Meta / DevOps（開發輔助工具）

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `cmd/gentags` | 從 Go struct 自動生成前端 TypeScript types + valid_fields.json | —（stdlib only） | ✅ Utility |
| `cmd/mapgen` | 系統架構圖生成（arch/routes/completeness/fe-be） | —（stdlib only） | ✅ Utility |
| `cmd/lint-pr` | Git diff → LLM code review（MiniMax + DeepSeek） | llm, config, adapters, capabilities, clients, schemas | ✅ Utility |
| `cmd/lint-prompts` | Prompt 檔案 LLM linting | llm, config, adapters, capabilities, clients, schemas | ✅ Utility |
| `cmd/archive-state` | 狀態目錄週期性歸檔（建議外部 cron 呼叫） | —（stdlib only） | ✅ Utility |

---

## 10. Experimental（PoC / Staging）

> 詳見 `cmd/experimental/AGENTS.md` — dry-run 禁令、dummy mode、live broker 管制。

| Binary | 用途 | 主要 internal 依賴 | 狀態 |
|--------|------|---------------------|------|
| `experimental/validate-risk-gate` | 風險閘 validation：session 勝率/blocked 率統計 | risk | ⚠️ Experimental |
| `experimental/validate-stress-index` | Stress index validation（replay data） | narrative | ⚠️ Experimental |
| `experimental/validate-twse-capital-flow` | TWSE 資金流 validation（非 production path） | marketdata | ⚠️ Experimental |
| `experimental/validate-broker` | Broker adapter signature format validation | live | ⚠️ Experimental |
| `experimental/stress-test` | 內建模境 stress test（internal/stress） | stress | ⚠️ Experimental |
| `experimental/test-hybrid` | Hybrid provider（Fubon → Fugle → TWSE）測試 | marketdata | ⚠️ Experimental |
| `experimental/staging-drill-strategy-techniques` | Phase 0/1 smoke drill（seed loading） | strategy_techniques, orchestrator, config | ⚠️ Experimental |
| `experimental/sector-allocation-closure-preflight` | Sector allocation closure pre-flight checklist | — | ⚠️ Experimental |
| `experimental/plugin-poc` | Plugin boundary 注入點驗證 | orchestrator, config, domain | ⚠️ Experimental |
| `experimental/plugin-e2e` | Plugin 完整端到端驗證（JSON spec + 自定義 executor） | orchestrator, config, domain | ⚠️ Experimental |
| `experimental/optimize-parameters` | 參數網格搜尋（暴力試誤） | config | ⚠️ Experimental |
| `experimental/janus-backtest` | Janus regime weighting 回測 | janus, backtest, prism | ⚠️ Experimental |
| `experimental/janus-status` | Janus 引擎狀態查詢 | janus, prism | ⚠️ Experimental |
| `experimental/c07-spot-check-recorder` | C07 人工抽檢記錄 | — | ⚠️ Experimental |
| `experimental/c07-preflight` | C07 pre-flight checklist（L2.4 模式克隆） | — | ⚠️ Experimental |
| `experimental/c07-day-evaluator` | C07 Day 7/14 acceptance gate 評估 | — | ⚠️ Experimental |
| `experimental/c07-obs-collector` | C07 每日觀測日誌自動填寫 | — | ⚠️ Experimental |

---

## Dead Code（建議移除）

### 🔴 Deprecated
| Binary | 原因 | 依據 |
|--------|------|------|
| `cmd/atlas-mcp-server-sdk-spike` | doc comment：「will be removed in T2.2–T2.4」 | cmd/atlas-mcp-server-sdk-spike/spike.go |

### ⚠️ Orphan Pattern（需確認是否仍在使用）

| Binary | 疑慮 | 建議 |
|--------|------|------|
| `cmd/backfill-var-returns` | 只依賴 stdlib，寫入 session summary 聚合 | 確認是否有 cron 調用 |
| `cmd/realtime-quote` | 需要 Redis + Fugle API Key；live trading path 受 `live-trading.guardrails` 管制 | 確認是否仍在使用 |

---

## cmd ↔ internal 模組對應缺口

### internal 模組無對應獨立 binary

以下 internal 模組全部由 `cmd/atlas` 作為 runtime 宿主，無獨立 binary：

| internal 模組 | AGENTS_INDEX 分類 | 備註 |
|---------------|-------------------|------|
| `llm`（含 adapters, clients, capabilities, schemas） | — | cmd/atlas runtime |
| `portfolio` | E · Evolving | cmd/atlas import |
| `screener` | S · Stable | cmd/atlas runtime |
| `industry` | S · Stable | cmd/atlas + backtest-event-flow |
| `ledger` | S · Stable | cmd/atlas runtime |
| `risk` | S · Stable | cmd/atlas runtime |
| `monitoring` | S · Stable | cmd/atlas runtime |
| `narrative` | S · Stable | cmd/atlas runtime |
| `orchestrator` | S · Stable | cmd/atlas runtime |
| `repository` | S · Stable | cmd/atlas runtime |
| `scheduler` | E · Evolving | cmd/atlas runtime |
| `sim` | S · Stable | cmd/atlas runtime |
| `spawning` | S · Stable | cmd/atlas runtime |
| `storage` | S · Stable | cmd/atlas runtime |
| `strategy_techniques` | S · Stable | cmd/atlas runtime |
| `tax` | S · Stable | cmd/atlas runtime |
| `live` | — | cmd/atlas runtime，live-trading.guardrails 管制 |
| `llm_annotator` | — | cmd/atlas runtime |
| `marketexplain` | — | cmd/atlas runtime |
| `marketdata`（含 realtime 子模組） | S · Stable | cmd/atlas runtime |
| `autobacktest` | E · Evolving | cmd/atlas import |
| `prism` | E · Evolving | cmd/atlas import，janus-backtest 使用 |
| `sectorallocation` | E | cmd/atlas import |
| `ml` | E · Evolving | cmd/backtest-pipeline |
| `feature` | E · Evolving | cmd/backtest-pipeline |
| `strategy` | E · Evolving | cmd/atlas import |
| `strategy_ranker` | X · Experimental | cmd/atlas import |
| `recommender` | E · Evolving | cmd/atlas import |
| `dailyreport` | E · Evolving | cmd/atlas import |
| `eventdriven` | E · Evolving | cmd/atlas + backtest-event-flow |
| `capitalflow` | X · Experimental | cmd/atlas + backtest-event-flow |

---

## 統計摘要

| 指標 | 數值 |
|------|------|
| 頂層 cmd 目錄 | 61 個 |
| experimental 子目錄 | 18 個 |
| **總 binary 數量** | **79 個** |
| Dead code（Deprecated + Spike） | 2 個 |
| Orphan（需確認） | 2 個 |
