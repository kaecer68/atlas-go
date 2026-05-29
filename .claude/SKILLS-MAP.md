# Atlas Skills Map

**版本**: 2.0  
**日期**: 2026-05-29  
**用途**: 統一管理 Atlas-Go 應用的所有 AI 技能與設計文件  

---

## 技能架構總覽

```
.claude/skills/
├── atlas-pre-change-protocol/        # 變更前協議（必先使用）
│   └── SKILL.md                      # 變更前檢查、風險界定、影響分析
│
├── atlas-core-architecture/          # 核心架構技能
│   └── SKILL.md                      # 系統架構總覽、分層設計原則
│
├── atlas-macro-narrative/            # 宏觀敘事技能
│   └── SKILL.md                      # 六大維度推導、外資流動預測
│
├── atlas-risk-management/            # 風險管理技能
│   └── SKILL.md                      # 回撤機制、熔斷策略、動態倉位
│
├── atlas-strategy-evolution/         # 策略進化技能
│   └── SKILL.md                      # 投資模型、權重調整、績效回饋
│
├── atlas-operations-guide/           # 操作指南技能
│   └── SKILL.md                      # 日常運維、實驗流程、緊急應變
│
├── atlas-swarm-analyst/              # Swarm 分析技能
│   └── SKILL.md                      # 群體模擬、協同分析、結果彙整
│
├── atlas-multi-strategy/             # 多策略編排技能
│   └── SKILL.md                      # 多策略路由、組合協調、選擇邏輯
│
├── atlas-news-sentiment/             # 新聞情緒技能
│   └── SKILL.md                      # 新聞解讀、情緒分類、事件影響
│
├── atlas-event-driven-weights/       # 事件驅動權重技能
│   └── SKILL.md                      # 事件觸發加權、動態調整、回饋
│
├── atlas-data-management/            # 資料管理技能
│   └── SKILL.md                      # 資料治理、讀寫規範、保存路徑
│
└── atlas-dynamic-correlation/        # 動態相關性技能
    └── SKILL.md                      # 關聯矩陣、聯動調整、共振分析
```

---

## 技能分類總覽

### 0. 變更前必用

- `atlas-pre-change-protocol`：**所有變更前的第一個技能**，先做影響分析、風險界定與變更邊界確認

### 1. Atlas 核心技能

- `atlas-core-architecture`：系統架構、模組邊界、資料流與核心設計原則
- `atlas-macro-narrative`：宏觀敘事、外資流向、事件情境推導
- `atlas-risk-management`：風險控制、回撤機制、動態倉位管理
- `atlas-strategy-evolution`：策略演化、模型績效、實驗生命週期
- `atlas-operations-guide`：日常運維、緊急應變、標準作業流程
- `atlas-swarm-analyst`：群體模擬、協同觀察、結果分析
- `atlas-multi-strategy`：多策略協調、路由與組合選擇
- `atlas-news-sentiment`：新聞情緒分析、事件分類、影響判讀
- `atlas-event-driven-weights`：事件驅動權重調整與回饋
- `atlas-data-management`：資料治理、保存、讀寫與一致性管理
- `atlas-dynamic-correlation`：動態相關矩陣、產業聯動、共振分析

### 2. GitNexus 技能

- `gitnexus-cli`：GitNexus CLI 操作、索引、狀態與維護
- `gitnexus-guide`：GitNexus 工具、資源與工作流指南
- `gitnexus-exploring`：結構探索、執行流程理解、架構追蹤
- `gitnexus-impact-analysis`：變更影響分析、爆炸半徑、風險評估
- `gitnexus-debugging`：錯誤追蹤、根因分析、故障定位
- `gitnexus-refactoring`：安全重構、抽取、拆分、搬移與改名

### 3. 自動生成技能（generated/*）

> **註**：以下為 auto-generated artifacts，由對應 `internal/*` 模組生成/同步，屬於技能索引的一部分。

- `generated/apigateway`：Apigateway 模組技能
- `generated/atlas`：Atlas 模組技能
- `generated/baseline`：Baseline 模組技能
- `generated/eventbus`：Eventbus 模組技能
- `generated/experiment`：Experiment 模組技能
- `generated/industry`：Industry 模組技能
- `generated/janus`：Janus 模組技能
- `generated/ledger`：Ledger 模組技能
- `generated/live`：Live 模組技能
- `generated/marketdata`：Marketdata 模組技能
- `generated/monitoring`：Monitoring 模組技能
- `generated/narrative`：Narrative 模組技能
- `generated/orchestrator`：Orchestrator 模組技能
- `generated/portfolio`：Portfolio 模組技能
- `generated/prism`：Prism 模組技能
- `generated/realtime`：Realtime 模組技能
- `generated/risk`：Risk 模組技能
- `generated/service`：Service 模組技能
- `generated/sim`：Sim 模組技能
- `generated/spawning`：Spawning 模組技能
- `generated/tax`：Tax 模組技能

