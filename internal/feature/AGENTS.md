# Feature AGENTS.md

## 模組概述

`feature` 提供命名特徵萃取（close, volume, return_1d/5d, hl_ratio, ma_ratio, volume_ratio）。

// Maturity: evolving

## 關鍵符號

- `Registry` — 特徵註冊表
- `MakeExtractor` — 特徵提取器工廠
- `ForwardReturnLabel` — 未來收益標籤

## 已知陷阱

- **共用模組**：由 `cmd/backtest-pipeline`（CLI）與 `internal/experiment`（Judge）共用。
- **簽名變更**：修改 `MakeExtractor` 簽名需同步更新兩個使用者。

## 相依關係

- 被 `cmd/backtest-pipeline` 使用
- 被 `internal/experiment`（Judge 重要性運算）使用
