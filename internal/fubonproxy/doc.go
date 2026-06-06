// Package fubonproxy provides automatic lifecycle management for the Python fubon-proxy service.
//
// fubonproxy 負責啟動、停止、監控 fubon-proxy Python FastAPI 微服務。
// 當 atlas 以 API/server 模式啟動時，ProcessManager 會：
//   - 自動偵測 fubon-proxy 是否已在運行（透過 /health 端點）
//   - 啟動 Python 程序（使用 ~/.config/atlas-go/.fubon-env/bin/python 或系統 python3）
//   - 等待健康檢查通過
//   - 在背景 goroutine 中監控程序狀態，崩潰時自動重啟（3s 初始、10s backoff）
//   - 在 atlas 關閉時發送 SIGINT、等待 5 秒後強制終止
//
// 此模組不會阻擋 atlas 啟動：若 proxy 啟動失敗，僅記錄警告後繼續。
//
// # Maturity: evolving
package fubonproxy