### 4. 補充說明

- `generated/*` 技能可視為各模組的自動同步索引，不取代人工撰寫的 Atlas 主技能
- `atlas-core-architecture` 與 `generated/atlas` 共同對應整體架構與入口知識

---

## 技能詳細說明

### 0. atlas-pre-change-protocol（變更前協議）

**職責**: 所有變更前的第一道門檻，先確認影響範圍、風險與執行順序

**涵蓋內容**:
- 變更前檢查清單
- 影響分析與風險界定
- 需求、邊界、驗證條件確認

**使用時機**:
- 任何程式、文件、設定變更前
- 開始探索、重構、修補前
- 需要先釐清變更邊界時

**對應文件**:
- `.claude/skills/atlas-pre-change-protocol/SKILL.md`

---

### 1. atlas-core-architecture（核心架構）

**職責**: 理解 Atlas-Go 的整體架構、資料流、模組邊界

**涵蓋內容**:
- 分層架構：`Market Data → Orchestrator → Simulator → Ledger`
- 核心模組：`internal/domain/`, `internal/orchestrator/`, `internal/sim/`, `internal/experiment/`
- 資料流：`quotes → recommendations → orders → positions → ledger`
- 關鍵設計模式：領域驅動、事件溯源、稽核導向

**使用時機**:
- 新功能開發前，確認架構邊界
- 跨模組修改時，確認影響範圍
- 效能優化時，確認瓶頸位置

**對應文件**:
- `docs/architecture.md`
- `docs/ai_agent_architecture.md`
- `agents.md`

---

### 2. atlas-macro-narrative（宏觀敘事）

**職責**: 基於宏觀數據推導外資流動方向與資金輪動趨勢

**涵蓋內容**:
- 六大輸入維度：美元、美債、日圓、匯率、商品、地緣政治
- 外資出逃機率模型（0-100%）
- 資金流向推導：`risk_off` / `sector_rotation` / `carry_trade_unwind`
- 情境感知：同樣的「戰爭」主題，不同的地點/環境導致不同結果

**使用時機**:
- 每日開盤前，評估當日宏觀風險等級
- 重大事件發生時（戰爭、升息、地緣政治），快速推導影響
- 投資組合調整時，確認宏觀環境是否支持

**對應文件**:
- `.claude/skills/atlas-macro-narrative/SKILL.md`
- `internal/narrative/` 相關程式碼

---

### 3. atlas-risk-management（風險管理）

**職責**: 動態調整投資組合風險暴露，保護資本

**涵蓋內容**:
- 三層回撤機制：綠/黃/橙/紅 風險等級
- 動態倉位調整：基於宏觀風險的單檔上限調整（22% → 15% → 10% → 0%）
- 結構性豁免：當 AI 趨勢強勁時，容忍宏觀逆風
- 產業輪動：非現金為王，而是切換至受益板塊

**使用時機**:
- 組合回撤達到閾值時，決定行動
- 宏觀風險升級時，預防性調整
- 每日收盤後，評估是否需要調整隔日策略

**對應文件**:
- `.claude/skills/atlas-risk-management/SKILL.md`
- `docs/superpowers/specs/2026-04-23-macro-aware-drawdown-strategy-v2.md`
- `internal/risk/` 相關程式碼

---

### 4. atlas-strategy-evolution（策略進化）

**職責**: 投資模型的動態調整與績效回饋

**涵蓋內容**:
- 投資模型權重動態調整（AI模型/避險模型/Fed模型）
- 模型績效追蹤（預測準確率、PnL影響）
- 實驗生命週期：Propose → Execute → Judge → Promote
- Darwinian 權重進化（含透明度改善）

**使用時機**:
- 每月模型績效評估
- 新實驗提案時，選擇合適的投資模型
- 市場環境變化時，調整模型權重

**對應文件**:
- `.claude/skills/atlas-strategy-evolution/SKILL.md`
- `docs/superpowers/specs/2026-04-23-experiment-safety-improvements-design.md`
- `internal/experiment/`, `internal/portfolio/` 相關程式碼

---

### 5. atlas-operations-guide（操作指南）

**職責**: 日常運維、緊急應變、流程標準化

**涵蓋內容**:
- 每日/每週/每月運維檢查清單
- 緊急情況應變流程（系統故障、資料異常、市場熔斷）
- 實驗執行標準流程
- 監控與告警設定

**使用時機**:
- 每日開盤前檢查
- 系統異常時的應變
- 新成員 onboarding

