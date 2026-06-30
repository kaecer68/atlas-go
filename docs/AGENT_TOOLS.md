# Atlas Agent Tools — 實戰指南

> **本文件**：給 AI agent 看的「何時該呼叫哪個 tool」決策表 + 完整 catalog（Phase 2.2: 74 tools）。
> **完整 schema / 安全 / 部署**：[`specs/agent-mcp-server.md`](./specs/agent-mcp-server.md)
> **底層 workflow 對應**：[`WORKFLOW_MAP.md`](./WORKFLOW_MAP.md)
> **Phase 1 stdio vs Phase 2 SSE/HTTP**：見 [README.md](../specs/agent-mcp-server.md) §3。

---

## 5 分鐘決策樹

```
你要做什麼？
├── 回答「現在市場怎樣？」
│   ├── 整體 regime  → regime_get_history
│   ├── 跨市場狀態  → crossmarket_get_status
│   └── 個股敘事    → narrative_get_events
│
├── 查 portfolio 狀態
│   ├── 風險指標    → risk_get_metrics
│   ├── Drawdown    → risk_get_drawdown
│   └── 持倉健康    → strategy_list_active
│
├── 觸發動作（需 audit）
│   ├── 跑回測      → task_list → task_get_events
│   ├── 評分策略     → experiment_judge
│   ├── 凍結/解凍   → control_* (read-only, audit-only)
│   └── 警報確認     → alert_list
│
└── 監控系統健康
    ├── LLM router  → llm_get_health
    ├── 整體        → system_get_health
    ├── 資料品質    → data_get_quality
    └── 警報計數    → alert_get_stats
```

---

## 完整工具 Catalog（74 tools，Phase 2.2 全部上線）

### Macro（6 個）
| Tool | 用途 |
|------|------|
| `macro_get_snapshot_latest` | 最新 macro snapshot |
| `macro_get_snapshot_history` | Macro snapshot 歷史（days 參數）|
| `macro_get_stress_index_current` | 當前 stress index |
| `macro_get_stress_index_history` | Stress index 歷史 |
| `macro_get_capital_flow_latest` | 外資/法人/散戶資金流 snapshot |
| `macro_get_ingest_status` | 通道 ingest 狀態 |

### Crossmarket（3 個）
| Tool | 用途 |
|------|------|
| `crossmarket_get_status` | 跨市場資料源狀態 |
| `crossmarket_get_correlation` | 台股 sector vs US indices 相關性 |
| `crossmarket_get_us_indices` | S&P 500 / NASDAQ / Dow Jones snapshot |

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

### Strategy（5 個：1 Phase 1 + 4 Phase 2.2）
| Tool | 用途 |
|------|------|
| `strategy_list_active` | Production 線上策略（Phase 1）|
| `strategy_get_layers` | L1-L5 層級清單 |
| `strategy_get` | 單筆策略 |
| `strategy_get_attribution` | 績效歸因 |
| `strategy_get_summary` | 策略摘要 |

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

### System（6 個：1 Phase 1 + 5 Phase 2.2）
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

### Prism（1 個）
| Tool | 用途 |
|------|------|
| `prism_get_training_results` | PRISM cohort 訓練結果 |

### Swarm（5 個）
| Tool | 用途 |
|------|------|
| `swarm_get_status` | MiroFish swarm 狀態 |
| `swarm_get_consensus` | Fish 多數決共識 |
| `swarm_get_anomalies` | 異常偵測 |
| `swarm_get_scenarios` | 監控中 scenario |
| `swarm_get_strategies` | Swarm 策略組合 |

---

## 排除的 admin / destructive 端點（per spec §3.2）

以下端點**不暴露給 MCP**（需要 admin 權限或屬於 destructive 操作）：

| 類別 | 排除清單 |
|------|---------|
| Admin 配置 | `/admin/reload-config`、`/api/admin/calibrate-thresholds`、`/api/dashboard/api-keys/update` |
| Control mutations | `/api/control/sector-ban`、`/api/control/set-model-weight`、`/api/control/pause-agent`、`/api/control/resume-agent` |
| Experiment mutations | `/api/experiment/promote`、`/api/experiment/revert` |
| Alert mutations | `/api/alerts/acknowledge`、`/api/alerts/acknowledge-bulk`、`/api/alerts/resolve`、`/api/alerts/silence` |
| Synergy mutations | `/api/synergy/l2-4-schedule/{start,stop,reset,update}` |
| Scheduler mutations | `/api/scheduler/toggle` |
| Strategy mutations | `/api/strategies/{id}/annotate` |
| Task mutations | `/api/tasks`（POST）、`/api/tasks/{id}/{cancel,retry,confirm}` |

這些操作仍可透過 `/admin/` 管理後台或 HTTP 呼叫觸發（需通過 apigateway 認證）。

---

## 對應的底層 Workflow

每個 tool 都對應 atlas-go 一條 workflow（WA-XXX），見 [`WORKFLOW_MAP.md`](./WORKFLOW_MAP.md) §3。
例如：
- `regime_get_history` → WA-200（體制判定）
- `strategy_get_summary` → WA-500（策略演化）
- `risk_get_metrics` → WA-400..403（風險閘門）
- `trace_get_reasoning` → WA-302（推理 trace）

完整對照表見 [`specs/agent-mcp-server.md`](./specs/agent-mcp-server.md) §3.1。
