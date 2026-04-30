# SPEC.md — FinMind 日內 (Intraday) 行情整合

**日期**: 2026-04-30
**狀態**: 調研完成，發現重大限制
**範圍**: FinMind API v4 日內資料集評估與整合規劃

---

## 1. 研究發現摘要

### 1.1 可用的日內資料集

| 資料集 | 內容 | 訂閱層級 | 備註 |
|--------|------|----------|------|
| `TaiwanStockPriceTick` | 個股逐筆成交 | **Premium** | 付費牆 |
| `TaiwanStockKBar` | K 棒（分鐘/日） | **Premium** | 付費牆 |
| `TaiwanVariousIndicators5Seconds` | **大盤**加權指數 5 秒資料 | **Free** | 可用，僅需 `start_date`，無需 `data_id` |
| `TaiwanStockEvery5SecondsIndex` | 大盤 5 秒指數 | **Premium** | 付費牆 |
| `TaiwanStockStatisticsOfOrderBookAndTrade` | 委託成交統計 | **Free** | 可用，無需 `data_id` |

### 1.2 關鍵限制

```
重大發現：
- 個股層級的日內資料（Tick、KBar、分鐘K）全部為 Premium 專屬
- Free tier 僅提供「大盤指數」層級的 5 秒資料（無個股）
- 目前 API 回應：「Your level is free. Please update your user level」
```

### 1.3 Rate Limit

- Free tier: **600 req/hour**（推測，尚未觸發）
- Premium 等級差異：https://finmindtrade.com/analysis/#/Sponsor/sponsor

---

## 2. 免費可用的大盤 5 秒資料

### 測試成功：`TaiwanVariousIndicators5Seconds`

```bash
curl -s -H "Authorization: Bearer $FINMIND_API_KEY" \
  "https://api.finmindtrade.com/api/v4/data?dataset=TaiwanVariousIndicators5Seconds&start_date=2026-04-29&end_date=2026-04-29"
```

**回應格式**:
```json
{
  "msg": "success",
  "status": 200,
  "data": [
    {"date": "2026-04-29 09:00:00", "TAIEX": 39521.73},
    {"date": "2026-04-29 09:00:05", "TAIEX": 39081.34},
    ...
  ]
}
```

**限制**：
- 資料為 TAIEX（大盤加權指數），非個股
- 一次只能查詢**一天**（`end_date` 參數無效）
- 欄位僅有 `date` + `TAIEX`（無成交量、無個股）

---

## 3. 架構影響評估

### 3.1 對現有系統的價值

| 功能 | 目前狀態 | Intraday 價值 |
|------|----------|---------------|
| 大盤時序特徵 | 僅日收盤 | **可用 5 秒大盤**建構日內動能特徵 |
| 超級投資人進出判斷 | 依賴日資料 | 可輔助判斷當日操作意圖 |
| 敘事事件觸發 | T+1 日才能確認 | 可在建構當日宏觀事件時使用 |

### 3.2 無法實現的功能

| 功能 | 原因 |
|------|------|
| 個股日內策略 | 個股 Tick/KBar 為 Premium |
| 日內價量進出點 | 需要個股 Level 2 資料 |
| 分鐘級別選股 | 需要付費訂閱 |

---

## 4. 建議行動方案

### 方案 A：僅整合大盤 5 秒資料（低成本，實現簡單）

**實作**:
- 新增 `internal/marketdata/finmind_intraday.go`
- 實作 `FetchTaiwan5SecIndex(date string) ([]IndexBar, error)`
- 將 5 秒大盤資料存入 replay 目錄 `data/replay/taiwan_5sec_index.jsonl`
- 為 `FactorEngine` 或 `NarrativeIngestor` 提供大盤時序特徵

**工時預估**: 1-2 小時

### 方案 B：等待 Premium 訂閱（長期規劃）

- 聯絡 FinMind 取得報價
- 評估 `TaiwanStockPriceTick` vs `TaiwanStockKBar` 的實際欄位
- 若有足夠價值，申請公司/個人 Premium 訂閱

---

## 5. 驗證結果

```bash
# Free tier 可用：TaiwanVariousIndicators5Seconds
✅ 回應正確的 5 秒 TAIEX 資料

# Premium only（均失敗）
❌ TaiwanStockPriceTick → "Your level is free. Please update your user level"
❌ TaiwanStockKBar → "Your level is free. Please update your user level"
❌ TaiwanStockEvery5SecondsIndex → "Your level is register"
```

---

## 6. 輸出產物

- `SPEC.md`（本文件）
- `docs/superpowers/plans/2026-04-30-finmind-intraday-plan.md`（實作計畫）
