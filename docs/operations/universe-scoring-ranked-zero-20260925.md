# 調查報告 — SmartUniverseBuilder `symbols_ranked = 0`（2026-09-25）

> **範圍**：生產（Mac Mini，atlas-go 容器，commit `db0709c1`）`universe_scheduler` 的
> `scoring_ok input=1599 ranked=0`
> **方法**：唯讀生產實測（`docker logs` / Prometheus `/api/v1/query` / `data/state/*.json`）
> ＋ 原始碼逐行追溯（`git blame` / `git log -S`）
> **性質**：**純報告，未修改任何生產行為或設定**。修法待業主核准。
> **與 PR #1979 的關係**：業主 PR **#1979**（`fix/20260925-inert-batch3`）已實作本報告 §6 的
> 修法選項 1 + 2（接上真實 quote provider、讓 `ranked=0` 不再是靜默值），並已
> **於 2026-09-25 合併（merge commit `bbce1b4a`）且部署**（見 §9.1 的部署事實）。
> 部署後的第一次觀察（授權的手動觸發）已在 §9.1.2–9.1.5：**provider 接線證實生效**，
> 而 `ranked` 的最終值受**非交易日／上游限流**影響，**仍待定**（§9.1.6）。
> 本報告**不重寫該修法**，而是提供它的「**為什麼**」：根因證據鏈（§3）、引入方式（§4）、
> 以及為何三個月來四道防線全部失效（§5）；並列出 #1979 **未覆蓋**的缺口與可直接落地的
> 告警／檢查條件（§6.2、§8）。
> **結論一句話**：`UniverseBuilderDeps.Quotes` 在生產 wiring 是**常數 nil**（`cmd/atlas/bootstrap_helpers.go:217`，
> 自 2026-06-22 commit `71771821` 起從未被改動），Step 3 因此走 `else` 分支使 `quoteMap` 恆為空 map，
> `ScoringScreener.applyVolumeAndPriceFilters` 對每個 symbol 都查不到 quote 而全部 `continue`，
> 於是 `survivors = 0 → ranked = 0`。**與 2026-09-25 啟用的 industry substrate gate（#1977）無關**：
> pre-gate 也是 0。

---

## 1. 事實鏈（生產實證，唯讀）

### 1.1 容器日誌（`docker logs -t atlas-go`）

```
2026-09-25T05:59:30.520Z level=INFO msg=daily_refresh_start  component=universe_scheduler
2026-09-25T05:59:30.538Z level=INFO msg=symbols_gathered  count=1599 full_rebuild=false component=universe_scheduler
2026-09-25T05:59:30.540Z level=INFO msg=industry_filter_ok input=1599 output=1599 component=universe_scheduler
2026-09-25T05:59:30.542Z level=INFO msg=scoring_ok      input=1599 ranked=0  component=universe_scheduler   ← ★
2026-09-25T06:00:00.717Z level=INFO msg=narrative_scrape_ok events=0 component=universe_scheduler
2026-09-25T06:00:00.719Z level=INFO msg=daily_refresh_ok built=1599 filtered=1599 ranked=0 excluded=0 component=universe_scheduler
2026-09-25T06:00:00.721Z level=INFO msg=coverage_check  mapped=27 total=27 ratio=1.00 component=universe_scheduler
2026-09-25T06:00:30.523Z level=INFO msg=daily_refresh_start  component=universe_scheduler      ← 同一分鐘窗第二次
2026-09-25T06:00:30.526Z level=INFO msg=scoring_ok      input=1599 ranked=0  component=universe_scheduler
2026-09-25T06:00:00.533Z level=INFO msg=coverage_check  mapped=27 total=27 ratio=1.00 component=universe_scheduler
```

**時間尺度**：`industry_filter_ok` 05:59:30.540 → `scoring_ok` 05:59:30.542，**2.4 ms 走完 1599 檔的排序**。
若真有 1599 筆因子評分（`CalculateMomentumScore` 等）與報價查詢，不可能在毫秒級完成。

### 1.2 快照（`data/state/universe_snapshot.json`）

```json
{"result": {"symbols_built": 1599, "symbols_filtered": 1599, "symbols_ranked": 0,
            "symbols_excluded": 0, "full_rebuild": false,
            "timestamp": "2026-09-25T06:00:30.523469111Z"},
 "ranked": []}
```

### 1.3 Prometheus（`localhost:9090`，instance `atlas:18080`）

| 指標 | 值 | 說明 |
|---|---|---|
| `atlas_universe_symbols_gathered_total` | 4797 | = 1599 + 3198（2 輪；見 §7.2 counter 灌爆） |
| `atlas_universe_symbols_screened_total{daily="failed"}` | 4797 | 全部 1599 檔每輪都被判 failed |
| `atlas_universe_symbols_screened_total{daily="passed"}` | **0** | 從未有任何 symbol 通過 |
| `atlas_universe_symbols_ranked_total` | **0** | 從未 > 0 |
| `atlas_universe_coverage_mapped_total{daily="all"}` | 81 | = 27 + 54（舊母體 27 × 2 輪；見 §7.2） |
| `atlas_universe_quotes_fetched_total` | **不存在** | ★ 決定性：Step 3 的 `if deps.Quotes != nil` 分支**從未執行** |

`atlas_universe_quotes_fetched_total` 是 Step 3 的 `if` 分支內唯一會建立 series 的地方
（`quotesFetchedCounter.Add(...)`，即使 Add(0) 也會建立 series —— 同一份程式碼的
`atlas_universe_symbols_ranked_total` 就是 `Add(0)` 後以 0 存在的實例）。
**它不存在 ⇒ 該分支從未進入 ⇒ 執行的是 `else` 分支。**

---

## 2. 呼叫鏈（commit `db0709c1`，精確到檔案:行號）

