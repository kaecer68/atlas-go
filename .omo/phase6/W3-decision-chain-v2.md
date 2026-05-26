# W3: 決策可視化鏈

## 目標

建立 `/api/dashboard/decision-chain` 聚合端點 + `web/static/js/pages/decision-chain.js` 前端頁面。

用戶從**即時事件 → 事件邏輯庫 → 該注意的產業 → 具體標的 → 出場提醒**一條線看完。

## 現有 API（W4 已完成）

```
GET /api/eventlogic/rules/active    → 6 seed rules (W4 deliverable)
GET /api/narrative/events            → narrative events
GET /api/dashboard/industry-overview → sector data
GET /api/dashboard/recommendation-pipeline → recommendations
GET /api/narrative/stress-index/current → market stress
```

## Phase 1: 聚合 API（後端）

新建 `internal/monitoring/api/decision/handlers.go`

### `GET /api/dashboard/decision-chain`

```json
{
  "events": {
    "today": [...],       // 今天的 narrative events
    "recent": [...],      // 最近 7 天的 events
    "premarket": {        // 盤前關鍵數據（美股收盤、外資、匯率）
      "us_market": {"sp500_pct": 1.2, "nasdaq_pct": 2.1, "sox_pct": 3.5},
      "foreign_flow": {"net_buy_twd": 234, "consecutive_days": 5},
      "fx": {"usd_twd": 32.15, "change_pct": 0.3},
      "bdi": {"value": 1520, "deviation_pct": 8}
    }
  },
  "logic_rules": [        // ← W4 的 /api/eventlogic/rules/active
    {
      "id": "sox-foreignflow-semiconductor",
      "pattern": "SOX > +3% 且外資連續買超 >= 3日 → 半導體上漲",
      "hit_rate": 0.80,
      "affected_sectors": ["semiconductor"],
      "direction": "up",
      "status": "active"
    }
  ],
  "sector_heatmap": [
    {"sector": "semiconductor", "confidence": "high", "reasons": ["SOX+3.5%", "外資連續買超"]},
    {"sector": "ai_supply_chain", "confidence": "high", "reasons": ["NVIDIA財報"]},
    {"sector": "shipping", "confidence": "medium", "reasons": ["BDI+8%"]}
  ],
  "recommendations": [    // ← pipeline recommendations
    {
      "symbol": "2330", "name": "台積電", "action": "buy", "shares": 300,
      "confidence": 0.92, "reasons": ["SOX+3.5%", "ADR+4.2%"]
    }
  ],
  "exit_alerts": [        // ← portfolio positions with PnL
    {
      "symbol": "2317", "name": "鴻海", "days_held": 15,
      "pnl_pct": 12, "suggestion": "部分獲利了結"
    }
  ]
}
```

### 實作方式

Handler 內部並行呼叫 5 個現有 endpoint（或直接呼叫內部 service method），aggregated 後回傳。

```go
func (h *Handlers) HandleDecisionChain(w http.ResponseWriter, r *http.Request) {
    // 1. Fetch narrative events
    // 2. Fetch active event logic rules
    // 3. Fetch industry overview
    // 4. Fetch recommendations
    // 5. Fetch portfolio positions for exit alerts
    // 6. Aggregate → one JSON response
}
```

### 註冊路由

```go
// 在 dashboard_api.go:
mux.HandleFunc("/api/dashboard/decision-chain", decisionHandler.HandleDecisionChain)
```

## Phase 2: 前端頁面（前端）

新建 `web/static/js/pages/decision-chain.js`

### 五個區塊

```
┌─────────────────────────────────┐
│ 即時事件雷達    [更新於 09:32]    │
│ 📡 美股 / 外資 / 匯率 / BDI      │
├─────────────────────────────────┤
│ 事件邏輯庫      [自我精進中]      │
│ 規則 #1: SOX+3% → 半導體 80%    │
├─────────────────────────────────┤
│ 該注意什麼       [產業熱力圖]      │
│ 🔥 半導體  🟡 航運  ⚪ 金融     │
├─────────────────────────────────┤
│ 推薦標的                         │
│ 2330 台積電 買入 300股 置信92%   │
├─────────────────────────────────┤
│ 出場提醒                         │
│ 🔔 2317 鴻海 +12% 部分獲利了結   │
└─────────────────────────────────┘
```

### 實作

- 一個 `fetch('GET /api/dashboard/decision-chain')` 拿全部 data
- 五個 `render()` 函數分別畫五個區塊
- 每個區塊獨立折疊、獨立刷新
- 註冊到 `main.js` 的頁面路由

## 不可碰的檔案

- `internal/portfolio/` — 不碰
- `internal/orchestrator/` — 不碰
- `internal/live/` — 不碰
- `internal/eventlogic/` — 只讀（API 已就緒）

## 可修改/新增

- 新建 `internal/monitoring/api/decision/handlers.go`
- `internal/monitoring/dashboard_api.go` — 加 route registration
- 新建 `web/static/js/pages/decision-chain.js`
- `web/static/index.html` — 加導航入口
- `web/static/js/main.js` — 註冊頁面

## 驗證

```bash
curl localhost:8080/api/dashboard/decision-chain | jq '.events.premarket'
# → 不為空
curl localhost:8080/api/dashboard/decision-chain | jq '.logic_rules | length'
# → >= 6 (seed rules)
curl localhost:8080/api/dashboard/decision-chain | jq '.sector_heatmap | length'
# → > 0
```

前端：
- 打開 localhost:8080 → 點擊「決策鏈」
- 五個區塊都有內容，無 JS error
- 「事件邏輯庫」區塊顯示 hit_rate 進度條

## 交付報告

存到 `/tmp/w3-report.md`：

```markdown
## W3 Completion Report

### API
| Endpoint | Status | Fields |
|----------|--------|--------|

### Frontend
- [ ] decision-chain.js renders
- [ ] 5 panels populated
- [ ] Event logic rules show hit_rate
- [ ] Sector heatmap matches events
- [ ] Exit alerts show PnL

### Screenshot
(paste screenshot of decision-chain page)
```
