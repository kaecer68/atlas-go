# AGENTS_INDEX.md — 模組 AGENTS.md 索引

> 進入 `internal/<mod>/` 工作前，先讀該目錄下的 `AGENTS.md`（或 `CONSTITUTION.md`）。模組特有陷阱寫在裡面，跳過會踩坑。
>
> **總計**：79 個模組（28 S / 33 E / 8 X / 1 A / 7 U）。保留 AGENTS.md 的 hot-path 覆蓋模組共 **15** 個（2026-07-11 從 27 合併精簡，清單見下方）。
> **v0.0.2.0 變更（2026-07-24）**：成熟度重分組——swarm X→A、replay/capitalflow/forecast/retail/strategy_ranker/stress/reporting/subscription 升至 E；sectorallocation 補加入 X；calibration 補加入 U。
> **2026-08-07 補齊**：16 個實際存在但未索引模組補入（S: constants/experiment/janus/live；E: acceptance/eventquality/marketexplain/methodology/observability/userstate；X: alerting/llm/llm_annotator/stocktools；U: backfill/buildinfo）。
>
> **與 MATURITY.md 的差異**：AGENTS_INDEX 計算頂層模組（59 個）；`internal/MATURITY.md` 計算所有 Go packages（含 sub-packages 如 `domain/shared`、`llm/clients`，約 80 個）。兩者 scope 不同，數字差異是正常的。

## 索引（按成熟度分組）

### S · Stable（穩定生產，28 個）

| 模組 | 關鍵主題 |
|------|---------|
| `apigateway` | **CONSTITUTION.md** — API Gateway、BackgroundTaskManager、架構憲法 |
| `baseline` | Policy 升降級、版本控制、A/B 測試 |
| `bootstrap` | 系統初始化、HTTP 路由註冊 |
| `config` | 環境變數、ParametersConfig（禁止硬編碼） |
| `db` | PostgreSQL 連線管理 |
| `domain` | 領域型別、string enum、JSON tag snake_case |
| `eventbus` | 事件匯流排 Publish/Subscribe |
| `industry` | 產業輪動、供給需求、季節性、週期 |
| `ledger` | JSONL append-only、OutcomeCount 計算 |
| `logging` | 統一日誌介面 |
| `marketdata` | Provider 抽象、TWSE / FinMind / Fugle |
| `monitoring` | Dashboard API、監控、人工干預入口 |
| `narrative` | 宏觀敘事、因果鏈、台灣壓力指數 |
| `orchestrator` | 三層 executor 路由、GuardOutcomes 對齊、PluginHost |
| `portprobe` | Stateless TCP port 探測 helper（`Probe`/`LookupOccupant`/`IsFubonZombie`/`KillOccupant`）— Maturity: stable |
| `repository` | PostgreSQL 持久化、DualWriteRepository |
| `risk` | RiskManager、VaR、宏觀回撤 |
| `screener` | 宣告式個股篩選 |
| `sim` | 模擬引擎、部位狀態轉換、JSONL replay |
| `spawning` | Agent 生成管理 |
| `startup` | 一次性啟動期 preflight 檢查（`Preflight(claims)`，`portprobe` 上層 consumer）— Maturity: stable |
| `storage` | 檔案儲存抽象、原子寫入 |
| `strategy_techniques` | 投資心法庫 — 5 層框架 + 4 核心指標 + 自我修正 |
| `tax` | 台灣稅務計算 |
| `constants` | 集中式預設值（跨 binaries/config 共用）|
| `experiment` | 突變生命週期 — Propose → Execute → Judge → Promote（evolution loop）|
| `janus` | JANUS meta-layer — cross-cohort regime 偵測 + PRISM 訓練動態加權 |
| `live` | **AGENTS.md** — live trading 協調、broker execution、order management、circuit breaking、state 管理 |

### E · Evolving（演進中，32 個）

