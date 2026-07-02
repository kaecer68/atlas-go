# AGENTS_INDEX.md — 模組 AGENTS.md 索引

> 進入 `internal/<mod>/` 工作前，先讀該目錄下的 `AGENTS.md`（或 `CONSTITUTION.md`）。模組特有陷阱寫在裡面，跳過會踩坑。

## 索引（按成熟度分組）

### S · Stable（穩定生產）

| 模組 | 關鍵主題 |
|------|---------|
| `apigateway` | **CONSTITUTION.md** — API Gateway、BackgroundTaskManager、架構憲法 |
| `baseline` | Policy 升降級、版本控制、A/B 測試 |
| `bootstrap` | 系統初始化、HTTP 路由註冊 |
| `config` | 環境變數、ParametersConfig（禁止硬編碼） |
| `db` | PostgreSQL 連線管理 |
| `domain` | 領域型別、string enum、JSON tag snake_case |
| `eventbus` | 事件匯流排 Publish/Subscribe |
| `strategy_techniques` | 投資心法庫 — 5 層框架 + 4 核心指標 + 自我修正 |
| `industry` | 產業輪動、供給需求、季節性、週期 |
| `ledger` | JSONL append-only、OutcomeCount 計算 |
| `logging` | 統一日誌介面 |
| `marketdata` | Provider 抽象、TWSE / FinMind / Fugle |
| `monitoring` | Dashboard API、監控、人工干預入口 |
| `narrative` | 宏觀敘事、因果鏈、台灣壓力指數 |
| `orchestrator` | 三層 executor 路由、GuardOutcomes 對齊、PluginHost |
| `repository` | PostgreSQL 持久化、DualWriteRepository |
| `risk` | RiskManager、VaR、宏觀回撤 |
| `screener` | 宣告式個股篩選 |
| `sim` | 模擬引擎、部位狀態轉換、JSONL replay |
| `spawning` | Agent 生成管理 |
| `storage` | 檔案儲存抽象、原子寫入 |
| `tax` | 台灣稅務計算 |

### E · Evolving（演進中）

| 模組 | 關鍵主題 |
|------|---------|
| `autobacktest` | 自動回測定時任務 |
| `backtest` | 視窗回測 Window.Run() |
| `eval` | 模型評估指標、可解釋性工具（SK-12~15） |
| `feature` | 命名特徵萃取、Registry |
| `fubonproxy` | Python FastAPI 微服務生命週期管理 |
| `globalmarket` | 全球總經資料管理 |
| `macroflow` | 宏觀 regime → factor weight 調整引擎（6 rules × VIX stress，consumed by orchestrator pipeline 第 7 步） |
| `metalearning` | 元學習協調器、策略選擇優化 |
| `ml` | 監督式學習模型 OLS/ElasticNet/PCR/PLS |
| `portfolio` | Darwinian 權重、FactorEngine、FactorType 變更流程 |
| `portprobe` | Stateless TCP port 探測 helper（`Probe`/`LookupOccupant`/`IsFubonZombie`/`KillOccupant`）— S-tier（Maturity: stable） |
| `prism` | Regime-specific 訓練佇列 |
| `realtime` | 即時資料轉接器 |
| `scheduler` | ML 模型重訓排程 |
| `startup` | 一次性啟動期 preflight 檢查（`Preflight(claims)`，`portprobe` 上層 consumer）— S-tier（Maturity: stable） |
| `strategy` | 策略選擇器與登錄 |
| `cmd/atlas-mcp/server` | **AGENTS.md** — MCP server（84 tools、stdio transport、auth/audit/anomaly、descgen）。SSE transport 與 binary merge 仍開放（見 roadmap P1/P2 殘留） |

### X · Experimental（實驗中）

| 模組 | 關鍵主題 |
|------|---------|
| `adversarial` | 對抗性訓練、BattleResult |
| `reflexivity` | 自反性價格動態引擎 |
| `retail` | RSI-tw 散戶情緒指數 |
| `robustness` | 穩健性與敏感度測試（SK-20~22） |
| `stress` | 壓力測試場景 |
| `swarm` | MiroFish swarm 模擬 + GARCH 波動率 |
| `mcp/anomaly` | MCP audit event 異常偵測（Phase 4 Direction A）|

### U · Utility（輔助工具）

| 模組 | 關鍵主題 |
|------|---------|
| `importer` | CSV → JSONL 資料匯入 |
| `replay` | TWSE CSV 載入與 forward return 計算 |
| `reporting` | 報告生成 Markdown/ASCII chart |
| `taskexec` | 非同步任務執行 Manager |

## 成熟度定義

- `stable` — 介面與行為穩定，可安全使用
- `evolving` — 活躍開發中，修改前請讀 AGENTS.md
- `experimental` — 功能未完全穩定，不應被其他模組依賴
- `utility` — CLI 工具或輔助函式，非 runtime 一部分

## 參考

- 完整成熟度對照表：`internal/MATURITY.md`
- 跨模組陷阱詳細參考：`docs/TRAPS.md`
- 根路由與全域規則：`AGENTS.md`
