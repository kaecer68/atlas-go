# docs/operations/FOLLOWUPS.md — 待辦 / 已知限制追蹤

> **用途**：記錄 atlas-go 在調查、修法或部署過程中**已知但刻意延後**的項目
> （含「已知限制」與「只建議未實作」的硬化方向），避免只存在於 PR 說明或 agent 回報中而流失。
>
> **不是**：缺陷主題的完整規格（放 `docs/` canonical 文件）、
> 也不是 channel 級長期故障（那放 `internal/monitoring/known_issues.go`）。
>
> **格式**：每個條目一個 `###`；`狀態` 只允許 `open` / `in-progress` / `done`。
> 完成時**不刪除**條目，改成 `done` 並補 `完成於`（保留決策痕跡）。
> 來源一律附可重現的命令或檔案:行號。

---

## 遺留項目

### FU-20260925-01 — `symbolIndustryCoverage.Coverage()` 只反映最近一次成功 Reload（TTL 6h）⇒ 靜默陳舊

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **為何放在 `docs/operations/` 而非 repo 根目錄**：atlas-go 原本**沒有** repo 根目錄的
  `FOLLOWUPS.md`（該路徑是 **a2a-dev** 的跨 repo 慣例）；且本 repo 的 `docs/` 根目錄受
  `scripts/ci/check_docs_governance.sh` 治理（根目錄新增未追蹤 `.md` 會被 CI 擋下），
  而 `docs/operations/README.md` 自述本目錄即為「操作 runbook 與**事件後續追蹤**文件」。
  ⇒ 依本 repo 慣例改放此處；若日後要與 a2a-dev 對齊，需一併更新該 governance 白名單與文件地圖。
- **來源**：PR #1983（`fix/universe-coverage-substrate`，**已於 2026-09-25 合併** squash commit `cd1f5b02`）+ 本 repo
  `docs/operations/universe-scoring-ranked-zero-20260925.md` §9.3
- **現況**：`cmd/atlas/symbol_industry_substrate.go` 的 `storeSymbolIndustrySubstrate.Coverage()`
  （新增，供 `monitoring.CheckUniverseCoverage` 的 `industry.SymbolIndustryCoverageReporter`
  型別斷言使用）讀的是記憶體視圖，只在 `Reload` **成功**時更新，TTL `substrateReloadTTL = 6h`。
- **風險**：store 在最後一次成功 Reload **之後**才壞掉時，覆蓋率稽核會**沿用舊視圖**並回報
  看似正常的比率，直到下一個 Reload 週期才可能改變。即「檢查回報有東西，而不是東西對不對」——
  與 #1944 / #1953 / 本 session 的 false-green 主題同族。目前亦**無法區分**
  「載入失敗」與「載入成功但母體為空」。
- **影響面**：`universe_scheduler` 的 `coverage_check`（daily / weekly）與其告警；
  不影響評分或排序（稽核路徑 only）。
- **最小修法建議（只建議，未實作）**：
  1. `Coverage()` 回傳附 **`as_of`**（最近一次成功 Reload 的時間戳），讓稽核能揭露資料年齡；
  2. 告警規則增加「substrate 最近成功 Reload 超過 N 小時」
     （需與 `universe-scoring-ranked-zero-20260925.md` §6.2 同組；注意 §7.2 的
     `atlas_universe_*` counter 灌爆問題，優先改用 gauge）；
  3. substrate 曝露 `last_reload_ok` / `load_error`，區分「載入失敗」與「母體為空」。
- **2026-09-25（任務 O）落地進度**：上面「最小修法建議」三項中的第 1、3 項**已落地**（PR
  `feat/monitoring-gaps-and-ttl-o`，只做可見性、不改任何既有語意）：
  1. `CoverageReport`（`internal/monitoring/universe_scheduler.go`）新增 `AsOf time.Time`，
     由**新的、獨立的**可選介面 `industry.SymbolIndustryCoverageAsOfReporter`（實作於
     `cmd/atlas/symbol_industry_substrate.go` 的 `(*storeSymbolIndustrySubstrate).CoverageAsOf()`）
     填入；zero value 代表「從來沒有成功 load 過」⇒ **不可當新鮮**。
     兩處 `coverage_check` 的結構化日誌各加 `as_of`（RFC3339 UTC；未知寫 `"unknown"`）、
     `data_age_hours`（未知寫 `-1`）⇒ 陳舊在日誌上直接可見。
  3. `CoverageReport.LoadError`（由 `industry.SymbolIndustryReloadErrorReporter` 填入）
     把「載入失敗」與「載入成功但母體為空」分開 —— 兩者都表現為 `Upstream == 0`，
     卻需要完全相反的行動。
  **第 2 項（規則/告警）仍未落地**：需要一個 gauge（`atlas_universe_*` 是灌爆的 counter，
  見 `atlas_universe_scoring_alerts.yml` 檔頭 (1)），且會動到新的指標面 ⇒ 留在本條目。
  兩個新增的日誌欄位**沒有自動化測試**（要跑完整 `BuildUniverse` pipeline 才驗得到；
  依任務護欄刻意不抽共用 helper），`data_age_hours` 用的是 wall clock 而非既有 `clockFunc()`。
