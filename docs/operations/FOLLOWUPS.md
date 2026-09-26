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
  記錄本身隨 PR #2006 交付；下方「殘留面」三項仍是 `open` 的後續硬化方向，故本條狀態維持 `open`。
- **狀態**：`open`（僅指下列三項殘留面；實作／驗收部分已完成）
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
- **殘留 1（跨行程可見性）**：latch 寫在 `data/state/finmind_daily_quota.json`，但**只在 client 建構時讀取**
  ——同一台機器上**已存在**的其他行程（例如另一顆 cron 容器、或長命 process 內另一份 client）
  不會立即看到別人的 latch，要等它自己撞一次 402 才會跟上。成本有界（每個行程浪費 1 次呼叫），
  但「一次 402 就全平台停手」的性質只在單一共享 process/state dir 下成立。
  **硬化方向**：latch 寫入後以短 TTL（例如 30s）重讀 state file，或把 latch 暴露成共享訊號
  （檔案 mtime / metric），讓多行程在一個週期內收斂。
- **殘留 2（上游真實上限未知）**：12,500 是**觀測到的拒絕點**，不是 FinMind 公布的上限；
  12000 這個 ceiling 是人工留 500 餘裕的估計值。目前以 `FINMIND_DAILY_LIMIT` 覆寫 +
  `finmindObservedUpstreamRefusalLimit` 常數 + 測試（`TestFinMindQuotaCeiling_StaysBelowObservedUpstreamRefusal`）
  把「不得超過觀測拒絕點」寫死，但**沒有自動校準**。
  **硬化方向**：連續多日記錄「首次 402 時的 calls_today」並回報，作為下一次調整 ceiling 的證據。
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

---

---

---


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
