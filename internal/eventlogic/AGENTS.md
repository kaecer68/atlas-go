# Eventlogic AGENTS.md

## 模組概述

`eventlogic` 提供系統事件處理規則引擎。

// Maturity: stable

## 關鍵符號

- 事件規則處理邏輯（具體符號見原始碼）

## 已知陷阱

- **穩定生產**：處於生產執行路徑，breaking change 需 migration plan。

## 相依關係

- 被 `cmd/atlas/main.go` 直接匯入