```
cmd/atlas/main.go:1963  newUniverseBuilderDeps(cfg, classTreeAdapter, gateway, um, suCfg, symbolIndustrySub)
  └─ cmd/atlas/bootstrap_helpers.go:188  func newUniverseBuilderDeps(...)
       └─ :217  Quotes:          nil,                      ← ★ 根因
  └─ cmd/atlas/main.go:1968  monitoring.NewDailyUniverseRefreshTask(suDeps)

internal/monitoring/universe_scheduler.go:143  NewDailyUniverseRefreshTask
  ├─ :156 alignToTarget(now)              → 06:00 UTC ±1min
  ├─ :164 BuildUniverse(ctx, deps, false)
  │    ├─ :332 gatherAllSymbols(deps.Tree, deps.Mapper, deps.Substrate)  → 1599（substrate 母體）
  │    ├─ :349 filter.Filter(allSymbols)                                 → 1599 → 1599
  │    ├─ :367-388 Step 3 Fetch quotes                                  → ★ 空
  │    ├─ :412 ss.Rank(filtered, quoteMap)                              → 0
  │    └─ :414 logging.Info(..., "scoring_ok", "input", 1599, "ranked", 0)
  └─ :191 CheckUniverseCoverage(deps.Mapper, deps.Tree, 0.50)           → 27/27 = 1.00

internal/monitoring/universe_scheduler.go:367-388（Step 3 全文）
    367:  // ── Step 3: Fetch quotes ─────────────────────────────
    369:  var quoteMap map[string]domain.Quote
    370:  if deps.Quotes != nil {                     ← 永不成立
    371:      quotes, err := deps.Quotes.GetQuotes(ctx, time.Now(), filtered)
    ...
    383:      if quotesFetchedCounter != nil {        ← 從未執行 → 指標不存在
    384:          quotesFetchedCounter.Add(int64(len(quoteMap)))
    385:      }
    386:  } else {
    387:      quoteMap = make(map[string]domain.Quote)   ← ★ 實際路徑：空 map
    388:  }

internal/monitoring/universe_builder.go:386 (s *ScoringScreener) Rank
  └─ :396 survivors := s.applyVolumeAndPriceFilters(normalizedUniverse, normalizedQuotes)
       (:416 為函式定義)
       :418  for _, sym := range universe {
       :419      q, ok := quotes[sym]
       :420      if !ok {                ← ★ 1599 檔全部在此 continue
       :421          continue
       :422      }
       :425      approxTWD := float64(q.Volume) * q.Last
       :426      if approxTWD < s.VolumeFloorTWD || q.Last < s.PriceMin { continue }
       :429      survivors = append(survivors, sym)
       :431  return survivors            → 長度 0
  └─ :401 passed, err := s.Screener.ScreenUniverse(ctx, survivors, ...)   → 空進空出
  └─ :406 ranked := s.scoreAndRank(passed, normalizedQuotes)              → 空進空出
  └─ :407 ranked = s.ApplyConcentrationCap(ranked)                        → :484 len==0 直接 return
  └─ :408 if s.TopN > 0 && len(ranked) > s.TopN                           → 不觸發
```

`Rank` 的**唯一入口屏障**就是 `applyVolumeAndPriceFilters`；`quoteMap` 為空 map 時它對
**每一個** symbol 都在 `:420` 短路，與 symbol 的身分、產業、價格、母體大小完全無關。

---

## 3. 根因證據鏈（三段全部可獨立驗證）

| 步驟 | 證據 | 原始輸出位置 |
|---|---|---|
| ① wiring 是 nil | `cmd/atlas/bootstrap_helpers.go:217  Quotes:          nil,` | §附錄 A1 |
| ② nil ⇒ 走 `else` | `atlas_universe_quotes_fetched_total` 在 live `/metrics` 完全不存在（同批 counter 全部存在） | §附錄 A2 |
| ③ 空 quoteMap ⇒ ranked=0 | `applyVolumeAndPriceFilters` 在 `quotes[sym]` 缺失時 `continue`；`screened{daily="failed"}=4797 / passed=0` | §附錄 A3 |

**排除「不是報價抓取失敗」**：若 `deps.Quotes` 非 nil 但 `GetQuotes` 回 error，日誌會有
`msg=quotes_fetch_error`（`universe_scheduler.go:373`），且 `quotesFetchedCounter.Add(0)` 仍會建立 series。
兩者皆無 ⇒ 不是「抓失敗」，是**根本沒有 provider**。

---

## 4. `#630`（2026-06-22）的引入方式：是**未完成接線**，不是刻意設計

`git log -S "Quotes:          nil"` 只有兩筆，`git blame` 顯示該行自首次寫入後**從未被改動**：

```
$ git log -S "Quotes:          nil" --format="%h %ad %s" --date=short
c77e7e5d 2026-06-22 refactor(cmd/atlas): extract 7 bootstrap helpers to bootstrap_helpers.go (#611 sub-issue-2 PR3)
71771821 2026-06-22 feat(monitoring): SmartUniverseBuilder pipeline + finmind infrastructure (#630)

$ git blame -L 217,217 cmd/atlas/bootstrap_helpers.go
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 217)         Quotes:          nil,      ← 95 天未動

$ git blame -L 239,239 71771821 -- cmd/atlas/main.go
717718216 (Kaecer 2026-06-22 06:39:49 +0800 239)         Quotes:          nil,      ← 首次寫入即 nil
```

判定為「未完成接線」的四項證據：

1. **沒有任何註解或 TODO**：`:217` 這一行是裸的 `nil`，同檔其他 nil 選項（例如
   `cmd_universe.go` 的 `RiskManager and QuoteProvider are nil (optional — dependent checks skip)`）
   都有說明，這裡沒有。
2. **同一結構的 doc 自相矛盾**：`internal/monitoring/universe_scheduler.go:76-77` 寫
   「**All fields except WorkDir are mandatory**; nil providers cause the associated pipeline step
   to be skipped gracefully rather than producing a hard error.」
   —— 契約說 mandatory，實作給 nil，而 `Quotes` 的「graceful skip」恰好等於**靜默丟掉整個母體**。
3. **同一 PR 正在關「nil 未接線」的洞**：`#630` 的 PR body 明列
   `P0-A6: ... cmd_universe.go + main.go 三處 nil→TreeBasedMapper 接線`。同一個 PR 把 Mapper 的
   nil 補掉了，`Quotes` 沒補 —— 同一類缺陷的漏網項，而不是另一種設計決定。
4. **當時就已有可用的 provider**：`internal/marketdata/hybrid_provider.go`（`NewHybridProvider`）
   於 2026-06-15 commit `07a3f85f` 就存在，比 pipeline 早 7 天；今天仍在使用
   （`cmd/atlas/main.go:2872`、`internal/orchestrator/system_dispatcher.go:251`）。
   ⇒ 「當時沒有 provider 可接」不成立。
   另一個旁證：**CLI 路徑**（`cmd/atlas/cmd_universe.go:104`）根本沒有走 `UniverseBuilderDeps`，
   它用 `marketdata.NewMockProvider()` 取報價（`internal/marketdata/twse.go:20`，`IsMock()==true`）
   ⇒ 兩條路徑都沒有接真實報價。

**對修法風險的意義**：這不是「刻意保留的 no-op 開關」（那種情況只要打開即可），
而是**從未被實作過的資料路徑**。所以修法必須真正選定 provider、驗證 1599 檔的抓取成本與
`Volume/Last` 語意，風險等級高於一般 config 回歸。

---

## 5. 為何 3 個月沒有被發現

四道「本該抓到」的防線全部失效，且失效方式彼此獨立：

