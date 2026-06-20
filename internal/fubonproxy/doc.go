// Package fubonproxy provides automatic lifecycle management for the Python fubon-proxy service.
//
// fubonproxy 負責啟動、停止、監控 fubon-proxy Python FastAPI 微服務。
// 當 atlas 以 API/server 模式啟動時，ProcessManager 會：
//   - 自動偵測 fubon-proxy 是否已在運行（透過 /health 端點）
//   - 啟動前先探測 :8081 連接埠：port 空時才 spawn；port 已由 healthy fubon-proxy
//     佔用時跳過 spawn（標記為外部已管理）；port 被未知 process 佔用時回傳
//     actionable error（含 PID 與 kill 指令），避免 supervisor 進入無窮 backoff-loop
//   - 啟動 Python 程序（使用 ~/.config/atlas-go/.fubon-env/bin/python 或系統 python3）
//   - 非同步等待健康檢查通過（non-blocking supervisor pattern：Start() 立即返回，
//     健康檢查在背景 goroutine 中進行；超時僅記錄警告，不阻擋 atlas 啟動）
//   - 在背景 goroutine 中監控程序狀態，崩潰時同步等待健康檢查後自動重啟
//     （3s 初始 backoff、10s 重試 backoff；僅在健康通過時重置為初始值）
//   - 在 atlas 關閉時發送 SIGINT、等待 5 秒後強制終止
//
// 此模組不會阻擋 atlas 啟動：若 proxy 啟動失敗，僅記錄警告後繼續。
//
// 注意：`Stop()` 為硬殺（hard kill）而非優雅關閉。`exec.CommandContext` 在 cancel 時
// 會由 Go runtime 內部送出 SIGKILL，因此後續的 SIGINT + 5s graceful timeout
// 屬於雙重安全網，正常路徑不會觸發。
//
// Maturity: evolving
package fubonproxy
