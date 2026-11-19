---
name: atlas-platform-guide
description: 指引 LLM agent 使用 Atlas 台股策略模擬與風控平台。當用戶提及「atlas」、「atlas 平台」、「策略模擬」、「台股模擬」、「MCP 工具」、「七維錢潮」、「壓力指數」、「Darwinian」、「PRISM」、「L1-L5 訊號」、「sector allocation」、「backtest」、「universe session」、「risk exposure」等與 Atlas 平台功能相關的詞彙時自動載入。協助 agent 正確查閱知識庫、依序呼叫 MCP 工具、組裝晨報/風險檢查/策略分析等工作流，並避免常見錯誤。
---

# Atlas 平台使用指南技能

> 本技能為 LLM agent 操作 Atlas（亦稱 atlas）台股策略模擬平台的入口指引。技能本體**不重複**平台文件內容，所有權威定義指向 `~/workspace/atlas-wiki/atlas-platform/` 下的知識庫文件。

## 1. 技能定位

Atlas 平台透過 MCP（Model Context Protocol）介面把市場數據、策略訊號、風險指標、模擬結果、控制能力對外開放。本技能教導 agent：

- **何時該載入**：遇到平台相關提問或任務時。
- **先讀哪份知識**：依任務類型選讀對應文件而非盲目呼叫工具。
- **如何組合 MCP 工具**：基本調用順序、one-shot endpoint、容錯處理。
- **常見任務的標準工作流**：晨報、風險檢查、策略分析、產業查詢。

## 2. 觸發條件（Triggers）

下列任一情境出現時，**必須**載入本技能並依下文工作流執行：

### 2.1 平台名詞提問
- 「atlas 是什麼？」「atlas 平台能做什麼？」「atlas 跟其他平台差在哪？」
- 「L1-L5 是什麼？」「Darwinian 權重是什麼？」「PRISM cohort 訓練？」

### 2.2 MCP 工具呼叫相關
- 用戶要求「用 atlas 工具查」、「呼叫 atlas MCP」、「跑一次策略模擬」
- 任務涉及 `atlas-mcp_*` 工具命名空間（macro_*, capital_flow_*, strategy_*, risk_*, control_*, report_*, experiment_* 等）

### 2.3 市場與策略分析
- 提及「七維錢潮雷達」「錢潮」「三大法人」「土洋對作」→ 涉及 `capital_flow_*`
- 提及「壓力指數」「TRJ」「regime」「risk-on/risk-off」→ 涉及 `*stress_index*`、`regime_*`
- 提及「晨報」「開盤前報告」「每日報告」→ 涉及 `daily_report`、`report_get_*`
- 提及「sector allocation」「產業配置」「rebalance」→ 涉及 `sector_allocation_plan`、`get_recommendations`
- 提及「backtest」「回測」「夏普比率」「最大回撤」→ 涉及 `backtest_*`、`experiment_*`、`prism_*`

### 2.4 風險與控制（敏感操作）
- 提及「sector ban」「暫停 agent」「approve/reject recommendation」→ 涉及 `control_*`（需 `ATLAS_API_KEY`）
- 提及「實驗 promote」「實驗 revert」「experiment judge」→ 涉及 `experiment_*`（需授權）

## 3. 必讀知識庫文件對照

> **權威來源**：`~/workspace/atlas-wiki/`（即 `~/workspace/atlas-wiki/`）
> 列出位置以 `~/workspace/atlas-wiki/` 為準，跨機器可移植。

| 任務 | 必讀文件 | 輔助文件 |
|------|----------|----------|
| 了解 Atlas 整體架構與工具分類 | `atlas-platform/platform-overview.md` | `INDEX.md` |
| 呼叫 MCP 工具（清單、調用順序、錯誤處理） | `atlas-platform/mcp-integration.md` | `atlas-platform/platform-overview.md` |
| 解讀策略模擬結果（PRISM、L1-L5、Darwinian） | `atlas-platform/simulation-guide.md` | `investment-strategies/strategy-taxonomy.md` |
| 查詢金融術語定義 | `domain-model.md` | — |
| 跨領域分析（需要先理解市場） | `taiwan-stock-market/*.md` | `domain-model.md` |

