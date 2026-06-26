# Atlas Skills Map — 技能分類體系地圖

**版本**: 5.0
**日期**: 2026-06-25
**用途**: Atlas-Go AI 技能統一索引與分類體系。AI Coding 時依分類快速定位所需技能。

---

## 技能總覽

| 分類 | 數量 | 說明 |
|------|------|------|
| 🔧 除錯維護迭代 | 2 | 修改前必用、資料可見性防護 |
| 📊 邏輯計算 | 8 | 策略、風控、宏觀、因子、權重 |
| 🤖 機器人溝通 | 4 | OpenClaw/Hermes Agent 投資人互動 |
| 🔗 第三方工具 | 6 | GitNexus 程式碼智慧 |

**總計: 20 技能**（10 手寫 + 6 GitNexus + 4 機器人溝通）

> ⚠️ **Token 節省提示**: 程式碼導航請優先使用 GitNexus 工具或 `internal/<mod>/AGENTS.md`，不再維護自動生成技能索引。

---

## 技能目錄結構

```
.claude/skills/
├── SKILL_TEMPLATE.md              # 統一手寫技能編寫模板
├── SKILLS-MAP.md                  # 本文件（技能分類地圖）
│
├── 🔧 除錯維護迭代
│   ├── atlas-pre-change-protocol/ # 修改前 7 步驟強制檢查清單
│   └── atlas-data-visibility/     # 四層資料可見性防護（亦屬 🛡️）
│
├── 📊 邏輯計算
│   ├── atlas-macro-narrative/     # 宏觀敘事：六大維度外資流向推導
│   ├── atlas-risk-management/     # 風險管理：四層架構、動態倉位、VaR
│   ├── atlas-strategy-evolution/  # 策略進化：Darwinian 權重、mutation brief
│   ├── atlas-swarm-analyst/       # Swarm 分析：MiroFish 模擬結果解讀
│   ├── atlas-multi-strategy/      # 多策略編排：選擇、切換、比較
│   ├── atlas-event-driven-weights/# 事件驅動權重：12 因子 FactorEngine 動態調整
│   ├── atlas-strategy-techniques/ # 投資心法庫：5 層架構 + 12 production seeds
│   └── atlas-taiwan-leading-indicators/ # 短線指標：4 核心領先指標
│
├── 🤖 robot-communication/        # 機器人溝通專用技能（OpenClaw/Hermes）
│   ├── README.md                  # 機器人溝通技能使用說明
│   ├── atlas-daily-briefing/      # 每日投資摘要
│   ├── atlas-portfolio-qa/        # 投資組合問答
│   ├── atlas-strategy-explain/    # 投資策略解釋
│   └── atlas-risk-status/         # 風險狀態解讀
│
└── 🔗 gitnexus/                   # 6 GitNexus 技能（程式碼智慧工具）
    ├── gitnexus-cli/  gitnexus-guide/
    ├── gitnexus-exploring/  gitnexus-impact-analysis/
    └── gitnexus-debugging/  gitnexus-refactoring/
```

---

## 分類詳細說明

### 🔧 除錯維護迭代（Debug / Maintenance / Iteration）

AI Coding 過程中必用的診斷、防護與迭代安全技能。

| 技能 | 用途 | 觸發條件 | 版本 |
|------|------|---------|------|
| `atlas-pre-change-protocol` | 7 步驟修改前檢查：blast radius → 模組陷阱 → 數據溯源 → 憲法檢查 → 模式匹配 → GitNexus 架構 → 代碼意圖 | **修改任何程式碼前強制執行** | v1.0 |
| `atlas-data-visibility` | 四層資料可見性防護：Gateway/Adapter/Service/Frontend，防止零值掩蓋通道靜默失敗 | 資料流修改、通道新增、前端 data_status 欄位變更時 | v2.0 |

> **強制規則**: `atlas-pre-change-protocol` 是所有程式碼修改的前置條件，不可跳過。執行方式: `skill(name="atlas-pre-change-protocol")`

### 📊 邏輯計算（Logic & Computation）

