# Live Mode 對 7 個新 US Market Channel 整合狀態稽核報告

**分支**: `docs/live-mode-audit`
**日期**: 2026-06-08
**稽核範圍**: `internal/live/` 模組是否需要整合 PR #416 引入的 7 個新 US market channel
**對標基準**: PR #416（`b0f04a8c`）在 `internal/marketdata/macro_provider.go` 與 `internal/apigateway/` 加入 7 個新 channel：TSMADR / SPXIndex / NDXIndex / DJIIndex / NVDA / AAPL / MSFT（apigateway channel ID 為 `us_tsm` / `us_spx` / `us_ndx` / `us_dji` / `us_nvda` / `us_aapl` / `us_msft`）
**Worktree**: `/Users/kaecer/workspace/atlas-area5-audit`

---

## 一、稽核結論

| 檢查項目 | 結論 | 證據 |
|---------|------|------|
| Live mode 是否呼叫 `MacroDataProvider.FetchSnapshot` | ❌ **不呼叫** | `internal/live/` 全目錄零匹配（見 §二） |
| Live mode 是否讀取 `MacroDataSnapshot` 結構 | ❌ **不讀取** | `internal/live/` 全目錄零匹配 |
| 7 個新 channel 對 live trading 邏輯是否有副作用 | ❌ **零影響** | `internal/live/` 對 `us_spx/us_ndx/us_dji/us_nvda/us_aapl/us_msft/us_tsm` 零匹配（見 §三） |
| Live mode 的「macro data」從何而來 | **無 macro 概念**，僅個股報價 | `internal/live/orchestrator.go:461` 唯一資料呼叫為 `o.marketData.GetQuotes(ctx, time.Now(), o.watchlist)`（見 §四） |
| 是否需要 follow-up issue | ✅ **建議建立**，追蹤中長期整合邊界 | 見 §六、§七 |

**一句話結論**: PR #416 對 live mode **零影響**。Live mode 與 macro pipeline 架構上完全分離，無需為此次 PR 補任何 live 整合。建議建立 follow-up issue 紀錄整合邊界，但**不立即實作**。

---

## 二、`internal/live/` 對 macro 介面零匹配

### 2.1 驗證指令與結果

```bash
$ grep -rn "MacroDataProvider\|FetchSnapshot" internal/live/
（無輸出）

$ grep -rn "MacroDataSnapshot" internal/live/
（無輸出）

$ grep -rni "macro" internal/live/
（無輸出）
```

**結論**: `internal/live/` 整個模組未引入 `MacroDataProvider` 介面、未呼叫 `FetchSnapshot`、未持有 `MacroDataSnapshot` 結構，未出現「macro」相關符號。

### 2.2 `internal/live/orchestrator.go` 結構檢視

`Orchestrator` 持有的依賴欄位（`internal/live/orchestrator.go:19` 一帶）：

| 欄位 | 型別 | 用途 |
|------|------|------|
| `marketData` | `marketdata.Provider` | **個股報價介面**（`GetQuotes`），非 macro |
| `stateStore` | `*livestore.StateStore` | 狀態儲存 |
| `eventBus` | `*ChannelEventBus` | 事件匯流排 |
| `broker` | `Broker` | 券商乾跑/真實下單 |
| `orderManager` | `OrderManager` | 訂單管理 |
| `riskGate` | `RiskGate` | 風控 |
| `circuitBreaker` | `CircuitBreaker` | 斷路器 |

與 `internal/live/AGENTS.md` 列出的整合元件清單（`StateStore / EventBus / Broker / OrderManager / CircuitBreaker`）**完全一致**，未列也未引用 `MacroDataProvider`。

---

## 三、7 個新 US Channel 對 Live Mode 零匹配

### 3.1 驗證指令

```bash
$ grep -rn "us_spx\|us_ndx\|us_dji\|us_nvda\|us_aapl\|us_msft\|us_tsm" internal/live/
（無輸出）

$ grep -rn "TSMADR\|SPXIndex\|NDXIndex\|DJIIndex\|NVDA\|AAPL\|MSFT" internal/live/
（無輸出）
```

**結論**: 7 個新欄位名稱（`us_spx` 等 apigateway channel ID）與 macro 結構欄位名（`TSMADR` / `SPXIndex` 等）在 `internal/live/` 全部零匹配。