**讀取順序原則**：
1. 永遠先 `cat ~/workspace/atlas-wiki/index.md` 確認知識庫地圖。
2. 遇到不明術語（regime、stress index、VaR、Cohort 等）→ 查 `domain-model.md`。
3. 任務屬於「平台能做什麼」→ 讀 `atlas-platform/platform-overview.md`。
4. 任務屬於「怎麼用工具」→ 讀 `atlas-platform/mcp-integration.md`。
5. 任務屬於「解讀結果語義」→ 讀 `atlas-platform/simulation-guide.md`。

## 4. MCP 工具基本調用紀律

### 4.1 讀類 vs 寫類

| 類型 | 工具前綴 | 授權 |
|------|----------|------|
| 唯讀 | `mcp_*`、`macro_*`、`capital_flow_*`、`strategy_*`、`risk_*`、`report_*`、`narrative_*`、`backtest_*`（唯讀） | 無需授權 |
| 觀測 | `alert_*`、`strategy_*_summary`、`*_status` | 無需授權 |
| 變更 | `control_*`、`experiment_promote/revert`、`alert_acknowledge/resolve`、`parameters_*`（修改類） | 需 `ATLAS_API_KEY` |

> **底線**：沒有明確授權時，**絕不**自動呼叫寫入類工具。應先提示用戶並要求確認。

### 4.2 共通調用順序

1. **先 one-shot**：能一次拿的不要分多次。優先：
   - `mcp_quickstart` — 宏觀 + 資金 + regime + 事件四合一
   - `daily_report` — 完整每日報告 JSON
   - `capital_flow_summary` — 簡明版七維（僅官方三維）
2. **後細節**：若 one-shot 不足以回答，再呼叫單點工具（`macro_*`、`capital_flow_daily`）。
3. **最後個股**：產業 → 個股的鑽取路徑為 `industry_sector_list` → `industry_sector_lookup` → `stock_get_*`。
4. **最後驗證**：報告引用前先 `report_get_*` 確認數據口徑。

### 4.3 工具失敗處理

- **MCP 工具回傳 `isError: true`**：將 `error` 內容原文回報用戶，**不要**自行猜測補值。
- **HTTP 401/403**：確認是否需要 `ATLAS_API_KEY` 或 `FUGLE_API_KEY`；提示用戶設定環境變數。
- **HTTP 5xx / 連線失敗**：建議稍後重試；若是 `data_get_channels` 顯示某 channel 異常，回報哪些資料源受阻。
- **資料延遲**：`macro_get_snapshot_latest` 有 5 分鐘 on-disk cache；如需即時，呼叫 `stock_get_quote`（需 FUGLE_API_KEY）。
- **404 / NotFound**：工具可能在新版本被移除；查 `{[[concepts/atals-mcp-tools-reference]]}` 是否已更新。

## 5. 常見任務工作流

### 5.1 晨報生成（Daily Morning Briefing）

```
1. daily_report                          ← 取得完整 JSON 報告
2. explain_market_move (format=plain)    ← 取得繁中平實解說
3. event_calendar (未來 14 天)           ← 預告近期事件
4. event_flow_prediction                 ← 5 日事件驅動資金流向預測
5. risk_get_metrics                      ← 當前風險指標
6. （選擇性）mcp_anomaly_get_recent       ← 觀察異常事件
```

**輸出**：以「全球資金總開關 → 台股七維摘要 → 重要事件 → 策略觀點 → 風險提示」五段式結構。

### 5.2 風險檢查（Risk Check）

```
1. risk_get_metrics                      ← 聚合風險指標
2. risk_get_drawdown                     ← 當前回撤與峰值回撤
3. risk_get_correlation_matrix           ← 策略相關性矩陣
4. （嚴重時）risk_get_calibration        ← VaR 預測 vs 實際校準
5. risk_get_commentary                   ← 自動風險敘事
6. （告警）alert_list_unacknowledged      ← 未確認告警
```

**判定**：regime = RISK_OFF 或 VaR 突破閾值 → 啟動 `control_*` 評估（仍需用戶授權）。

### 5.3 個股深度分析（Stock Drill-down）

```
1. industry_sector_lookup (symbol=2330)  ← 確認產業歸屬
2. stock_get_quote / _fundamentals / _technical
3. stock_get_chips                       ← 三大法人買賣超
4. （跨市場）crossmarket_get_correlation ← 該產業 vs 美指相關性
5. trace_get_decision_chain (symbol=...) ← 取得決策因果鏈
```

### 5.4 策略模擬結果解讀

