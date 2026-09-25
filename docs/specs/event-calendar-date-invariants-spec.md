---
title: 事件日曆日期不變式規格（event-calendar date invariants）
status: active
updated: 2026-09-25
owner: industry / taiwanholidays
related:
  - docs/reference/traps.md（`build*Event` 契約：EventType + 日期自洽）
  - docs/jev/JEV-EVAL-STAGE3-EVENTS.md（Stage 3 事件層評估，缺陷的發現處）
  - docs/jev/JEV-EVAL-FRAMEWORK.md（匯出器品質閘門）
---

# 事件日曆日期不變式規格

本檔是 issue [#1973](https://github.com/kaecer68/atlas-go/issues/1973) 的權威規格：事件日曆一個 occurrence 的三個日期
（`StartDate` / `PeakDate` / `EndDate`）必須滿足什麼契約、契約如何被程式與測試守住、以及「日期不可判定」時的正確行為。

## 1. 問題（2021 實測）

`internal/industry/event_calendar.go` 的日期規則是決定性的，但在 2021 產出三類**語意錯誤**的日期，且沒有任何測試或護欄擋住：

| 缺陷 | 產生器 | 2021 實測 |
|---|---|---|
| **D1** 年份固定 peak | `buildMonthlyEvent` 對每個月都取 `rule.ComputePeakDate(year)` | `futures_settlement` 12 個 occurrence 的 `PeakDate` 全是 `2021-06-16`（落在自己窗口內 1/12）；`investor_conference` 4 個全是 `2021-07-15`（1/4） |
| **D2** 反轉窗口 | `buildPositionBuildingEvent` 的 Start 來自 `lastWeekStart`、End 來自 `lastTwoWeekStart` | 4/4 occurrence `StartDate > EndDate`（例 `2021-06-24 > 2021-06-16`）⇒ `DetectActiveEvents` 永不 active |
| **D3** 未驗證農曆走 placeholder | `taiwanPublicHolidays` / `spring_festival` 在表外年份回傳慣例猜測日期 | 2021 春節算成 `02-01`（真值 `02-12`）、端午 `06-10`（真值 `06-14`）、中秋 `09-20`（真值 `09-21`）；`spring_festival` 窗口 `01-27..02-11` 完全錯過真實春節 |

症狀只在「取峰值日」與「取窗口」的路徑出現（`DetectActiveEvents` 比對 Start..End），所以缺陷難以察覺。
發現處：Stage 3 事件層 GT 匯出（`cmd/experimental/jev-eval-events` 的 `-quality-gate`）在 2021 視窗擋下 14（peak 在窗口外）+ 4（反轉）+ 8（農曆未驗證）= 26 筆。

## 2. 契約（occurrence 日期不變式）

對**每一個**產生的 occurrence：

```
StartDate <= EndDate             （窗口方向正確）
StartDate <= PeakDate <= EndDate （峰值日屬於這個 occurrence）
```

程式碼層對應：

- `industry.CheckDateInvariants(events)` 回傳所有違反者（`DateInvariantViolation{EventID, Reason, Start, Peak, End}`）。
  Reason 只有兩種：`InvariantReasonInvertedWindow`（`inverted_window`）、`InvariantReasonPeakOutsideWindow`（`peak_outside_window`）。
- `EventCalendar.RefreshEvents` 在品質閘門之後對每個違反者寫 `logging.Warn("event_calendar", "event_date_invariant_violation", ...)`。
  **只警告不丟棄**：丟棄會讓事件集無聲變小，警告讓缺陷可見（本 issue 的教訓是「沒有東西會叫」）。
- 測試：`internal/industry/event_calendar_dates_test.go` 對 2021/2022/2023/2024/2026/2030/2040 断言違反清單為空，
  另有一條測試**故意**餵入 D1/D2 兩種壞 shape，證明守衛不是空轉。

## 3. D1：每個 occurrence 的 peak 必須屬於自己

`EventRule` 新增 `ComputePeakDateInMonth(year, month)`，`buildMonthlyEvent` 優先使用它；未提供時退回 `midMonth(year, month)`（每月 15 日）。
`ComputePeakDate(year)` 仍是「年度錨定」欄位，只供一年一個 occurrence 的規則使用。

| 規則 | `ComputePeakDateInMonth` | 語意 |
|---|---|---|
| `futures_settlement` | `thirdWednesday(year, month)` | 期貨結算日＝**該月**第三個週三 |
| `investor_conference` | `midMonth(year, month)` | 法說會旺季＝該季月月中（1/4/7/10 月各一次；7 月值仍為 7/15，與修正前一致） |

occurrence 的**窗口仍是「整個日曆月」**（`buildMonthlyEvent` 的既有語意，未改變），因此契約 §2 自動成立；
`ComputeStartDate` / `ComputeEndDate`（這兩個規則的年度錨定版本）不被 `buildMonthlyEvent` 使用，維持原樣以免改變
`DetectActiveEvents` 的結果。

## 4. D2：`position_building` 的窗口方向

`卡位行情` 的語意是「季底作帳（`window_dressing`）窗口**之前**的那一週」。`window_dressing` 的窗口是每月最後兩週
（`[lastTwoWeekStart, 月末]`），因此：

```
EndDate   = lastTwoWeekStart(year, month) - 1 天   （作帳窗口打開前一天）
StartDate = EndDate - 6 天                        （七天窗口）
PeakDate  = StartDate + 2 天
```

影響面（D2 讓原本「永不 active」的事件變成 active）：見 §6 的 2021 對照表；`position_building` 一年 4 個 occurrence
（3/6/9/12 月）現在都會在各自窗口內被 `DetectActiveEvents` 選中，因此
`GetEventAdjustment` / `GetCompositeEventSentiment` / `IsTaiwanTradingDay`（long_holiday 以外）的輸入集合變大，
`window_dressing` 之前的該週多了一個 bullish 事件參與加權。

## 5. D3：農曆不可判定 ≠ 猜一個日期

### 5.1 覆蓋範圍

`internal/taiwanholidays` 的農曆表（春節初一 / 清明 / 端午 / 中秋）驗證範圍**由 2023–2040 擴為 2021–2040**
（`CoverageYears()`），並補上原本缺漏的 **2023 春節初一 `01-22`**（2023 名義上在範圍內卻沒有條目，同樣走了 placeholder）。

新增年份的證據（2026-09-25）：

- **第一方市場資料**：`data/state/sector_index` 的 2021 session universe（密集、244 個交易日）。2021 年**平日無 session** 的日期為
  `01-01, 02-08, 02-09, 02-10..02-12, 02-15, 02-16, 03-01, 04-02, 04-05, 04-30, 06-14, 09-20, 09-21, 10-11, 12-31`
  ⇒ 春節初一 `02-12`、端午 `06-14`、中秋 `09-21`（清明 `04-04` 為週日，市場休市日為 `04-05` 補假）。
- **官方第一方文件**：DGPA 政府行政機關辦公日曆表（110/111/112 年）與中央氣象署日曆資料表（節氣表）逐項交叉：
  2021 `02-12 / 04-04 / 06-14 / 09-21`、2022 `02-01 / 04-05 / 06-03 / 09-10`、2023 `01-22 / 04-05 / 06-22 / 09-29`。
- **獨立計算**：lunardate／節氣天文計算（與 2031–2040 區塊相同方法）。

### 5.2 不可判定的行為（強制）

表外年份**不得**回傳慣例 placeholder。實作：

| 層 | 行為 |
|---|---|
| `taiwanholidays` | `lunarDatesInYear` 對表外年份**略過**該移動型假日並 `logging.Warn("taiwanholidays","lunar_unavailable",...)`；`HolidaysInYear` 只回固定日期假日；新增 `VerifiedLunarYear(year)` 供呼叫端判斷 |
| `industry` | `taiwanPublicHolidays` 的農曆 `Compute` 直接回傳 map 值（缺 key ⇒ zero time）；`buildHolidayEvent`/`buildSingleEvent` 回 `ok=false` 且不生成 occurrence，並 `logging.Warn("event_calendar","event_date_unavailable",...)` |
| 消費端 / 品質閘門 | `industry.LunarYearDeterminable(year)`、`industry.GetLunarCoverageYears()`；`cmd/experimental/jev-eval-events` 的 `unverified_lunar_calendar` 閘門維持保守（整個 `long_holiday` 型別排除，因為套件外無法區分 4 個農曆推導 + 4 個固定日期） |

語意：表外年份的 `spring_festival` 與 4 個農曆 `long_holiday` **沒有 occurrence**，這是「不可判定」而不是「沒有事件」；
需要完整事件母體的消費端必須自行比對覆蓋範圍（或呼叫 `LunarYearDeterminable`）。

### 5.3 為什麼選「擴表 + 不可判定」而非只做其中一個

- 只擴表：未涵蓋的年份仍會拿到 placeholder（本 issue 的根因），缺陷只是往後延。
- 只做不可判定：2021（評估最早年份）永遠拿不到春節事件，「春節前後行情完全沒被覆蓋」的問題原樣存在。
- 兩者並用：需要的年份有**可驗證**日期，其餘年份誠實回答「不可判定」。

## 6. 驗收證據（2021 視窗）

指令：`go run ./cmd/experimental/jev-eval-events -dir <sector_index> -start 2021-01-04 -end 2021-12-30 -anchor peak`（`-quality-gate=false` 看未過濾日期）。

| 指標 | 修正前 | 修正後 |
|---|---|---|
| `generated` | 61 | 61 |
| `exported` | 34 | 60 |
| `dropped(peak_outside)` | 14 | **0** |
| `dropped(inverted)` | 4 | **0** |
| `dropped(lunar)` | 8 | **0** |
| `futures_settlement` 匯出 | 1 | 12 |
| `position_building` 匯出 | 0 | 4 |
| `long_holiday` 匯出 | 0 | 8 |

日期對照（節錄）：`futures_settlement_2021_01` peak `2021-06-16 → 2021-01-20`；`investor_conference_2021_01` peak `2021-07-15 → 2021-01-15`；
`position_building_2021_06` `2021-06-24..2021-06-16 → 2021-06-10..2021-06-16`（peak `06-26 → 06-12`）；`spring_festival_2021` peak `02-01 → 02-12`（窗口 `01-27..02-11 → 02-07..02-22`）。

## 7. 已知未處理（誠實列表，皆非本 issue 範圍）

1. **兒童節／補假缺口**：`adjustedHolidays` 缺 2021/2022，且 2023 起 `04-04` 兒童節本身也不在表內（`2023-03-25` 補行上班、`2023-04-04` 市場休市）；
   `taiwanholidays.IsTradingDay` 對這些日期仍會判為交易日。屬「假日表」議題，會影響 2021–2023 replay，需獨立評估（有第一方 session 證據可查）。
2. **颱風臨時休市**（例：2023-08-03 卡努）不在年度開休市表內，`IsTradingDay` 無法判定。
3. **`springFestivalClosures` 缺 2021/2022 完整休市區間**（2021 `02-08..02-16`、2022 `01-27..02-04`）；事件日曆只用「初一」，不受影響。
4. **`long_holiday` 是混合型別**（4 農曆 + 4 固定），套件外無法區分，品質閘門只能整型別處理。
5. **`futures_settlement` 的 occurrence 窗口仍是整個日曆月**（非「結算日 ±2 天」）；改變會縮小 `DetectActiveEvents` 的命中範圍，需獨立決策。

## 8. 維護者注意

- 新增任何「一年多個 occurrence」的規則時，**必須**提供 `ComputePeakDateInMonth`，否則會退回月中（15 日）語意。
- 新增任何 `build*Event` helper 時，必須保持 §2 的不變式；`event_calendar_dates_test.go` 會檢查每個支援年份。
- 農曆表補年份時，**必須**同時補足四個表；`TestLunarTables_Completeness` 會對 `CoverageYears()` 內每個年份檢查完整性。
- 不要在表外年份「先給一個合理的日期」：placeholder 與真值在下游無法區分，這正是 #1973 的成因。