| 模組 | 關鍵主題 |
|------|---------|
| `autobacktest` | 自動回測定時任務 |
| `backtest` | 視窗回測 Window.Run() |
| `charter` | **v0.0.2.0 新** — Phase C3 charter A/B 開關 + 統計（paired t-test / BCa bootstrap）+ per-arm 控制（WithCharterMode） |
| `capitalflow` | **v0.0.0.32 新** — 七維錢潮雷達（3+2+2 分層）分解 + 共振強度（`capital_flow_daily` / `capital_flow_summary` MCP tool 來源）；詳見 `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04 |
| `dailyreport` | **v0.0.0.32 新** — 每日市場報告 JSON 組裝（`daily_report` MCP tool 來源，agent morning briefing 入口） |
| `eval` | 模型評估指標、可解釋性工具（SK-12~15） |
| `eventdriven` | **v0.0.0.32 新** — 事件日曆 + 5 日事件驅動資金流預測（`event_calendar` / `event_flow_prediction` MCP tool 來源） |
| `feature` | 命名特徵萃取、Registry |
| `forecast` | 個股方向性預測 — per-symbol directional forecasts（Phase 3.5 M4 PoC）|
| `fubonproxy` | Python FastAPI 微服務生命週期管理 |
| `globalmarket` | 全球總經資料管理 |
| `macroflow` | 宏觀 regime → factor weight 調整引擎（6 rules × VIX stress，consumed by orchestrator pipeline 第 7 步） |
| `metalearning` | 元學習協調器、策略選擇優化 |
| `ml` | 監督式學習模型 OLS/ElasticNet/PCR/PLS |
| `portfolio` | Darwinian 權重、FactorEngine、FactorType 變更流程 |
| `prism` | Regime-specific 訓練佇列 |
| `realtime` | 即時資料轉接器 |
| `recommender` | **v0.0.0.32 新** — tier-gated 投資組合推薦（`get_recommendations` MCP tool 來源，需 JWT） |
| `replay` | TWSE CSV 載入與 forward return 計算（27 個 runtime 檔引用，核心資料引擎） |
| `reporting` | 報告生成 Markdown/ASCII chart（進入 monitoring dashboard API） |
| `retail` | RSI-tw 散戶情緒指數（進入 portfolio factor bridge + monitoring API） |
| `scheduler` | ML 模型重訓排程 |
| `strategy` | 策略選擇器與登錄 |
| `strategy_ranker` | **v0.0.0.32 新** — 策略表現排名引擎（`strategy_ranker` MCP tool 來源，按 tier 標 free/registered/premium） |
| `stress` | 壓力測試場景（進入 orchestrator SystemCore live risk evaluation） |
| `subscription` | **v0.0.0.32 新** — JWT tier 認證 + 使用者訂閱狀態解析（`/api/auth/*` + `/api/user/profile` 來源） |
| `acceptance` | experiment acceptance gate 框架 — 可插拔 Evaluator（judge 前閘門）|
| `eventquality` | 事件資料品質 gate — event driven 資金流輸入校驗 |
| `marketexplain` | 「為什麼漲跌」compose endpoint（`explain_market_move` MCP tool 來源）|
| `methodology` | 方法論顧問層 — market regime → methodology rules 映射（E4 七時期）|
| `observability` | OpenTelemetry tracing 初始化與 runtime observability helpers |
| `userstate` | per-user 行為狀態實體（product positioning 支撐）|
| `cmd/atlas-mcp/server` | **AGENTS.md** — MCP server（tool 數量詳見 [`docs/reference/tool-catalog.md`](docs/reference/tool-catalog.md)）、stdio/SSE/streamable-HTTP transport、auth/audit/anomaly、descgen、5 protocol extensions |

> 註：24 個 S-tier + 27 個 E-tier 中，`cmd/atlas-mcp/server` 為跨 internal/ 與 cmd/ 的特殊位置；其餘模組位於 `internal/` 下。


### X · Experimental（實驗中，8 個）

| 模組 | 關鍵主題 |
|------|---------|
| `adversarial` | 對抗性訓練、BattleResult（⚠️ 被 orchestrator runtime 使用但無 AGENTS.md） |
| `reflexivity` | 自反性價格動態引擎（⚠️ 被 orchestrator + sim runtime 使用但無 AGENTS.md）|
| `mcp/anomaly` | MCP audit event 異常偵測（Phase 4 Direction A，僅供 atlas-mcp 消費）|
| `sectorallocation` | 產業權重單一權威 — 統一三路計算（industry/portfolio/monitoring）為多因子引擎（`docs/specs/sector-allocation-simulation-closure-spec.md`）|
| `alerting` | Alertmanager webhook 接收（observability 堆疊警報）|
| `llm` | **AGENTS.md** — capability-based 多 provider routing（Router 唯一入口、DataClass gate）|
| `llm_annotator` | LLM 自然語言註解 — strategy_techniques 的 LLM 註解路徑 |
| `stocktools` | per-symbol 台股查詢端點（quote/fundamentals/chips/technical）|
| `stockpicker` | **PR 1a/1b/1c/2a** — 個股選股核心：勝率數學 + outcome/win-rate 儲存 + PIT 回測聚合 job（可設定條件引擎 `conditions.go`，參數自 `parameters.json`；`-conditions`/`-list-conditions` 選條件）；CLI 盤後執行，尚未接 runtime 排程 |

### A · Archived（封存，1 個）

| 模組 | 說明 |
|------|------|
| `swarm` | 目錄已刪除（PR #963）；模擬引擎已降級為 pass-through；保留條目供歷史參考 |

### U · Utility（輔助工具，6 個）

| 模組 | 關鍵主題 |
|------|---------|
| `importer` | CSV → JSONL 資料匯入 |
| `testdb` | PG 整合測試單一 DATABASE_URL 政策（CI fail-loud / 本地 skip，M6 2026-08-28）|
| `taskexec` | 非同步任務執行 Manager |
| `taiwanholidays` | **v0.0.2.0 新** — 台灣交易日曆單一來源（P1-8：lunar/固定假日表合併，2023-2040） |
| `liveness` | 任務活性心跳 — task_liveness upsert store、staleness monitor、cron ping 端點（Phase 1, 2026-08-17）|
| `calibration` | 參數校準純邏輯 — GARCH/VaR/Darwinian/Factor 推斷（`cmd/calibrate-parameters`）|
| `backfill` | ledger 狀態一次性修復工具（drift from canonical）|
| `reconcile` | session summary 雙寫對帳 — PG vs JSONL 對稱差 + 單邊缺口回填（`cmd/reconcile-sessions`，B6）|
| `buildinfo` | runtime metadata — version/commit hash/build time |

## 15 個保留 AGENTS.md（2026-07-11 從 27 合併精簡）

以下為所有目錄下含有 `AGENTS.md` 的位置。多個模組的陷阱已合併至集群檔案：

| 檔案位置 | 涵蓋模組 | 關鍵陷阱主題 |
|---------|---------|-------------|
| `AGENTS.md`（root） | 跨模組高頻陷阱 | 前 5 條高頻陷阱速查表；完整列表見 `docs/reference/traps.md` |
| `internal/apigateway/` | apigateway | Gateway.Fetch、BackgroundTaskManager、CircuitBreaker |
| `internal/capitalflow/` | capitalflow + eventdriven + recommender + subscription | 資金流/事件日曆/推薦/認證集群 |
| `internal/fubonproxy/` | fubonproxy | ProcessManager supervisor F1-F9、Stop/backoff |
| `internal/live/` | live | broker execution、nonce、EventPositionUpdate |
| `internal/llm/` | llm | Router唯一入口、DataClass gate、capability SOP |
| `internal/marketdata/` | marketdata | Provider抽象、DecodeJSON、fubon URL guard |
| `internal/monitoring/` | monitoring + api/shared | Dashboard API、auth whitelist、Wave 9 |
| `internal/orchestrator/` | orchestrator | Executor路由、PluginHost、ANTIPATTERNS |
| `internal/strategy_techniques/` | strategy + strategy_ranker + strategy_techniques | 策略集群（選擇/排名/心法） |
| `admin_web/` | admin_web | 行事曆組件、參考檔案 |
| `client_web/` | client_web | API field contract、shared_web fallback |
| `cmd/experimental/` | cmd/experimental | dry-run禁令、dummy mode、live broker |
| `cmd/atlas-mcp/server/` | cmd/atlas-mcp/server | tool註冊、audit、auth、transport |
| `scripts/openclaw/` | scripts/openclaw | baseline政策、閘門、dry-run |

> 共 15 個。被刪除的 19 個模組級 AGENTS.md 的陷阱已搬遷至集群檔案或 root AGENTS.md 陷阱表。

## 成熟度定義

- `stable` — 介面與行為穩定，可安全使用
- `evolving` — 活躍開發中，修改前請讀 AGENTS.md
- `experimental` — 功能未完全穩定，不應被其他模組依賴
- `utility` — CLI 工具、測試輔助或非 runtime 一部分
- `archived` — 已被 Phase 2 canonical 取代；API frozen，僅接受 bug fix；新程式碼禁止依賴

## v0.0.2.0 變更摘要（2026-07-24 成熟度重分組）

| 變更 | 模組 | 說明 |
|------|------|------|
| 升級 | `replay` | U → E（27 個 runtime 檔引用，核心資料引擎） |
| 升級 | `reporting` | U → E（進入 monitoring dashboard API） |
| 升級 | `subscription` | U → E（進入 MCP auth + recommender runtime） |
| 升級 | `capitalflow` | X → E（12 個 runtime 檔引用，Wave 11 shipped） |
| 升級 | `forecast` | X → E（Phase 3.5 M4 shipped，被 orchestrator 引用） |
| 升級 | `retail` | X → E（進入 portfolio factor bridge + monitoring API） |
| 升級 | `strategy_ranker` | X → E（Wave 11 shipped，驗證與排名邏輯合併於此包） |
| 升級 | `stress` | X → E（進入 orchestrator SystemCore live risk evaluation） |
| 封存 | `swarm` | X → A（目錄已刪除，PR #963） |
| 新增 | `sectorallocation` | 加入 X-tier（產業權重單一權威，多因子引擎） |
| 新增 | `calibration` | 加入 U-tier（`cmd/calibrate-parameters`）|

## cmd/ 索引

`cmd/` 目录下所有 binary 的分類索引，用於快速定位 CLI 工具所屬類別與依賴模組。

| 分類 | 說明 |
|------|------|
| [cmd/REGISTRY.md](cmd/REGISTRY.md) | **完整 binary 索引** — 10 大分類、Dead Code、cmd↔internal 缺口表 |

**快速參考**：生產主程序在 `Orchestrator`；實驗工具在 `Experimental`；資料回填在 `Backfill`；校準工具在 `Calibrate`；健康檢查在 `Check/Health`。

## AI 操作 workflow 索引

| 入口 | 用途 |
|------|------|
| `atlas-pre-change-protocol` | 修改任何程式碼前的 7 步強制檢查 |
| `atlas-audit-manifest-protocol` | 除錯 / 審計 / 修復：從根因調查到 manifest → commit → PR 的完整 workflow |
| `docs/manifests/TEMPLATE.md` | 審計 manifest 模板 |

## 參考

- 完整成熟度對照表：`internal/MATURITY.md`
- 跨模組陷阱詳細參考：`docs/reference/traps.md`
- 根路由與全域規則：`AGENTS.md`
- 保留 AGENTS.md 模組清單：見下方「15 個保留模組」
- v0.0.0.32 完整 release notes：`CHANGELOG.md` v0.0.0.32 區段