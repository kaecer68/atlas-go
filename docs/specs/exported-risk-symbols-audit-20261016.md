# 匯出但零生產引用：風控語意符號稽核（2026-10-16）

> **任務**：`FU-20261016-32`（root 指派）。來源是 `FU-20260930-01` 的「同型掃描（只列清單、未逐條處置）」。
> **範圍**：只處置 8 個符號；其餘候選**只列清單**（附錄 A），不改也不判定。
> **原則**：涉生產風控行為者**只提案不執行**（依「模糊決策只提案」）。本文件內的處置只有兩種：
> ①在 doc comment 標 `DECLARED ONLY / NOT ENFORCED` ＋ 加守門測試（不改行為）；
> ②純文件更正。**沒有任何一條改了生產計算路徑。**

---

## 0. 結論摘要（8 條）

| # | 符號 | 定義（`origin/main` `012f9845e`） | 測試在用 | 非測試引用 | 風控語意分類 | 判定 |
|---|---|---|---|---|---|---|
| 1 | `ModulatePosition` | `internal/orchestrator/industry_cycle_modulator.go:72`（method） | 有 | **0** | 部位大小縮放（信心 <0.3 ⇒ ×0.80） | **明示未啟用**（雙重關閉：`ModulatorWiringActive=false` ＋ `SetCycleCard` 無生產呼叫者） |
| 2 | `ModulateScore` | 同檔 `:90`（method） | 有 | **0** | 信心分數縮放（係數 → ×0.6–1.4） | **明示未啟用**（同 1） |
| 3 | `CycleAmplifiedMultiplier` | `internal/narrative/seasonal_bridge.go:36`（method） | 有 | **0** | 季節性乘數的週期放大 | **明示未啟用**（`SetCycleCard` 無生產呼叫者 ⇒ 卡片恆為 nil） |
| 4 | `GetRegimeMultiplier` | `internal/sim/dynamic_threshold.go:121`（method） | 有 | **0** | regime → 門檻乘數的**讀取子** | **其實不是缺陷**（值已在 `GetThreshold` 內套用；引擎讀取端全空是另一件事） |
| 5 | `ApplyRiskFilters` | `internal/live/agent_runner.go:221`（method） | 有 | **0** | 實盤 CRO/CIO 控制層過濾 | **其實不是缺陷**（上游分析管線已套用同一控制層；再呼叫會**重複過濾**） |
| 6 | `SetHaltTrading` | `internal/live/risk_gate.go:76`（method） | 有 | **0** | 停止交易閘門的**寫入端** | **該接線**（涉生產風控行為 ⇒ **只提案**） |
| 7 | `ShouldHaltTrading` | `internal/risk/macro_aware_drawdown.go:248`（method） | 有 | **0**（僅 `doc.go` 文件提及） | 回撤 → 是否停止交易的**述詞** | **其實不是缺陷**（同一述詞已內聯在 `StrategyEvolver`，見 §2.7） |
| 8 | `GetPositionSizeAdjustment` | 同檔 `:243`（method） | 有 | **0**（僅 `doc.go` 文件提及） | 回撤決策 → 部位倍數／曝險上限 | **該接線**（曝險上限從未到達 sizing ⇒ **只提案**） |

**一句話根因**：這 8 個符號的名稱都在描述「風控槓桿」，但其中 3 條由**別的**入口生效、3 條**雙重關閉**，
只有 **2 條**（停止交易閘門的寫入端、回撤曝險上限）**確實沒有任何生產路徑會行動**；
本 PR 只讓「已關閉／無效」顯性化 ＋ 加雙向守門，把真正要改行為的 2 條留成提案。

### 0.1 為什麼「測試在用」不是缺陷證據
這 8 條的測試呼叫都是**合法的**（測試直接呼叫公開方法驗算數字）。
缺陷不是「有測試」，而是**除了測試沒有生產呼叫者**，或**生產路徑由別的入口生效而文件沒說**。

---

## 1. 判準與掃描方法（可重現）

### 1.1 判準（逐字）
1. **候選母體**：`git ls-files '*.go'` 中**非 `_test.go`** 的檔案裡宣告的 **exported** 識別字（`func` / `method` / `type` / `var` / `const`）。
2. **生產引用**：`internal/` 與 `cmd/` 的**非測試** `.go` 中對該識別字的引用次數：
   - 同套件無限定的識別字（`Foo(`）；
   - `pkg.Foo` 形式（import 別名 → 套件目錄解析）；
   - `x.Foo` 形式的**選擇子**一律算引用（保守：同名就給分 ⇒ **寧可低估零引用清單**）。
