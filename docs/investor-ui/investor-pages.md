# Atlas 投資人 UI — 頁面結構與 Wireframe

**版本**: 1.0  
**日期**: 2026-06-02  
**成熟度**: X（experimental）  
**父技能**: `atlas-investor-ui`

---

## 一、描述

本技能規範 Atlas 投資人 UI 的**頁面結構、wireframe、每個頁面的資料來源與 API 需求**。共 6 個頁面，遵循父技能的設計原則（投資人語言、一頁一答案、基準對比、圖表優先）。

---

## 二、頁面 0：投資人儀表板（唯一入口頁）

**目的**：回答三個問題——現在市場怎麼樣？我應該關注什麼？系統最近表現如何？

```
┌─────────────────────────────────────────────────┐
│  Atlas 每日投資摘要              2026-05-26     │
├─────────────────────────────────────────────────┤
│                                                 │
│  📊 市場狀態：⚠️ 謹慎（風險等級：黃）              │
│  建議曝險比例：70%    |   現金保留：30%            │
│                                                 │
│  ── 今日推薦關注 ────────────────────────────     │
│  ┌──────────┬──────┬──────┬──────────────────┐  │
│  │ 2330.TW  │ 買入 │ 82%  │ AI需求+低PE       │  │
│  │ 0050.TW  │ 持有 │ 75%  │ 大盤穩健           │  │
│  │ 00635U.TW│ 買入 │ 68%  │ 避險需求           │  │
│  └──────────┴──────┴──────┴──────────────────┘  │
│                                                 │
│  ── 近 30 天模擬績效 ────────────────────────    │
│  組合報酬：+3.2%（TAIEX：+1.8%）                  │
│  [累積報酬曲線 vs 0050 vs TAIEX]                  │
│  Sharpe：1.42  |  勝率：68%  |  最大回撤：-2.1%  │
│                                                 │
│  ── 今日重要事件 ────────────────────────────    │
│  • AI 資本支出擴張（命中率 81%）                   │
│  • 外資連續 3 日買超                              │
│  • Nasdaq 昨收 +1.2%，費半 +2.1%                  │
│                                                 │
│  ── 信任分數 ───────────────────────────────    │
│  [儀表板刻度：78/100]                             │
│                                                 │
│  [查看詳細績效] [查看推薦詳情] [查看市場洞察]       │
└─────────────────────────────────────────────────┘
```

**API 端點**：`/api/client/summary`  
**無 tabs，不折疊**。

---

## 三、頁面 1：績效分析

**目的**：驗證 Atlas 的歷史表現（信任金字塔第 4 層基石）。

**核心圖表**：
- 累積報酬曲線（Atlas 組合 vs TAIEX vs 0050 vs 台灣 50）
- Sharpe 比率時間序列（30 日滾動）
- 最大回撤時間序列
- 月報酬熱力圖
- 推薦命中率時間序列（按 agent、按 sector、按 regime）

**API 端點**：`/api/client/performance?days=N`、`/api/client/benchmark`  
**資料來源**：`ledger/` outcomes、`internal/experiment/` ForwardReturn、`marketdata/` 基準價格

---

## 四、頁面 2：推薦詳情

**目的**：讓投資人理解「為什麼推薦這檔股票」。

**每檔推薦展開後顯示**：
- 綜合評分（分項：動能 / 價值 / 品質 / 波動 / 成長 / 情緒）
- 一句話摘要（NLG 生成，見 `atlas-investor-nlg`）
- 詳細投資邏輯（NLG 3-5 句說明）
- 因子分數長條圖（可視化）
- 信心度分解（ConvictionBreakdown steps）
- 相關宏觀事件（含歷史命中率）
- 歷史類似情境表現（見 `atlas-investor-roadmap` §歷史情境匹配層）

**NLG 輸出範例**：
> 「台積電（2330.TW）綜合評分 82/100。主要優勢：技術面動能強勁（20 日漲幅 5.2%）、估值具吸引力（本益比 15.3 vs 5 年平均 18.7）。主要風險：若 AI 資本支出降溫可能回調。過去 5 次類似環境下，10 日後均報酬 +2.1%。」

**API 端點**：`/api/client/recommendations?symbol=X`（含完整 breakdown）

---

## 五、頁面 3：市場洞察

**三個子區塊**：

1. **宏觀環境**：當前 regime、JANUS 跨 cohort 對比、重要 narrative events、全球市場連動
2. **產業輪動**：簡化版產業生態系（供應鏈連動、季節性模式、週期羅盤）
3. **散戶情緒**：融資融券變化、RSI-tw 零售情緒指數、外資買賣超趨勢

**API 端點**：`/api/client/macro-brief`、現有 `/api/industry/cycles`、`/api/industry/seasonality`  
**注意**：產業生態系圖需簡化為投資人可讀版本（隱藏內部參數細節）

---

## 六、頁面 4：風險報告

**目的**：回答「最壞情況會虧多少？」

**核心內容**：
- 當前風險等級（RiskGate 輸出）
- VaR（95% 信賴水準）
- 壓力測試結果（5 個歷史情境）
- 風險參數校準狀態（CalibrationReport）
- 回撤保護狀態（MacroAwareDrawdownEngine）

**API 端點**：`/api/client/risk-status`  
**資料來源**：`risk/gate.go`、`risk/self_calibrate.go`、`risk/macro_aware_drawdown.go`

---

## 七、頁面 5：每日摘要

**盤前摘要（Morning Brief）**：
- 市場狀態（regime、風險等級、建議曝險）
- 今日推薦（top 5，附理由和信心度）
- 今日重要事件（narrative event + 命中率）
- 昨日推薦回顧（對 vs 錯）

**盤後回顧（Evening Brief）**：
- 今日模擬交易結果
- 推薦 vs 實際表現
- 風險事件觸發狀況
- 明日展望

**生成引擎**：`narrative/report_generator.go`（已存在，需擴展投資人語言模板）  
**API 端點**：`/api/client/macro-brief?type=morning|evening`

---

## 八、圖表渲染規範

- 使用 Canvas API 或 Chart.js CDN（無 npm）
- 不可引入 React/Vue 重型框架
- 現有 `web/static/` 已有 chart 程式碼，可參考複用
- 所有圖表需有基準對比線（TAIEX/0050）
- 支援 RWD（桌機為主，平板相容）

---

## 九、頁面導航結構

```
/client_web/index.html（儀表板）
  ├── → /client_web/pages/performance.html（績效分析）
  ├── → /client_web/pages/recommendations.html?symbol=X（推薦詳情）
  ├── → /client_web/pages/insights.html（市場洞察）
  ├── → /client_web/pages/risk.html（風險報告）
  └── → /client_web/pages/daily-brief.html（每日摘要）
```

全部從儀表板連結出發。**不設導航列**（避免開發者控制塔風格）。
