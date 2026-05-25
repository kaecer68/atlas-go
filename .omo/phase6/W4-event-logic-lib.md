# W4: 事件邏輯庫

## 目標

建立一個**自我精進的事件因果邏輯庫**，讓系統從歷史數據中自動學習「什麼事件 → 什麼產業/股票 → 什麼方向」的因果模式，並持續驗證、自我糾正。

## 為什麼需要這個？

用戶的原始回饋（節錄）：

> 台幣始終盯著與美元的匯率，根本不是自由匯率
> 台灣股市深度不夠，是外資最喜歡操作的標的
> 股票漲跌幾乎與外資的進出有明顯的依賴關係
> 台灣的股市幾乎都是外銷為主，特別是幾乎為美國的電子業、半導體業
> 美股漲台股也跟著漲，而且有時差，若是盯著時差這個事情做好，都可以賺爆
> 產業連動、供應鏈連動、季節性連動等等
> 如何看的見？不然信心在哪裡？

核心命題：**這些模式都是客觀存在的，但系統沒有把他們「捕捉、記錄、驗證、可視化」**。

## 架構設計

```
internal/eventlogic/
├── rule.go              ← EventRule 定義 (pattern, conditions, hit_rate, status)
├── registry.go          ← RuleRegistry (CRUD + 查詢)
├── detector.go          ← PatternDetector (從歷史數據自動發現新模式)
├── validator.go         ← RuleValidator (回測 + 計算 hit_rate)
├── corrector.go         ← SelfCorrector (低命中率自動降權/停用)
├── timezone.go          ← TimezoneArbitrage (美股→台股時差信號)
├── foreignflow.go       ← ForeignFlowTracker (外資流向→個股預測)
├── supplychain.go       ← SupplyChainEvent (供應鏈事件→上下游連動)
├── api_handlers.go      ← API handlers (讀取/管理規則)
└── rule_test.go          ← 測試
```

### 核心類型

```go
type EventRule struct {
    ID            string
    Pattern       string        // 人類可讀描述 "SOX > +3% AND 外資連續買超 >= 3日"
    Conditions    []Condition   // 可程式化條件
    AffectedSectors []string    // 受影響產業
    AffectedStocks  []string    // 受影響個股（可選）
    Direction     string        // "up" | "down" | "volatile"
    HitRate       float64       // 0.0 - 1.0
    TotalTests    int           // 總驗證次數
    TotalHits     int           // 命中次數
    ConfidenceSource string     // "backtest" | "manual" | "auto_discovered"
    Status        string        // "active" | "degraded" | "expired"
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type Condition struct {
    Field    string  // 例如 "SOXIndex.ChangePct"
    Operator string  // "gt" | "lt" | "gte" | "lte" | "eq"
    Value    float64
}
```

### 自我精進流程

```
每場模擬結束後：
  RecordOutcome() 
    → 對照實際 forward return
    → 更新每條規則的 HitRate
    → 連續 10 次失敗 → 標記 "degraded"
    → 連續 20 次失敗 → 標記 "expired"（自動停用）
    → 連續 5 次命中 → 從 degraded 恢復 active

自動發現新模式（每週）：
  DiscoverPatterns()
    → 掃描最近 90 天的 narrative events + 價格變化
    → 找統計顯著的因果關聯（p < 0.05）
    → 生成候選規則 → 人工審查 → 啟用
```

## 內建規則（種子規則）

這些是你提到的台灣市場特有模式，直接內建為種子規則：

```go
var seedRules = []EventRule{
    {
        ID: "sox-foreignflow-semiconductor",
        Pattern: "SOX 指數 > +3% 且外資連續買超 >= 3 日 → 半導體產業上漲",
        Conditions: []Condition{
            {Field: "SOXIndex.ChangePct", Operator: "gt", Value: 3.0},
            {Field: "ForeignInvestorNet.ConsecutiveDays", Operator: "gte", Value: 3},
        },
        AffectedSectors: []string{"semiconductor"},
        Direction: "up",
        ConfidenceSource: "manual",
    },
    {
        ID: "usmarket-taiwan-lag",
        Pattern: "美股收盤漲跌 → 台股開盤方向（時差套利）",
        Conditions: []Condition{
            {Field: "USMarketClose.Direction", Operator: "eq", Value: 1}, // up
        },
        AffectedSectors: []string{"semiconductor", "ai_supply_chain", "electronics"},
        Direction: "up",
        ConfidenceSource: "manual",
    },
    {
        ID: "dxy-strong-export-boost",
        Pattern: "美元走強 + BDI 上升 → 航運受惠",
        Conditions: []Condition{
            {Field: "DXY.ChangePct", Operator: "gt", Value: 0.5},
            {Field: "Bdi.ChangePct", Operator: "gt", Value: 5.0},
        },
        AffectedSectors: []string{"shipping"},
        Direction: "up",
        ConfidenceSource: "manual",
    },
    {
        ID: "foreign-outflow-bearish",
        Pattern: "外資連續賣超 >= 5 日 → 全市場偏空",
        Conditions: []Condition{
            {Field: "ForeignInvestorNet.ConsecutiveDays", Operator: "gte", Value: 5},
            {Field: "ForeignInvestorNet.Direction", Operator: "eq", Value: -1},
        },
        AffectedSectors: []string{"*"}, // all sectors
        Direction: "down",
        ConfidenceSource: "manual",
    },
    {
        ID: "nvidia-earnings-ai-chain",
        Pattern: "NVIDIA 財報超預期 → AI supply chain 連動",
        Conditions: []Condition{
            {Field: "NarrativeTheme", Operator: "eq", Value: "AI_capex_surge"},
        },
        AffectedSectors: []string{"ai_supply_chain", "semiconductor", "electronics"},
        Direction: "up",
        ConfidenceSource: "manual",
    },
    {
        ID: "usd-twd-managed-float",
        Pattern: "USD/TWD 突破 32.0 → 出口股受惠（台幣貶值有利出口）",
        Conditions: []Condition{
            {Field: "USD_TWD.Value", Operator: "gt", Value: 32.0},
        },
        AffectedSectors: []string{"electronics", "semiconductor", "shipping"},
        Direction: "up",
        ConfidenceSource: "manual",
    },
}
```

