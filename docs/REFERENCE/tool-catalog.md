# atlas-mcp Tool Catalog

> **91 tools** grouped by functional area. For investor use cases, see [`docs/INVESTOR/use-cases/`](../INVESTOR/use-cases/).
> For natural language query examples, see [`docs/INVESTOR/query-examples.md`](../INVESTOR/query-examples.md).
>
> This is the authoritative catalog (moved from `docs/REFERENCE/tool-catalog.md` which is deprecated).

## 完整工具 Catalog（約 91 個 tool，Phase 2 全部上線）

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

### Capital Flow（2 個）
| Tool | 用途 |
|------|------|
| `capital_flow_daily` | 台股每日七大資金勢力分解 + 共振強度 |
| `capital_flow_summary` | 資金流向摘要（適合晨報） |

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

### Strategy（6 個：1 Phase 1 + 5 Phase 2.2）
| Tool | 用途 |
|------|------|
| `strategy_list_active` | Production 線上策略（Phase 1）|
| `strategy_get_layers` | L1-L5 層級清單 |
| `strategy_get` | 單筆策略 |
| `strategy_get_attribution` | 績效歸因 |
| `strategy_get_summary` | 策略摘要 |
| `strategy_ranker` | 依勝率排序的策略排名（free / registered / premium tier）|

### Recommendation（1 個）
| Tool | 用途 |
|------|------|
| `get_recommendations` | 依訂閱層級回傳市場 overview、活躍策略與排序建議 |

### Experiment（3 個：1 Phase 1 + 2 Phase 2.2）
| Tool | 用途 |
|------|------|
| `experiment_judge` | 觸發 LLM judge 評分（Phase 1，destructiveHint=true）|
| `experiment_diff` | 候選 vs baseline 差異 |
| `experiment_history` | 歷史實驗清單 |

### Synergy（3 個）
| Tool | 用途 |
|------|------|
| `synergy_get_darwinian_status` | 達爾文權重當前狀態 |
| `synergy_get_darwinian_trend` | 達爾文權重歷史趨勢 |
| `synergy_get_l2_4_schedule` | L2.4 觀察窗口時程 |

### Control（4 個，皆 read-only）
| Tool | 用途 |
|------|------|
| `control_get_audit_log` | 控制覆寫 audit log |
| `control_get_active_overrides` | 當前 active 覆寫 |
| `control_approve_recommendation` | 批准推薦狀態 |
| `control_reject_recommendation` | 拒絕推薦狀態 |

### Scheduler / Task（4 個）
| Tool | 用途 |
|------|------|
| `scheduler_get_status` | 背景排程器狀態 |
| `task_list` | 背景任務清單 |
| `task_get` | 單筆任務 |
| `task_get_events` | 任務 lifecycle events |

### System（7 個：1 Phase 1 + 6 Phase 2.2）
| Tool | 用途 |
|------|------|
| `system_get_health` | 系統健康總覽（Phase 1）|
| `system_get_metrics` | 即時 metrics |
| `system_get_metrics_trend` | Metrics 趨勢 |
| `system_get_thresholds` | SLO 門檻值 |
| `system_get_data_pipeline` | Data pipeline 狀態 |
| `system_get_circuit_breaker` | Circuit-breaker 狀態 |
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
| `stock_get_fundamentals` | 個股基本面（PE、PB、EPS、殖利率等） |
| `stock_get_chips` | 個股籌碼面（法人/外資/投信買賣超，可選日期） |
| `stock_get_technical` | 個股技術面（收盤價、均線、RSI，預設 90 天、上限 365 天） |

> **API Contract**：[`docs/specs/stock-api-contract.md`](specs/stock-api-contract.md) 定義 4 個 `/api/stock/*` endpoint 的 typed schema（含 Symbol normalization 規則、單位、欄位）。
> **Frontend 狀態**：client_web「個股快查」頁面（Issue #1038）已 ship — 後端 normalize（PR-A #1044）+ 前端 14 檔（PR-B #1045）+ 文件同步（PR-C #1046）+ RSI pre-existing bug fix（PR #1047）。頁面路徑 `/client/quote?symbol=<4-6 digit symbol>`。剩餘 follow-up 見 `.omo/plans/2026-07-09-stock-quote-followup.md`。

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

> 安全邊界：`mcp_roots_read_file` 強制 O_RDONLY、TOCTOU 防護、size cap、AllowedRoots 檢查；詳見 [spec Phase 4 B](./specs/agent-mcp-server.md)。

### MCP Audit / Observability（6 個 — agent 自我觀測）

| Tool | 用途 |
|------|------|
| `mcp_get_session_topology` | 回傳 Agent×Tool 呼叫矩陣（哪個 agent 用了哪些 tool） |
| `mcp_get_call_stats` | Tool 呼叫頻率與延遲統計 |
| `mcp_get_tenant_usage` | Multi-tenant 用量報表 |
| `mcp_get_top_slow_tools` | 最慢的 N 個 tool（延遲排行） |
| `mcp_anomaly_get_recent` | 近期異常事件（error spike、延遲飆升） |
| `mcp_anomaly_ack` | 標記異常為已確認 |

> 這些 tool 屬於 atlas-mcp 的自我觀測層，供 agent 了解自己的呼叫模式與系統健康。

### Prism（1 個）
| Tool | 用途 |
|------|------|
| `prism_get_training_results` | PRISM cohort 訓練結果 |

---
