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
    **2026-09-25 更新：已由 #1989 修復**（回呼改傳真標籤 map ⇒ 單標籤 series 不再被丟掉，
    `symbols_ranked_total` 現在帶 `stage`）⇒ 缺口本身關閉。
    ⚠️ **但規則層的結論在過渡期內不變、只是理由換了**：TSDB 在 **≤ 6 天**內仍同時保有舊的
    **無標籤** series（`[6d]` 視窗的長度），期間加 `{stage="daily"}` 會讓舊 series 完全不
    match = 漏報 ⇒ 仍**不可**加 stage matcher；最早可加的時間見 FU-20260925-11。
  - **(f)** 共同後果：整組規則是「有產出但產出是空的」的偵測器，**不是**萬能心跳。
- **驗收條件**：（a）（b）各有一條新規則或其等價的 Go 端訊號；（d）-`empty_universe` 有一條
  Go 端心跳指標；（e）已由 #1989 修復，剩下的驗收是「過渡期結束後能把 daily/weekly 分開判定」
  （FU-20260925-11）。

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
  2. `AtlasUniverseRunsPerDayHigh`：**理由已於 2026-09-25 隨 #1989 改變**。原本的阻礙是
     「`increase()` 在灌爆的 counter 上不等於執行次數」（第 N 輪貢獻 N，不是 1），
     #1989 改傳 per-event delta 後該成因消失。**仍未落地**，現在的前置條件是：
     ① 要數「執行次數」必須先確定所選 series 在**每一次**執行（含失敗與 Step 1/2
     early-return）都恰好 +1 —— 這是 Go 層的性質，需要測試釘住，不是規則層能保證的；
     ② 單日門檻需要交易日/假日語意（與 FU-20260925-02 的缺口 (a) 同一張票）。
     或改用 `_last` / gauge 形態與專用 runs 計數器。
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
- **日曆更正（2026-09-26 記錄）**：**09-26 不可當成複驗日** —— 2026-09-26 是**週六**，且
  **2026 中秋＝09-25（週五，休市）** ⇒ 09-24（四）之後的下一個交易日就是 **09-28（一）**。
  權威判定（`internal/taiwanholidays`，以一次性探針 `go test ./internal/taiwanholidays/` 產出後**已刪除**）：
  `09-24 Thu true`／`09-25 Fri IsHoliday=true`／`09-26 Sat false`／`09-27 Sun false`／`09-28 Mon true`。
  ⇒ 本條的 `2026-09-28（週一）06:00Z` **維持有效**；且撰寫時（2026-09-26T03:40Z）該時刻**尚未到**
  ⇒ 狀態維持 `open`（本條**尚未**複驗，勿把 09-25/09-26 當成已複驗的日子）。
  （同族事實：`CHANGELOG.md:91`「2026 中秋＝09-25」。）
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

- **狀態**：`done`
- **記錄日期**：2026-09-25 ／ **完成於**：2026-09-25（#1989，任務 N）
- **來源**：任務 M 實證（規則檔檔頭 (2)）；`cmd/atlas` 內的註解寫「06:00 TW」，
  而容器**不設 `TZ`** ⇒ `alignToTarget` 的 06:00 實際是 **06:00 UTC = 14:00 台北**。
- **風險（當時）**：**註解是錯的，行為是對的** —— 這種不一致最容易讓人照註解去查錯時段
  （例如在台北時間 06:00 查「為什麼沒跑」，而排程根本還沒到）。
- **實際修法（#1989 選了更強的做法）**：不是改註解了事，而是把觸發時刻**釘死成瞬間** ——
  `alignToTarget` 改成以 `14:00 Asia/Taipei`（= 06:00 UTC）為目標並用瞬間比較
  （`universeLocation()`，載不到 IANA tzdata 時用固定 +08:00 後備），
  日/週兩個 task 的 weekday 判斷也改用同一個時區；`cmd/atlas` 的註解同步改成
  「14:00 TW = 06:00 UTC」。compose 仍**刻意不設** `TZ`（觸發已與它無關）。
- **驗收條件**：註解、觸發時刻、規則檔的視窗推導三者一致 ⇒ **已達成**：
  規則檔檔頭 (2) 段與第 4/5 條的註解都已改成「06:00 UTC(= 14:00 台北)、不隨環境 TZ 漂移」。

---

### FU-20260925-06 — 本包（任務 O）之後才能做的事：counter 灌爆與 label 修復是前置條件

- **狀態**：`open`（**前置條件已於 2026-09-25 由 #1989 滿足**，剩下的是後續項）
- **記錄日期**：2026-09-25 ／ **更新**：2026-09-25（#1989 進 main）
- **來源**：任務 M 的檔頭 (1)(3) 段 + 任務 O 的規則設計約束
- **原現況（歷史）**：`atlas_universe_*` 被自身累積值灌爆（回呼傳 `c.Value()` × `RecordCounter`
  累加 ⇒ `x·N(N+1)/2`），且 label 配對缺陷讓 `symbols_ranked_total` 無標籤。
- **#1989 已落地**：回呼改傳 **per-event delta**、標籤改傳**真名 map**
  （`internal/monitoring/metrics_bridge.go` 的 `CollectorOnInc`，兩處 wiring 共用）⇒ 兩個缺陷都關閉。
- **仍待辦（順序依賴仍在，只是換了內容）**：
  1. **別名不得提早移除**：TSDB 在 **≤ 6 天**內同時保有舊標籤形狀 ⇒ 第 2 條的
     `{daily=…}` / `{weekly=…}` union 在過渡期內**仍必要**（見 FU-20260925-11）。
  2. **stage matcher 仍不可提早加**：同樣是過渡期理由（舊**無標籤** series 仍在）
     ⇒ 最早在舊形狀離場後才可加（FU-20260925-11）。
  3. 之後才輪到：`symbols_ranked_total` 的 stage 隔離、`AtlasUniverseCoverageBelowFloor`
     （FU-20260925-03 第 1 項）、`AtlasUniverseRunsPerDayHigh`（同第 2 項）。
  4. **持久化 collector（FU-20260925-08）的硬性前置條件已解除**（灌爆已修），可視為可做的下一步。
- **本檔的事實陳述已同步**：`monitoring/rules/atlas_universe_scoring_alerts.yml` 的檔頭與
  第 1–5 條註解/annotations 已在同一個 PR（任務 O 的 re-sync）改成「歷史缺陷 + 過渡期注意」，
  expr 與 labels 一律未動。
- **驗收條件**：舊標籤形狀的最後樣本離開 6 天視窗後，（a）可評估移除別名、（b）可加 stage matcher；
  兩者都要附「過渡期結束」的證據（TSDB 查詢舊形狀回空）。

---

### FU-20260925-07 — metrics collector 是 in-memory（原 FU-A）：每次重啟 `/metrics` 的 app 指標歸零

- **狀態**：`done`（**2026-09-26 由 issue #1995 的修法關閉「family 消失」這一半**；
  「數值不跨重啟」仍 `open`，由 FU-20260925-08 的持久化 collector 追蹤）
- **記錄日期**：2026-09-25 ／ **完成於**：2026-09-26（issue #1995 修法）
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
  第三條路（**2026-09-26 實際採用**）：在啟動時就把要曝露的 family **主動建立** ——
  `UniverseMetrics.WarmUp()`（`internal/monitoring/metrics/universe.go`，由 `cmd/atlas/main.go`
  在 `um.SetOnInc(...)` 之後呼叫）以 `Add(0)` 建立整族 ⇒ 重啟後 `/metrics` 立刻有
  `atlas_universe_*`（值為 0），不必等下一次排程執行。**不改** collector 的持久化語意，
  因此沒有 FU-20260925-08 的 replay 風險。
- **驗收條件**：重啟後 `/metrics` 仍含 `atlas_universe_*`（family 集合與重啟前一致），
  且數值不會因一次重啟而整批消失。
  ⇒ **已達成（family 集合）**：重啟後 family 集合一致、值為 0；
  **未達成（數值連續性）**：counter 值仍歸零（Prometheus 對 counter reset 的處理是正確的，
  但「重啟前後數值連續」需要持久化 collector）⇒ 該半邊留在 FU-20260925-08。
  同一修法也讓任務 O 第 4 條的 peer gate 語意更乾淨（缺口只剩「真的被改名/移除」）。

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
- **2026-09-26 補充（issue #1995 調查）**：本條**未被** #1995 的修法關閉，而且又有一次實例 ——
  2026-09-25T17:01:40Z 的手動/部署後執行產生 `ranked=150` 的快照
  （`universe_snapshot.json` 的 `timestamp`），但 Prometheus 對 `atlas_universe_symbols_ranked_total`
  的樣本自 09-25T06:59Z 起就沒有更新（`increase(ranked[6d]) == 0`）。
  ⇒ 告警在「只有 CLI 路徑成功」的世界裡**會持續 firing 且是對的**（指標面確實 6 天沒有增量），
  但**快照是綠的** —— 判讀時必須分開看兩邊，不要用快照推翻指標。
  #1995 只讓 `atlas_universe_*` 的 family 一定存在（`WarmUp()`），不改變本條。
- **2026-09-26（本輪缺陷收斂批次複查；結論不變：仍 `open`）**：以 worktree HEAD `81e4fe62` 實測 ——
  CLI 路徑 `cmd/atlas/cmd_universe.go`（全檔 258 行）**0 命中** `NewUniverseMetrics` / `SetOnInc` / `UniverseMetrics`；
  唯一的服務端接線仍在 `cmd/atlas/main.go:2007-2017`（`metrics.NewUniverseMetrics()` →
  `um.SetOnInc(monitoring.CollectorOnInc(collector))` → `um.WarmUp()`）。
  ⇒ CLI 跑成功時，`atlas_universe_*` 中帶 `result="passed"` 的 series
  （`internal/monitoring/metrics/universe.go:93`、`:96`）**不會前進**：手動驗證與告警觀測仍分屬兩個世界。
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

### FU-20260925-11 — #1989 的**標籤過渡期**：舊形狀離場前，別名不可移除、stage matcher 不可加

- **狀態**：`open`
- **記錄日期**：2026-09-25（#1989 進 main 後建立）
- **來源**：`internal/monitoring/metrics_bridge.go` 的 `CollectorOnInc`（#1989）；
  `monitoring/rules/atlas_universe_scoring_alerts.yml` 檔頭 (3) 段與缺口 (e) 段
- **現況**：`/metrics` 現在曝露的是**真標籤名**（`{stage="daily",result="failed"}`、
  `{stage="daily"}`）；但 Prometheus 的 TSDB 對「舊形狀」的 series 不會立刻消失 ——
  舊形狀的最後一個樣本會留在視窗內，而本檔的視窗是 `[6d]`。
  ⇒ **過渡期 ≤ 6 天**內，兩組形狀**同時存在**（`{daily="failed"}` 與
  `{stage="daily",result="failed"}`）。
- **兩個「不可提早做」的動作（硬性）**：
  1. **不可移除別名**：第 2 條的 `{daily="passed"}` / `{weekly="passed"}` union 在過渡期內
     仍**必要**（不是「無害但冗餘」）。移除 ⇒ 舊 series 落在判定之外 ⇒ 漏報。
  2. **不可加 stage matcher**：第 1 條若加 `{stage="daily"}` ⇒ 舊的**無標籤** series 完全不
     match ⇒ 變成零偵測（2026-09-25 曾把這條寫成「缺陷修好前不可加」，現在的理由換成
     「過渡期內不可加」）。
  ⇒ 規則檔的檔頭與第 5 條註解都已改成這個**有時限的說法**（不是永久禁令）。
- **最早可評估的時間**：舊形狀的**最後一個樣本**離開 `[6d]` 視窗之後（本次部署在
  2026-09-25，估 ≈ **2026-10-01 之後**；以 TSDB 查詢舊形狀回空為準，不要憑日期猜）。
- **屆時要一起做的**：① 移除第 2 條的舊別名運算元；② 評估第 1 條加 `{stage="daily"}`
  以真正隔離 daily/weekly（缺口 (e) 的完整關閉）；③ 同步更新 promtool 測試的
  `exp_alerts`（expectation 是逐字比對）。
- **驗收條件**：`count({__name__=~"atlas_universe_.*",daily=~".+"})` 等舊形狀查詢回空，
  且移除別名 / 加 matcher 後 promtool 測試全綠、且用「合成舊形狀序列」證明移除後會漏報
  （負向對照）—— 換句話說：**先證明舊形狀真的沒了，才動規則**。

---

### FU-20260925-12 — 舊明文 DB 密碼已從現行檔案移除；**輪替問題經生產實測結案（不需輪替）**

- **狀態**：`done`
- **完成日**：2026-09-25
- **結案依據（生產實測；結論**推翻**原判斷）**：
  以 **SCRAM 認證路徑**實測（目標必須用**容器自身 IP** 才會命中 `pg_hba.conf` 最後一行
  `host all all all scram-sha-256`；用 `-h 127.0.0.1` 會命中前面的 **`trust`** 規則 ⇒
  **測什麼都成功**，是無效測試 ✗）：生產 DB **拒絕**該 18 字元文件值
  （錯誤為 `password authentication failed`，**不是**「角色不存在」），且**有效的負對照同樣被拒**
  ⇒ 測試本身有效；另該值引用的 dev 主機**不可達**（iMac 時代的位址）。
  ⇒ **該值在生產 DB 早已失效、且不是 prod 在用的憑證 ⇒ 不需輪替** ✓。
- **記錄日期**：2026-09-25
- **來源**：任務 Q（分支 `fix/secrets-and-monitoring-guard-q`）。
  **已移除明文的現行檔案**：`docs/operations/docker-compose.prod.yml`（`DATABASE_URL` / `POSTGRES_PASSWORD`）、
  `docs/operations/docker-compose.crons.yml`（`DATABASE_URL` ×4）、
  `.claude/skills/atlas-imac-prod-guard/SKILL.md`（復原 SOP 內的字面值）。
  改法＝一律從 compose 插值／env 檔取值；未提供值時 `${VAR:?...}` **直接讓 `docker compose` 失敗**（fail-closed），
  不會靜默用錯值。防再犯＝`scripts/secret-scan.sh` 新增 3 個通用憑證樣式 +
  「可部署設定檔（`*.yml`/`*.yaml`）不降級為 warn-only」，並由
  `scripts/ci/check_secrets.sh`（CI job `secret-scan`）與 `make ci-static` 把關。