| # | 既有檢查 | 為何抓不到 |
|---|---|---|
| 1 | `universe_coverage_check`（`cmd/atlas/main.go:1985` 起） | 它是 `symbols_built / TotalClassifiedSymbols(tree)`，**用的是 `built` 不是 `ranked`**，且分母是舊母體 27：pre-gate `27/27=100%`（無 alert）；post-gate `1599/27=5922%`（無 alert，只在 `< 90%` 才發）。**這個檢查在數學上不可能因 `ranked=0` 而觸發。** |
| 2 | Prometheus 告警規則 | 生產 Prometheus 目前載入 **35 條規則（10 個 group）**，**0 條**引用 `atlas_universe_*` 或 `ranked`（`/api/v1/rules` 實查）。repo 內 `monitoring/rules/` 三個檔也沒有 universe 指標。⇒ `atlas_universe_symbols_ranked_total = 0` 被輸出，但**沒有任何人看**。 |
| 3 | `scoring_ok` 日誌本身 | 它是 `logging.Info`，且**沒有「input > 0 但 ranked == 0」的判斷**（`universe_scheduler.go:414`）。空結果與正常結果在日誌上同等級。 |
| 4 | 快照的**消費端** | `data/state/universe_snapshot.json` 與 `data/state/universe.json` 的 ranked 清單在生產**沒有消費者**：`grep` 全 repo，讀取者只有 (a) 上述永遠不會 fired 的 coverage check、(b) `LoadUniverseRegistry` 只為了 `version+1`（`universe_scheduler.go:497-499`）、(c) CLI `build-universe status` 顯示。⇒ ranked 清單是空的也**不會弄壞任何下游功能**，因此沒有任何人回報症狀。 |

**加值觀察**：`0` 其實**已經被人看到**，只是被歸因錯誤。`CHANGELOG.md` 與
`configs/parameters.json` 的 gate rationale 都明寫 pre-flight 觀察到
「`universe_snapshot.json` = `symbols_built: 27 / symbols_filtered: 27 / **symbols_ranked: 0**`」，
並把它歸因為「母體只有 27 檔 ⇒ 產業層數字不顯著」。於是修法是**放大母體**（#1977），
而 `ranked=0` 的真因（沒有報價 provider）沒有被追。母體放大成功（27 → 1599）但 `ranked` 仍為 0，
正是這個歸因偏差的結果。gate 自己的驗收條件（`parameters.json` 的 `todo`）也明列
`symbols_ranked > 0` 屬於觀察窗不變量 —— 此刻是**違反狀態**，但沒有自動化執行該不變量。

---

## 6. 修法：選項 1+2 **已由 PR #1979 實作**；本報告是其根因與失效防線的完整佐證

> **本節定位（2026-09-25 更新）**：業主 PR **#1979**（`fix/20260925-inert-batch3`）
> 已實作本節的選項 1 + 2，並**已合併**（merge commit `bbce1b4a`）**且部署**（§9.1）。
> **本報告不重寫該修法**；它提供的是 #1979 的「**為什麼**」——
> 根因證據鏈（§3）、引入方式（§4）、以及為何 3 個月沒被任何防線抓到（§5）。
> §6.2 列出**#1979 未覆蓋的缺口**，並寫成可直接落地的告警條件。

### 6.0 #1979 的內容與交叉引用（★ 修法已由 #1979 實作並合併，不得重複實作）
在寫本報告的同時發現：**另一個 PR（#1979）已經實作選項 1 與選項 2**，並且**獨立得出同一個根因**。
**後續狀態：`bbce1b4a` 已於 2026-09-25 合併進 `main` 並部署**（部署事實見 §9.1）。

```
$ git log --all --oneline -S "Quotes:          quotes" -- cmd/atlas/
c39303a2 / 939d36a6  fix(monitoring,cmd/atlas): wire real quote provider into
                     SmartUniverse + make quote-less ranked=0 explicit (#1944 Batch 3)

$ gh pr list --state open
#1979  fix/20260925-inert-batch3  fix(inert): issue #1944 Batch 3 — 高嚴重 inert 項逐項結案（接線／明示未啟用／移除）
```

該 commit 的訊息（獨立來源，與本報告 §3 的結論一致）：

> The SmartUniverse pipeline has always been constructed with
> `UniverseBuilderDeps.Quotes == nil`, so `ScoringScreener.applyVolumeAndPriceFilters`
> dropped every symbol (it cannot evaluate volume or price) and the production snapshot
> recorded `symbols_ranked=0` / `ranked: []` for months. Nothing in the artifact
> distinguished "no quote provider was wired" from "the market really has no qualifying
> stock", so the defect was silent (#1944 I25).

它做的處置（`git show 939d36a6 --stat`，9 檔 +1133/−207）：

| 項 | 內容 |
|---|---|
| 接線 | `newUniverseQuoteProvider(cfg)` → `orchestrator.NewGatewayBackedProvider`（與 simulation 路徑同源），接進 `UniverseBuilderDeps.Quotes` 並共用給 Layer 2.5 `RiskExclusionFilter` |
| 不再靜默 0 | `UniverseBuildResult` 新增 `quotes_status` / `quotes_returned` / `ranked_fallback_reason` / `ranked_trustworthy`，每條 BuildUniverse 路徑都記錄 machine-readable 理由 |
| CLI | `atlas -build-universe run` 不再用 `marketdata.NewMockProvider()` + nil symbol list |
| 快照 schema | CLI 與 scheduler 的**雙 schema** 統一（原本兩邊互讀都靜默得到 0，連帶弄壞 D6 watchlist 輸入與 coverage alert，N-U3/N-U4） |

**對本報告的影響**：
1. 本報告的 §3 根因得到**第二個獨立來源**確認（不是只有本 session 的推論）。
2. §6 選項 1、2 **不必重新實作**，應改為「審核 #1979 是否覆蓋 §6 的風險 A–D」。
3. **#1979 沒有動 `CheckUniverseCoverage`**（`git show 939d36a6:internal/monitoring/universe_scheduler.go`
   仍是 `total = mapped`）⇒ §7.1 的 false green 與 #1979 **不重疊**；該 false green 另由 PR **#1983**
   修正，並已於 2026-09-25 **合併**（squash commit **`cd1f5b02`**；**尚未部署**，見 §9.1.7）。
4. 併發風險：`#1979` 與 `universe_coverage_check` 修正（另案）都改
   `internal/monitoring/universe_scheduler.go`；實測 `git merge-tree` 兩分支**無衝突**
   （`git merge-tree --write-tree` 乾淨，合併後同時含 `quotes_status`/`ranked_trustworthy`
   與 `CoverageReport`），但**合併後必須重跑編譯與測試**。


### 選項 1（**已由 #1979 實作**）：接上真實 quote provider
把 `cmd/atlas/bootstrap_helpers.go:217` 的 `Quotes: nil` 換成真實 provider
（`marketdata.NewHybridProvider(cfg.FinMindAPIKey, cfg.FugleAPIKey)`，與 `main.go:2872` 同源），
必要時加一層 adapter 滿足 `monitoring.QuoteProvider`。

- **風險 A（成本/配額）**：每日一次對 ~1599 檔取報價。FinMind 有日配額
  （`finmindDailyLimit = 14400`，`internal/marketdata/finmind_client.go:82`）且實測同日
  `quote_backfill` 曾因剩餘配額不足而 early-stop（`stopping early: FinMind quota remaining 1463 < 1500`）。
  Fugle 免費層亦有額度。**必須先離線量測**「1599 檔在現行 provider 下的實際呼叫數與配額足跡」。
- **風險 B（語意）**：`applyVolumeAndPriceFilters` 用 `q.Volume * q.Last` 近似成交金額，
  門檻 `volume_floor_twd = 10,000,000`、`price_minimum = 10`（`configs/parameters.json`）。
  若 provider 回的是「日線 bar」而 `Volume` 單位（股/張）與歷史語意不同，過濾結果會系統性偏移。
