# internal/narrative/AGENTS.md

**模組快照**：敘事引擎 + KB-pipeline / snapshot-pipeline / InvestmentModel + 29 個 CausalTemplate（含 2026-09-16 五條第一性因果鏈，spec v0.2）+ DetectorRegistry 抽象層（Stage 5 新增）

## 核心抽象（Stage 5 重點）

| 型別 | 檔案 | 用途 |
|---|---|---|
| `Detector` (interface) | `detector.go` | Stage 5 新增：每個 trigger_theme 一個獨立 Detector impl，可單獨啟用/停用 |
| `DetectorRegistry` | `detector.go` | 統一註冊 + 並發呼叫所有 enabled detectors |
| `DetectionResult` | `detector.go` | 統一輸出 (Theme/Severity/Confidence/Source/Metadata)；`ToNarrativeEvent()` 向後相容 |
| `DetectorInput` | `detector.go` | 同時承載 `MarketNarrativeData` (KB) 與 `MacroDataSnapshot` (snapshot) |
| `CausalTemplate` | `templates.go` | 29 個 trigger_theme 的硬編碼模板（`DefaultTemplates()`） |
| `InvestmentModel` | `knowledge_base.go` | 21 個 Darwinian weight 演化模型（`NewNarrativeEngine()`） |

## 模組陷阱

### 1. Detector 與 detect 函式的雙軌制（INTENTIONALLY NOT MERGED）
`narrative_detectors.go:108-113` 明確標示：KB pipeline（`detectXxxEvent(data MarketNarrativeData)`，讀 DXY/綜合指標）與 ingestor pipeline（`detectXxxEventFromSnapshot(curr, prev marketdata.MacroDataPoint)`，用 ChangePct 代理）**不可合併**。前者 authoritative，後者是 degraded-mode proxy。

→ 新增 trigger detector 時，先確認應該對應到哪條 pipeline，不要混用。

### 2. tariff_shock 缺 KB pipeline
Stage 4 PR#2 的 detector_impls.go 把 tariff_shock 透過 ingestor 的 `detectTariffShockEventFromSnapshot` 包成 snapshot-pipeline detector（其餘走 KB pipeline；29 個中僅 `tariff_shock` 與 `conflict_deescalation` 走 snapshot pipeline）。這是當時的真實缺口 — KB 沒有 tariff_shock 函式。

→ Stage 6+ 可考慮新增 `detectTariffShockKBEvent`（讀 TradeNews 或 GeopoliticalGPR proxy）。

### 3. Seasonal detector 用 `time.Now()` 不可測
`detectSeasonalEvent()` 內部呼叫 `time.Now().UTC()` 決定月份 window。測試時**必須先 disable 全部 6 個 seasonal detector**（見 detector_e2e_test.go 的 `disableSeasonals` helper），否則會依測試執行日期 flaky。

### 4. NarrativeEvent.HitRate 由 lifecycle 補，不在 DetectResult
`DetectionResult` 不含 HitRate / Sentiment / Region — 這些由 `EventLifecycleManager` 後處理補上。如需這些欄位，呼叫 `ToNarrativeEvent()` 投影後再讀。

### 5. import cycle：narrative ← ledger
`internal/ledger/detector_scan_store.go` 為了 ScanResultRow 使用 `narrative.Severity` / `Source` 型別而 import narrative。**敘事套件的測試不能 import ledger**，否則 cycle。在敘事套件裡要測 SQLite round-trip 就放到 ledger package 測。

### 6. 29 templates 數量是 hard gate
`detector_e2e_test.go:TestE2E_AllThemesRegistered` 與 `detector_count_gate_test.go` 是 regression gate：