3. **測試引用**：`_test.go` 內的同型引用。
4. **零生產引用**＝判準 2 為 0。**風控語意過濾**＝符號名命中
   `Risk|Halt|Circuit|Breaker|Drawdown|VaR|Leverage|Exposure|Modulat|Multiplier|Regime|Guard|Kill|Throttle|Concentrat|PositionSize|MaxOpenPositions`（不分大小寫）。
5. **人工複驗**：每條入選者在 `origin/main` 上以 `git grep` 逐字再確認（§1.3），
   避免 AST 掃描器的選擇子近似造成假零引用。

### 1.2 指令（我實際跑的）
```bash
# 批次 AST 掃描（參考實作見附錄 B；在 /tmp 內以獨立 module 執行，不進 repo）
cd /tmp/fu32-exported-scan && go run . <repo-root> /tmp/fu32-decls.json
#   輸出：decls=20627 zero_prod=1009 zero_prod_zero_test=272
```

### 1.3 單條複驗（任何人可重跑；這是權威清單的來源）
```bash
SYM=SetHaltTrading
git -C <repo> grep -n "$SYM" origin/main -- internal cmd | grep -v _test.go
#   origin/main:internal/live/risk_gate.go:74:// SetHaltTrading enables or disables a trading halt...
#   origin/main:internal/live/risk_gate.go:76:func (g *RiskGate) SetHaltTrading(halt bool) {
#   ⇒ 除定義檔本身，0 個生產引用
git -C <repo> grep -l "$SYM" origin/main -- internal cmd | grep _test.go
#   origin/main:internal/live/risk_gate_test.go
#   ⇒ 測試在用（合法）
```

### 1.4 與 `FU-20260930-01` 的 522／53 數字對帳（**不能逐字複現，據實說明**）
`FU-20260930-01` 寫「非測試 exported func/method 零生產引用但測試有引用 ⇒ **522**；風控語意過濾 ⇒ **53**」。
我用**同一段文字**當判準重跑：

| 判準讀法 | 我的結果 |
|---|---|
| 以「宣告」為單位、`func`+`method`、零生產引用、測試有引用、限 `internal/`＋`cmd/` | **652** |
| 以「名稱」去重（同名任一宣告有引用即算有引用） | **581** |
| 全部 exported（含 type/var/const）、零生產引用 | 1009（其中測試也零引用 272） |
| 再過風控語意（寬版 30+ 關鍵字） | 324 |
| 再過風控語意（**窄版**，§1.1 第 4 點） | **92** |

差異來源（無法消除：原始車道**沒有留下腳本**，只留一段文字判準）：
1. 他們是 **lexical**（`git grep` 等級）掃描，會扣掉「只出現在 doc comment／interface 宣告」的字串
   ⇒ 零引用數量應**更高**；
2. 我的選擇子近似（`x.Name` 一律算引用）讓同名方法互相給分 ⇒ 零引用數量應**更低**；
3. 兩者方向相反、都沒有腳本可對拍 ⇒ **數字不可逐字複現**。

⇒ 本文件**不沿用** 522／53；處置範圍就是 `FU-20261016-32` 指定的 **8 條**（§2），
其餘候選列附錄 A（84 條＝窄版 92 條扣掉本次 8 條）。

---

## 2. 逐條判定（8 條）

### 2.1 `ModulatePosition` — **明示未啟用**（不改行為）

- **位置**：`internal/orchestrator/industry_cycle_modulator.go:72`（`origin/main`）；修後 `:101`。
- **語意**：依 `CycleStatusCard.CycleConfidence` 縮放部位大小（<0.3 ⇒ ×0.80、>0.7 ⇒ ×1.0、其餘 ×0.90）。
- **非測試引用**：**0**。**測試引用**：`industry_cycle_modulator_test.go`。
- **★ 對 F-item 敘述的更正兩處**（動手前讀碼的結果）：
  1. F-item 寫「輸入是**已在生產消費**的 `CycleStatusCard`」。卡片的確有生產消費者，
     但**這條方法的輸入鏈沒有**：`IndustryCycleModulator.SetCycleCard`（`:53`）**零生產呼叫者**（僅測試 8 處），
     所以即使有人注入 modulator，這一側的 `card` 也恆為 nil。
  2. 真正的生產入口不是這個方法：modulator 若被注入，走的是
     `CollectModulationSteps`（`executor_collection.go:349`）⇒ 產生 `domain.ConvictionStep`；
     `ModulatePosition` **不在那條路徑上**。故它是**第二套未被消費的編碼**，不是「同一條路徑上少接一段」。
