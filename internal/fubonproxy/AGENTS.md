# Fubonproxy AGENTS.md

## 模組概述

`fubonproxy` 提供 Fubon-proxy 生命週期管理 — 自動啟動/停止/監控 Python FastAPI 微服務。

// Maturity: evolving

## 關鍵符號

- `ProcessManager` — Python 程序管理器
- `Start()` — 啟動 fubon-proxy
- `Stop()` — 停止 fubon-proxy

## 已知陷阱

- **非致命失敗**：若 proxy 啟動失敗，僅記錄警告後繼續，不阻擋 atlas 啟動。
- **Python 依賴**：使用 `~/.config/atlas-go/.fubon-env/bin/python` 或系統 `python3`。
- **健康檢查**：透過 `/health` 端點偵測運行狀態。
- **自動重啟**：崩潰時自動重啟（3s 初始、10s backoff）。

## 相依關係

- 由 `cmd/atlas` API 模式使用
- 關閉時發送 SIGINT，等待 5 秒後強制終止