- **風險 C（代號對齊）**：`quoteMap` 以 `normalizeSymbol(q.Symbol)` 為鍵，母體鍵來自
  `substratePopulation`（去 `.TW`、去空白、排序）。必須確認 provider 回傳代號正規化後
  **完全對齊**，否則會出現「有報價但鍵不匹配」→ 仍為 0（與現況症狀相同，極難察覺）。
- **風險 D（時序）**：修好報價後 `ranked` 會第一次變成非 0，TopN=150 加上
  `MaxIndustryConcentration=0.40` 會第一次真正作用 ⇒ 這是**行為變更**，需要一次影子比對。

### 選項 2（**已由 #1979 實作**）：fail-loud，不要靜默 0
保留 nil 行為，但讓它**吵**：
- Step 3 的 `else` 分支加一次 `logging.Warn("universe_scheduler", "quotes_provider_missing")`；
- `len(filtered) > 0 && len(ranked) == 0` 時發出 `logging.Warn` + `monitor.Alert(warning)`；
- 讓 `quotes_missing` 成為可觀測欄位（新 counter 或既有 `QuotesErrors` 的新 label）。
- **風險**：極低（純觀測）。唯一副作用是每日會產生新的 WARN/alert 噪音，因此需與選項 1 一起排程。

### 選項 3：用既有 DB/歷史 bar 取代即時報價
以 `portfolio.NewHistoricalPrices()`（risk filter 已使用）或其他已落地的日線資料提供
`Volume/Last`，避免外部 API 配額。
- **風險**：需新增 adapter 與資料可得性驗證（哪些檔在 DB 有 bar）；若只有部分檔有資料，
  等於把「全部 0」換成「部分 0」，仍需選項 2 的可觀測性。

### 選項 4（**不建議自行實施**）：缺報價時不停用、改為降級
改 `applyVolumeAndPriceFilters` 讓缺 quote 的 symbol 進入評分。
- **風險**：這是**改變排序/評分語意**（會讓無報價資料的股票進入 universe），
  依本任務護欄屬**需業主核准**範疇。此外它會把「資料缺失」偽裝成「合格候選」，與
  `#1944`／`#1953` 這類「靜默降級」事故同族，建議不做。
- **狀態**：#1979 **沒有**採用此選項（正確：它選擇記錄 `ranked_fallback_reason` 而不是放行）。

### 6.1 選項 3 的狀態
#1979 **沒有**採用選項 3（DB/歷史 bar 取代即時報價）：它接的是
`orchestrator.NewGatewayBackedProvider`（即時來源），與選項 1 同路。
選項 3 仍可作為日後配額吃緊時的替代方案（見選項 1 風險 A），但**目前不需要**。

### 6.2 ★ #1979 **未覆蓋**的缺口（具體告警條件建議）

#1979 讓「缺報價」不再是靜默 0，但**沒有任何一條告警規則會被 `ranked=0` 觸發**。
本 session 實查到兩個尚未被覆蓋的洞：

**缺口 A：生產 35 條規則、0 條引用 `atlas_universe_*`**

```
$ curl -s localhost:9090/api/v1/rules   # 生產 Prometheus
total rules: 35 ; groups: atlas_container_liveness / atlas_startup_anomalies /
  channel_health_latent_staleness / llm_annotator_* / wave9_channel_individual_health
universe-related rules: 0
```
建議新增（`monitoring/rules/atlas_universe_pipeline_alerts.yml`，示意，**未實作**）：

```yaml
groups:
  - name: atlas_universe_pipeline
    rules:
      # (1) 有母體但零排名 —— 就是本案的形狀。
      #     注意：必須用「最後一次執行」的 gauge，不能用現行 counter（見 §7.2 灌爆）。
      - alert: AtlasUniverseRankedZero
        expr: atlas_universe_symbols_ranked_last > 0 == 0 and atlas_universe_symbols_built_last > 0
        for: 1m
        labels: {severity: warning}
        annotations:
          summary: "universe pipeline ranked=0 with a non-empty population"
          description: "built={{ $labels.built }} ranked=0 — 看起來像缺少報價 provider 或全數被篩掉"

      # (2) 報價來源沒有貢獻任何資料（本案的決定性訊號）。
      - alert: AtlasUniverseQuotesMissing
        expr: atlas_universe_quotes_fetched_last == 0 and atlas_universe_symbols_built_last > 0
        for: 1m
        labels: {severity: warning}
        annotations:
          summary: "universe pipeline ran with no quote data (provider missing or empty)"

      # (3) 覆蓋率跌破第一方母體的真實比例（配合 §7.1 的修正後才有意義）。
      - alert: AtlasUniverseCoverageBelowFloor
        expr: atlas_universe_coverage_ratio < 0.9
        for: 10m
        labels: {severity: warning}

      # (4) 執行次數異常（§7.4：±1 分鐘窗造成單日兩跑）。
      - alert: AtlasUniverseRunsPerDayHigh
        expr: increase(atlas_universe_snapshot_persisted_total[24h]) > 2
        labels: {severity: info}
```

**前置條件（重要）**：(1)(2)(4) 現在**不能**直接寫在 counter 上 —— `atlas_universe_*` 被 §7.2
的累加 bug 灌爆，數值會隨天數平方成長。落地順序必須是「先修 §7.2，或先補 `_last` gauge」。

**缺口 B：gate 的驗收條件目前處於違反狀態，但沒有自動化**

`configs/parameters.json` 的 gate `todo` 明列觀察窗不變量 `symbols_ranked > 0`；
如今（gate on、母體 1599）該不變量**違反**，卻沒有任何檢查會說出來。
建議（示意，**未實作**）：

```yaml
      # (5) 觀察窗不變量：連續 2 個交易日 ranked=0 即視為 gate 未達成，
      #     應觸發 #1971 的 rollback 路徑（parameters.json 的 todo 已定義該政策）。
      - alert: AtlasUniverseGateInvariantViolated
        expr: atlas_universe_symbols_ranked_last == 0 and atlas_universe_runs_last_2_sessions > 0
        for: 24h
        labels: {severity: critical}
        annotations:
          summary: "issue #1971 observation-window invariant (symbols_ranked > 0) violated"
          runbook: "docs/operations/universe-scoring-ranked-zero-20260925.md"
```

**缺口 C：`universe_coverage_check`（`cmd/atlas/main.go:1985`）本身不可用**
它的門檻是 `symbols_built / TotalClassifiedSymbols(27) < 90%`，post-gate 算出來是 5922%
⇒ 永遠不發。若保留這條檢查，分母必須改成第一方母體（此即另一案的修正範圍）。

---

## 7. 附帶缺陷（**只記錄，不修**，全部已證實）

### 7.1 `coverage_check` 的 `ratio=1.00` 是恆等式（false green）
`internal/monitoring/universe_scheduler.go:780-800`（`db0709c1`）：

