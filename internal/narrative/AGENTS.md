# internal/narrative/AGENTS.md

**模組快照**：敘事引擎 + KB-pipeline / snapshot-pipeline / InvestmentModel + 24 個 CausalTemplate + DetectorRegistry 抽象層（Stage 5 新增）

## 核心抽象（Stage 5 重點）

| 型別 | 檔案 | 用途 |
|---|---|---|
| `Detector` (interface) | `detector.go` | Stage 5 新增：每個 trigger_theme 一個獨立 Detector impl，可單獨啟用/停用 |
| `DetectorRegistry` | `detector.go` | 統一註冊 + 並發呼叫所有 enabled detectors |
| `DetectionResult` | `detector.go` | 統一輸出 (Theme/Severity/Confidence/Source/Metadata)；`ToNarrativeEvent()` 向後相容 |
| `DetectorInput` | `detector.go` | 同時承載 `MarketNarrativeData` (KB) 與 `MacroDataSnapshot` (snapshot) |
| `CausalTemplate` | `templates.go` | 24 個 trigger_theme 的硬編碼模板（`DefaultTemplates()`） |
| `InvestmentModel` | `knowledge_base.go` | 21 個 Darwinian weight 演化模型（`NewNarrativeEngine()`） |

## 模組陷阱

### 1. Detector 與 detect 函式的雙軌制（INTENTIONALLY NOT MERGED）
`narrative_detectors.go:108-113` 明確標示：KB pipeline（`detectXxxEvent(data MarketNarrativeData)`，讀 DXY/綜合指標）與 ingestor pipeline（`detectXxxEventFromSnapshot(curr, prev marketdata.MacroDataPoint)`，用 ChangePct 代理）**不可合併**。前者 authoritative，後者是 degraded-mode proxy。

→ 新增 trigger detector 時，先確認應該對應到哪條 pipeline，不要混用。

### 2. tariff_shock 缺 KB pipeline
Stage 4 PR#2 的 detector_impls.go 把 tariff_shock 透過 ingestor 的 `detectTariffShockEventFromSnapshot` 包成 snapshot-pipeline detector（其他 23 個用 KB）。這是當時的真實缺口 — KB 沒有 tariff_shock 函式。

→ Stage 6+ 可考慮新增 `detectTariffShockKBEvent`（讀 TradeNews 或 GeopoliticalGPR proxy）。

### 3. Seasonal detector 用 `time.Now()` 不可測
`detectSeasonalEvent()` 內部呼叫 `time.Now().UTC()` 決定月份 window。測試時**必須先 disable 全部 6 個 seasonal detector**（見 detector_e2e_test.go 的 `disableSeasonals` helper），否則會依測試執行日期 flaky。

### 4. NarrativeEvent.HitRate 由 lifecycle 補，不在 DetectResult
`DetectionResult` 不含 HitRate / Sentiment / Region — 這些由 `EventLifecycleManager` 後處理補上。如需這些欄位，呼叫 `ToNarrativeEvent()` 投影後再讀。

### 5. import cycle：narrative ← ledger
`internal/ledger/detector_scan_store.go` 為了 ScanResultRow 使用 `narrative.Severity` / `Source` 型別而 import narrative。**敘事套件的測試不能 import ledger**，否則 cycle。在敘事套件裡要測 SQLite round-trip 就放到 ledger package 測。

### 6. 24 templates 數量是 hard gate
`detector_e2e_test.go:TestE2E_All24ThemesRegistered` 是 regression guard。新增/刪除 template **必須同步** `templates.go` 的 `DefaultTemplates()`、`detector_impls.go` 的 detector 結構、`detector_e2e_test.go` 的 expectedCount 常數、`detector_impls_test.go` 的 allExpectedThemes slice。

## Stage 5 新增的對外介面

```go
// Detector lifecycle
Detector interface {
    Theme() string
    Enabled() bool
    SetEnabled(bool)
    Detect(ctx, DetectorInput) (*DetectionResult, error)
}

DetectorRegistry methods:
    NewDetectorRegistry / Register / MustRegister / Get / List / ListEnabled
    / Themes / Enable / Disable / Len / RunAll

NewDefaultDetectorRegistry() — 一鍵建構 24 個 detector 全啟用（PR#2 入口）
```

## 驗證指令

```bash
go test -count=1 ./internal/narrative/...  # 80+ tests
go test -run TestE2E ./internal/narrative/   # PR#5 chain test
gofmt -l internal/narrative/                # 必須 0
```