- **判定理由**：兩道閘門都關著（`ModulatorWiringActive=false`（`plugin_registry.go:314`，issue #1944 Batch 2）
  ＋ `SetCycleCard` 無生產呼叫者），且接線會**改變部位大小**（值語意）⇒ 依「模糊決策只提案」**不執行**。
- **處置**：doc comment 加 `DECLARED ONLY / NOT ENFORCED (FU-20261016-32)` ＋ 四條重新認證條件；列入守門測試（§3）。

### 2.2 `ModulateScore` — **明示未啟用**（不改行為）

- **位置**：同檔 `:90`；修後 `:126`。**非測試引用**：0。**測試引用**：有。
- 與 2.1 同一組閘門；生產的信心調整走 `CollectModulationSteps`。
- **處置**：同 2.1（doc 標記指向 2.1 的四條條件）。

### 2.3 `CycleAmplifiedMultiplier` — **明示未啟用**（不改行為）

- **位置**：`internal/narrative/seasonal_bridge.go:36`；修後 `:62`。**非測試引用**：0。**測試引用**：`seasonal_bridge_test.go`。
- **★ 讀碼結論（澄清）**：這個 bridge 的**另一半是活著的**：
  `SeasonalMultiplier` 是 `industry.NarrativeSeasonalProvider` 介面方法（`internal/industry/seasonality.go:56-60`），
  由 `SeasonalEngine` 在 `seasonality.go:287` 與 `:525` 呼叫 ⇒ 季節性調整在生產上生效。
  **沒被套用的只有「週期放大」這一層**，而它不在介面裡。
- **為什麼現在必然是 no-op**：`SeasonalBridge.SetCycleCard`（`:28`）**零生產呼叫者** ⇒ `cycleCard` 恆為 nil
  ⇒ 函式一律回傳 base（就算有人呼叫也不改變任何數值）。
- **判定理由**：接上去會讓「週期 × 季節性」相乘（同一擴張相位可能被表達兩次），
  與 `FU-20260930-01` 對矽相位乘數記錄的疑慮同型 ⇒ **只提案**。
- **處置**：doc 標記 ＋ 4 條重新認證條件 ＋ 守門測試。

### 2.4 `GetRegimeMultiplier` — **其實不是缺陷**（純讀取子）

- **位置**：`internal/sim/dynamic_threshold.go:121`；修後 `:135`。
  **非測試引用**：0（另有 `testdata/sim_api.golden.json` 的 API surface 記錄）。
- **語意**：回傳 `e.regimeMultipliers[regime]`；**同一個值早已被套用**在
  `GetThreshold`（`:71`）與 `calculateThresholdUnsafe`（`:95`）的 `regimeAdjustment := e.regimeMultipliers[regime]`。
  ⇒「沒人呼叫這個 getter」**不等於**「regime 乘數沒有生效」。
- **★ 查證時另發現一件事（不在 8 條範圍，僅記錄）**：整個 `DynamicThresholdEngine` 的**讀取端都沒有生產呼叫者**——
  `GetThreshold`（0）、`GetCurrentThreshold`（0）、`GetStats`（0）；
  生產端只有 `SetRiskAppetite`（`orchestrator/system_dispatcher.go:122`、`:135`）。
  ⇒「風險偏好寫進去、門檻從來沒被讀出來」。這會改變生產門檻計算 ⇒ **只提案**，列 §4 限制 2。
- **處置**：doc comment 加稽核註記（明寫「這不是控制缺口」＋ 指名真正缺口在哪），
  並在守門測試檔頭列入「**故意不進表**」的理由，避免後人重查。

### 2.5 `ApplyRiskFilters` — **其實不是缺陷**（上游已套用；再呼叫會重複過濾）

