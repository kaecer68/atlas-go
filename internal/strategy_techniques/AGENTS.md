# AGENTS.md — internal/strategy_techniques

**成熟度**: stable (S-tier)
**模組職責**: 投資心法庫（`StrategyFrame` 規則引擎）

> 五層框架、短線指標、自我修正機制的完整規格見 **`docs/specs/strategy-techniques.md`**（尚未建立，規劃中）。本節僅保留 hot-path 陷阱。

## 核心型別

| 型別 | 功用 |
|------|------|
| `Layer` | L1-L5 分層 enum |
| `StrategyFrame` | 心法主結構 |
| `Condition` | 觸發條件（Timeframe/Source） |
| `Registry` | 心法儲存（JSON 外部化 `data/seeds/strategy_techniques.json`） |

## 已知陷阱

- 已穩定生產，L4/L5 與 LLM 歸因已接入
- 歷史 `internal/eventlogic/` 已取代
- 修改 `data/seeds/strategy_techniques.json` 需同步更新 `Registry.Load()`

## 相依關係

- `cmd/atlas/main.go` 匯入
- 與 `internal/narrative/`、`internal/portfolio/`、`internal/monitoring/` 互動