### 3.2 對照：7 個新 channel 確實已整合到下游 consumer

| 模組 | 位置 | 整合狀態 |
|------|------|---------|
| `internal/marketdata/macro_provider.go:45-51` | `MacroDataSnapshot` struct 欄位 | ✅ 已加入 7 個新欄位 |
| `internal/apigateway/limits.go:96-101` | apigateway channel 註冊 | ✅ 已加入 7 個新 channel |
| `internal/apigateway/gateway.go:183-188` | `channelIDs()` 枚舉 | ✅ 已加入 |
| `internal/narrative/ingestor.go:61` | 敘事引擎攝取 | ✅ 使用 macro snapshot |
| `internal/industry/silicon_data_aggregator.go:44` | 半導體資料聚合 | ✅ 使用 macro snapshot |
| `internal/monitoring/dashboard_api.go:292,331,383` | 監控儀表板 | ✅ 經 gateway 取得 |
| `internal/monitoring/api/decision/handlers.go:155` | decision API | ✅ 經 gateway |
| `internal/monitoring/service/narrative.go:48` | narrative service | ✅ 經 gateway |
| `internal/monitoring/service/crossmarket.go:89,137` | 跨市場 service | ✅ 經 gateway |
| `internal/orchestrator/system.go:192-193,353,634,1556,1591` | SimulationCore | ✅ 維護 macroSnapshot |

**觀察**: 7 個新 channel 已被所有監控、敘事、產業 API 消費。**唯一未消費的模組 = `internal/live/`**。

---

## 四、Live Mode 的市場資料來源：純個股報價

### 4.1 唯一資料呼叫路徑

`internal/live/orchestrator.go:461`：

```go
quotes, err := o.marketData.GetQuotes(ctx, time.Now(), o.watchlist)
```

這是 `internal/live/` 中**唯一**的市場資料呼叫。

- 介面：`marketdata.Provider.GetQuotes(ctx, timestamp, symbols []string) (QuoteList, error)`
- 範圍：僅 `o.watchlist`（監控清單）內的個股
- **不**包含 macro 指標、regime 訊號、敘事事件、指數/ADR 報價

### 4.2 Live mode 設計哲學

`internal/live/doc.go` 與 `internal/live/AGENTS.md` 揭示的設計原則：

1. **預設 broker mode = `dry-run`**：本地與 CI 不下真實單。
2. **Live 模式需顯式旗標 `-allow-live-broker`**：AGENTS.md line 75 跨模組陷阱明文：「本地測試切勿啟用 -allow-live-broker」。
3. **模組標記「混合基礎設施與業務邏輯」結構債務**（P3 重構目標，見 `doc.go`）— 意思是 live 模組刻意維持精簡，避免疊加複雜度。
4. `.github/instructions/live-trading.guardrails.instructions.md:14-15` 明確指出：
   - 「將 replay 與 simulation 路徑視為可靠預設」
   - 「除非任務明確要補齊，否則假設 internal/live 仍有部分整合 TODO」

**綜合判斷**: live mode 故意採用「精簡依賴、清單驅動、零外部訊號」的設計哲學，刻意不耦合 macro/narrative/regime 等複雜訊號源。這是**架構決策**，不是疏漏。

---

## 五、API mode（SimulationCore）對比：macro pipeline 的完整整合

`internal/orchestrator/system.go`（即 PR #416 主要 consumer）使用 macro 的方式：

| 行號 | 用途 |
|------|------|
| `:123` | `System` struct 欄位 `macroSnapshot *marketdata.MacroDataSnapshot` |
| `:192-193` | `NewSystemWithEventBus` 構造時建立空 snapshot 傳入 `buildFactorEngine` |
| `:353` | `RunDailySimulation` 執行 `*s.macroSnapshot = QuotesToMacroDataSnapshot(quotes)` |
| `:634` | `runReplaySimulation` 同上 |
| `:1556` | `QuotesToMacroDataSnapshot` 函式：從個股 quotes 推導 macro 欄位 |
| `:1591` | `assessStructuralTrends` 使用 `sectorDataProvider.FetchSnapshot(ctx)` |

**API mode 與 live mode 的關鍵差異**:

- API mode 是**批次/回放**路徑（daily simulation / replay），每次 tick 都重新計算 macro 快照、用於 factor engine 與結構性趨勢評估。
- Live mode 是**即時單筆**路徑，設計上**不**做 macro 快照、不維護 factor engine、不評估結構性趨勢。

