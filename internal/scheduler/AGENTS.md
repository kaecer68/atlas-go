# Scheduler AGENTS.md

## 模組概述

`scheduler` 提供 ML 模型重訓排程器 — 定時從 replay 資料重訓 OLS/ElasticNet/PCR/PLS。

// Maturity: evolving

## 關鍵符號

- `MLRetrainScheduler` — ML 重訓排程器
- `RetrainAll()` — 執行全模型重訓
- `GetLatestModel()` — 取得最新模型

## 已知陷阱

- **BackgroundTaskManager 排程**：必須透過 `BackgroundTaskManager` 註冊，禁止自行啟動 `time.Ticker`。
- **模型格式同步**：重訓產出的模型格式需與 `internal/ml` 的預期格式一致。

## 相依關係

- 依賴 `internal/ml`
- 依賴 `internal/replay`
- 由 `BackgroundTaskManager` 排程執行
