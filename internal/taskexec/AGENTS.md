# Taskexec AGENTS.md

## 模組概述

`taskexec` 提供非同步任務執行 — `Manager`、`Cancel`、`Subscribe`。

// Maturity: utility

## 關鍵符號

- `Manager` — 任務管理器
- `Cancel` — 取消任務
- `Subscribe` — 訂閱任務事件

## 已知陷阱

- **輔助基礎設施**：非核心 runtime，提供通用任務執行能力。
- **錯誤處理**：非同步任務的錯誤需透過訂閱機制傳遞，不可靜默丟失。

## 相依關係

- 被多個模組作為通用基礎設施使用
