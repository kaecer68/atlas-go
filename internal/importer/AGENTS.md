# Importer AGENTS.md

## 模組概述

`importer` 提供 CSV → JSONL 資料匯入（TWSE、FinMind）。

// Maturity: utility

## 關鍵符號

- CSV 解析與 JSONL 轉換邏輯

## 已知陷阱

- **CLI 工具**：非 runtime 一部分，僅供 `cmd/import-replay` 使用。
- **一次性執行**：資料匯入後無需常駐記憶體。

## 相依關係

- 僅被 `cmd/import-replay` 使用
