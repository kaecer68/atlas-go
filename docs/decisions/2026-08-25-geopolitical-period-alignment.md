# 決策紀錄：時期判別引入地緣（台海）條件與 G5 事件捕捉（2026-08-25）

> 狀態：Shipped（PR #1692 + 本 PR）
> 關聯：hermes 回報 current_period 口徑差異 → 溯源 R6-R9 → 憲章對齊
> 詳細調查：`.omo/audit/2026-08-25-period-detector-constitution-mismatch.md`（時效性，不進 git）

## 背景

hermes 回報 atlas v8.227 回傳 `current_period=turnaround_down`，但其引用「憲章 v1.0 §3
第 6 條 6 指標」（台海緊張<4、DXY 微升、融資 5452~5481 區間穩態、ETF bit-equal）判讀偏
盤整/低迷。調查確認：

1. **憲章 SSoT = `docs/ATLAS_METHODOLOGY.md`**（ATLAS 方法論憲章 v1.0，審計源）。
   hermes 引用的 6 指標不在憲章 §3 任何時期定義內 → 屬延伸口徑。
2. **hermes「台海緊張 4.29」實為壓力指數 geopolitical 元件值**（intensity × weight 0.13），
   非 atlas GeoIntensity 原始刻度 → 口徑混淆根源。
3. **atlas 實作當時不含任何地緣（台海）判別條件** → current_period 判定完全不受台海危機影響。

## 決策

1. **GeoIntensity 引入時期判別**（PR #1692）：`PeriodIndicators.GeoIntensity`（0-100，
   `TaiwanRSSGeopoliticalProvider` 產出，與壓力指數 geopolitical 元件同源）；黑天鵝加條件
   ≥ 60（4 級制 ≥ 高張(3)）、轉折下壓加條件 ≥ 40（4 級制 ≥ 升溫(2)）；閾值參數化於
   `internal/config/period_detection_config.go`。
2. **台海緊張 4 級制**：0-25 平靜(1) / 26-50 升溫(2) / 51-75 高張(3) / 76-100 危機(4)，
   寫入憲章 §3；hermes 端引用統一以此為準。
3. **判別聚合規則補文件**：黑天鵝 ≥1/6、轉折下壓 ≥3/6、低迷/上升/高原 ≥3/5、轉折開高 ≥2/5、
   盤整 ≥3/4；零值輸入 = 資料不可用自動跳過（寫入憲章 §3）。
4. **§3 ↔ §5 角色對位**：§3 判別條件 = 權威（machine-executable）；§5 六個核心觀測指標 =
   寬鬆觀測（非判別）；延伸指標不得覆寫 §3。
5. **A1（國安基金接 R8）**：`NationalFundActive` 由
   `NationalStabilizationProvider.IsInterventionActive(date)` 注入（static 表 9 次護盤
   2000-2026）；黑天鵝條件 4 由死碼轉為生效。
6. **G5-4（事件層 trace）**：geo RSS 命中事件（title/keyword/source）寫入
   `geopolitical_history.sources_json`（新格式 `{"feeds":[...],"events":[...]}`，
   舊格式相容）；provider 每 feed 保留至多 20 筆去重事件。

## 影響

- 實作：`internal/portfolio/period_detector.go`、`internal/config/period_detection_config.go`、
  `internal/monitoring/dashboard_api.go`、`internal/narrative/geopolitical/`、
  `internal/ledger/{historical_store,postgres_historical}.go`
- 對外：current_period 現在會受地緣強度與國安基金護盤影響（黑天鵝/轉折下壓）；v8.227 前
  舊判別不含地緣屬預期。
- 相容性：`sources_json` 新格式向後相容（loader 可讀舊 plain array）；executor session 路徑
  仍為 12 欄位 degraded（GeoIntensity/NationalFundActive 未接，period_history 為權威）。

## 待辦（未含於本決策）

- hermes／atlas-wiki 端 6 指標引用統一（director-atlas-wiki 範圍，另行派工）
- 「地緣 5 日趨勢上升」強化（需 geopolitical_history 長序列）
- 壓力指數元件 MCP 工具公開（憲章審計 M5）
