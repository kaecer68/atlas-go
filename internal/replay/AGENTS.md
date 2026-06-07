# Replay AGENTS.md

## 模組概述

`replay` 提供 TWSE CSV 載入與 forward return 計算。

// Maturity: utility

## 關鍵符號

- TWSE CSV 載入邏輯
- Forward return 計算

## 已知陷阱

- **工具層**：非 runtime 一部分，由 experiment 流程使用。
- **資料格式**：輸入 CSV 格式需符合 TWSE 規範。

## 相依關係

- 被 `internal/experiment` 使用