- **位置**：`internal/live/agent_runner.go:221`；修後 `:243`。**非測試引用**：0。
- **證據（生產路徑）**：
  1. 控制層（CRO/CIO，`domain.LayerControl`）在**分析管線**就被套用：
     `orchestrator/executor_pipeline.go:131` → `DefaultControlLayerStrategy.ApplyControl`
     → `PluginRegistry.ApplyControl`（`executor_control.go:104`）。
  2. 實盤 runner 拿到的是**已過濾完**的結果：`ApplyExecutionInput` 把
     `input.FinalRecommendations` 直接寫成 filtered 集合（`agent_runner.go:65`）。
  3. 本方法寫入的 `StateStore.SetFilteredRecommendations`，其讀取端 `GetFilteredRecommendations` **也只有測試在用**。
- ⇒ 不呼叫它**不會漏掉任何過濾**；呼叫它才是缺陷（同一批 recs 被控制層過濾兩次，
  且第二次輸入是「已被第一次裁過的集合」，`blocked_count` 統計也會失真）。
- **處置**：doc 改為「NOT A LIVE RISK GATE」＋ 指明上游路徑 ＋ 4 條重新認證條件；列入守門測試
  （防止有人「以為漏了」把它接上）。

### 2.6 `SetHaltTrading` — **該接線（只提案，不改行為）**

- **位置**：`internal/live/risk_gate.go:76`；修後 `:101`。**非測試引用**：0。
- **關鍵事實**：**閘門本體是活的**——`OrderManager.checkRiskGate` 每次下單都呼叫 `RiskGate.Check`
  （`order_manager.go:128`），`Check` 的第一個分支就是 `if g.haltTrading`。
  但 `haltTrading` **沒有生產寫入者**：
  - `SetHaltTrading`（`:76`）0 個生產呼叫者；
  - `SyncHaltFromCircuitState`（`:86`）只被 `SyncRiskGateFromCircuitBreaker`（`:97`）呼叫，
    而後者**0 個呼叫者**（連測試都沒有）。
- ⇒ `CircuitBreaker` 可以進 `CircuitHalted`／`CircuitPaused`（`internal/live/circuit_breaker.go` 有生產使用者），
  **但狀態從未鏡射進閘門**，所以「停止交易」這條硬規則在生產上**永遠不成立**。
- **★ F-item 敘述的更正**：`SyncRiskGateFromCircuitBreaker` 的 doc 原寫
  「The orchestrator **should call this on a schedule**」——查證後**沒有任何排程呼叫**（全 repo grep 0 命中），
  這是**與現況不符的文件**；本 PR 已改寫為 `NOT WIRED`。
- **判定**：**該接線**（斷路器 → 閘門 → `Check` 拒單）。但這會**改變生產風控行為** ⇒ 依「模糊決策只提案」**只提案不執行**。
  提案（3 個選項）：
  1. orchestrator 每次電路評估後呼叫 `SyncRiskGateFromCircuitBreaker(o.riskGate, o.circuitBreaker, varProvider)`
     （最小改動、最接近原設計）；
  2. 只鏡射 halt、不鏡射 VaR（避免把未校準的 VaR 接到拒單路徑）；
  3. 不接線，改為**移除**這三個寫入端（須先確認 `Status()` 的 `halt_trading` 沒被外部運維依賴）。
- **處置**：doc 標 `DECLARED ONLY / NOT ENFORCED` ＋ 4 條重新認證條件（含端到端測試要求：斷路器 Halted ⇒ 下單被拒）；
  3 個寫入端（`SetHaltTrading`／`SyncHaltFromCircuitState`／`SyncRiskGateFromCircuitBreaker`）全列進守門測試。

### 2.7 `ShouldHaltTrading` — **其實不是缺陷**（述詞已在生產內聯生效）

- **位置**：`internal/risk/macro_aware_drawdown.go:248`；修後 `:289`。
  **非測試引用**：0（`internal/risk/doc.go:18` 是流程文件提及；另有 `testdata/risk_api.golden.json` 的 API surface 記錄）。
- **證據**：本體是 `return decision.Action >= DrawdownSevere`，而**同一比較式**已在生產路徑：
  - `StrategyEvolver.determineState`：`if drawdown.Action >= risk.DrawdownSevere { return StrategySuspended }`
    （`internal/orchestrator/strategy_evolver.go:200`）；
  - `isEmergencyEscalation`：同一比較式用來繞過演化冷卻（同檔 `:183`）。
