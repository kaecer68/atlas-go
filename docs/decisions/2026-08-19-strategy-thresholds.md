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


---

## k3 審計追蹤（2026-08-20，`kimi-coding/k3`）

> 追蹤結果（審計 repo main + iMac prod，image build 2026-08-20 04:24Z，HEAD c0705f97 含 #1619）：

| 項 | 狀態 | 核對 |
|----|------|------|
| 1b cb-fx 32.3 | ✅ **CLOSE** | main 與 iMac seeds 均 32.3；第二條件 USD_TWD>0.5 保留；registry_test 有鎖定測試 |
| 1a dealer-domestic | ✅ 實作面 CLOSE | 兩機 seeds 均 30/20/-50，未誤改（維持 C）|
| 2 stale monitor | ✅ **CLOSE** | #1619 TimeGated 旗標（daily_report_generate/autobacktest_daily/tej_refresh）+ GatedStaleWindow 72h；部署後 24h 無誤報 |
| 1c 通道有來源 | ✅ CLOSE | — |

> **唯一 rest（1a 尾巴）**：決策文採「維持但不 close、待數據」——現況無追蹤註記，會變孤兒。
> **再評觸發條件（追蹤鍵，K3 建議）**：待 macro snapshot 歷史涵蓋極端行情（現況 dom max 3.13 億 vs 閾值 30 億，需股災級土洋對作才可能觸發），**或 3–6 個月後回顧**；屆時若仍 0 觸發且無歷史極端日佐證，可考慮降閾值或標 deprecated。

## 低度殘留風險（不需現在動，記錄以防再發）
- 未來新增 time-gated task 若漏標 `TimeGated` → stale 誤報會再現（新增 task 時檢查）。
- gated task 永遠 skip（非 fail）時 `lastSuccess` 不更新 → 屬輕微盲區。
