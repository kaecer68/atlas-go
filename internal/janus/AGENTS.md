# AGENTS.md — internal/janus

本目錄定義了系統的 meta-layer 決策機制「JANUS」，負責跨 cohort 的市場盤勢偵測（Regime Detection）與權重動態調整。

---

## 核心職責

JANUS 位於 `internal/prism` 之上，透過監控不同 PRISM regime cohort 的表現，計算出最符合當下市場環境的權重分佈，並產出突現（emergent）的盤勢信號。

- **跨 Cohort 績效追蹤**：記錄各 regime（Risk-On, Risk-Off, Low-Volatility, Transition）的 Sharpe Ratio、命中率與報酬。
- **動態權重計算**：混合短期（5d）、中期（20d）與長期（60d）表現，產出各 cohort 的配置權重（預設範圍 `[0.05, 0.60]`）。
- **盤勢分類偵測**：
    - `NOVEL_REGIME`：短期表現最優的 cohort 顯著超越其歷史地位（新趨勢成形）。
    - `HISTORICAL_REGIME`：歷史強勢 cohort 持續主導（穩定趨勢）。
    - `MIXED`：無明顯主導群體或權重分佈平均。
- **信念調整**：透過 `ApplyAdjustment` 將 JANUS 權重套用至 `domain.Recommendation`，修正推薦標的的 `Conviction`。

---

## 關鍵概念

- **Rolling Aggregation**：`CohortPerformanceTracker` 維持 90 天滾動歷史，確保指標計算的即時性。
- **Blended Score**：權重計算採用 50% 短期 + 30% 中期 + 20% 長期權重，在靈敏度與穩定度間取得平衡。
- **Domain Mapping**：JANUS 將系統 `domain.Regime` 映射至對應的 `prism.RegimeType` cohort 以決定縮放倍率。

---

## 重要陷阱（ANTI-PATTERNS）

- **權重靜默正規化**：`CohortWeightCalculator` 會在計算後自動正規化使權重總和為 1.0，勿假設原始 Sharpe 分數直接對應權重。
- **負 Sharpe 處理**：當所有 cohort Sharpe 皆為負值時，系統會回退至平均分配（Equal Weighting）。
- **遺漏初始化**：使用 `Engine` 前應呼叫 `EnsureAllRegimes()` 以避免在資料尚未流入前出現權重空缺。
- **Conviction 夾制**：調整後的 `Conviction` 一律夾制在 `[0, 100]`，超過此範圍會被靜默修正。
- **RecordedAt 誤區**：snapshot 的 `RecordedAt` 僅代表記錄時間，JANUS 的決策排序應以 tracker 內的隊列順序為準。