```go
780: func CheckUniverseCoverage(mapper SymbolIndustryMapper, tree ClassificationTreeAccessor, threshold float64) (mapped int, total int, ratio float64, alert string) {
...
     allSymbols := map[string]bool{}
     for _, seg := range tree.GetLevel1() {
         for _, sym := range mapper.GetSymbolsByIndustry(seg.ID) { allSymbols[...] = true }
     }
790: mapped = len(allSymbols)
791: // Total is approximated as mapped; in a full implementation this would come
792: // from a separate universe source (e.g., TWSE listing count).
793: total = mapped            ← ★ 分母永遠等於分子
797: ratio = float64(mapped) / float64(total)   ← 恆為 1.00
```

- 量的是**舊的代表股母體 27 檔**，而 pipeline 已改用 1599 檔的 substrate 母體 ⇒ 兩者不同母體。
- `ratio` 在 27 檔、1599 檔、甚至母體塌成 3 檔時都會是 1.00；`alert` 只在 `mapped==0` 時才有機會出現。
- **影響**：這是「覆蓋率」告警的唯一來源，等於**不存在**。
- **現況（2026-09-25）**：本項已由 PR **#1983**（`fix/universe-coverage-substrate`）修正並合併
  （squash commit **`cd1f5b02`**）——分母改為第一方上游母體、無母體可測時明確回報不可測（§9.3）。
  **但尚未部署**（生產容器的 image 建於 `bbce1b4a`；見 §9.1.7）。
- **與 §3 根因的關係（明確回答）**：**不是同一個 bug**。`1.00` 來自 `total = mapped`（恆等式），
  與 `Quotes` 是否 nil 無關；即使報價接好、`ranked=1599`，這個 1.00 仍會照印。
  ⇒ 修 coverage **不會遮住** `ranked=0`，兩者是獨立缺陷。
- 另一條同族的死路：`cmd/atlas/main.go:1985` 的 `universe_coverage_check` 任務在
  `Asia/Taipei 06:00`（= 22:00 UTC）才進入受保護區塊，且以 `built/27` 計算
  ⇒ pre-gate 100%、post-gate 5922%，`< 90%` 永不成立。
  （本容器存活期間沒有 `coverage_check` label 的 series ⇒ 該區塊在本輪未執行。）

### 7.2 `atlas_universe_*` 計數器被自己的累積值灌爆（quadratic）
`internal/monitoring/metrics/degraded.go:73-78`：

```go
func (c *Counter) Add(n int64) {
    c.value.Add(n)
    if c.vec != nil && c.vec.OnInc != nil {
        c.vec.OnInc(c.name, c.labels, c.Value())   ← 傳的是「累積值」，不是 delta
    }
}
```

而 `internal/monitoring/metrics.go:87` 的 `MetricsCollector.RecordCounter` 是**累加語意**
（`existing.Value += value`）。⇒ 第 N 輪之後輸出值 = `x·N(N+1)/2`，不是 `x·N`。

實測完全吻合（今日 2 輪）：

| 指標 | 每輪增量 x | 觀測值 | 公式 x·N(N+1)/2 (N=2) |
|---|---|---|---|
| `symbols_gathered_total` | 1599 | **4797** | 1599+3198 = 4797 ✓ |
| `symbols_filtered_total{daily="industry_filter"}` | 1599 | 4797 | ✓ |
| `symbols_screened_total{daily="failed"}` | 1599 | 4797 | ✓ |
| `coverage_mapped_total{daily="all"}` | 27 | **81** | 27+54 = 81 ✓ |
| `coverage_total{daily="all"}` | 27 | 81 | ✓ |
| `snapshot_persisted_total` | 1 | **3** | 1+2 = 3 ✓ |
| `pipeline_duration_seconds` | 30s, 0s | **60** | 30+30 = 60 ✓ |

- **影響面**：所有以 `atlas_universe_*` 為基礎的 Prometheus 查詢/圖表會**隨天數平方成長**；
  目前的數值（4797）看似「跑了很多次」，實際只跑了 2 輪。任何未來的門檻/告警若寫在這組指標上，
  門檻會隨時間自然失效。
- **嚴重度建議**：P2（觀測正確性；不影響交易語意）。
- 附帶：label 名稱也被替換成值（`main.go` 的 `onInc` 把連續值兩兩配成 name=value）
  ⇒ 觀測到 `{daily="all"}`、`{daily="industry_filter"}`，而 `symbols_gathered_total` 完全沒有 label。
  同一族的觀測管線缺陷。

### 7.3 `alignToTarget` 使用**容器時區**，實際觸發時間是 06:00 UTC（= 14:00 台北），非 06:00 TW
`internal/monitoring/universe_scheduler.go:621-630`：

```go
621: // alignToTarget returns true when now is within ±1 minute of 06:00 local
622: // time (Taiwan stock market open).
623: func alignToTarget(now time.Time) bool {
624:     target := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, now.Location())
```

事實（唯讀實測）：

```
$ docker inspect atlas-go --format '{{range .Config.Env}}...' | grep -i '^TZ='   → （無輸出：未設 TZ）
$ docker exec atlas-go date                                                      → Fri Sep 25 06:50:47 UTC 2026
日誌時間戳（RFC3339，'Z'）為 05:59:30Z / 06:00:30Z  ⇒ 觸發於 06:00 UTC ±1min = 13:59/14:00 台北
```

- `now.Location()` 是**容器本地時區**（UTC）⇒ 觸發點 = **06:00 UTC = 14:00 台北**，
  與註解／註冊訊息宣稱的「06:00 TW」相差 **8 小時**。
- **跨日邊界查證（父代理要求）**：`target` 由 `now` 自身的 Y/M/D 組成，06:00 UTC 距 UTC 午夜 6 小時，
  **±1 分鐘窗不可能跨越 UTC 日期邊界**；`isTradingDay`/週一判斷也用同一個 UTC 週別，
  與台北週別在該時刻一致（14:00 台北 = 同一日）。⇒ **未發現跨日或跨週錯位的直接證據**。
- **真正的事實風險**：觸發時間是**環境相依**的 —— 只要未來為容器／主機設 `TZ`（例如
  `TZ=Asia/Taipei`），觸發點會**靜默前移 8 小時**（22:00 UTC = 06:00 台北）。同一個 binary
  在不同環境跑出不同排程時間，且沒有任何測試或告警綁住它。
- 另一事實：註解聲稱 06:00 是「Taiwan stock market open」，但台股開盤是 09:00；且 14:00 台北
  **在收盤（13:30）之後**，與「開盤前產生 universe」的敘事相反。此為文件與行為的雙重落差。
- **嚴重度建議**：**P1 候選**（排程時間與其宣稱契約不符 + 環境相依），但**不影響 `ranked=0` 根因**，
  也**未觀察到**已發生的錯誤執行時段。是否與 `#1978` 的 EventCalendar 日期缺陷同族，**不予結論**。

