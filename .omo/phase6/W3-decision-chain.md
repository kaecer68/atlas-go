# W3: 決策可視化鏈

## 目標

建立一個統一的「每日決策視圖」，讓用戶從**即時事件 → 該注意的產業 → 具體標的 → 進出場建議**一條線看完，不用在 15 個頁面之間跳來跳去。

## 為什麼需要這個？

目前後端什麼 data 都有（52+ API handlers），但前端分散在 15 個頁面：
- 事件在 narrative 頁
- 產業在 industry 頁
- 標的在 pipeline 頁
- 進出場在 portfolio 頁

用戶看不到因果鏈。跑過的所有邏輯（數學、管理、架構）都是黑盒子。用戶不知道系統「為什麼推薦這檔股票」，只能看最後的 portfolio 總績效。但總績效是落後指標，用戶需要的是**解釋力**。

## 核心設計：決策鏈

```
┌────────────────────────────────────────────────────────────────────┐
│ 即時事件雷達                                          [更新於 09:32] │
│                                                                    │
│  📡 美股收盤: S&P500 +1.2%, NASDAQ +2.1%, SOX +3.5%               │
│  📡 外資動向: 昨日買超 234 億（連續 5 日買超）                      │
│  📡 匯率: USD/TWD 32.15 (+0.3%), 台幣走貶有利出口                  │
│  📡 AI capex: NVIDIA 財報超預期, 台積電 ADR +4.2%                  │
│  📡 BDI: 1520 (+8% from baseline), 航運需求升溫                    │
│                                                                    │
├────────────────────────────────────────────────────────────────────┤
│ 事件邏輯庫 — 從歷史學到的教訓                          [自我精進中] │
│                                                                    │
│  規則 #1: 「SOX +3% 且 外資連續買超 3 日」→ 半導體 80% 機率上漲   │
│  規則 #2: 「美元走強 + BDI 上升」→ 航運 65% 機率受惠               │
│  規則 #3: 「NVIDIA 財報超預期」→ AI supply chain 75% 機率連動       │
│  [+] 查看更多規則                                                  │
│                                                                    │
├────────────────────────────────────────────────────────────────────┤
│ 該注意什麼                                   [產業熱力圖]           │
│                                                                    │
│  🔥 半導體          置信度: HIGH   理由: SOX+3.5%+外資+AI事件       │
│  🔥 AI供應鏈        置信度: HIGH   理由: NVIDIA+台積電ADR            │
│  🟡 航運            置信度: MEDIUM 理由: BDI+8%+美元走強            │
│  ⚪ 金融            置信度: LOW    理由: 無觸發事件                  │
│                                                                    │
├────────────────────────────────────────────────────────────────────┤
│ 推薦標的                                                           │
│                                                                    │
│  2330 台積電   買入 300 股  理由: SOX+3.5%+ADR+4.2%  置信: 92%    │
│  2454 聯發科   買入 200 股  理由: AI供應鏈+外資買超  置信: 85%    │
│  2603 長榮     買入 500 股  理由: BDI上升+SCFI轉強   置信: 72%    │
│  2303 聯電     觀望         理由: 成熟製程需求不明    置信: 45%    │
│                                                                    │
├────────────────────────────────────────────────────────────────────┤
│ 出場提醒                                                           │
│                                                                    │
│  🔔 2317 鴻海   持股 15 天  獲利 +12%  建議: 部分獲利了結          │
│  🔔 3008 大立光 持股 30 天  虧損 -8%   建議: 停損檢討              │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

## 時間維度（你的第 1、2 點意見）

事件不再是「今天收盤後才有的東西」，而是：

```
即時事件雷達的時間窗口：
  ├── 本週大事件（往前 7 天）
  │     └── 來源: 已儲存的 narrative events + 手動標記
  ├── 今日盤前（今日 08:30 前）
  │     └── 來源: 美股收盤數據（約台灣時間 04:00-05:00）+ 晨間新聞
  ├── 昨日殘留（昨日 13:30 到今日 09:00）
  │     └── 來源: 昨日 MacroDataSnapshot + narrative events
  └── 盤中觸發（即時）
        └── 來源: TWSE 即時 quote + 閾值突破（價格/量/外資）