- **驗收條件**：稽核輸出可看到資料年齡；且「store 壞掉」在一個 Reload TTL 內會被告警指出。

---

### FU-20260925-02 — 母體評分告警的 6 個未覆蓋缺口（a）–（f）的下落

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：任務 M 的盤點 + `monitoring/rules/atlas_universe_scoring_alerts.yml` 文末「已知缺口」
  清單；任務 O 追加第 4/5 條後的更新（同一份規則檔的文末清單已同步改寫）
- **現況**（逐項，含 2026-09-25 任務 O 之後的最新狀態）：
  - **(a) 連假 ≥ 6 天**（相鄰兩次執行間隔 > 144h，農曆年就是）活動側歸零 ⇒ 不覆蓋，
    需要假日曆。
  - **(b) 整條 pipeline 完全沒跑**（不是「跑了但空轉」）⇒ 不覆蓋，需要排程心跳 / `absent()`。
  - **(c) `atlas_universe_symbols_screened_total` 被改名/移除** ⇒ 三條同時沉默。
    **已由任務 O 的第 4 條 `AtlasUniverseScreenedFamilyMissing` 覆蓋**（含 peer gate，理由與
    殘餘缺口寫在規則檔內）。殘餘：若 `filtered` 與 `screened` 同時被改名 ⇒ 仍沉默。
  - **(d) Step 1/2 early-return** ⇒ 所有 counter 存在但永不增加：
    - `empty_filtered` ⇒ **已由任務 O 的第 5 條 `AtlasUniverseEmptyFiltered` 覆蓋**；
    - `empty_universe`（Step 1 就 0 檔）⇒ **仍不覆蓋，需要 Go**：`gatheredCounter.Add`
      在 `if SymbolsBuilt == 0 { return }` **之後** ⇒ 這條路徑真的不動任何 counter。
      現有可用的機器可讀標記只有 snapshot 的 `result.quotes_status="not_attempted"`（未曝露成指標）。
  - **(e) `symbols_ranked_total` 的 stage 在生產無法區分**（label 配對缺陷使該 family 無標籤，
    兩個 stage 共用同一個 series）⇒ weekly 成功一次就會遮蔽「daily 評分死亡」≥ 6 天。
    **不可**在標籤缺陷修好前加 stage matcher（那會讓規則對現行的無標籤 series 完全不 match
    = 零偵測）；見 FU-20260925-06。
  - **(f)** 共同後果：整組規則是「有產出但產出是空的」的偵測器，**不是**萬能心跳。
- **驗收條件**：（a）（b）各有一條新規則或其等價的 Go 端訊號；（d）-`empty_universe` 有一條
  Go 端心跳指標；（e）在 label 修好後能把 daily/weekly 分開判定。

---

### FU-20260925-03 — 任務 L 報告 §6.2 草案中 3 條「未落地」規則的理由（不是遺漏）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：`monitoring/rules/atlas_universe_scoring_alerts.yml` 檔頭「與任務 L 報告 §6.2 草案的關係」；
  L 報告 §6.2（`docs/operations/universe-scoring-ranked-zero-20260925.md`）
- **現況**：草案列 5 條「示意、未實作」，其中 (1)(2) 已由任務 M 落地（第 1/3 條），
  另外三條**刻意不落地**，理由如下（每一條都是「現在寫會寫出假規則」，不是還沒排到）：
  1. `AtlasUniverseCoverageBelowFloor`：**沒有 ratio 形態的 series**。只能拿
     `coverage_mapped_total / coverage_total` 相除，但那是「產業母體映射覆蓋」這個**不同維度**
     的訊號（本案是評分階段）；而且累積比值與視窗比值的語意需要現場校準 ⇒ 建議另開一張票。
  2. `AtlasUniverseRunsPerDayHigh`：`increase()` 在這族**被灌爆的 counter** 上**不等於執行次數**
     （第 N 輪貢獻的是 N，不是 1 ⇒ 單日兩跑在 N 大時會算出 5、7…），門檻 `> 2` 會隨天數自然誤報。
     需先修 counter 語意（任務 N），或改用 `_last` / gauge 形態。
  3. `AtlasUniverseGateInvariantViolated`：與第 1 條**同源**（同一個 expression，只差 severity 與
     `for:`）。不為同一個根因發兩條會各自 paging 的規則；#1971 gate 的 rollback 政策寫在
     `configs/parameters.json` 的 todo，屬業主裁決 —— 若決定自動化，把第 1 條的 severity 升為
     `critical` 或加一條 escalation 即可。