- ⇒ 緊急回撤**確實會**讓策略層進 `Suspended`；缺的只是這個便利包裝。
- **處置**：doc 改為「NOT A MISSING CONTROL」並指名內聯位置；**不進守門表**
  （不用「零引用」框住一個已被強制的語意）。

### 2.8 `GetPositionSizeAdjustment` — **該接線（只提案）**

- **位置**：`internal/risk/macro_aware_drawdown.go:243`；修後 `:271`。**非測試引用**：0（文件提及同上）。
- **★ 這條是真的缺口（與 2.7 相反）**：`Evaluate` **有**生產呼叫者
  （`internal/orchestrator/system_risk_session.go:380`），但決策裡的**曝險上限從未到達部位大小**：
  - `decision.MaxExposure` 的讀取者只有 `GetPositionSizeAdjustment`（0 呼叫者）與
    `CalculatePortfolioAdjustment`（同檔 `:275`，同樣 0 呼叫者）；
  - 會填 breakdown 的 `EvaluateWithPortfolio` 唯一呼叫者是 `cmd/stress-test`；
  - `PortfolioRiskAdjuster.AdjustWeights`（15% / 10% / 5% / 0 權重上限）唯一呼叫者也是 `cmd/stress-test`。
- ⇒ `DrawdownLevel.MaxExposure`（1.0 / 0.85 / 0.65 / 0.40 / 0.10）**只是一張意圖表**；
  緊急回撤時平台**不會**把曝險壓到 10%。
- **判定**：**該接線**，但屬生產風控行為 ⇒ **只提案**。提案要點即 doc 的重新認證條件：
  1. 決定上限在哪一層生效（組合權重引擎／下單 sizing／sim 每股 gate）；
  2. 證明不與 `PortfolioRiskAdjuster` 的權重上限重複套用；
  3. 端到端測試：`DrawdownEmergency` ⇒ 實際曝險 ≤ `MaxExposure`。
- **處置**：doc 標 `DECLARED ONLY / NOT ENFORCED` ＋ 條件；進守門表；
  並更正 `internal/risk/doc.go` 的「Decision flow」（原把兩步都寫成流程的一部分，未說哪一步沒生效）。

---

## 3. 負向證明（守門測試有牙齒）

**新增**：`internal/risk/exported_risk_levers_wiring_test.go` — 表驅動、雙向耦合：
`wired=false` 而有生產引用 ⇒ 紅；`wired=true` 而無生產引用 ⇒ 紅。
第三道「表不能腐爛」檢查：定義檔必須真的還宣告著該符號，否則紅。

**修前**（`origin/main`）：這支測試不存在（`git grep exported_risk_levers_wiring origin/main` ⇒ 空），
所以修前**任何接線都不會變紅**。

| 突變 | 打在行為行 | 結果（rc 讀自 log 檔） |
|---|---|---|
| **M1** 新增生產檔 `internal/live/zz_fu32_probe.go`（`g.SetHaltTrading(true)`；`go build ./internal/live/` rc=0） | 掃描面 | **rc=1**：`SetHaltTrading now has production consumer(s): [internal/live/zz_fu32_probe.go] — it is declared DECLARED ONLY / NOT ENFORCED (FU-20261016-32) ...`；移除探針後 **rc=0** |
| **M2** `GetPositionSizeAdjustment` 那列加 `wired: true` | 表的那一列 | **rc=1**：`GetPositionSizeAdjustment is declared wired but no non-test production file references it — wire it, or set wired=false ...`；`cp` 還原後 sha256 相等 ⇒ **rc=0** |
| **M3** 把列上的符號名改一個字（模擬改名／刪除） | 表的那一列 | **rc=1**：`CycleAmplifiedMultiplierRenamed: internal/narrative/seasonal_bridge.go no longer declares it — update or drop this guard row`；還原後 sha256 相等 |
| **M4** 在**同套件**新增只「提及」符號的生產檔 `internal/risk/zz_fu32_probe2.go` | 掃描面（證明 `extraSkip` 不是整包跳過） | **rc=1**：`GetPositionSizeAdjustment now has production consumer(s): [internal/risk/zz_fu32_probe2.go]`；移除後 **rc=0** |

守門測試檔案本身以 `cp` 備份，每次突變後還原，`sha256` 與突變前**逐位元組相等**
（`21ea43aa5cc69f5346cec1c12c4d1d757a6c358ea74644ddca1466af4cf49524`）。

---

## 4. 已知限制

