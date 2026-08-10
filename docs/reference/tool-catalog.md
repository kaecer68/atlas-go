# atlas-mcp Tool Catalog

> **119 tools**（預設啟用；sampling/elicitation feature-gated 全開時 119+）grouped by functional area. For investor use cases, see [`docs/investor/use-cases/`](../investor/use-cases/).
> For natural language query examples, see [`docs/investor/query-examples.md`](../investor/query-examples.md).

## 工具數量

業務 110+ + template_detector 2 + audit 4 + audit_state 1 + strategy_for_period 1 + stock_get_monthly_revenue 1 + Phase 2 alert lifecycle 4 = 117+（**基礎 119+**, **+2** sampling/elicitation feature-gated 預設關閉 → 最多 121+；啟動期 assert ∈ [115, 121]，見 `cmd/atlas-mcp/server/server.go`）

## 完整工具 Catalog（121 個 tool 槽位，其中 119+ 個預設啟用；Phase 2 與 PR 1/2/3 全部上線）


### Regime（1 個）
| Tool | 用途 |
|------|------|
| `regime_get_history` | 指定期間的市場體制歷史（RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL） |

### Macro（6 個）
| Tool | 用途 |
|------|------|
| `macro_get_snapshot_latest` | 最新 macro snapshot |
| `macro_get_snapshot_history` | Macro snapshot 歷史（days 參數）|
| `macro_get_stress_index_current` | 當前 stress index |
| `macro_get_stress_index_history` | Stress index 歷史 |
| `macro_get_capital_flow_latest` | 外資/法人/散戶資金流 snapshot |
| `macro_get_ingest_status` | 通道 ingest 狀態 |