- **為何仍需輪替（歷史推論；已被上方「結案依據」取代）**：舊值**已在 git 歷史中**
  （本 repo 為 PUBLIC ⇒ 永久可見）。「現行檔案不再含明文」**不等於**「憑證安全」；
  當初的推論是「輪替是唯一補救」——但生產實測顯示**該值已失效且非 prod 憑證** ⇒ 不需輪替。
  （此條保留為決策痕跡；方法論教訓見文末「已定案的判準」。）
- **仍含同一組明文的現行檔案**：`tasks/misleading-mechanisms-fix-plan-ds4pro-20260828.md`、
  `tasks/stockpicker-misleading-mechanisms-audit-k3-20260828.md`（各 1 行）
  ⇒ **已於本 repo PR #1996 一併遮罩** ✓（原記錄為「未動，超出授權」）。
- **附帶發現（a2a-dev，非本 repo）**：新的 DSN 樣式會在 a2a-dev 的舊 iMac runbook、稽核報告與
  治理報告等文件命中（warn-only，不擋 CI）⇒ **已於 a2a-dev PR #123 清除** ✓（5 檔 8 處，含
  `secret-scan.sh` 樣式說明註解內的字面值）；唯一**刻意保留**者是 Jev eval 的 baseline 資料
  ⇒ a2a-dev PR #124 以**具名 allowlist 條目 + 決策紀錄**登錄（理由：改字元會動到實驗基準）。
- **驗收條件（已滿足）**：業主／root 以**生產實測**回覆「該值已失效、不需輪替」並補記於此；
  `tasks/*.md` 的處置已完成（#1996 遮罩）✓。

### FU-20260925-13 — 生產 DB 仍跑在 **compose 預設密碼**（字典詞）上（非緊急；需維護窗）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **事實（生產實測，2026-09-25）**：
  - 生產 DB 的**實際**密碼＝compose 的 **fallback 預設值**（5 字元、**字典詞**）。
    **值不在此記載**；來源即 live `docker-compose.yml` 的 `${DB_PASSWORD:-…}` 預設
    （`docs/operations/docker-compose.prod.yml` 為同一份設定）。
  - app 容器 DSN 內嵌值 **== 該實際值** ⇒ app 連線正常（這也是它一直未被發現的原因）。
- **緩解現況（是緩解，不是修好）**：
  - ~~DB 埠以 `0.0.0.0:55432` 對外發佈，但 root 實測 **LAN 不可達** ⇒ 目前外部打不到。~~
    **更正（2026-09-26）：上述「緩解」已失效且敘述錯誤** —— `docker port atlas-postgres` 實測
    `5432/tcp -> 0.0.0.0:55432`（＋`[::]:55432`）；root 以 Tailscale（`kmacmini` = 100.65.194.77）
    實測 **55432 可達**，且對 DB 做認證：**負對照**（明知錯誤的值）被拒 ✓、真實值（= compose 預設字典詞）
    可登入 ⚠️ ⇒ 「遠端免認證可達」＋「loopback 免密」的組合是真的存在過的。
    修法見 PR #2004（`docker-compose.yml` 的 postgres 埠發佈改為 `127.0.0.1:${ATLAS_POSTGRES_PORT:-5432}:5432`，
    **尚未部署**；in-stack 的 atlas／6 個 cron／postgres-exporter 都走 docker 網路名 `postgres:5432`，不受影響）。
    與本條的密碼輪替**互補而非互斥**：綁 loopback 讓「遠端整類」消失，但 `pg_hba` 對 `127.0.0.1/32` 是 `trust`
    ⇒ 輪替防不到本機路徑；反之輪替也不能取代 bind。
  - `pg_hba.conf` 對 `127.0.0.1/32` 與 `::1/128` 是 **`trust`** ⇒ **本機存取免密**。
    ⚠️ 這既是緩解也是**陷阱**：在容器內以 `-h 127.0.0.1` 測密碼**一律成功**（見文末判準）。
- **風險**：只要網路曝露面改變（埠改 bind、加入新網段、SSH tunnel、其他容器同網段），
  一組**字典詞**密碼即可被猜；而 `trust` 讓「能連到本機」等同「已通過認證」。
- **建議計畫（未實作；需維護窗、非緊急）**：
  1. `ALTER USER atlas PASSWORD '<強值>'`
  2. 同步 `.env` 的 `DATABASE_URL` 與 `DB_PASSWORD`（值一律不進版控）
  3. 重建受影響容器並以 `docker inspect` 驗 DSN
  4. 更新 DB 容器的 `POSTGRES_PASSWORD`（僅影響下次 initdb；實際以 `ALTER USER` 為準）
- **驗收條件**：以**容器自身 IP**（SCRAM 路徑）實測新值可登入、舊的預設值**被拒**，
  且負對照（明知錯誤的密碼）同樣被拒。

---

### FU-20260925-14 — `check_postgres` 的憑證來源應硬化（避免「過期值靜默生效」）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **事實**：`cmd/atlas/check_postgres.go:274` 以 `config.GetSecret("DB_PASSWORD")` 直接組出
  `DB_PASSWORD=<值>` 傳給 `docker compose`；而該鍵在 `~/.config/atlas-go/.env` 曾是**過期值**
  （生產實測：DB 以 SCRAM 路徑**拒絕**它）。
- **今日處置（root，2026-09-25）**：已自 `~/.config/atlas-go/.env` **移除該過期行**
  （備份 `~/.config/atlas-go/.env.bak-dbpassword-20260925-174749`、權限 600、回退＝一行指令）。
  移除後該工具會落到 compose 的 `${DB_PASSWORD:-…}`（`:-` 對「未設」與「空」皆生效）
  ⇒ **現在的行為反而正確** ✓。
- **為何要硬化**：問題不在「值錯了」，而在**取用方式**——工具直接信任一個**可能過期**的鍵，
  而且失敗形態是「用了錯的值」而不是「沒有值」（後者至少會吵）。
- **建議（未實作）**：
  1. 優先**從 `DATABASE_URL` 解析**（單一來源；DSN 才是 runbook 與 `.env` 的權威）；
  2. 或取用前**斷言非空且非已知過期值**，並在 log 明示來源檔與鍵名（可稽核）。
- **驗收條件**：用「故意放一組過期值」的 fixture 驗證：工具必須**失敗或明確警告**，
  不得靜默帶著錯值繼續（同族：FU-20260925-01 的「回報有東西，而不是東西對不對」）。

### FU-20260926-09 — `FU-20260926-01` 的實作／驗收記錄（latch＋持久化＋token 遮蔽）

- **定位（root 2026-09-26 定案）**：本條**不是**與 `FU-20260926-01` 平行的待辦 ⇒
  **`-01` 是待辦（已 `done`，由 PR #2006 實作）**、**本條是它的實作／驗收記錄**。
  記錄本身隨 PR #2006 交付；下方「殘留面」三項中 **殘留 1 已於 #2014 完成**，殘留 2／3 仍是後續
  硬化方向，故本條狀態維持 `open`。
