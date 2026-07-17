# 官股行庫資金勢力代理（Government Force Proxy）

> **文件角色**：產品定位 §7「資金勢力分類學」中**官股**主體的代理方法論（manifest #E04）。
> **對齊**：[`docs/reference/product-positioning.md`](../reference/product-positioning.md) §7、§8。

---

## 1. 為什麼需要代理

台灣證交所不公布「官股行庫」整體買賣超。常被引用的「三大法人」只涵蓋**外資、投信、自營商**——**官股**並非證交所官方分類。

實務上所稱「官股進場」至少有兩個不同對象，必須分清：

| 對象 | 範圍 | 訊號性質 | 可觀測性 |
|------|------|----------|----------|
| **官股行庫** | 8 家公股行庫旗下券商（臺銀、土銀、合庫、兆豐、一銀、華南、彰銀、元大*）的自營 / 子公司買賣超 | 常態性、長期、方向性弱但事件性強 | 證交所每日分點資料可加總；無官方 OpenAPI |
| **國安基金** | 國家金融安定基金，僅於重大事件進場 | 極罕見（2008、2020、2022），單筆大額公告 | 公開新聞稿即可，不需資料通道 |

\* 元大為泛公股但常被排除；本文件採嚴格定義：8 家公股行庫旗下的券商（不含元大）。

## 2. 代理選項（由優至劣）

### 選項 A：券商分點加總（首選，v1 實作路徑）
- **資料來源**：證交所每日公布的各股分點進出 → 篩出官股行庫旗下券商分點 → 加總淨買賣超。
- **優點**：原始資料權威、可追溯到分點層級。
- **缺點**：每檔股票分點表為公開網頁（非 OpenAPI），需自建爬蟲或第三方加工；非即時，T+1 公布。
- **狀態**：本系統目前無對應通道（**backlog BK-13**）。

### 選項 B：第三方媒體整理（備援）
- **資料來源**：CMoney、Goodinfo、財訊等媒體每日匯整的「八大行庫買賣超」。
- **優點**：現成資料，省爬蟲。
- **缺點**：商業授權、口徑不一、無法逐筆驗證。
- **狀態**：未採購，列為評估項（**backlog BK-14**）。

### 選項 C：國安基金事件驅動（輔助）
- **資料來源**：國安基金委員會公開決議、新聞稿。
- **優點**：影響極大、訊號明確。
- **缺點**：極罕見，不能作為日常代理。
- **狀態**：本文件不額外建通道；由 `narrative` 的 `geopolitical_taiwan` 事件訊號覆蓋。

## 3. v1 實作策略（manifest #E04）

**現實**：無自動資料源 → 不能寫自動抓取。**v1 採誠實的「操作員匯入」模式**：

1. **資料流**：操作員把每日官股行庫買賣超（任何來源彙整皆可，但須註明 source）寫入
   ```
   data/state/government_flow/YYYYMMDD.json
   ```
   schema：
   ```json
   {
     "date": "20260716",
     "total_net": 1234567890,
     "source": "operator-imported",
     "raw_url": "https://example.com/breakdown"
   }
   ```
   - `total_net` 單位：新台幣元（正值 = 淨買超；負值 = 淨賣超）。
   - `source` 必填；值限定 `operator-imported | broker-aggregate | media-curated`。
   - 缺檔或解析失敗 → 視為「無資料」，force 保持 neutral + 標 `data_available: false`。

2. **通道**：`government_flow`（資料源是檔案，非外部 API）。
   - `internal/marketdata/government_flow_provider.go`：讀取最新日期的 state 檔，回傳 `GovernmentFlowReading{Date, TotalNet, Source}`。
   - `internal/apigateway/adapter_government_flow.go`：封裝成 channel adapter，channel id `government_flow`。
   - `cmd/atlas/capital_tasks.go`：每日收盤後排程 refresh（與 `auto_taifex_institutional` 對齊）。

3. **注入**：`MacroDataSnapshot` 新增 `GovernmentNet MacroDataPoint json:"government_net,omitempty"`；`macroDataGatewayAdapter.applyGovernment` 從 channel 讀入並設值。

4. **下游消費者（E05 範圍）**：`ForceExtractor.extractGovernment` 改讀 `snap.GovernmentNet.Value`，資料缺則 Trend 維持 neutral（不假裝）。

## 4. 校準哲學（§8 對齊）

任何「官股進場訊號 → 漲跌」的啟發式（例如「八大行庫連買 3 日是底部訊號」）**不得硬編碼為規則**，必須經過：

```
假設登錄 → 歷史資料校準（命中率 / 報酬統計）→ 寫回 parameters.json → 持續追蹤 → 退化自動降權
```

v1 階段資料源為操作員手動匯入，樣本量不足以做有意義的統計校準；**正式啟用前 force 必須標記為「資料不足」**，不得據此給出共振訊號。

## 5. 驗收與拒收條件

**通過**：
- `data/state/government_flow/20260716.json` 存在時，`GET /api/capital-flow/daily` 的 forces 列表 `government` 項的 `raw_value` = 該檔的 `total_net`，`trend` 由 60 日滾動 Z 分數決定。
- 缺檔時 force 項仍存在但 `raw_value=0`、`trend="neutral"`，且 API 標記 `data_available: false`。

**拒收**：
- 從分點資料以外用啟發式（如「投信動向 × 0.3 + 自營避險倉」）硬湊出官股代理 — 違反 §8。
- 在 < 30 個資料點的樣本下對外顯示「官股共振」訊號 — 違反 §8。

## 6. 後續工作

- **BK-13**：建立證交所分點加總通道（選項 A 自動化）。
- **BK-14**：評估購買第三方整理資料（選項 B）。
- **E05**：將 `GovernmentNet` 接進 `ForceExtractor` + 共振模型。