1. **掃描器的選擇子近似**：`x.Name` 一律算引用，同名方法在不同套件會互相給分
   ⇒ 清單**傾向低估**零引用符號（保守方向）。逐條結論仍以 `git grep` 逐字複驗（§1.3）為準。
2. **`DynamicThresholdEngine` 的讀取端全空**（`GetThreshold` / `GetCurrentThreshold` / `GetStats` 皆 0 生產呼叫者，
   生產只有 `SetRiskAppetite`）：比 `GetRegimeMultiplier` 大得多的缺口，**本次不動**（會改生產門檻計算）。
3. **`SyncRiskGateFromCircuitBreaker` 的呼叫語意未定**：接上後是否同時鏡射 VaR
   （`UpdateVaR` 目前也是 0 呼叫者）需業主決定；本 PR 只標成 `NOT WIRED`。
4. **API surface golden 檔未動**：`internal/risk/testdata/risk_api.golden.json`、
   `internal/sim/testdata/sim_api.golden.json` 仍把這些符號列在公開介面上。
   本 PR **不移除**任何符號，故 golden 不需更新；若日後採「移除」路線，必須同步更新這兩份。
5. **只處理 8 條**：附錄 A 的 84 條候選未判定（其中 `GetRiskAppetite`、`ComputeComponentVaR` 已由本次註記覆蓋，
   `SyncHaltFromCircuitState`／`SyncRiskGateFromCircuitBreaker` 已併入 2.6 的守門）。

---

## 附錄 A：其餘風控語意零生產引用候選（84 條，**只列清單、未判定**）

判準同 §1.1（零生產引用 ＋ 窄版風控語意），已扣除本次 8 條。
`測試` 欄＝`_test.go` 引用次數（0 ＝ 測試也未用）。

