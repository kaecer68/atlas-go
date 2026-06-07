# ML AGENTS.md

## 模組概述

`ml` 提供監督式學習模型（Fin-Skills SK-05~09）— OLS、ElasticNet、PCR、PLS。

// Maturity: evolving

## 關鍵符號

- `Model` — 模型介面
- `Trainer` — 訓練器
- OLS、ElasticNet、PCR、PLS 實作

## 已知陷阱

- **Fin-Skills 規範驅動**：模型實作需對齊 Fin-Skills SK-05~09 規範。
- **gonum 依賴**：矩陣運算使用 `gonum.org/v1/gonum/mat`，禁止引入外部 ML 函式庫。
- **被 scheduler 使用**：`scheduler` 會定時重訓這些模型，修改儲存格式需同步更新。

## 相依關係

- 被 `internal/scheduler`（MLRetrainScheduler）使用
- 被 factor/research 相關流程使用
