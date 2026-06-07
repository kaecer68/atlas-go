# Robustness AGENTS.md

## 模組概述

`robustness` 提供穩健性與敏感度測試（Fin-Skills SK-20~22）。

// Maturity: experimental

## 關鍵符號

- `Model` — 穩健性測試模型
- `SizeGroupReport` — 規模分組報告
- SizeGroup、PennyExclusion、Ablation 測試

## 已知陷阱

- **實驗性質**：API 不穩定，不應被 stable/evolving 模組依賴。
- **Fin-Skills 驅動**：測試項目需對齊 Fin-Skills SK-20~22。
- **依賴 eval**：使用 `internal/eval` 的評估指標，簽名變更需同步更新。

## 相依關係

- 依賴 `internal/eval`
