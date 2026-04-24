# Atlas Skills Map

**版本**: 1.0  
**日期**: 2026-04-23  
**用途**: 統一管理 Atlas-Go 應用的所有 AI 技能與設計文件  

---

## 技能架構總覽

```
.claude/skills/
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
└── atlas-operations-guide/           # 操作指南技能
    └── SKILL.md                      # 日常運維、實驗流程、緊急應變
```

---

## 技能詳細說明

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
- `docs/ai-agent-architecture.md`
- `AGENTS.md`

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
- `docs/operations-playbook.md`
- `docs/iteration-playbook.md`

---

## 技能使用流程

### 標準決策流程

```
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
   ├── atlas-core-architecture: 確認架構邊界
   ├── atlas-macro-narrative: 確認是否需要新增推導規則
   └── atlas-risk-management: 確認風險控制點
```

---

## 文件位置對照表

| 類型 | 統一位置 | 舊位置（已遷移） |
|------|---------|----------------|
| 技能文件 | `.claude/skills/atlas-*/SKILL.md` | 散落各處 |
| 設計規格 | `docs/superpowers/specs/` | `docs/superpowers/specs/`（維持）|
| 實施計劃 | `docs/superpowers/plans/` | `docs/superpowers/plans/`（維持）|
| 操作手冊 | `docs/operations-playbook.md` | `docs/operations-playbook.md`（維持）|

---

## 維護指南

### 新增技能

1. 在 `.claude/skills/` 下建立 `atlas-{name}/` 目錄
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

*技能地圖版本: 1.0*  
*最後更新: 2026-04-23*  
*維護者: Atlas-Go AI Agent*