**對應文件**:
- `.claude/skills/atlas-operations-guide/SKILL.md`
- `docs/operations_playbook.md`
- `docs/iteration_playbook.md`

---

### 6. atlas-swarm-analyst（Swarm 分析）

**職責**: 群體模擬、協同分析、結果彙整

**涵蓋內容**:
- swarm 結果解讀
- 多代理觀察整合
- 集體行為模式分析

**使用時機**:
- 需要分析 swarm 產出時
- 多代理結果互相比對時
- 彙整群體決策訊號時

**對應文件**:
- `.claude/skills/atlas-swarm-analyst/SKILL.md`

---

### 7. atlas-multi-strategy（多策略編排）

**職責**: 多策略路由、協調與組合選擇

**涵蓋內容**:
- 多策略並行與切換
- 組合配置與優先序
- 策略衝突處理

**使用時機**:
- 需要同時考慮多策略輸出時
- 配置策略組合時
- 進行策略切換判斷時

**對應文件**:
- `.claude/skills/atlas-multi-strategy/SKILL.md`

---

### 8. atlas-news-sentiment（新聞情緒）

**職責**: 新聞情緒分析、事件分類與市場影響判讀

**涵蓋內容**:
- 新聞標題與內容解讀
- 情緒分類與強度判斷
- 事件對行情的可能影響

**使用時機**:
- 新聞事件出現時
- 需要快速判斷市場情緒時
- 建立事件敘事時

**對應文件**:
- `.claude/skills/atlas-news-sentiment/SKILL.md`

---

### 9. atlas-event-driven-weights（事件驅動權重）

**職責**: 依事件調整模型或策略權重

**涵蓋內容**:
- 事件觸發的權重變更
- 權重調整回饋
- 動態校準機制

**使用時機**:
- 重大事件後需要重配權重時
- 策略對事件反應不足時
- 權重演化檢討時

**對應文件**:
- `.claude/skills/atlas-event-driven-weights/SKILL.md`

---

### 10. atlas-data-management（資料管理）

**職責**: 資料治理、保存、讀寫與一致性管理

**涵蓋內容**:
- 資料來源與保存規範
- 讀寫路徑與結構一致性
- 資料品質與可追溯性

**使用時機**:
- 變更資料結構時
- 檢查資料路徑與保存策略時
- 整理資料治理規範時

**對應文件**:
- `.claude/skills/atlas-data-management/SKILL.md`

---

### 11. atlas-dynamic-correlation（動態相關性）

**職責**: 動態相關矩陣、產業聯動與共振分析

**涵蓋內容**:
- 相關性矩陣調整
- 產業聯動觀察
- 共振與傳導分析

**使用時機**:
- 需要分析產業連動時
- 進行相關矩陣更新時
- 檢查結構性共振時

**對應文件**:
- `.claude/skills/atlas-dynamic-correlation/SKILL.md`

---

### 12. gitnexus-cli（GitNexus CLI）

**職責**: GitNexus 索引、狀態、清理與維護操作

**涵蓋內容**:
- analyze / status / clean
- wiki 生成
- repo 索引管理

**使用時機**:
- 需要更新或檢查索引時
- 要執行 GitNexus CLI 操作時

**對應文件**:
- `.claude/skills/gitnexus/gitnexus-cli/SKILL.md`

---

### 13. gitnexus-guide（GitNexus 指南）

**職責**: GitNexus 工具、資源、schema 與工作流說明

**涵蓋內容**:
- 工具總覽
- MCP / 資源使用方式
- schema 與查詢方式

**使用時機**:
- 需要理解 GitNexus 用法時
- 不確定該用哪個工具時

**對應文件**:
- `.claude/skills/gitnexus/gitnexus-guide/SKILL.md`

---

### 14. gitnexus-exploring（GitNexus 探索）

**職責**: 理解程式碼結構、執行流程與架構脈絡

**涵蓋內容**:
- 執行流程追蹤
- 呼叫關係理解
- 架構探索

**使用時機**:
- 想知道某段程式如何運作時
- 需要找呼叫鏈或流程時

**對應文件**:
- `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md`

---

### 15. gitnexus-impact-analysis（GitNexus 影響分析）

**職責**: 變更前爆炸半徑、風險與下游影響分析

**涵蓋內容**:
- 上下游依賴
- 風險等級判定
- 變更影響範圍

**使用時機**:
- 修改函式、類別、方法前
- 想知道改動會壞掉什麼時

**對應文件**:
- `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md`

---

### 16. gitnexus-debugging（GitNexus 除錯）

**職責**: 錯誤追蹤、根因分析、故障定位

