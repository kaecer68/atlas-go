# atlas-mcp 工具權限分級建議表（draft, C-02/phase2 input）

> 狀態: 建議 draft（2026-08-19）。由 atlas-go tool catalog 對照 _tier-matrix §1.1 產生；**以 go-member data-permission-matrix SSOT 為準**。
> tier: free / silver+（含 basic/pro 映射）/ admin（內部，不對外訪客）。

| tool | 類別 | 建議 tier | 對應 _tier-matrix 維度 |
|---|---|---|---|
| `alert_acknowledge` | Alert | admin | 內部/管理/後台 |
| `alert_get_rules` | Alert | admin | 內部/管理/後台 |
| `alert_get_stats` | Alert | admin | 內部/管理/後台 |
| `alert_list` | Alert | admin | 內部/管理/後台 |
| `alert_list_unacknowledged` | Alert | admin | 內部/管理/後台 |
| `alert_resolve` | Alert | admin | 內部/管理/後台 |
| `alert_scan` | Alert | admin | 內部/管理/後台 |
| `alert_silence` | Alert | admin | 內部/管理/後台 |
| `audit_state` | audit | admin | 內部/管理/後台 |
| `backtest_signals` | Backtest | silver+ | 進階策略/回測/持倉/獨家 |
| `backtest_status` | Backtest | silver+ | 進階策略/回測/持倉/獨家 |
| `capital_flow_daily` | Capital Flow | free | 公開觀測/免費層策略 |
| `capital_flow_summary` | Capital Flow | free | 公開觀測/免費層策略 |
| `channel_health` | channel | admin | 內部/管理/後台 |
| `control_approve_recommendation` | Control | admin | 內部/管理/後台 |
| `control_get_active_overrides` | Control | admin | 內部/管理/後台 |
| `control_get_audit_log` | Control | admin | 內部/管理/後台 |
| `control_pause_agent` | Control | admin | 內部/管理/後台 |
| `control_reject_recommendation` | Control | admin | 內部/管理/後台 |
| `control_resume_agent` | Control | admin | 內部/管理/後台 |
| `control_sector_ban` | Control | admin | 內部/管理/後台 |
| `crossmarket_get_correlation` | Crossmarket | free | 公開觀測/免費層策略 |
| `crossmarket_get_status` | Crossmarket | free | 公開觀測/免費層策略 |
| `crossmarket_get_us_indices` | Crossmarket | free | 公開觀測/免費層策略 |
| `daily_report` | daily | free | 公開觀測/免費層策略 |
| `data_get_channel_detail` | Data | admin | 內部/管理/後台 |
| `data_get_channels` | Data | admin | 內部/管理/後台 |
| `data_get_field_contract` | Data | admin | 內部/管理/後台 |
| `data_get_quality` | Data | admin | 內部/管理/後台 |
| `detector_registry_list` | detector | admin | 內部/管理/後台 |
| `event_calendar` | Events | free | 公開觀測/免費層策略 |
| `event_flow_prediction` | Events | free | 公開觀測/免費層策略 |
| `experiment_diff` | Experiment | admin | 內部/管理/後台 |
| `experiment_history` | Experiment | admin | 內部/管理/後台 |
| `experiment_judge` | Experiment | admin | 內部/管理/後台 |
| `experiment_promote` | Experiment | admin | 內部/管理/後台 |
| `experiment_revert` | Experiment | admin | 內部/管理/後台 |
| `explain_market_move` | explain | free | 公開觀測/免費層策略 |
| `get_recommendations` | get | silver+ | 進階策略/回測/持倉/獨家 |
| `industry_sector_list` | Industry | free | 公開觀測/免費層策略 |
| `industry_sector_lookup` | Industry | free | 公開觀測/免費層策略 |
| `llm_get_cost` | LLM | admin | 內部/管理/後台 |
| `llm_get_health` | LLM | admin | 內部/管理/後台 |
| `macro_get_capital_flow_latest` | Macro | free | 公開觀測/免費層策略 |
| `macro_get_ingest_status` | Macro | free | 公開觀測/免費層策略 |
| `macro_get_snapshot_history` | Macro | free | 公開觀測/免費層策略 |
| `macro_get_snapshot_latest` | Macro | free | 公開觀測/免費層策略 |
| `macro_get_stress_index_current` | Macro | free | 公開觀測/免費層策略 |
| `macro_get_stress_index_history` | Macro | free | 公開觀測/免費層策略 |
| `mcp_anomaly_ack` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_anomaly_get_recent` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_get_call_stats` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_get_session_topology` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_get_tenant_usage` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_get_top_slow_tools` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_quickstart` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_roots_list` | MCP 觀測 | admin | 內部/管理/後台 |
| `mcp_roots_read_file` | MCP 觀測 | admin | 內部/管理/後台 |
| `narrative_get_bundle` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_get_chains` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_get_events` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_get_model_inventory` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_get_models` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_get_seasonal` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_get_templates` | Narrative | free | 公開觀測/免費層策略 |
| `narrative_stress_index_thresholds` | Narrative | free | 公開觀測/免費層策略 |
| `parameters_get` | Parameters | admin | 內部/管理/後台 |
| `parameters_get_audit_log` | Parameters | admin | 內部/管理/後台 |
| `parameters_get_categories` | Parameters | admin | 內部/管理/後台 |
| `parameters_get_metadata` | Parameters | admin | 內部/管理/後台 |
| `parameters_get_snapshots` | Parameters | admin | 內部/管理/後台 |
| `regime_get_history` | Regime | free | 公開觀測/免費層策略 |
| `report_get_daily_summary` | Report | free | 公開觀測/免費層策略 |
| `report_get_export_link` | Report | silver+ | 進階策略/回測/持倉/獨家 |
| `report_get_performance` | Report | silver+ | 進階策略/回測/持倉/獨家 |
| `report_get_tax_snapshot` | Report | silver+ | 進階策略/回測/持倉/獨家 |
| `risk_exposure` | Risk | silver+ | 進階策略/回測/持倉/獨家 |
| `risk_get_calibration` | Risk | silver+ | 進階策略/回測/持倉/獨家 |
| `risk_get_commentary` | Risk | silver+ | 進階策略/回測/持倉/獨家 |
| `risk_get_correlation_matrix` | Risk | silver+ | 進階策略/回測/持倉/獨家 |
| `risk_get_drawdown` | Risk | silver+ | 進階策略/回測/持倉/獨家 |
| `risk_get_metrics` | Risk | silver+ | 進階策略/回測/持倉/獨家 |
| `scheduler_get_status` | Scheduler | admin | 內部/管理/後台 |
| `sector_allocation_plan` | sector | silver+ | 進階策略/回測/持倉/獨家 |
| `stock_get_chips` | Stock | free | 公開觀測/免費層策略 |
| `stock_get_fundamentals` | Stock | free | 公開觀測/免費層策略 |
| `stock_get_monthly_revenue` | Stock | free | 公開觀測/免費層策略 |
| `stock_get_quote` | Stock | free | 公開觀測/免費層策略 |
| `stock_get_technical` | Stock | free | 公開觀測/免費層策略 |
| `strategy_for_period` | Strategy | silver+ | 進階策略/回測/持倉/獨家 |
| `strategy_get` | Strategy | free | 公開觀測/免費層策略 |
| `strategy_get_attribution` | Strategy | silver+ | 進階策略/回測/持倉/獨家 |
| `strategy_get_layers` | Strategy | free | 公開觀測/免費層策略 |
| `strategy_get_summary` | Strategy | free | 公開觀測/免費層策略 |
| `strategy_list_active` | Strategy | free | 公開觀測/免費層策略 |
| `strategy_ranker` | Strategy | free | 公開觀測/免費層策略 |
| `synergy_get_darwinian_status` | Synergy | silver+ | 進階策略/回測/持倉/獨家 |
| `synergy_get_darwinian_trend` | Synergy | silver+ | 進階策略/回測/持倉/獨家 |
| `synergy_get_l2_4_schedule` | Synergy | silver+ | 進階策略/回測/持倉/獨家 |
| `system_get_circuit_breaker` | System | admin | 內部/管理/後台 |
| `system_get_data_pipeline` | System | admin | 內部/管理/後台 |
| `system_get_health` | System | admin | 內部/管理/後台 |
| `system_get_maturity` | System | admin | 內部/管理/後台 |
| `system_get_metrics` | System | admin | 內部/管理/後台 |
| `system_get_metrics_trend` | System | admin | 內部/管理/後台 |
| `system_get_thresholds` | System | admin | 內部/管理/後台 |
| `task_get` | Task | admin | 內部/管理/後台 |
| `task_get_events` | Task | admin | 內部/管理/後台 |
| `task_list` | Task | admin | 內部/管理/後台 |
| `template_detector_status` | template | admin | 內部/管理/後台 |
| `trace_get_agent_observatory` | Trace | silver+ | 進階策略/回測/持倉/獨家 |
| `trace_get_decision_chain` | Trace | silver+ | 進階策略/回測/持倉/獨家 |
| `trace_get_reasoning` | Trace | silver+ | 進階策略/回測/持倉/獨家 |
| `trace_get_sim_latest` | Trace | silver+ | 進階策略/回測/持倉/獨家 |
| `universe_get_session_detail` | Universe | silver+ | 進階策略/回測/持倉/獨家 |
| `universe_get_sessions` | Universe | silver+ | 進階策略/回測/持倉/獨家 |
| `universe_get_universe_overlap` | Universe | silver+ | 進階策略/回測/持倉/獨家 |
