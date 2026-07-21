# Screener Screening Pipeline Contract

> **文件角色**：atlas-go screener 篩選管線契約。
> **取代對象**：原 internal/screener/AGENTS.md（已遷移至此）。

`internal/screener` 在標的進入各 executor 之前，根據 `configs/agents.json` 中 per-agent 的 `ScreeningCriteria` 進行篩選。

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **永不回傳錯誤 — 失敗在 Result 中** | `ScreenDetailed()` 合約為「永不回傳非 nil 錯誤」。所有篩選失敗透過 `ScreenResult.Passed=false` + `Reason` 傳達。不要為了失敗而檢查 `err`。 |
| **零值 P/E 或 P/B 篩選掉所有標的** | 若 `ScreeningCriteria.PE` 或 `PB` 為 `0`，預設比較器 `>` 使用 `CriteriaValue=0`。所有 PE 值 >0，這會讓**所有**標的**通過**（`0` 表示「無篩選」）。在依賴篩選前請確認預期行為。詳見下文「篩選流程」。 |
| **Nil FactorEngine/FundamentalProvider = panic** | `Engine` 建構函式接收指標。若其中之一為 nil，`ScreenDetailed` 會在存取 `e.fundamentals` 或 `e.factorEngine` 時發生 panic。 |
| **第一次失敗即停止** | `ScreenDetailed` 依序評估條件（成交量→PE→PB→股息率→動能→分數），在**第一次**失敗時回傳。不累積失敗，無 `AllFailures` 摘要。 |
| **Context 已傳遞但未用於取消** | `ctx` 傳遞給所有方法，但底層篩選未監聽 `ctx.Done()`。長時間篩選無法透過 context 取消。 |
| **`ScreenUniverse` 重複建立 Engine** | `ScreenUniverse` 對每個符號在迴圈中呼叫 `ScreenDetailed`。對大型 universe（200+ 符號），建議快取 FactorEngine 結果或批次處理。 |

---

## 篩選流程

```
ScreenDetailed(ctx, symbol, criteria, quotes)
  1. 檢查成交量門檻（若 criteria.Volume 已設定且非零）
  2. 檢查 P/E 範圍（若 criteria.PE 已設定且非零）
  3. 檢查 P/B 範圍（若 criteria.PB 已設定且非零）
  4. 檢查股息率門檻（若 criteria.DividendYield 已設定且非零）
  5. 檢查動能分數（若 criteria.MomentumScore 已設定）
  6. 檢查 FactorEngine 總分（若 criteria.TotalScore 已設定）
  → ScreenResult { Passed, Reason, Criterion, Label, Threshold, Actual }
```

**重要**：步驟 2-4 的「非零」檢查為關鍵。`PE=0` 表示「跳過 PE 篩選」。請勿設 `PE: -1` 表示「無篩選」— 它會通過 `!= 0` 檢查並與 `> -1` 比較，導致全部通過。若要讓欄位為空，請使用 `0`。