週一特例：
  ├── 週五收盤到週一開盤 = 2.5 天空窗
  ├── 顯示「週末國際要聞」— 美股週五收盤 + 週末重大事件
  └── 標記「數據滯後」— 讓用戶知道某些數據是 3 天前的
```

## 實作順序

### Phase W3-1: 事件時間窗 API
- 現有 `/api/narrative/events` 只能查單一 snapshot
- 需要新增：`/api/narrative/events/window?from=-7d&to=now`
- 需要新增：`/api/narrative/events/premarket`（只看盤前新鮮的）

### Phase W3-2: 統合 API
- 新增 `/api/dashboard/decision-chain` — 一次回傳事件+產業+標的+進出場
- 這個 API 是 W3 和 W4 的交集點（W4 的 EventLogicLibrary 注入邏輯規則）

### Phase W3-3: 前端決策鏈頁面
- 一個新頁面：`web/static/js/pages/decision-chain.js`
- 五個區塊：即時事件 / 事件邏輯庫 / 產業熱力 / 推薦標的 / 出場提醒
- 每個區塊可獨立折疊、獨立刷新

## API Contract（W4 需要遵守）

```
GET /api/dashboard/decision-chain
Response:
{
  "events": {
    "weekly":   [...],  // 本週大事件
    "premarket": [...], // 盤前事件
    "residual":  [...], // 昨日殘留
    "realtime":  [...]  // 盤中觸發
  },
  "logic_rules": [       // ← W4 提供
    {
      "id": "rule-1",
      "pattern": "SOX +3% 且 外資連續買超",
      "hit_rate": 0.80,
      "affected_sectors": ["semiconductor"],
      "confidence": "high"
    }
  ],
  "sector_heatmap": {
    "semiconductor":  {"confidence": "high", "reasons": [...]},
    "shipping":       {"confidence": "medium", "reasons": [...]}
  },
  "recommendations": [...],
  "exit_alerts": [...]
}
```

## 不可碰的檔案

- `internal/orchestrator/` — W2 的工作區
- `internal/portfolio/` — 不碰 optimizer
- `internal/live/` — 不碰 W4 以外的 live code

## 可修改/新增的範圍

- `internal/monitoring/api/narrative/handlers.go` — 加 window/premarket 端點
- `internal/monitoring/dashboard_api.go` — 加 decision-chain route
- 新建 `internal/monitoring/api/decision/handlers.go`
- `web/static/js/pages/decision-chain.js` — 新頁面
- `web/static/index.html` — 加導航入口
- `web/static/js/main.js` — 註冊新頁面

## 驗證條件

```bash
# API:
curl http://localhost:8080/api/narrative/events/window?from=-7d
# → 回傳本週事件，不為空

curl http://localhost:8080/api/dashboard/decision-chain
# → 回傳完整決策鏈 JSON，所有欄位非 null

# 前端:
# 瀏覽器打開 localhost:8080 → 點擊「決策鏈」
# → 五個區塊都渲染
# → 即時事件區塊顯示美股收盤、外資、匯率
# → 產業熱力區塊正確對應事件
# → 推薦標的區塊顯示理由
# → 出場提醒區塊不為空
```

## 完成報告格式

```markdown
## W3 Completion Report

### New API Endpoints
| Endpoint | Status | Sample Response Size |
|----------|--------|---------------------|

### Frontend
- [ ] decision-chain.js page renders
- [ ] All 5 sections populated
- [ ] Real-time events section shows current data
- [ ] Sector heatmap reflects event logic
- [ ] Recommendation reasons visible
- [ ] Exit alerts functional

### API Contract Compliance (for W4)
- [ ] /api/dashboard/decision-chain responds with logic_rules field
- [ ] logic_rules array is empty but properly typed (ready for W4 to fill)

### Screenshot
(paste screenshot of decision-chain page)

### To Investigate Next
(empty event sections, missing data sources, etc.)
```

將此報告存到 `/tmp/w3-report.md`