## API

```
GET  /api/eventlogic/rules              → 列出所有規則（含 hit_rate, status）
GET  /api/eventlogic/rules/:id          → 單一規則詳情
GET  /api/eventlogic/rules/active       → 只回傳 active 的規則（W3 用）
GET  /api/eventlogic/rules/expired      → 失效規則列表（供人工審查）
POST /api/eventlogic/rules/:id/validate → 手動觸發驗證
GET  /api/eventlogic/stats              → 總體統計（總規則數、active數、平均命中率、最近發現）
POST /api/eventlogic/discover           → 手動觸發自動發現
```

## 與 W3 的合約

W3 的 `/api/dashboard/decision-chain` 中的 `logic_rules` 欄位由 W4 的 `GET /api/eventlogic/rules/active` 提供。

```
W3 前端顯示:
  規則 #1: 「SOX +3% 且 外資連續買超 3 日」→ 半導體 80% 機率上漲
  規則 #2: 「美元走強 + BDI 上升」→ 航運 65% 機率受惠
  
  每條規則旁顯示:
    - hit_rate 進度條
    - 最近 N 次驗證結果（✅✅❌✅✅）
    - 是否 active / degraded / expired
```

## 實作順序

### Phase W4-1: 核心類型 + Registry
- `rule.go` + `registry.go`
- CRUD 操作
- 種子規則載入

### Phase W4-2: Validator
- `validator.go`
- 對照 forward return 計算 hit_rate
- 在每次模擬結束後自動觸發

### Phase W4-3: Self-Corrector
- `corrector.go`
- degraded/expired 狀態轉換
- 自動降權/恢復

### Phase W4-4: Pattern Detector
- `detector.go`
- 從歷史數據發現新模式
- 統計顯著性檢驗

### Phase W4-5: API + W3 合約
- API handlers
- 與 W3 的 decision-chain 整合點

## 不可碰的檔案

- `internal/orchestrator/` — W2 的工作區
- `internal/portfolio/` — 不碰 optimizer
- `internal/live/` — W2 的工作區（但 live 模擬結果可以用來驗證規則）

## 可修改/新增的範圍

- 新建 `internal/eventlogic/` 整個目錄
- `internal/monitoring/dashboard_api.go` — 加 RegisterEventLogicRoutes
- `internal/orchestrator/system.go` — 加 PostSimulation 勾子（呼叫 Validator）
- `cmd/atlas/main.go` — 初始化 EventLogicLibrary

## 驗證條件

```bash
go build ./internal/eventlogic/...
go test ./internal/eventlogic/...

# 種子規則載入:
# 啟動 atlas → 6 條種子規則自動載入
curl http://localhost:8080/api/eventlogic/rules
# → 回傳 6 條規則，每條有 hit_rate=0.0（未驗證）

# 模擬跑完後:
go run ./cmd/atlas --simulate --date 2026-03-26
curl http://localhost:8080/api/eventlogic/rules/active
# → hit_rate 更新（有些規則可能命中、有些可能未命中）

# 自我糾正:
# 修改一條規則的 hit_rate 為 0（模擬連續失敗）
curl -X POST http://localhost:8080/api/eventlogic/rules/rule-1/validate
# → status 變 "degraded"
```

## 完成報告格式

```markdown
## W4 Completion Report

### Module Structure
| File | Purpose | LOC |
|------|---------|-----|

### Seed Rules
| ID | Pattern | Sectors | HitRate (initial) |
|----|---------|---------|-------------------|

### Validator
- [ ] Post-simulation hook fires
- [ ] Rules validated against forward returns
- [ ] HitRate updates correctly

### Self-Corrector
- [ ] 10 consecutive failures → degraded
- [ ] 20 consecutive failures → expired
- [ ] 5 consecutive hits → recovered from degraded

### API
- [ ] All 6 endpoints respond
- [ ] GET /api/eventlogic/rules/active returns only active rules
- [ ] W3 contract: logic_rules field populated

### Pattern Discovery
- [ ] DiscoverPatterns() runs without error
- [ ] (optional) Found any new patterns?

### Test Coverage
- [ ] go test ./internal/eventlogic/... coverage >= 60%
```

將此報告存到 `/tmp/w4-report.md`
