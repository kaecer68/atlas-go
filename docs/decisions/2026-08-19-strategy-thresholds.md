# 決策：strategies #9/#10 sample=0 閾值 + scheduler 逾期判定 (2026-08-19)

> 狀態：業主已拍板（2026-08-19）
> 由 parent 自主檢查 T86 資料源後，1c 確認通道具來源，不需另查資料源完整性。

## 背景
`/client/strategies` 中 `dealer-domestic-support`(#10) 與 `cb-fx-intervention-warning`(#9)
sample_days=0、annual/sharpe/maxDD 顯示 `--`。已確認為策略條件閾值 vs 資料實際量級不匹配。

## 1a. dealer-domestic-support（土洋對作護盤）：採 **C — 維持閾值（稀有訊號）**
- 條件：`domestic_fund_net>30 且 dealer_net>20 且 foreign_investor_net<-50`（億）
- 資料（macro snapshot，TWSE T86 加總全市場，`÷1e8`→億）：投信 -3.4~+3.1、自營 -32~5.3、外資 -14.7~+21.8
- 純正負對作（dom>0 & dir>0 & fx<0）108 天僅 3 天 → 極端稀有
- **決策**：不改閾值，視為稀有極端訊號；0 樣本就是沒觸發。前端維持 `--`（合理）。
- 不採 A/B（降閾值樣本極少且改變策略原意）。

## 1b. cb-fx-intervention-warning（央行匯市干預）：閾值 **32.5 → 32.3**
- 條件：`usd_twd>32.5`；資料（macro snapshot）歷史最高 32.396，>32.5 永遠 0。
- **決策**：`data/seeds/strategy_techniques.json` 閾值 32.5 改 32.3 → 歷史前 1%，可產生 ~13 天樣本，仍具「高位警示」意義。
- 第二條件 `USD_TWD>0.5`（疑似日升幅判斷）保留，實作時確認語意。

## 1c. capital_flow 資料源：**通道具來源，不需另查**
- `twse_capital_flow` 通道抓 TWSE 官方 T86（`/rwd/zh/fund/T86?response=json&date=...`），每檔資料加總全市場 `÷1e8`→億。
- 實測 2026-08-13：外資 5.317 億、投信 -0.116 億、自營 5.32 億 — 與 snapshot 一致，為真實市況（非縮水/非無來源）。
- **結論**：通道正常、資料正確；量級小是市況，非 bug。**不需另查資料源完整性**。

## 2. scheduler 逾期（stale_count=2）：監控誤報（待實作修正）
- `daily_report_generate` 僅台北 14:00 執行（`now.Hour()!=14 → ErrTaskSkipped`）；昨天 14:00 band 已正常跑。
- `autobacktest_daily` 亦 window-gated（`ErrNotInWindow → ErrTaskSkipped`）。
- stale monitor 用「3x interval 沒跑」判定，未考量 time-gate → 誤報。
- **決策**：修正 stale monitor（對 ErrTaskSkipped / time-gated task 排除或調整 threshold）。task 本身正常。

## 待實作（派工，連測試進 PR）
1. `data/seeds/strategy_techniques.json`：cb-fx 閾值 32.5→32.3（+ 確認第二條件語意）
2. stale monitor：time-gated task 移除 stale 誤報
3. dealer-domestic：不改（維持）
4. frontend `--`：維持現狀（稀有訊號合理展示）

## 參考
- `internal/marketdata/twse_capital_flow_provider.go`（T86 /1e8）
- `cmd/atlas/main.go` L1330 (daily_report_generate) / L2154 (autobacktest_daily)
- stale monitor：`internal/monitoring` background_task stale 判定
