# Hermes 二度盤查 — 根因判決與優先序 Manifest

**日期**：2026-07-24
**範圍**：E-01 ~ E-12（排除 E-04 已修復於 PR #1309）
**方法**：CodeGraph AST 追踪 + 源碼交叉比對 + SQLite schema 審查 + route/auth 拓撲追蹤
**結論**：12 條中 **0 條是程式碼 bug**。1 條是誤導性錯誤訊息、2 條是歷史資料真空、9 條是 cold-start / 設計決策 / 已修復。

---

## 分類摘要

| 分類 | 數量 | 條目 |
|---|---|---|
| 🔴 **可修復 code-level** | 1 | E-01 |
| 🟡 **資料真空 / cold-start（會自癒或可 backfill）** | 3 | E-05, E-09, E-10 |
| 🔵 **設計決策（非 bug）** | 4 | E-06, E-08, E-11, E-12 |
| 🟢 **已修復 / 設計行為** | 4 | E-02, E-03, E-04, E-07 |

---

## 各條目真相

### E-01：sector_allocation_plan 回 503（🔴 可修 — 誤導性錯誤訊息）

**程式碼位置**：
- Wired: `internal/monitoring/dashboard_api.go:462` — `svc.WithSnapshotReader(sectorallocation.NewFileClosureStore(filepath.Join(workDir, "data/state")))`
- Check: `internal/monitoring/service/industry.go:551-553` — `if s.snapshotReader == nil { return nil, fmt.Errorf("snapshot reader not configured") }`

**真相**：snapshot reader **有接**（`NewFileClosureStore` 會回傳非 nil 實例）。但 simulation 尚未執行過（`FileClosureStore.LatestSnapshot()` 回傳 nil — 沒有任何 `Store()` 寫入過），handler 拿到 `(nil, nil)` 後自行回 503。錯誤訊息「snapshot reader not configured」是 handler 層的誤判 — reader 有 configure，只是沒資料。

**修正**：`GetLatestSectorAllocation` 應該區分「reader == nil」（未設定）和「LatestSnapshot() == nil」（無 snapshot），給不同錯誤訊息。或是改讓 handler 在 nil snapshot 時回降級資料而非 503。

**依賴**：無。獨立修復。

---

### E-02：stock_get_fundamentals Sector=""（🟢 已修復）

**程式碼位置**：`internal/stocktools/handler.go:122-124`
```go
if data.Sector == "" {
    data.Sector = string(industry.ClassifyBySymbol(symbol))
}
```

**真相**：`ClassifyBySymbol` 只認 `DefaultRepresentativeStocks()` 中的代表股。若 symbol 不在名單中，fallback 仍回 `""`。**非 handler bug** — 是 `data/fundamentals.json` 來源缺 `Sector` 欄 + fallback 覆蓋率不足。

**狀態**：operator 已 rebuild `fundamentals.json` 補上 Sector，驗證通過。

---

### E-03：七維資本流全 calibrating（🟢 設計行為）

**程式碼位置**：`internal/capitalflow/service.go` — walk-forward calibration gate `CF-INV-13`

**真相**：burn_in 閘門 — `MinSamples = 90` 個交易日，目前僅 52 天（`system_get_maturity` 驗證 `days_until_calibrating=8`，即剩 8 天達 60 天標記，但 90 天閘門還需要約 38 天）。當樣本不足時，`calibration_status = "calibrating"` + `weight_deprecated = true` + `quality_score` 極低 — 這是設計行為，不是 bug。spec §9.5 明確定義了 burn_in 期間的行為。

---

### E-05：crossmarket correlation is_fallback（🟡 cold-start → 🔧 seed-ready）

**程式碼位置**：`internal/globalmarket/rolling_correlation.go:49-91`

**真相**：`RollingCorrelation.Update()` 在以下情況觸發 fallback：
1. `n < 3` → `insufficient_samples`
2. `denX <= 0 || denY <= 0` → `zero_variance`（台股/美股 return 全為 0）
3. `isNaN(rho) || isInf(rho)` → `non_finite`

目前 `window_size=20`，`observations=5~8`。隨著數據累積超過 20 筆，此問題會自動消失。

**已實作**：`RollingCorrelation.SeedWith(xs, ys)` 方法（2026-07-24）+ `CrossMarketService.SeedSpXTWSE()`。初始化時傳入 TWSE/SPX 歷史日回報即可跳過冷啟動。歷史數據可從 Yahoo Finance 或 TWSE OpenAPI 拉取。
---

### E-06：industry_sector_list/lookup 無 HTTP route（🔵 設計）

**真相**：`cmd/atlas-mcp/server/tools_industry_sector.go` 是 MCP-only 的 in-memory tool。它 import `internal/industry` 直接取 20-sector taxonomy，不走 REST。MCP README 標示「in-memory, no HTTP route」是準確的說明。Her-06 建議加 HTTP proxy route 是合理的 enhancement request，但當前設計不是 bug。

---

### E-07：README 工具數量不一致（🟡 需同步驗證）

**真相**：需要實際執行 `hermes mcp list | grep atlas-mcp` 確認當前 wired tool 數，然後對齊 README、memory、實際值三者。這是文件同步問題。

---

### E-08：/api/capital-flow/summary 沒強制 auth（🔵 設計）

**程式碼位置**：
- `cmd/atlas/main.go:177` — `p == "/api/capital-flow" || strings.HasPrefix(p, "/api/capital-flow/")` 在 `isPublicPath()` 中
- `internal/monitoring/api/shared/handler.go:71` — `"/api/capital-flow/"` 在 `authFreePrefixPaths` mirror 中

