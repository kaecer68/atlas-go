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

### FU-20260925-04 — 部署後複驗：最早可複驗日＝2026-09-29（週二）06:00Z（原訂 09-28，已更正）

- **狀態**：`open`
- **記錄日期**：2026-09-25
- **為何是這一天**：`auto_universe_refresh`（daily）**顯式跳過週一**
  （`internal/monitoring/universe_scheduler.go` 的 `now.Weekday() == time.Monday` 分支），
  週一由 `auto_universe_full_rebuild`（weekly）跑。所以要驗「修好後的常態」必須挑一個
  06:00Z（= 14:00 台北；容器不設 TZ，見 FU-20260925-05）**且是交易日**的日子 ——
  ~~2026-09-28 是最近的這種日子~~ **✗ 已被推翻，見下方「更正（2026-09-26 第二次）」**。
- **❗ 更正（2026-09-26 第二次；原訂日期作廢）**：`2026-09-28`（週一）是
  **孔子誕辰紀念日／教師節，依規定放假 1 日 ⇒ 休市** ⇒ 該日**不會產生新的交易 session**，
  不可能滿足本條驗收條件 ⇒ **原訂的「2026-09-28（週一）06:00Z」作廢**；
  **最早可複驗的交易日＝2026-09-29（週二）06:00Z**（下一班車）。
  更正原因：前一版把 09-28 當成交易日，讀的是 `internal/taiwanholidays` 的 **`IsTradingDay`** 欄位；
  當時（以及現行 main）的假日表**缺**教師節 ⇒ 該函式對 09-28 回 **`true`** ⇒ **是資料缺口，不是探針答錯**。
  權威來源改以 **TWSE 官方 API** 為準（2026-09-26 實測，`stat:"ok"`，標題「115 年市場開休市日期」）：
  `curl -sS 'https://www.twse.com.tw/rwd/zh/holidaySchedule/holidaySchedule?response=json&date=20260101'`
  ⇒ `["2026-09-28","孔子誕辰紀念日/ 教師節","依規定放假1日。"]`；清單內**無** 09-29 ⇒ 09-29 為交易日。
- **根因（2026-09-26 於 main `0ce6e48f` 實測；一次性探針，跑完已刪）**：
  `IsTradingDay` 的資料來自 `internal/taiwanholidays`（`fixedHolidays` ＋ `adjustedHolidays` ＋ 農曆表），
  而 `fixedHolidays` **不含**教師節、`adjustedHolidays[2026]` **也不含** 09-28 ⇒
  `2026-09-28 Mon IsHoliday=false IsTradingDay=true`（**錯**）；
  同日 `2026-12-25 Fri IsHoliday=false IsTradingDay=true`（**亦錯**，行憲紀念日放假1日）；
  `2026-09-29 Tue IsTradingDay=true` ✓、`2026-10-05 Mon IsTradingDay=true` ✓。
  ⇒ **修法在 SSOT（`internal/taiwanholidays`），不在排程的判斷式**；由 open PR **#2054**
  （`fix/holiday-aware-universe-gate`：補 2025/2026 休市日 ＋ weekly 假日 gate）承接。
- **⚠️ 排程事實（勿讀成「該日一定什麼都不會跑」）**：現行 main 的 `isTradingDay` 仍是 **weekday-only**
  （`internal/monitoring/universe_scheduler.go`）⇒ **2026-09-28（週一）weekly 仍會執行**，
  但休市日不可能產出可信排名 ⇒ 該日 snapshot **不可當複驗證據**。
  要「daily 與 weekly 都不執行」需等 **#2054** 合併（daily 改 `marketdata.IsTaiwanTradingDay`、weekly 加假日 gate）。
  另外 **09-29 是週二 ⇒ 走 daily（增量）路徑**；若要一併驗 **weekly（全量重建）**路徑，
  下一個「是交易日的週一」＝ **2026-10-05 06:00Z**（TWSE 2026 清單中 10-05 無休市）。
- **日曆更正（2026-09-26 記錄）**：**09-26 不可當成複驗日** —— 2026-09-26 是**週六**，且
  **2026 中秋＝09-25（週五，休市）** ⇒ ~~09-24（四）之後的下一個交易日就是 **09-28（一）**~~
  **✗ 此句已被推翻：09-28（一）本身也是休市日 ⇒ 09-24（四）之後的下一個交易日是 09-29（二）。**
  權威判定（`internal/taiwanholidays`，以一次性探針 `go test ./internal/taiwanholidays/` 產出後**已刪除**）：
  `09-24 Thu true`／`09-25 Fri IsHoliday=true`／`09-26 Sat false`／`09-27 Sun false`／`09-28 Mon true`。
  ~~⇒ 本條的 `2026-09-28（週一）06:00Z` **維持有效**；~~
  **✗ 該結論已於同日（第二次更正，見上）被 TWSE 官方 API 推翻：09-28 為休市日。**
  誤判來源＝把 `IsTradingDay` 的 `true` 讀成「是交易日」，而該函式當時的假日表缺教師節。
  該時刻（2026-09-26T03:40Z 撰寫時）**尚未到** ⇒ 狀態仍維持 `open`
  （本條**尚未**複驗，勿把 09-25/09-26 當成已複驗的日子）。
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

