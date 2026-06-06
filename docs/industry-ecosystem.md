# 產業生態系（Industry Ecosystem）

前端頁面「產業生態系」包含三個核心板塊，各自對應完整的後端計算鏈。

## 供應鏈連動（Supply Chain Linkage）

- **核心檔案**：`internal/industry/linkage.go`、`configs/supply_chain_graph.json`
- **圖譜定義**：`configs/supply_chain_graph.json` 定義節點關係（upstream/downstream/supplier），可在不重新編譯下修改。
- **圖譜載入**：`LoadSupplyChainGraph()` 從 JSON 載入後同時填入 `SupplyChainGraph` 與 `CorrelationMatrix`。
- **相關矩陣**：`CorrelationMatrix` 支援三種初始化方式：
  1. `DefaultCorrelationMatrix()` — 硬編碼預設值（回退方案）
  2. `LoadCorrelationMatrixFromConfig()` — 從 `configs/parameters.json` 的 `industry.linkage_params.correlation_matrix` 讀取
  3. `RecalculateFromReturns()` — 從產業報酬率時間序列實證計算
- **敘事感知調整**：`NarrativeLinkageProvider` 介面允許宏觀敘事主題動態調整產業間相關係數。
- **衝擊傳導**：`PropagateShock()` 計算衝擊從來源產業向下游與上游的傳導。
- **系統重要性**：`CalculateLinkageScore()` 基於產業在圖中的連線數計算。
- **實證校準**：`cmd/calibrate-seasonal` 支援 `--replay` 旗標。

## 季節性模式（Seasonal Patterns）

- **核心檔案**：`internal/industry/seasonality.go`、`internal/industry/seasonal_calibrator.go`
- **校準管道**：`cmd/calibrate-seasonal` CLI 支援合成數據或實際歷史回測數據。
- **API**：`/api/industry/seasonality`、`/api/industry/seasonality/calendar`
- **決策鏈透明化**：`GetAdjustmentBreakdown()` 提供四層調整分解（季節性 × 敘事 × 循環 × 環境）。

## 週期羅盤（Cycle Compass）

- **核心檔案**：`internal/industry/cycle.go`、`internal/industry/dynamic_env.go`
- **商業週期偵測**：`CycleTracker` 管理五種產業階段（expansion/recovery/mature/recession）。
- **API**：`/api/industry/cycles` 回傳各產業的週期位置與趨勢。