- `expectedCount` 讀 `documentedDetectorCount` 常數（不再各檔各寫一個數字）。
- `TestDetectorCount_RegistryMatchesDocumentedCount` 斷言 registry 大小 = 文件寫的數字。
- `TestDetectorCount_NoStaleCountClaims` 掃描 `countClaimCarriers` 內所有 detector/theme/template 數量敘述（前導式 `N detectors` / `N-template`、中文 `N 個偵測器` / `N 個主題`、後導式 `detector count: N` / `偵測器總數 N`、`(N) detectors`、全形數字…），只要一處與 registry 不符就紅。
- `TestDetectorCount_LegacyAllowlistIsNarrow`：唯一允許放行的是 `legacyCountLines`（逐檔、逐字串明列），而且**豁免是 span 級、不是整行** —— 同一行若還有其他數量敘述仍會被判（實測：把一句現況「N detectors 已註冊」接在 Wave／Stage 子系統的措辭後面會紅）。數字黏在字母／點／斜線上的形式（`...stageN-detector-plan.md` 這類檔名、`N/N themes` 這類 ratio、`N.0` 這類小數）由 token guard 直接視為非敘述 —— 同時也是明文記錄的盲區（`N/N detectors` 抓不到）。**沒有「看起來像歷史就跳過」的啟發式** — 第一版用距離視窗，一句現況敘述只要前面掛著 `Stage 5` 這類 marker 就整句被放行（本檔寫這段說明時被自己的閘門抓過兩次，正好證明它會咬人）。
- `TestDetectorCount_ClaimFilesAreClassified`：`cmd/atlas/` 與 `internal/narrative/` 底下任何「有講數量」的檔案，必須是 carrier 或明列在 `unclassifiedClaimFiles`（附理由）⇒ 新增這種檔案不可能再像當初 `cmd/atlas/main.go` 那樣被漏掉。
- `TestDetectorCount_ClaimClassifier_Contract`：pattern 層的雙向契約表（含明文記錄的盲區：英文數字 `twenty-four`、數字與關鍵字分離、跨行）。改 pattern 前先改這張表。
- `frontend_theme_label_sync_test.go`：直接斷言前端 `theme-labels.js` / `constants.js` 的 key 集合 == Go 側 `NewDefaultDetectorRegistry()` / `DefaultThemeDurations()`（註解不再自我保證）。

新增/刪除 template **必須同步** `templates.go` 的 `DefaultTemplates()`、`detector_impls.go` 的 detector 結構、`detector_count_gate_test.go` 的 `documentedDetectorCount` 常數、`detector_impls_test.go` 的 `allExpectedThemes` slice。

### 7. 五個主題尚無 InvestmentModel（knownModelGaps）
`knowledge_base_test.go` 的 `TestAllThemesHaveModel` 現在由 `DefaultTemplates()` 推導主題清單（不再手寫），沒有 model 的主題必須明文列在 `knownModelGaps`：目前是 `conflict_deescalation`、`dollar_softening`、`inflation_cool`、`inflation_moderate`、`us_earnings_boom`。這些主題命中時不會產生 sector bet — 缺口是顯式的，不是被一份過期清單藏起來。補上 model 後測試會要求你從 `knownModelGaps` 刪掉該筆（雙向閘門）。

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

NewDefaultDetectorRegistry() — 一鍵建構 29 個 detector 全啟用
```

## 驗證指令

```bash
go test -count=1 ./internal/narrative/...  # 80+ tests
go test -run TestE2E ./internal/narrative/   # PR#5 chain test
gofmt -l internal/narrative/                # 必須 0
```

### 7. 命中率來源標記不可移除（#1944 Batch 3, I23）
`InvestmentModel.HitRate` / `CausalTemplate.HistoricalHitRate` / `NarrativeEvent.HitRate` / `StructuralTrend.HitRate` 都是**手寫先驗常數**（`knowledge_base.go`、`templates.go`、`structural_trend.go`）；唯一的量測路徑（`EvaluateModels` → `updateTemplateHitRates`）**只存在記憶體**、重啟即失效、且**從不寫回 `hitRateForTheme()` 的靜態表**。

因此每個欄位都帶一個 `hit_rate_source`（值見 `hitrate_provenance.go`），由 `hitrate_provenance_test.go` 釘住：
- `handwritten_prior`：手寫先驗（detector 事件一律如此）
- `replay_eval_in_memory`：本行程由 replay 評估算出（未落地）
- `unavailable_no_samples` / `unavailable_no_template` / `not_populated`：無證據

改動任何一個 HitRate 的寫入點時，**必須同步**更新來源標記與該測試；對外欄位不得再出現無來源的命中率。另外 `ThemesWithoutTemplate` 列出無模板、因此不影響任何產業的 detector 主題（N-P4，目前為 `semiconductor_cycle_peak`）。

### 8. Duration 雙地圖技術債（2026-09-16 記錄，未合併）
`lifecycle.go::DefaultThemeDurations()`（canonical，golden 鎖定）與 `narrative_detectors.go::getThemeDuration`（KB 偵測器建事件用）是兩份獨立地圖，既有主題已有不一致（US_rates_up：14d vs 7d）。新增主題時兩處都要補 case；合併列 backlog。