- **驗收條件**：三條各自在其前置條件成立後才被實作；實作時不得讓任兩條對同一根因同時 paging。

---

### FU-20260925-04 — 2026-09-28（週一）交易日 06:00Z 部署後複驗

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **為何是這一天**：`auto_universe_refresh`（daily）**顯式跳過週一**
  （`internal/monitoring/universe_scheduler.go` 的 `now.Weekday() == time.Monday` 分支），
  週一由 `auto_universe_full_rebuild`（weekly）跑。所以要驗「修好後的常態」必須挑一個
  週一 06:00Z（= 14:00 台北；容器不設 TZ，見 FU-20260925-05）**且是交易日**的日子 ——
  2026-09-28 是最近的這種日子（`-simulate` / 交易日曆另可交叉驗證）。
- **要驗什麼**：`data/state/universe_snapshot.json` 的
  `result.quotes_status == "ok"` **且** `result.ranked_trustworthy == true`；
  並確認 `symbols_ranked` > 0。這兩個欄位是 #1979 之後專為本案加的機器可讀判定
  （見 `docs/operations/universe-scoring-ranked-zero-20260925.md`）。
- **同時要看的（任務 O 的新規則）**：五條規則（第 1/2/3/4/5 條）在常態下**全部靜默**。
  若第 4 條 firing，先按它的 triage 第 1 步看 `quotes_status`，不要直接假設 family 被改名。
- **來源**：任務 M 的規則檔檔頭 (2) 段的排程事實；任務 O 的規則驗證需求。
- **驗收條件**：上述兩個欄位成立；且五條規則的 Alertmanager 狀態為 inactive（或 resolve）。

---

### FU-20260925-05 — `alignToTarget` 的時區環境相依（註解與行為不一致）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：任務 M 實證（規則檔檔頭 (2)）；`cmd/atlas` 內的註解寫「06:00 TW」，
  而容器**不設 `TZ`** ⇒ `alignToTarget` 的 06:00 實際是 **06:00 UTC = 14:00 台北**。
- **風險**：**註解是錯的，行為是對的** —— 這種不一致最容易讓人照註解去查錯時段
  （例如在台北時間 06:00 查「為什麼沒跑」，而排程根本還沒到）。
  已被本團隊實際引用過一次（規則視窗 `[6d]` 的推導就是靠正確的 UTC 事實）。
- **最小修法建議**：把 `cmd/atlas` 內「06:00 TW」的註解改成「06:00 UTC（容器不設 TZ）＝ 14:00 台北」，
  或在 compose 明確設 `TZ=Asia/Taipei` 並同步改註解與規則視窗推導。**兩者只能選一**。
- **指向**：任務 N（counter/label/語意修正包）一起處理，避免同一檔案兩次改動。
- **驗收條件**：註解、`docker-compose.prod.yml` 的 TZ 設定、以及規則檔的視窗推導三者一致。

---

### FU-20260925-06 — 本包（任務 O）之後才能做的事：counter 灌爆與 label 修復是前置條件

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：任務 M 的檔頭 (1)(3) 段 + 任務 O 的規則設計約束
- **現況**：`atlas_universe_*` 這族 counter 被自身累積值灌爆
  （`internal/monitoring/metrics/degraded.go` 的 `Counter.Inc/Add` 回呼傳的是**累積值**
  `c.Value()`，而 `internal/monitoring/metrics.go` 的 `RecordCounter` 是**累加**語意
  ⇒ 第 N 輪之後的輸出 = `x·N(N+1)/2`），而且 label 配對缺陷讓 `symbols_ranked_total` 無標籤
  （`cmd/atlas/main.go` 的 `onInc` 把標籤**值**清單兩兩配成 name=value）。