- **狀態**：`done`
- **完成於**：2026-09-26 ~ 2026-09-30（本條自述的判準「守門＋持久化＋可見性＋runtime 寫入端遷移全部合併並部署後才 done」已滿足）
  - 合併：`f90258e7`（PR [#2013](https://github.com/kaecer68/atlas-go/pull/2013) 逐參數 sanity floor ＋ 校準值持久化／可見性）、
    `af7a45af`（PR [#2017](https://github.com/kaecer68/atlas-go/pull/2017) 所有 runtime 校準寫入端改走 calibrated overlay）、
    `b1fc524d`（PR [#2039](https://github.com/kaecer68/atlas-go/pull/2039) CLI／容器校準寫入端改走 overlay，收尾之二）；
    另有 `17a3eede`（#2076：校準新鮮度改標的 overlay ＋ image-vs-effective drift 偵測）
  - 部署：2026-09-26 之後生產已多次重建／重啟（含 2026-09-30 08:08 本輪窗口）
  - 現況讀數（2026-09-30，生產唯讀）：`atlas_calibration_freshness_ok{artifact="parameters_overlay"}=1`（新鮮）、
    而 `{artifact="parameters"}`＝0（年齡約 86 天，屬 repo 檔案的設計狀態，非 overlay 路徑失效）
  - 條目正文的「PR：`fix/calibration-drift-floor-and-overlay`；尚未合併／部署」為**當時敘述**，已由上述三筆合併取代
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
3. **其餘 runtime 寫入端（2026-09-26 第二批，`feat/calibration-overlay-all-writers`）**：全部改走同一個 overlay ✓
   - `internal/config/calibrator.go`（`CalibrateParameters`，5 個 task 呼叫者）→ named entries
   - `internal/portfolio/factor_weight_calibrator.go`（`applyFactorWeights`）→ dotted path
     `factor_weight.base_weights.value`（整張 8 因子表；SSOT 驗證要求完整集合，故不可部分 patch）
   - `internal/retail/calibration.go`（`CalibrateRSITw`）→ dotted path `rsi_tw.<param>.value` ＋
     `rsi_tw.last_calibrated_score.*`
   - `internal/industry/data_aggregator.go`（`RecalibrateThresholds`，任務 ＋ admin route）→ dotted path
     `industry.cycle_thresholds.source/calibrated_at/value.<industryID>`；**簽章改為 `RecalibrateThresholds(revenuePath)`**
     （不再接受 configPath ⇒ 未來呼叫者無法不小心寫回 SSOT）
   - overlay 文件格式升級為 **v2**：條目可為 named tunable（純量）或 **dotted JSON path**（任意 JSON），
     . 兩者共用同一份文件／同一組 fail-closed 規則（未知鍵/路徑移除＋WARN、SSOT 基準變動 ⇒ 失效移除＋WARN、
     未註冊路徑＝完全停用）。dotted path 規則：**容器必須存在、落葉可新增**（打錯段落名會被擋）。
4. **CLI／容器寫入端（2026-09-26 第三批，`feat/calibration-overlay-cli-writers`）**：
   - **真缺口（容器內執行）**：`seasonal_calibration` 背景任務（7d）以**子行程**執行 `calibrate-seasonal -update`
     （`internal/scheduler/seasonal_task.go`；映像內只有這一支 CLI：`Dockerfile` `COPY …/calibrate-seasonal /app/`）
     ⇒ 它原本會寫 `/app/configs/parameters.json`（**不在 bind mount**）✗。
     處置：新增 `-writeback` flag（`ssot` 預設／`overlay`）＋ 生成端**強制** `-writeback=overlay`（有測試斷言 args ✓）。
   - **同一個 flag 也給其他 CLI**（`calibrate-rsi-tw`、`calibrate-thresholds`、`calibrate-parameters`、
     `backfill-industry-tree`）：預設 `ssot`（人工在 checkout ⇒ 可審查 git diff ✓ 不變），需要時可 `-writeback=overlay`。
     ⚠️ 這四支**不在** image 內（Dockerfile 只 ship `calibrate-seasonal`）⇒ 目前只能人工執行 ✓；
     給 flag 是為了「若日後 ship 進映像」時不必再改一次 ✓，而不是現行缺口。
   - `auto_calibrate` 任務（`go run ./cmd/calibrate-parameters`）已加 `--writeback=overlay` ✓；但該路徑在容器內
     **本來就跑不動**（映像無 Go toolchain）⇒ 這是防護，不是行為變更（未把它改成 in-process ✗，避免**默默啟用**一個
     從未在生產跑過的校準）。
   - overlay 路由的共用實作：`internal/config/calibration_writeback.go`（`ParseCalibrationWriteback`／
     `RegisterForWorkDir`／`WriteDocumentOverlay`／`WriteConfigOverlay`／`DiffParametersDocuments`）：
     以「本次載入的 SSOT 文件」為 baseline 做 JSON diff ⇒ 只落**被改到的葉節點**；未註冊 overlay 路徑而要求
     `overlay` 時**高聲失敗**（不靜默回退寫 SSOT ✗）；條目名稱含 `.` 的路徑無法定位 ⇒ WARN 列出（不亂寫）。
   - **修正一項事實**：`docker-compose.yml` 的 `cron-darwinian`（每日 09:00）跑的是 `scripts/darwinian_adjust.sh`，
     該檔自述 **DEPRECATED（2026-08-17）** 且寫的是 `configs/darwinian_weights.json`（**不是** `parameters.json`）
     ⇒ 與本條無關 ✓（本條的容器內證據是 `seasonal_calibration` ✓）。
5. **刻意仍寫 SSOT（不是缺口）**：
   - CLI 工具**預設**行為（人工在 checkout 執行）：`-writeback` 預設 `ssot` ⇒ 寫入是**可審查的 git diff** ✓。
   - admin parameters API（`POST /api/parameters`、`/api/parameters/rollback`）：人工編輯 SSOT 的介面 ✓。
6. **生產不可達、刻意未遷移（不為死碼整齊而動）**：
   - `internal/orchestrator/calibration_engine.go` 的 `ApplyToConfig`/`ApplyToConfigPath`：**無任何 production caller**
     （`grep -rn 'ApplyToConfig' internal/ cmd/` 只回定義；`CalibrationEngine` 只被用於 `Calibrate`）。
   - `internal/scheduler/auto_rollback.go:274`（`RestoreFromBackup(GetParametersConfigPath())`）：由
     `checkCalibrationDegradation` 觸發，而 `AutoRollback.RecordCalibration`（唯一設定 `lastCalibSnapshot` 者）
     在 production **無呼叫者**（只有測試）。
   - ⚠️ 若日後有人接線這兩條，**必須**改走 overlay（否則本條缺口原地復活）。
   - `docs/calibration-loop.md` 已同步記錄「誰還寫 SSOT（刻意的）」清單。
7. **admin API 與 overlay 的互動（僅記錄，未改）**：`POST /api/parameters` 的讀寫基準是
   `h.params`（SSOT 視圖）⇒ **不會**把 overlay 值寫進 SSOT ✓。但 `POST /api/parameters/rollback`
   走 `SnapshotStore.RollbackToSnapshot`（直接換掉**單例**＝effective 那份）再把單例寫回 SSOT
   ⇒ 該次 rollback 會把當下 overlay 的值一併寫進 SSOT；之後 overlay 條目會因「SSOT 基準變動」而失效
   （charter 勝）✓ 不會卡死，但被 overlay 的鍵會回到 snapshot 的內容。若要語意精確一致，
   需讓 rollback 同時處理 overlay（後續）。

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

- **狀態**：`done`
- **完成於**：2026-09-27（PR **#2049**，main `0ce6e48f` → 後續 `14a3da03`）
- **實作**：`.githooks/pre-push` 新增 **Gate 0b** `bash scripts/check-binary-freshness.sh --host-only --diff-base origin/main`；`--host-only` 完全不碰 docker、`--diff-base` 重用既有 `BUILD_INPUT_PATHS`（predicate 不會漂移）；只判 `bin/atlas`＋`bin/atlas-mcp`；純 docs push 不擋；**無 `bin/` 的 clone 軟跳過**。附 hermetic 契約測試 `tests/scripts/test-prepush-gates.sh`（7 組案例，含「同一組 binary 不給 `--diff-base` 時 rc=1」的控制組）＋ mutation test。
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

- **狀態**：`done`
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
### FU-20260926-18 — E8：`internal/config` 的 `WalkDir("internal")` 偶發假紅（apigateway 測試在 repo 樹內寫／刪相對 `data/`）

- **狀態**：`done`
- **記錄日期**：2026-09-26
- **完成於**：本 PR（`fix/20260926-flaky-and-sa12`）
- **徵狀**：`go test ./...` 偶發紅燈，且失敗的是**無關的** package：
  `internal/config/parameters_shadow_declarations_test.go:104` →
  `walk .../internal: open .../internal/apigateway/data: no such file or directory`。
- **根因（兩段；已用「flapping dir」在 1.9s 內確定性複現，非猜測）**：
  1. `internal/apigateway/register_adapters.go` 的 `saveSnapshot(channelID, data)` 以**相對路徑**
     `data/state/<channelID>/latest.json` 寫檔（= process CWD；`go test` 下 CWD 是 package 目錄）
     ⇒ adapter 測試把 `internal/apigateway/data/…` 寫進 repo 樹。
  2. `internal/apigateway/adapter_finmind_util_test.go` 的 `TestSaveSnapshot` 用
     `defer os.RemoveAll("data")` 把整個目錄刪掉。
  ⇒ 跨 package 並行時，config 的嚴格 walker 會撞上「目錄存在又消失」的窗口（`WalkDir` 的 readdir
     得到 ENOENT）⇒ **假紅**。同型嚴格 walker 另有 `internal/config/parameters_inert_declarations_test.go:85`。
- **修法（修根因；**未**放寬/刪除任何斷言、未動 config 的 walker）**：
  - `saveSnapshot(workDir, channelID, data)`：base dir **注入**；`workDir == ""` 時**拒絕寫入**（warn），
    不退回 CWD 相對路徑。
  - finmind / fugle / fubon adapter 建構子收 `workDir`；fubon 自癒註冊路徑改用
    `Gateway.WorkDir()`（`Gateway` 新增 `workDir` 欄位＋accessor）。
  - 測試端全面注入 `t.TempDir()`（adapter work dir ＋ quota state dir），移除 `RemoveAll("data")`；
    `TestSaveSnapshot` 改斷言「寫在注入目錄」＋「CWD 下不得出現 `data/`」＋「空 workDir 必須拒絕」；
    三個 adapter 的 Fetch 測試新增「snapshot 真的落在注入目錄」斷言。
  - 同類修正：`GetSharedTEJClient` 收 `stateDir`（與 finmind/fugle 一致）、新增
    `NewFugleClientWithStateDir`；`register_adapters.go` 一律傳 `filepath.Join(workDir, "data", "state")`。
  - 找出並修掉第二個 writer：`adapter_tsmc_revenue_test.go` 先建 provider 才建 shared client，
    而 `GetSharedFinMindClient` 的 state dir 由**第一次呼叫**（sync.Once）決定 ⇒ 被釘在相對
    `data/state`。改為先建 shared client（注入 temp dir）再建 provider。
    （以 509 個 top-level 測試逐一掃描確認：清空後只有這兩個測試會產生 `internal/apigateway/data`。）
- **驗收**：`go test ./internal/... -count=1` 連跑 3 次全綠；跑完 `internal/apigateway` ＋ `internal/config`
  後 `internal/apigateway/data` 不存在、`git status` 乾淨（見 PR body）。
- **殘留（同類、未修；另需決策）**：`internal/marketdata` 的**自有**測試仍會留下
  `internal/marketdata/data/state/*_daily_quota.json`（`NewFinMindClient` / `NewFugleClient` /
  `NewTEJClient` 的預設 state dir 是相對 `data/state`）。**沒有 deletor** ⇒ 不造成假紅，
  但屬同一類「測試污染 repo 樹」；要清掉需在該 package 的測試全面注入 temp state dir。

---

### FU-20260926-19 — E9：`sa12-negative-evidence.sh` 在 main 固定 2 條 FAIL 且**沒接進任何 gate**

- **狀態**：`done`
- **記錄日期**：2026-09-26
- **完成於**：本 PR（`fix/20260926-flaky-and-sa12`）
- **量測（改前，main；`bash scripts/ci/sa12-negative-evidence.sh` ⇒ exit 1，10 PASS / 2 FAIL）**：
  - `FAIL 08 unversioned CapitalFlowAction (found=3, expected=2)`
  - `FAIL 09 synthetic ranking literal (found=5, expected=1)`
- **根因（腳本側）**：舊檔名不符 `make ci` 的 `scripts/ci/check_*.sh` glob，也不在任何 workflow 內；
  2026-07-19 建立後**從未被任何 gate 執行**，因此「命中檔案數 == N」型斷言腐化而無人察覺。
- **判定**：兩條 FAIL 皆為**過時期望（假陽性）**，N 是被**合法的後續程式碼**撐破：
  - 08：2026-07-27 (#1372) 起 `internal/orchestrator/strategy_evolver.go:611` 開始消費 canonical
    的 `sectorallocation.CapitalFlowActionRiskOn`（SA-INV-09 想要的方向）⇒ 3 檔。
    真正的風險（出現第三份 taxonomy、或 deprecated `capitalflow.*` enum 長出消費者）**不存在**。
  - 09：2026-07-24 (2282122b) 之後陸續有檔案合法標註或**排除** synthetic 列
    （F06「只使用 non-synthetic outcome」，實作見 `internal/portfolio/darwinian_period_matrix.go:115`）
    ⇒ 5 檔；風險（synthetic 餵進 ranking）**不存在**。
- **二選一決定：採 (a)「修到 PASS 並接進既有 job」**（不是停用）：
  - 08 改量「定義點恰好兩份」；新增 08b「deprecated `capitalflow.CapitalFlowAction*` 零 production 消費者」；
    09 改量「`"synthetic"` 不與 rank/score/weight 同現」；新增 09b「F06 排除守門存在（>=1，`atleast`）」。
    改後 14/14 PASS（`bash scripts/ci/check_sa12_negative_evidence.sh` ⇒ exit 0）。
  - 接線：**改名** `sa12-negative-evidence.sh` → `scripts/ci/check_sa12_negative_evidence.sh`，
    由 `make ci` 的既有 glob 自動納入；`make ci` 由 `make ci-full` 呼叫，而 `make ci-full`
    是 `.githooks/pre-push` 與 PR lifecycle 的 MUST gate。**未動 `Makefile`／`quality.yml`**（本輪由他 lane 佔用）。
  - **已知限制（已寫進腳本檔頭）**：GitHub Actions 端**沒有任何 workflow 呼叫 `make ci`**
    （實測 `grep -rn "make ci" .github/workflows/` 無命中）⇒ 這是 local/pre-PR gate，
    不是 GitHub required check。要讓它在 GH 端變紅燈必須改 `quality.yml`（本輪已被佔用）⇒ 留待後續。
- **附帶發現（未修；需另一次決策）**：同族的 `scripts/verify-sector-allocation-closure.sh` 自 2026-07
  (#1255 manifest lifecycle) 起就 **exit 2**：它依賴的 manifest（原本放在 `docs/manifests/`
  之下、檔名為 `2026-07-18-sector-allocation-simulation-closure` ＋ `.md`）已被刪除且未歸檔 ⇒
  該 verifier 的 17 條檢查自那時起實際上沒有跑過（含 check 16 的檔案存在檢查；本 PR 已更新其檔名）。


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

---

### FU-20260926-24 — `internal/startup` 的 `TestPreflight_MixFreeAndForeign_ReturnsFirstForeign` 埠競爭假紅：**「free」位址是 reserve→release 出來的**（TOCTOU）

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：另一條 lane 於 2026-09-26 多次被它擋在 `make ci-full`（既有繞道 `PRE_PUSH_FULL=never`，不緊急）；本票**只登記、不實作**（避免同時開太多戰場）。
  **與 `FU-20260926-17`（fubonproxy）不同族**：那條是「寫死上限落在工作分布內／單次抽樣」的**機率性**紅；
  本條是**位址真的被佔用**（reserve→release→re-check 的 TOCTOU）＋「佔用者查詢」的第二個時間窗。
- **事實**：
  - 症狀（外部 lane 實測）：`TestPreflight_MixFreeAndForeign_ReturnsFirstForeign` ⇒
    `127.0.0.1:50379 is held by an unknown process`；負對照：單跑 `go test ./internal/startup/ -count=1` ⇒ `ok 1.44s`（exit 0）。
- **機制（本次親驗，非推論）**：
  - `internal/startup/preflight_test.go` 的 `freeAddr(t)`：`net.Listen("tcp","127.0.0.1:0")` 取到埠後**立刻 `ln.Close()`**，
    回傳的位址只在**該瞬間**是 free；測試稍後才呼叫 `Preflight()` 讓 `portprobe.Probe` **重新判定** ⇒ reserve→release→re-check 的 TOCTOU。
    而 `make ci-full` 跑的是 `go test … $(go list ./...)`（**全 package 並行**）⇒ 其他 package 的 test binary 也在高頻 reserve/release `127.0.0.1:0`，
    本測試剛釋放的埠完全可能在「釋放」與「重新判定」之間被（暫態）拿走。
  - 為何訊息是「**unknown** process」而不是帶 PID 的 foreign（`internal/startup/preflight.go:168-173` 的 `actionableForeignError`，
    `occupant.PID <= 0` 分支）：`portprobe.Probe` 是**先** `net.Listen` 撞 EADDRINUSE、
    **再**重試 `/health` 5 次 × 100ms（≈400ms，`portprobe.classifyOccupied`）**之後**才呼叫 lsof 取佔用者；
    若佔用者在那 ~400ms 內已釋放（暫態 listener），lsof 就找不到 PID ⇒ `Occupant{PID:0}` ⇒ 這句訊息。
    ⇒ 這裡其實有**兩個**時間窗：①位址被搶、②佔用者在查 PID 前消失。
  - 紅的是哪條斷言：該訊息是由**第一個（本應 free 的）claim `atlas-http`** 產生 ⇒ `Preflight` 回的是它的錯誤，
    測試的 `strings.Contains(err.Error(), "fubonproxy")` 就不成立 ⇒ 紅在 `expected first foreign error, got: …`（正是被引用的那個片段）。
  - **決定性探針（可複現；臨時測試檔已刪，`git status` 乾淨）**：在 package 內加臨時測試 ——
    `addr := freeAddr(t)` → goroutine `net.Listen("tcp", addr)` 持有 250ms 後 `Close()` → 呼叫 `Preflight`。輸出：
    ```
    PROBE addr=127.0.0.1:51922 err=atlas-http address 127.0.0.1:51922 is held by an unknown process; identify it with `lsof -nP -iTCP:127.0.0.1:51922 -sTCP:LISTEN` and stop it
    ```
    ⇒ 簽名**逐字相同**（含 `unknown process` 與 `lsof` 提示）⇒ 機制確定。
  - 位址族不一致（輔助成因）：`freeAddr()` 走 `127.0.0.1:0`（loopback），但 `occupyAddr()` 綁 `0.0.0.0:<port>`（wildcard）。
  - **自然重現未達成（照實）**：我外加「高頻 reserve/release `127.0.0.1:0` 的 churner」並 4 個 process 並行跑
    `go test -count=8 -run TestPreflight ./internal/startup/`（共 32 輪）⇒ **全部 exit 0、0 次 `unknown process`** ⇒ 自然紅是低頻事件，只能靠上面的探針證明機制。
- **為何先開票、不實作**：本條坐在 `make ci-full` 必經路徑上，但修法要改「free 位址怎麼取得」的測試設計；
  且今日多條 lane 正在同檔（`FOLLOWUPS.md`）編輯 ⇒ 先登記、由 root 排程。
- **建議方向（只建議，未實作）**：
  1. **首選：讓「free」不再靠 reserve→release**。`freeAddr` 回傳的位址改成測試用不到的**哨兵** `127.0.0.1:0`：
     已實測 `portprobe.Probe("127.0.0.1:0")` ⇒ `state=0`（free）、occupant 空、`err=nil`，且
     `Preflight([]PortClaim{{Component:"atlas-http", Addr:"127.0.0.1:0"}})` 連 9 次皆回 `nil`（`net.Listen(":0")` 每呼叫必成功 ⇒ 無 TOCTOU、不需佔用/釋放）。
     ⚠️ 這只適用於「**語意上只要 free**」的 claim（測試替身），不是 production 位址；`portFromAddr("…:0")==0` 也要留意（僅影響 zombie 分支的訊息）。
     foreign 那一側則改為**一次綁定、永不釋放**（`net.Listen("127.0.0.1:0")` 直接交給 `http.Server`，即 `internal/fubonproxy` 的 `bindEphemeralPort` 形狀），
     **不要** `freeAddr` 之後再 `occupyAddr` 重綁同一個埠。
  2. 或走既有 **seam**：`preflight.go` 已有 package-level `probeFn`（另有 `killFn`／`isFubonZombieFn`）⇒ 可注入
     「free claims 回 `StateFree`、foreign claim 轉呼叫真 `portprobe.Probe`」的 stub，完全移除真實埠競爭；
     並**保留**一個只含 foreign claim 的整合測試（真 listener、不釋放、測到 actionable error 與 PID）以維持真實路徑覆蓋。
  3. **禁止**：把真失敗吞成 `t.Skip`／`|| true`（真被佔用時**仍必須紅**）；也**禁止**只放大門檻。
     若最終仍要重試，必須限制在**明確界定的外部競爭**、且輸出能區分「真的被佔用（有 PID）」與「暫態佔用者已消失（PID=0）」。
  4. 順手統一位址族：測試佔用者改綁 loopback（與 `probeProxyPort`／`Probe` 先檢查的 `127.0.0.1:<port>` 一致），避免 loopback/wildcard 混用。
- **驗收條件**：在 `make ci-full` 的實際條件下（全 package 並行；建議**外加**高頻 `127.0.0.1:0` reserve/release 的 churner 當加壓器）
  該測試與整個 `internal/startup` package **連續 ≥10 輪 0 紅**；
  **負對照**：用真 listener 佔住某個 claim 的位址**並持續持有** ⇒ `Preflight` **仍必須**回報該 claim 的 actionable error（不得變 nil、不得 skip）。
- **不可動**：`.githooks/pre-push`（`FU-20260926-15`）、`Makefile` ci 段（`FU-20260926-12`／E3）、`quality.yml`、
  `docs/reference/traps.md`、`docs/operations/remediation-manifest.md`（另一 lane 的 SSOT）。

### FU-20260927-01 — replay 幻影列只剩「週末」可判：**週間休市（假日）的幻影列在 Prometheus 端結構上無法判別**（需交易日曆）

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：`monitoring/rules/atlas_replay_alerts.yml` 的 5 條新規則（PR #2063／`b1a1ee7d`）交付時**已知的界線**；規則檔自己已在 annotation 內註明（同檔 `:125-126`）。
- **事實**（規則檔與程式碼位置＝我實查;**生產數字已由 root 於 2026-09-27 的 CSV 清理親自複驗（`removed=396 (weekend=352 holiday=44)`）**）：
  - `AtlasReplaySessionDateInvalid`（同檔 `:93-104`）只有兩個 arm：`day_of_week(...) == 0|6`（`:100`／`:102`）與「資料日晚於 `time()`」（`:95`）。
  - 檔頭記載：2026-08-29~09-25 生產 replay CSV 有 **396 列**幻影，其中 **weekend=352／holiday=44**（同檔 `:21-22`、`:75-76`）。
    ⇒ 其中 **44 列（約 11%）是週間休市**，現行規則**抓不到**。
  - **為什麼不在規則端補**（不是懶，是結構限制）：PromQL 拿不到台灣交易日曆；同一份規則檔已因同一個限制把「CSV 落後」的門檻由 3d／72h 放寬到 14d（同檔 `:55-58`）。
- **最小修法建議（只建議，未實作）**：
  1. 在寫入端（`cmd/daily-replay-sync`）新增 counter `atlas_replay_nontrading_rows_total`，
     僅在「**實際寫入**的列,其資料日經 `marketdata.IsTaiwanTradingDay`（`internal/marketdata/calendar.go:49`）判為非交易日」時 `Inc()`。
  2. 規則加一條 `> 0`（severity 依 #2063 的既有分級）。
  - 用 **counter（已發生的事件）**而不是 gauge（狀態）：`#2057` 之後正常路徑**永遠 0**，任何非 0 都是缺陷；
    缺席的處理沿用第 4 條（`AtlasReplayFreshnessExporterDown`，同檔 `:218`）的 `absent()` arm 設計，不另立第二套心跳。
  - **不要**改第 1 條的 `day_of_week` arm：週末與週間假日是**互斥的兩個判據**，合併會讓 annotation 說不清是哪一種。
- **驗收條件**：注入一列週間假日資料 ⇒ 新規則 firing、第 1 條維持沉默；正常路徑連續 0；`promtool test rules` 有對應正向案例與負向對照。

---

### FU-20260927-02 — `atlas_channel_health_status` 的**值域對照表**仍散落,且 #2068 只修了一半（測試檔內部自相矛盾）

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：#2064（`b6fe7d8a`）把 `degraded` 的 gauge 映射由 **4 改成 1**；#2068（`b5108fbb`）修了規則檔文案。
- **已定案的值域（我實查程式碼,不是抄註解）**：
  - 映射：`cmd/atlas/channel_health_metrics_task.go:156-168` `healthStatusValue` ⇒ `ok=0`、`warn|stale|degraded=1`、`error=2`、`inactive=3`、其他 `=4`。
  - 語意：`internal/apigateway/channel_status.go:22`（`StatusDegraded = "degraded"`）＋ `:84-92`（Rule 2b：degraded 的**資料**年齡超過 `EffectiveFreshnessWindow()` 時**自升級為 `error`**）。
- **仍不一致（我實測,可重現 —— 這是本票的直接證據）**：
  1. `monitoring/rules/atlas_replay_alerts.yml:280-281` 的 annotation 已寫對（`gauge 值 {{ $value }}`;`1=warn/stale/**degraded**`…`4=其他`）✓ — 這半邊 #2068 修好了。
  2. 但 `monitoring/tests/atlas_replay_alerts_test.yml` 的 **J 案例仍停在舊映射,且同一段內三處互相矛盾**：
     - `:27` 註解已寫「gauge 1 = degraded,#2064 前的映射是 4」；
     - `:28`（**隔一行**）仍寫「`4=degraded`」；
     - `:370` 案例名寫「gauge 1 = degraded」,但 `:380` 的 `input_series` 實際餵 **`4x700`**,`:396` 的 `exp_annotations.description` 也仍寫「gauge 值 4」。
  3. **為什麼會「看起來綠」**：第 5 條的 expr 是 `atlas_channel_health_status{channel="twse_replay_sync"} > 0`（同檔 `:269`）——**1 與 4 都會成立** ⇒ 餵哪個值都 PASS。
     實測（2026-09-27,本機;與 CI 同版 pinned 映像）：
     `docker run --rm -v "$PWD":/work -w /work --entrypoint /bin/promtool prom/prometheus:v3.14.0 test rules monitoring/tests/atlas_replay_alerts_test.yml` ⇒ `SUCCESS`、`RC=0`。
     ⇒ 該案例**已不再驗證 #2064 之後的映射**,但沒有任何機制會說出來（「宣告與事實不符」正是本 repo 反覆在獵的缺陷類別）。
- **最小修法建議（只建議,未實作）**：
  1. 值域對照表**只留一份**：寫進規則檔頭（或 `docs/operations/` 的 runbook）,測試檔與 annotation 都引用同一份;
  2. 讓 J 案例的**名稱／輸入／期望三者對齊**（`input_series` 至少反映現行映射）,並補一個「degraded 資料超窗 ⇒ 升級 `error` ⇒ 命中既有 `ChannelHealthStatusError`」的案例（目前那條升級路徑在規則層沒有任何 fixture）。
- **驗收條件**：`grep -rn '4=degraded' monitoring/ docs/` ⇒ 僅出現在**明確標為「#2064 前」的歷史註記**內（現況 1 命中且未標記）;`promtool test rules monitoring/tests/*.yml` RC=0。

---

### FU-20260927-03 — replay 異常**沒有專屬 runbook**：5 條新規則的 `runbook_url` 全指向通用部署文件

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2063（`b1a1ee7d`）交付 5 條 replay 規則後刻意留下的界線（前一 lane 明確記載）。
- **事實（我實查）**：
  - `monitoring/rules/atlas_replay_alerts.yml` 的 5 條規則（`:93`／`:141`／`:185`／`:218`／`:268`）的 `runbook_url` **全部**指
    `https://github.com/kaecer68/atlas-go/blob/main/docs/operations/local-deploy.md`（`:127`／`:174`／`:207`／`:247`／`:294`）。
  - 該檔是**部署與 `.env` 分工**文件,**不含**這 5 條告警各自的處置步驟;`docs/operations/` 內**沒有** replay 專屬 runbook
    （`ls docs/operations/ | grep -i replay` ⇒ 空）。
- **代價**：值班點連結後拿不到判讀步驟。相對於「無連結」這是改善（#2066 已把死鏈修成可達）,但與 #2066 自己立的判準
  （**不要指向目錄／要指向真的處理那份告警的文件**）仍有落差。
- **建議（只建議,未實作）**：新增 `docs/operations/` 下的 replay 專屬 runbook（檔名建議 `replay-runbook.md`）,
  把 #2063 PR body 的 triage 步驟（**5 條規則逐條**的立即檢查／判讀前提／補救）整段搬過去,再把 5 處 `runbook_url` 改指它。
  - 與 `FU-20260927-04` 的關係：那張是「指到**不存在**的東西」,這張是「指到存在但**不對**的東西」——兩者都要收斂,但驗收方式不同。
- **驗收條件**：`docs/operations/` 內存在 replay runbook,且 5 條規則的 `runbook_url` 指向它;`promtool check rules` 與 `scripts/ci/check_markdown_links.sh` RC=0。

---

### FU-20260927-04 — **12 處** `runbook_url` 指向 `wiki.internal`（DNS **NXDOMAIN**）：其中 **5 處**在 repo 內沒有對應 runbook

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **事實（我實查,全部可重現）**：
  - `grep -rn 'wiki.internal' monitoring/` ⇒ **12 處**。
  - `host wiki.internal` ⇒ `Host wiki.internal not found: 3(NXDOMAIN)`（2026-09-27;MacBook,解析器 192.168.0.1）⇒ 這 12 個連結**對值班是死的**。
- **分類（依「哪一份 runbook 才指得對」計;我實測的計數與前一 lane 的判斷一致＝7／5）**：
  - **(a) channel-health 7 處 —— 可修**：
    - `monitoring/rules/channel_health_latent_staleness.yml` **6 處**（`ChannelDataStale`:`22`／`ChannelFetchLatencyHigh`:`40`／`ChannelHealthStatusError`×4:`60`／`:76`／`:92`／`:108`）。
    - `monitoring/rules/wave9_channel_individual_health.yml` **1 處**（`ChannelHighErrorRatePerChannel`:`17`）。
    - 可指向的**現行**文件：`docs/operations/wave9-runbook.md`（存在;§3.3 處理 `atlas_channel_health_errors_total`,§7／§8 有 Wave 9 自身監控與緊急處置）。
      ⚠️ **我的判斷（需 owner 覆核,不要直接照抄）**：該檔**部分**涵蓋「通道錯誤率」,但**不涵蓋**「資料陳舊／抓取延遲」的處置
      ⇒ 直接把 7 處都改指它,會重演「看起來修好」的形狀。建議**同一張票內**先補 `wave9-runbook.md` 的一節（或另立 channel-health runbook）再改指。
  - **(b) llm-annotator 5 處 —— 需先寫 runbook（依賴 `FU-20260927-03` 的同型格式）**：
    - `monitoring/rules/llm_annotator_alerts.yml` 共 9 條告警,其中 **5 條**帶死鏈（fast／medium／slow burn、circuit-breaker、no-traffic）。
    - repo 內**沒有** llm-annotator 的 on-call runbook（`ls docs/operations/ | grep -i 'llm\|annotator'` ⇒ 空;
      `docs/` 內的 llm 文件是 spec／策略框架,不是處置步驟）⇒ 硬指現有文件＝把死鏈換成另一種謊。
- **為什麼前一 lane 刻意不動**：同上 —— 避免製造假的閉環。本票把界線寫清楚,讓後手不會誤以為「12 處都只是換連結」。
- **最小修法建議**：拆兩步 —— ① 先寫 llm-annotator runbook;② 12 處**一次**改指正確目標（含 channel-health 那 7 處所需的補節）。
  驗收不是「grep 歸零」而是「每個新目標真的含該告警的處置步驟」。
- **驗收條件**：`grep -rn 'wiki.internal' monitoring/` ⇒ 0 命中;且 12 個新目標逐條**人工複核**含對應處置（不接受只換 URL）。

---

### FU-20260927-05 — `scripts/ci/check_monitoring_single_source.py` 可再加 **R4：`runbook_url` 必須指到存在且對應的文件**（只建議,未實作）

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：本日的 replay 告警工作（#2063／#2066／`FU-20260927-03`／`FU-20260927-04`）暴露的同一道縫：
  `runbook_url` 是**唯一沒有人守的欄位**（#2066 是一次人工修;12 處死鏈仍在）。
- **為什麼落在這支檢查而不是新開一支**：`check_monitoring_single_source.py` 已經是「監控設定只有一棵權威樹」的 PR 階段斷言
  （R1 唯一設定樹／R2 掛載源落點／R3 舊樹路徑）,而 runbook 目標正是同一棵樹上的**引用完整性**;加一條 R4 比再養一支腳本便宜。
- **建議的 R4 形狀（門檻需 owner 定案）**：
  - **可離線判定的部分（建議必做,fail）**：`runbook_url` 若指向本 repo
    （`https://github.com/kaecer68/atlas-go/blob/main/<path>`）,則 `<path>` 必須是 repo 內**存在的檔案**,且**不得是目錄**
    —— #2066 修的就是「指向目錄」那一種。
  - **不可離線判定的部分（建議只 warn,不 fail）**：外部 URL（含 `wiki.internal`）**不做 DNS／HTTP 檢查**。
    理由：CI 依賴外部網路會製造新的 flake,而本 repo 當日已有「`check_jev_contract.sh` 連外服務一 flake 就擋合法 push」的實證（`FU-20260926-21`）。
    ⇒ 外部 URL 只檢查「是否在具名 allowlist 內」或「是否帶具名的『尚未存在』標記」。
  - ⚠️ **界線**：本票**不動** `check_monitoring_single_source.py`、`Makefile`、`.github/workflows/quality.yml`
    （實作需 owner 確認佔用;且在 `FU-20260927-04` 收斂前就把外部 URL 設成 fail,會讓 gate 立刻紅）。
- **驗收條件**：故意把一條 `runbook_url` 改成 `.../blob/main/docs/operations/`（目錄）⇒ 檢查 **exit 1**;
  12 處 `wiki.internal` 的既定狀態下 ⇒ **只 warn、不 FAIL**（且有自我測試,不可退化成永遠 PASS）。

### FU-20260927-06 — `AtlasReplayJsonlBehindCsv` 的門檻以**日曆天**計，但修後的真實落後是**1 個交易日** ⇒ 常態下每週約 96% 相位會 firing

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2079（`fix/20260927-decouple-csv-to-jsonl-conversion`，**未併**）Verification §(c) 的**否證結果**（該 PR 自己寫「✗ 否證：每週仍會 firing 一次」）；本票只把它從 PR body 撈進登記表，**未在本 PR 修**（`monitoring/` 是凍結區）。
- **事實（位置由我複核；模擬數值引用來源 lane，我未重跑）**：
  - 門檻是**日曆天**且**嚴格小於**：`monitoring/rules/atlas_replay_alerts.yml:144` 的
    `atlas_replay_jsonl_latest_date_timestamp_seconds < atlas_replay_csv_latest_date_timestamp_seconds - 2 * 86400`
    ⇒ **只有日曆落差 ≥3 天才 firing**；`for: 1h`（同檔 `:148`）。
  - 兩側 gauge 的值都是**資料日 UTC 00:00**（`cmd/atlas/replay_freshness_metrics_task.go:152-155` 的註解明寫「正規化為該日 UTC 00:00」）⇒ 差額必為 86400 的整數倍。
  - 規則自己的註解把門檻依據寫成**週期**：同檔 `:137-138`「門檻 2d 依據:轉檔週期 = auto_backfill 的 24h ⇒ 單一輪沒跑不 page…連續兩輪沒跑才 page」。
  - 修後的真實落後＝**1 個交易日、最多到下一次 tick**：`auto_backfill` 是 `Interval: 24 * time.Hour`（`cmd/atlas/operations_tasks.go:106`），
    而 **jitter 對排程相位沒有作用** —— `runTask` 每個 task 只被 spawn 一次（`internal/apigateway/background.go:264`；另一條路徑 `:341`），
    jitter 區塊的守衛是 `!task.LastRun().IsZero()`（同檔 `:443`），而第一次執行前 `LastRun` 必為零 ⇒ **相位＝process 啟動時間**，不隨輪次漂移。
  - **來源 lane 的模擬（我未重跑，引用其數值）**：以**真 `runAutoBackfill`** 驅動 48 相位 × 3 模式（144 次、0 輪失敗）：
    一般週的缺口後首個交易日（週一）`csv − jsonl = 3 天` ⇒ 台北 `φ ∈ [00:30, 23:30]` 時 **firing（47/48 ≈ 96%）**，
    只有 `φ ∈ (23:30, 00:30)`（落後窗 <1h、`for: 1h` 不成立）沉默；
    長假變體（`2026-09-29` 週：中秋 09-25 + 週末 + 09-28 休市）`csv − jsonl = 5 天` ⇒ **該週就會 firing**；
    週二~週五 CSV 前進永遠只差 1 天 ⇒ 沉默。
- **為什麼是缺陷（語意不符，不是門檻太鬆）**：規則要表達的是「**連續兩輪轉檔沒跑**」，量到的卻是**日曆落差**；
  週末被算成真實落後 ⇒「只漏了 1 輪、且下一輪就自我修復」這種**正常**形狀會在每週固定響一次 warning
  ⇒ 與本 repo 反覆處理的警報疲乏同族（**門檻的單位必須與它宣稱的語意一致**）。
- **最小修法建議（只建議，未實作；採來源 lane 的選項 ①）**：
  1. **語意最正確**：export 一個「**落後 session 數**」的 gauge（以交易日曆算 CSV 資料日與 JSONL 資料日的**交易日差**），
     規則改比 session 數（例如 `> 1` ⇒ 真的漏了不只一輪）。
     ⚠️ 這會**新增指標面**（`cmd/atlas/replay_freshness_metrics_task.go`）**並且**改規則 ⇒ 兩者都在凍結區，需 owner 排程。
  2. 次之：`for: 1h → 26h`（量不變，只把窗口拉過一個轉檔週期）—— 便宜，但仍在「日曆天」的語意內繞。
  3. **不足**：只把門檻由 `- 2 * 86400` 放寬到 `- 3 * 86400` ⇒ 一般週不響，**但長假變體（4~5 天）照樣響**。
- **驗收條件**：一般週（週一落後 3 天、下一輪追上）⇒ 規則**全程沉默**；真的漏 ≥2 輪 ⇒ **必須 firing**；
  長假變體 ⇒ 沉默；且有 `promtool test rules` 的正向案例與負向對照（不可退化成永遠 PASS）。

---

### FU-20260927-07 — CSV→JSONL 轉檔**非原子**：`os.Create` 先 truncate 再逐行寫 ⇒ 讀端可能讀到 0-byte 檔

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2079 的未驗項 ②（該 PR 讓轉檔在**沒有缺口的日子也會評估／執行** ⇒ 這條既有缺陷的**曝露面變大**）。
- **事實（位置由我複核）**：`internal/importer/twmarket.go:11-34` 的 `ImportTWOpenDataCSVToJSONL`：
  - `:21` `f, err := os.Create(targetPath)` —— **當場 truncate 目標檔**（舊內容先消失）；
  - `:27-32` 逐列 `enc.Encode(bar)`（每個資料日 × 每檔一列）⇒ 整段寫入期間檔案都處於**不完整**狀態；
  - **沒有** temp+rename、**沒有** 檔案鎖。
- **讀端（位置由我複核）**：JSONL 是 FactorEngine 的正式輸入，於 composition 階段讀入：
  `internal/orchestrator/composition.go:113` 的 `hp.LoadFromExtendedJSONL(jsonlPath)` → `internal/portfolio/historical_prices.go:57-87` 逐行 `bufio.Scanner`。
- **來源 lane 的實測（我未重跑）**：30 天 × 300 檔（1,359,000 bytes）的寫入迴圈取樣 ⇒ **1/2012 次觀測為 0 bytes**。
- **代價**：與讀端存在**短窗**（轉檔寫入期間的交會）；舊碼同形，但修後**執行次數變多** ⇒ 風險上升。
- **最小修法建議（只建議，未實作）**：寫入**同目錄的暫存檔**後 `os.Rename`（同檔系統的 rename 為原子）⇒ 讀端只會看到「完整舊檔」或「完整新檔」。
  ⚠️ **權限語意會變（本機實測）**：`os.Create` ⇒ `0644`（＝`0666 & ~umask 0022`），`os.CreateTemp` ⇒ `0600`
  ⇒ 換法時必須顯式 `Chmod` 回原權限，否則會**默默收緊**檔案權限。
- **驗收條件**：寫入進行中（或注入延遲）時由讀端開檔 ⇒ 只會拿到合法 JSONL 或舊檔，**不得**出現 0-byte／截斷；
  轉檔失敗（例如目標路徑是目錄）⇒ **不得**留下半成品檔（有負向對照）。

---

### FU-20260927-08 — 轉檔閘門**單向**：只比「CSV 較新」⇒ JSONL 領先時永不修，而日誌會把它說成「已同步」

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2079 的未驗項 ③。
- **事實（位置由我複核，來自該 PR 的新增碼）**：PR #2079 的 `cmd/atlas/operations_tasks.go` 新增 `replayFreshness.conversionNeeded()`：
  - `csvErr != nil` ⇒ `false`（沒有來源可轉）；
  - `jsonlErr != nil` ⇒ `true`（**缺檔或不可讀 ⇒ 重建**，這是自我修復路徑）；
  - 其餘 ⇒ `return f.csvLatest.After(f.jsonlLatest)` —— **只有嚴格「CSV 較新」才轉**。
- **為什麼是缺陷**：JSONL 若因**外部寫入者**（人工修復、其他工具、`cmd/import-replay`）而**領先或等於** CSV，
  閘門恆為 false ⇒ 該檔**永遠不會被帶回與 CSV 一致**；而該狀態的日誌只會寫
  `conversion: skipped (already up to date)` ⇒ **把「不一致」講成「已同步」**（與本 repo 反覆在獵的「宣告與事實不符」同族）。
- **最小修法建議（只建議，未實作）**：把閘門由「落後」改為**不等**（`!csvLatest.Equal(jsonlLatest)` ⇒ 轉），
  並讓「不一致但刻意不動」成為**具名**的第三態日誌。
  ⚠️ 要先決定**語意**：JSONL 領先時轉檔會**覆蓋外部寫入者的資料** ⇒ 這不是實作細節，是 owner 決策（不宜由實作者順手改）。
- **驗收條件**：JSONL 尾行日期 > CSV 的 fixture ⇒ 下一輪轉檔後兩檔一致；且日誌**不再**把該狀態寫成 `already up to date`。

---

### FU-20260927-09 — **時區錯位**（pre-existing）：`getLatestReplayDate` 回 **UTC 00:00**、`end` 是 **Asia/Taipei 午夜** ⇒ 差的 8h 讓「還有一天缺口」被當成 `gap: none`

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2079 的未驗項 ④（**pre-existing**，非該 PR 引入；`origin/main` 同形）。
- **事實（位置由我複核；行號＝`origin/main` `b15105cd`）**：
  - `cmd/atlas/operations_tasks.go:113` 取 `getLatestReplayDate(...)`；該 helper（`cmd/atlas/bootstrap_helpers.go:89-113`）以
    `time.Parse("2006-01-02", …)` 解析 ⇒ 得到的是 **UTC 00:00**（CSV 的日期欄只有日期，沒有時區）。
  - 同檔 `:121` 的 `end` 是 `time.Date(now.Year(), …, 0, 0, 0, 0, now.Location())`，而 `now` 已轉 **Asia/Taipei**（`:118-120`）
    ⇒ `end` 是**台北午夜**。兩者基準相差 **8 小時**。
  - PR #2079 保留這個形狀，只把「無缺口」路徑**補上日誌**：`backfill gap: none (csv_latest=… target=…)`。
- **機制（我用這 12 行的等價重寫做的最小驗算，台北時間；`start = csv_latest + 1 天（跳週末）`）**：
  - 例（週三 16:00 台北，`csv_latest` = 週二）：`start = 週三 00:00Z`、`end = 週二 16:00Z`（＝台北週三 00:00）
    ⇒ `start.After(end)` 為**真** ⇒ 走 `gap: none` —— 但 16:00 已過 15:30 收盤基準，正確答案是**有**週三這天缺口。
  - **對照**（`csv_latest` = 前一交易日，即落後 ≥2 天）⇒ `start < end` ⇒ **照常偵測到缺口**
    ⇒ 這不是「全盤失效」，是**邊界上少偵測 8 小時**（實際形狀＝該一天的缺口**晚一輪**才回補）。
  - ⇒ 最直接的症狀是**日誌自相矛盾**：`gap: none` 那行會同時印出 `csv_latest=2026-03-24` 與 `target=2026-03-25`
    （兩個日期不同卻宣稱「沒有缺口」）⇒ **值班會被這行誤導**。
- **閘門本身不受影響**：JSONL 閘門兩側都走 `internal/replay.GetLatestDate`（**兩檔皆 UTC 解析**，見 `cmd/atlas/replay_freshness_metrics_task.go:152-167`）
  ⇒ 本條只影響 **gap 視窗與它的日誌**，不影響轉檔判定（這也是它被判為「日誌缺陷」而非「資料缺陷」的原因）。
- **最小修法建議（只建議，未實作）**：`latestDate` 與 `end` 一律在**同一基準**比較（建議都取「(亞洲/台北) 的日期」再比，
  或全部 `time.Date(..., time.UTC)`）；並在日誌同時印出 `timezone` 與 `now`，讓相位可稽核。
- **驗收條件**：「CSV 落後 1 個交易日」的 fixture ⇒ `gap: none` **不再**出現（日誌與實際缺口一致）；
  台北 `00:00~08:00` 與 `15:30` 前後的邊界各有一個測試（雙向）。

---

### FU-20260927-10 — `AtlasReplayJsonlBehindCsv` 的**排除步驟已過期**：規則 description 仍以「無缺口的日子結構上不會轉檔」解釋（修好後失效）

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2079 的未驗項 ⑤（`monitoring/` 是**凍結區** ⇒ 該 PR 刻意不動，**本次只登記**）。
- **事實（位置由我複核）**：`monitoring/rules/atlas_replay_alerts.yml:165-167` 的立即檢查第 2 點寫：
  「轉檔是否被執行過（**#2057 之前的已知缺陷**:auto_backfill 在『CSV 最大日+1 > 今天』時**提前 return**，
  而轉檔在 return 之後，所以**沒有缺口的日子結構上不會轉檔**）」。
- **為什麼會過期**：那段描述的正是 PR #2079 修的**根因**（轉檔被放在「有缺口」分支內，`:132-133` 的提前 `return` 讓 `:167` 不可達）。
  **#2079 併入後**該結構性缺陷不再存在 ⇒ 值班會照著 description 去找一個**已經不存在的缺陷**，
  而正確的判讀前提（「每個 tick 都會評估轉檔」）沒有寫在任何地方 ⇒ 屬「宣告與事實不符」。
- **最小修法建議（只建議，未實作；需在 #2079 併入後另開一個 `monitoring/` PR）**：
  1. 把該段改成**現行**語意：「轉檔**每個 tick 都會評估**；判讀時看 `backfill CSV→JSONL conversion` 的三態
     （`needed` / `skipped (already up to date)` / `failed (non-fatal)`）」，並保留一句**歷史註記**
     （「#2079 之前：轉檔只在有缺口的分支內」）—— 決策痕跡要留，但必須標明那是**修前**形狀。
  2. 一併複核同檔其他規則的 description 是否也引用了修前形狀（本票只涵蓋這一處）。
- **驗收條件**：`grep -n '沒有缺口的日子結構上不會轉檔' monitoring/rules/atlas_replay_alerts.yml` ⇒ 命中只出現在**明確標為修前**的歷史註記內；
  `promtool check rules monitoring/rules/*.yml` 與 `promtool test rules monitoring/tests/*.yml` 皆 RC=0（本次是純文案，**不得**動 expr）。

---

### FU-20260927-11 — `replay_freshness_metrics_task.go` 的註解把**寫入端**描述成自己做 `strings.TrimSuffix(...)`（#2079 之後寫入端改呼叫 `replayJSONLPath`）

- **狀態**：`open`
- **記錄日期**：2026-09-27
- **來源**：PR #2079 的未驗項 ⑥。
- **事實（位置由我複核）**：
  - 註解：`cmd/atlas/replay_freshness_metrics_task.go:141-145` 寫「（寫入端把 CSV 轉成 `strings.TrimSuffix(path, ".csv") + ".jsonl"`）——刻意共用同一條推導」。
  - 修前（`origin/main`）：寫入端 `cmd/atlas/operations_tasks.go:162` **自己**做 `strings.TrimSuffix(d.cfg.ReplayDataPath, ".csv") + ".jsonl"` ⇒ 註解當時**是對的**。
  - 修後（PR #2079）：寫入端改為 `absJSONL := replayJSONLPath(absCSV)` ⇒ **真的共用同一個函式**（PR body 亦明寫「check 端＝write 端同函式」）。
- **為什麼要改**：那段註解的用意是「兩端不得漂移」；修好之後，它反而成為**唯一還在說兩端各做一次推導**的地方
  ⇒ 讀者會以為要繼續防漂移，而事實上是**同一個函式**（同一形狀：宣告落後於事實）。
- **最小修法建議（只建議，未實作；一行註解，需在 #2079 併入後動）**：
  把括號那句改成「寫入端（`auto_backfill`）**呼叫同一個** `replayJSONLPath()`」，並保留「刻意共用同一條推導」的結論。
  ⚠️ **只能動註解**：該檔沒有任何行為變更需求，且不宜在同一個 PR 內順手改監控相關面。
- **驗收條件**：該檔註解不再描述「寫入端自行 TrimSuffix」的形狀；`gofmt`／`go vet` 不受影響、diff 僅含註解（無行為變更）。

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


---

### FU-20260926-25 — manifest 收尾：**E13/E14 結案（複查後撤銷）＋ E15 重新定性 ＋ 新增 E17–E20**（全部由 root 實查，非採信回報）

- **狀態**：`open`（E17/E18/E19/E20 待派；E13/E14 已結案）
- **記錄日期**：2026-09-26
- **對應**：`docs/operations/remediation-manifest.md` §3（同 PR 更新）、§6 對帳表
- **來源（可重現，全部由 root 實跑）**：
  - **E13 → 結案**：`grep -n 'IMAC_HOST\|WATCHDOG_DST' Makefile` ⇒ `IMAC_HOST ?= kaecer@kmacmini`（105–107 行已具名記錄 E13）——⚠️ **歸因更正**：修者為 **#2041**（13:42Z），非 #2031（原記錯誤，2026-09-26 由 root 以 `git log -- Makefile` 核對更正）；
    實跑 `make imac-watchdog-diff` 舊因（`ssh: Could not resolve hostname kimac`）**已消失** ⇒ **原描述不成立，撤銷**。
  - **E14 → 結案**：`grep -c -iE 'kk@kimac|iMac' docs/reference/traps.md .github/workflows/quality.yml` ⇒ **0 / 0** ⇒ 無殘留，不需派工。
  - **E15 → 重新定性**：`~/.prime/agent/models.json:140` 確有 provider `"kimac"`，但其 `baseUrl` 已是 `http://kmacmini:4000/v1`
    ⇒ **僅名稱歷史債、路由正確**（低風險）；原描述的懸空 symlink `~/bin/imac-recover` 以 `[ -e ]` 複查**不存在**
    ⇒ **不可重現，不登記為事實**（依「因果主張最小否證」紀律）。
  - **E17（新增，排程空轉）**：`head -12 scripts/darwinian_adjust.sh` ⇒ 檔頭自述 `DEPRECATED — D1 決策退役 2026-08-17`、
    核心計算註解於 L101；實跑 `bash scripts/darwinian_adjust.sh` ⇒ **rc=1**；`grep -rn 'darwinian_adjust'` ⇒
    `docker-compose.yml:453` `CRON_COMMAND=/app/scripts/darwinian_adjust.sh --apply`（容器 `atlas-cron-darwinian`、`0 9 * * *`）
    ⇒ **排程有、實際不做且失敗**。
  - **E18（新增，文案）**：`grep -c` 於 `quality.yml` ⇒ 「照現狀合併就會回退」**1**、
    「a stale branch deleting a shared asset must be blocked」**1**（行為正確，僅文字沿用已被 k3 否證的 v1 框架）。
  - **E19（新增，孤兒群）**：對 11 支候選腳本以 `grep -rl` 掃 `Makefile`/`.github/workflows`/`docker-compose*.yml`/`scripts/`
    ⇒ **10 支 0 個可叫用引用**；`verify-manifest.sh` 唯一引用來自**同樣 0 引用的** `verify-atlas.sh:82`（孤兒互叫）。
  - **E20（新增 → 同日已解除）**：`make imac-watchdog-diff` ⇒ `❌ 不一致`、`exit 2`；**同日補測（main `b1fc524d`）：`✅ 一致（無漂移）` rc=0（兩側 `cdb4a7d6`）⇒ 已由跨機器 `make imac-watchdog-install` 收斂**；
    `ssh kmacmini shasum -a 256` ⇒ 遠端 `424944ce`（85 行、Sep 25）vs repo `70ee1a45`（104 行、#2031）；
    `diff` 判定 ⇒ **37 行差異全為註解、非註解 0 行 ⇒ 行為零風險**。
- **殘項／後續**：
  1. **E20 的修法**（`make imac-watchdog-install`）是**跨機器執行** ⇒ 依政策**交 a2a-dev**，root 不直接執行。
  2. E17 需在「移除排程」與「讓 stub 明確 no-op + log」之間擇一（需 owner 決定方向）。
  3. E18/E19 皆等 `quality.yml` / owner 讓出後處理（E18 串行在 E6 之後）。
  4. 本條目**不含**任何未經實跑的推論；被否證的 E13/E14 原描述已在 manifest §3 以刪除線保留，避免日後重複盤查。

---

### FU-20260926-26 — 更正與複測：**E13 歸因改為 #2041（原記 #2031 係錯誤）**；**E20 watchdog 漂移已解除**

- **狀態**：`done`（純文件更正 + 複測，無程式變更）
- **記錄日期**：2026-09-26
- **對應**：`docs/operations/remediation-manifest.md` §3（E13、E20 列）、§6 對帳表；同 PR 更新
- **① E13 歸因更正（原記載錯誤）**：
  - 原記（`FU-20260926-25` / manifest §3、§6）：E13「**隨 #2031 修**」。
  - 核對指令：`git log --format='%h %ad %s' --date=short origin/main -- Makefile`
    ⇒ `12edcec0 2026-09-26 fix(ops): E13 Makefile 退役 iMac 殘留 — IMAC_HOST/WATCHDOG_DST 預設改指 Mac Mini (#2041)`
    ＋ `gh pr diff 2041` ⇒ `-IMAC_HOST ?= kk@kimac` → `+IMAC_HOST ?= kaecer@kmacmini`、
    `-WATCHDOG_DST := /Users/kk/bin/…` → `+WATCHDOG_DST := /Users/kaecer/bin/…`。
  - **結論**：修者為 **#2041**（13:42Z 併入）；`#2043`/`#2044` 補文件。**原歸因錯誤，已更正**（觀察無誤、歸因有誤）。
- **② E20 已解除（同日複測）**：
  - 原觀測（main `12edcec0`）：repo `70ee1a45`（104 行）≠ Mac Mini `424944ce`（85 行、Sep 25）⇒ `make imac-watchdog-diff` `exit 2`；
    diff 判定 37 行差異**全為註解、非註解 0 行**。
  - 複測（main `b1fc524d`）：`make imac-watchdog-diff` ⇒ `✅ 一致（無漂移）` **rc=0**、兩側 sha256 均 `cdb4a7d6`
    ⇒ 已由**跨機器** `make imac-watchdog-install` 收斂（依政策該執行由 a2a-dev 承接，root 只複測）。
- **③ 保留的診斷價值（避免後人誤讀）**：
  `#2043`/`#2044` 的文件寫「兩個 target 都可直接跑」；**「可用」≠「必 exit 0」**——
  修好後 `imac-watchdog-diff` 的 `exit 2` 代表**真的偵測到漂移**（本項即為實例），
  與修好前的「host 不存在 ⇒ 必失敗」是**兩種不同意義的 exit 2**。判讀時必須先看輸出訊息（`❌ 不一致` vs `Could not resolve hostname`）。
- **教訓（本次適用於本專案全體）**：**觀察正確 ≠ 歸因正確**。寫進 SSOT 的因果（「X 由 Y 修」）必須以 `git log -- <file>` 指認到**引入該變更的 commit/PR**，不可用相鄰時序代替。

---

### FU-20260926-27 — manifest 狀態同步：**E5/E18 結案、E4/E19/E21–E28 登記**（含 8 項新發現；root 單一寫者）

- **狀態**：`open`（E22/E23/E26/E28 未派或待決；其餘在飛或已結案）
- **記錄日期**：2026-09-27
- **對應**：`docs/operations/remediation-manifest.md` §3（E4/E5/E18/E19 更新 ＋ 新增 E21–E28）、§6 對帳表
- **本次結案（root 複驗過）**：
  - **E5** → **#2048**（`fe779473`）：原記「預設 warn」**經查為誤** —— 實況是**死守門**（`.claude/settings.json` 只有 `SessionStart`、無 `PreToolUse`；唯一 PreToolUse 在 gitignored `.claude/settings.local.json`）。修法＝tracked `settings.json` 加 `PreToolUse`→薄 adapter（pattern 邏輯無第二份）＋修 `${CHECK,,}`（bash 4；macOS `/bin/bash` 3.2 對**每個**指令 `bad substitution`，root 以 `3.2.57` 實測確認）＋11 組契約測試納入 `make ci-gate`。
  - **E18** → **#2046**（`2404dbe2`）：`quality.yml` v1 文案清零（root 複查 main 兩句 0/0）；spec 殘留條目由 **#2050** 更新。
- **本次新增（證據見 manifest §3 各列）**：
  - **E21** CLI 靜默忽略未知子命令 ⇒ `daily-maintenance` 三 job 跑模擬（**#2053** armed；root 複驗公開 task-liveness `200／114 tasks／stale_count=0`，CI run `36255705561` 四 job 全 success）。**注意**：child **未照我給的兩個選項**（退役／改寫），而是**改用真實來源並保留 job** —— 理由（daemon 自身告警與被監控對象同主機、GitHub runner 是唯一局外觀察者）**比我的選項更強**。
  - **E22** `Makefile:1042-1046` coverage 共用 `/tmp` ⇒ 同機多 worktree 併發**假紅**（root 複驗；與 E4/FU-15 同族）。
  - **E23** guard 切 `enforce` 的三個實測誤擋面（`secret` 一字即擋／prod worktree 擋 `docker compose build|up`（部署路徑）／擋 `go test`）。
  - **E24** staging soak 鏈：2026-07-15 的 7 天 soak、期限已過、**2026-08-15 專案宣告「無 staging」**、Day-7 收尾未執行（`post-soak-cleanup.sh` 一生只跑過 `--dry-run`）。**root 已停本機 LaunchAgent**（停前 `runs=39838`、每 60 秒約 5 次重生、log 133 MB、報告停於 09-17）。
  - **E25** replay 幻影列：非交易日以 `time.Now()` 蓋章寫入 ⇒ 生產 **396 列／9 天（自 2026-08-29）**；連鎖使 `auto_backfill` 卡死、CSV→JSONL 自 2026-08-24 停擺。修復在飛（`fix-replay-dated-source`）。
  - **E26** 生產 Prometheus **Calibration 告警未載入**（Rule 檔在容器內、`/api/v1/rules` 0 命中）⇒ **#2016 的校準監控實際無效**；同一查核**正面確認 `Universe` 6 條已載入**（週一 #1995 驗收可行）。
  - **E27** `verify-manifest.sh` 假綠（`gsub` 只給 2 參數 ⇒ 改 `$0`、每列 continue；root 實測 `done`＋空 Notes ⇒ `OK` exit 0）＋驗的檔在 gitignored `.omo/` ⇒ 刪除並修 2 處文件引用。
  - **E28** `internal/taiwanholidays` 缺 9 個真實休市（颱風假等）⇒ **刻意延後至週一驗收後**（會改變 `IsTradingDay`）。
- **殘項／後續**：
  1. **E22/E23** 需 owner 決定或等 `Makefile` 讓出（E1 佔用）。
  2. **E24/E25/E27** 目錄內外均須在對應 PR 併入後，於本 registry 補「已併」與實測輸出。
  3. **E26** 由 a2a-dev 查「規則未載入」的根因（載入面 vs repo 面）。
  4. **liveness 殘留列**：`cron_darwinian` 仍存在於 liveness store（`internal/liveness/store.go` 無 Delete、端點只寫不刪）⇒ 需一次性 SQL 清除，屬**生產寫入**，待 owner 決定。
- **方法論註記（本次三次「派遣前重新定性」的成果）**：E5（死守門 ≠ 預設 warn）、E13（歸因錯：修者是 #2041 非 #2031）、E19（孤兒 ≠ 死碼：3 支有操作性引用）⇒ 已固化為「先重新定性再派遣」紀律。
### FU-20260926-28 — `make ci-full` 的 coverage profile 走**共用** `/tmp` 路徑：其他 lane 的 `rm -f` 讓本 lane 假紅（「coverprofile 缺失或為空」）

- **狀態**：`done`
- 已由 **#2075**（`963be848`）修好 —— coverage profile/log 改 per-run `mktemp -d` ＋ `trap … EXIT` 私有狀態 ✓
- **記錄日期**：2026-09-26
- **來源（多 lane 同日並行實測）**：本機 `make ci-full`（由 `.githooks/pre-push` gate 1b 觸發）在**最後一步**紅：
  `❌ 取不到覆蓋率：coverprofile 缺失或為空（/tmp/atlas-ci-full-coverage.out）`，
  而**同一輪**的 ci-gate／golangci-lint／staticcheck／`go test`／`go test -race`／`cmd/atlas`／`ci-slow` **全綠**
  ⇒ 不是程式碼或測試的問題。
- **缺陷本體（`Makefile:1042-1071`）**：profile 與 log 的路徑是**硬編的共用路徑**（無 PID、無 `mktemp`）：
  - 起點 `rm -f "$${COV_PROFILE}" "$${COV_FUNC_LOG}"`（:1047）；
  - 成功路徑結尾再 `rm -f "$${COV_PROFILE}" "$${COV_LOG}" "$${COV_FUNC_LOG}"`（:1071）。
  ⇒ 本機同時有 20+ worktree、多個 lane 跑同一個 target 時，**別人的 `rm -f` 會刪掉自己正在寫的 profile**。
- **機制（2026-09-26 最小實驗，可複現）**：`go test -coverprofile=<path>` **在測試一開始就建立該檔**
  （1 秒後即存在、大小 10 bytes＝`mode:` 檔頭），並在**同一個 inode** 上於結束時才寫內容 ⇒
  測試進行中若該路徑被刪除，`go test` **仍 exit 0**，但結束後檔案**不存在**：
  - **負向（共用路徑）**：測試中 `rm -f` 該路徑 ⇒ 跑完 `FILE_ABSENT_AFTER_RUN`、`go test` rc=0
    ⇒ recipe 的 `[ ! -s "${COV_PROFILE}" ]`（:1053-1056）判紅，訊息與 pre-push 觀察到的**完全一致**。
  - **正向（隔離路徑）**：同一份工作改用 `/tmp/cov-self.out`，同時另一行程持續 `rm -f` 共用路徑
    ⇒ `go test` rc=0、`go tool cover -func` 得到 `total: 86.2%`（單一 package 子集）
    ⇒ **路徑一隔離就消失**（另一 lane 以相同手法自證 repo 全量 `total: 70.5%`）。
  ⇒ 這**不是毫秒級競態**：**視窗＝整段 `go test` 的執行時間**（全量約數分鐘）⇒ 兩個 lane 時間重疊就會中。
- **影響面**：本機 pre-push／`make ci-full` 的**假紅燈**；會誘使人 `--no-verify` 或無意義重跑，
  對「閘門必須可信」是負面誘因（與本 repo 同日反覆出現的 false-green／假紅同族）。
  **不影響 GitHub CI**（runner 上同時只有一個 job），且**不是** #2009 修過的 fail-closed 缺陷。
- **修法方向（只建議、未實作；本 PR 刻意不動 `Makefile`）**：
  1. profile／log 改**每 process 唯一**：`COV_PROFILE=$$(mktemp -t atlas-ci-full-coverage.XXXXXX.out)`
     （或至少帶 `$$` PID），並以 `trap 'rm -f …' EXIT` 或結尾 `rm -f` 收尾清理。
  2. **必須保留**「取不到覆蓋率 ⇒ 非零」的 fail-closed（#2009 已修，**不得回退**）：
     `[ ! -s "${COV_PROFILE}" ]` 與 `go tool cover` 解析失敗兩條都要留；60% 閾值不變。
  3. 建議 `COV_LOG` 一併唯一化 —— 共用 log 會讓失敗訊息對不上真正的那一輪。
- **同一缺陷的另一處登記（[#2055](https://github.com/kaecer68/atlas-go/pull/2055) 併入後補記）**：`docs/operations/remediation-manifest.md` 的 **E22** 記的是**同一件事**
  （`Makefile:1042-1046` coverage 共用 `/tmp` ⇒ 同機多 worktree 併發假紅，與 E4/FU-20260926-15 同族，root 複驗）
  ⇒ 兩者**同源、修法共用**；本票的角色是補上**最小實驗（機制）**與**fail-closed 不得回退**的驗收條件。
- **為何不順手改（本 PR 的界線）**：開票當下**有兩個 open PR 正在編 `Makefile`**
  （#2049 同日稍後合併、#2053 仍 open）⇒ 依指派界線**只開票、不改檔**，避免同日多 lane 互踩。
- **驗收條件**：兩個 lane 同時跑 `make ci-full` 時任一 lane 都不會因覆蓋率步驟紅；
  且刻意讓 `go test` 階段的 profile 被外部刪除時，該 lane **仍必須**以非零碼失敗（fail-closed 不回退）。
- **負向證明（修完後必須具備）**：profile 指向不存在的路徑／產出空 profile ⇒ **必須 `exit 1`**（不得靜默通過）。
- **2026-09-27 第 3 次獨立確認（另一 lane）**：同一 commit 並行跑**3 個** `make ci-full` ⇒ 該 lane 的失敗訊息竟指向**另一個 worktree** 的檔案 `…/atlas-calib-drift/internal/monitoring/calibration_effective.go`（不是它自己的 worktree）✗
  —— 正是上面「機制」一節與修法方向第 3 點預測的「共用路徑 ⇒ 失敗訊息對不上真正的那一輪」；該 lane 改用**私有** profile 路徑後 ⇒ `exit 0`、`total: 70.7%` ✓。


---

### FU-20260926-29 — `internal/industry/event_calendar.go` 是**第二份**假日名稱清單：缺教師節／行憲紀念日 ⇒ `long_holiday` 視窗 2026 不含 09-28

- **狀態**：`open`
- **記錄日期**：2026-09-26
- **來源**：同日 `FU-20260925-04` 的 TWSE 官方 API 比對（09-28 教師節休市）順帶盤出的**第二處**假日清單。
- **現況**：`internal/industry/event_calendar.go:345-354` 的 `taiwanPublicHolidays` 自帶一份清單
  （`元旦`／`春節`／`228和平紀念日`／`清明節`／`勞動節`／`端午節`／`中秋節`／`國慶日`）。
  其中**農曆日期已委派 SSOT**（`:304-310`，P1-8 去重：`taiwanholidays.LunarNewYearDates()` 等），
  但**固定日期那 4 筆（元旦／228／勞動節／國慶日）仍是本地硬編**，且整份清單**缺**
  `教師節 09-28`、`行憲紀念日 12-25`（2025 起復為放假日）、`光復節 10-25`。
- **影響（已核對程式碼）**：`buildHolidayEvent`（`event_calendar.go:1166-1200`）對清單每一筆產生
  `[holidayDate-3, holidayDate+2]` 的 `long_holiday` 視窗 ⇒ 2026 年的視窗**不含 09-28**（也缺 10-25／12-25）
  ⇒ 事件日曆與 SSOT（`internal/taiwanholidays`）**不一致**；以事件日曆判斷「長假前後」的消費端在這些日子會拿到錯的視窗。
  對照同日實測：**SSOT 自己當時也缺**這兩天（`IsTradingDay(2026-09-28)=true`、`IsTradingDay(2026-12-25)=true`，**皆為錯**），
  該資料缺口由 open PR **#2054** 補（`adjustedHolidays[2026]` 加 09-28／12-25）
  ⇒ **本票要在 #2054 之後才有意義**：#2054 修 SSOT 的資料，本票修**第二份清單的漂移**。
- **修法方向（只建議，未實作）**：
  1. 讓 `taiwanPublicHolidays` 的四筆固定日期**也**由 `internal/taiwanholidays` 提供。
     **import 方向不成環 —— 已確認**：`event_calendar.go:15` **已經** import `internal/taiwanholidays`，
     而該套件只 import `internal/logging`（`grep -rn 'atlas-go/internal' internal/taiwanholidays/*.go` 僅 1 筆）。
  2. 或者，至少在清單旁**明確標註**「本表用途不同（事件日曆的長假視窗，非交易日判定）
     ＋ 已與 SSOT 對齊清單」，並補一個**漂移測試**（同名假日與 SSOT 逐項比對）。
- **驗收條件**：`internal/industry` 的 `long_holiday` 視窗在 2026 年含 09-28（教師節），
  或該清單有明示的「用途不同＋已對齊」註記且有測試釘住與 SSOT 的一致性。
- **備註**：本票**不**主張 `long_holiday` 的語意要改成「交易日」語意 —— 語意變更需 owner 裁決。

---

---

### FU-20260926-30 — 最終狀態同步：**E4/E19/E21/E24/E25/E27/E28 結案、新增 E29/E30/E31、FU-15 標 done**（root 單一寫者）

- **狀態**：`open`（E29/E30/E31 待派或待 owner 決定；其餘皆已結案）
- **記錄日期**：2026-09-27
- **對應**：`docs/operations/remediation-manifest.md` §3（7 列結案 ＋ 新增 3 列）、§6 對帳表
- **本次結案（root 逐項複驗，非採信回報）**：
  - **E19 → #2052**（`14a3da03`）：**刪 10 支**（6 支無殘餘價值 ＋ `verify-manifest.sh` ＋ soak 鏈 3 支）；**`agent-guard` 改為 tracked wrapper（非 symlink）** —— child 以實驗證明 rel/abs symlink 都使 `BASH_SOURCE` 推導的 `REPO_ROOT` **高錯一層**，wrapper `exec` 真檔才正確 ⇒ **它推翻了我建議的 symlink 方案**；`.gitignore` 的 `/agent-guard` 條目同時移除。連帶編輯 `deploy-staging.sh`（4 步→3 步）。
  - **E21 → #2053**（`88f6b33d`）：CLI 未知位置參數 **exit 2 ＋ usage**；三個 job 改用 daemon 公開端點。**它也未照我給的兩個選項**（退役／改寫 CLI），而是「改用真實來源並**保留 job**」，理由是 **GitHub runner 是唯一局外觀察者** ⇒ 比我的選項更強。
  - **E24 → #2052**：整條 soak 鏈刪除。**root 另已處置本機**：`launchctl bootout`（rc=0）＋ plist 改名 `.DISABLED-20260927` ＋ 133 MB log 歸檔；停前 `runs=39838`、每 60 秒約 5 次重生。
  - **E25 → #2057**（`a3d54321`）：非交易日不抓不寫 ＋ 改帶日期 MI_INDEX ＋ 回應日期守門 ＋ 修 `fetch-historical` envelope。**root 親驗數值等價**：`MI_INDEX(20260917)` 與舊路徑寫出的 CSV 對 0050/2317/2330 之 volume 與 OHLC **逐欄相同**。**資料面**（既有 396 列）須**部署後**清理。
  - **E27 → #2052**：刪 `verify-manifest.sh` ＋ 修 2 處失效文件引用。
  - **E28 → #2054**（`db7fdb44`）：補關鍵休市日 ＋ **weekly 加假日 gate**（`weekly_skip_holiday`）＋ daily 改用單一來源。**root 行為測試**（臨時 worktree 跑 `IsTradingDay`，非字串比對）：09-28／12-25／09-25／10-09／10-26／04-03／05-01／06-19 皆正確；**仍缺 3 個過去日期**（`2026-01-02`、`02-11`、`02-23`）。
  - **E4 ＋ FU-20260926-15 → #2049**：fail-closed ＋ Gate 0b；併同修掉一個**無聲死亡**（測試以 rc=1、stdout/stderr 皆 0 bytes 結束 ⇒ 不可行動的假紅）。
- **本次新增**：
  - **E29** replay 路徑三個同型「靜默成功」缺陷（`checkReplayHealth` 讀最後一行／`MarketVolumeProvider` 用請求日當資料日／`degraded` 不自我升級）—— `#2057` 只登記未動。
  - **E30** `git push --delete <branch>` 被 pre-push **Gate 3**（zero diff vs origin/main）誤擋 ⇒ 刪遠端分支必須 `--no-verify`（刪分支沒有內容可守）。
  - **E31** `scripts/hooks/pre-commit` = DEPRECATED `exit 0` stub、0 呼叫端 ⇒ 待 owner 決定刪除。
- **殘項／後續**：
  1. **部署要求**已交 a2a-dev：`main ≥ db7fdb44`（同時含 #2054 與 #2057）＋ `--remove-orphans`（E17）＋ 部署後 replay CSV 清理（預期 396 列）＋ JSONL 重建；**預期現象**：09-28 weekly 不重建（教師節休市，`weekly_skip_holiday` 為正確行為）、下一次 weekly ＝ **2026-10-05**。
  2. **決定性驗收 ＝ 2026-09-29（週二）06:00Z**（daily 增量；root 已行為驗證該日為交易日）。
  3. **E26**（生產 `Calibration` 告警未載入）仍交 a2a-dev 查載入面。
  4. **待 owner**：~~E23（guard 是否收緊）、E29/E30/E31 的處置、`cleanup-manifests.sh`~~ ⇒ **已於 2026-09-27 全部決定並落地**：E23 → **#2061**（`1601c65e`，預設 `enforce`）、E30 → **#2059**（`f0d658d9`）、E31＋`cleanup-manifests.sh` → **#2060**（`20c414ea`，兩支皆刪）；僅 **E29** 仍在飛（`fix-replay-silent-and-gate3`）。liveness `cron_darwinian` 殘留列 → **已交 a2a-dev**（一次性 SQL）。
- **方法論註記**：本輪**三次**由 child 推翻我的指示（E21 的選路、E19 的 symlink vs wrapper、E25 的來源切換範圍），且我**兩次攔下自己的驗證方法錯誤**（落後工作樹／字串比對）⇒ 兩者都源於同一紀律：**先實測再定調，且不把未驗證的推論寫成事實**。

---

### FU-20260926-31 — 第二批同步：**E23/E30/E31 結案**（#2061／#2059／#2060）＋ E19/E22 更新

- **狀態**：`open`（僅 E29 在飛；其餘結案）
- **記錄日期**：2026-09-27
- **對應**：`docs/operations/remediation-manifest.md` §3（E19/E22/E23/E30/E31 五列）＋ §6
- **本次結案（root 逐項複驗）**：
  - **E23 → #2061**（`1601c65e`）：guard 預設收緊為 **`enforce`**，但**先消除三面已實測誤擋**。**root 實跑複核**：`grep -rn secret internal/`／`go test ./...`／`ATLAS_GIT_COMMIT=$(…) docker compose up -d` 皆 **rc=0**；`cat .env`／`rm -rf /`／`git push --force origin main` 皆 **rc=2**；`ATLAS_HOOK_MODE=warn` 下 **rc=0**；契約測試（76 條矩陣）PASS。**child 對自己的改動做了對抗式複查，抓到自身引入的覆蓋退化**（`sudo -u root cat .env` 的 command word 被讀成 `root`）並修正 —— 這是本輪最值得複製的行為。
  - **E30 → #2059**（`f0d658d9`）：pre-push **讀一次 stdin 分類 refspec**，delete-only push 明示跳過內容閘門；非 delete push 仍走完整 gate。
  - **E31 → #2060**（`20c414ea`）：`scripts/hooks/pre-commit`（exit-0 stub）與 `scripts/cleanup-manifests.sh` 皆刪；5 處文件改為反映實況（含替代的 `find` 指令），未動 `docs/manifests/` 的位置治理 gate。
- **root 的數字被更正（誠實記錄）**：我以 1 層 glob 誤判 `.omo/manifests` 最新為 2026-08-07（休眠 7 週），實際最新為 **2026-08-30**（休眠約 **4 週**）；`find -type f` = **78 檔**；`.gitignore` 為 **:66-67**。**結論（刪除）仍成立**（兩項停手條件皆未觸發），但這說明：**判斷的品質取決於數字的品質**。
- **E22 擴充為兩種形態**：① coverage 硬編共用 `/tmp`（`Makefile:1042-1046`）② **golangci-lint 共用快取重播已刪除的兄弟 worktree**（`cannot read file`）⇒ 0 個 `.go` 變更的 PR 亦假紅（`#2052`/`#2053`/`#2060` 各中一次）。
- **殘項**：**E29**（replay 三個同型靜默缺陷）在飛；**E26**（生產 `Calibration` 告警載入）由 a2a-dev 查；**09-29 06:00Z** 母體決定性驗收（root 執行）。

---

### FU-20260926-32 — 最終同步：**E26/E29 結案 ＋ 新增 E32/E33**；生產已部署 `b1a1ee7d`（規則 40 條）

- **狀態**：`open`（E29 的 5 項殘項、E23 之後的 pattern 調校、E22 串行等待皆未派；其餘結案）
- **記錄日期**：2026-09-27
- **對應**：`docs/operations/remediation-manifest.md` §3（E26/E29 更新 ＋ 新增 E32/E33）、§6
- **本次結案／新增（root 逐項複驗）**：
  - **E29 → #2064**（`b6fe7d8a`）：replay 判讀三處失真（`checkReplayHealth` 取資料自身最新日／`MarketVolumeProvider` 讀表格自身標題的民國日期並拒收不符／`degraded` 取最新戳記並超窗升級 error）。**root 複核**：凍結區 0 檔、生產 45 通道 **degraded=0**、E29-2 testdata 與線上 payload **byte-identical** 且值 = 生產 `TSE_VOLUME(20260924)`（交易日值不變）。**5 項殘項**見 manifest。
  - **E26 → 已恢復**：生產 Prometheus 由 29 條／`Calibration`=0 變成 **40 條／12 組**（`Universe` **9**、`Replay` **5**、`Calibration` **3**）⇒ **#2016 的校準監控已實際生效**。仍待 a2a-dev 回報「是什麼讓載入生效」與「`#53` 閘門判檔案存在或真的載入」（若只判存在＝假綠）。
  - **E32（新）→ #2065**（`91cd0564`）：**宇宙三條警報的規則層根因** —— 舊表達式以 counter 增量為唯一判據，無法區分（i）沒跑（ii）跑了但產出沒用（iii）**跑了、產出健康但計數器沒發射（本日實況）**。修法＝9 條**輸出優先、bucket 互斥**規則（新增 `EmptyUniverse`／`RunOverdue`（假日感知）／**`CounterEmissionMissing`**）＋ Go 端 `last_run_*`（per stage，**單一 defer** 發佈、走**獨立 `GaugeSink`**）與 `next_run_timestamp_seconds`；**未動 pipeline 行為**。決定性驗證＝真 `BuildUniverse` → 真 `/metrics` 文字 ＋ mutation。
  - **E33（新）→ #2063**（`b1a1ee7d`）：**replay 告警的結構性盲區** —— `monitoring/rules/` 先前**零 replay 規則**，而 JSONL 停擺一個月與 396 列幻影列**當時都零告警**；child 以生產保留窗內的**歷史樣本**證明抓取層訊號**結構上看不到內容落後**。修法＝5 條規則 ＋ 最新資料日接線。
- **⚠️ 本 session 我兩次被 child 更正（誠實記錄）**：
  1. `.omo/manifests` 最新日期我以 **1 層 glob** 誤判（說休眠 7 週，實為 4 週）。
  2. **counter 推理陷阱**：我把 `increase(atlas_universe_symbols_screened_total[7d])` 的 **4906** 當成「管線跑了約 3 輪」的真增量，實際那是 **legacy 錯標籤系列 `{daily="failed"}` 的殘留**；正確形狀 `{stage="daily"|"weekly"}` 的系列**全為 0**。⇒ 紀律：**看到 counter 非零 increase 時，必須先驗該 series 的標籤形狀**，否則不得推論「跑過幾輪」。
- **生產部署狀態（2026-09-27 03:24Z，a2a-dev 執行、root 複查）**：binary = **`b1a1ee7d`**（含 #2052–#2065）；規則載入 **40 條／12 組**；新指標族皆在（`atlas_universe_last_run_*` 20 條、`atlas_universe_next_run_*` 1 條、`atlas_replay_*` 3 條）；**三條舊 Universe 警報已不再 firing**（新規則在「尚未跑」時保持沉默 —— 正是設計意圖）。
- **決定性驗收（2026-09-29 06:00Z，週二 daily；root 執行）**：以 **`universe_snapshot.json` 輸出為真值**（計數器會說謊），並核對 `last_run_valid{stage="daily"}=1`、`last_run_symbols_ranked{stage="daily"}=150`、`last_run_ranked_trustworthy=1`、counters 有增量、三條舊警報維持沉默；**若 `CounterEmissionMissing` 反而 firing ⇒ 是發射面又壞（非評分問題）**。`stage="weekly"` 仍會沉默（下次 10-05）。

---

### FU-20260926-33 — 第四批（最終）同步：**E12/E22 更新 ＋ 新增 E34–E39**（本 session 修復的完整收斂）

- **狀態**：`open`（E36/E37 的部署後驗收、E39 的文件補行、以及各列列出的低優先殘項待處理）
- **記錄日期**：2026-09-27
- **對應**：`docs/operations/remediation-manifest.md` §3（E12/E22 更新 ＋ E34–E39 新增）、§6
- **本批納入的 PR（皆已併）**：**#2072**（E34 N-U4）、**#2074**（E38 負向證明規則層斷言）、**#2075**（E22 修復）、**#2076**（E36 校準改標的＋drift 偵測）、**#2077**（E37 Batch A）、**#2078**（E12 annotate→Router）、**#2079＋#2081**（E35 `auto_backfill` 解耦＋補測）、**#2082**（registry 收斂）
- **本 session 的兩項流程修正（已寫進各列）**：
  1. **重工事故與協定**（E35）：我的 child #2073 與 a2a-dev 的 #2079 是同項重複實作（相差 12 分鐘）⇒ 保留驗證較強者、撤回我的；建立「**指名實作者** ＋ 派工前三查（open PR／manifest owner／直接問對方）」協定，並在後續 **#2081／#2083** 上實際驗證協定有效。
  2. **「敘述 → 可執行斷言」的升級模式**（E38/E36/E37）：本 session 三次把「註解/敘述」升級為**會咬人的契約測試**（規則層標記 #2074、閘門↔指標一致性 #2081、`failedCount>0` 不可達的假設守門 #2083）。
- **本 session 我自己的兩次錯誤（已如實記錄）**：① `.omo/manifests` 最新日期以 **1 層 glob** 誤判（7 週 vs 實際 4 週）② **counter 推理陷阱**：把 legacy 錯標籤系列 `{daily="failed"}` 的 4906 當成真增量（正確形狀 `{stage=…}` 全為 0）。⇒ 紀律：**看到 counter 非零 increase 必須先驗 series 標籤形狀**。
- **決定性驗收（2026-09-29 06:00Z）**：以 `universe_snapshot.json` **輸出**為真值 ＋ `last_run_*` ＋ counters 增量 ＋ 三條舊告警維持沉默；若 `CounterEmissionMissing` 反而 fire ⇒ **發射面又壞**（非評分問題）。腳本：`session-artifacts/…/verify_0929.sh`。

---

### FU-20260926-34 — 第五批同步：**D2 量測結案、E37/E39 殘項已併、新增 E40–E43**（本 session 觀測性/真相類修復的收斂）

- **狀態**：`open`（E40/E41/E42 的**部署後驗收**、E43 的**跨 repo 修正**、以及各列殘項待處理）
- **記錄日期**：2026-09-27
- **對應**：`docs/operations/remediation-manifest.md` §3（D2/E37/E39 更新 ＋ E40–E43 新增）、§6
- **本批納入的 PR（皆已併）**：**#2063/#2065**（宇宙告警改讀輸出）、**#2072**（N-U4 明示化）、**#2074**（負向證明規則層斷言）、**#2075**（E22）、**#2076**（校準改標的＋drift）、**#2077**（Batch A）、**#2078**（E12）、**#2079/#2081**（B）、**#2082–#2084**（registry／spec／manifest）、**#2085/#2086**（traps 內嵌陷阱＋generate-drift 缺口）、**#2087**（真值模型＋判層工具＋規則 #10）、**#2090**（awk＋generate 覆蓋）、**#2091**（promtool 預算）、**#2092**（9→10 敘述）、**#2096**（可觀測性誠實化批次＋規則 #11）、**#2097/#2099**（Fubon/TWSE 量能語意契約 spec）、**#2098**（Batch B＋99.9% 更正）、**#2100/#2101**（detector 真相 24→29＋閘門）
- **本 session 我（root）自己的更正紀錄（8 次，皆為同一病：用代理指標取代權威來源）**：
  1. `.omo/manifests` 最新日期（1 層 glob ⇒ 誤判休眠 7 週，實為 4 週）
  2. counter 的 `increase` 讀數（把 **legacy 錯標籤系列 `{daily="failed"}`** 當真增量；正確形狀 `{stage=…}` 全為 0）⇒ 紀律：**看到 counter 非零 increase 必先驗標籤形狀**
  3. N-U4 產品意圖（用 flag help ＋ compose 取代 **`product-positioning.md`** ⇒ 判錯方向，業主更正）
  4. 「不要手動 update-branch」（**被自己的 watcher 污染**的觀察）⇒ 受控觀測反證：**本 repo 無 merge queue ⇒ BEHIND 必須手動對齊**（今晚 4 次實證）
  5. `merge-tree` 的 `changed in both` 被誤讀成「衝突」（實為「兩邊都改過」；`mergeable=MERGEABLE`、conflict 標記 0）
  6. 檢查方法用未解析成功的 ref（`origin/$HB`）⇒ 得 239 行假差；改用權威 `gh pr diff` ⇒ 0 非註解變更
  7. 任務書裡的因果主張未量測就寫（「(C) 的新告警會對過期 artifact 亮紅」⇒ child 實測推翻：**交易日曆**判定 ⇒ 連假不誤報）
  8. 轉述他人結論當事實（多次）⇒ **紀律：轉述一律區分「實測事實」與「推論／未驗證假設」，並要求對方複驗**（含任務書本身）
- **本 session 由 child 更正我的次數**：≥8（上列 3、5、6、7、8 皆屬此類）⇒ 我已把「**派遣前先驗當前狀態**」收緊為固定紀律
- **待辦**：E40/E41/E42 的部署後驗收（規則＋binary 一起、09-29 06:00Z 之後）；E43 由 wiki 維護線修；`snapshot_persisted_total` 語意、I29 分母半邊、`VolumeScope` 契約欄位、`PhaseOverheat` 語意錯配等皆已立列

### FU-20260929-01 — 生產 `quotes` 在 2026-06-25 之前只有 ~114 檔 ⇒ per-stock 量測的寬基樣本只剩 44–63 個交易日

- **狀態**：`done`（**已於 2026-09-29 部署窗口解除**；標題與原敘述全數保留，處置與證據見本條文末）
- **記錄日期**：2026-09-29
- **來源**：issue **#2093**（SBL per-stock 訊號價值量測）；工具 `cmd/experimental/sbl-ic-study`（**PR #2126** 新增）
- **現況**（2026-09-29 唯讀查生產）：`quotes` 共 **67,125 列 / 853 標的 / 2026-01-02 → 2026-09-29**，但逐日檔數在 **2026-06-24 前僅 114 檔**（`source='fugle_candles'`，大型權值），**2026-06-29 起才 834+ 檔**（`source='finmind_backfill'` 自 2026-06-25 起）。
  復現：`docker exec atlas-postgres psql -U atlas -d atlas -Atc "select date,count(*) from quotes where date < '2026-07-01' group by 1 order by 1 desc limit 5"`
- **影響**：事前註冊的樣本「SBL 2026-03-02 → 2026-09-24 **全市場**（~144 交易日）」實際不可得。可用的**寬基**（≥500 檔有標籤）日期數：T+1 **63**、T+5 **59**、T+20 **44**、同窗同步 **57**；其餘日期只有 ~114 檔大型股 ⇒ (a) T+20 的 `n_dates >= 60` 判準**結構上無法達成**（與訊號好壞無關）；(b) 2026-06-25 前的「小型/中型」分位只是大型股裡較小者。
- **最小修法建議（只建議，未實作）**：把 FinMind 全市場回填往前推到 2026-01（或至少推到 SBL 起點 2026-03-02），再重跑 `cmd/experimental/sbl-ic-study` —— 判準與工具都不必改，缺的只是價格廣度。量測工具已把「每個視野的寬基日期數」寫進報告 §8，避免下一個人再次把「樣本不足」誤讀成「沒有訊號」。
- **處置（2026-09-29 部署窗口；root 實測）**：quotes range 回補完成 ⇒
  `select count(distinct symbol) from quotes where date < '2026-06-25'` = **114 → 853** ✓
  可複現（生產 kmacmini）：

  ```bash
  docker exec atlas-postgres psql -U atlas -d atlas -Atc \
    "select count(distinct symbol) from quotes where date < '2026-06-25'"
  # 期望：853（回補前：114）
  ```
- **影響更新**：本阻塞**解除** —— M1 真小型股版本的**樣本條件具備**。
  **但 #2093 不因此重開**：其結案留言的重開觸發只認 **T+1／T+5**，且 **M1 方向已固定為負** ⇒
  本條僅更新**前置條件狀態**，不是「訊號結論翻案」。
- **仍未解（同批觀測的另外兩項阻礙）**：`FU-20260929-02`（`sbl_borrow_balance` 全 0 ⇒ 供給面無法量測）＋ forward IC ≈ 0。

---

### FU-20260929-02 — `sbl_borrow_balance` 在全部 316,171 筆 SBL 觀測中皆為 0 ⇒ 借券供給面無法量測

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：同 FU-20260929-01（#2093；同一份報告 §8 的自動註記）
- **現況**：`data/state/sbl/*_sbl.json`（生產 **144 檔 / 2026-03-02 → 2026-09-24**）的 `sbl_borrow_balance` **非零筆數 = 0 / 316,171**；`internal/marketdata/twse_sbl_provider.go` 的 FinMind 路徑只填 `sbl_short_balance` / `sbl_short_volume` / `sbl_return_volume` 三欄。
  復現：`jq -r '.[].sbl_borrow_balance' data/state/sbl/20260924_sbl.json | sort -u`（輸出只有 `0`），或看量測工具報告 §8 的自動註記。
- **影響**：供給面（券源 / 借券餘額）的分類學與後續檢定今天做不到；**不影響**已完成的 forward-IC 結論 —— 該結論只用到已填充的 3 個欄位，且 `sbl_borrow_balance` 為 0 不會產生「假訊號」，只會讓「供給面」這一格留白。

---

### FU-20260929-03 — #2134 `twse_oddlot` 通道退役後的**部署驗收**（未驗證：需容器重啟）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：issue **#2134**（CLOSED）→ PR **#2136**（已併）＋ root 的部署窗口驗收清單
- **現況**：`twse_oddlot` 已判定為**上游永久損壞**（TWSE 移除該端點）並在組態層退役；
  本項是**部署後**才成立的驗收條件（容器需以含該變更的 image 重啟）。
- **驗收（部署窗口，owner: a2a-dev）**：
  - `atlas_channel_health_status{channel="twse_oddlot"} = 3`
  - `ChannelHealthStatusError` 對該 channel **清空**
  - 可接受的**額外** series：`atlas_channel_health_status{channel="twse-oddlot"} = 3`（derived alias；若 regulator 端仍以舊名查詢）
  - （**於 #2138 落地後**）`atlas_channel_governance_overdue = 0`
- **影響**：未驗收前，無法區分「通道已退役」與「告警疲乏只是暫時消失」。

### FU-20260929-04 — 母體排除**比例告警**的門檻需先校準再上線（不得以未校準門檻 paging）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：issue **#2019**（CLOSED）→ PR **#2139**（已併）＋ root 的裁定
- **現況**：`symbols_excluded_reasons` 的細分已落地，但**排除比例的常態分布**尚無足夠樣本；
  門檻若此時硬編，會再製造一顆「看似有守門、實際誤報/漏報」的告警。
- **待辦（owner: 監控 lane）**：累積 **N 輪** `symbols_excluded_reasons` 後**校準門檻**再上線；
  **不得**以未校準門檻上 paging 告警（可用 warning 或先只做記錄）。
- **驗收**：部署後 baseline（首輪）排除比例為 **0** 或已在校準後門檻內，且無新增誤報。

### FU-20260929-05 — predictor 價值被證明後，再評估「讓 evaluator 依賴候選參數」（並把四個名字入表）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：issue **#2133**（CLOSED）→ PR **#2140**（已併）的結案衍生
- **現況（量測事實）**：`predictor_calibrate` 目前是 **measurement-only**：
  四個 `predictor_*` 名字**不在** `parameterTable`，`CalibrateParameters` 對解析不到的名字
  `if !ok { continue }` **靜默跳過** ⇒ 不會寫入任何參數（#2123 的 reader 修好後也只讓**量測**變誠實）。
- **待辦（owner: root）**：**若**市場方向預測器的價值被證明（樣本量、命中率、以及它對下游決策的貢獻），
  才評估「讓 evaluator 依賴候選參數 ＋ 把四個名字加入 `parameterTable`」。
- **硬性前置**：必須先有「**候選無改善 ⇒ 不得寫入**」的機制保證（**#2133 已完成**：`improved := improvement >= MinImprovement*100`，
  `improved==false` 跳過整個寫入階段）。⇒ 沒有這道保證就入表，會讓平曲面上的任意候選值被寫進參數
  （實測：`darwinian_weight_min 0.3 → 0.20571428`，−31.4%）。
- **影響**：在噪音上調參比不調更糟；本項刻意延後，直到價值被證明。

> **來源注記（2026-09-29/30 批次）**：`FU-20260929-03`～`-05` 與 `docs/operations/remediation-manifest.md`
> 的同步，來自 **#2134**（twse_oddlot 退役，PR #2136）／**#2019**（symbols_excluded 細分，PR #2139）／
> **#2133**（校準「無改善不得寫入」＋ predictor 誠實標示，PR #2140）／**#2139** 的結案。

### FU-20260929-06 — `#1944` 剩餘 **15 項** inert（唯一權威＝registry 的 bounded 清單；不在此複製明細）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：issue **#1944**；PR **#2146**（`daa55a25`，`2026-09-29T18:03:39Z` 已併，I29 分母改 pipeline 母體＋N-P1／N-P2 明示）交付其中 3 列；**本條為其餘項目的追蹤入口**
- **唯一權威（刻意不複製 15 條明細，避免與 registry 形成第二份互相矛盾的清單）**：
  `docs/reference/inert-registry.md` 的 `#### 仍待處理（bounded 清單）`（**18 列 − #2146 已交付 3 列**〔`I29`（`cmd/atlas` 半邊）／`N-P1`／`N-P2`〕＝ **15 列**）。
  該 registry 內這 3 列已各自標「（→ **Batch C：已處理**）」，故不需要另立「已交付」清單。
- **可複現抽取（期望輸出必須逐字相符；以下指令**照抄即可**，勿改字、勿加縮排）**：

```bash
python3 - <<'PY'
import re
src = open('docs/reference/inert-registry.md', encoding='utf-8').read()
i = src.index('#### 仍待處理（bounded 清單）'); j = src.index('#### 具名缺口', i)
rows = [l for l in src[i:j].split('\n') if l.startswith('|') and '---' not in l and not l.startswith('| ID')]
ids = [re.match(r'\|\s*(.+?)\s*\|', r).group(1) for r in rows]
delivered = {'**I29**（`cmd/atlas` 半邊）', '**N-P1**', '**N-P2**'}
print('bounded rows:', len(ids), '| open after #2146:', len([x for x in ids if x not in delivered]))
PY
# 期望輸出：bounded rows: 18 | open after #2146: 15
```

- **分流政策（窗內只允許「零行為誠實化」）**：
  - **會改變生產行為／值者** ⇒ 排在 **#1971 觀察窗（期滿 2026-10-28）之後**；
  - **只讓宣稱／量測變誠實且不改變行為者** ⇒ **現在做**；
  - 觀察窗內僅允許**零行為誠實化變體**（WARN log／標記／明示未使用），不得藉此改動數值或觸發條件。
- **保護分流（依上政策）**：
  - **受保護（窗後才動）**：`I32`／`I36` 的**修法半邊**／`I19` 殘留／`N-A1` 殘留／`N-A5`／`I22-overheat`／`I27` 殘留／`config validator`（現行修法會擋掉 production 啟動）／`I16(a)`。
  - **可即辦（零行為）**：`I7`（死碼移除）／`I30`／`I31`（CI 政策）／`I23`（純文字）／`I21` 與 `I36` 的**盤查半邊**（只查不修）。

---

### FU-20260929-07 — `internal/monitoring` 捕捉 harness 的**治本**殘留：暖機 goroutine 的生命週期

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：issue **#2147** → PR **#2148**（測試側並發安全；其 PR body 明寫「洩漏的**治本**修法刻意不塞本 PR，由 root 以 FU 登錄（指向該段註解）」）
- **已修（#2148，僅測試檔 +100/−3）**：`internal/monitoring/universe_scheduler_holiday_test.go` 的日誌捕捉改為 `syncBuffer`（mutex 保護的 `bytes.Buffer`），`captureLogs`／`captureLogsAt` 回傳它；並新增正向控制 `TestCaptureLogs_IsSafeUnderConcurrentWriters`（8 goroutine × 25 行＋主測試同時讀取 ⇒ 斷言 **200 行不掉**）。
  ⇒ 這移除的是**整個 race 類別**（任何現在或未來的寫者都受保護），不只是那一個已知洩漏。
- **殘留（治本未做）**：`NewDashboardAPI()` → `newWiredIndustryService` 啟的**非同步暖機 goroutine**（`go func()` ＋ 120s timeout，註解自陳 "don't block API startup"）會**存續到它所屬測試結束之後**，並透過 `internal/logging` 往**全域 logger** 寫 macro 警告 ⇒ 其下一行 log 可能落進**另一個測試**安裝的 sink（目前可接受，因為同套件斷言的是**排程 skip 訊息**＝不同字串，且暖機另有 120s 上限）。
- **建議下一步（治本）**：讓暖機具備**可取消的生命週期**（`Close()`／context 取消）⇒ 從根上移除「跨測試寫入」這個類別，而非靠「斷言字串不同」繞過。
- **註記**：屬 **#1971 觀察窗保護之外**（純測試基建、不動任何生產值或觸發條件）⇒ **可即辦**。

> **來源注記（2026-09-30 第二批）**：`FU-20260929-06`／`-07` 與 `docs/operations/remediation-manifest.md`
> 的兩處校正，來自 **#1944**（inert 追蹤需求）／**#2146**（I29 分母改 pipeline 母體＋N-P1／N-P2，`daa55a25`）／
> **#2148**（`internal/monitoring` 測試側並發安全）的結案與複核。

### FU-20260929-08 — Darwinian 權重對「未登記 agentID」**靜默 return**（G3 缺口；可即辦、零行為）

- **狀態**：`done`
- **記錄日期**：2026-09-29
- **完成於**：2026-09-29 `c2aa0e8f`（PR [#2159](https://github.com/kaecer68/atlas-go/pull/2159) squash）
  — `internal/portfolio/darwinian_weights.go:320-322` 補一則**每 (manager, `agent_id`) 一次**的 WARN
  `outcome_for_unregistered_agent`（`gap = "G3 / FU-20260929-08"`），只增可見度、**不改權重計算**；
  `ResetAgent()`（`:960-962`）的 `return false` 依原條目註記**未動** ✓
- **來源**：`#1944` T2／G3 分析（root 指派）；本 session 的靜默 no-op 家族
- **現況**：`internal/portfolio/darwinian_weights.go:321-323` 的 `recordOutcome()`：
  `w, exists := m.weights[agentID]` ⇒ `if !exists { return }` ⇒ **未登記 agent 的 outcome 被靜默丟棄**（無 log、無計數）。
  ⚠️ 同檔 `:961-963`（`ResetAgent()`）是 `return false`＝**具名回傳值**，呼叫端可判 ⇒ **不同型缺陷，不要一起改**（本條只處理 `recordOutcome`）。
- **影響**：weights map 未含該 id 時（拼字不一致、載入失敗、21 個 key 之外的 id），該 agent 的訊號**永久不進 Darwinian 權重且不留痕** —— 無從分辨「沒有訊號」與「訊號被丟掉」。
- **處置（可即辦、零行為）**：補一則 **WARN**（`agent_id` ＋「weights map 未含此 id」；量多時可一次性節流）⇒ 只增加可見度，**不改權重計算**。

---

### FU-20260929-09 — Form B：`atlas_agent_recommendation_skips_total{agent_id,reason}`（趨勢需求出現時再做）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：`#1944` T2 的觀測面缺口分析（Form B）
- **現況**：skip 歸因目前是**結構化輸出**（`internal/orchestrator/executor_collection.go:80` 的 label 映射：
  `skips_no_quote`／`skips_not_tradable`／`skips_factor_quality_gate`／`skips_executor_declined`，以及
  `skips_by_agent` 的 per-(agent, reason) 明細，`:460`）＋ trace/log；**沒有 Prometheus counter**（實查 `grep atlas_agent_recommendation_skips_total` ⇒ 0 命中）。
- **邊界**：agent **21** × reason **4**（`no_quote`／`not_tradable`／`factor_quality_gate`／`executor_declined`）
  ⇒ **84 條 series**（有界 ✓，符合本 repo 的基數紀律）。
- **處置**：**先不做**。單場次歸因用現行結構化輸出已足夠；只有出現「跨場次趨勢／告警」需求時才新增 counter。

---

### FU-20260929-10 — 4 個 criteria 擋住的 agent 校準（**值語意 ⇒ 觀察窗後**）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：T2 線的校準分析（root 轉述）；agent 定義實查於 `configs/agents.json`
- **現況**：
  - `stockpicker-winrate-01`（`configs/agents.json:645`，`enabled: true`）：校準門檻**遠超可達**；且**準則／母體不匹配**
    —— 以 **ETF 母體**套用**股票級**門檻 ⇒ 結構上不可能通過，與訊號好壞無關。
  - `leo-satellite-desk-01`：邊緣（貼近門檻）。
- **影響**：這幾個 agent 的參數**永遠不會被校準**，但其「未達標」會被讀成模型表現差而非判準錯配。
- **處置**：需要改**門檻值語意**（準則與母體對齊）⇒ **排在 `#1971` 觀察窗（期滿 2026-10-28）之後**。

---

### FU-20260929-11 — T2 結論更正：A 組的真正鑑別閘門是 `factor_quality_gate`（B 型）；`no_quote` 主導是**全域注入基線**（**值語意 ⇒ 窗後**）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：T2 線的生產讀數（**2026-09-29 場次 `session-20260929-daily`**；數字為該線實測，非本條作者量測）
- **現況（已更正；讀數已取得並經獨立複核）**：部署後 `#2155` 拆標籤的實際讀數（trace `session-20260929-daily`，
  `ts=2026-09-29T20:51:04Z` ≥ 上線 `20:43:21Z`；日誌側同一支回掃描 16 行）**推翻了本條先前的先驗**：
  1. **`not_tradable` 在部署後 16 行中全為 0** ⇒ 先前的「已追蹤場次偏 `not_tradable`／未追蹤批次路徑偏 `no_quote`」**不成立**。
  2. **A 組的真正鑑別閘門＝`factor_quality_gate`（B 型）**（per-agent：`semi-desk-01` **4**／`mining-desk-01` **5**／
     `consumer-desk-01` **5**／`earnings-quality-01` **6**）—— 這才是 agent 自己的判斷差異。
  3. **`no_quote`（881／85.8%）不是 A 組專屬，而是「全域注入基線」**（機制與資料指紋見 `FU-20260929-14`）：
     19 個 agent 皆 44、`ai-desk-01` 45、**唯一未被注入的 `stockpicker-winrate-01` 為 0** ⇒ 該值量的是
     「注入集合 ∩ 當場 quote set」，**不帶 agent 判斷資訊**。
  4. **可複現**：`881（19×44＋1×45）＋0＋137＋9＝1027` 由 per-agent breakdown 逐項重算即得，與彙總一致；
     同一場次兩次執行（`20:48:31Z`／`20:51:04Z`）逐項相同。
- **影響（更正）**：把 A 型讀成「agent 挑不到股票」＝**把注入基線誤讀成 agent 品質**；真正要看的 per-agent 訊號是
  `factor_quality_gate`（A 組 4–6；全體 2–31）與單一 agent 的 `executor_declined`（9）。
- **待辦**：修**值語意**（候選集合）⇒ **排在 `#1971` 觀察窗之後**（子分支分辨已完成，不再是待辦）。

---

### FU-20260929-12 — T2 追蹤入口（T1／T2 已上線；**讀數已取得並經獨立複核**；結論分層見 `-11`／機制見 `-14`）

- **狀態**：`open`
- **記錄日期**：2026-09-29
- **來源**：`#1944` T1／T2 系列（`#2153`／`#2155`）；本條為**追蹤入口**，不重複其餘條目的細節
- **現況**：
  - **T1（`#2153`）已上線**：推薦收集器的三個靜默 skip 已可量測，並已有**第一份生產讀數** ✓
  - **T2（`#2155`）已上線**：skip 歸因**拆標籤**（`no_quote`／`not_tradable`）＋ **per-agent × per-reason**（21×4＝84）＋
    無場次 ID 時降為 DEBUG（避免雜訊）✓（已隨小部署窗口進入生產：`/api/version` = `6f64c223`）
  - 診斷**已收斂並更正**：per-agent 真訊號＝`factor_quality_gate`（**B 型**；A 組 4–6）；`no_quote`（881／85.8%）是
    **全域注入基線**（`FU-20260929-14`），**不是** agent 品質差異；`not_tradable` 部署後全為 0 ⇒
    原「tradability 為主」的判斷**不成立**（見 `FU-20260929-11`）。
  - **讀數**：trace 側 `session-20260929-daily`（`ts` 部署後）＋ 日誌側同一支回掃描 16 行 ⇒ 已由 **lane A 獨立複核**
    （數字由原始欄位逐項重算一致；`ts` ≥ 上線時間；注入機制作 `file:line` 驗證）。
- **剩餘工作**：窗後修**值語意**（指向 `FU-20260929-11` 與 `FU-20260929-14`）。
- **同批已結**：`screened_symbols.jsonl` 的 rejects-only 敘述（I36）**已由 `#2154` 修掉**（測試釘住）⇒ 不另立條目。

---

### FU-20260929-13 — `#2151`：replay／extended 價格序列未做**公司行為調整**（**值語意 ⇒ 窗後**）

- **狀態**：`done`
- **記錄日期**：2026-09-29
- **完成於**：2026-09-30 `401138df`（PR [#2161](https://github.com/kaecer68/atlas-go/pull/2161) squash；來源 issue **#2151 已 CLOSED（COMPLETED）**）
  — 修法（**業主 2026-09-30 授權提前變更**，以 #2161 上線日為分界）：
  ①新增 `internal/marketdata/official_series_actions.go`：由**官方還原序列**（FinMind `TaiwanStockPriceAdj`）推導 `domain.CorporateAction`
  ②`internal/orchestrator/composition.go` 的 `loadHistoricalPrices(replayCSVPath, jsonlPath)` seam 固定「確保 JSONL（CSV→JSONL）⇒ 載入 ⇒ 套用調整」
  ③`internal/portfolio/historical_prices.go` 修正**股票股利係數公式**（`(10−S)/10` 為負 ⇒ 改 `10/(10+S)`，以官方指紋釘住：`6669` 官方/原始 `2614.997/7800=0.33526` vs `10/(10+19.827946)=0.33528`）
  ⇒ 驗收：3 檔（`6669`／`0050`／`0052`）假斷崖消除（max 單日 ≤10%）、`ret20` 與官方一致、無事件符號**逐位元不變**、mutation 紅；**回滾兩條**（移除資料目錄＝no-op＋一行 log／`git revert`）
- **部署驗收（窗口）**：log 必須出現 `official price adjustments applied` 且 `symbols_adjusted > 0`（若見 `official adjusted prices unavailable…` ⇒ 未命中推導路徑 ⇒ 等於沒修）
- **同機制的第二表現（本次量測發現）**：收集器端 `screened_items` 對 `6669.TW` 的 `factor_scores` 為退化值（`momentum=-1, quality=-1, liquidity=0`）⇒ `ai-desk-01`／`growth-momentum-01` 因此被 `momentum_20d.min=-0.5` 與 `factor_quality_gate` 擋下。
  ⇒ **部署後複驗**：`6669.TW` 的 `momentum ≠ −1`（且 quality／liquidity 不再同時退化）；若拒絕仍在，**必須報出殘留的 `criterion`／`threshold`／`actual_value`**，不得直接判「#2151 未生效」（該符號仍可能過不了其他閘）
- **部署後複驗結果（2026-09-30，lane B 唯讀；場次 `session-20260930-daily`，`ts 00:16:49Z` ≥ 分界 00:08:50Z）**：
  ✅ **PASS** — ①部署後**無任何** `momentum_20d_min` 拒絕（部署前 ai＋growth 共 20 row 全為該符號）②skip 桶位移對帳：`ai-desk-01` `{fq2,no_quote45}`→`{fq3,no_quote1}`、`growth-momentum-01` `{fq5,no_quote44}`→`{fq6}` ⇒ 兩者各恰好 **+1 進品質閘**＝該符號 ✓ ③輸入端自行重算（官方序列）**20 根動能 = −0.13** vs raw **−0.7083**（raw 巨幅跳點 2026-09-01 `7800→2610` ✓）
  ⚠️ 限制（申報）：引擎端**不印出**該符號的 `factor_scores.momentum`（既非 rejected 亦非 recommended）⇒ 結論來自「準則通過＋桶位移＋輸入重算」三層，非直讀 ✓
  ★ **可見行為變更（本條的預期效果）**：`etf-rotation-01` 的建議集合 **1 進（`00929.TW`）／2 出（`00881.TW`、`00891.TW`）**；
  對照組 `session-20260927→0928→0929` 變動 = **0**（排除日常市場差異 ✓）；該 3 檔 raw vs 官方序列在 **517／517／539（共 546）根**上比值 ≠1 ⇒ 屬本條調整的 **replay 44 檔** ✓
  ⇒ 已另記於 `#1971` 的部署分界附錄 ✓
- **現況**：replay/extended 的價格序列未做公司行為（分割／減資）調整 ⇒ **索引式** `ret20`／`volatility`
  對分割符號產生**假跌**（實證：3/44 檔）。
- **影響**：任何以「索引」抓取該序列的因子在分割日附近會看到不存在的崩跌 ⇒ 汙染特徵與回測結論。
- **處置**：修法改的是**數值語意** ⇒ **排在 `#1971` 觀察窗之後**（窗內僅允許零行為誠實化變體）。

> **來源注記（2026-09-30 第三批）**：`FU-20260929-08`～`-13` 來自 `#1944` 的 T1／T2 系列
> （`#2153` T1、`#2155` T2 拆標籤、`#2154` I36 敘述修正）與 G3 缺口盤查；其中 T2 的生產讀數由 T2 線實測，
> 本批僅登錄（來源具名於各條）。

### FU-20260929-14 — `ExpandUniverse(constants.ReplayCSVPath, nil)` 把整組 CSV 符號注入**每個自帶 universe 的 agent** ⇒ 產生與 agent 判斷無關的 `no_quote` 基線（**計數可即辦／值語意 ⇒ 窗後**）

- **狀態**：`done`（**計數半邊**；值語意半邊改由 `FU-20260930-05` 承接）
- **完成於（計數半邊）**：2026-09-30 `76ad565c`（PR [#2163](https://github.com/kaecer68/atlas-go/pull/2163) squash，「零行為變更」）
  — 根因：`ExpandUniverse` 回傳的是 `loadSymbolsFromCSV` 的**裸 code**，而報價 map 的鍵是 `<code>.TW` ⇒ **注入的 44 檔 ETF 永遠對不上報價** ⇒ 每 agent 恰好 44 的 `no_quote` 基線（第三證據：唯一未被注入的 `stockpicker-winrate-01` `no_quote=0`）。
  — 修法：注入仍保留（意圖不變 ✓），但只把**有報價者**加入掃描清單；靜默 no-op 改成**每場一行** `injected_symbols_unquoted` WARN（具名 `likely_cause`）。
  — 實測：`skips_total` 1027 → 預期 ~**146**（1027 ＝ 881 注入 ＋ 137 `factor_quality_gate` ＋ 9 `executor_declined`）；`injected_no_quote` 成為**不變量（gate on 時結構上必 0）**。
  — 證明方式：包住 `PluginRegistry.screener` 記錄 `ScreenDetailed` 呼叫序列，**gate on／off 逐位元相同**（＋反空洞斷言）；gate off 案例保留**修前 2/2** 作 before 對照。
- **記錄日期**：2026-09-29
- **來源**：T2 讀數的**獨立複核**（lane A）＋ B lane 的 trace 產物（`t2-reading-20260929T205104Z.txt`）
- **機制（file:line）**：
  `internal/orchestrator/executor_collection.go:98-115`
  ```
  symbols := agent.Universe                                            // :98
  if len(symbols) == 0 { symbols = DefaultSymbols() … }                // :99-100
  else { expanded := ExpandUniverse(constants.ReplayCSVPath, nil); … } // :103 ＋ 合併去重 :104-115
  …
  if !ok { skips.record(agent.ID, skipReasonNoQuote) }                 // :121
  ```
  ⇒ **有自帶 universe 的 agent 一律被注入整份 replay CSV 的符號集**，而這些符號不在當場的 quote set ⇒
  逐符號記 `no_quote` ⇒ 形成**與 agent 判斷無關的均勻基線**。
  （`ExpandUniverse` 定義：`internal/orchestrator/executor_symbols.go:136`。）
- **資料指紋（讀者可自行複驗）**：
  1. per-agent `no_quote`：**19 個 agent = 44**、`ai-desk-01` = **45**（44 ＋ 它自己 universe 多出的 1 檔）
     ⇒ `19×44 ＋ 1×45 = 881` ＝ 彙總 `skips_no_quote` ✓
  2. **唯一未被注入者 `stockpicker-winrate-01` 的 `no_quote` = 0** ✓（它走 `DefaultSymbols()` 分支）
  3. 總額對帳：`881 ＋ 0（not_tradable）＋ 137（factor_quality_gate）＋ 9（executor_declined）＝ 1027 = skips_total` ✓
- **措辭（重要）**：日誌側 16 行（同一支回補掃描 14 行 ＋ 追蹤 2 行）數字幾乎相同 ⇒ 只能說
  「**對場次日期不變（同一掃描內）**」，**不是**「16 個獨立場次各自量到相同值」。
- **警告（兩個 44 不同義）**：`quote_count=44`（**當場 quote set 大小**）≠ 「注入基線 44」（**replay CSV 符號數**）
  ⇒ 文件與判讀不得當同義。
- **讀數陷阱（順帶登記）**：
  1. **同一份資料兩個鍵名** —— 日誌行用**裸名**（`no_quote=`，來源 `executor_collection.go:641` ＋ 常數 `:510`），
     trace 用 **`skips_` 前綴**（`skips_no_quote`，`:452`）⇒ join 兩側的工具必踩空。
  2. 日誌的 `agents_iterated=30` **≠** trace per-agent 的 **21**（30＝迭代過的 agent 數，含零 skip 者）。
- **待辦（兩支，分流不同）**：
  - **(a) 計數／可觀測性（零行為 ⇒ 可即辦）**：把「**注入**」與「**agent 自帶**」兩個來源的 skip **分開計數／分開輸出**
    （例如 `skips_*` 之外多一組 `injected_*`，或 trace 另加欄位）⇒ 只改輸出、**不改候選集合** ⇒ 與 G3 的 WARN 同類。
  - **(b) 候選集合（值語意 ⇒ 觀察窗後）**：只展開「**該場真的有報價**」的符號 ⇒ 改變候選集合 ⇒ 排在 `#1971` 觀察窗之後。
- **可選更深檢查（不阻擋）**：① 注入集合與當場 quote set 的**實際集合差** ② `agents_iterated=30` 中**零 skip 的 9 個**是誰、
  是否 enabled ③ 兩個 44 是否**恰好不交集**（若部分交集 ⇒ 基線 < 44，可反推交集大小）。
- **影響**：在 (a) 完成前，任何「以 `no_quote` 大小比較 agent」的盤查都會**把注入量誤讀成 agent 品質**（本條即為該更正）。


### FU-20260930-01 — `GetPhaseWeightMultiplier`（過熱 0.90）**未被任何生產路徑消費**（test-only ⇒ 誤導性風控外觀）

- **狀態**：`open`
- **記錄日期**：2026-09-30
- **來源**：`#2160`（I22-overheat 修復）設計審查時發現
- **現況**：`internal/industry/silicon_cycle.go:414-419` 定義 `GetPhaseWeightMultiplier`（過熱回 0.90），
  但**全 repo 唯一呼叫點是測試** `internal/industry/silicon_cycle_test.go:276` ⇒ 生產 0 呼叫者。
  實際接線的曝險路徑是**狀態卡的矽層分數**（`internal/industry/cycle_status_card.go` 的 `buildAdj`：
  擴張 `+0.5w`、過熱 `−0.1w`），與這個函式無關。
- **指紋**：`git grep -n 'GetPhaseWeightMultiplier' origin/main`
  ⇒ 只有 `silicon_cycle.go`（定義／`// GetPhaseWeightMultiplier returns ...` 註解）＋ `silicon_cycle_test.go`（測試）
- **風險**：函式名與數值**看起來**是風控槓桿，實際上不存在 ⇒ 讀者或後續 agent 可能誤以為
  「調它就能降曝險」（與 `#1944` inert 家族同型：規則存在但不在生效路徑上）。
- **最小處置建議（不改變行為 ⇒ 可即辦）**：在其 doc comment 明寫
  `DECLARED ONLY / NOT ENFORCED`＋理由，並把測試改為**斷言「無生產消費者」的守門測試**
  （若日後有人接線，測試即紅 ⇒ 迫使同步更新本條目）。要真正接線則是**值語意變更 ⇒ 需設計決定**。

---

### FU-20260930-02 — `macro` 快照保留政策 `MaxAgeDays=90` 對「60 交易日 MA」視窗餘裕不足

- **狀態**：`open`
- **記錄日期**：2026-09-30
- **來源**：`#2160`（`internal/industry/silicon_index_ma.go` 需 60 個交易日序列）
- **現況**：`internal/storage/lifecycle.go:76-80`：`Dir: "macro"`、`MaxAgeDays: 90`、
  `Pattern: "20*.json"`、`ExcludeFiles: ["latest.json"]`（`margin` 亦為 90）。
  生產實測（kmacmini，2026-09-30）：`data/state/macro/*.json` **629 檔、最舊 2024-07-01**
  ⇒ **目前並未在清**；但政策一旦生效（或部署到新主機）⇒ 只剩約 64 個交易日 ⇒ MA60 僅餘 ~4 日緩衝。
- **指紋**：`ls data/state/macro/*.json | wc -l` ⇒ 629；最早檔名 ⇒ `2024-07-01.json`
- **風險**：序列深度不足時 `LoadSiliconIndexMADeviation` 走 **fallback ＋ 具名 WARN**（非靜默 ✓）
  但指標語意退回 legacy（單日報酬）⇒ 對外數值與修後定義不一致。
- **最小處置建議**：把 `macro` 的 `MaxAgeDays` 提高到 ≥180（≈128 交易日）或改以**交易日數**為保留單位；
  與 `#1971` 觀察窗無關（純保留策略、不改變任何計算）⇒ **可即辦**。

---

### FU-20260930-03 — `quotes` 壞值（**非公司行為**）待資料修復：`close<=0` ＋ 10 倍價 ＋ 垃圾列

- **狀態**：`open`
- **記錄日期**：2026-09-30
- **來源**：`#2151`（公司行為調整）調查；`#2095`（`-14`(b)）複核時另一 lane 以官方還原序列逐檔驗證
- **現況**（生產實測 2026-09-30）：
  1. `quotes` 共 **125,615 列**，其中 `close <= 0` **109 列／32 個符號**（`Close=0` 被寫入 ⇒ 停牌/無收盤未表達為缺值）
  2. `5904.TW` 2026-08-10：原始 `close=720`，而官方還原序列同日為 `72` ⇒ **約 10 倍壞值**（非分割）
  3. `2380.TW` 2026-06-29：原始 `close=6.6`，官方序列 `23.86 → 21.5` ⇒ **垃圾列**（非乾淨倍數、無股利記錄）
- **指紋**：`select count(*) from quotes where close <= 0` ⇒ **109**；
  `select count(*) from quotes` ⇒ **125615**（`atlas-postgres`，2026-09-30）
- **影響**：任何**未先過 `Close>0`** 的 return／volatility 消費者會把上述日子算成 **−100%**（或 10 倍誤差）。
  `IsTradable`（`internal/apigateway/adapter_twse.go:131`：`Close > 0 && Volume > 0`）只守住部分路徑。
- **最小處置建議**：①寫入端把 `close<=0` 視為**缺值**（拒絕或標記，而非 0）②`5904`／`2380` 修值或標缺
  ③與 `FU-20260930-04` 併做（先盤點再決定修法）。

---

### FU-20260930-04 — 稽核「未守 `Close>0`」的 return／volatility 消費者

- **狀態**：`open`
- **記錄日期**：2026-09-30
- **來源**：`#2151`／`FU-20260930-03` 衍生（壞值 × 未守門 = −100% 假訊號）
- **現況**：尚未逐一盤點。已知主要消費面為 `portfolio.HistoricalPrices` 的
  `GetCloseSeries`／`MomentumReturn`／`Volatility`（`internal/portfolio/historical_prices.go`）；
  `#2151` 已在其上做**公司行為**調整，但**未處理壞值**（調整不該處理壞值 ⇒ 兩件事分開）。
- **指紋**：待產出「消費者 × 是否守 `Close>0`」清單（本條即為該清單的產出任務）
- **最小處置建議**：產出**表格化盤點**（消費者 `file:line`／是否守門／未守門時對壞列的行為），
  再決定是否補守門 —— 補守門屬**值語意變更 ⇒ 窗後或需業主授權**。

---

### FU-20260930-05 — `stockpicker-winrate-01` 的 `volume_intraday` 門檻按**股票**校準、母體是 **ETF**（4 檔被擋；**預期交付效果為 0** ⇒ 產品決定）

- **狀態**：`open`（**產品決定**，非缺陷修復）
- **記錄日期**：2026-09-30
- **來源**：`FU-20260929-10`（criteria 校準）的量測結論；量測由 B lane 以生產 API ＋ trace 對照完成
- **現況**：`configs/agents.json` 的 `stockpicker-winrate-01`：`volume_intraday.min = 1,000,000`，
  而它的 universe 為空 ⇒ 走 `DefaultSymbols()`（**ETF 母體**）。實測被 screening 擋下的 **4 檔 ETF**：
  `0051.TW`／`0053.TW`／`006208.TW`／`00692.TW`（36 rows ÷ 每場 10 pass ⇒ distinct 4 檔），
  實際量能 **45,452–978,191（中位 164,985 ⇒ 門檻的 0.045–0.98×）**。
- **★ 為何不改（量化理由）**：用**收集器同一個品質閘判準**（`momentum/value/quality/liquidity` 平均 ≥ 0.40）
  對這 36 rows 的 `factor_scores` 重算 ⇒ **0/36 過得了**（min 0.127／中位 0.146／max 0.188）
  ⇒ 把 `volume_intraday` 降到 ETF 可及，只會**換一個閘被擋**（screening → `factor_quality_gate`）
  ⇒ **有值變更、零交付效果** ⇒ 依「預期交付效果為 0 的值變更不得做」**不做** ✓
- **三條候選路（皆為值變更 ⇒ 需業主決定）**：
  ① 改判**成交金額**（不受股數／ETF 面額影響）② 對 ETF **豁免**股數門檻 ③ 降到 ETF 可及值（依賴 ETF 規模分布，脆弱）
- **為何不在此動 `configs/agents.json`**：JSON 無法寫註解 ⇒ 為了一句說明而動設定會變成「設定即文件」
  ⇒ 以本 FU 記載取代 ✓（若日後選 ①／③，才連同 `configs/agents.json` 一起改）
- **一併結案（同批量測）**：`leo-satellite-desk-01` 在該場 **0 個 screening 拒絕** ⇒ 該半邊
  **非準則問題**（其 5 檔是被 `factor_quality_gate` 擋，屬輸入值語意 ⇒ `FU-20260929-13` 家族）✓
- **量測註記（防後人誤讀）**：`GET /api/dashboard/recommendation-pipeline` 的 `screened_items` 對同一
  `(agent, symbol)` **每場約記 10 次**（收集器 ~10 pass）⇒ 任何「按 row 數」的結論都會 **×10**
  ⇒ 一律使用 **distinct symbol** ✓

---

### FU-20260930-06 — 排程任務 `auto_calibrate` 在容器內**必然失敗**（`exec: "go": executable file not found in $PATH`）

- **狀態**：`open`
- **記錄日期**：2026-09-30
- **來源**：`#2161`/#2151 部署窗口驗收時，逐條讀 `atlas-go` 容器 log 發現（root 唯讀實查）
- **現況（原文）**：
  ```
  time=2026-09-30T00:10:47.695Z level=INFO msg=task_started name=auto_calibrate component=background_task
  time=2026-09-30T00:10:47.695Z level=WARN msg=failed err="exec: \"go\": executable file not found in $PATH" output="" component=auto_calibrate
  ```
  同一支 log 內，**兄弟任務** `seasonal_calibration` 則正常：`msg=exec_ok binary=/app/calibrate-seasonal output_len=20599`
  ⇒ 即：容器內**沒有 Go 工具鏈**，而 `auto_calibrate` 是以 `go`（run/exec）為入口 ⇒ 該任務在生產**結構上不可能成功**
- **影響**：自動校準路徑在生產**從未執行**（只留下的痕跡是一行 WARN）⇒ 與 `#1944` inert 家族同型
  （規則／任務存在，但不在生效路徑上）。⚠️ **未確認**：校準是否另有等價路徑（例如其他 cron 容器或 overlay 寫入端）在實際補上；
  在有定論前**不得**宣稱「校準完全沒跑」（本條只陳述觀察到的事實）。
- **觀測量**：近 6h `task_started name=auto_calibrate` 3 筆、`executable file not found` 1 筆（同進程可能只記一次）
- **最小修法建議（照兄弟任務的模式）**：改為呼叫**隨映像出貨的 binary**（如 `/app/calibrate-seasonal` 之於 `seasonal_calibration`），
  或在容器內明示為 no-op ＋ 具名 `reason`（而非一行 WARN 假裝嘗試過）⇒ **不改變數值語意** ⇒ 可即辦
- **查法（可重現）**：
  ```
  ssh kmacmini '/usr/local/bin/docker logs atlas-go --since 6h 2>&1 | grep -i "auto_calibrate\|executable file not found"'
  ```

---

### FU-20260930-07 — pre-push **Gate 1b（ci-full）** 被 4 個「**本機資料相依**」既有測試擋下 ⇒ 所有 lane 都無法正常 push

- **狀態**：`open`
- **記錄日期**：2026-09-30
- **來源**：`FU-20260930-06`（讓 `auto_calibrate` 真的跑）的 PR 被 pre-push 擋下時定位；由 B lane 實查
- **現象（原文）**：`make ci-full` 在**本 PR 未觸及**的 2 個 package 紅：
  ```
  --- FAIL: TestRegisterRoutes_UsesDefaultStaticCF            internal/eventdriven  handler_inject_test.go:83
           default staticCF summary missing "關鍵事件"
  --- FAIL: TestRegisterRoutesWithCapitalFlow_BullishTilt      internal/eventdriven  handler_inject_test.go:139
  --- FAIL: TestRegisterRoutesWithCapitalFlow_NilProviderFallsBack  internal/eventdriven  handler_inject_test.go:167
  --- FAIL: TestHandleConditionWinRate_HappyPath               internal/stocktools   win_rate_test.go:352
           observations=2 symbols=2, want 4/2／hits=0 win_rate=0, want 2/0.5／date range 2026-08-20~2026-08-20
  ```
- **★ 分類舉證（在**未改動的 main** 上可重現）**：
  ```
  $ go test ./internal/eventdriven/ ./internal/stocktools/ -count=1
    main clone @ d431f695（未改動）→ 同樣 4 個測試全紅
    lane worktree @ 6a5cf464      → 同樣 4 個測試全紅
  ```
  ⇒ 屬 **既有／本機資料相依紅**（非 `misspell`／`staticcheck SA4000`／`gofmt` 等真失敗 ✓）
- **交叉證據（為何判斷為「本機」而非「CI」）**：近期合併的 PR（#2161／#2163／#2165 等）**CI 全綠** ⇒ 這 4 個測試在 CI 環境會過
- **影響（真實且立即）**：`.githooks/pre-push` 的 **Gate 1b＝有程式碼變更時跑 `ci-full`** ⇒ 這 4 個無關紅會**拒絕所有 lane 的 push** ✗
  （正規出口只有 `.githooks/pre-push:16` **記載**的 `PRE_PUSH_FULL=never`；`--no-verify` 為**禁止**用法 ✗）
- **歸類**：issue **#1927**（`tests/scripts fixtures are not hermetic`）的**具體實例**
- **最小修法建議**：讓期望值**不依賴本機 `data/state/**`**（注入固定 fixture ✓），或在 fixture 缺席時**明確 `skip` ＋ 具名 reason** ✓（**不得**靜默調整期望值 ✓）
- **查法（可重現）**：`go test ./internal/eventdriven/ ./internal/stocktools/ -count=1`（在任何未改動 main 的 clone 上 ✓）

---

> **來源注記（2026-09-30 第五批）**：`FU-20260930-01`／`-02` 來自 `#2160`（I22-overheat）實作與審查；
> `-03`／`-04` 來自 `#2151`（公司行為調整）與 `#2095`（注入符號）兩線的交叉調查；
> `-05` 來自 `FU-20260929-10`（criteria 校準）的量測結論（同一批的 `stockpicker` ETF 母體與 `leo` 半邊結案）。
> 數字皆由**原始欄位重算**（`quotes` 列數、`macro` 檔數與最早日期），機制皆附 `file:line`。

> **來源注記（2026-09-30 第四批／T2 更正）**：`FU-20260929-14` 與 `-11`／`-12` 的更正，來自
> **T2 讀數的獨立複核**（lane A，依 `~/workspace/atlas-notes/README-t2-reading.md` 的三項先寫死判準）
> ＋ B lane 的 trace 產物；數字由**原始欄位逐項重算**、注入機制作 **`file:line` 驗證**。

`FU-20260929-08`～`-13` 來自 `#1944` 的 T1／T2 系列
> （`#2153` T1、`#2155` T2 拆標籤、`#2154` I36 敘述修正）與 G3 缺口盤查；其中 T2 的生產讀數由 T2 線實測，
> 本批僅登錄（來源具名於各條）。

## 相關文件

- [universe-scoring-ranked-zero-20260925.md](universe-scoring-ranked-zero-20260925.md)
  — SmartUniverseBuilder `symbols_ranked=0` 根因報告（含 §6.2 缺口與 §8 防再犯檢查建議）
- [README.md](README.md) — 本目錄索引
- [local-deploy.md](local-deploy.md) — 部署與 `.env` 分工（prod DSN 為何不放 `.env`）