金融工程核心技能 — 策略推導、風險計算、因子分析、權重調整與市場解讀。

| 技能 | 用途 | 實作狀態 | 版本 |
|------|------|----------|------|
| `atlas-macro-narrative` | 六大維度外資流向推導：Fed 利率/美元流動性、美台股市連動、地緣政治、電子業週期、政策干預、選舉兩岸 | ✅ 完整 | v1.2 |
| `atlas-risk-management` | 四層風險架構：RiskGate → VaR → Drawdown → AutoCalibration，動態倉位調整 | ✅ 完整 | v2.0 |
| `atlas-strategy-evolution` | 策略進化循環：模型績效追蹤、Darwinian 權重、mutation brief 生成 | ✅ 完整 | v1.1 |
| `atlas-swarm-analyst` | MiroFish Swarm 模擬結果解讀：市場共識、異常偵測、API 端點規範 | ✅ 完整 | v1.0 |
| `atlas-multi-strategy` | 多策略框架：策略選擇器、分配器、策略比較與切換 | ✅ 完整 | v1.0 |
| `atlas-event-driven-weights` | 12 因子動態權重系統：FactorEngine 事件驅動調整、FactorBridge 銜接 | ✅ 完整 | v1.0 |
| `atlas-strategy-techniques` | 5 層投資心法庫：基本面、技術面、籌碼面、宏觀面、事件面，含 12 production seeds | ✅ 完整 | v1.0 |
| `atlas-taiwan-leading-indicators` | 4 核心短線指標：外資期貨未平倉、TSE 融資餘額、VIX 台指、美元/台幣匯率 | ✅ 完整 | v1.0 |

### 🛡️ 資料安全（Data Security）

確保資料完整性、可追溯性與安全防護。

| 技能 | 用途 | 版本 |
|------|------|------|
| `atlas-data-visibility` | 四層資料可見性防護（同時歸屬 🔧 除錯維護），防止以零值掩蓋底層失敗 | v2.0 |

### 🤖 機器人溝通（Robot Communication）

供 OpenClaw、Hermes Agent 等 AI 機器人使用的投資人溝通技能。目標：將 Atlas 複雜數據轉化為投資人可理解的繁體中文回應。

| 技能 | 用途 | 目標受眾 | 版本 |
|------|------|---------|------|
| `atlas-daily-briefing` | 每日投資摘要：市場狀態、風險等級、建議曝險、Top Picks、關鍵宏觀事件 | 投資人 | v1.0 |
| `atlas-portfolio-qa` | 投資組合問答：持倉解釋、配置理由、績效指標、再平衡建議 | 投資人 | v1.0 |
| `atlas-strategy-explain` | 投資策略白話解釋：因子選股、Darwinian 進化、多策略框架（非技術語言） | 投資人 | v1.0 |
| `atlas-risk-status` | 風險狀態解讀：風險燈號（綠/黃/紅）、VaR 意涵、進出場時機建議 | 投資人 | v1.0 |

> **使用注意**: 機器人溝通技能僅供 OpenClaw/Hermes Agent 載入。AI Coding 時**不應自動載入**（`auto_load: false`），除非正在開發投資人面向功能。

### 🔗 第三方工具（Third-party Tools）

GitNexus 程式碼智慧工具技能。

| 技能 | 用途 | 觸發範例 |
|------|------|---------|
| `gitnexus-exploring` | 程式碼架構理解 | "How does X work?" |
| `gitnexus-impact-analysis` | 變更影響分析 | "What breaks if I change X?" |
| `gitnexus-debugging` | 除錯追踪 | "Why is X failing?" |
| `gitnexus-refactoring` | 安全重構 | "Rename this function" |
| `gitnexus-guide` | 工具與 Schema 參考 | "What GitNexus tools available?" |
| `gitnexus-cli` | CLI 命令操作 | "Index this repo" |

---

## AI Coding 技能使用流程