- **順序依賴（硬性）**：
  1. **counter 灌爆修正（任務 N）先落地** —— 否則任何絕對值門檻、任何 `increase()` 當「執行次數」
     的用法、以及**啟用持久化 collector**（見 FU-20260925-08）都會放大既有錯誤。
  2. **label 修復（任務 N）之後**才可以加 stage matcher（`stage="daily"`）——
     在現行「無標籤 series」上加 `{stage="daily"}` 會讓規則完全不 match，把偵測換成零偵測。
  3. 上述兩項都完成後，才輪到：`symbols_ranked_total` 的 stage 隔離（缺口 (e)）、
     `AtlasUniverseCoverageBelowFloor`（FU-20260925-03 第 1 項）、
     `AtlasUniverseRunsPerDayHigh`（同第 2 項）。
- **驗收條件**：任務 N 的 PR 內附「灌爆前/後同一天同一時段的數值對照」；label 修好後，
  `symbols_ranked_total` 帶 `stage` 標籤且第 1 條的 exp 在 promtool 測試中被更新為帶 matcher 的版本。

---

### FU-20260925-07 — metrics collector 是 in-memory（原 FU-A）：每次重啟 `/metrics` 的 app 指標歸零

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：`internal/bootstrap/bootstrapper.go` 的 `InitMetrics()`（→
  `monitoring.NewMetricsCollector()` → `NewMetricsCollectorWithPath("")`，`persistencePath = ""`
  ⇒ `replayFromFile` 不會被呼叫）；`cmd/atlas/main.go` 的 `collector := rt.MetricsCollector`；
  `/metrics` 由 `cmd/atlas/api_routes.go` 的 `monitoring.PrometheusHandler(collector)` 提供。
  另：series 是**第一次 Add 才建立**（`CounterVec.WithLabelValues` 只建內部物件，
  真正讓 family 出現在 `/metrics` 的是 `OnInc` → `RecordCounter`）。
- **後果**：
  - 每次重啟清空 `/metrics` 的 app 指標；**低頻 family（daily universe）最多缺 24 小時**
    （daily 是每個交易日 06:00Z；週五跑完才重啟 ⇒ 到下週二 = 95 小時；連假更久）。
  - **而且它看起來像 wiring 壞掉**：2026-09-25 已**實際造成一次誤判**（連 root 本人在內）。
- **證據**（2026-09-25 唯讀實證）：重啟後 `curl -s localhost:18080/metrics | grep -c atlas_universe`
  = **0**，而同一個 Prometheus TSDB 裡仍有 4 個 `atlas_universe_*` series（最後樣本 07:05Z）。
- **這正是任務 O 第 4 條加 peer gate 的原因**：裸 `absent()` 會在每一次健康的重啟後誤報一次。
- **最小修法建議**：讓服務的 collector 跨重啟保存（見 FU-20260925-08 的前置條件），
  或在驗收文件/腳本明確排除這個空窗（見下方「判讀註記」）。
- **驗收條件**：重啟後 `/metrics` 仍含 `atlas_universe_*`（family 集合與重啟前一致），
  且數值不會因一次重啟而整批消失。

---

### FU-20260925-08 — 啟用持久化 collector 的**前置條件**（原 FU-B，順序性最重要）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：`internal/monitoring/dashboard_api.go` 的 `newPersistedMetricsCollector`
  （寫 `data/state/metrics/metrics.jsonl`）與 `internal/monitoring/metrics.go` 的
  `applyRecord`（counter 分支是 `existing.Value += rec.Value`）。
- **風險（為什麼不能「順手打開」）**：`applyRecord` 對 counter 是**累加**，
  而 `RecordCounter` 也是累加，且回呼傳的是**累積值** ⇒ replay 會把整份歷史**再加一次**，
  之後每一輪又再疊一層。**未修 counter 灌爆之前啟用持久化 collector，數字會「每重啟一次再長一層」**
  —— 比現況（重啟歸零）更糟，因為它會讓「數值」看起來長期可信卻系統性偏大。
- **硬性前置條件**：**counter 灌爆修正（任務 N）先落地**。落地後才可：
  1. 讓 `cmd/atlas` 使用 `newPersistedMetricsCollector`（或等價的 replay 路徑）；
  2. 一併定義 replay 的語意（counter 用「覆蓋最後值」而非「累加」）。
- **驗收條件**：在 counter 修正已合併的前提下，連續兩次重啟後同一 family 的數值
  不再逐次膨脹，且重啟後 family 集合不消失。

---

### FU-20260925-09 — CLI 路徑不寫入服務 collector（原 FU-C）：手動驗證與監控觀測斷鏈

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**（child M，唯讀）：`-build-universe run` 走 `cmd/atlas/cmd_universe.go` 的
  `buildUniverseRun` —— 它**自建 deps、跑一次即結束**；而服務的 `/metrics`（`:18080`）只曝露
  **apiMode 區塊**（`cmd/atlas/main.go` 的 `metrics.NewUniverseMetrics()` + `SetOnInc`）。
