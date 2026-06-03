# Atlas Skills Map

**版本**: 3.0
**日期**: 2026-06-03
**用途**: Atlas-Go AI 技能索引

---

## 技能架構

```
.claude/skills/
├── atlas-pre-change-protocol/     # 變更前協議（所有修改前必用）
├── atlas-macro-narrative/         # 宏觀敘事：六大維度推導外資流向
├── atlas-risk-management/         # 風險管理：四層架構、動態倉位、VaR
├── atlas-strategy-evolution/      # 策略進化：模型績效、Darwinian 權重
├── atlas-swarm-analyst/           # Swarm 分析：MiroFish 模擬結果解讀
├── atlas-multi-strategy/          # 多策略編排：策略選擇、切換、比較
├── atlas-event-driven-weights/    # 事件驅動權重：FactorEngine 動態調整
├── generated/                     # 21 個自動生成技能（對應 internal/* 模組）
│   ├── apigateway/  atlas/  baseline/  eventbus/
│   ├── experiment/  industry/  janus/  ledger/
│   ├── live/  marketdata/  monitoring/  narrative/
│   ├── orchestrator/  portfolio/  prism/  realtime/
│   └── risk/  service/  sim/  spawning/  tax/
└── gitnexus/                      # 6 個 GitNexus 技能
    ├── gitnexus-cli/  gitnexus-guide/
    ├── gitnexus-exploring/  gitnexus-impact-analysis/
    └── gitnexus-debugging/  gitnexus-refactoring/
```

---

## 技能分類

### 變更前必用
- `atlas-pre-change-protocol`：7 步驟檢查清單，禁止跳過

### 核心分析技能（7 個）
| 技能 | 用途 | 實作狀態 |
|------|------|----------|
| `atlas-macro-narrative` | 宏觀敘事、外資流向、六大維度推導 | ✅ |
| `atlas-risk-management` | 風險閘門、VaR、回撤、自主校準 | ✅ |
| `atlas-strategy-evolution` | 策略進化、模型績效、mutation brief | ✅ |
| `atlas-swarm-analyst` | Swarm 模擬結果、市場共識、異常偵測 | ✅ |
| `atlas-multi-strategy` | 策略選擇器、分配器、策略比較 | ✅ |
| `atlas-event-driven-weights` | 事件驅動因子權重、FactorBridge | ⚠️ 部分實作 |

### 自動生成技能（generated/*）
對應 `internal/*` 模組，由工具自動同步，提供模組級程式碼導航。

### GitNexus 技能（6 個）
CLI 操作、工具指南、程式碼探索、影響分析、除錯、重構。

---

## 已搬移的設計文件

以下原本是 skill 檔案，已移至 `docs/investor-ui/`：
- `investor-ui.md` — UI 核心架構與設計原則
- `investor-pages.md` — 6 頁面 wireframe 規格
- `investor-nlg.md` — NLG 推薦解釋層設計
- `investor-trustscore.md` — TrustScore 信任分數系統設計
- `investor-roadmap.md` — Phase A/B 實作路線圖

## 已刪除的重複/藍圖技能

以下技能因與 AGENTS.md、docs/、internal/*/AGENTS.md 重複，或為未實作的純藍圖，已移除：
`core-architecture`、`data-management`、`operations-guide`、`fin-backtest-engine`、`fin-ml-pipeline`、`fin-model-eval`、`fin-robustness`、`dynamic-correlation`、`news-sentiment`

---

## 技能使用流程

```
修改任何程式碼前
  └── atlas-pre-change-protocol（強制）

領域相關工作
  ├── 風險/倉位 → atlas-risk-management
  ├── 宏觀/敘事 → atlas-macro-narrative
  ├── 策略/進化 → atlas-strategy-evolution
  ├── 權重/因子 → atlas-event-driven-weights
  ├── 策略選擇   → atlas-multi-strategy
  └── Swarm 分析 → atlas-swarm-analyst

程式碼導航
  └── generated/* 或 gitnexus/*
```