**涵蓋內容**:
- 錯誤來源追查
- 執行流問題定位
- 根因與修正方向

**使用時機**:
- 出現錯誤、例外或異常行為時
- 需要查出故障源頭時

**對應文件**:
- `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md`

---

### 17. gitnexus-refactoring（GitNexus 重構）

**職責**: 安全地改名、抽取、拆分、搬移與結構重整

**涵蓋內容**:
- 符號改名
- 結構搬移
- 安全重構流程

**使用時機**:
- 需要大範圍重整程式碼時
- 要避免手動取代造成遺漏時

**對應文件**:
- `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md`

---

## 自動生成技能（generated/*）

> 這些技能為 auto-generated artifacts，通常對應 `internal/*` 模組的索引或技能包裝；內容會隨模組更新而同步。

### generated/apigateway
- `generated/apigateway`：Apigateway 模組技能

### generated/atlas
- `generated/atlas`：Atlas 模組技能

### generated/baseline
- `generated/baseline`：Baseline 模組技能

### generated/eventbus
- `generated/eventbus`：Eventbus 模組技能

### generated/experiment
- `generated/experiment`：Experiment 模組技能

### generated/industry
- `generated/industry`：Industry 模組技能

### generated/janus
- `generated/janus`：Janus 模組技能

### generated/ledger
- `generated/ledger`：Ledger 模組技能

### generated/live
- `generated/live`：Live 模組技能

### generated/marketdata
- `generated/marketdata`：Marketdata 模組技能

### generated/monitoring
- `generated/monitoring`：Monitoring 模組技能

### generated/narrative
- `generated/narrative`：Narrative 模組技能

### generated/orchestrator
- `generated/orchestrator`：Orchestrator 模組技能

### generated/portfolio
- `generated/portfolio`：Portfolio 模組技能

### generated/prism
- `generated/prism`：Prism 模組技能

### generated/realtime
- `generated/realtime`：Realtime 模組技能

### generated/risk
- `generated/risk`：Risk 模組技能

### generated/service
- `generated/service`：Service 模組技能

### generated/sim
- `generated/sim`：Sim 模組技能

### generated/spawning
- `generated/spawning`：Spawning 模組技能

### generated/tax
- `generated/tax`：Tax 模組技能

---

## 技能使用流程

### 標準決策流程

```
0. 任何變更前
   └── atlas-pre-change-protocol: 先做影響分析、風險界定、變更邊界確認

1. 每日開盤前
   ├── atlas-macro-narrative: 評估宏觀風險等級
   ├── atlas-risk-management: 決定當日倉位上限
   └── atlas-strategy-evolution: 確認當前投資模型權重

2. 重大事件發生時（戰爭、升息、地緣政治）
   ├── atlas-macro-narrative: 快速情境推導
   ├── atlas-risk-management: 決定是否需要緊急調整
   └── atlas-operations-guide: 執行應變流程

3. 每月回顧
   ├── atlas-strategy-evolution: 模型績效評估
   ├── atlas-risk-management: 回撤機制效能檢討
   └── atlas-core-architecture: 系統效能與瓶頸分析

4. 新功能開發
   ├── atlas-pre-change-protocol: 先確認變更前提與風險
   ├── atlas-core-architecture: 確認架構邊界
   ├── atlas-macro-narrative: 確認是否需要新增推導規則
   └── atlas-risk-management: 確認風險控制點
```

---

## 文件位置對照表

| 類型 | 統一位置 | 舊位置（已遷移） |
|------|---------|----------------|
| 技能文件 | `.claude/skills/**/SKILL.md` | 散落各處 |
| 自動生成技能 | `.claude/skills/generated/*/SKILL.md` | 由內部模組自動同步 |
| 設計規格 | `docs/superpowers/specs/` | `docs/superpowers/specs/`（維持）|
| 實施計劃 | `docs/superpowers/plans/` | `docs/superpowers/plans/`（維持）|
| 操作手冊 | `docs/operations_playbook.md` | `docs/operations_playbook.md`（維持）|

---

## 維護指南

### 新增技能

1. 在 `.claude/skills/` 下建立對應目錄
2. 撰寫 `SKILL.md`，包含：職責、涵蓋內容、使用時機、對應文件
3. 更新 `SKILLS-MAP.md`
4. 提交 PR

### 更新技能

1. 修改對應的 `SKILL.md`
2. 更新版本號與日期
3. 同步更新 `SKILLS-MAP.md`
4. 提交 PR

### 技能審查

- 每季度審查一次技能有效性
- 檢查是否有過時的推導規則或架構描述
- 確認與程式碼的一致性

---

*技能地圖版本: 2.0*  
*最後更新: 2026-05-29*  
*維護者: Atlas-Go AI Agent*