```
開始任何程式碼修改
  └── 🔴 atlas-pre-change-protocol（強制，不可跳過）

依任務類型選擇技能：
  ├── 資料流/通道變更
  │   └── 🟡 atlas-data-visibility
  │
  ├── 風險/倉位相關
  │   └── 📊 atlas-risk-management
  │
  ├── 宏觀/敘事相關
  │   └── 📊 atlas-macro-narrative
  │
  ├── 策略/進化相關
  │   ├── 📊 atlas-strategy-evolution
  │   ├── 📊 atlas-multi-strategy
  │   └── 📊 atlas-strategy-techniques
  │
  ├── 權重/因子相關
  │   └── 📊 atlas-event-driven-weights
  │
  ├── Swarm 分析相關
  │   └── 📊 atlas-swarm-analyst
  │
  ├── 短線指標相關
  │   └── 📊 atlas-taiwan-leading-indicators
  │
  ├── LLM 框架（router / capability handler / DataClass 治理）
  │   └── internal/llm/AGENTS.md §1 跨模組陷阱必讀
  │
  ├── 投資人面向功能開發
  │   └── 🤖 robot-communication/*
  │
  └── 跨模組影響分析 / 程式碼導航
      └── 🔗 GitNexus 工具（依 CLAUDE.md 規範）
```

---

## Token 節省規範

為防止 AI Coding 時無效 token 消耗，以下技能**禁止自動載入**：

| 技能群 | 載入規則 | 原因 |
|--------|---------|------|
| `robot-communication/*` (4 個) | `auto_load: false` — 僅機器人載入 | 投資人面向內容，開發時不需要 |
| `gitnexus/*` (6 個) | 依 CLAUDE.md 規範按需載入 | 工具技能，非每次 session 需要 |

**自動載入白名單**（AI Coding session 中有領域價值時載入）：
- `atlas-pre-change-protocol` — 每次修改前強制
- `atlas-data-visibility` — 資料流變更時
- 8 個 📊 邏輯計算技能 — 依任務類型按需

---

## 文件關聯

| 文件 | 角色 | 關聯 |
|------|------|------|
| `AGENTS.md` | 全域規則與模組路由 | 引用本文件為技能索引 |
| `CLAUDE.md` | 工具進入點（含 GitNexus 規範） | 存放 GitNexus 完整規範以避免重複 |
| `docs/GUIDELINES_INDEX.md` | 規範階層與衝突仲裁 | 技能為階層 3 |
| `docs/ENVIRONMENT.md` | 外部依賴與開發環境狀態單一真相來源 | 技能使用前先確認環境狀態 |
| `.claude/skills/SKILL_TEMPLATE.md` | 統一手寫技能模板 | 新建技能時的格式規範 |
| `internal/apigateway/CONSTITUTION.md` | 憲法級強制規範 | 技能不可違反憲法 |
| `docs/CONSTITUTION.md` | 深度憲法（矩陣運算、證偽） | 策略技能須遵守數學約束（2026-06-26 PR #752 從 `.omo/CONSTITUTION.md` 移入，因 `.gitignore` 排除而新 clone 不可見）|

---

## 修訂歷史

| 版本 | 日期 | 修訂內容 |
|------|------|---------|
| 5.0 | 2026-06-25 | 移除全數 `generated/*` 技能；總數 43 → 20；程式碼導航改由 GitNexus 與 `internal/<mod>/AGENTS.md` 負責 |
| 4.2 | 2026-06-25 | 加入 `internal/llm/` generated 技能（LLM 路由器、12 capability handlers）；總數 42 → 43 |
| 4.1 | 2026-06-25 | 加入 `docs/ENVIRONMENT.md` 引用；更新日期以反映 PR #700 |
| 4.0 | 2026-06-17 | 全面重寫：建立 6 大分類體系、補齊 3 缺失技能（strategy-techniques, taiwan-leading-indicators, data-visibility）、新增機器人溝通分類（4 個新技能）、加入 Token 節省規範、修正不存在技能引用、更新總數為 42 |
| 3.0 | 2026-06-03 | 加入 generated/ 和 gitnexus/ 目錄 |