**建議處置（二選一；本報告未實作）**
1. **pin TZ**：在容器規格明確設 `TZ=Asia/Taipei`（`docker-compose.prod.yml` / cron 容器同樣處理），
   讓「06:00 台北」的字面契約成立；**風險**：所有其他依賴本地時間的排程/日誌會一起位移 8 小時，
   必須逐一盤點（不能只改 universe 這條路徑）。
2. **改成明確的台北時間**：把 `now.Location()` 換成
   `time.LoadLocation("Asia/Taipei")`，與 `cmd/atlas/main.go:1985` 的
   `universe_coverage_check`（已明確使用 Asia/Taipei）一致；**風險**：觸發時間由 14:00 台北
   變回 06:00 台北 ⇒ 這是**排程行為變更**，需要業主確認要哪一個時刻，並選在沒有其他
   依賴 universe 快照的流程之前/之後。
   無論選哪一個，都要**同步修正** `universe_scheduler.go:621-622` 的註解
   （「06:00 local time (Taiwan stock market open)」兩處都與事實不符）。

### 7.4 同一觸發窗會執行**兩次**
`alignToTarget` 允許 ±1 分鐘；任務每分鐘 tick ⇒ `05:59:30` 與 `06:00:30` 兩個 tick 都通過。
實測日誌兩輪完整出現，且 `snapshot_persisted_total`（扣除 §7.2 的灌爆後 = 2）確認跑了 2 輪。

- **影響面**：同一交易日重複執行整個 pipeline（含 D6 watchlist 的 read-modify-write、
  NarrativeEventBridge 重複抓 RSS、快照／registry 寫兩次、`version` 每次 +1）。
  D6 以「連續失敗交易日數」計數，重複執行是否會加速計數需要另外查證（**本報告未證實**）。
- **嚴重度建議**：P2（浪費 + 潛在計數語意），修法簡單但**需核准**。

---

## 8. 若要防再犯：最小檢查建議（**只寫建議，不實作**）

本報告的四項缺陷都不需要「更努力看日誌」才能發現，它們需要的只是**把契約寫成會失敗的檢查**。
以下按「最小、hermetic、可在 CI 跑」排序。

### 8.1 metrics 契約測試（對 §7.2，最小且最高價值）
- **斷言**：對每一個 counter vec，`Add(n)` 之後傳給 `OnInc` 的值必須是 **delta**（`n`），
  不是累積值；或反過來，`MetricsCollector.RecordCounter` 對 counter 型別必須是**覆寫**而非累加。
  兩者只能選一個語意，測試要把它釘住。
- **形式**：`internal/monitoring/metrics` 套件內的單元測試，不需外部服務。
  例：`m := NewUniverseMetrics(); m.SetOnInc(spy); m.SymbolsGathered.WithLabelValues("daily").Add(5); ... Add(5)`
  ⇒ 斷言 collector 的導出值 = 10，**不是 15**。
- **為什麼是最小**：這條測試若存在，§7.2 在寫下的當天就會紅。

### 8.2 snapshot 契約測試（對本案根因 + 單一 schema）
- **斷言**：`BuildUniverse` 的輸出在「有 provider 但回 0 筆」與「沒有 provider」兩種情況下，
  都必須帶可判讀的理由欄位（#1979 已加 `ranked_fallback_reason` ⇒ 測這個欄位）。
- **形式**：`internal/monitoring` 的單元測試，用 fake provider（nil / 空 / 有資料）三態各一案例。
- **加一條**：`UniverseSnapshot` 的 JSON 形狀只有一種（#1979 的 N-U3 已統一 ⇒ 用 golden 或欄位集斷言鎖住）。

### 8.3 「零結果」的 ops 檢查（對 §5 的四道失效防線）
- **最小形式**：一條 Prometheus 規則 `ranked_last == 0 and built_last > 0`（見 §6.2 缺口 A）。
- **CI 可做的部分**：`monitoring/rules/*.yml` 已有 `promtool` gate（#1981 引入）
  ⇒ 只要規則檔存在且語法正確就會被 CI 驗；
  另可加一條「每個會輸出 pipeline 結果的元件，至少要有一條引用它的規則」的**清單檢查**
  （純文字斷言，成本低）。
- **門檻校準**：不要用 `> 0` 當唯一門檻（母體正常時 ranked 本來就該 > 0，但數值範圍未校準）；
  先跑一段影子期記錄 `ranked` 的分布再加門檻。

### 8.4 排程時區契約（對 §7.3）
- **斷言**：`alignToTarget` 的行為以**明確的台北時間**為準，測試用固定 `time.Time`
  （`time.Date(..., time.UTC)`）驗證「06:00 UTC 不觸發、06:00 Asia/Taipei 觸發」（或反之，取決於 8.4 的決策）。
  目前 `alignToTarget(now)` 只看 `now.Location()`，因此測試若用 `time.Local` 就會隨執行環境漂移
  —— 這正是要釘住的地方。
- **形式**：`internal/monitoring/universe_scheduler_test.go` 的表格測試，零外部依賴。

### 8.5 「同一個觸發窗跑兩次」的 idempotency 檢查（對 §7.4）
- **斷言**：給定一天內的兩個相鄰 tick，pipeline 只執行一次（可用 `clockFunc` 覆寫測試）。
- **形式**：單元測試；若刻意允許重跑，則需斷言 D6 計數器不會被重複推進（見 §7.4 的未證實項）。

### 8.6 這個 session 的元教訓（為什麼 §5 的四道防線同時失效）
四道防線都**只檢查「有沒有東西」，不檢查「東西對不對」**：
coverage 用 `built` 而非 `ranked`、規則集完全不引用該子系統、日誌沒有門檻、產物沒有消費者。
⇒ 最小可持續做法不是加更多檢查，而是**替每個 pipeline 產物指定一個會因它而失敗的消費者或告警**。

---

## 9. 後續狀態、已知限制與追蹤

### 9.1 部署事實與第一次修法後的觀察（已證實）

#### 9.1.1 部署事實（唯讀實測）

本報告 §1 的所有事實取自 commit `db0709c1`（gate 開啟後、修法合併前的生產狀態）。
`#1979` 合併後的最新狀態：

```
$ ssh kaecer@kmacmini "cd ~/workspace/atlas && git log --oneline -3"
bbce1b4a fix(inert): issue #1944 Batch 3 — 高嚴重 inert 項逐項結案（接線／明示未啟用／移除） (#1979)
3ff4fc8a ci(workflows): 監控設定 gate（promtool/amtool，釘版容器）+ PR base guard（任務 I） (#1981)
6068f5db feat(ci): inert 閉環靜態檢查 + allowlist（#1944 建議 2） (#1980)

$ docker inspect atlas-go --format '{{.Image}} created={{.Created}} started={{.State.StartedAt}}'
sha256:a06f7d2e… created=2026-09-25T07:09:28.611161904Z started=2026-09-25T07:09:50.54550951Z
$ docker images atlas-atlas --format '{{.ID}} {{.CreatedAt}} {{.Tag}}'
a06f7d2e4aba 2026-09-25 15:09:24 +0800 CST latest
```

- 生產 repo 為 `bbce1b4a`，容器於 **2026-09-25 15:09（台北）重建並重啟**
  ⇒ **部署標的 = `bbce1b4a`**（部署驗收另記）。