兩者本質上是**不同運行模式**，共享市場資料介面（`marketdata.Provider`），但不共享 macro pipeline。

---

## 六、Follow-up 建議

### 6.1 短期（本次 PR 不處理）

- ✅ **不需任何程式碼修改**。本次 PR #416 已完成 API mode 整合，live mode 因架構分離無需補齊。

### 6.2 中長期（建議建立 follow-up issue）

**Issue 主題**: `Live mode 與 macro pipeline 整合邊界設計`

**討論議題**:

1. **若 live mode 要支援 regime 偵測驅動下單**：需要建立 `MacroDataProvider` 到 `live.Orchestrator` 的注入路徑，與 `assessStructuralTrends` 對接。
2. **若 live mode 要支援敘事事件觸發暫停**：需要從 `internal/eventbus/` 訂閱 `internal/narrative` 的事件。
3. **若要讓 live mode 風控納入 VIX / 信用利差**：需要 macro snapshot 在 live 路徑上的即時版本。
4. **模組成熟度**：`internal/live/doc.go` 已標記為「混合基礎設施與業務邏輯」P3 重構目標，macro 整合應併入該重構，而非單獨 PR。

### 6.3 不建議的選項

- ❌ **為本次 PR 補 live 整合**：違背架構分離設計，且 `internal/live/AGENTS.md` 明文「live mode 需謹慎旗標」，任何 live 路徑改動都需獨立 PR + 人工審查。
- ❌ **直接複用 `internal/orchestrator/system.go` 的 macroSnapshot 模式**：會把 `factorEngine` / `assessStructuralTrends` 等複雜依賴帶進 live 模組，違背精簡設計。
- ❌ **在 live mode 加 `tsm_adr` 報價進入 watchlist**：這是監控清單變更，與 macro 整合無關，應另案討論。

---

## 七、驗證指令複現

```bash
# 1. 切換到 audit worktree
cd /Users/kaecer/workspace/atlas-area5-audit

# 2. 驗證 internal/live/ 對 macro 介面零匹配
grep -rn "MacroDataProvider\|FetchSnapshot" internal/live/
grep -rn "MacroDataSnapshot" internal/live/
grep -rni "macro" internal/live/

# 3. 驗證 internal/live/ 對 7 個新 channel 零匹配
grep -rn "us_spx\|us_ndx\|us_dji\|us_nvda\|us_aapl\|us_msft\|us_tsm" internal/live/
grep -rn "TSMADR\|SPXIndex\|NDXIndex\|DJIIndex\|NVDA\|AAPL\|MSFT" internal/live/

# 4. 對照 API mode 已整合
grep -rn "macroSnapshot" internal/orchestrator/system.go
grep -n "us_spx\|us_ndx\|us_dji\|us_nvda\|us_aapl\|us_msft\|us_tsm" internal/apigateway/limits.go

# 5. 確認唯一資料來源
sed -n '461p' internal/live/orchestrator.go
# 預期: quotes, err := o.marketData.GetQuotes(ctx, time.Now(), o.watchlist)
```

---

## 八、參考資料

- **PR #416**: `b0f04a8c feat(apigateway): wire 7 new US market channels (SPX/NDX/DJI/NVDA/AAPL/MSFT/TSM)`
- **根 AGENTS.md**: line 75「Live 旗標 | 本地測試切勿啟用 -allow-live-broker」
- **`.github/instructions/live-trading.guardrails.instructions.md`**: 第 14-15 行，live 模式預設策略
- **`internal/live/AGENTS.md`**: live 模組整合元件清單
- **`internal/live/doc.go`**: live 模組「混合基礎設施與業務邏輯」P3 重構說明
- **`internal/marketdata/macro_provider.go:45-51`**: 7 個新 macro 欄位定義
- **`internal/apigateway/limits.go:96-101`**: 7 個新 apigateway channel 註冊
- **`internal/orchestrator/system.go:123,192-193,353,634,1556,1591`**: API mode macro pipeline

---

## 九、稽核範圍聲明

本報告**僅為唯讀調查**，未修改任何 `*.go` 程式碼、未變動 `internal/live/`、`web/`、`cmd/`、`configs/` 任何檔案。**唯一新增檔案為本文件本身**。
