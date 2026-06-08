# AI 觀測台審計修復狀態追蹤

> 審計日期：2026-06-08
> 最後更新：2026-06-08

---

## 已修復（PR #437 已合併）

| 優先級 | 問題 | 修復內容 |
|--------|------|----------|
| **P0** | SharpeLike 公式錯誤 | `mean/variance` → `mean/std_dev`（樣本標準差 n-1） |
| **P0** | 缺失統計顯著性 | 新增 T-statistic、HitRate T-stat、95% CI、StatisticallySignificant |
| **P1** | UI 字段不完整 | 擴展表格：層級、觀察數、平均報酬、95% CI、統計警告 |
| **P2** | HTML class 重複 | 合併為 `badge info cursor-help` |
| **P2** | 參數不匹配 | 移除 `window.darwinianTrend` 多餘傳參 |
| **P2** | BrokerRuntime 洩露 | 從 API 響應中移除 |
| **P2** | NaN/Inf 風險 | 添加數值消毒 |
| **P3** | 核心算法零測試 | 5 組新單元測試 |
| **預存** | Lint 問題 | 3 個拼寫/錯誤處理 + 1 個浮點數精度測試 |

---

## 未修復（需架構層面決策）

### 1. Darwinian 權重閉環 — 部分實作，缺少觀測台整合

**現狀分析：**

系統 **已有** 完整的 Darwinian 權重管理實現：

```
internal/portfolio/darwinian_weights.go
  └── DarwinianWeightManager (完整的權重管理器)
      ├── RecordOutcome()       — 記錄推薦結果
      ├── PerformDailyAdjustment() — 每日權重調整
      ├── calculateSharpe()     — 正確的 Sharpe 計算（mean/std_dev）
      ├── AppendSnapshot()      — 歷史記錄持久化
      ├── LoadHistory()         — 歷史記錄加載
      └── GetAllWeights()       — 獲取所有權重
```

**問題：**

| 問題 | 說明 |
|------|------|
| 觀測台與 Darwinian 系統脫節 | 觀測台讀取 `Scorecard`（來自 `ledger.BuildScorecards`），Darwinian 系統獨立維護 `DarwinianAgentWeight`。兩個數據源沒有統一。 |
| 觀測台不顯示當前權重 | 用戶無法在觀測台看到 Agent 的當前 Darwinian 權重值。 |
| 無權重調整歷史趨勢 | `LoadHistory()` 可讀取歷史，但觀測台沒有顯示權重變化趨勢圖。 |
| 無自動調整觸發指示 | `PerformDailyAdjustment()` 是否執行、何時執行，對用戶不可見。 |

**建議修復方案：**

1. **短期**：在觀測台 API 響應中添加 `darwinian_weight` 字段，從 `DarwinianWeightManager.GetAllWeights()` 讀取
2. **中期**：添加權重歷史趨勢 API 和前端折線圖
3. **長期**：統一數據源 —— 讓 `BuildScorecards` 直接讀取 `DarwinianWeightManager` 的數據，而非獨立計算

---

### 2. Regime-Conditional 績效分析 — 已實作，但未暴露給觀測台

**現狀分析：**

系統 **已有** regime 分組績效計算：

```
internal/reporting/performance.go:561
  └── calculateRegimeBreakdown(summaries, outcomes)
      └── RegimeBreakdown{Regimes: map[string]RegimePerformance}
          └── RegimePerformance{Regime, TotalReturn, AvgReturn, WinRate, SessionCount}
```

**問題：**

| 問題 | 說明 |
|------|------|
| 觀測台不顯示 regime 分組 | 用戶只能看到「全期間」績效，無法了解 Agent 在 risk-on vs risk-off 的差異。 |
| 無 regime 適應性指標 | 無法判斷 Agent 是否只在特定 regime 有效（如僅在牛市有效）。 |
| `calculateRegimeBreakdown` 僅用於 reporting | 該函數的結果只出現在 `PerformanceReport` 中，觀測台未引用。 |

**建議修復方案：**

1. **短期**：在觀測台 API 中添加 regime breakdown 字段
2. **中期**：前端添加 regime 分組表格（類似現有的績效表，但按 regime 分組）
3. **長期**：添加 regime 穩定性指標（不同 regime 下 Sharpe 的標準差，越小越穩定）

---

### 3. Out-of-Sample 驗證 — 完全缺失

**現狀分析：**

系統 **沒有**針對 Agent 績效的 OOS 驗證框架：

```
internal/robustness/ablation.go
  └── AblationAnalysis() — 因子消融分析（R2OOS）
      ⚠️ 這是針對因子/特徵的穩健性檢驗，不是針對 Agent 績效的
```

**問題：**

| 問題 | 說明 |
|------|------|
| 無 walk-forward 框架 | 所有績效指標均基於 in-sample 數據。無法檢測過擬合。 |
| 無滾動窗口驗證 | 無法觀察 Agent 績效是否隨時間衰減。 |
| 無訓練/測試分離 | Darwinian 選擇和淘汰基於與訓練相同的數據，存在 bias。 |
| 觀測台無 OOS 指標 | 用戶無法區分「真實預測能力」與「過擬合」。 |

**建議修復方案（高複雜度，需架構設計）：**

1. **設計 OOS 框架**：
   - 定義「訓練窗口」（如過去 60 天）和「測試窗口」（如最近 20 天）
   - 在 `BuildScorecards` 中計算兩套指標：In-Sample Sharpe 和 OOS Sharpe

2. **添加過擬合檢測**：
   - 當 IS Sharpe > 1.0 但 OOS Sharpe < 0 時，標記「過擬合警告」
   - 計算 IS/OOS Sharpe 比率（ratio > 2 視為過擬合信號）

3. **添加績效衰減檢測**：
   - 滾動計算 Sharpe（如 20 日窗口）
   - 檢測 Sharpe 趨勢（線性回歸斜率為負 = 衰減）

4. **觀測台顯示**：
   - IS Sharpe | OOS Sharpe | 過擬合警告 | 衰減趨勢

---

## 代碼引用索引

| 功能 | 文件 | 行號 |
|------|------|------|
| DarwinianWeightManager | `internal/portfolio/darwinian_weights.go` | 99-700+ |
| RecordOutcome | `internal/portfolio/darwinian_weights.go` | 265 |
| PerformDailyAdjustment | `internal/portfolio/darwinian_weights.go` | 372 |
| calculateSharpe (正確) | `internal/portfolio/darwinian_weights.go` | 338 |
| LoadHistory | `internal/portfolio/darwinian_weights.go` | 187 |
| RegimeBreakdown | `internal/reporting/performance.go` | 30-36 |
| calculateRegimeBreakdown | `internal/reporting/performance.go` | 561 |
| AblationAnalysis (R2OOS) | `internal/robustness/ablation.go` | 36 |

---

## 優先級建議

| 優先級 | 項目 | 預估工作量 | 影響 |
|--------|------|-----------|------|
| **P2** | Darwinian 權重顯示 | 1-2 天 | 用戶可見現有功能 |
| **P2** | Regime-Conditional 顯示 | 1-2 天 | 用戶可見現有功能 |
| **P3** | OOS 驗證框架 | 1-2 週 | 需架構設計，高價值 |

---

> 本報告基於 PR #437 合併後的代碼狀態生成。