- `origin/main` 其後前進到 **`47de2381`**（本報告自身的合併）；差異為 **docs-only**
  ⇒ **功能上與 `bbce1b4a` 無差異**。
- 部署任務包含一項**已授權的手動觸發**（趁部署後立即驗證母體管線），於 `07:10Z` 起在容器內執行
  `/app/atlas-go -build-universe run`（bounded ≤ 12 分）。以下 9.1.2–9.1.4 為該次執行的觀察。

#### 9.1.2 結論 ①：**provider 接線已證實生效**（部署前為 mock）

| 觀察（執行者於生產取得） | 意義 |
|---|---|
| `initialized inner=hybrid-fubon provider_cfg=hybrid` | 真實 provider 已建立；**部署前** CLI 路徑用的是 `marketdata.NewMockProvider()`（§4 第 4 點） |
| `get_quotes_ok provider=hybrid-fubon symbols=50` / `symbols=39` | 報價實際抓回，且**分批（50/批）**生效 ⇒ 對 1599 檔母體的抓取形狀符合 #1979 的設計 |

⇒ 這是本報告 §3 根因（`Quotes == nil` ⇒ 空 `quoteMap` ⇒ `ranked=0`）的**正面反證**：
同一條程式路徑在接線後確實能取得報價。

#### 9.1.3 結論 ②：`ranked` 的實際值受**資料可得性**影響，**不是** wiring 失敗

| 觀察 | 意義 |
|---|---|
| `finmind: asOf 2026-09-25 is not a Taiwan trading day (weekend or holiday)` | ★ **2026-09-25 非台股交易日**（當日為國定假日）⇒ 該日「當日報價」在資料源端本來就不存在 |
| `fetch_failed symbol=1605 err="rate limit wait: rate limited" component=fugle` | fugle 端限流 |
| `health_probe_failed "fubon proxy: … /health: context deadline exceeded"` + `fubon_failed_fallback` | Fubon proxy 逾時 → 回退 |

**明確的判讀紀律**：`ranked` 偏低（甚至為 0）在這種組合下**不可**記為 wiring 失敗。
必須先排除「非交易日 / 上游限流 / 回退」這三類資料可得性因素，否則就是把 §5 的歸因偏差
再犯一次（`0` 被誤讀成「市場沒有合格標的」）。

#### 9.1.4 配額與限流的範圍（不是 CLI 造成的偶發）

| 項 | 觀察 | 意義 |
|---|---|---|
| FinMind 配額 | `13368 → 13390`（**+22**） | 分批化 + 非交易日拒答**成功保護了 FinMind**（1599 檔未逐檔打） |
| Fugle 配額 | `236 → 439`（**+203**） | 壓力**落在 fugle**（被當成回退來源反覆嘗試） |
| 服務自身 log（30 分窗） | `rate limit` 出現 **19 行**；`level=ERROR` **0 行** | ⇒ 「fugle 限流」是**持續性**狀況，**不是** CLI 造成的偶發 |

#### 9.1.5 服務本體全程健康（本次手動觸發未造成中斷）

執行者實測：**24 個容器、0 個 not-running**、`/api/version` **HTTP 200**、服務 log `ERROR = 0`。

#### 9.1.7 coverage 修正（PR #1983）已合併但**尚未部署**

```
$ git log --oneline origin/main -2
cd1f5b02 fix(monitoring): universe coverage_check 改以第一方母體為分母，移除 total=mapped 的假綠 (#1943) (#1983)
47de2381 docs(operations): 調查報告 — SmartUniverseBuilder symbols_ranked=0（唯讀根因，未修） (#1982)
```

- **程式碼狀態**：`CheckUniverseCoverage` 的誠實版（分母＝第一方上游母體；無母體時
  `Available=false` + 「not measurable」）已在 `main`（`cd1f5b02`，2026-09-25 合併）。
- **部署狀態**：**尚未部署**。本次部署標的由業主指定為 **`bbce1b4a`**，容器 image 建於
  **2026-09-25 15:09（台北）**（`started=2026-09-25T07:09:50Z`，§9.1.1）
  ⇒ **`cd1f5b02`（coverage 修正）與 `47de2381`（本報告的 docs 合併）都不在本次部署內**。
  生產日誌目前仍會印舊形狀的 `coverage_check`，**直到下一次部署**才可能改變。
  ⇒ 看到 `main` 已合併**不等於**生產已修（**merged ≠ effective**）。

#### 9.1.6 待定（**尚未定案，不寫數字**）

- **狀態：觀察中。** 手動觸發（`07:09–07:10Z` 起）**仍在執行中**且容器未重啟；本報告完成時
  **尚未收到定案數字**，因此本節**刻意不寫任何 `ranked` 數值**。
- **判讀框架已先行寫定（§9.1.3）**：本次觀察的解釋變數是「非交易日 + fugle 限流 + Fubon 逾時」，
  與「provider 接線是否生效」（§9.1.2，已證實）**必須分開陳述**；在執行結束前不對結果下定論。
- **`symbols_ranked` 的最終值：待定。** 下一個觀察點為「本次手動觸發結束」或
  「下一次 06:00 UTC（14:00 台北）排程」；定案後本節會更新為定案值。
- `ratio ≈ 0.80`（§6）仍是推得的預期值，需在**交易日**以第一方母體重新觀察。

---

### 9.2 已知限制 (1)：`Reasons` 只收「未解析列」的理由（**刻意取捨**）

Part 2（`CheckUniverseCoverage`）的 `CoverageReport.Reasons` **只**收錄未解析列
（`unmapped` / `unknown`）的 `mapping_reason`，不收「已成功映射」那一側的理由。

- **為什麼刻意**：已成功映射的列，其理由多為「canonical L1 X 為唯一對應」這類長中文字串（約 20 條）
  ⇒ 若全收，alert 與 log 會被灌滿，真正描述缺口的理由反而被淹掉。
  稽核的目的正是「缺口在哪」，不是「成功的每一條為什麼成功」。
- **要看完整理由的入口**：
  `data/state/symbol_industry.json` 的 `unmapped_codes` / `unknown_codes`（含 `code`/`name`/`count`/`reason`）
  與 `counts`，或每一列 entry 的 `mapping_status` / `mapping_reason`。
- **若業主要全收**：改動是一行（把已映射列的理由一併 append），但需一併決定 alert 的截斷策略。

### 9.3 已知限制 (2)：`Coverage()` 反映最近一次**成功** Reload（TTL 6h）＝**靜默陳舊**風險

Part 2 的 substrate 稽核入口 `Coverage()` 讀的是 `storeSymbolIndustrySubstrate` 的記憶體視圖，
該視圖只在 `Reload` 成功時更新，且 TTL 為 6 小時。

- **風險情境**：store 在最後一次成功 Reload **之後**才壞掉 ⇒ 稽核會**繼續沿用舊視圖**，
  回報一個看起來正常（甚至很好）的比率，直到下一次 Reload 週期才可能改變。
  這正是本報告 §5、§8.6 在消滅的模式：**檢查回報「有東西」而不是「東西對不對」**。
