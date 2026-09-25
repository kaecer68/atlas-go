# Sim Engine 模擬引擎規格

> **文件角色**：atlas-go 投資組合模擬引擎規格。
> **取代對象**：原 internal/sim/AGENTS.md（已遷移至此）。

`internal/sim` 執行投資組合模擬引擎：給定報價與建議，執行委託單、計算成交價、維持部位狀態。

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **`RunWithState()` 就地變異狀態** | `state *domain.SimulationState` 引數會就地修改。多次透過相同狀態呼叫 `RunWithState` 會疊加影響。每次模擬前**必須**呼叫 `domain.NewSimulationState()`。參見根 `AGENTS.md`「高危陷阱」第二項。 |
| **無 SlippageModel = 固定 SlippageBPS** | 若不呼叫 `WithSlippageModel()`，引擎使用 `constraints.SlippageBPS`。此靜態值對高成交量股票過於寬鬆，對低流動性股票過於嚴格。 |
| **Nil TaxCalculator 靜默跳過稅務** | 若 `taxCalc` 為 nil，引擎記錄警告並跳過稅務計算。最終 `SimulationResult` 的 PnL 為未稅，`FallbackEvents` 附 `"tax: nil calculator, skipping"`。 |
| **股息資料耦合** | 稅務計算器需要 `dividends` map（透過 `WithDividends()` 設定）。若遺漏，稅務調整使用 0 股息，導致低估稅務責任。 |
| **反身性規則就地變異 recs** | `reflexivity.Rule.Apply(recs, *state, ...)` 修改 `recs` slice 內容。多次模擬之間**絕對不能**共用同一份 `recs` slice。 |
| **動態閾值減少部位** | `DynamicThresholdEngine` 對高度相關的訊號套用相關性過濾，可能移除被判斷為冗餘的建議，導致實際持倉少於預期。 |
| **買入執行順序（legacy 路徑）** | `executeLegacyBuys` 對建議排序：`Conviction` 降冪 → `Symbol` → `Agent` → `Reason`。此順序是確定性的；optimizer 路徑則由 optimizer 決定。 |
| **`DynamicThresholdEngine` 重複符號過濾** | 若 `thresholdEngine` 已設定，`RunDay` 會過濾重複符號的建議，保留信心度最高者。若預期同符號有不同方向的建議，信心度較低者會被跳過。 |
| **無 `MarketImpactModel` = 忽略大單衝擊** | 若不呼叫 `WithMarketImpactModel()`，大額訂單相對於 ADV 仍會以收盤價成交，導致回測績效過度樂觀。建議在流動性較差的標的上啟用。 |

---

## 核心執行流程

```
Engine.Run(regime, quotes, recs)
  → dayResult := Engine.RunDay(state, time, regime, quotes, recs)
    1. 套用反身性規則（就地變異 recs）
    2. 現有部位依市價評估
    3. 透過 thresholdEngine 過濾重複符號
    4. 依信心度與可用現金執行買入
    5. 依退出訊號執行賣出
    6. 計算成交價與滑價
    7. 套用稅務（若已設定）
  → SimulationResult { Orders, Trades, Positions, EndingCash, ... }
```

**關鍵**：`Run()` 建立新的 `SimulationState`；`RunWithState()` 使用現有狀態。多日期回測一律使用 `RunWithState`；單次執行使用 `Run`。

---

## daily_returns 序列契約（交易日語意，#1900 / #1935）

`domain.SimulationState.DailyReturns` / `EquityCurve` 是**每個交易日一筆**，不是每次執行一筆。這個語意由 `sim.Engine.RunDay` 持有，靠兩個欄位表達：

| 欄位 | 語意 |
|------|------|
| `last_session_date` | 最後一筆所屬的交易日（`YYYY-MM-DD`，`domain.SessionDateKey`；空字串 = 未知，見 `domain.SimulationState`） |
| `session_base_value` | 最後一筆日報酬的分母 = **前一交易日收盤**（不是同日上一次執行的收盤） |

`RunDay` 的三條分支：

- 本次執行交易日 == `last_session_date` → **取代**最後一筆（`daily_returns[n-1]`、`equity_curve[n-1]`），並以 `session_base_value` 重算報酬；`domain.SimulationResult.SessionRerun` = true。
- 不同交易日 → append，並更新 `last_session_date` 與 `session_base_value`（後者取 `previous_values["_portfolio_"]`）。
- 交易日未知（legacy 檔、或報價無交易日）→ 維持 append（保留歷史、不 panic），下一次有日期的執行才建立語意。

### writer 清單與義務

| writer | 位置 |
|--------|------|
| `auto_daily_simulation` | orchestrator 每日模擬 |
| `stress_test_daily` | orchestrator 壓力測試 |
| `POST /admin/trigger-simulation` | orchestrator 手動觸發 |
| `cmd/backfill-var-returns` | 運維 CLI：由 `data/state/sessions/session-<YYYYMMDD>-<mode>/summary.json` 重建序列 |

**新 writer 的義務**：寫入 `daily_returns` 時必須同時維護 `last_session_date` 與 `session_base_value`，並以前一交易日收盤當分母。只覆寫 `daily_returns` 會讓檔案退回「無日期語意」，之後的同日執行就會 append 出同日重複（#1935 的實例：同日零報酬把 `var95` 拉成 0）。

### `cmd/backfill-var-returns` 的重建規則（#1935）

- **依交易日去重**：同一交易日可能有多個 session 目錄（`session-<YYYYMMDD>-<replay mode>`），只留一個。預設 `-on-duplicate=last`（`recorded_at` 最新者；無 `recorded_at` 者視為當日最舊、以目錄名決勝），可用 `-on-duplicate=first` 取最早者。
- **寫入日期語意**：`last_session_date` = 重建序列的最後一個交易日，`session_base_value` = 前一個交易日的收盤；與 `RunDay` 相同。重建後引擎再跑同一交易日時 `daily_returns` 長度不變（取代而非 append）。
- 只覆寫 `daily_returns`；`equity_curve` 與 `previous_values["_portfolio_"]` 不動（不在 issue 範圍，且可能截斷檔案仍持有的歷史）。
- **不**回溯清理舊格式已寫入的同日零報酬（已無法歸屬交易日，issue 明示刻意不動）。
- 相異交易日少於 2 → 報錯並**不改檔**（重建需要前一交易日收盤）。
- 參數格式：`backfill-var-returns [-sessions-dir DIR] [-state-file FILE] [-on-duplicate last|first] [-dry-run]`，或舊的位置參數形式 `<sessions-dir> <state-file>`。

驗證：`go test ./cmd/backfill-var-returns/ -v -run 'TestEngineRerunAfterBackfill|TestRiskSnapshot'`。