- **後果**：**手動執行的結果只進 `data/state/universe_snapshot.json`，Prometheus 完全看不到。**
  2026-09-25 實例：手動觸發 07:31:50Z 產生 `ranked=150` / `quotes_returned=1301`（snapshot 有），
  但 Prometheus 對 `atlas_universe_*` 的**最後樣本仍是 07:09:19Z**（之後停更）
  ⇒ **手動驗證與監控觀測斷鏈**，並實際造成「三條規則 pending，但 snapshot 說成功」的落差。
- **為什麼要記**：這件事同時解釋了兩件事 ——
  ① 為什麼「snapshot 成功」不能推翻「Prometheus 沒動」；② 為什麼驗收要看 snapshot **與**
  指標兩邊。它也是任務 O 第 4 條 peer gate 的另一個理由（重啟空窗 + CLI 不寫入 = 指標面
  可以長期沒有 universe 資料，而系統其實健康）。
- **與 FU-20260925-08 的順序關係**：若要「統一寫入持久化 collector」來消除斷鏈，
  **仍須先修 counter 灌爆**（同 FU-20260925-08 的前置條件），否則 replay 會把 CLI 那一次的累積值
  也算進去。
- **最小修法建議**：讓 `buildUniverseRun` 的 deps 指向服務層共用（或持久化）的
  `UniverseMetrics` 實例；或在 CLI 結束時把結果明確寫成一個 gauge/series 供 Prometheus 讀。
- **驗收條件**：手動執行 `-build-universe run` 後，Prometheus 能看到對應的
  `atlas_universe_*` 樣本更新（時間戳前進），且 `universe_snapshot.json` 與指標敘事一致。

---

### FU-20260925-10 — 規則群 reload 後「首次評估」的取樣盲窗（樣本假象）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **來源**：Prometheus 行為（規則群首次評估發生在其 `interval` 之後）。
  本群 `atlas_universe_scoring` 的 `interval: 1m` ⇒ 剛 reload 後最大 **60 秒**盲窗；
  其他 atlas 群為 15–30 秒。
- **要記的事**：剛 reload 時讀到 `iterations_total = 0` / `lastEvaluation = 0` /
  `state = unknown` 是**預期**，**不是缺陷**、也不是「規則沒生效」。
- **目的**：避免未來的人（或 agent）再把這個**取樣假象**誤判成「規則沒被載入 / 規則壞了」，
  進而去「修」一個沒有壞的東西（2026-09-25 曾幾乎發生）。
- **驗收條件**：（無程式變更）未來判讀時能看到本條目；若要把盲窗縮到最小，
  可把本群的 `interval` 與其他群對齊（但會增加評估次數，本群視窗以天計 ⇒ 無實益）。

---

## 判讀註記（讀告警與做驗收前必讀）

以下三則不是待辦，而是**判讀規則**：已實際造成過一次誤判（含 root 本人），所以寫進登記表。

1. **部署後 app 指標 family 會消失，直到下一次排程執行** ⇒ 驗收時看到
   `curl -s localhost:18080/metrics | grep -c atlas_universe` 回 **0 不等於 wiring 壞掉**
   （原因與證據：FU-20260925-07）。判讀時要同時看 Prometheus TSDB 是否還留有舊樣本、
   以及 snapshot（`data/state/universe_snapshot.json`）。
2. **手動執行 `-build-universe run` 不會更新 Prometheus 的 `atlas_universe_*`**
   （原因：FU-20260925-09）⇒ 「手動跑成功」與「監控看到活動」是兩件事，不可互相證明。
3. **三條 universe 告警在 metric 缺陷修好前的預期分類**（2026-09-25 實證，07:31:50Z：
   `ranked=150`、`quotes_returned=1301`）：
   - `AtlasUniverseRankedZero` ＝ **真陽性**
   - `AtlasUniverseScreeningAllRejected` / `AtlasUniverseQuotesMissing` ＝
     **metric 缺陷期間的預期假陽性（unexpected-false-positive）**
   判讀時先確認 metric 面的缺陷狀態，再決定這三條是訊號還是假象；
   任務 O 新增的第 4/5 條同理（它們的 peer gate / 分工就是為了不製造新的假象）。

---

## 相關文件

- [universe-scoring-ranked-zero-20260925.md](universe-scoring-ranked-zero-20260925.md)
  — SmartUniverseBuilder `symbols_ranked=0` 根因報告（含 §6.2 缺口與 §8 防再犯檢查建議）
- [README.md](README.md) — 本目錄索引
