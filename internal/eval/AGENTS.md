# Eval AGENTS.md

## 模組概述

`eval` 提供模型評估指標與可解釋性工具（Fin-Skills SK-12~15）。

// Maturity: evolving

## 關鍵符號

- `OOSR2`、`SharpeRatio`、`CumulativeReturn`、`MaxDrawdown` — 樣本外評估指標
- `PermutationImportance` — 置換特徵重要性（SK-13）
- `PartialDependence` — 偏依賴圖（SK-14）
- `FriedmanH` — 因子交互作用檢測（SK-15）
- `CheckSLRLAlignment` — 監督/強化學習獎勵錯配檢測（SK-28）

## 已知陷阱

- **Fin-Skills 規範驅動**：新增指標需對齊 Fin-Skills 規範編號。
- **被 robustness 使用**：robustness 模組依賴此處的評估函數，修改簽名需同步更新。

## 相依關係

- 被 `internal/robustness` 使用
- 被 `cmd/backtest-pipeline` 間接使用