- **目前無法區分**「載入失敗」與「載入成功但母體為空」——兩者都可能表現為不可測或舊值。
- **最小修法建議（**只建議，未實作**）**：
  1. `Coverage()` 回傳附 **`as_of`**（最近一次成功 Reload 的時間戳），讓稽核能揭露資料年齡；
  2. 告警端加一條「substrate 最近成功 Reload 超過 N 小時」的規則
     （與 §6.2 的規則同一組，需先解決 §7.2 的 counter 灌爆或改用 gauge）；
  3. substrate 曝露 `last_reload_ok` / `load_error`，讓「載入失敗」與「母體為空」可區分。
- **追蹤**：已記入 `docs/operations/FOLLOWUPS.md`（條目 **FU-20260925-01**，狀態 `open`，未實作）。

### 9.4 其他尚未證實項
見 §10 的「未證實」清單（`ratio ≈ 0.80` 亦為推得的預期值，尚未在生產觀察到）。

---

## 10. 已證實 vs 未證實

### 已證實（證據在 §1–§7、§9.1 引用的原始輸出／程式碼行）
1. `ranked=0` 的直接原因是 `deps.Quotes == nil` → Step 3 `else` 分支 → 空 `quoteMap`
   → `applyVolumeAndPriceFilters` 對 1599 檔全部 `continue`。
2. 該 nil 自 2026-06-22 `71771821`（#630）首次寫入即存在，95 天未被改動；**不是** #1977 造成的。
3. 「不是報價抓取失敗」：無 `quotes_fetch_error`、且 `quotes_fetched` series 完全不存在。
4. `scoreAndRank` 的 `len(rawScores)==0` 不是本案因子（`FactorEngine.CalculateAllScores` 至少回
   4 個鍵；且 `passed` 已為空，根本無法到達）。
5. `ApplyConcentrationCap` 不可能把非空輸入變成 0（`maxCount >= 1`）。
6. `IndustryFilter` 沒有濾掉任何人（`input=1599 output=1599`，無 `TargetLevel1`）。
7. 快照 `ranked=[]` 與日誌 `ranked=0` 同源（`:412-414` 同一個 slice）。
8. §7.1–§7.4 四項附帶缺陷（含 §7.2 的算術對帳全部吻合）。

### 未證實（**不要**當成結論）
1. **接上 provider 後 `ranked > 0`** —— **尚未定案**。`Quotes` 已由 #1979 接上且
   「provider 接線生效」已在生產觀察到（§9.1.2），但 `ranked` 的最終值受**非交易日 + fugle 限流 +
   Fubon 逾時**影響，須待該次手動觸發結束或下一次排程（§9.1.6）。風險見 §6 選項 1 的 A–D。
2. provider 對 1599 檔的實際呼叫數／配額足跡 —— 未量測。
3. provider 回傳代號與 `substratePopulation` 鍵是否 100% 對齊 —— 未量測。
4. `Volume/Last` 的單位語意是否與 `volume_floor_twd` 的設計假設一致 —— 未查證。
5. §7.4 重複執行是否會加速 D6 的 60 交易日計數 —— 未查證。
6. `alignToTarget` 的時區問題是否與 #1978 同族 —— 僅並列事實，**不下結論**。
7. 生產 Prometheus 的 35 條規則之外，是否有 Grafana 面板依賴 `atlas_universe_*` —— 未查。

---

## 11. 附錄：原始輸出

### A1 wiring（`cmd/atlas/bootstrap_helpers.go`，origin/main）

```
$ git blame -L 210,220 cmd/atlas/bootstrap_helpers.go
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 210)         return monitoring.UniverseBuilderDeps{
3ffc95f1e (Kaecer 2026-09-25 12:51:43 +0800 211)         Mapper:          monitoring.NewSubstrateIndustryMapper(treeMapper, substrate, classTreeAdapter),
3ffc95f1e (Kaecer 2026-09-25 12:51:43 +0800 212)         Substrate:       substrate,
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 213)         Tree:            classTreeAdapter,
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 214)         SupplyChain:     monitoring.AdaptSupplyChainGraph(industry.NewSupplyChainGraph()),
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 215)         Screener:        screener.NewEngine(factorEngine, portfolio.NewFundamentalProvider()),
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 216)         FactorEng:       monitoring.AdaptFactorEngine(factorEngine),
c77e7e5dc (Kaecer 2026-06-22 22:18:13 +0800 217)         Quotes:          nil,
```

### A2 live `/metrics`（Mac Mini，`curl localhost:18080/metrics | grep atlas_universe`）

```
atlas_universe_coverage_mapped_total{daily="all"} 81.000000
atlas_universe_coverage_total{daily="all"} 81.000000
atlas_universe_pipeline_duration_seconds 60.000000
atlas_universe_symbols_gathered_total 4797.000000
atlas_universe_symbols_filtered_total{daily="industry_filter"} 4797.000000
atlas_universe_symbols_ranked_total 0.000000
atlas_universe_symbols_screened_total{daily="failed"} 4797.000000
atlas_universe_symbols_screened_total{daily="passed"} 0.000000
atlas_universe_snapshot_persisted_total 3.000000
atlas_universe_narrative_events_scraped_total 0.000000
                          ← atlas_universe_quotes_fetched_total 不存在（★ 決定性）
```

### A3 快照與母體

```
data/state/universe_snapshot.json:
  result = {"symbols_built":1599,"symbols_filtered":1599,"symbols_ranked":0,
            "symbols_excluded":0,"full_rebuild":false,"timestamp":"2026-09-25T06:00:30.523469111Z"}
  ranked = [] (len 0)

data/state/symbol_industry.json:
  channel=symbol_industry  updated_at=2026-09-25T05:26:49Z  entries=1988
  counts = {"total":1988,"mapped":1599,"unmapped":379,"unknown":10,"canonical_l1":20}
  unknown_codes = [{"code":"91","count":10,"reason":"code not declared in namespace twse_industry_code
                    (upstream drift; report, do not map)"}]
```

### A4 生產 Prometheus 規則盤點

```
$ curl -s localhost:9090/api/v1/rules | (統計)
total rules: 35
groups: ['atlas_container_liveness','atlas_startup_anomalies','channel_health_latent_staleness',
         'llm_annotator_availability_fast_burn','llm_annotator_availability_slow_burn',
         'llm_annotator_circuit_breaker','llm_annotator_error_category_spikes',
         'llm_annotator_traffic_cessation','llm_annotator_recording','wave9_channel_individual_health']
universe-related rules: 0
```

### A5 容器時區

```
$ docker inspect atlas-go --format '{{range .Config.Env}}...' | grep -i '^TZ='   （無）
$ docker exec atlas-go date                                                      Fri Sep 25 06:50:47 UTC 2026
$ docker inspect atlas-go --format '{{.HostConfig.LogConfig}} {{.State.StartedAt}} {{.RestartCount}}'
  {json-file map[max-file:5 max-size:20m]} 2026-09-25T05:25:47.095347596Z 0
```