### Capital Flow（3 個）
| Tool | 用途 |
|------|------|
| `capital_flow_daily` | 台股每日七維錢潮雷達（3+2+2 分層）分解 + 共振強度：官方法人（外資 / 投信 / 自營商）+ 行為代理（官股 / 散戶）+ 領先／跨市場訊號（期貨 / TSM ADR）。actor 共識僅計入官方actor 層；詳見 `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 |
| `capital_flow_summary` | 資金流向摘要（適合晨報）；摘要敘事來自 official_actor 共識＋行為／訊號層支援 |
| `explain_market_move` | 「為什麼漲跌」市場解說（繁體中文）。支援 `format=emoji`（預設）或 `format=plain` 控制表情符號。回傳大盤表現、資金面、國際環境與風險提示 |

### Crossmarket（3 個）
| Tool | 用途 |
|------|------|
| `crossmarket_get_status` | 跨市場資料源狀態 |
| `crossmarket_get_correlation` | 台股 sector vs US indices 相關性 |
| `crossmarket_get_us_indices` | S&P 500 / NASDAQ / Dow Jones / SOX / NVDA / AAPL / MSFT / TSM ADR 即時 snapshot（live-fetched from Yahoo Finance） |

### Narrative（7 個）
| Tool | 用途 |
|------|------|
| `narrative_get_events` | 最新敘事事件 |
| `narrative_get_chains` | 因果鏈 |
| `narrative_get_models` | 敘事模型清單 |
| `narrative_get_templates` | 因果模板 |
| `narrative_get_seasonal` | 季節性敘事 |
| `narrative_get_bundle` | 編譯好的 briefing bundle |
| `narrative_stress_index_thresholds` | Stress index 門檻值 |

### Events（2 個）
| Tool | 用途 |
|------|------|
| `event_calendar` | 近期市場事件日曆（營收、ETF 換股、MSCI、休市） |
| `event_flow_prediction` | 未來 5 天事件驅動資金流預測 |

### Template Detector（2 個 — Stage 5 PR#4 Stage B 新增）
| Tool | 用途 |
|------|------|
| `template_detector_status` | 最近一次（或前 N 次）trigger theme scan 結果（從 ledger.detector_scan_log 讀取） |
| `detector_registry_list` | narrative.DetectorRegistry 內所有 detector 的 theme + enable/disable 狀態 |

### Risk（5 個）
| Tool | 用途 |
|------|------|
| `risk_get_metrics` | 風險聚合指標 |
| `risk_get_correlation_matrix` | 跨策略相關性 |
| `risk_get_drawdown` | 當前 / 最大 drawdown |
| `risk_get_calibration` | 風險模型校準 |
| `risk_get_commentary` | 風險敘事 |

### Alert（4 個：1 Phase 1 + 3 Phase 2.2）
| Tool | 用途 |
|------|------|
| `alert_list_unacknowledged` | 未確認警報（Phase 1）|
| `alert_list` | 所有警報 |
| `alert_get_stats` | 警報統計 |
| `alert_get_rules` | 警報規則配置 |

### Strategy（7 個：1 Phase 1 + 5 Phase 2.2 + 1 M4）
| Tool | 用途 |
|------|------|
| `strategy_list_active` | Production 線上策略（Phase 1）|
| `strategy_get_layers` | L1-L5 層級清單 |
| `strategy_get` | 單筆策略 |
| `strategy_get_attribution` | 績效歸因 |
| `strategy_get_summary` | 策略摘要 |
| `strategy_ranker` | 依勝率排序的策略排名（free / registered / premium tier）|
| `strategy_for_period` | 給定市場時期（downturn/turnaround_up/bull/plateau/consolidation/turnaround_down/black_swan）回傳適用策略清單（含 category/priority）；讀 `configs/methodology_rules.yaml` 同源 MethodologyAdvisor（M4）|

### Recommendation（1 個）
| Tool | 用途 |
|------|------|
| `get_recommendations` | 依訂閱層級回傳市場 overview、活躍策略與排序建議 |

### Experiment（5 個：1 Phase 1 + 2 Phase 2.2 + 2 PR 3）
| Tool | 用途 |
|------|------|
| `experiment_judge` | 觸發統計式 replay judge：對比 baseline vs candidate 並跑 17 種統計 gate（Welch t-test、Sharpe 穩定性、回撤保護、OOS 驗證等；非 LLM 評審）（Phase 1，destructiveHint=true）|
| `experiment_diff` | 候選 vs baseline 差異 |
| `experiment_history` | 歷史實驗清單 |
| `experiment_promote` | 候選實驗 promote 為 baseline（PR 3，destructiveHint=true，需 ATLAS_API_KEY）|
| `experiment_revert` | 候選實驗 revert（PR 3，destructiveHint=true，需 ATLAS_API_KEY）|

### Synergy（3 個）
| Tool | 用途 |
|------|------|
| `synergy_get_darwinian_status` | 達爾文權重當前狀態 |
| `synergy_get_darwinian_trend` | 達爾文權重歷史趨勢 |
| `synergy_get_l2_4_schedule` | L2.4 觀察窗口時程 |

### Control（7 個：4 read-only + 3 write — PR 3）
| Tool | 用途 | 備註 |
|------|------|------|
| `control_get_audit_log` | 控制覆寫 audit log | read-only |
| `control_get_active_overrides` | 當前 active 覆寫 | read-only |
| `control_approve_recommendation` | 批准推薦狀態 | read-only |
| `control_reject_recommendation` | 拒絕推薦狀態 | read-only |
| `control_pause_agent` | 暫停特定 agent（PR 3）| destructiveHint=true，需 ATLAS_API_KEY |
| `control_resume_agent` | 恢復已暫停 agent（PR 3）| destructiveHint=true，需 ATLAS_API_KEY |
| `control_sector_ban` | 禁止特定 sector 新倉位（PR 3）| destructiveHint=true，需 ATLAS_API_KEY |

> 寫入操作（pause/resume-agent、sector-ban）需後端 auth 強制保護，ATLAS_API_KEY 必須設定。MCP server 透過 `X-API-Key` header 轉發 API key。

### Scheduler / Task（4 個 — PR #1277 scheduler health split）
| Tool | 用途 |
|------|------|
| `scheduler_get_status` | 背景排程器狀態（含 `data_health` 欄位：channel freshness、ingestion lag；`summary` 計算各 status count；與 `system_health` 分離，PR #1277 + #1354） |
| `task_list` | 背景任務清單 |
| `task_get` | 單筆任務 |
| `task_get_events` | 任務 lifecycle events |
> **CI 防回歸**：`tools_canary_test.go`（PR #1276）在 CI 對 98 個 read-only tools 跑 runtime smoke test，確保所有 tool response 非空且符合基本 sanity。

### System（7 個：1 Phase 1 + 6 Phase 2.2）
| Tool | 用途 |
|------|------|
| `system_get_health` | 系統健康總覽（Phase 1）|
| `system_get_metrics` | 即時 metrics |
| `system_get_metrics_trend` | Metrics 趨勢 |
| `system_get_thresholds` | SLO 門檻值 |
| `system_get_data_pipeline` | Data pipeline 狀態 |
| `system_get_circuit_breaker` | Live 風險熔斷器狀態（非資料通道 breaker） |
| `system_get_maturity` | 模組成熟度評分 |

### LLM（2 個）
| Tool | 用途 |
|------|------|
| `llm_get_cost` | LLM 成本 snapshot |
| `llm_get_health` | LLM router health |

### Trace（4 個）
| Tool | 用途 |
|------|------|
| `trace_get_sim_latest` | 最新 simulation trace |
| `trace_get_agent_observatory` | Agent 活動觀測 |
| `trace_get_decision_chain` | 決策因果鏈 |
| `trace_get_reasoning` | 推理 trace |

### Data（4 個）
| Tool | 用途 |
|------|------|
| `data_get_channels` | 資料通道清單 |
| `data_get_channel_detail` | 單一通道 detail |
| `data_get_quality` | 資料品質 metrics |
| `data_get_field_contract` | Field contract schema |

### Stock（4 個）
| Tool | 用途 |
|------|------|
| `stock_get_quote` | 個股即時報價（最新價、漲跌、成交量） |
| `stock_get_fundamentals` | 個股基本面（PE、PB、PS、殖利率、sector 等） |
| `stock_get_chips` | 個股籌碼面（法人/外資/投信買賣超，可選日期） |
| `stock_get_technical` | 個股技術面（收盤價、均線、RSI，預設 90 天、上限 365 天） |

> **API Contract**：[`../specs/stock-api-contract.md`](../specs/stock-api-contract-spec.md) 定義 4 個 `/api/stock/*` endpoint 的 typed schema（含 Symbol normalization 規則、單位、欄位）。
> **Frontend 狀態**：client_web「個股快查」頁面（Issue #1038）已 ship — 後端 normalize（PR-A #1044）+ 前端 14 檔（PR-B #1045）+ 文件同步（PR-C #1046）+ RSI pre-existing bug fix（PR #1047）+ E2E unskip（PR #1274）。頁面路徑 `/client/quote?symbol=<4-6 digit symbol>`。

### Parameters（5 個 — PR 2 新增）
| Tool | 用途 |
|------|------|
| `parameters_get` | 當前參數 flat map（key → type）|
| `parameters_get_categories` | 參數分類清單（darwinian/factor/optimizer/sizing/health 等）|
| `parameters_get_audit_log` | 參數變更稽核紀錄 |
| `parameters_get_metadata` | 參數含 provenance（rationale、source、citation）|
| `parameters_get_snapshots` | 參數歷史快照（days 參數，預設 20、上限 365）|

### Backtest（2 個 — PR 2 新增）
| Tool | 用途 |
|------|------|
| `backtest_status` | 回測執行狀態摘要（最後一次自動回測日期與 portfolio 值）|
| `backtest_signals` | 當前 auto-backtest 訊號（active_signals、VaR、Sharpe、drawdown）|

### Industry Extension（3 個 — PR 2 新增）
| Tool | 用途 |
|------|------|
| `sector_allocation_plan` | 產業配置計畫（base_weight、adjusted_weight、derivation_factors）|
| `channel_health` | 通道健康（channel_id、status、updated_at）|
| `risk_exposure` | 投資組合風險敞口（VaR/CVaR、sector/factor/concentration breakdown）|

> 這 3 個 tool 提供 MCP 對 frontend `industry.js`、`pipeline.js`、`risk.js` 等頁面所需資料的鏡像存取。
> 註：`calendar_events`（deprecated）與 `taiwan_stress_index`（與 `macro_get_stress_index_current` 重複）已於 2026-08-07 移除。

### Sector Canonical（2 個 — FU-7 Phase F 新增）
| Tool | 用途 |
|------|------|
| `industry_sector_list` | 列出所有 20 個 sector 的 canonical ID、中文標籤、代表股 |
| `industry_sector_lookup` | 依股號（2330）或 sector 名稱（半導體 / semiconductor）查 sector 資訊 |

### Universe（2 個）
| Tool | 用途 |
|------|------|
| `universe_get_sessions` | Simulation session 清單 |
| `universe_get_universe_overlap` | Universe overlap 分析 |

### Report（4 個）
| Tool | 用途 |
|------|------|
| `report_get_daily_summary` | 每日摘要報告 |
| `report_get_performance` | 績效報告 |
| `report_get_tax_snapshot` | 稅務 snapshot |
| `report_get_export_link` | 匯出連結（短 TTL）|

### Briefing（2 個）
| Tool | 用途 |
|------|------|
| `mcp_quickstart` | 一站式市場速覽：macro snapshot、活躍策略、壓力指數、事件、資金流向 |
| `daily_report` | 最新每日市場報告（JSON + Markdown） |

### Protocol Extensions（4 個 — Phase 4 B，agent 友善擴充）

| Tool | 用途 | 啟用狀態 |
|------|------|---------|
| `mcp_roots_list` | 列出 MCP client 宣告的 file:// roots | always on |
| `mcp_roots_read_file` | 讀取 root 內檔案（O_RDONLY、path-traversal + symlink escape 防護） | always on |
| `mcp_elicit_user` | 向使用者請求結構化輸入（schema validate） | 預設 OFF，`ATLAS_MCP_ELICITATION_ENABLED=true` |
| `mcp_sample_llm` | 透過 atlas LLM router 抽樣（讓 server 呼叫 LLM 完成 model-assisted 工具） | 預設 OFF，`ATLAS_MCP_SAMPLING_ENABLED=true` |

> 安全邊界：`mcp_roots_read_file` 強制 O_RDONLY、TOCTOU 防護、size cap、AllowedRoots 檢查；詳見 [spec Phase 4 B](../specs/agent-mcp-server-spec.md)。

### MCP Audit / Observability（7 個 — agent 自我觀測）

| Tool | 用途 |
|------|------|
| `mcp_get_session_topology` | 回傳 Agent×Tool 呼叫矩陣（哪個 agent 用了哪些 tool） |
| `mcp_get_call_stats` | Tool 呼叫頻率與延遲統計 |
| `mcp_get_tenant_usage` | Multi-tenant 用量報表 |
| `mcp_get_top_slow_tools` | 最慢的 N 個 tool（延遲排行） |
| `mcp_anomaly_get_recent` | 近期異常事件（error spike、延遲飆升） |
| `mcp_anomaly_ack` | 標記異常為已確認 |
| `audit_state` | 憲章審計追蹤表快照（§附錄 D 22 項 + §附錄 F 14 行 + 統計）；供 agent self-audit 憲章對齊狀態 |

> 這些 tool 屬於 atlas-mcp 的自我觀測層，供 agent 了解自己的呼叫模式與系統健康。

### Prism（1 個）
| Tool | 用途 |
|------|------|
| `prism_get_training_results` | PRISM cohort 訓練結果 |

---