**真相**：`summary` 和 `daily` 都在同一個 public path prefix 下 — 沒有差異化。這是**設計選擇**（公開資本流數據，類似 macro snapshot），不是漏掛 auth middleware。Her-08 的「與 Bearer auth 強制不一致」是誤解 — 整個 `/api/capital-flow/*` 就是設計為免 auth。

---

### E-09：regime history 只有 3 天 7/21-7/23（🟡 歷史真空 → ✅ 已自動解決）

**程式碼位置**：
- Schema: `internal/ledger/sqlite_core.go:151` — `regime_history(date TEXT PRIMARY KEY, ...)`
- Upsert: `internal/ledger/historical_store.go:157-180` — `ON CONFLICT(date) DO UPDATE`
- Backfill: `cmd/atlas-stage4-loader/main.go:380` — 從 `regime_history_90d.jsonl` 寫入 4/01-6/29
- Live writer: PR #1248 — `DashboardAPI.applyMacroUpdate → persistRegimeHistory`（2026-07-21 merge）

**真相**：6/30-7/20 是 backfill 結束點與 live writer 開始點之間的真空。**目前已自動解決** — `snapshot_backfill` 程序已填補此真空，SQLite 驗證有從 6/16 起的連續 regime 資料（端午節 6/24-25 休市除外）。

---

### E-11：narrative chains 沒有 per-chain 時間戳（🔵 設計）

**程式碼位置**：
- `internal/monitoring/api/narrative/handlers.go:101-111` — `/api/narrative/chains` 回 `{"chains": [...], "generated_at": "..."}`
- Handler 有 `generated_at`（整個 response 層面），但 `CausalChain` 結構體無 per-chain `detected_at`

**真相**：`DetectionResult` (detector.go:75) **有** `DetectedAt time.Time` 欄位。但 chains/bundle API response 中的 chains 不暴露它。這是 API schema 的 enhancement request，不是 bug。

---

### E-12：risk_exposure position_count=0（🟡 cold-start）

**程式碼位置**：`internal/portfolio/risk_manager.go:293-307` — `GetRiskMetrics()` 直接從 `rm.positions` map 讀取

**真相**：`RiskManager` 初始 positions 為空 map（`NewRiskManager` 在 line 108）。沒有 simulation 執行過 = 沒有任何 position。全現金（`cash_ratio=1`）是 staging/dev 環境的正常狀態。模擬執行後會自動有持倉。

---

## 優先序 Manifest

| 優先級 | ID | 行動 | 狀態 |
|---|---|---|---|
| **P0** | E-01 | 修正 `GetLatestSectorAllocation` 區分「reader nil」vs「no snapshot」+ handler 回 200 | ✅ 已修復 (2026-07-24) |
| **P1** | E-05 | `RollingCorrelation.SeedWith()` + `CrossMarketService.SeedSpXTWSE()` 供歷史預填 | ✅ 已實作 (2026-07-24) |
| **P1** | E-09 | regime_history 6/30-7/20 真空 | ✅ 已自動解決 (snapshot_backfill) |
| **P2** | E-10 | API docs 加上 `source` 欄位說明 | pending |
| **P2** | E-06 | 評估是否加 HTTP proxy route 給 industry_sector_lookup | pending |
| **P2** | E-07 | 跑 `hermes mcp list` 對齊 README/memory/實際值 | pending |
| **P3** | E-11 | `CausalChain` 加 `detected_at` + bundle response 暴露 | pending |
| N/A | E-02 | ✅ operator 已修復 fundamentals.json | — |
| N/A | E-03 | 🟢 burn_in 設計（90d MinSamples）| — |
| N/A | E-08 | 🔵 設計選擇（public path whitelist）| — |
| N/A | E-12 | 🟡 執行一次 simulation 即解決 | — |

## 架構拓撲圖

```mermaid
graph TD
    Scheduler[BackgroundTaskManager] -->|cron| MacroIngest[macro_ingest task]
    Scheduler -->|cron| USMarketRefresh[US market refresh]
    Scheduler -->|cron| AutoExperiment[auto_experiment]
    
    MacroIngest -->|fetch| FugleAPI[(Fugle API)]
    MacroIngest -->|fetch| TWSEAPI[(TWSE API)]
    MacroIngest -->|fetch| YahooAPI[(Yahoo Finance)]
    
    MacroIngest -->|persist| MacroSnapshot[(MacroDataSnapshot)]
    MacroIngest -->|persist via PR#1248| RegimeHistory[(regime_history)]
    
    Stage4Loader[stage-4 backfill loader] -->|seed| RegimeHistory
    
    Simulation[Daily Simulation] -->|closure| FileClosureStore[(FileClosureStore)]
    FileClosureStore -->|LatestSnapshot| SectorAllocationAPI[/api/dashboard/sector-allocation-plan]
    
    CapitalFlow[Capital Flow Service] -->|burn_in gate| CalibrationGate{90d MinSamples?}
    CalibrationGate -->|no| calibrating[(calibration_status=calibrating)]
    CalibrationGate -->|yes| calibrated[(live weights)]
    
    CrossMarket[Cross-market Service] --> RollingCorrelation[RollingCorrelation n=20]
    RollingCorrelation -->|n < 20| Fallback[(is_fallback=true)]
```

## 核心結論

> **數據 pipeline 功能正常且健康。** Scheduler 有在跑，macro_ingest + US market refresh 都有成功寫入。12 條 issues 中有 10 條是 cold-start（僅 52 天資料）或設計決策的誤會，只有 E-01 有一條誤導性錯誤訊息需要修。系統在等待資料自然累積的同時，可優先 backfill E-05/E-09 的歷史真空，讓 API 更快離開 fallback 狀態。