| 符號 | 位置（`origin/main`） | 種類 | 測試 | 生產 | 備註 |
|---|---|---|---|---|---|
| `NewCircuitBreakerManager` | `internal/apigateway/circuitbreaker.go:213` | func | 15 | – |  |
| `BreakerStatus` | `internal/apigateway/gateway.go:267` | method | 5 | – |  |
| `RecentRegimes` | `internal/autobacktest/comparator.go:153` | method | 2 | – |  |
| `CapitalFlowActionRiskOn` | `internal/capitalflow/action_mapper.go:19` | value | 0 | – |  |
| `CapitalFlowActionRiskOff` | `internal/capitalflow/action_mapper.go:20` | value | 0 | – |  |
| `MacroOISignalInputGuard` | `internal/capitalflow/oi_alignment.go:47` | func | 1 | – |  |
| `CalibrateVaR` | `internal/config/inference.go:755` | method | 1 | – |  |
| `MacroRiskEvaluator` | `internal/config/macro_risk_calibrator.go:166` | func | 0 | – |  |
| `SetEventThrottle` | `internal/eventbus/eventbus.go:633` | method | 1 | – |  |
| `CalculateGlobalExposure` | `internal/globalmarket/global_market.go:414` | method | 2 | – |  |
| `AsymmetricRiskConfig` | `internal/industry/risk.go:60` | type | 2 | – |  |
| `AddCustomerConcentration` | `internal/industry/risk.go:103` | method | 11 | – |  |
| `GetCustomerConcentration` | `internal/industry/risk.go:110` | method | 3 | – |  |
| `GetAllRisksForIndustry` | `internal/industry/risk.go:392` | method | 2 | – |  |
| `SiliconPhaseWeightMultiplierWired` | `internal/industry/silicon_cycle.go:180` | value | 1 | – |  |
| `GetPhaseWeightMultiplier` | `internal/industry/silicon_cycle.go:481` | func | 3 | – |  |
| `RiskProfile` | `internal/industry/types.go:83` | type | 0 | – |  |
| `RegimeEventPayload` | `internal/live/eventbus.go:16` | type | 2 | – |  |
| `GuardOutcomeEventPayload` | `internal/live/eventbus.go:19` | type | 0 | – |  |
| `RiskEventPayload` | `internal/live/eventbus.go:21` | type | 1 | – |  |
| `EventRegimeChange` | `internal/live/eventbus.go:32` | value | 3 | – |  |
| `EventGuardOutcome` | `internal/live/eventbus.go:44` | value | 0 | – |  |
| `SetCircuitBreaker` | `internal/live/orchestrator.go:201` | method | 1 | – |  |
| `NewRiskAuditLog` | `internal/live/risk_audit.go:34` | func | 2 | – |  |
| `SyncRiskGateFromCircuitBreaker` | `internal/live/risk_gate.go:97` | func | 0 | – | guarded as part of item 6 |
| `GetRegime` | `internal/live/store/store.go:217` | method | 3 | – |  |
| `NewRiskSurfaceExtractionHandler` | `internal/llm/capabilities/risk_surface_extraction.go:32` | func | 5 | – |  |
| `BreakerState` | `internal/llm_annotator/annotator.go:183` | method | 8 | – |  |
| `CircuitClosed` | `internal/llm_annotator/circuit_breaker.go:21` | value | 4 | – |  |
| `CircuitOpen` | `internal/llm_annotator/circuit_breaker.go:22` | value | 2 | – |  |
| `CircuitHalfOpen` | `internal/llm_annotator/circuit_breaker.go:23` | value | 1 | – |  |
| `BreakerInfo` | `internal/marketdata/finmind_client.go:691` | method | 19 | – |  |
| `BreakerInfo` | `internal/marketdata/fubon_client.go:189` | method | 19 | – |  |
| `BreakerInfo` | `internal/marketdata/government_broker_aggregator.go:251` | method | 19 | – |  |
| `CircuitBreakerStats` | `internal/marketdata/hybrid_provider.go:706` | method | 1 | – |  |
| `BreakerInfo` | `internal/marketdata/taifex_futures_bars.go:151` | method | 19 | – |  |
| `BreakerInfo` | `internal/marketdata/taifex_provider.go:106` | method | 19 | – |  |
| `BreakerInfo` | `internal/marketdata/twse_openapi.go:417` | method | 19 | – |  |
| `BreakerInfo` | `internal/marketdata/yahoo_session.go:434` | method | 19 | – |  |
| `MacroflowRiskLevel` | `internal/methodology/advisor.go:51` | method | 3 | – |  |
| `GetBufferedRiskGateEvents` | `internal/monitoring/api/events/sse_handler.go:155` | func | 6 | – |  |
| `GetBufferedRegimeChangeConfirmedEvents` | `internal/monitoring/api/events/sse_handler.go:491` | func | 4 | – |  |
| `RiskExposureResponse` | `internal/monitoring/dashboard_types.go:84` | type | 0 | – |  |
| `NewDrawdownConsumer` | `internal/monitoring/drawdown_consumer.go:33` | func | 5 | – |  |
| `GetCircuitBreakerEvents` | `internal/monitoring/service/circuitbreaker.go:100` | method | 3 | – |  |
| `GetRegimeBreakdown` | `internal/monitoring/service/performance_service.go:133` | method | 4 | – |  |
| `GetRegimeCorrelation` | `internal/narrative/calibration/calibration_regime.go:299` | func | 0 | – |  |
| `DefaultRegimeConfig` | `internal/narrative/calibration_facade.go:87` | func | 0 | – |  |
| `SetRegimeConfig` | `internal/narrative/taiwan_stress_index.go:381` | method | 0 | – |  |
| `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins` | `internal/orchestrator/executor_pipeline.go:243` | func | 1 | – |  |
| `NewIndustryCycleModulator` | `internal/orchestrator/industry_cycle_modulator.go:39` | func | 3 | – |  |
| `RegisterRegime` | `internal/orchestrator/loader.go:45` | method | 2 | – |  |
| `NewNarrativeConvictionModulator` | `internal/orchestrator/narrative_conviction_modulator.go:27` | func | 2 | – |  |
| `ModulatorWiringActive` | `internal/orchestrator/plugin_registry.go:314` | value | 1 | – |  |
| `WithCycleModulator` | `internal/orchestrator/plugin_registry.go:316` | method | 1 | – |  |
| `WithNarrativeModulator` | `internal/orchestrator/plugin_registry.go:321` | method | 1 | – |  |
| `RegimeWinRateReader` | `internal/orchestrator/stockpicker_winrate_executor.go:128` | type | 0 | – |  |
| `GetPositionSizeLimit` | `internal/orchestrator/strategy_evolver.go:344` | method | 0 | – |  |
| `EvaluateAgentBreakers` | `internal/portfolio/agent_health.go:250` | method | 1 | – |  |
| `PeriodToRegime` | `internal/portfolio/period_detector.go:1229` | func | 2 | – |  |
| `GetRegimeConfig` | `internal/portfolio/regime.go:170` | method | 1 | – |  |
| `GetMaxExposure` | `internal/portfolio/regime.go:201` | method | 2 | – |  |
| `IsRiskOn` | `internal/portfolio/regime.go:227` | method | 2 | – |  |
| `NewRiskManager` | `internal/portfolio/risk_manager.go:95` | func | 28 | – |  |
| `GetRiskMetrics` | `internal/portfolio/risk_manager.go:294` | method | 6 | – |  |
| `CalculatePositionSize` | `internal/portfolio/risk_manager.go:384` | method | 4 | – |  |
| `CalculateStyleExposures` | `internal/portfolio/style.go:325` | func | 1 | – |  |
| `SetRegimeChangeCallback` | `internal/realtime/regime_adapter.go:491` | method | 0 | – |  |
| `WithRegimeListener` | `internal/recommender/handler.go:117` | method | 2 | – |  |
| `CalculateMaxPositionSize` | `internal/risk/capital_controller.go:103` | method | 1 | – |  |
| `NewCircuitBreaker` | `internal/risk/circuit_breaker.go:34` | func | 17 | – |  |
| `NewConcentrationAlertEmitter` | `internal/risk/concentration_alert_emitter.go:51` | func | 7 | – |  |
| `IndustryRiskProvider` | `internal/risk/industry_risk.go:31` | type | 0 | – |  |
| `NewCycleTrackerRiskProvider` | `internal/risk/industry_risk.go:44` | func | 5 | – |  |
| `DefaultDrawdownLevels` | `internal/risk/macro_aware_drawdown.go:48` | value | 0 | – |  |
| `NewMacroAwareDrawdownEngineFromParameters` | `internal/risk/macro_aware_drawdown.go:109` | func | 1 | – |  |
| `ComputeComponentVaR` | `internal/risk/var_calculator.go:39` | method | 0 | – |  |
| `RecordInvariantViolation` | `internal/sectorallocation/closure_state.go:204` | method | 2 | – |  |
| `MacroActionRiskOff` | `internal/sectorallocation/projector.go:18` | value | 0 | – |  |
| `GetRiskAppetite` | `internal/sim/dynamic_threshold.go:139` | method | 1 | – |  |
| `RegimeFromDomain` | `internal/sim/dynamic_threshold.go:170` | func | 1 | – |  |
| `WithRiskCalculator` | `internal/sim/engine.go:145` | method | 1 | – |  |
| `CloneAgentWithVariation` | `internal/spawning/agent_factory.go:371` | method | 4 | – |  |
| `RegimeFilteredWinRate` | `internal/stockpicker/winrate.go:135` | func | 4 | – |  |