- **狀態**：`open`（僅指殘留 2／3；實作／驗收部分與殘留 1 已完成）
- **記錄日期**：2026-09-26
- **編號說明（本條改號兩次）**：本條隨 PR #2006 建立，原號 `FU-20260926-01`；
  **第一次撞號** = [#2005](https://github.com/kaecer68/atlas-go/pull/2005) 併入 main 的
  `FU-20260926-01..07`（追平 main `788045dd` 時改為 `-08`）；**第二次撞號** =
  [#2008](https://github.com/kaecer68/atlas-go/pull/2008) 併入 main 的同號 `-08`
  （追平 main `b5fca3a5` 時改為本號 `-09`）。root 已定案本號（不再改動）。
- **編號現況**：`grep -o "^### FU-[0-9-]*" docs/operations/FOLLOWUPS.md | sort | uniq -d` 必須為空
  （追平 main 後已驗；見 PR #2006 說明）。
- **實作範圍（= `FU-20260926-01` 的 ① ② ＋ 驗收條件；並涵蓋 `FU-20260926-02` 的 token 洩漏）**：
  `fetchDataset`／`FetchTaiwan5SecIndex` 收到 402（或 body/msg 含 `upper limit`）⇒ latch 當日額度耗盡
  並持久化 ⇒ 之後本地短路、不發 HTTP、跨日解除；`Remaining()` 期間回 0 ⇒ 所有保留水位立即停止（重啟亦然）；
  ceiling 12000 + `FINMIND_DAILY_LIMIT` + `marketdata.FinMindDailyLimit()` 單一讀取點；
  上游 body 一律經 `sanitizeFinMindBody()`（`token_tail` 等欄位整段刪除）。
  證據（測試、負對照、exit code）見 PR #2006 說明。
- **來源**：`fix/finmind-quota-honor-402-r`（FinMind 402 ⇒ 當日額度 latch 持久化 + ceiling 14400→12000
  + upstream body 去機密）。生產事實：2026-09-26 02:10Z `auto_quote_backfill`（824 檔）跑到
  `calls_today≈12500` 時上游回 402 `Requests reach the upper limit`（且該 body 回帶 `token_tail`）。
- **殘留 1（跨行程可見性）— `done`**（2026-09-26，issue [#2014](https://github.com/kaecer68/atlas-go/issues/2014)，
  PR [#2021](https://github.com/kaecer68/atlas-go/pull/2021)、branch `fix/20260926-finmind-quota-cross-process`）：原狀是 latch 寫在
  `data/state/finmind_daily_quota.json` 卻**只在 client 建構時讀取** ⇒ 同機其他行程（另一顆 cron 容器、
  或長命 process 內另一份 client）不會立刻看到別人的 latch，要等它自己撞一次 402 才跟上。
  **已改為**：`DailyQuotaTracker` 的每一次讀與每一次遞增都在 `<state>.lock` 的 flock 之下進行
  read-modify-write，latch／計數／`Remaining()` 一律以 state file 為權威 ⇒ 一個行程 latch 後，
  其他行程在下一次呼叫即可見（不再需要「短 TTL 重讀」這種折衷）。同時修掉同源的更嚴重缺陷：
  跨 process 的**上限**原本只是「每個 process 各自的上限」。**邊界（誠實）**：這只保證共用同一
  state dir 的行程（同機同 volume）；另一台機器各自的 state dir 不受此鎖約束。
- **殘留 2（上游真實上限未知）**：12,500 是**觀測到的拒絕點**，不是 FinMind 公布的上限；
  12000 這個 ceiling 是人工留 500 餘裕的估計值。目前以 `FINMIND_DAILY_LIMIT` 覆寫 +
  `finmindObservedUpstreamRefusalLimit` 常數 + 測試（`TestFinMindQuotaCeiling_StaysBelowObservedUpstreamRefusal`）
  把「不得超過觀測拒絕點」寫死，但**沒有自動校準**。
  **硬化方向**：連續多日記錄「首次 402 時的 calls_today」並回報，作為下一次調整 ceiling 的證據。
  **2026-09-26 新增反證（未收斂，僅登記）**：`#2014` 盤查時發現，本地計數器停在 `calls_today=12982`
  的那一秒（2026-09-26T14:20:15+08:00）**同目錄的 `data/state/tsmc_revenue/11509_revenue.json` 也被寫入**，
  而該檔只在 `TSMCRevenueProvider.FetchSnapshot` 的**成功**路徑才寫（失敗走 cache fallback、不寫檔）
  ⇒ 那一天「本地 12,982」時上游仍正常回應；同日 04:05Z–06:05Z 也有 `no data for symbol`（HTTP 200 但空）
  而非 402。因此 **12,500 是否真是「日配額牆」存疑**——本地 counter 記的是**嘗試**（402 被拒也 +1，
  而 `fetchWithRetry` 的重試不 +1），與上游自己的用量本來就不是同一個數。**尚未否證/證實**：
  02:10Z 那次的 response body 已隨舊容器消失，唯讀手段取不到 ⇒ 需要 FinMind 後台用量或一次受控實驗。
  **在收斂前不要據此調升 ceiling**（維持 12000 與 `FinMindDailyLimit()` 單一讀取點）。
- **殘留 3（探針語意）**：latch 期間 `QuotaRemaining()==0`，`channel_health_finmind` 因此仍以
  一般 quota 訊息呈現；「本地自己停手」與「上游已宣告今日結束」目前只靠錯誤字串
  （`upstream-exhausted … reason=…`）區分。
  **硬化方向**：把 latch 狀態（bool + observed_at）納入 channel-health 記錄/指標，
  讓儀表板不必解析錯誤字串。

---

### FU-20260926-01 — FinMind 上游 402 早於本地護欄：`finmindDailyLimit=14400` 與 1,500 保留值都太高

- **狀態**：`done`
- **記錄日期**：2026-09-26
- **完成於**：2026-09-26，**已由 #2006 的 latch 實作**（PR [#2006](https://github.com/kaecer68/atlas-go/pull/2006)
  `fix/finmind-quota-honor-402-r`，head `b74d8692`；**未 merge、未部署**——狀態的最終確認在部署驗收後）。
  實作／驗收記錄見 `FU-20260926-09`（本條是待辦、那條是證據，不是兩條平行待辦）。
- **實作對照本條建議**：
  - ①「以上游 402 為準」⇒ `DailyQuotaTracker` upstream latch：402（或 body/msg 含 `upper limit`）
    ⇒ 標記當日耗盡＋**持久化**（`data/state/finmind_daily_quota.json` 的
    `upstream_exhausted` / `upstream_reason` / `upstream_at`）⇒ 之後所有呼叫在**本地短路、不發 HTTP**，
    跨日才解除；`Remaining()` 期間回 0 ⇒ backfill 1,500 與 sbl/tdcc 500 保留水位立即停止（**重啟也一樣**）。
  - ②「14400／1500 改為可設定並印出來源」⇒ ceiling **12000**（新增觀測常數
    `finmindObservedUpstreamRefusalLimit = 12500`，ceiling 嚴格在其下、留 500 餘裕）＋ `FINMIND_DAILY_LIMIT`
    覆寫 ＋ `marketdata.FinMindDailyLimit()` 跨套件單一讀取點（原 3 份複製的 14400 已收斂）；
    覆寫高於觀測拒絕點會記 WARN。
  - ③「402 時記一筆量測供校準」⇒ latch 持久化 `upstream_at` ＋ `upstream_reason`（含上游 status），
    但**自動校準仍未做** ⇒ 見 `FU-20260926-09` 殘留 2。
- **驗收條件對照**：①「上游 402 之後，backfill 當日不再發送且 log 明示 `upstream 402 at used=N`」⇒
  錯誤字串為 `finmind: daily quota exhausted (upstream-exhausted, used=N, remaining=0, observed_at=…,
  reason=upstream HTTP 402: {…})`，且測試以 **upstream hit count** 證明不再發送（402 後再打 20 次，
  上游總 hit 數仍為 1）✓；② 負對照（拿掉短路）⇒ 測試 FAILED：
  `21 upstream requests escaped the latch; want 1 (the single 402)` ✓。
- **事實（root 生產實測，2026-09-26）**：`auto_quote_backfill` 於 02:10Z 啟動、載入 824 symbols；
  配額計數器 01:0xZ ≈ 35 → 02:1xZ ≈ **12,500**，此時**上游已回 402**（`Requests reach the upper limit`）。
- **為何護欄沒擋住**：本地兩道門檻都在真實上限**之上** ——
  - `internal/marketdata/finmind_client.go:83` `const finmindDailyLimit = 14400`（同檔註解自述為
    「observed upstream daily quota cap (used=14400, remaining=0 exhaustion on 2026-09-02)」）；
  - `internal/monitoring/quote_backfill_task.go:70` `const backfillQuotaStopRemaining = 1500`
    ⇒ 早停點 = 已用 **12,900**（14400 − 1500），**晚於**上游實際 402（≈12,500）
    ⇒ #1954 的保留值停損**永遠不會觸發**（該設計的前提「真上限 = 14,400」不成立）。
  - 同一段註解也寫著該帳號是**贊助方案（active until ~2026-10-03）**、小時預算 6000/hr
    ⇒ 常數與真上限的關係**隨方案變動**，硬編值必然過時。
- **建議（未實作）**：① **以上游 402 為準** —— `ErrQuotaExhausted` 應能「標記當日耗盡 + 當日不再嘗試 + 退避」，
  不靠本地 counter 猜；② 14400／1500 改為可設定並在 log 印出「本次用的上限值與來源」；
  ③ 402 時記一筆「上游回報的上限」量測，供下次校準常數。
- **驗收條件**：上游 402 之後，backfill **當日不再發送**且 log 明示 `upstream 402 at used=N`；
  負對照：本地 counter 未到門檻時，也不得在 402 之後繼續重試同一 dataset。

---

### FU-20260926-02 — FinMind 錯誤回應回帶 `token_tail`，而 log 與錯誤字串**原文照抄**整段 body

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **事實（程式碼）**：`internal/marketdata/finmind_client.go:373-381` 在非 200 時讀 body（上限 512B）並
  `logging.Warn("finmind", "fetch_non_2xx", "body", bodyStr, …)` **原文進 log**；
  `:395` 再把同一段 body 併進錯誤字串（`fmt.Errorf("finmind: %w: %s", ErrQuotaExhausted, bodyStr)`）
  ⇒ 一路帶進 channel health／BTM task 的錯誤訊息。
- **為何是機密問題**：FinMind 的錯誤 envelope **會回帶 token 尾碼**（測試 fixture 早已記載此形狀：
  `internal/marketdata/finmind_client_extra_test.go:924-929`
  `{"msg":"Token is illegal.","status":400,"token_tail":"...stale-key"}`）；root 在生產 402 回應上**實測看到 `token_tail`**。
  尾碼可利用性低（本 repo 為 PUBLIC，log 也可能被貼進 PR／issue），但它是**機密片段**，不應出現在 log／錯誤訊息／貼文。
- **建議（未實作）**：① 寫 log 與組錯誤字串之前**遮蔽已知敏感鍵**（至少 `token_tail`；用白名單比黑名單安全）；
  ② 保留可診斷性：遮蔽後仍要能分辨「token 失效」與「配額耗盡」（例：只留尾 2 碼）；
  ③ 業主**評估輪替 FinMind token**（非緊急）。
- **驗收條件**：以含 `token_tail` 的 fixture 驅動 fetch ⇒ log 與 error **皆不含**原值；
  負對照：`msg`／`status` 仍完整可見（不得為了遮蔽而讓錯誤不可診斷）。

---

### FU-20260926-03 — `seasonal_calibration` **有排程且真的在生產跑**；但校準結果寫進**容器內** `/app/configs`（不回流版控、重建即失）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **事實（生產實測）**：
  - 排程**存在**：`cmd/atlas/data_sync_health_tasks.go:152-176` 註冊 `seasonal_calibration`
    （`Interval: scheduler.SeasonalCalibrationDefaults.Interval`，7d）。前置條件是
    ① image 內有 `calibrate-seasonal` ② `data/replay/finmind_2020_2024.jsonl` 存在 —— **兩者都成立**
    （`/app/calibrate-seasonal`、host 與容器皆有 `data/replay/finmind_2020_2024.jsonl`）。生產 log：
    `2026-09-26T02:05:12.772Z registered seasonal_calibration background task (7d interval)`、
    `02:06:58.816Z task_started name=seasonal_calibration`、`02:06:58.942Z exec_ok binary=/app/calibrate-seasonal output_len=4024`。
  - 傳給二進位的參數由 `internal/scheduler/seasonal_task.go:84-95` 決定：**固定 `-update`**，有 replay 時再加 `--replay <path>`。
  - **寫入位置是容器內**：`docker inspect atlas-go` 的 Mounts 只有 `data`／`reports`／`logs`（`configs` **不在**其中），
    `docker diff atlas-go` 顯示 `C /app/configs/parameters.json` 與 `A /app/configs/parameters.json.snapshot.bak`
    ⇒ 校準只改**容器可寫層**：容器一重建就回到 image 的值（本日 02:04Z 就重建過一次），**永不回流版控**。
  - 反面：**在 host clone 手動跑** `-update` 才會寫 tracked 檔
    （`cmd/calibrate-seasonal/main.go:63`；目標是 `constants.ParametersFile` = `configs/parameters.json`）
    ⇒ 弄髒 clone、擋 `git pull --ff-only`。**該禁令只適用 host CLI，不是排程**
    （實測 host `git status --porcelain` 只有 `?? finmind_daily_quota.json`，未被排程弄髒 ✓）。
- **資料品質前提（root 實測）**：synthetic 與真實 replay 差異大
  （synthetic `{overstated:11,understated:2}` vs real `{validated:6,overstated:6,understated:1}`）
  ⇒ 用 synthetic 判定等於寫錯 11 條；`-update` 沒有 `--replay` 會被**硬拒**（護欄存在 ✓）。
- **建議（未實作）**：① 選定**單一條正規流程**：開發端 `--update -replay data/replay/merged.csv` → commit → PR → 部署
  （可考慮 CI 定期開 PR），或把容器 `/app/configs` 改成**有回流路徑**的機制；
  ② 不論選哪條，都要留下「本次改了哪些係數、值從哪來、誰審過」的痕跡（目前完全沒有）。
- **驗收條件**：任一輪校準都能回答上述三問；且生產不會出現「跑過但沒有任何痕跡」。

---

### FU-20260926-04 — 季節調整係數超界（4 個 pattern；消費端已夾，**版控設定檔本身**仍是壞的）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **事實（生產 log，2026-09-26T02:04:03Z 與 02:04:24Z 各一次）**：
  `seasonal_adjustment_factor_out_of_range bound_low=0.30 bound_high=2.50
  patterns="dividend_season=-0.2634,summer_electricity=-0.2883,ai_infrastructure_buildout=2.5555,year_end_positioning=3.4414"
  action=clamped_at_consumption`
- **來源是版控值，不是執行期產物**：`configs/parameters.json` 的 `industry.seasonal_patterns.value`
  實際值為 `dividend_season=-0.2634376289519962`、`summer_electricity=-0.2883015395477875`、
  `ai_infrastructure_buildout=2.555461115152644`、`year_end_positioning=3.44143401207912`
  （與 WARN 的 4 個 offender 完全對應）⇒ **版控的設定檔就是超界的**，不是某次校準才寫壞。
- **語意**：`internal/industry/seasonality.go:563-597`（I17／issue #1944）—— 超界值在**消費端被夾**
  （`ClampAdjustmentFactor`），所以不會直接算錯，但設定檔仍是壞的；該 warn 是**刻意設計的機讀痕跡**。
- **風險**：warn 只在**啟動時**出現（`NewSeasonalEngineFromConfig`）⇒ 不會持續告警；
  被夾幅度不小（例 `year_end_positioning=3.4414` → 2.50），而「被夾」本身沒有任何持久化痕跡。
- **建議（未實作）**：① 4 個 pattern 逐一定調（是校準輸入壞、還是 bound 太窄？）；
  ② 把「被夾」寫進 startup 以外的痕跡（config 檢查腳本或 metrics），不要只在 log 一閃。
- **驗收條件**：`configs/parameters.json` 內所有 `adjustment_factor` 落在 0.30–2.50；
  或放寬 bound 時附明確理由與紀錄。

---

### FU-20260926-05 — sector 預測**未持久化**（`persisted:false` + 具名 reason）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **事實（生產實測，2026-09-26）**：`GET /api/events/prediction` 回應內
  `sector_prediction_status = {"enabled":true,"applied":true,"days":5,"sector_rows":100,
  "strategic_prior_applied":false,"cycle_provider_wired":true,"persisted":false,
  "persistence_reason":"ledger_event_flow_prediction_record_has_no_sector_column"}`。
- **程式碼對應**：`internal/eventdriven/types.go:107-108` 定義該 reason 常數；`internal/eventdriven/handler.go:278`
  在寫入時填入；`types.go:162` 是 `Persisted bool \`json:"persisted"\`` 欄位。
- **影響**：預測**算得出來、也套用了**（`applied:true`, `cycle_provider_wired:true`），但**沒有落盤** ⇒
  事後無法重建「當時預測了什麼」，與「回測／歸因需要當時的預測」直接衝突（同族：FU-20260925-07／08 的
  「算得出來但看不到」）。
- **建議（未修）**：ledger 記錄需要一個 sector 維度欄位（或另存一份帶 sector 的列），
  否則任何「預測 vs 實際」的對帳都只能靠 log。
- **驗收條件**：同一支 API 回 `persisted:true`（且 DB 內查得到該筆），負對照：`persistence_reason` 不再是
  `..._has_no_sector_column`。

---

### FU-20260926-06 — industry／cycle 狀態的 **live 讀取配方**（供驗收；含「此端點唯讀免 token」的實測）

- **狀態**：`open`（不是待辦，是**驗收配方**登記）
- **記錄日期**：2026-09-26
- **配方（生產實測，2026-09-26，唯讀）**：
  ```
  docker exec atlas-go sh -c 'curl -s -o /tmp/pred.json -w "%{http_code}\n" \
      -H "Authorization: Bearer $ATLAS_API_KEY" http://127.0.0.1:18080/api/events/prediction'
  # → HTTP 200；token 留在容器內用，不落地、不列印
  ```
  要看的欄位：`sector_prediction_status.cycle_provider_wired`（本次 `true`）、
  `.applied`、`.persisted`／`.persistence_reason`（見 FU-20260926-05）。
- **同時記錄的事實（避免下次誤判 auth）**：`/api/events/` 屬 **public-read 白名單**
  （`internal/monitoring/api/shared/authlist.go` 的 `AuthFreeExactPaths`／`AuthFreePrefixPaths`）
  ⇒ **唯讀 GET/HEAD/OPTIONS 不需要 token**（實測：**不帶** `Authorization` 也回 200）；
  POST/PUT/DELETE/PATCH 仍要 API key。⇒ 驗收時「沒帶 token 也 200」**不是** auth 失效。
- **驗收條件**：任何「industry／cycle 有沒有接線」的結論，都要附上述指令的原始回應欄位（不得只憑推論）。

---

### FU-20260926-07 — 生產**實際生效**的 `/app/configs/parameters.json` 與版控那份**不同**（差 61 個葉節點、16 個值）；**寫入者已定位＝應用自身 calibration 任務**（runtime 自適應、不回流版控、重建即失）

- **狀態**：`in-progress`（**寫入者已定位（2026-09-26）；守門 ＋ 持久化 ＋ 可見性已實作（本 PR），待合併與部署後才 `done`**）
- **記錄日期**：2026-09-26
- **事實（生產唯讀實測，2026-09-26）**：
  - **image 內的是對的**：以 `docker create atlas-atlas`（**不啟動**）+ `docker cp` 取出
    `/app/configs/parameters.json` ⇒ sha256 `b203485f…`，與 host repo `configs/parameters.json` **byte-identical**、
    JSON 相等（35 個頂層鍵）。
  - **running 容器內的不是那份**：`docker exec … sha256sum /app/configs/parameters.json` ⇒ `2c939dd0…`；
    其 `updated_at = 2026-09-26T03:08:44.90062791Z`，而容器 `Created = 2026-09-26T02:04:02Z`
    （image built `02:03:58Z`，`/api/version` 回 `commit 7b4451bd…`）⇒ 該檔在**啟動後被改寫**。
  - 差異（以 JSON 樹比）：**葉節點 4,841 vs 4,780（生產多 61）**、**16 個值不同**，例如
    `risk/max_daily_loss_pct.value` `0.03 → 0.0108`、`risk/max_position_size.value` `0.15 → 0.054`
    （兩者 `last_calibrated` 都是 `2026-09-26T03:08:44Z`）、`industry/cycle_calibration/value/*` 由 `0` 變成
    一組具體值（10／0.05／0.55／0.45／0.05／0.4／30）；多出來的多屬「程式碼預設值具象化」。
  - 容器內同目錄另有 `parameters.json.snapshot.bak`（`docker diff` 顯示 `C /app/configs`、`C …/parameters.json`、
    `A …/parameters.json.snapshot.bak`），而 `configs` **不在** bind mount（只有 `data`／`reports`／`logs`）。
### 寫入者鑑定（2026-09-26 更新：**已定位**）
**寫入者 ＝ 應用自身的 calibration 背景任務群**（`cmd/atlas/calibration_tasks.go`；同檔自述 18 個任務＝1 inner ＋ 17 top-level），屬 **by design 的 runtime 自適應校準**，**不是隨機 bug**：
- `cmd/atlas/calibration_tasks.go:165` `configPath := filepath.Join(d.Cfg.WorkDir, "configs", "parameters.json")`
  → `auto_threshold_calibrate`（每月 1 日 03:00 台北）走 `industry.RecalibrateThresholds(revenuePath, configPath)`。
- `cmd/atlas/calibration_tasks.go:265` `risk_gate_calibrate`（24h）→ `d.RiskGate.SelfCalibrate(...)` →
  `internal/risk/self_calibrate.go:197` `config.GetParametersConfig().LockedSaveWithRollback(config.GetParametersConfigPath())`
  ⇒ **這就是觀測到的 `risk/*` 值在 `2026-09-26T03:08:44Z` 被改寫的路徑**（與檔內 `last_calibrated` 時間戳一致）。
- 檔案寫入本體：`internal/config/parameters.go:2293 Save` / `:2419 LockedSaveWithRollback`（tmp 檔 ＋ rename，並產生
  `parameters.json.snapshot.bak` ⇒ 與 `docker diff` 觀察到的 `A …snapshot.bak` 一致）；
  路徑權威 = `internal/config/parameters.go:2183 GetParametersConfigPath()`。

### 真正的缺口（不是「誰寫的」，而是「寫完之後」）
1. **不回流版控**：寫入發生在容器可寫層（`configs` 不在 bind mount）⇒ git 永遠看不到。
2. **容器重建即失**：每次部署（image 重build ＋ 容器重建）都回到 image 內的值。
   **反差樣本（2026-09-26 實測）**：`risk/max_daily_loss_pct` 容器 `0.0108` vs repo `0.03`、
   `risk/max_position_size` 容器 `0.054` vs repo `0.15` ⇒ **重建後風控會「放寬」回 repo 值**（不是收緊）。
- ⚠️ **待業主確認是否 intended**：若「runtime 自適應校準」是預期行為，則 repo 值只是**出廠預設**，
  但必須留下「誰在何時把哪個參數改成什麼」的痕跡；若不是，則寫入需回流或停用。
- **建議（未做）**：① 先做政策決定（回流版控 vs 明確宣告 runtime-only）；② 不論選哪條，都加**啟動時的漂移偵測**
  （比對 image 內那份與 effective 那份，不同就告警）並在 runbook 標明「生產有效值 ≠ repo 值」；
  ③ 保留可稽核的寫入痕跡（目前只有 `last_calibrated` 時間戳）。
- **驗收條件**：能回答「誰在何時寫了這個檔的哪些鍵」＋漂移有顯性痕跡（負對照：不得只靠「檔案看起來正常」判定）。

### 修復（2026-09-26，本 PR：`fix/calibration-drift-floor-and-overlay`；**尚未合併／部署**）

真根因有**兩層**，修法順序不可顛倒（先擋漂移，否則持久化只會把漂移值鎖住）：

1. **漂移本身被現行守門允許** ✗ — `internal/risk/self_calibrate.go` 的相對窗
   `[current*0.3, current*3.0]` 是**每輪速率限制**而非守門：每輪縮 ≤3× 永遠在窗內，累積就無限下行；
   共享絕對下限 `0.005` 只擋住終點、擋不住過程（`0.0108 > 0.005`）。
   ⇒ 改為**逐參數 sanity floor**（`calibrationSanityFloor`）：
   `risk_max_position_size 0.12`（= repo 內已文件化的最保守持倉比例
   `engine.strategy_evolution.configs.value.cautious.max_position_size`）、
   `risk_max_daily_loss_pct 0.03`（= SSOT 值本身，"3% max daily loss"）。
   低於 floor 一律拒絕；**已在 floor 之下**的既有值（舊版寫入的部署）走 recovery：
   接受 `[floor, floor*3]` 讓它一輪爬回 sane 值（不凍結在漂移值）。
   拒絕不再只是 `fmt.Printf`，而是 `report.Rejected` ＋ 結構化 WARN。
2. **持久化與可見性** ✗ — 校準值原本寫回 `configs/parameters.json`（**不在 bind mount**）。
   ⇒ 改寫到 **`data/state/parameters.calibrated.json`**（= `constants.StateParametersCalibrated`，
   在 `${WORKDIR}/data` 這個**已掛載**的樹內），`configs/parameters.json` **保持 pristine 作為 SSOT**；
   啟動時由 `config.ApplyCalibratedOverlayLayer` 疊在 SSOT 之上（`config.GetParametersConfig` /
   `ReloadParametersConfig` / `cmd/atlas/main.go` / `parameters` API handler 皆走
   `LoadEffectiveParametersConfig`）。
   - 可見性：每個套用項以結構化 log 同時輸出 `ssot` 與 `effective`（＋ `ratio`）；`ratio` 落在
     單輪窗 `[0.3x, 3x]` 之外 ⇒ 額外 WARN。被 floor 拒絕的提案走 `report.Rejected`。
   - **特例（刻意的 fail-closed）**：overlay 條目記錄它疊在哪個 SSOT 值上；若 SSOT 值後來被改
     （charter 編輯）⇒ 該條目**失效並移除** ＋ WARN，人工審查過的 charter 永遠優先於過期的 runtime 適應。
   - **風險（已在 PR 說明）**：校準值變成「跨容器重建存活」（這正是本條要的），
     因此以前「重建即重置回 repo 值」的意外剎車消失；漂移的上限現在由 (1) 的 floor 承擔。
3. **仍未涵蓋（後續）**：其他校準器仍寫 `configs/parameters.json`
   （`internal/config/calibrator.go:200`、`internal/portfolio/factor_weight_calibrator.go:148`、
   `internal/orchestrator/calibration_engine.go:330`、`cmd/atlas/calibration_tasks.go:165` 的
   `industry.RecalibrateThresholds`）；它們的寫入**一樣**在容器重建時遺失、且對 git 不可見。
   本 PR 只遷移 `risk_gate_calibrate → risk.SelfCalibrate` 這條（觀測到漂移的那條）。

---

### FU-20260926-08 — `ATLAS_BROKER_NONCE_REDIS_URL` 與「已改綁 loopback 的 16379」之間的**潛在**依賴（目前惰性）

- **狀態**：`open`（**目前不生效**；任何人要開啟 Redis nonce store 前**先讀本條**）
- **記錄日期**：2026-09-26
- **事實**：`docker-compose.yml` 的 `redis` 埠已改綁 `127.0.0.1:16379:6379`（PR #2004）；同檔 atlas service 仍保有
  `ATLAS_BROKER_NONCE_REDIS_URL=redis://host.docker.internal:16379/8` ⇒ 一旦有人設 `ATLAS_BROKER_NONCE_STORE=redis`，
  就會依賴「**容器能否連到 host 的 loopback-bound 埠**」——OrbStack 官方文件只說 `host.docker.internal` 可連「Mac 上的 server」，
  **未保證** loopback-only bind 可達（本機未實證，也不在生產做實驗）。
- **為何現在無害（2026-09-26 生產實測）**：`ATLAS_BROKER_NONCE_STORE` **未設** ⇒ 預設 `memory`
  （`internal/config/config.go:134`）⇒ 該 URL **惰性**；`~/.config/atlas-go/.env` 的 `^ATLAS_BROKER` 鍵數 = **0**；
  `atlas-redis` 的 `dbsize` = **0**（keyspace 空）⇒ 目前沒有任何 in-stack 消費者在用 16379。
- **修法（未做；改 app env 需重建 atlas-go，故先登錄）**：把該 URL 改成 docker 網路名
  **`redis://redis:6379/8`**（**同一顆 atlas-redis**，語意等價），不要讓容器依賴 host 的 loopback 埠。
- **驗收條件**：若啟用 `ATLAS_BROKER_NONCE_STORE=redis`，URL 必須走 docker 網路名且**容器重建後** broker nonce 仍正常；
  負對照：不得以「host 埠在 Mac 上 curl 得到」當成容器可達的證據。

### FU-20260926-11 — 「負向證明的非預期 exit code」殘留盤查：1 處未修（`check_finmind_quota.sh`）＋ 一類 `$VAR（` 展開陷阱

- **狀態**：`open`（同族多數已在 issue #2011 的 PR 修掉；本條追蹤**刻意未修**與**另票處理**的殘留）
- **記錄日期**：2026-09-26
- **來源**：issue #2011（PR #2003 的設計審查發現）＋ 本條所列可重現的 grep 命令
- **已修（同 PR 交付）**：`.github/workflows/quality.yml` 的 `secret-scan` / `monitoring-single-source`
  兩處 **inline** 負向證明抽成 `scripts/ci/*-negative-proof.sh`，改為精確斷言 `rc==1`
  （共用斷言庫 `scripts/ci/negative-proof-lib.sh`；自我測試 `tests/scripts/test-negative-proofs.sh` 餵 127/2 必須紅燈）。
  同一族順手收緊：`tests/scripts/test-secret-scan.sh`（`must_block`/B2/B7）、`test-check-frontend-dist.sh`
  （scenario2/3/5/6）、`test-check-routes.sh`（scenario2）、`test-install-webhook.sh`（C1–C3）；
  `Makefile` 的 `ci` / `ci-quick` 加「**0 支檢查被執行 ⇒ 失敗**」（空集合不得算通過）。
- **未修 ①（bash 變數展開，非 exit-code 問題但同屬「訊息/判定誠實性」）**：
  `scripts/ci/check_finmind_quota.sh:65` 的 `echo "❌ finmind quota: 無法解析 $STATE_FILE（calls_today 缺失）"`
  —— `$STATE_FILE` 後面**緊接全角「（」**，bash 會把該非 ASCII 字元併入變數名
  （實測 bash 3.2 與 5.3 皆然）⇒ 在 `set -euo pipefail` 下變成 `unbound variable` 崩潰，
  使用者看到的是 shell 錯誤而不是這句可行動訊息。**修法＝改成 `${STATE_FILE}`（1 字元）**；
  本次未改以避免與進行中的 FinMind lane 衝突。
  同型命中另有 `scripts/ops/imac-container-watchdog.sh:32`（**僅註解**，無害，不需修）。
  重現：`grep -rnP '\$[A-Za-z_][A-Za-z0-9_]*[^\x00-\x7f]' --include=*.sh --include=Makefile .`
  （`set -u` 下會崩潰；未開 `set -u` 時會**靜默吃掉變數值**，例如 `count=$N筆` 印成 `count=筆`）。
- **2026-09-26（後續 PR 進度）**：**未修 ① 已落地** —— `scripts/ci/check_finmind_quota.sh:65` 改為 `${STATE_FILE}`，
  且同型全掃後另修 `tests/scripts/test-install-webhook.sh:55/78/104`、`.github/workflows/quality.yml`（`mcp-tool-count`
  的 `$DOC_MIN–$DOC_MAX`）與**本條目併入後才出現**的 3 處（`tests/scripts/test-binary-freshness-guard.sh:104/127`、
  `tests/scripts/test-cron-entrypoint.sh:42`）。更重要的是把它變成**機制**：
  `scripts/ci/check_fullwidth_var_expansion.{sh,py}`＋ self-test
  `tests/scripts/test-fullwidth-var-expansion.sh`（30 項、精確 exit code 1/0/2）＋ `quality.yml` 的
  `fullwidth-var-expansion` job ＋ `make ci-gate`。該掃描器**移植自 a2a-dev 既有的同名守門**
  （`scripts/check-fullwidth-var-expansion.py`，tokenizer 原樣沿用；a2a-dev 是這個陷阱的原生守門），
  atlas-go 端擴充檔案類型：`*.sh` / `Makefile` / `*.mk` / `*.py` / **YAML 的 `run:` 區塊**
  （只掃區塊：整檔掃會被 YAML 的 `name:` 與引號污染 tokenizer 狀態而誤報）。
  該檢查是**唯一**能擋下這類 bug 的手段：實測 macOS+UTF-8 locale 崩潰、`LC_ALL=C` 與 linux/glibc/musl **皆不發作**
  ⇒ 只在 ubuntu 上跑的 CI **永遠不會紅**。
- **未修 ②**：`Makefile` coverage 段（`#2009`）的「門檻變數為空 ⇒ 比較反向通過」由另票處理（本 PR 未動該段）。
- **未修 ③（觀察，非缺陷）**：`.github/workflows/ci-cd.yml` 的 gosec 用 `-no-fail`、
  `vuln-scan.yml` 的 govulncheck 以 `|| true` + advisory-only SARIF 上傳 ⇒ **這兩個安全掃描永遠不會讓 pipeline 紅**
  （兩檔檔頭都明寫了理由）。若哪天要把它們變成真正的 gate，需要另票。
- **驗收條件**：任何**新增**的「證明某閘門會擋」測試，都必須同時餵「命令不存在(127)」與「用法錯誤(2)」
  並確認**紅燈**（範本：`tests/scripts/test-negative-proofs.sh`）；只驗「非 0」不算。
- **未修 ④（觀察，非本 PR 範圍；本次 CI 被它擋下）**：`internal/config` 的
  `TestShadowParametersDeclarationMatchesConsumers` 會 `filepath.WalkDir("internal")` 且
  **error 一律往上拋**（`internal/config/parameters_shadow_declarations_test.go:44-47,71-73`），
  而 `go test ./...` 是**跨 package 並行**；`internal/apigateway/register_adapters.go:401` 的
  `saveSnapshot` 寫的是**相對路徑** `data/state/<channel>`（package 測試的 CWD 下 =
  `internal/apigateway/data/…`），`internal/apigateway/adapter_finmind_util_test.go:91` 又會
  `os.RemoveAll("data")` ⇒ 該目錄在 walk 期間「出現又消失」，walker 讀不到就硬失敗：
  `walk …/internal: open …/internal/apigateway/data: no such file or directory`。
  這是**flaky 假紅**（同 commit 重跑會過；本機 main 與本分支跑同一支測試皆 PASS）。
  修法方向（未做）：walker 對 `fs.ErrNotExist` 寬容，或把該寫入路徑改成 `t.TempDir()`
  （別再依賴 CWD 相對路徑）。

---

### FU-20260926-10 — I31 的 production 半邊：新鮮度已接上既有監控（本 PR）；**仍缺「校準任務心跳」指標**，產物年齡可能誤報

- **狀態**：`open`（**程式面已交付**；生產驗收與殘留面 1 待做）
- **記錄日期**：2026-09-26
- **來源**：issue #1944 / I31；PR #1991 的「未完成項 1」；分支
  `fix/20260926-calibration-freshness-monitoring`（worktree `~/workspace/atlas-calib-freshness`，
  base `origin/main@df726b89`）。
- **已完成（本 PR）**：`configs/parameters.json` 的新鮮度由背景任務
  `calibration_freshness_metrics_export`（`cmd/atlas/calibration_freshness_metrics_task.go`，5 分鐘）
  評估，重用 `config.ValidateCalibration`（CLI 用的同一個判定），輸出
  `atlas_calibration_freshness_*` 五個 gauge；規則在
  `monitoring/rules/calibration_freshness_alerts.yml`（3 條，promtool **11 案例**含 6 個負向對照
  與 1 個「已知交接窗」；另做 8 項變異測試全部被咬住），
  落地說明在 [`calibration-freshness-runbook.md`](calibration-freshness-runbook.md)。
  契約收斂為**單一常數** `config.DefaultCalibrationMaxAge`（= CLI `--max-age` 預設值
  = production 命令的 48h）。
- **順帶查實（影響本條的判讀）**：政策上的 production 命令 `atlas-validate` **沒有隨 image 出貨**
  ——`cmd/calibration-validate` 是 CI 現場 build 的，本 repo 的 Dockerfile 只把
  `atlas-go`/`atlas-mcp`/`calibrate-seasonal`/`daily-replay-sync` 放進 `/app`，且 image 內
  **沒有** `python3`/`jq`/`node`（實查指令見 runbook §1）。⇒ 在本 PR 之前，生產上「資料已不新鮮」
  是**零觀測**（不是值班忘了跑，而是沒有東西會跑）；所有 triage 指令已改為 image 內確實存在的
  `grep`/`stat`/`head`/`tail`/`curl`。
- **殘留面 1（本條的主要缺口）：沒有「校準任務已執行」的心跳指標**
  - 現況：校準寫入是**有變更才寫**（`internal/risk/self_calibrate.go`：
    `if len(report.Changes) > 0` 才 `LockedSaveWithRollback`）⇒ `updated_at` 的年齡是
    「校準活動」的**上界**，不是直接量測。一個已收斂、連續多輪 `verdict=stable` 的系統
    會合法地超過 48h 不改寫檔案 ⇒ `CalibrationArtifactStale` 可能誤報。
  - 為何現在只做到這樣：要給出直接訊號必須接到 `cmd/atlas/calibration_tasks.go` 的
    18 個任務（1 inner + 17 top-level）並定義「執行成功」語意（含 early-return 與
    maturity gate），那是另一個範圍；本 PR 先交付可量測的部分並把誤報形狀寫進
    runbook §3.1 的第一順位排查。
  - **修法（未實作）**：新增 `atlas_calibration_task_last_run_timestamp_seconds{task=…}`
    （或沿用 completion handler）＋一條「校準任務超過 N 小時未執行」的規則；
    門檻由實測 cadence（24h 主、6h/1h 例外）決定。
  - **驗收條件**：連續 `stable` 的多輪（產物不變）必須**不**觸發任何告警；
    而任務真的停止執行時必須有告警 —— 負對照：不得再靠「產物年齡」推論任務死活。
- **殘留面 1b（與 #2013 的交互，已複驗）**：`risk_gate_calibrate` 的寫入已由 PR #2013 遷到
  `data/state/parameters.calibrated.json`（overlay），SSOT 保持 pristine ⇒ 本條監控的
  「SSOT 超過 48h」仍然有意義（其他校準器仍寫 SSOT，清單見 FU-20260926-07 第 3 點），
  但**看不到 risk 校準是否停滯**。要涵蓋它需要第二個判定語意（per-entry `calibrated_at`），
  不是把本族的 `max-age` 套上去就好；`config.GetParametersConfigPath()` 仍指 SSOT
  （複驗：`internal/config/calibration_overlay.go:186`），所以本族沒有被無聲換對象。
- **殘留面 2：結構性 finding 仍未進生產監控** —— `L1/L2_NO_REPRESENTATIVES` 之類由 CI 的
  `--policy=configs/calibration-validation-policy.json` 負責；生產端的結構漂移（有人手改
  parameters.json）目前仍無自動訊號。修法：加一個結構面的 gauge 或讓既有 policy 在生產
  也跑一次，並決定 accepted 集合在生產的語意（屬政策裁決）。
- **已知且刻意的行為（不是缺陷，不要「順手」改掉）**：
  1. **凍結樣本**：`_run_ok=0`（無法評估）之後，`age`/`last_calibrated` 仍以最後一次可評估的
     值留在 `/metrics`（collector 是 last-write-wins 且不移除序列）。沒有任何規則拿它們做判定
     （判定只看 `_ok`/`_run_ok`）⇒ 不會誤報；但**判讀順序**必須是「先 `_run_ok` 再 `_ok`
     再 `age`」（runbook §2.1）。行為由
     `TestObserveCalibrationFreshness_UnverifiableFreezesLastKnownSeries` 釘住。
  2. **≤15 分鐘交接窗**：`_run_ok` 由 1 翻 0 時，第 1 條立刻 resolve、第 2 條要累積 15m
     才 firing ⇒ 窗內兩條都不 firing。這是「同一個根因不重複 paging」的取捨，
     已寫成 promtool 案例 K；要縮窗就改第 2 條的 `for` 並同步該案例。
- **殘留面 3：生產驗收未執行**（本 PR 不得動 production）。部署後照 runbook §4：
  `curl -s localhost:18080/metrics | grep '^atlas_calibration_'`、
  `curl -s localhost:9090/api/v1/rules | grep -o 'Calibration[A-Za-z]*'`，
  並確認 `atlas_calibration_freshness_ok` 在生產為 1（生產檔案實測約 65 分鐘前被改寫）。
- **驗收條件（本條整體）**：生產上 `atlas_calibration_freshness_*` 有值、三條規則已載入、
  且「資料不新鮮」在無人記得跑 CLI 的情況下也會被看見。

### FU-20260926-13 — `empty_universe`（Step 1 就 0 檔）在 Prometheus 面**完全沒有訊號**：整條 pipeline 不動任何 counter

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：本輪缺陷收斂批次（代號 **A2**）；SSOT＝**PR #2026 的缺陷收斂 manifest（docs/operations/remediation-manifest.md）**（該檔隨 #2026 併入 main，故刻意不加反引號以免 markdown-links 誤判）。
  本票在該 manifest §2/§3 **未列**（最接近的 §3 E11 是「`symbols_excluded` 無排除原因細分」，主題不同）
  ⇒ 本票為**新增登記**，請 root 決定是否補進 manifest §3。
- **事實（實測行號，2026-09-26，worktree HEAD `81e4fe62`）**：
  - 原因常數存在：`internal/monitoring/universe_scheduler.go:89-90`（`RankedFallbackEmptyUniverse = "empty_universe"`）。
  - **early-return 發生在計數之前**：`internal/monitoring/universe_scheduler.go:751-754`
    （`if result.SymbolsBuilt == 0 { markRankedUntrustworthy(result, RankedFallbackEmptyUniverse); return result, nil, nil }`），
    而 `gatheredCounter.Add(...)` 在 `:758-760` ⇒ 這條路徑**真的不動任何 counter**。
  - 同檔 `:750` 有 `result.QuotesStatus = QuotesStatusNotAttempted`，是目前**唯一**的機器可讀標記，
    但它只寫進 snapshot（`data/state/universe_snapshot.json`），**未曝露成指標**。
  - 規則檔自述此缺口：`monitoring/rules/atlas_universe_scoring_alerts.yml:207-210`
    （「`gatheredCounter.Add` 在 `if SymbolsBuilt == 0 { return }` **之後** ⇒ 整個 pipeline 不動任何 counter」），
    並在 `:233` 再述「全部規則（新舊）都不覆蓋 `empty_universe`：需要 Go 端新增心跳指標」。
  - 這個缺口是**刻意且有測試釘住**的：`monitoring/tests/atlas_universe_scoring_gaps_test.yml:436`
    （負向對照 3：WarmUp 建立的整族都在、都零增量 ⇒ 六條都不 firing）。
- **為何先開票、不實作**：修法要**同時動三處**且屬跨檔功能新增 ——
  ① 指標面（`internal/monitoring/metrics/universe.go` 的 series 清單 `:85-100` ＋ `WarmUp()` `:137`）；
  ② alert rule（`monitoring/rules/atlas_universe_scoring_alerts.yml`）；
  ③ promtool 測試（`monitoring/tests/atlas_universe_scoring_gaps_test.yml` 的負向對照 3 必須改寫）。
  它也會碰到與 #1995 同一族的指標契約 ⇒ 不是本輪的 bounded 修正，先登記。
- **驗收條件**：生產上「Step 1 就 0 檔」當日必須有一條**真陽性**告警（或至少一個會前進的 series）；
  **負對照**：`SymbolsBuilt > 0` 的正常執行不得觸發；且「整條 pipeline 沒跑」（同族缺口 (b)）
  不得被本票的規則誤報成 `empty_universe`（兩者需要相反的 triage）。

---

### FU-20260926-14 — Telegram bot token（實際名稱 `TELEGRAM_BOT_TOKEN`）無 hot-reload：輪替只能靠重載 launchd plist

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：本輪缺陷收斂批次（代號 **B13**）；SSOT＝**PR #2026 的缺陷收斂 manifest（docs/operations/remediation-manifest.md）**（§2/§3 未列 ⇒ 新增登記）。
- **事實（實測，2026-09-26）**：
  - **對照組：資料通道 key 與 MCP token 都有 hot-reload**
    - channel key：`cmd/atlas/main.go:700`
      （`channelKeyMgr.RegisterApplier("finmind", marketdata.UpdateSharedFinMindAPIKey)`）；
      套用端 `internal/marketdata/finmind_client.go:374-379`（`SetAPIKey`，thread-safe 換 key，不重建 rate limiter）；
      機制自述 `internal/channelsecrets/doc.go:2`、`:8`（issue #1776 Phase 1：persist → 熱套用到 live client）。
    - MCP token：`cmd/atlas-mcp/server/token_admin_handler.go:32`（`POST /api/admin/mcp/tokens/<id>/rotate`）。
  - **env 名稱是 `TELEGRAM_BOT_TOKEN`**（**不是** `ATLAS_TELEGRAM_BOT_TOKEN`；後者在 repo 內 0 命中，
    唯一出現處是另一份文件的示例名 `docs/modules/alert-system.md:21`）。
  - 兩個消費端、**兩者都在啟動時讀一次**：
    ① `scripts/alertmanager-webhook/atlas-alertmanager-webhook-to-telegram.py:18`
      （`BOT_TOKEN = os.environ.get('TELEGRAM_BOT_TOKEN')`，module 層讀取 ⇒ 換 token 必須重啟行程）；
    ② `monitoring/alertmanager.yml:97`（`bot_token: ${TELEGRAM_BOT_TOKEN}`）⇒ Alertmanager 自己也要 reload/restart。
  - 注入路徑是**安裝期寫進 LaunchAgent plist**：
    `scripts/alertmanager-webhook/com.goluck.atlas-webhook-to-telegram.plist:22-23`
    （`TELEGRAM_BOT_TOKEN` = `__INJECT_AT_INSTALL__`），由 `scripts/alertmanager-webhook/install-webhook.sh`
    寫入 `~/Library/LaunchAgents/` 那一份（`:152`）並 `launchctl bootout` + `bootstrap`（`:159-164`）。
  - **刻意不用 file-based token**：plist `:18-20` 與 `install-webhook.sh:10-12` 明寫理由 ——
    a2a-dev 的 macmini-recover #50 會從**已安裝 plist 的 EnvironmentVariables** 讀 `TELEGRAM_BOT_TOKEN` 做 `getMe` 檢查，
    **換 key 名會讓它變成 WARN**（破壞 DoD 48 OK/0 WARN）。
- **為何先開票、不實作**：這不是漏一行，而是**需要先裁決介面**：
  ① 引入 `TELEGRAM_BOT_TOKEN_FILE`（**必須同時改 a2a-dev 的檢查腳本** ⇒ 跨 repo）；
  ② 加 SIGHUP 或 admin rotate 端點（跨 process、跨兩類 handler）；
  ③ 接受「輪替＝重跑 install 腳本」但把步驟納入 runbook。
  三個方向的半徑都大於本輪 bounded 修正 ⇒ 先登記。
- **驗收條件**：能在**不重建 image** 的前提下完成一次 token 輪替（若最終決定「必須重載 plist」，
  則該步驟必須寫進 runbook 並可被非作者照做）；輪替後 a2a-dev 的 `getMe` 檢查**仍為 OK**（不得因換 key 名退化成 WARN）；
  **負對照**：輪替後舊 token 在 BotFather 端撤銷必須立刻失效（不得出現「新舊都能用」）。

---

### FU-20260926-15 — `.githooks/pre-push` 沒有 host binary 新鮮度閘門：source 已前進但 `bin/atlas-mcp` 仍舊也能 push

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：本輪缺陷收斂批次（代號 **B14**）；SSOT＝**PR #2026 的缺陷收斂 manifest（docs/operations/remediation-manifest.md）**。
  **與 manifest §3 E4 的關係**：E4 是**同一個檔**的另一個缺口（`pre-push` 取不到 `origin/main` 就放行）。
  本票是**不同缺口**（完全沒有 binary 新鮮度檢查）⇒ 修法需協調，但不是重複開票。
- **事實（實測，2026-09-26）**：
  - `.githooks/pre-push` 現有閘門：`make ci-gate`（`:29`）、`scripts/ci/check_frontend_dist.sh`（`:50`）、
    `make ci-full`（`:70`）、Gate 2 HEAD == `origin/main`（`:82-94`）、Gate 3 zero diff（`:96-106`）。
    全檔 **0 命中** `rebuild` 或 `check-binaries`。
  - 目標存在且會檢驗：**`Makefile:733` `rebuild-host-bin:`**（只重建 host `bin/atlas-mcp`）；
    `Makefile:710-711` `check-binaries:` → `scripts/check-binary-freshness.sh`；`Makefile:729` `check: check-binaries`。
  - 檢查腳本對「檔案不存在」是**軟出口**：`scripts/check-binary-freshness.sh:152-157`
    （`if [ -f "$HOST_ATLAS_MCP" ] … else echo "  ⚠ bin/atlas-mcp not found at … (skipping)"`）⇒ 不存在**不算失敗**。
  - 同族文件已承認這個形狀：`docs/developer-guide.md:66`（「host `bin/atlas-mcp` 缺失但檢查仍綠 ⇒ 執行 `make rebuild-host-bin` 後重跑」）。
- **為何先開票、不實作**：修法要動 `.githooks/`（**共用檔**，同檔正由 manifest §3 E4 那一線處理），
  而且要先把**閘門政策**定下來：pre-push 是否強制 `rebuild-host-bin`（會弄動 working tree 產物、也拖慢 push）、
  或只把 `check-binaries` 的 skip 改成 fail-closed、或維持現狀但把責任明文寫給 session-start。
  政策未定就改 ＝ 有機會製造第二個 false-green（或第二個誤紅）⇒ 先登記。
- **驗收條件**：在 source 領先 `bin/atlas-mcp` 的狀態下 push **必須**得到可行動的紅燈
  （或明確記錄此為刻意不防、並指出替代路徑）；
  **負對照**：`bin/atlas-mcp` 與 HEAD 一致時**不得**誤紅、也不得為此多付明顯時間成本。
- **相關（2026-09-26 追加）**：全新 worktree 的 `//go:embed all:dist` 死結（`admin_web/dist`／
  `client_web/dist` 不存在 ⇒ `go build ./...` 紅 ⇒ 新 lane 第一次 push 被擋）——同屬
  **host/worktree 環境前置**，修在 PR #2034（`make embed-dirs`）。

---

### FU-20260926-16 — 季節性校準的**污染源歸因**仍未定調（4 個超界 `adjustment_factor` 是哪來的）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：本輪缺陷收斂批次（代號「**A5 殘**」）；SSOT＝**PR #2026 的缺陷收斂 manifest（docs/operations/remediation-manifest.md）**。
- **與 FU-20260926-04 的關係（先講清楚，避免重複計數）**：`-04` 登錄的是「**值本身**是壞的 ＋ 消費端被夾沒有任何痕跡」；
  本票**只認領「歸因／定調」這一半**（是校準輸入壞？是 bound 太窄？還是有別的寫入端？），且**不重複** `-04` 的驗收條件。
  **若 root 判定兩者同源，請把本票併入 `-04` 並將本票標 `done`。**
- **事實（實測，worktree HEAD `81e4fe62`）**：
  - 版控檔仍是壞的：`configs/parameters.json:4602`（`dividend_season = -0.2634376289519962`）、
    `:4686`（`summer_electricity = -0.2883015395477875`）、`:4729`（`ai_infrastructure_buildout = 2.555461115152644`）、
    `:4772`（`year_end_positioning = 3.44143401207912`）。合法區間 `[0.3, 2.5]` ＝
    `internal/industry/seasonal_health.go:16-17`（`DarwinianMinAdjustment` / `DarwinianMaxAdjustment`）。
  - 消費端確實會夾：`internal/industry/seasonal_health.go:32`（`ClampAdjustmentFactor`）、
    `:41`（`IsAdjustmentFactorInRange`）；一次性 warn 在 `internal/industry/seasonality.go:578-594`
    （由 `NewSeasonalEngineFromConfig`（`:563-573`）呼叫）。
  - **寫入端的守門早就在**：`cmd/calibrate-seasonal/main.go:180` 呼叫 `validateCalibrationResult`
    （定義 `:408-425`，三軸：`adjustment_factor` 對 Darwinian 區間、`historical_accuracy ∈ [0,1]`、`avg_market_return ∈ [-1,1]`）；
    超界時**不寫** `adjustment_factor`，只記 verdict（`:180-188`）
    ⇒ **現行 `-update` 路徑結構上不可能產生這 4 個值**，污染源必在「守門之前」或「另一個寫入端」。
  - **兩個已實測的歸因陷阱（本票存在的直接理由）**：
    - ① `git blame` **不能**當歸因證據：`10f019a1`（2026-08-28「reformat parameters.json to repo 2-space format」）
      是純重排（`git show --stat`：8107 insertions / 8107 deletions），4 行現在都 blame 到它
      ⇒ 拿它推論「值是那時寫入的」是**錯的**（已否證）。
    - ② 本 clone 是 **shallow**（`git rev-parse --is-shallow-repository` = `true`）：
      `git log -S "3.44143401207912" -- configs/parameters.json` 只回得到 `07a3f85f`（2026-06-15, PR #532），
      而該 commit 在本地**沒有 parent 物件**（`git show 07a3f85f^` ⇒ `invalid object name`）
      ⇒ 「首次寫入的 commit」在此 clone 內**不可證**。
    - ③ 另一個直覺陷阱（**已排除**）：這 4 個 pattern 沒有 `calibration_verdict` 欄位，看起來像「不是校準器寫的」——
      但 `calibration_verdict` 是 `183ed65c`（2026-09-26, PR #1990）才加進寫入端的
      ⇒ **缺欄位不代表寫入者不是校準器**，不要用這點下結論。
- **為何先開票、不實作**：定調需要**量測**：用 `data/replay/finmind_2020_2024.jsonl` 重跑這 4 個 pattern，
  比對「校準觀測值」與 ① bound ② 現值 的關係；且要先把 shallow clone 補成完整歷史（或明確放棄 blame 路線）才有歸因證據。
  這是研究型工作、不是 bounded 修正，而且**直接改值會把現場洗掉** ⇒ 先登記。
- **驗收條件**：能對這 4 個值逐個回答「誰寫的（哪一個寫入端／哪一次執行）」，或明確結論「不可證」並給出替代防線；
  **在定調完成前不得改動 `configs/parameters.json` 的這 4 個值**（保留現場）。

---

### FU-20260926-17 — `internal/fubonproxy` 測試 flaky：`TestProcessManager_Supervise_RestartFailureCap` 距寫死的 3s 上限只剩約 0.2–0.4s

- **狀態**：`open`
- **修復**：PR #2033（測試 hermetic 化：系統配發埠＋決定性等待）
- **記錄日期**：2026-09-26
- **來源**：本輪缺陷收斂批次（代號「fubonproxy flaky」）；SSOT＝**PR #2026 的缺陷收斂 manifest（docs/operations/remediation-manifest.md）**。
  manifest §3 E8 是**另一支** flaky（`WalkDir("internal")` 撞 apigateway 的相對 `data/`）⇒ 本票不是重複；
  與 `FU-20260926-18`（E8，`internal/config` 的 `WalkDir`）為**不同** flaky，勿合併處理。
- **事實（實測，worktree HEAD `81e4fe62`，macOS arm64 / go1.26.4）**：
  - 失敗訊息出處：`internal/fubonproxy/manager_test.go:1345`
    （`t.Fatal("supervisor did not exit within 3s")`），位於 helper `waitForSupervisorDone`（`:1333-1347`），**上限寫死 3s**。
  - 使用者：`internal/fubonproxy/manager_test.go:1475`
    （`TestProcessManager_Supervise_RestartFailureCap`，定義 `:1444`）與 `:1433`（yield-to-external-proxy 測試）。
  - **本機實測（2026-09-26）**：
    - `go test ./internal/fubonproxy/ -run 'TestProcessManager_Supervise_…' -count=5` ⇒ **5/5 PASS**，
      但每次 **2.61–2.63s**；log 內單次 `process_started`(17:09:14.826) → `stopped`(17:09:17.400) ＝ **2.574s**。
    - `ATLAS_STORE_BACKEND=sqlite go test -race -count=3 …` ⇒ 3/3 PASS，但 **2.79s / 2.72s / 2.63s**
      ⇒ 距 3s 只剩約 **0.2s**。
  - **成本為何固定約 2.6s**：`internal/fubonproxy/manager.go:62` `maxRestartFailures = 5`，
    每輪前有 `sleep 0.3` 的假 proxy（`manager_test.go:1449`）＋ backoff 10ms（`:1325-1326`）
    ⇒ 5 輪 ≈ 2.4s，加健康檢查與收尾 ≈ 2.6s。**餘裕比一次排程抖動還小。**
  - **為何在 `make ci-full` 會紅的機制**：`Makefile:947` 跑的是
    `go test -race -count=1 $(go list ./... | grep -v '/cmd/atlas$')` ⇒ **全 package 並行 ＋ race detector**；
    本票的測量只跑了**單一 package**，並行負載下只會更慢。這解釋「單獨重跑 ok、`make ci-full` 紅」。
  - **附帶觀察（同一次 log）**：`restart_foreign_port port=… pid=9927 cmd=fubonproxy.test`
    —— 被判為「外部佔用者」的其實是**測試行程自己**（`:1473` 的 `bindPort` 在行程內綁埠）。
    不是缺陷，但會讓 triage 的人誤判「有外部程序在搶 port」。
- **為何先開票、不實作**：flaky 要先**量化**（重現率、`make ci-full` 併發下的分布），並裁定修法方向：
  ① 提高 helper 上限（會連「真的卡死」一起放寬）；② 改成事件驅動等待（要動 supervisor 的觀測面）；
  ③ 把固定 `sleep 0.3` 與 backoff 參數化／縮短（最小改動，但要先證明仍測到同一件事）。
  直接改測試＝可能把真紅一起吃掉 ⇒ 先登記。
- **驗收條件**：在 `make ci-full` 的實際併發條件下，該測試連續 N 輪（建議 ≥ 20）不再出現
  `supervisor did not exit within 3s`；**負對照**：把 supervisor 改成真的不退出時，該測試**仍必須紅**。

---

---

---

### FU-20260926-12 — `make ci` 把「掛住（timeout 124）」算成 skipped ⇒ 閘門回 0（**已可複現**，未修）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源（可重現）**：`Makefile:409-429`（`ci:` target 的 `for script in scripts/ci/check_*.sh` 迴圈）。
  取**同一個迴圈形狀**（只把 glob 換成 fixture、`timeout 30` 縮成 `1`）：

  ```bash
  # ① 造 fixture（hang.sh 永遠跑不完；ok.sh 正常）
  d=$(mktemp -d)
  printf '#!/usr/bin/env bash\nsleep 5\n' > "$d/hang.sh"
  printf '#!/usr/bin/env bash\nexit 0\n'  > "$d/ok.sh"
  # ② 把 Makefile:409-429 的迴圈貼成 "$d/Makefile"，只改兩處：
  #    for script in scripts/ci/check_*.sh  →  for script in hang.sh ok.sh
  #    timeout 30                           →  timeout 1
  # ③ 跑它
  make -C "$d" ci; echo "rc=$?"
  # 實測輸出（2026-09-26，macOS + GNU timeout）：
  #   → hang.sh
  #       TIMEOUT (>1s): hang.sh
  #   → ok.sh
  #   CI: 1 passed, 0 failed, 1 timed out
  #   rc=0        ← 掛住的檢查沒有讓閘門變紅
  ```
- **現況**：`timeout` 回 124 時只 `skipped=$((skipped+1))`，而收尾只檢查 `failed > 0`
  ⇒ **一支永遠跑不完的檢查**（等網路、等鎖、等 docker）在 `make ci` 眼裡等於「通過」。
- **風險**：false-green 家族（#2011）。`make ci` 是本機與 GH Actions 都會跑的閘門 ⇒
  「檢查掛住」不會讓任何人變紅，只會在多跑幾次之後被當成雜訊忽略。
- **影響面**：只有 `ci:` 這一段。`ci-quick`（`Makefile:431+`）用 `if timeout 10 …; then passed; else FAILED`
  —— 124 落 `else` ⇒ **會紅**，不受影響；`ci-gate` 是逐支明列（沒有 timeout/吞碼）。
- **最小修法建議（只建議，未實作；需業主定 policy）**：三選一 ——
  ① 124 ⇒ 計入 `failed`（fail-closed，最直白）；
  ② 保留 `skipped` 但**收尾時 `skipped > 0` 也 exit 非 0**（可先量：30s 預算對現有 `check_*.sh` 夠不夠）；
  ③ 對已知慢的檢查改成明列清單＋較長 timeout，其餘一律 fail-closed。
  **取捨**：`make ci` 是每天跑的路徑，①/② 都可能讓「合法的慢檢查」在負載高的機器上誤紅 ⇒
  要先有 timeout 預算的量測，不是直接改。
- **為何不納入本 PR（`$VAR` 緊接非 ASCII 的靜態閘門）**：
  ① 主題不同（一個是展開語法，一個是**閘門政策**）；
  ② 會改到 `make ci` 這段所有人每天都跑的路徑 ⇒ 回滾半徑大；
  ③ 同一個迴圈區域正由 **PR #2020**（`fix/20260926-negative-proof-exact-rc`）改動（在 `make ci` 收尾加
     `passed -eq 0` 守衛）⇒ 一起改會製造衝突與「兩個 false-green 混在一起」的審查困難。
- **驗收條件**：在 `scripts/ci/` 放一支 `sleep 31` 的 `check_*.sh` ⇒ `make ci` 必須回非 0
  （或至少在 timeout 發生時讓 job 變紅），且正常情況下 `skipped` 不增加。

---

### FU-20260926-20 — atlas-go 的 iMac 殘留清理（任務 E10）：A 類已改 Mac Mini；**2 個 watchdog 操作入口仍指向不存在的 `kk@kimac`（在禁改檔內）**

> **號段說明（撞號處置）**：本條目原配置為 `FU-20260926-14`，但該號在併入前已被他線的
> 「Telegram bot token 無 hot-reload」條目取走（race）⇒ 依 `docs/operations/remediation-manifest.md`
> §6 的強制分配表改為 **`FU-20260926-20`**（表內明載 lane `fix-E10-retired-imac` = -20）。

- **狀態**：`open`（殘留項落在禁改檔與 B 類，需另一條 lane）
- **記錄日期**：2026-09-26
- **來源（可重現）**：branch `fix/20260926-e10-imac-residue`；盤查指令
  `git grep -n -I -e 'iMac' -e 'kk@kimac'`，並另外掃過 iMac 時代的 Tailscale 數值 IP（本檔刻意不寫出該
  字面值，避免清單條目自我指涉製造命中；該 IP 在本 repo 已 0 命中）。背景：iMac 已於 2026-09-22 退役出售，
  現行 production = Mac Mini（`ssh kmacmini`）；go-member 在 Mac Mini 是 launchd 服務
  `:8093`（**不是** iMac 時代的 `:3000`，`:3000` 在 Mac Mini 是 gitea）；主 API 容器實名 = `atlas-go`
  （`-imac` 後綴淘汰）。
- **本 PR 已做（A 類＝今天還會被執行/遵循者）**：`AGENTS.md`、`CLAUDE.md`、`.env.example`
  （本 repo 唯一殘留的 iMac 時代 Tailscale 數值 IP，已改成生產實查值 `http://host.docker.internal:8093`）、
  `docs/operations/local-deploy.md`、`docs/guides/install-and-deploy.md` §4.2、`monitoring/rules/atlas_container_liveness_alerts.yml`
  ＋`monitoring/tests/atlas_container_liveness_test.yml`（promtool **完整比對**，兩檔必須同步）、
  `scripts/sync-darwinian.sh`、`scripts/ops/imac-container-watchdog.sh`、
  `scripts/ops/launchd/com.goluck.atlas-container-watchdog.plist`（`/Users/kk`→`/Users/kaecer`，與 Mac Mini
  實際安裝檔逐欄一致）、`docs/operations/docker-compose.{prod,crons}.yml`（加註）、
  `docs/operations/watchdog/*`（淘汰告示，**不刪檔**）、
  `.claude/skills/atlas-imac-prod-guard/SKILL.md`）（4 處指向不存在的 Mac Mini 復原文件 dead link；該檔只存在於
  a2a-dev，路徑 `~/workspace/a2a-dev/docs/operations/MACMINI-RECOVER.md`，本 repo 沒有）。
  B 類（歷史紀錄）**只加註記、不改歷史值**。
- **仍存在的殘留（未修，需另一條 lane）**：
  1. `Makefile:101` `IMAC_HOST ?= kk@kimac` 與 `Makefile:103` `WATCHDOG_DST := /Users/kk/bin/atlas-container-watchdog.sh`
     ⇒ 兩個 watchdog target **必然失敗**（實跑：`ssh: Could not resolve hostname kimac`，`make imac-watchdog-diff` exit 2）。
     `Makefile` 在本任務屬禁改檔；`docs/operations/local-deploy.md` 已補上等價的手動指令。
  2. `Makefile` 其餘 iMac 措辭（`sync-imac`、`sync-imac-deploy`、`test-makefile-imac-guard`、guard 訊息）同屬同檔禁改。
  3. `docs/reference/traps.md` 與 `.github/workflows/quality.yml` 屬禁改檔 ⇒ **本任務未掃描、未處置**（可能有同類殘留）。
  4. `internal/**`、`cmd/**` 的 Go 註解（B 類，本次**未動**）：`internal/orchestrator/system.go:58`、
     `internal/orchestrator/system_risk_session.go:238`、`internal/orchestrator/risk_forensics_hydration_test.go:217`、
     `internal/marketdata/bls_cpi_provider.go:8`、`internal/channelsecrets/crypto.go:36`、
     `internal/narrative/narrative_test.go:760`、`internal/monitoring/api/narrative/handlers_test.go:661`、
     `cmd/atlas/prism_wiring_test.go:58`。皆為**帶日期的歷史觀測**，且不含退役主機的 SSH 目標或數值 IP，依分類規則不應改寫；
     唯 `crypto.go:36`（"back it up alongside the iMac .env"）與 `system_risk_session.go:238`（可執行指令
     `docker logs ... atlas-go-imac`，在主機上會 `No such container`）讀起來像現行操作 → 建議另開小 PR 各加一行。
  5. 其餘 B 類保留原值（`CHANGELOG.md`、`docs/decisions/**`、`docs/llm-adr-log.md`、
     `docs/operations/investigation-twse-timeout-2026-08-18.md`、`tasks/**`、`.gitignore` 註解、
     `docker-compose.yml:144` 的舊檔名）；其中 `tasks/stockpicker-misleading-mechanisms-audit-k3-20260828.md`
     依規則已加**一行**歷史註記（該檔含 `kk@kimac`）。
- **風險**：無 runtime 風險（純文件/註解/CI 註解）。唯一的行為面殘留是上述 `make imac-watchdog-*`：
  它是**硬失敗**（不是靜默錯），且文件已寫明手動等價指令，故不緊急。
- **最小修法建議（只建議，未實作）**：`Makefile` 兩行各改一個值
  （`IMAC_HOST ?= kmacmini`、`WATCHDOG_DST := /Users/kaecer/bin/atlas-container-watchdog.sh`），
  **target 名稱保留**（`imac-` 前綴是歷史債；改名會動 launchd label 與既有文件）。

### FU-20260926-21 — `scripts/ci/check_jev_contract.sh` **會真的連外呼叫 Jev 服務**：外部服務／網路一 flake 就紅 ⇒ 擋住合法 push（同日第二次同型事故）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：2026-09-26 推送 PR #2029（`fix/ci-swallowed-errors`，commit `e2e1b0de`）時，pre-push hook 的
  `make ci-full` 在 `make ci-gate` → `ci-quick` 階段紅：
  `❌ FAILED: scripts/ci/check_jev_contract.sh`（`✅ CI-quick: 14 passed, ❌ 1 failed`）。
- **現況（實測，皆為本機可重現）**：
  1. 該檢查**會真的對外呼叫 Jev**：單獨執行輸出
     `✓ 實際呼叫 OK: noul=0.93 model=jev-1.13.0 tokens=281 445ms attempts=1`
     （`✓ jevkit 自測通過`）。
  2. **失敗後單獨重跑 2 次都是 exit 0**（`✅ Jev 契約檢查通過`）。
  3. 同一個 patch 在**數分鐘前**的另一次 push 中，`make ci-full` **全綠**
     （log 含 `✅ ci-full passed` 與 coverage step `Total coverage: 70.4%`），
     且兩次 patch 內容 sha256 **逐位元組相同**（`62a6f43caf94ddacbf9e…`）。
  4. 當次改動只有 `.github/workflows/daily-maintenance.yml`，與該檢查無關。
- **判定：這是外部相依（Jev 服務／網路）造成的間歇性紅燈**，不是確定性缺陷。
  因此「紅燈本身」沒有問題（不可靜默吞掉），問題在於**它坐在 pre-push 的阻擋路徑上**。
- **風險**：誤紅會擋住合法推送 ⇒ 實務壓力會逼人用 `git push --no-verify`（**連 `make ci-gate` 都跳過**），
  結果是**真正的**紅燈更容易被忽略 —— 與本 session 在修的同族（gate 靜默失效／閘門可信度流失）互為表裡。
  同日已有兩次同型：本票（`check_jev_contract.sh`）與 FU-20260926-17（`internal/fubonproxy` 時序 flake）。
- **建議方向（只建議，未實作）**：
  1. **首選：離線化**。以 hermetic fixture／stub 取代真呼叫；repo 已有同型前例
     （`tests/scripts/*` 與 `scripts/ci/negative-proof-lib.sh` 的 fixture 式負向證明）。
     若真呼叫仍要保留，應移到**非阻擋** lane（nightly 或 `workflow_dispatch`），
     **不要**放在 pre-push／PR 必經路徑上。
  2. **次選：明示外部相依 + 有界重試 + 記錄**。重試 N 次，每次都要**寫出**第 k 次失敗／逾時的
     訊息與最終判定（`::warning::` 或步驟輸出），並在腳本檔頭明列「本檢查需外網」。
  3. 不論選哪個：**不得**在 pre-push 路徑加 `|| true` / `continue-on-error`
     （那正好是本 session 正在修的同族 false-green）。
- **驗收條件（供實作者）**：在有外網阻斷的環境（例如 `http_proxy` 指向黑洞）跑
  `make ci-gate`，行為必須**明示原因**且**不因外部 flake 而擋住合法 push**
  （離線化版本的期望：該檢查不依賴外網即可判定；重試版本的期望：輸出可區分「契約不符」與「連不上」）。
- **不可動**：`.githooks/pre-push` 本身（該檔已由 FU-20260926-15 佔用）。

---

---

### FU-20260926-22 — `internal/apigateway` 的 `TestBackgroundTaskManager_RunTask_AppliesStartupJitter` 機率性假紅：**full jitter 單次抽樣落在 1ms 以下**（≈0.2%/次；**不是**負載相依）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：修 `FU-20260926-17`（fubonproxy flaky）時，同一 worktree 跑 `make ci-full` 第一次紅在此測試（同一次 run 的其他 package 全綠）。
  本票**尚未收錄進** `docs/operations/remediation-manifest.md`（該檔屬 #2026／另一 lane）⇒ **不於本 PR 改 manifest**。
- **事實（實測，2026-09-26，worktree `fix/20260926-fubonproxy-flake-hermetic2`，macOS arm64 / go1.26.4）**：
  - 失敗輸出（`make ci-full` → `Makefile:951-952` 的 `go test -race -count=1 $(go list ./... | grep -v '/cmd/atlas$')`）：
    ```
    --- FAIL: TestBackgroundTaskManager_RunTask_AppliesStartupJitter (0.00s)
        background_test.go:1368: subsequent run (LastRun non-zero): elapsed=331.459µs, expected ≥ 1ms. Jitter should be applied.
    FAIL	github.com/kaecer68/atlas-go/internal/apigateway	52.464s
    ```
  - **機制（程式碼事實，非推論）**：`internal/apigateway/background.go:444` 是
    `jitter := time.Duration(rand.Int63n(int64(task.Jitter)))` ⇒ **full jitter（均勻分布 `[0, Jitter)`）**；
    測試 `background_test.go:1307-1310` 取 `targetJitter=500ms`、`minElapsed=1ms`、`maxElapsed=700ms`，
    並在 Phase B（`task.SetLastRun(now-2h)` ⇒ LastRun 非零）以**單次** wall-clock 量測斷言 `elapsed ≥ 1ms`（`:1366-1370`）。
    ⇒ 單次抽樣 < 1ms 的機率 ＝ 1ms / 500ms ＝ **0.2%／次**，與 `CHANGELOG.md:1074` 自載的「偽陽性率 ≈ 0.2%」一致。
  - **判定：不是負載相依**（我先前口頭假設「負載造成」已**被否證**）：機率來自**單次隨機抽樣**，與並行負載無關；
    負載只會讓 `elapsed` 偏大 ⇒ 更不容易紅。
  - **機制探針（可複現；暫時改測試常數後已還原，`git status` 乾淨）**：把 `targetJitter` 由 `500ms` 改成 `5ms`
    （jitter 窗口縮 100 倍、抽樣分布不變）⇒ 同一個斷言立刻大量紅，且簽名完全相同：
    `-count=60` ⇒ **10 FAIL / 50 PASS**（≈16.7%，與「抽到 < ~0.83ms」的理論值 ≈16.6% 相符），
    失敗樣本 `elapsed=172µs / 211µs / 553µs / 599µs / 684µs` 全部 < 1ms
    ⇒ 紅燈確實源自**抽樣值**，不是排程抖動、也不是環境負載。
  - **與 `FU-20260926-17` 無關**：那條只改 `internal/fubonproxy/manager_test.go`（別的 package 的/test 檔）⇒ 不可能影響本測試；
    且該次 `make ci-full` 的 race 步驟裡 `internal/fubonproxy` 是綠的，本套件才是唯一紅燈。
- **重跑證據（本機，2026-09-26）**：
  - `go test -race -count=10 -run 'TestBackgroundTaskManager_RunTask_AppliesStartupJitter' -v ./internal/apigateway/` ⇒ **exit 0，10/10 PASS**。
  - `go test -race -count=200 -run '…' -v ./internal/apigateway/` ⇒ **exit 0，200/200 PASS**（與 0.2%/次 一致：200 次的期望紅燈 ≈ 0.4 次）。
  - 整條 race 指令單跑：`ATLAS_STORE_BACKEND=sqlite go test -race -count=1 $(go list ./... | grep -v '/cmd/atlas$')` ⇒ **exit 0（181 packages ok、0 FAIL）**。
- **風險**：它坐在 `make ci-full`／pre-push 的**必經路徑**上 ⇒ 0.2%/次的假紅會擋合法 push
  （與 `FU-20260926-17`、`FU-20260926-21` 同族：閘門可信度流失 ⇒ 逼人用 `--no-verify`）。
- **建議方向（只建議，未實作）**：
  1. **首選：把 jitter 抽樣做成可注入 seam**（同 repo 已有同型做法：`restartInitialDelayForTest`、`portprobe.lsofPath`），
     測試固定抽樣值 ⇒ 斷言回到**決定性**，同時保住「jitter 被誤刪 ⇒ 立即紅」的 regression 能力。
  2. **次選：統計式改寫**：對 N 次抽樣取統計量（如 20 次取 max ≥ 1ms、且每次 ≤ 700ms），
     並把偽陽率寫成 `0.2%^N`（N=20 ⇒ ~8e-62）；**只放大 `minElapsed` 不算修**（會讓「抖動被移除」更難被抓到）。
  3. **另一條路：不靠 wall-clock**：改觀測「確實走了 jitter 分支」的可觀測事實（事件／欄位／log），
     並在測試內**明確界定 jitter 上界**（full jitter 的上界 ＝ `task.Jitter` 本身 ＝ 500ms）。
  4. **不需改 production 行為**：full jitter 是刻意的 thundering-herd 防護（`background.go:184` 附近的註記）；
     本票是**測試可測性**問題。
- **驗收條件**：在 `make ci-full` 的實際條件下該測試連續 ≥ 200 次 0 紅；
  **負對照**：移除 `background.go:443` 的 `!task.LastRun().IsZero() && task.Jitter > 0`（jitter 不再套用）⇒ 該測試**仍必須紅**。
- **不可動**：`docs/operations/remediation-manifest.md`（另一 lane 的 SSOT）、`.githooks/pre-push`（`FU-20260926-15` 佔用）。

### FU-20260926-23 — `scripts/verify-sector-allocation-closure.sh`：**從未被執行過**的死 gate（依賴檔 #1255 移出 repo ＋ `check()` eval bug ⇒ 今 exit 2、呼叫端 0）⇒ **明示停用**（E16）

- **狀態**：`open`（本 PR 只做「明示停用＋登記」；真正重啟的條件見下方「驗收條件」）
- **記錄日期**：2026-09-26
- **來源**：root 交辦（atlas-go 小任務）。本 PR：`fix/20260926-dead-gate-closure`（worktree `atlas-deadgate`，base `origin/main` `0cc32d16`）。
  對應 `docs/operations/remediation-manifest.md` §3 **E16**。
- **量測（實跑，非推論）**：
  - `bash scripts/verify-sector-allocation-closure.sh` ⇒ `FAIL: manifest docs/manifests/sector-allocation-simulation-closure-manifest.md not found` ⇒ **rc=2**。
  - **依賴檔去向**：`docs/manifests/` 的兩個 manifest 都在 **#1255**（`bcc06abc`「docs governance overhaul」）被移出 `docs/`；
    現存 `.omo/manifests/`（**gitignored**：`.gitignore:67-68`）。`scripts/ci/check_docs_governance.sh:9-10` 明文
    「`docs/manifests/` 只允許 README.md + TEMPLATE.md；個別 manifest 必須放 `.omo/manifests/`」
    ⇒ **搬回去會讓 `make ci` 變紅**（而 `.omo/` 在 fresh clone / CI 不存在 ⇒ 指向它也一樣 exit 2）。
  - **呼叫端 = 0**：`grep -rn 'verify-sector-allocation-closure'`（排除 `.omo/` 歷史計畫）只命中
    `cmd/experimental/sector-allocation-closure-preflight/main.go:206-214` 的**字串訊息**（該檔**無** `exec.Command`）與本檔自身。
    且 `scripts/*.sh` 不在 `make ci` 的 glob 內（`Makefile:429` 只掃 `scripts/ci/check_*.sh`）。
  - **第 2 個獨立損壞點（比依賴檔更早）**：`b252b10c`（#1250）把 `check()` 的 `eval "$@"` 改成 `"$@"`，
    但呼叫端仍傳**整串 shell 字串**（`check "07 …" "grep -qE '…' '$MANIFEST'"`）⇒ `result=$("$@" 2>&1)`
    把整串當命令名 ⇒ **rc=127**，配 `set -e` 在第一個 check 就中止。
    最小重現：`bash -c 'set -euo pipefail; r=$("grep -qE x /etc/hosts" 2>&1); echo after'` ⇒ 未印 `after` 即離開（127）；
    改回 `eval` 則 PASS。
    ⇒ **#06–#17 這 12 條自 #1250 起就從未執行**（早於 #1255 的檔案搬移）。17 條中真正跑得動的只有
    「SA00–SA12 表格列存在」的 grep 迴圈，而它只印 stdout、不計入 `passes`。
  - **17 條逐項量測**（把 manifest 複製回 `.omo` 並把 `eval` 模擬回來後的「假想結果」：13 列 + 12 條 check **全 PASS** ——
    但這些 PASS 全是空的，見最後一欄）：

| # | 斷言 | 資料來源 | 現在還適用嗎 |
|---|---|---|---|
| 01–05 | manifest 有 `SA00`–`SA12` 共 13 個表格列（腳本標為「Check 1-5」）| `.omo` manifest（gitignored）| **不適用**：資料源在 CI 不存在；SA00–SA12 已全數 shipped（`internal/sectorallocation/`、`docs/specs/sector-allocation-simulation-closure-spec.md` 均在版控）|
| 06 | SA12 狀態 done/implemented | 同上 | **不適用**（同 01–05）|
| 07 | SA06 done | 同上 | **不適用** |
| 08 | SA08 done | 同上 | **不適用** |
| 09 | retail manifest F05 為 `**done**` | `.omo/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md`（gitignored）| **不適用**：外層 `[[ -f ]]` 讓它在檔案不存在時**連 skipped 都不算**（永遠靜默跳過）|
| 10 | manifest 無 `source=empirical` | 同上 | **已被 Go 取代**：`SourceHeuristic` 預設 + 單元測試（`internal/config`、`internal/sectorallocation`）|
| 11 | manifest 無 `calibration_status=calibrated` | 同上 | **已被 Go 取代**（同上；`internal/config/parameters.go:1939-1941` 型別註記 + tests）|
| 12 | manifest 有 `## Binding Invariants` | 同上 | **不適用**：純文字結構檢查，對象是 gitignored 檔 |
| 13 | manifest 含 `SA-INV-20` | 同上 | **標的已不存在**：`SA-INV-20` 在 origin/main **只出現在本腳本**（spec 與程式碼皆無）|
| 14 | manifest 含 `ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED` | 同上 | **已被更好來源取代**：`configs/allowed_env_vars.md:51`（受版控）+ `cmd/atlas/main.go:2853`；但 repo **沒有**「env 變數是否登記」的自動檢查 |
| 15 | manifest 含 `SA-INV-11` | 同上 | **標的已不存在**（同 13）|
| 16 | `scripts/ci/sa12-negative-evidence.sh` 存在 | 檔案系統 | **假信心**：該腳本存在但**零呼叫端**（唯一引用者就是本檔）且自身 2 條 FAIL ⇒ 已由 **E9** 覆蓋（不重複派遣）|
| 17 | `internal/orchestrator/composition/root_test.go` 存在 | 檔案系統 | **已被更強機制取代**：`go test ./...` 真的會執行它；存在性不是守門 |

- **決定：(b) 明示停用**（`scripts/verify-sector-allocation-closure.sh` 改為 no-op，印出 `⛔ 已停用` 理由後 `exit 0`），理由：
  1. **(a) 的依賴檔無法合法存在**：唯一的 manifest 路徑受 docs governance 明文禁止，`.omo/` 又不受版控 ⇒ 任何「修好並接線」的版本在 CI 仍會 exit 2，只會製造第二個假 gate。
  2. **語意已被取代**：#10/#11 有 Go 層鎖定 + 測試；#17 由 `go test ./...` 真執行；#13/#15 的標的（`SA-INV-11/20`）在 repo 內不存在；#01–#09 的資料源在 CI 不存在。
  3. **接點受佔用 / 需政策決定**：`Makefile` 與 `.github/workflows/quality.yml` 由 E1 lane 佔用（§1）；若要真接線，最自然的接點是 `scripts/ci/check_*.sh` glob ⇒ 但那需要先有一個**受版控**的驗證標的（政策決定，非本票範圍）。
  4. **不留靜默 exit 2**：停用後任何人不小心手動跑它都會看到明確的 `DISABLED` 訊息（不再有「它好像有在守」的錯覺）。
  5. 附帶：`cmd/experimental/sector-allocation-closure-preflight/main.go` 的訊息原本宣稱「enforced by closure verifier」——
     那是**假的**，已同步改成誠實描述（並註明該 preflight 項是永遠回報 OK 的 stub）。
- **同族掃描（`scripts/*.sh`（非 `scripts/ci/`）＋ `tasks/*.sh`；只登記、不修）**：
  - `tasks/*.sh` **不存在**（`tasks/` 只有 2 個 `.md`）⇒ 該半邊 N/A。
  - 判定法：受版控檔案中的「執行路徑呼叫端」＝ `Makefile` / `*.sh` / `*.yml` / `*.yaml` / `*.go`（排除文件與自身）；
    另做 `bash -n` 全檢，並對**安全子集**實跑取 rc。
  - 全 28 支 `bash -n` 皆 OK（無語法錯）。清單：

| 腳本 | 執行路徑呼叫端 | 實跑 rc | 判定 |
|---|---|---|---|
| `verify-manifest.sh` | 1（**呼叫者 `verify-atlas.sh` 本身是孤兒**）| 2（無參數＝usage）| ⚠️ **遞移孤兒**；docstring 仍指向 docs governance 已禁的 `docs/manifests/*.md` |
| `verify-atlas.sh` | **0** | 未跑（`go build/vet/test` 全量；fresh worktree 另有 `//go:embed` 前置）| ⚠️ **孤兒**（「一鍵驗證」卻無人叫）|
| `coverage.sh` | **0** | 未跑（`go test ./...` 全量）| ⚠️ **孤兒**（60% 門檻與 CI coverage gate 重複，見 E1 lane）|
| `daily-twse-fetch.sh` | **0** | 未跑（連外抓 TWSE＋寫 `data/`）| ⚠️ **孤兒**（cron/compose 皆未見）|
| `install-soak-automation.sh` | **0** | 未跑（`launchctl` 會動本機 LaunchAgents）| ⚠️ **孤兒** |
| `reflexivity_report.sh` | **0** | 未跑（`gh` 連外）| ⚠️ **孤兒** |
| `sync-darwinian.sh` | **0** | 未跑（`ssh`/`rsync`/`docker`）| ⚠️ **孤兒**（跨機同步 ⇒ 疑退役 iMac 遺留，與 **E10** 同族）|
| `prism_manage.sh` | **0** | 0（`status`）| 孤兒（手動 ops 工具）|
| `spawning_manage.sh` | **0** | 0（`status`）| 孤兒（手動 ops 工具）|
| `generate_replay_data.sh` | **0** | 0 | 孤兒但可跑（產 sample replay CSV）|
| `cleanup-manifests.sh` | **0** | 0（fresh worktree 無 `.omo` ⇒ 直接 no-op）| 孤兒（harness 私有工具）|
| `darwinian_adjust.sh` | 2（`docker-compose.yml`、`docs/operations/docker-compose.crons.yml`）| **1**（`--dry-run`：`configs/darwinian_weights.json` 不存在）| ⚠️ 檔頭自述 **DEPRECATED stub**，卻仍掛在 cron compose ⇒ 「排程有、實際不做」（**同族：gate 看似存在**）|

- **驗收條件（要真的重啟才算 done）**：
  1. 驗證標的改讀**受版控** artifact（例：`docs/specs/sector-allocation-simulation-closure-spec.md` 的契約段、
     `configs/allowed_env_vars.md` 的 env 登記、以及 `internal/sectorallocation/*_test.go` 的存在＋真的被 `go test` 跑），**不得**讀 `.omo/`。
  2. 以 `scripts/ci/check_*.sh` 命名接入（自動進 `Makefile:429` 的 glob），或由既有 workflow job 明確呼叫。
  3. **負對照**：把被驗的 artifact 弄壞（例：把 spec 的契約段移除）⇒ 該檢查**必須變紅**；修好 ⇒ 變綠（兩向都要有 log）。
  4. 停用橫幅與本票同時移除（不得留著 no-op 又宣稱有 gate）。
- **不可動（已遵守）**：`Makefile`、`.github/workflows/quality.yml`、`docs/reference/traps.md`（他人/他線佔用）；
  未動 production、未 merge、未放寬或刪任何測試。

## 判讀註記（讀告警與做驗收前必讀）

以下三則不是待辦，而是**判讀規則**：已實際造成過一次誤判（含 root 本人），所以寫進登記表。

1. **部署後 app 指標 family 會消失，直到下一次排程執行** ⇒ 驗收時看到
   `curl -s localhost:18080/metrics | grep -c atlas_universe` 回 **0 不等於 wiring 壞掉**
   （原因與證據：FU-20260925-07）。判讀時要同時看 Prometheus TSDB 是否還留有舊樣本、
   以及 snapshot（`data/state/universe_snapshot.json`）。
2. **手動執行 `-build-universe run` 不會更新 Prometheus 的 `atlas_universe_*`**
   （原因：FU-20260925-09）⇒ 「手動跑成功」與「監控看到活動」是兩件事，不可互相證明。
3. **三條 universe 告警在 metric 缺陷期間的預期分類**（2026-09-25 實證，07:31:50Z：
   `ranked=150`、`quotes_returned=1301`）：
   - `AtlasUniverseRankedZero` ＝ **真陽性**
   - `AtlasUniverseScreeningAllRejected` / `AtlasUniverseQuotesMissing` ＝
     **metric 缺陷期間的預期假陽性（unexpected-false-positive）**
   判讀時先確認 metric 面的缺陷狀態，再決定這三條是訊號還是假象；
   任務 O 新增的第 4/5 條同理（它們的 peer gate / 分工就是為了不製造新的假象）。
   ⚠️ **適用範圍（2026-09-25 更新）**：這段只適用於**修復前的歷史判讀** ——
   counter 灌爆與 label 配對缺陷已由 **#1989** 於 2026-09-25 修復。
   修復之後若又看到 `AtlasUniverseScreeningAllRejected` / `AtlasUniverseQuotesMissing`，
   那是**真訊號**，不要再用這條註記把它當假陽性。

---

## 已定案的判準（決策紀錄）

### 2026-09-25 — secret-scan 的 block/warn 分界與降噪原則（任務 Q；業主明確背書）

- **可部署設定檔（`*.yml`/`*.yaml`，非 `*.example*`/`*sample*`/`*template*`）→ block 級，即使路徑在 `docs/` 底下。**
  理由（因果）：`docs/operations/*.yml` 是**會被 `docker compose` 拿去跑**的設定，不是散文；
  在那裡出現字面憑證就是真外洩 —— 本次外洩（`docker-compose.prod.yml` 的明文 DB 密碼）
  能長期存活，正是因為它當時落在 warn-only 類別。
- **散文 `.md`、範例／模板 → 維持 warn-only。**
  理由（反脆弱）：把散文升成 block 只會逼人不停加 allowlist；**allowlist 一多，護欄就會被繞過**。
- **降噪設計刻意不排除 `host.docker.internal`**：那正是 prod DSN 的主機，必須保持會被抓到。
  （排除的是 RFC 2606 保留域名／本機位址／placeholder 字／程式碼取值／純字母且 <16 字的假 key／路徑型 env 預設值。）
### 2026-09-25 — 測「密碼認證」之前，必須先確認 `pg_hba` 的認證方式，且**必須有負對照**（元教訓）

- **實例**：本次憑證盤查的第一版在容器內用 `-h 127.0.0.1` 測密碼，得到「兩個值都可登入」的
  **無效結論**——因為 `pg_hba.conf` 對 `127.0.0.1/32`、`::1/128` 是 **`trust`**（免密），
  測試根本沒走到密碼驗證。是**負對照**（拿一組明知錯誤的密碼去測，結果也「成功」）把它抓出來的。
- **正確作法**：目標用**容器自身 IP**（才會命中最後一行 `host all all all scram-sha-256`）；
  而且任何「認證被拒」的觀測都要附**有效負對照**（已知錯誤的憑證也必須被拒）才算證據。
- **一般化**：宣稱「X 有效／無效」之前，先證明**測試本身有鑑別力**（能對錯誤輸入說不）。
  這是本 repo 同日反覆出現的同一族缺陷（false-green / vacuous test），也是「誤判比漏抓更危險」的來源。

- **`check_monitoring_single_source.py` 的 R2 用行掃描、不引入 YAML 依賴**：GitHub runner 不保證有 PyYAML；
  限制已明列於該檔檔頭，且生產外側仍有 a2a-dev `scripts/drift-check.sh [9/9]`（`docker inspect`）作為第二道。

---


## 相關文件

- [universe-scoring-ranked-zero-20260925.md](universe-scoring-ranked-zero-20260925.md)
  — SmartUniverseBuilder `symbols_ranked=0` 根因報告（含 §6.2 缺口與 §8 防再犯檢查建議）
- [README.md](README.md) — 本目錄索引
- [local-deploy.md](local-deploy.md) — 部署與 `.env` 分工（prod DSN 為何不放 `.env`）
