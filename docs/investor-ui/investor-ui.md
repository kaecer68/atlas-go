# Atlas 投資人 UI — 核心架構與設計原則

**版本**: 2.0（拆分自 v1.0）  
**日期**: 2026-06-02  
**成熟度**: X（experimental）  
**原始來源**: `atlas-investor-ui` v1.0（526 行）→ 拆分為 5 個聚焦技能

---

## 一、描述

本技能定義 Atlas 投資人面向 Web UI 的**架構、設計原則、目錄佈局、API 路由**。核心任務是**信任工程**（Trust Engineering）：將 Atlas 世界級的金融引擎能力，轉化為投資人可理解、可信任、可驗證的介面。

> Atlas 像一台沒有儀表板的 F1 賽車——引擎世界級，但駕駛座只有繼電器和電路圖。本技能的目標是裝上投資人看得懂的儀表板。

### 相關子技能

| 子技能 | 職責 |
|--------|------|
| `atlas-investor-pages` | 6 頁面 wireframe 與詳細規格 |
| `atlas-investor-nlg` | NLG 推薦解釋層（FactorScoreBreakdown → 繁體中文） |
| `atlas-investor-trustscore` | TrustScore 信任分數系統設計 |
| `atlas-investor-roadmap` | 實作階段、基準比較、歷史情境匹配、驗證 |

---

## 二、信任金字塔

投資人對量化系統的信任是分層建立的。每一層必須穩固，上一層才有意義：

```
                  5   真錢交易       ← 暫不觸及
                4   績效驗證       ← 🔴 最大缺口
              3   可解釋性       ← 🟡 有數據但沒轉化為語言
            2   追蹤紀錄       ← 🔴 無歷史績效儀表板
          1   透明度         ← 🟡 儀表板給開發者，非投資人
```

### 信任公式

```
信任 = 透明度 + 可驗證性 + 可解釋性 + 一致性 + 安全性 + 時間
```

---

## 三、架構決策

### 目錄分離（物理隔離）

```
./web/
├── admin_web/     # 現有開發者控制塔（從 web/static/ 搬遷）
└── client_web/    # 🆕 投資人專用介面
    ├── index.html # 儀表板（單頁總覽）
    ├── pages/     # 子頁面 HTML
    │   ├── performance.html
    │   ├── recommendations.html
    │   ├── insights.html
    │   ├── risk.html
    │   └── daily-brief.html
    ├── js/        # 投資人 UI 邏輯
    └── style/     # 投資人 UI 樣式
```

### 技術決策

| 決策 | 選擇 | 理由 |
|------|------|------|
| 技術棧 | HTML/CSS/JS + Go 後端 | 同現有，不引入新依賴 |
| 認證 | 無 | localhost-only |
| 語言 | 繁體中文 | 台股場景 |
| 部署 | 共用後端，只加 HTTP handler | 最小改動 |
| 框架 | 無框架 | 同現有模式 |

### API 路由（前綴 `/api/client/`）

| 端點 | 用途 | 主資料源 |
|------|------|----------|
| `GET /api/client/summary` | 每日摘要 | orchestrator + narrative + risk |
| `GET /api/client/performance?days=N` | 累積報酬 + 基準對比 | ledger + marketdata |
| `GET /api/client/recommendations?limit=N` | 推薦列表 | pipeline API |
| `GET /api/client/macro-brief` | 宏觀摘要 | narrative/report_generator.go |
| `GET /api/client/trust-score` | 信任分數 | TrustScore 模組 |
| `GET /api/client/risk-status` | 風險狀態 | risk/gate.go + self_calibrate.go |
| `GET /api/client/benchmark` | 基準指數 | marketdata provider |

---

## 四、核心設計原則

### 原則 1：投資人語言優先

| 開發者看到（❌） | 投資人看到（✅） |
|---|---|
| `"momentum": {"score": 0.37}` | 「動能強勁（+5.2%），技術面呈上升趨勢」 |
| `"regime": "expansion"` | 「市場狀態：溫和擴張」 |
| `"drawdown_guard_triggered"` | 「風險保護已啟動」 |

### 原則 2：一個頁面，一個答案

儀表板不應該有 tabs、不應該有技術術語。它是「我今天應該做什麼」的答案頁。

### 原則 3：永遠有基準對比

「組合賺 3%（TAIEX +1.8%）」→ 有意義。「組合賺 3%」→ 無意義。

### 原則 4：圖表優先，數字輔助

累積報酬曲線 > 單一數字。Sharpe 時間序列 > 單一 Sharpe。

### 原則 5：信任需要時間

回測歷史 + 紙上交易期（2-3 月）+ 每月透明度報告。

### 原則 6：不過度承諾

✅「建議關注」、「綜合評分 X/100」 ❌「強烈買入」、「保證獲利」

---

## 五、與其他技能整合

| 技能 | 整合方式 |
|------|---------|
| `atlas-core-architecture` | 理解系統架構後設計 API 路由 |
| `atlas-macro-narrative` | 宏觀摘要、事件展示、NLG 模板取用 |
| `atlas-risk-management` | 風險狀態、回撤保護展示 |
| `atlas-strategy-evolution` | 命中率、agent 績效排行 |
| `atlas-data-management` | 資料來源標記、fallback 標記 |
| `atlas-investor-pages` | 頁面結構與 wireframe 規範 |
| `atlas-investor-nlg` | 推薦理由轉換為投資人語言 |
| `atlas-investor-trustscore` | 信任分數計算與展示 |
| `atlas-investor-roadmap` | 實作階段、基準比較、驗證 |

---

## 六、高危陷阱

| 陷阱 | 預防 |
|------|------|
| 投資人語言 vs 開發者用語混淆 | 禁止 `FactorScore`、`DrawdownGuard` 等術語出現在投資人 UI |
| 沒有基準對比的績效 | 任何數字必須附 TAIEX/0050 對比 |
| 過度承諾 | 永不使用「保證」、「必賺」 |
| fallback 數據靜默 | VIX 用美版、因子 fallback → UI 標記 `⚠️` |
| 繞過統一 API 架構 | 透過 `internal/apigateway/` 統一路由 |
| 目錄混放 | `./admin_web/` vs `./client_web/` 物理隔離 |
| 儀表板不該有 tabs | 單頁總覽，其他頁透過連結導航 |
| API 效能不足 | Response time < 200ms，績效需 BackgroundTaskManager 預計算 |

---

## 七、關鍵檔案

| 檔案 | 用途 |
|------|------|
| `web/client_web/index.html` | 🆕 投資人儀表板主頁 |
| `internal/narrative/report_generator.go` | 報告產生器 — 參考擴展 |
| `internal/orchestrator/executors.go` | 推薦產生主入口 |
| `internal/risk/self_calibrate.go` | 校準報告 — 信任分數輸入 |
| `internal/risk/gate.go` | 風險閘門 — 風險報告輸入 |
| `internal/portfolio/factor_engine.go` | 因子分數 — NLG 輸入 |
| `.claude/SKILLS-MAP.md` | 技能索引 — 需加入本技能及子技能 |