```
1. universe_get_sessions                 ← 最近模擬 sessions
2. universe_get_session_detail (session_id=...)
3. prism_get_training_results            ← PRISM cohort 訓練數據
4. backtest_signals                      ← 當前 backtest 訊號
5. experiment_history                    ← 實驗歷史（含 judge 結果）
6. experiment_diff (experiment_id=...)   ← 與 baseline diff
```

**重點**：判定策略是否通過統計驗證（Welch t-test、Sharpe stability、OOS validation）才建議 promote。

### 5.5 產業與配置檢視

```
1. industry_sector_list                  ← 20 產業全景
2. sector_allocation_plan                ← 模擬層最新配置
3. strategy_get_layers                   ← L1-L5 訊號偵測器啟用狀態
4. synergy_get_darwinian_status          ← Darwinian 權重動態
5. parameters_get_categories             ← 參數類別盤點
```

### 5.6 異常與敘事除錯

```
1. narrative_get_events                  ← 最新敘事事件
2. narrative_get_chains                  ← 因果鏈
3. narrative_get_bundle                  ← 編譯後 briefing bundle
4. narrative_get_templates               ← 偵測器模板
5. template_detector_status              ← 最新掃描結果
```

## 6. 注意事項與限制

### 6.1 職責邊界

- **平台不下單**：所有「建議」皆為模擬層輸出，**不可**作為實際買賣依據。回應用戶時必須明確這一點。
- **資料延遲**：on-disk cache 5 分鐘、即時報價需 `FUGLE_API_KEY`；不可聲稱「即時」數據除非已驗證。
- **PRISM 與 backtest 為歷史**：統計顯著不等於未來報酬；不可對未來表現做任何承諾式預測。

### 6.2 安全與帳號

- **絕不**在輸出中含有 `ATLAS_API_KEY`、`FUGLE_API_KEY` 等真實金鑰。
- **絕不**未經用戶確認就呼叫 `control_*`、experiment `promote/revert`、`alert_acknowledge/resolve` 等寫入操作。
- **審計日誌**：所有寫入類操作都有 audit log；如有疑慮，事前讀 `control_get_audit_log` 看規範。

### 6.3 知識庫更新

- 知識庫位置：`~/workspace/atlas-wiki/`
- 維護規範：見 `~/workspace/atlas-wiki/CONTRIBUTING.md`
- 若發現 `mcp-integration.md` 與實際工具表對不上，**優先**以實際工具清單為準並回報用戶更新知識庫。

### 6.4 不可為之事

- **不可**給出明確「買 XX 個股」「賣 XX 個股」等具體指示。能力所及是「彙整資訊、呈現平台訊號、提示風險」，決策屬於用戶。
- **不可**將 `simulation_guide` 中以「反推」得到的描述（如 L1-L5 細節）當作官方保證；保持術語警語。
- **不可**假設用戶已知 AI 機制（如 Darwinian 自動權重）——首次提及需簡述。

## 7. 自我檢核清單

每次完成 Atlas 相關任務，輸出前回顧：

- [ ] 是否依知識庫層級讀過對應文件（避免瞎猜工具）？
- [ ] 工具呼叫順序是否合理（one-shot → 細節 → 個股）？
- [ ] 是否標註資料來源與時間戳記（on-disk cache vs 即時）？
- [ ] 是否迴避了具體買賣建議？
- [ ] 寫入類操作是否在用戶明確同意後執行？
- [ ] 失敗時是否原文回報錯誤，而非猜測補值？
- [ ] 是否引用了 `~/workspace/atlas-wiki/atlas-platform/` 對應文件路徑供用戶追查？

## 8. 參見

- `~/workspace/atlas-wiki/index.md` — 知識庫完整地圖
- `{[[concepts/atals-platform-overview]]}` — 平台架構、功能分層
- `{[[concepts/atals-mcp-tools-reference]]}` — 工具清單、調用順序、錯誤處理
- `{[[concepts/atals-simulation-guide]]}` — 策略模擬、PRISM、Darwinian、實驗管理
- `{[[concepts/taiwan-financial-domain-model]]}` — 金融術語字典
- `~/.agents/skills/taiwan-market-analysis/SKILL.md` — 姊妹技能：台股市場知識
- `~/.agents/skills/investment-strategy-review/SKILL.md` — 姊妹技能：投資策略審視
- `~/.agents/skills/financial-data-interpretation/SKILL.md` — 姊妹技能：金融數據解讀