---

## 附錄 B：掃描器參考實作（`/tmp` 內獨立 module，**不進 repo**）

```go
// go.mod: module expscan / go 1.21；只用標準庫
// 用法: go run . <repo-root> /tmp/decls.json
package main

import (
	"encoding/json"; "fmt"; "go/ast"; "go/parser"; "go/token"
	"os"; "path"; "path/filepath"; "sort"; "strconv"; "strings"
)

type decl struct {
	Name, Kind, File, PkgDir string
	Line, ProdRefs, TestRefs int
}

func main() {
	root, out := os.Args[1], os.Args[2]
	var files []string
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			switch filepath.Base(p) {
			case ".git", ".worktrees", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	fset := token.NewFileSet()
	// 1) 收集宣告：非測試檔的 exported func / method / type / var / const
	// 2) 逐檔 ast.Inspect：識別字即候選引用；先用 Pos 扣掉「宣告本身」；
	//    SelectorExpr.Sel 按名稱記給所有同名宣告（保守）；
	//    檔案內的 import 別名 → pkgDir 可精確記給該套件
	// 3) prod（非 _test.go）與 test（_test.go）分開累計
	_ = path.Base
	_ = strconv.Unquote
	_ = json.Marshal
	_ = fmt.Println
}
```

完整版（含 import 別名解析與 `byPkgName` / `allByName` 兩套索引）在本次車道暫存目錄 `/tmp/fu32-exported-scan/`；
上面保留**判準可辨識**的最小骨幹，避免把工具碼放進 repo。
統計輸出（`origin/main` `012f9845e`）：`decls=20627 zero_prod=1009 zero_prod_zero_test=272`。
